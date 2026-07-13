package cache

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const storeLockName = ".store.lock"

// PruneOptions makes the membership authority explicit. A narrow or failed
// walk must pass FullWalkSucceeded=false and therefore cannot delete entries.
type PruneOptions struct {
	ObservedPaths     map[string]struct{}
	FullWalkSucceeded bool
}

type storeLocks struct {
	global *fileLock
	root   *fileLock
}

func (locks *storeLocks) Close() error {
	if locks == nil {
		return nil
	}
	rootErr := locks.root.Close()
	globalErr := locks.global.Close()
	if rootErr != nil {
		return rootErr
	}
	return globalErr
}

func (store *FileStore) ensureRoots(ctx context.Context) error {
	if ctx == nil {
		return ErrInvalidRoot
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if store == nil || store.resolver.baseDir == "" {
		return ErrCacheUnavailable
	}
	roots := store.resolver.rootsDirectory()
	if err := os.MkdirAll(roots, 0o700); err != nil {
		return classifyCacheFailure("create cache roots", roots, err)
	}
	if err := os.Chmod(roots, 0o700); err != nil {
		return classifyCacheFailure("secure cache roots", roots, err)
	}
	return nil
}

func (store *FileStore) acquireGlobalLock(ctx context.Context) (*fileLock, error) {
	if err := store.ensureRoots(ctx); err != nil {
		return nil, err
	}
	lock, err := acquireWriterLock(ctx, filepath.Join(store.resolver.rootsDirectory(), storeLockName))
	if err != nil {
		return nil, classifyLockFailure("acquire global cache lock", store.resolver.rootsDirectory(), err)
	}
	return lock, nil
}

func (store *FileStore) acquireLocks(ctx context.Context, location CacheLocation) (*storeLocks, error) {
	global, err := store.acquireGlobalLock(ctx)
	if err != nil {
		return nil, err
	}
	if err := location.Ensure(ctx); err != nil {
		_ = global.Close()
		return nil, classifyCacheFailure("create root cache directory", location.Directory, err)
	}
	root, err := acquireWriterLock(ctx, location.ManifestPath+".lock")
	if err != nil {
		_ = global.Close()
		return nil, classifyLockFailure("acquire root cache lock", location.ManifestPath, err)
	}
	return &storeLocks{global: global, root: root}, nil
}

func (store *FileStore) quarantineManifest(ctx context.Context, location CacheLocation, category CacheFailureKind) (err error) {
	info, err := os.Stat(location.ManifestPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return classifyCacheFailure("inspect cache manifest for quarantine", location.ManifestPath, err)
	}
	locks, err := store.acquireLocks(ctx, location)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := locks.Close(); err == nil && closeErr != nil {
			err = classifyLockFailure("release quarantine lock", location.ManifestPath, closeErr)
		}
	}()
	current, err := os.Stat(location.ManifestPath)
	if err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return classifyCacheFailure("inspect cache manifest under lock", location.ManifestPath, err)
	}
	if !os.SameFile(info, current) {
		return nil
	}
	return quarantineManifestLocked(ctx, location, category)
}

func quarantineManifestLocked(ctx context.Context, location CacheLocation, category CacheFailureKind) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if _, err := os.Stat(location.ManifestPath); err != nil {
		if errors.Is(err, fs.ErrNotExist) {
			return nil
		}
		return classifyCacheFailure("inspect cache manifest", location.ManifestPath, err)
	}
	name := fmt.Sprintf("manifest.quarantine-%s-%d", category, time.Now().UnixNano())
	quarantinePath := filepath.Join(location.Directory, name)
	if err := os.Rename(location.ManifestPath, quarantinePath); err != nil {
		return classifyCacheFailure("quarantine cache manifest", location.ManifestPath, err)
	}
	return nil
}

func readManifestStatus(ctx context.Context, location CacheLocation) (Status, error) {
	status := Status{Root: location.Root}
	if ctx == nil {
		return status, ErrInvalidRoot
	}
	if err := ctx.Err(); err != nil {
		return status, err
	}
	for attempt := 0; attempt < 3; attempt++ {
		info, err := os.Stat(location.ManifestPath)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return status, nil
			}
			classified := classifyCacheFailure("stat cache manifest", location.ManifestPath, err)
			status.Failure = CacheFailureOf(classified)
			return status, classified
		}
		manifest, err := LoadManifestForRoot(ctx, location)
		if err != nil {
			classified := classifyCacheFailure("read cache manifest", location.ManifestPath, err)
			status.Failure = CacheFailureOf(classified)
			return status, classified
		}
		current, err := os.Stat(location.ManifestPath)
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return Status{Root: location.Root}, nil
			}
			classified := classifyCacheFailure("restat cache manifest", location.ManifestPath, err)
			status.Failure = CacheFailureOf(classified)
			return status, classified
		}
		if !sameManifestFile(info, current) {
			continue
		}
		return manifestStatus(location.Root, manifest, current), nil
	}
	err := fmt.Errorf("manifest changed during status read")
	classified := classifyCacheFailure("read stable cache manifest status", location.ManifestPath, err)
	status.Failure = CacheFailureOf(classified)
	return status, classified
}

