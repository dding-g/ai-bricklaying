//go:build darwin || linux

package worklog

import (
	"errors"
	"os"
	"syscall"
	"time"

	"ai-bricklaying/internal/safeio"
)

func acquireProcessLock(path string) (func(), error) {
	file, err := os.OpenFile(path, os.O_RDWR|os.O_CREATE, safeio.PrivateFileMode)
	if err != nil {
		return nil, err
	}
	if err := file.Chmod(safeio.PrivateFileMode); err != nil {
		_ = file.Close()
		return nil, err
	}
	for attempt := 0; attempt < 100; attempt++ {
		err = syscall.Flock(int(file.Fd()), syscall.LOCK_EX|syscall.LOCK_NB)
		if err == nil {
			return func() {
				_ = syscall.Flock(int(file.Fd()), syscall.LOCK_UN)
				_ = file.Close()
			}, nil
		}
		if errors.Is(err, syscall.EINTR) {
			continue
		}
		if !errors.Is(err, syscall.EWOULDBLOCK) && !errors.Is(err, syscall.EAGAIN) {
			_ = file.Close()
			return nil, err
		}
		time.Sleep(20 * time.Millisecond)
	}
	_ = file.Close()
	return nil, ErrFlowBusy
}
