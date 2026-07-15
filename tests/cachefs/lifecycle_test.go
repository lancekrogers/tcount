//go:build container

package cachefs

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/lancekrogers/tcount/internal/cache"
)

func TestFileStoreQuarantinesCorruptManifestInContainer(t *testing.T) {
	_, root, store, location := newLifecycleFixture(t)
	if err := store.Commit(context.Background(), root, 0, cache.UpdateSet{"stable.txt": atomicTestEntry(1)}); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(location.ManifestPath, []byte("truncated manifest"), 0o600); err != nil {
		t.Fatal(err)
	}
	_, err := store.Load(context.Background(), root)
	if !errors.Is(err, cache.ErrCacheCorrupt) || cache.CacheFailureOf(err) != cache.FailureCorrupt {
		t.Fatalf("corrupt load error = %v, category %q", err, cache.CacheFailureOf(err))
	}
	assertQuarantineExists(t, location.Directory, "corrupt")
	if _, err := os.Stat(location.ManifestPath); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("manifest after quarantine error = %v, want absent", err)
	}
	_, err = store.Load(context.Background(), root)
	if !errors.Is(err, cache.ErrSnapshotNotFound) || cache.CacheFailureOf(err) != cache.FailureAbsent {
		t.Fatalf("second load error = %v, category %q", err, cache.CacheFailureOf(err))
	}
	if err := store.Commit(context.Background(), root, 0, cache.UpdateSet{"rebuilt.txt": atomicTestEntry(2)}); err != nil {
		t.Fatalf("cold rebuild commit error = %v", err)
	}
}

func TestFileStoreColdRebuildsIncompatibleManifestInContainer(t *testing.T) {
	_, root, store, location := newLifecycleFixture(t)
	manifest := cache.Manifest{SchemaVersion: cache.CurrentSchemaVersion, Root: location.Root, Entries: map[string]cache.FileEntry{}}
	data, err := cache.EncodeManifest(manifest)
	if err != nil {
		t.Fatal(err)
	}
	binary.LittleEndian.PutUint32(data[4:8], cache.CurrentSchemaVersion+1)
	if err := location.Ensure(context.Background()); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(location.ManifestPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := store.Commit(context.Background(), root, 99, cache.UpdateSet{"rebuilt.txt": atomicTestEntry(2)}); err != nil {
		t.Fatalf("cold rebuild commit error = %v", err)
	}
	snapshot, err := store.Load(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Generation != 1 || len(snapshot.Entries) != 1 {
		t.Fatalf("rebuilt snapshot = generation %d, entries %d", snapshot.Generation, len(snapshot.Entries))
	}
	assertQuarantineExists(t, location.Directory, "incompatible")
}

func TestFileStoreStatusClearAndClearAllInContainer(t *testing.T) {
	base := t.TempDir()
	cacheBase := filepath.Join(base, "cache")
	rootOne := filepath.Join(base, "one")
	rootTwo := filepath.Join(base, "two")
	if err := os.MkdirAll(rootOne, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(rootTwo, 0o755); err != nil {
		t.Fatal(err)
	}
	resolver, err := cache.NewLocationResolverAt(cacheBase)
	if err != nil {
		t.Fatal(err)
	}
	store := cache.NewFileStore(resolver)
	if err := store.Commit(context.Background(), rootOne, 0, cache.UpdateSet{"one.txt": atomicTestEntry(1)}); err != nil {
		t.Fatal(err)
	}
	if err := store.Commit(context.Background(), rootTwo, 0, cache.UpdateSet{"two.txt": atomicTestEntry(2)}); err != nil {
		t.Fatal(err)
	}
	status, err := store.Status(context.Background(), rootOne)
	if err != nil {
		t.Fatal(err)
	}
	if !status.Present || status.SchemaVersion != cache.CurrentSchemaVersion || status.Generation != 1 || status.Entries != 1 || status.Bytes <= 0 || status.Age < 0 || status.ModifiedAt.IsZero() {
		t.Fatalf("status = %+v", status)
	}
	if err := store.Clear(context.Background(), rootOne); err != nil {
		t.Fatal(err)
	}
	if err := store.Clear(context.Background(), rootOne); err != nil {
		t.Fatal(err)
	}
	status, err = store.Status(context.Background(), rootOne)
	if err != nil || status.Present {
		t.Fatalf("status after clear = %+v, error %v", status, err)
	}
	if err := store.ClearAll(context.Background()); err != nil {
		t.Fatal(err)
	}
	status, err = store.Status(context.Background(), rootTwo)
	if err != nil || status.Present {
		t.Fatalf("status after clear all = %+v, error %v", status, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := store.Status(ctx, rootTwo); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled status error = %v", err)
	}
	if err := store.ClearAll(ctx); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled clear-all error = %v", err)
	}
}

func TestFileStorePrunesOnlyAfterSuccessfulFullWalkInContainer(t *testing.T) {
	_, root, store, _ := newLifecycleFixture(t)
	if err := store.Commit(context.Background(), root, 0, cache.UpdateSet{
		"live.txt":  atomicTestEntry(1),
		"stale.txt": atomicTestEntry(2),
	}); err != nil {
		t.Fatal(err)
	}
	paths := map[string]struct{}{"live.txt": {}}
	if _, err := store.Prune(context.Background(), root, cache.PruneOptions{ObservedPaths: paths}); !errors.Is(err, cache.ErrPruneNotApproved) {
		t.Fatalf("unapproved prune error = %v", err)
	}
	snapshot, err := store.Load(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Generation != 1 || len(snapshot.Entries) != 2 {
		t.Fatalf("snapshot after unapproved prune = generation %d, entries %d", snapshot.Generation, len(snapshot.Entries))
	}
	pruned, err := store.Prune(context.Background(), root, cache.PruneOptions{ObservedPaths: paths, FullWalkSucceeded: true})
	if err != nil || pruned != 1 {
		t.Fatalf("approved prune = count %d, error %v", pruned, err)
	}
	snapshot, err = store.Load(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Generation != 2 || len(snapshot.Entries) != 1 {
		t.Fatalf("snapshot after approved prune = generation %d, entries %d", snapshot.Generation, len(snapshot.Entries))
	}
}

func TestFileStoreClassifiesPermissionFailureInContainer(t *testing.T) {
	root := t.TempDir()
	resolver, err := cache.NewLocationResolverAt("/workspace")
	if err != nil {
		t.Fatal(err)
	}
	err = cache.NewFileStore(resolver).Commit(context.Background(), root, 0, cache.UpdateSet{"blocked.txt": atomicTestEntry(1)})
	if cache.CacheFailureOf(err) != cache.FailurePermission || !errors.Is(err, cache.ErrCachePermission) {
		t.Fatalf("permission commit error = %v, category %q", err, cache.CacheFailureOf(err))
	}
}

func newLifecycleFixture(t *testing.T) (string, string, *cache.FileStore, cache.CacheLocation) {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, "repository")
	cacheBase := filepath.Join(base, "cache")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	resolver, err := cache.NewLocationResolverAt(cacheBase)
	if err != nil {
		t.Fatal(err)
	}
	location, err := resolver.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	return base, root, cache.NewFileStore(resolver), location
}

func assertQuarantineExists(t *testing.T, directory, category string) {
	t.Helper()
	matches, err := filepath.Glob(filepath.Join(directory, "manifest.quarantine-"+category+"-*"))
	if err != nil {
		t.Fatal(err)
	}
	if len(matches) != 1 {
		t.Fatalf("quarantine files for %s = %v, want one", category, matches)
	}
}
