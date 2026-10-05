//go:build windows

package proxy

import (
	"crypto/sha256"
	"fmt"
	"golang.org/x/sys/windows"
	"strings"
)

// acquireInstanceLock attempts to create a named Windows mutex for single-instance isolation.
// If an instance with this name is already running, it returns (nil, false).
func acquireInstanceLock(name string) (func(), bool) {
	// Global namespace prevents console/service duplicates across login sessions.
	name = fmt.Sprintf("Global\\HomeProxyAgent_%x", sha256.Sum256([]byte(strings.TrimPrefix(name, "Local\\HomeProxyAgent_"))))
	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return nil, false
	}
	h, err := windows.CreateMutex(nil, false, namePtr)
	if err == windows.ERROR_ALREADY_EXISTS {
		if h != 0 {
			windows.CloseHandle(h)
		}
		return nil, false
	}
	if err != nil {
		return nil, false
	}
	unlock := func() {
		windows.CloseHandle(h)
	}
	return unlock, true
}
