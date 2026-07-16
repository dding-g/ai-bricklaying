//go:build windows

package safeio

// Windows does not expose a portable directory fsync through os.File.Sync.
// The file itself is flushed before installation, and the atomic hard-link or
// rename operation remains crash-safe to the guarantees of the filesystem.
func syncDir(string) error {
	return nil
}
