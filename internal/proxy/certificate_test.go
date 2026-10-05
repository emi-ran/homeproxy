package proxy

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPersistentCertificate(t *testing.T) {
	path := filepath.Join(t.TempDir(), "state", "server-tls.pem")
	first, err := loadOrCreateCertificate(path)
	if err != nil {
		t.Fatal(err)
	}
	second, err := loadOrCreateCertificate(path)
	if err != nil {
		t.Fatal(err)
	}
	if certificateFingerprint(first) != certificateFingerprint(second) {
		t.Fatal("certificate changed on reload")
	}
	if err = os.WriteFile(path, []byte("broken"), 0600); err != nil {
		t.Fatal(err)
	}
	if _, err = loadOrCreateCertificate(path); err == nil {
		t.Fatal("corrupt certificate replaced")
	}
	b, err := os.ReadFile(path)
	if err != nil || string(b) != "broken" {
		t.Fatal("corrupt state modified")
	}
}
