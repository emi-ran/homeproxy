//go:build windows

package proxy

import (
	"golang.org/x/sys/windows"
)

// acquireInstanceLock attempts to create a named Windows mutex for single-instance isolation.
// If an instance with this name is already running, it returns (nil, false).
func acquireInstanceLock(name string) (func(), bool) {
	namePtr, err := windows.UTF16PtrFromString(name)
	if err != nil {
		return func() {}, true
	}
	h, err := windows.CreateMutex(nil, false, namePtr)
	if err == windows.ERROR_ALREADY_EXISTS {
		if h != 0 {
			windows.CloseHandle(h)
		}
		return nil, false
	}
	if err != nil {
		return func() {}, true
	}
	unlock := func() {
		windows.CloseHandle(h)
	}
	return unlock, true
}
