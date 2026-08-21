//go:build container

package cache

import (
	"context"
	"sync"
)

var manifestTestHook struct {
	sync.RWMutex
	fn func(context.Context, string, string) error
}

// SetManifestTestHook installs a container-only synchronization hook at the
// point where a complete temporary manifest is closed and ready for rename.
// It exists only to make kill-point tests deterministic.
func SetManifestTestHook(fn func(context.Context, string, string) error) {
	manifestTestHook.Lock()
	manifestTestHook.fn = fn
	manifestTestHook.Unlock()
}

func runManifestTestHook(ctx context.Context, path, temporaryPath string) error {
	manifestTestHook.RLock()
	fn := manifestTestHook.fn
	manifestTestHook.RUnlock()
	if fn == nil {
		return nil
	}
	return fn(ctx, path, temporaryPath)
}

var directorySyncTestHook struct {
	sync.RWMutex
	fn func(string) error
}

// SetDirectorySyncTestHook observes or injects parent-directory fsync
// failures after the atomic rename. The real directory sync still runs when
// the hook returns nil.
func SetDirectorySyncTestHook(fn func(string) error) {
	directorySyncTestHook.Lock()
	directorySyncTestHook.fn = fn
	directorySyncTestHook.Unlock()
}

func runDirectorySyncTestHook(directory string) error {
	directorySyncTestHook.RLock()
	fn := directorySyncTestHook.fn
	directorySyncTestHook.RUnlock()
	if fn == nil {
		return nil
	}
	return fn(directory)
}