func sameManifestFile(before, after os.FileInfo) bool {
	return os.SameFile(before, after) && before.Size() == after.Size() && before.ModTime().Equal(after.ModTime())
}

func manifestStatus(root string, manifest Manifest, info os.FileInfo) Status {
	age := time.Since(info.ModTime())
	if age < 0 {
		age = 0
	}
	return Status{Root: root, Present: true, SchemaVersion: manifest.SchemaVersion, Generation: manifest.Generation, Entries: len(manifest.Entries), Bytes: info.Size(), ModifiedAt: info.ModTime(), Age: age}
}

func (store *FileStore) loadLatest(ctx context.Context, location CacheLocation) (Manifest, bool, error) {
	manifest, err := LoadManifestForRoot(ctx, location)
	if err == nil {
		return manifest, false, nil
	}
	if errors.Is(err, fs.ErrNotExist) {
		return emptyManifest(location.Root), false, nil
	}
	classified := classifyCacheFailure("read cache manifest", location.ManifestPath, err)
	category := CacheFailureOf(classified)
	if category != FailureCorrupt && category != FailureIncompatible {
		return Manifest{}, false, classified
	}
	if err := quarantineManifestLocked(ctx, location, category); err != nil {
		return Manifest{}, false, errors.Join(classified, err)
	}
	return emptyManifest(location.Root), true, nil
}

func emptyManifest(root string) Manifest {
	return Manifest{SchemaVersion: CurrentSchemaVersion, Root: root, Entries: make(map[string]FileEntry)}
}

func cleanupManifestTemps(ctx context.Context, directory string) error {
	entries, err := os.ReadDir(directory)
	if err != nil {
		return err
	}
	for _, entry := range entries {
		if !strings.HasPrefix(entry.Name(), ".manifest-") {
			continue
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := os.Remove(filepath.Join(directory, entry.Name())); err != nil && !errors.Is(err, fs.ErrNotExist) {
			return err
		}
	}
	return nil
}

// ClearAll removes every root cache while holding the global lifecycle lock.
// Root writers acquire that lock first, so a clear cannot race a commit.
func (store *FileStore) ClearAll(ctx context.Context) (err error) {
	lock, err := store.acquireGlobalLock(ctx)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := lock.Close(); err == nil && closeErr != nil {
			err = classifyLockFailure("release global cache lock", store.resolver.rootsDirectory(), closeErr)
		}
	}()
	entries, err := os.ReadDir(store.resolver.rootsDirectory())
	if err != nil {
		return classifyCacheFailure("list cache roots", store.resolver.rootsDirectory(), err)
	}
	for _, entry := range entries {
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Name() == storeLockName {
			continue
		}
		path := filepath.Join(store.resolver.rootsDirectory(), entry.Name())
		if err := os.RemoveAll(path); err != nil {
			return classifyCacheFailure("clear cache roots", path, err)
		}
	}
	return nil
}

// Prune removes entries absent from an explicitly successful full walk. A
// failed or narrow run returns ErrPruneNotApproved without changing state.
func (store *FileStore) Prune(ctx context.Context, root string, options PruneOptions) (pruned int, err error) {
	location, err := store.resolve(ctx, root)
	if err != nil {
		return 0, err
	}
	if !options.FullWalkSucceeded {
		return 0, ErrPruneNotApproved
	}
	for path := range options.ObservedPaths {
		if err := validatePath(path); err != nil {
			return 0, fmt.Errorf("%w: %v", ErrInvalidSnapshot, err)
		}
	}
	locks, err := store.acquireLocks(ctx, location)
	if err != nil {
		return 0, err
	}
	defer func() {
		if closeErr := locks.Close(); err == nil && closeErr != nil {
			err = classifyLockFailure("release prune lock", location.ManifestPath, closeErr)
		}
	}()
	manifest, _, err := store.loadLatest(ctx, location)
	if err != nil {
		return 0, err
	}
	pruned = 0
	for path := range manifest.Entries {
		if _, live := options.ObservedPaths[path]; live {
			continue
		}
		delete(manifest.Entries, path)
		pruned++
	}
	if pruned == 0 {
		return 0, nil
	}
	manifest.Generation++
	if err := WriteManifestForRoot(ctx, location, manifest); err != nil {
		return 0, classifyCacheFailure("persist pruned cache manifest", location.ManifestPath, err)
	}
	return pruned, nil
}
