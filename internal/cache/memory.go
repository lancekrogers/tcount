package cache

import (
	"context"
	"fmt"
	"sync"
)

// MemoryStore is a deterministic, persistence-independent Store for unit and
// integration development. It clones snapshots at both boundaries so callers
// cannot mutate a stored generation through a returned map.
type MemoryStore struct {
	mu        sync.RWMutex
	snapshots map[string]Snapshot
}

var _ Store = (*MemoryStore)(nil)

// NewMemoryStore creates an empty in-memory cache store.
func NewMemoryStore() *MemoryStore {
	return &MemoryStore{snapshots: make(map[string]Snapshot)}
}

// Load returns a defensive copy of the latest complete snapshot.
func (store *MemoryStore) Load(ctx context.Context, root string) (*Snapshot, error) {
	if err := contextAndRoot(ctx, root); err != nil {
		return nil, err
	}

	store.mu.RLock()
	snapshot, ok := store.snapshots[root]
	store.mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("%w: %s", ErrSnapshotNotFound, root)
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	clone := cloneSnapshot(snapshot)
	return &clone, nil
}

// Commit merges a complete or partial update set against the expected
// generation and publishes one new complete in-memory snapshot.
func (store *MemoryStore) Commit(ctx context.Context, root string, baseGeneration uint64, updates UpdateSet) error {
	if err := contextAndRoot(ctx, root); err != nil {
		return err
	}

	store.mu.Lock()
	defer store.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}

	base, ok := store.snapshots[root]
	if !ok {
		base = Snapshot{SchemaVersion: CurrentSchemaVersion, Root: root, Entries: make(map[string]FileResult)}
	}
	if base.Generation != baseGeneration {
		return fmt.Errorf("%w: root %q expected %d, current %d", ErrGenerationConflict, root, baseGeneration, base.Generation)
	}
	merged, err := MergeEntries(base, updates)
	if err != nil {
		return fmt.Errorf("%w: %v", ErrInvalidSnapshot, err)
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	store.snapshots[root] = cloneSnapshot(merged)
	return nil
}

// Status reports whether a complete snapshot is present. Missing roots are
// represented by Present=false rather than an error so status is idempotent.
func (store *MemoryStore) Status(ctx context.Context, root string) (Status, error) {
	if err := contextAndRoot(ctx, root); err != nil {
		return Status{}, err
	}
	store.mu.RLock()
	snapshot, ok := store.snapshots[root]
	store.mu.RUnlock()
	if !ok {
		return Status{Root: root}, nil
	}
	return Status{Root: root, Present: true, SchemaVersion: snapshot.SchemaVersion, Generation: snapshot.Generation, Entries: len(snapshot.Entries)}, nil
}

// Clear removes a root snapshot. Clearing an absent root is intentionally
// idempotent for management commands.
func (store *MemoryStore) Clear(ctx context.Context, root string) error {
	if err := contextAndRoot(ctx, root); err != nil {
		return err
	}
	store.mu.Lock()
	defer store.mu.Unlock()
	if err := ctx.Err(); err != nil {
		return err
	}
	delete(store.snapshots, root)
	return nil
}

func contextAndRoot(ctx context.Context, root string) error {
	if ctx == nil {
		return ErrInvalidRoot
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	return validateCanonicalRoot(root)
}
