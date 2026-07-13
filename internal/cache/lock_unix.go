//go:build darwin || dragonfly || freebsd || linux || netbsd || openbsd || solaris

package cache

import (
	"context"
	"errors"
	"fmt"
	"os"
	"time"

	"golang.org/x/sys/unix"
)

const defaultLockPollInterval = 10 * time.Millisecond

type fileLock struct {
	file *os.File
}

// acquireWriterLock uses an advisory kernel lock so separate tcount
// processes serialize the reload/merge/publish section.
func acquireWriterLock(ctx context.Context, path string) (*fileLock, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening cache writer lock: %w", err)
	}
	lock := &fileLock{file: file}
	for {
		if err := ctx.Err(); err != nil {
			_ = lock.Close()
			return nil, err
		}
		err := unix.Flock(int(file.Fd()), unix.LOCK_EX|unix.LOCK_NB)
		if err == nil {
			return lock, nil
		}
		if errors.Is(err, unix.EINTR) {
			continue
		}
		if !errors.Is(err, unix.EWOULDBLOCK) && !errors.Is(err, unix.EAGAIN) {
			_ = lock.Close()
			return nil, fmt.Errorf("acquiring cache writer lock: %w", err)
		}
		timer := time.NewTimer(defaultLockPollInterval)
		select {
		case <-ctx.Done():
			if !timer.Stop() {
				<-timer.C
			}
			_ = lock.Close()
			return nil, ctx.Err()
		case <-timer.C:
		}
	}
}

func (lock *fileLock) Close() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	unlockErr := unix.Flock(int(lock.file.Fd()), unix.LOCK_UN)
	closeErr := lock.file.Close()
	lock.file = nil
	if unlockErr != nil {
		return fmt.Errorf("releasing cache writer lock: %w", unlockErr)
	}
	if closeErr != nil {
		return fmt.Errorf("closing cache writer lock: %w", closeErr)
	}
	return nil
}
