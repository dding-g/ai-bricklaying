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

type WriteOptions struct {
	FileMode fs.FileMode
	DirMode  fs.FileMode
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
	if err := EnsureDir(dir, dirMode); err != nil {
		return err
	}
	if err := rejectSymlink(path); err != nil {
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
	if err := file.Close(); err != nil {
		closed = true
		return err
	}
	closed = true

	if err := rejectSymlink(path); err != nil {
		return err
	}
	if err := os.Rename(tempPath, path); err != nil {
		return err
	}
	cleanup = false
	if err := os.Chmod(path, fileMode); err != nil && runtime.GOOS != "windows" {
		return err
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
	if err := os.MkdirAll(path, mode); err != nil {
		return err
	}
	if err := os.Chmod(path, mode); err != nil && runtime.GOOS != "windows" {
		return err
	}
	return nil
}

func rejectSymlink(path string) error {
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

func tempName(base string) string {
	return fmt.Sprintf(".%s.%d.%d.%x.tmp", base, os.Getpid(), time.Now().UnixNano(), rand.Uint64())
}
