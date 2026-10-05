//go:build windows

package windowsagent

import (
	"errors"
	"fmt"
	"golang.org/x/sys/windows"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/mgr"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

func Run(args []string) error {
	if len(args) == 0 {
		return errors.New("use service install --user-sid SID, uninstall, start, stop, status, or run")
	}
	if args[0] == "run" {
		yes, err := svc.IsWindowsService()
		if err != nil {
			return err
		}
		if !yes {
			return errors.New("service run is SCM-only")
		}
		return svc.Run(ServiceName, &handler{})
	}
	if args[0] == "install" || args[0] == "uninstall" {
		if !windows.GetCurrentProcessToken().IsElevated() {
			return errors.New("installation/removal requires UAC elevation; run service command from elevated PowerShell (GUI must use ShellExecuteEx runas)")
		}
	}
	m, err := mgr.Connect()
	if err != nil {
		return err
	}
	defer m.Disconnect()
	if args[0] == "install" {
		if len(args) != 3 || args[1] != "--user-sid" {
			return errors.New("install requires --user-sid for explicitly authorized interactive user")
		}
		sid, err := windows.StringToSid(args[2])
		if err != nil {
			return err
		}
		if !strings.HasPrefix(sid.String(), "S-1-5-21-") {
			return errors.New("only explicit local/domain user SIDs allowed")
		}
		_, _, kind, err := sid.LookupAccount("")
		if err != nil || kind != windows.SidTypeUser {
			return errors.New("authorized SID must resolve to a user account, not a group")
		}
		if s, e := m.OpenService(ServiceName); e == nil {
			s.Close()
			return errors.New("service already installed; uninstall first, settings retained")
		}
		binary, dir, err := paths()
		if err != nil {
			return err
		}
		if err = os.MkdirAll(filepath.Dir(binary), 0755); err != nil {
			return err
		}
		if err = protectPath(filepath.Dir(binary), "D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;GRGX;;;BU)"); err != nil {
			return err
		}
		src, err := os.Executable()
		if err != nil {
			return err
		}
		in, err := os.Open(src)
		if err != nil {
			return err
		}
		defer in.Close()
		out, err := os.OpenFile(binary, os.O_CREATE|os.O_EXCL|os.O_WRONLY, 0755)
		if err != nil {
			return fmt.Errorf("destination already exists or cannot be created: %w", err)
		}
		installed := false
		defer func() {
			if !installed {
				os.Remove(binary)
			}
		}()
		_, err = io.Copy(out, in)
		ce := out.Close()
		if err != nil {
			return err
		}
		if ce != nil {
			return ce
		}
		s, err := m.CreateService(ServiceName, binary, mgr.Config{DisplayName: "HomeProxy Agent", StartType: mgr.StartAutomatic, ServiceStartName: `NT SERVICE\HomeProxyAgent`, Description: "HomeProxy outbound QUIC agent and local GUI control"}, "service", "run")
		if err != nil {
			return err
		}
		defer s.Close()
		defer func() {
			if !installed {
				s.Delete()
			}
		}()
		// No Start here: explicit separate GUI action starts installed service.
		account, _, _, err := windows.LookupSID("", `NT SERVICE\HomeProxyAgent`)
		if err != nil {
			return err
		}
		if err = protectPath(filepath.Dir(binary), "D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;GRGX;;;BU)(A;OICI;GRGX;;;"+account.String()+")"); err != nil {
			return err
		}
		if err = os.MkdirAll(dir, 0700); err != nil {
			return err
		}
		if err = protectPath(dir, "D:P(A;OICI;FA;;;SY)(A;OICI;FA;;;BA)(A;OICI;FA;;;"+account.String()+")"); err != nil {
			return err
		}
		if err = os.WriteFile(filepath.Join(dir, "owner.sid"), []byte(sid.String()), 0600); err != nil {
			return err
		}
		if err = s.SetRecoveryActions([]mgr.RecoveryAction{{Type: mgr.ServiceRestart, Delay: 5 * time.Second}, {Type: mgr.ServiceRestart, Delay: 30 * time.Second}}, 86400); err != nil {
			return err
		}
		if err = s.SetRecoveryActionsOnNonCrashFailures(false); err != nil {
			return err
		}
		installed = true
		return nil
	}
	s, err := m.OpenService(ServiceName)
	if err != nil {
		return err
	}
	defer s.Close()
	switch args[0] {
	case "start":
		return s.Start()
	case "stop":
		_, err = s.Control(svc.Stop)
		return err
	case "status":
		st, err := s.Query()
		if err == nil {
			fmt.Printf("SCM state: %d\n", st.State)
		}
		return err
	case "uninstall":
		st, err := s.Query()
		if err != nil {
			return err
		}
		if st.State != svc.Stopped {
			return errors.New("stop service and wait for Stopped before uninstall")
		}
		if err = s.Delete(); err != nil {
			return err
		}
		binary, _, err := paths()
		if err != nil {
			return err
		}
		return os.Remove(binary)
	default:
		return errors.New("unknown service command")
	}
}

type handler struct{}

func (*handler) Execute(_ []string, changes <-chan svc.ChangeRequest, statuses chan<- svc.Status) (bool, uint32) {
	statuses <- svc.Status{State: svc.StartPending}
	_, dir, err := paths()
	if err != nil {
		return true, 1
	}
	controller, err := newController(dir)
	if err != nil {
		return true, 2
	}
	defer controller.close()
	statuses <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	for {
		select {
		case err := <-controller.failed:
			if err != nil {
				return true, 3
			}
		case c := <-changes:
			switch c.Cmd {
			case svc.Interrogate:
				statuses <- c.CurrentStatus
			case svc.Stop, svc.Shutdown:
				statuses <- svc.Status{State: svc.StopPending}
				return false, 0
			}
		}
	}
}
