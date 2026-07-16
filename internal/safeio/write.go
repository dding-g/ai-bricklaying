package safeio

import (
	"errors"
	"fmt"
	"io/fs"
	"math/rand/v2"
	"os"
	"path/filepath"
	"runtime"
	"time"
)

const (
	PublicFileMode  fs.FileMode = 0o644
	PrivateDirMode  fs.FileMode = 0o700
	PrivateFileMode fs.FileMode = 0o600
)

var ErrSymlinkTarget = errors.New("Refusing to overwrite symbolic link")
var ErrTargetExists = errors.New("refusing to overwrite existing file")

type WriteOptions struct {
	FileMode  fs.FileMode
	DirMode   fs.FileMode
	NoClobber bool
}

func WriteFile(path string, contents []byte, options WriteOptions) error {
	fileMode := options.FileMode
	if fileMode == 0 {
		fileMode = PublicFileMode
	}
	dirMode := options.DirMode
	if dirMode == 0 {
		dirMode = 0o755
	}

	dir := filepath.Dir(path)
	if err := RejectSymlinkAncestors(path); err != nil {
		return err
	}
	if err := rejectTargetSymlink(path); err != nil {
		return err
	}
	if err := EnsureDir(dir, dirMode); err != nil {
		return err
	}
	if err := RejectSymlinkAncestors(path); err != nil {
		return err
	}
	if err := rejectTargetSymlink(path); err != nil {
		return err
	}

	tempPath := filepath.Join(dir, tempName(filepath.Base(path)))
	file, err := os.OpenFile(tempPath, os.O_WRONLY|os.O_CREATE|os.O_EXCL, fileMode)
	if err != nil {
		return err
	}
	closed := false
	cleanup := true
	defer func() {
		if !closed {
			_ = file.Close()
		}
		if cleanup {
			_ = os.Remove(tempPath)
		}
	}()

	if _, err := file.Write(contents); err != nil {
		return err
	}
	if err := file.Chmod(fileMode); err != nil && runtime.GOOS != "windows" {
		return err
	}
	// Persist the contents and requested mode before making the temporary file
	// visible at the destination. A directory sync below then persists the
	// namespace update which installs the file.
	if err := file.Sync(); err != nil {
		return err
	}
	if err := file.Close(); err != nil {
		closed = true
		return err
	}
	closed = true

	if err := RejectSymlinkAncestors(path); err != nil {
		return err
	}
	if err := rejectTargetSymlink(path); err != nil {
		return err
	}
	if options.NoClobber {
		if err := installNoClobber(tempPath, path); err != nil {
			return err
		}
		// The destination is committed once Link succeeds. Temp cleanup is
		// best-effort from this point so callers never receive an ordinary
		// failure after a complete destination has become visible. Keeping
		// cleanup true lets the deferred removal retry once on transient
		// Windows scanner/filesystem interference.
		if err := os.Remove(tempPath); err == nil {
			cleanup = false
		}
	} else {
		if err := os.Rename(tempPath, path); err != nil {
			return err
		}
		cleanup = false
	}
	if err := syncDir(dir); err != nil {
		return err
	}
	return nil
}

// installNoClobber atomically publishes a completed temporary file without
// replacing an entry created by another process. The temporary file lives in
// the destination directory, so a hard link is an atomic, same-filesystem
// create-if-absent operation on POSIX systems and Windows filesystems which
// support hard links. Unsupported filesystems fail closed instead of falling
// back to a check-then-rename sequence.
func installNoClobber(tempPath, path string) error {
	if err := os.Link(tempPath, path); err != nil {
		if errors.Is(err, fs.ErrExist) {
			return fmt.Errorf("%w: %s", ErrTargetExists, path)
		}
		return err
	}
	return nil
}

// RejectSymlinkAncestors rejects a path when the target or any existing
// user-scoped ancestor is a symbolic link. Direct children of the filesystem
// root are treated as trusted platform aliases (for example macOS /var and
// /tmp). Callers use this before creating directories so a configured output
// root cannot redirect a write outside its apparent tree.
func RejectSymlinkAncestors(path string) error {
	current, err := filepath.Abs(path)
	if err != nil {
		return err
	}
	current = filepath.Clean(current)
	for {
		info, statErr := os.Lstat(current)
		if statErr == nil && info.Mode()&fs.ModeSymlink != 0 {
			parent := filepath.Dir(current)
			// macOS exposes a small set of root-level system aliases such as
			// /var -> private/var. Do not trust arbitrary root-child symlinks:
			// they can be user-controlled in containers, mounts, and on Windows.
			if !isTrustedPlatformAlias(current, parent) {
				return fmt.Errorf("%w: %s", ErrSymlinkTarget, current)
			}
		}
		if statErr != nil && !errors.Is(statErr, fs.ErrNotExist) {
			return statErr
		}
		parent := filepath.Dir(current)
		if parent == current {
			return nil
		}
		current = parent
	}
}

func rejectTargetSymlink(path string) error {
	info, err := os.Lstat(path)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return err
	}
	if info.Mode()&fs.ModeSymlink != 0 {
		return fmt.Errorf("%w: %s", ErrSymlinkTarget, path)
	}
	return nil
}

func WriteConfigFile(path string, contents []byte) error {
	return WriteFile(path, contents, WriteOptions{FileMode: PrivateFileMode, DirMode: PrivateDirMode})
}

func EnsureDir(path string, mode fs.FileMode) error {
	if mode == 0 {
		mode = 0o755
	}
	missing, err := missingDirectories(path)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(path, mode); err != nil {
		return err
	}
	if err := os.Chmod(path, mode); err != nil && runtime.GOOS != "windows" {
		return err
	}
	// Mkdir durability requires syncing the directory containing each newly
	// created entry. Sync from the highest missing component downward so a
	// successful return does not depend only on the final directory's fsync.
	for index := len(missing) - 1; index >= 0; index-- {
		if err := syncDir(filepath.Dir(missing[index])); err != nil {
			return err
		}
	}
	if len(missing) > 0 {
		if err := syncDir(path); err != nil {
			return err
		}
	}
	return nil
}

func missingDirectories(path string) ([]string, error) {
	current, err := filepath.Abs(path)
	if err != nil {
		return nil, err
	}
	current = filepath.Clean(current)
	var missing []string
	for {
		info, statErr := os.Lstat(current)
		if statErr == nil {
			if info.Mode()&fs.ModeSymlink != 0 && isTrustedPlatformAlias(current, filepath.Dir(current)) {
				resolved, resolveErr := os.Stat(current)
				if resolveErr != nil {
					return nil, resolveErr
				}
				if resolved.IsDir() {
					return missing, nil
				}
			}
			if !info.IsDir() {
				return nil, &fs.PathError{Op: "mkdir", Path: current, Err: fmt.Errorf("not a directory")}
			}
			return missing, nil
		}
		if !errors.Is(statErr, fs.ErrNotExist) {
			return nil, statErr
		}
		missing = append(missing, current)
		parent := filepath.Dir(current)
		if parent == current {
			return missing, nil
		}
		current = parent
	}
}

func isTrustedPlatformAlias(path, parent string) bool {
	if runtime.GOOS != "darwin" || filepath.Dir(parent) != parent {
		return false
	}
	switch filepath.Clean(path) {
	case string(filepath.Separator) + "etc", string(filepath.Separator) + "tmp", string(filepath.Separator) + "var":
		return true
	default:
		return false
	}
}

func tempName(base string) string {
	return fmt.Sprintf(".%s.%d.%d.%x.tmp", base, os.Getpid(), time.Now().UnixNano(), rand.Uint64())
}
