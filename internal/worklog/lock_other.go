//go:build !darwin && !linux

package worklog

import "sync"

var fallbackLocks sync.Map

func acquireProcessLock(path string) (func(), error) {
	value, _ := fallbackLocks.LoadOrStore(path, &sync.Mutex{})
	lock := value.(*sync.Mutex)
	lock.Lock()
	return lock.Unlock, nil
}
