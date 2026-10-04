//go:build !windows

package proxy

// acquireInstanceLock is a no-op fallback on non-Windows platforms.
func acquireInstanceLock(name string) (func(), bool) {
	return func() {}, true
}
