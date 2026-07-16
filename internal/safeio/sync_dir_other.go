//go:build !aix && !darwin && !dragonfly && !freebsd && !illumos && !linux && !netbsd && !openbsd && !solaris && !windows

package safeio

// Directory synchronization is not available through a portable Go API on
// this platform. File contents are still flushed before atomic installation.
func syncDir(string) error {
	return nil
}
