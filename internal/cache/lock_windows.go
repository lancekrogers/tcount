//go:build windows

package cache

import (
	"errors"

	"golang.org/x/sys/windows"
)

func tryLockWriter(lock *fileLock) error {
	overlapped := new(windows.Overlapped)
	flags := uint32(windows.LOCKFILE_EXCLUSIVE_LOCK | windows.LOCKFILE_FAIL_IMMEDIATELY)
	return windows.LockFileEx(windows.Handle(lock.file.Fd()), flags, 0, 1, 0, overlapped)
}

func unlockWriterLock(lock *fileLock) error {
	overlapped := new(windows.Overlapped)
	return windows.UnlockFileEx(windows.Handle(lock.file.Fd()), 0, 1, 0, overlapped)
}

func writerLockRetry(err error) lockRetry {
	if errors.Is(err, windows.ERROR_LOCK_VIOLATION) {
		return lockRetryWait
	}
	return lockFail
}
