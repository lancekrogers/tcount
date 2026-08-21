//go:build unix

package cache

import (
	"errors"

	"golang.org/x/sys/unix"
)

func tryLockWriter(lock *fileLock) error {
	return unix.Flock(int(lock.file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
}

func unlockWriterLock(lock *fileLock) error {
	return unix.Flock(int(lock.file.Fd()), unix.LOCK_UN)
}

func writerLockRetry(err error) lockRetry {
	if errors.Is(err, unix.EINTR) {
		return lockRetryImmediate
	}
	if errors.Is(err, unix.EWOULDBLOCK) || errors.Is(err, unix.EAGAIN) {
		return lockRetryWait
	}
	return lockFail
}
