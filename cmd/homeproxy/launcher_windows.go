//go:build windows

package main

import (
	"fmt"
	"homeproxy/internal/proxy"
	"homeproxy/internal/windowsagent"
	"os"
	"os/exec"
	"path/filepath"
)

func run(args []string) error {
	if len(args) > 0 {
		if args[0] == "service" {
			return windowsagent.Run(args[1:])
		}
		return proxy.RunCLI(args)
	}
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	gui := filepath.Join(filepath.Dir(exe), "homeproxy-gui.exe")
	if _, err := os.Stat(gui); err != nil {
		return fmt.Errorf("Windows GUI missing: %s; package Flutter runner as homeproxy-gui.exe, or use agent/server/select/service", gui)
	}
	return exec.Command(gui).Start()
}
