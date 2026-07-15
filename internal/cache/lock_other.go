//go:build !darwin && !dragonfly && !freebsd && !linux && !netbsd && !openbsd && !solaris

package cache

import "context"

type fileLock struct{}

func acquireWriterLock(ctx context.Context, path string) (*fileLock, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return nil, ErrLockUnsupported
}

func (lock *fileLock) Close() error { return nil }
