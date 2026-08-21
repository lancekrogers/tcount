//go:build unix || windows

package cache

import (
	"context"
	"fmt"
	"os"
)

type fileLock struct {
	file *os.File
}

func openLockFile(path string) (*fileLock, error) {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_RDWR, 0o600)
	if err != nil {
		return nil, fmt.Errorf("opening cache writer lock: %w", err)
	}
	return &fileLock{file: file}, nil
}

// acquireWriterLock uses an advisory kernel lock so separate tcount
// processes serialize the reload/merge/publish section.
func acquireWriterLock(ctx context.Context, path string) (*fileLock, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	lock, err := openLockFile(path)
	if err != nil {
		return nil, err
	}
	if err := waitForLock(ctx, func() error { return tryLockWriter(lock) }, writerLockRetry); err != nil {
		_ = lock.file.Close()
		lock.file = nil
		return nil, wrapLockAcquireError(err)
	}
	return lock, nil
}

func (lock *fileLock) Close() error {
	if lock == nil || lock.file == nil {
		return nil
	}
	unlockErr := unlockWriterLock(lock)
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
