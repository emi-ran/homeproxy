//go:build !windows

package main

import (
	"os"
	"os/exec"
	"syscall"
	"testing"
	"time"
)

func TestSIGTERMShutdown(t *testing.T) {
	if os.Getenv("HOMEPROXY_SIGNAL_HELPER") == "1" {
		os.Args = []string{"homeproxy", "agent", "-id", "test", "-server-name", "localhost"}
		if err := cli(); err != nil {
			t.Fatal(err)
		}
		return
	}
	cmd := exec.Command(os.Args[0], "-test.run=^TestSIGTERMShutdown$")
	cmd.Env = append(os.Environ(), "HOMEPROXY_SIGNAL_HELPER=1", "HOMEPROXY_TOKEN=0123456789abcdef")
	if err := cmd.Start(); err != nil {
		t.Fatal(err)
	}
	defer cmd.Process.Kill()
	time.Sleep(200 * time.Millisecond)
	if err := cmd.Process.Signal(syscall.SIGTERM); err != nil {
		t.Fatal(err)
	}
	done := make(chan error, 1)
	go func() { done <- cmd.Wait() }()
	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("SIGTERM not graceful: %v", err)
		}
	case <-time.After(2 * time.Second):
		t.Fatal("SIGTERM ignored")
	}
}
