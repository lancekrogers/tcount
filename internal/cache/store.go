package cache

import (
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
)

// FileStore persists one complete snapshot per canonical root. A caller may
// compute updates from an optimistic load; Commit then reloads under the
// writer lock and only merges updates whose observed identity is compatible
// with the latest generation.
type FileStore struct {
	resolver LocationResolver
}

var _ Store = (*FileStore)(nil)

// NewFileStore creates a filesystem-backed Store using resolver's injected
// user-cache parent.
func NewFileStore(resolver LocationResolver) *FileStore {
	return &FileStore{resolver: resolver}
}

// NewDefaultFileStore creates a filesystem-backed Store in the platform user
// cache directory.
func NewDefaultFileStore() (*FileStore, error) {
	resolver, err := NewLocationResolver()
	if err != nil {
		return nil, err
	}
	return NewFileStore(resolver), nil
}

// Load returns the latest complete snapshot for root.
func (store *FileStore) Load(ctx context.Context, root string) (*Snapshot, error) {
	location, err := store.resolve(ctx, root)
	if err != nil {
		return nil, err
	}
	snapshot, err := LoadManifestForRoot(ctx, location)
	if err == nil {
		clone := cloneSnapshot(snapshot)
		return &clone, nil
	}
	classified := classifyCacheFailure("load cache manifest", location.ManifestPath, err)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, &CacheError{Category: FailureAbsent, Operation: "load cache manifest", Path: location.ManifestPath, Err: fmt.Errorf("%w: %s", ErrSnapshotNotFound, location.Root)}
	}
	category := CacheFailureOf(classified)
	if category == FailureCorrupt || category == FailureIncompatible {
		if quarantineErr := store.quarantineManifest(ctx, location, category); quarantineErr != nil {
			return nil, errors.Join(classified, quarantineErr)
		}
	}
	return nil, classified
}

// Commit publishes one generation after reloading the latest state under an
// inter-process writer lock. Updates based on an older generation survive
// when they add a new path or preserve the latest path identity; a stale
// same-path identity is rejected instead of overwriting newer data.
func (store *FileStore) Commit(ctx context.Context, root string, baseGeneration uint64, updates UpdateSet) (err error) {
	location, err := store.resolve(ctx, root)
	if err != nil {
		return err
	}
	if err := validateUpdateSet(updates); err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidSnapshot, err)
	}
	locks, err := store.acquireLocks(ctx, location)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := locks.Close(); err == nil && closeErr != nil {
			err = classifyLockFailure("release cache writer lock", location.ManifestPath, closeErr)
		}
	}()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := cleanupManifestTemps(ctx, location.Directory); err != nil {
		return classifyCacheFailure("clean interrupted manifest writes", location.Directory, err)
	}

	latest, recovered, err := store.loadLatest(ctx, location)
	if err != nil {
		return err
	}
	if !recovered && latest.Generation < baseGeneration {
		return generationConflict(location.Root, baseGeneration, latest.Generation)
	}

	compatible := updates
	if !recovered && latest.Generation > baseGeneration {
		compatible = compatibleUpdates(latest, updates)
		if len(compatible) == 0 && len(updates) != 0 {
			return generationConflict(location.Root, baseGeneration, latest.Generation)
		}
	}
	merged, err := MergeEntries(latest, compatible)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidSnapshot, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := WriteManifestForRoot(ctx, location, merged); err != nil {
		return classifyCacheFailure("persist cache manifest", location.ManifestPath, err)
	}
	return nil
}

// Status reports whether a complete snapshot is present without creating a
// cache directory for a root that has never been committed.
func (store *FileStore) Status(ctx context.Context, root string) (Status, error) {
	location, err := store.resolve(ctx, root)
	if err != nil {
		return Status{}, err
	}
	status, err := readManifestStatus(ctx, location)
	if err == nil || status.Failure == FailureAbsent {
		return status, err
	}
	if status.Failure == FailureCorrupt || status.Failure == FailureIncompatible {
		if quarantineErr := store.quarantineManifest(ctx, location, status.Failure); quarantineErr != nil {
			return status, errors.Join(err, quarantineErr)
		}
	}
	return status, err
}

// Clear removes the current manifest under the same writer lock used by
// Commit. The lock file is retained as an inert coordination inode so a
// concurrent process cannot race directory removal with lock acquisition.
func (store *FileStore) Clear(ctx context.Context, root string) (err error) {
	location, err := store.resolve(ctx, root)
	if err != nil {
		return err
	}
	locks, err := store.acquireLocks(ctx, location)
	if err != nil {
		return err
	}
	defer func() {
		if closeErr := locks.Close(); err == nil && closeErr != nil {
			err = classifyLockFailure("release cache writer lock", location.ManifestPath, closeErr)
		}
	}()
	if err := ctx.Err(); err != nil {
		return err
	}
	if err := os.Remove(location.ManifestPath); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return classifyCacheFailure("clear cache manifest", location.ManifestPath, err)
	}
	return nil
}

func (store *FileStore) resolve(ctx context.Context, root string) (CacheLocation, error) {
	if err := contextAndRoot(ctx, root); err != nil {
		return CacheLocation{}, err
	}
	if store == nil || store.resolver.baseDir == "" {
		return CacheLocation{}, ErrCacheUnavailable
	}
	return store.resolver.Resolve(root)
}

func compatibleUpdates(latest Manifest, updates UpdateSet) UpdateSet {
	compatible := make(UpdateSet, len(updates))
	for path, update := range updates {
		current, exists := latest.Entries[path]
		if !exists || sameFileIdentity(current, update) {
			compatible[path] = cloneFileResult(update)
		}
	}
	return compatible
}

func validateUpdateSet(updates UpdateSet) error {
	if uint64(len(updates)) > uint64(MaxManifestEntries) {
		return errors.New("update set exceeds entry limit")
	}
	for path, entry := range updates {
		if err := validatePath(path); err != nil {
			return fmt.Errorf("update path %q: %w", path, err)
		}
		if err := validateFileEntry(entry); err != nil {
			return fmt.Errorf("update %q: %w", path, err)
		}
	}
	return nil
}

func generationConflict(root string, expected, current uint64) error {
	return fmt.Errorf("%w: root %q expected at least %d, current %d", ErrGenerationConflict, root, expected, current)
}
