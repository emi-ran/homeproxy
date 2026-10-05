//go:build windows

package windowsagent

import (
	"golang.org/x/sys/windows"
	"homeproxy/internal/proxy"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestPipeAuthorizationDescriptor(t *testing.T) {
	for _, sid := range []string{"", "WD", "S-1-1-0", "S-1-5-32-545", "S-1-5-21-1)(A;;GA;;;WD)"} {
		if _, err := pipeSDDL(sid); err == nil {
			t.Fatalf("accepted %q", sid)
		}
	}
	sd, err := pipeSDDL("S-1-5-21-1-2-3-1001")
	if err != nil {
		t.Fatal(err)
	}
	if _, err = windows.SecurityDescriptorFromString(sd); err != nil {
		t.Fatal(err)
	}
	if strings.Contains(sd, ";;;WD") || strings.Contains(sd, ";;;BU") {
		t.Fatal("broad pipe access")
	}
	c := &controller{}
	if c.apply(Request{Version: 2, Op: "status"}).OK {
		t.Fatal("version accepted")
	}
	if c.apply(Request{Version: 1, Op: "install"}).OK {
		t.Fatal("privileged IPC accepted")
	}
}

func TestEncryptedSettings(t *testing.T) {
	dir := t.TempDir()
	cfg := Config{Address: "example.com:4433", ID: "pc", Token: "test-secret-0123456789", Insecure: true}
	if err := saveConfig(dir, cfg); err != nil {
		t.Fatal(err)
	}
	b, err := os.ReadFile(filepath.Join(dir, "settings.dpapi"))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(b), cfg.Token) {
		t.Fatal("plaintext secret stored")
	}
	got, err := loadConfig(dir)
	if err != nil || got != cfg {
		t.Fatalf("round trip: %v", err)
	}
	cfg.Enabled = true
	if err := saveConfig(dir, cfg); err != nil {
		t.Fatal(err)
	}
	got, err = loadConfig(dir)
	if err != nil || !got.Enabled {
		t.Fatalf("replace: %v", err)
	}
	if err := os.WriteFile(filepath.Join(dir, "settings.dpapi"), []byte("corrupt"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = loadConfig(dir); err == nil {
		t.Fatal("accepted corrupted settings")
	}
}

func TestIPCConfigRedactionAndDisconnect(t *testing.T) {
	c := &controller{dir: t.TempDir(), agent: proxy.NewMobileAgent()}
	defer c.agent.Stop()
	if c.apply(Request{Version: 1, Op: "connect"}).OK {
		t.Fatal("unconfigured connect accepted")
	}
	cfg := Config{Address: "example.com:4433", ID: "pc", Token: "test-secret-0123456789", Insecure: true}
	r := c.apply(Request{Version: 1, Op: "set_config", Config: &cfg})
	if !r.OK || r.Config == nil || r.Config.Token != "" {
		t.Fatal("config save/redaction")
	}
	cfg.Token = ""
	cfg.ID = "pc-new"
	if !c.apply(Request{Version: 1, Op: "set_config", Config: &cfg}).OK {
		t.Fatal("retained token update")
	}
	if !c.apply(Request{Version: 1, Op: "disconnect"}).OK {
		t.Fatal("disconnect")
	}
	saved, err := loadConfig(c.dir)
	if err != nil || saved.Enabled || saved.Token != "test-secret-0123456789" {
		t.Fatal("persisted disabled state/token retention")
	}
}
