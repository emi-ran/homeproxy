//go:build !windows

package main

import "homeproxy/internal/proxy"

func run(args []string) error { return proxy.RunCLI(args) }
