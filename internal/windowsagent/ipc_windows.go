//go:build windows

package windowsagent

import (
	"encoding/json"
	"errors"
	"github.com/Microsoft/go-winio"
	"golang.org/x/sys/windows"
	"homeproxy/internal/proxy"
	"io"
	"net"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
)

func pipeSDDL(owner string) (string, error) {
	sid, err := windows.StringToSid(owner)
	if err != nil || !strings.HasPrefix(owner, "S-1-5-21-") {
		return "", errors.New("invalid authorized user SID")
	}
	// Specific read/write rights omit FILE_CREATE_PIPE_INSTANCE, unlike GENERIC_WRITE.
	return "D:P(A;;GA;;;SY)(A;;GA;;;BA)(A;;0x12019b;;;" + sid.String() + ")", nil
}

type controller struct {
	mu         sync.Mutex
	dir        string
	config     Config
	configured bool
	started    bool // Active run includes connecting and reconnect backoff.
	agent      *proxy.MobileAgent
	listener   net.Listener
	failed     chan error
	done       chan struct{}
}

func newController(dir string) (*controller, error) {
	b, err := os.ReadFile(filepath.Join(dir, "owner.sid"))
	if err != nil {
		return nil, err
	}
	sd, err := pipeSDDL(string(b))
	if err != nil {
		return nil, err
	}
	sid, _, _, err := windows.LookupSID("", `NT SERVICE\HomeProxyAgent`)
	if err != nil {
		return nil, err
	}
	sd += "(A;;GA;;;" + sid.String() + ")"
	l, err := winio.ListenPipe(PipePath, &winio.PipeConfig{SecurityDescriptor: sd, InputBufferSize: 16384, OutputBufferSize: 16384})
	if err != nil {
		return nil, err
	}
	c := &controller{dir: dir, agent: proxy.NewMobileAgent(), listener: l, failed: make(chan error, 1), done: make(chan struct{})}
	cfg, err := loadConfig(dir)
	if err == nil {
		c.config, c.configured = cfg, true
		if cfg.Enabled {
			if err = c.agent.StartWithTLS(cfg.Address, cfg.ID, cfg.Token, cfg.Fingerprint, cfg.Insecure); err != nil {
				l.Close()
				return nil, err
			}
			c.started = true
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		l.Close()
		return nil, errors.New("settings unreadable; repair encrypted settings before starting service")
	}
	go c.serve()
	return c, nil
}

func (c *controller) close() {
	c.listener.Close()
	<-c.done
	c.mu.Lock()
	defer c.mu.Unlock()
	c.agent.Stop()
	c.started = false
}

// transitionConfig is called with mu held. Persist before disturbing a live
// run; a failed start leaves started false so a subsequent connect can recover.
func (c *controller) transitionConfig(desired Config) error {
	plan, err := planConfigTransition(c.config, c.configured, desired, c.started)
	if err != nil {
		return err
	}
	if plan.Save {
		if err := saveConfig(c.dir, plan.Config); err != nil {
			return errors.New("cannot save encrypted settings")
		}
	}
	if plan.Stop {
		c.agent.Stop()
		c.started = false
	}
	c.config, c.configured = plan.Config, true
	if plan.Start {
		cfg := plan.Config
		if err := c.agent.StartWithTLS(cfg.Address, cfg.ID, cfg.Token, cfg.Fingerprint, cfg.Insecure); err != nil {
			return errors.New("agent start failed")
		}
		c.started = true
	}
	return nil
}
func (c *controller) serve() {
	defer close(c.done)
	// Serial bounded connections avoid unbounded local goroutines. Deadline limits blocked callers.
	for {
		conn, err := c.listener.Accept()
		if err != nil {
			if !errors.Is(err, net.ErrClosed) {
				c.failed <- err
			}
			return
		}
		conn.SetDeadline(time.Now().Add(5 * time.Second))
		var req Request
		d := json.NewDecoder(io.LimitReader(conn, 16384))
		d.DisallowUnknownFields()
		res := Response{Version: 1}
		if err := d.Decode(&req); err != nil {
			res.Error = "invalid request"
		} else {
			res = c.apply(req)
		}
		json.NewEncoder(conn).Encode(res)
		conn.Close()
	}
}

func (c *controller) apply(r Request) Response {
	c.mu.Lock()
	defer c.mu.Unlock()
	res := Response{Version: 1}
	fail := func(s string) Response { res.Error = s; return res }
	if r.Version != 1 {
		return fail("unsupported version")
	}
	switch r.Op {
	case "status", "get_config":
	case "set_config":
		if r.Config == nil {
			return fail("config required")
		}
		if err := c.transitionConfig(*r.Config); err != nil {
			return fail(err.Error())
		}
	case "connect", "disconnect":
		if !c.configured {
			return fail("configure agent first")
		}
		cfg := c.config
		cfg.Enabled = r.Op == "connect"
		if err := c.transitionConfig(cfg); err != nil {
			return fail(err.Error())
		}
	default:
		return fail("unknown operation")
	}
	res.OK = true
	res.Status = c.agent.Status()
	if c.configured {
		cfg := c.config
		cfg.Token = ""
		res.Config = &cfg
	}
	return res
}
