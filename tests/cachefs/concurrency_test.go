//go:build container

package cachefs

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"
	"time"

	"github.com/lancekrogers/tcount/internal/cache"
)

const (
	barrierWriterEnvironment = "TCOUNT_BARRIER_WRITER"
	readerEnvironment        = "TCOUNT_CACHE_READER"
)

// TestBarrierWriterProcess pauses after its complete temporary manifest has
// been flushed but before rename. Parent tests use the barrier to launch
// readers and a cancellation attempt against a known lock holder.
func TestBarrierWriterProcess(t *testing.T) {
	if os.Getenv(barrierWriterEnvironment) != "1" {
		return
	}
	base := os.Getenv("TCOUNT_ATOMIC_CACHE_BASE")
	root := os.Getenv("TCOUNT_ATOMIC_ROOT")
	path := os.Getenv("TCOUNT_ATOMIC_PATH")
	identity, err := strconv.Atoi(os.Getenv("TCOUNT_ATOMIC_IDENTITY"))
	if err != nil {
		t.Fatal(err)
	}
	ready := os.Getenv("TCOUNT_BARRIER_READY")
	release := os.Getenv("TCOUNT_BARRIER_RELEASE")
	cache.SetManifestTestHook(func(ctx context.Context, _, temporaryPath string) error {
		if err := os.WriteFile(ready, []byte(temporaryPath), 0o600); err != nil {
			return err
		}
		for {
			if _, err := os.Stat(release); err == nil {
				return nil
			} else if !errors.Is(err, os.ErrNotExist) {
				return err
			}
			select {
			case <-ctx.Done():
				return ctx.Err()
			case <-time.After(5 * time.Millisecond):
			}
		}
	})
	defer cache.SetManifestTestHook(nil)
	resolver, err := cache.NewLocationResolverAt(base)
	if err != nil {
		t.Fatal(err)
	}
	store := cache.NewFileStore(resolver)
	if err := store.Commit(context.Background(), root, 1, cache.UpdateSet{path: atomicTestEntry(identity)}); err != nil {
		t.Fatal(err)
	}
}

func TestCacheReaderProcess(t *testing.T) {
	if os.Getenv(readerEnvironment) != "1" {
		return
	}
	base := os.Getenv("TCOUNT_ATOMIC_CACHE_BASE")
	root := os.Getenv("TCOUNT_ATOMIC_ROOT")
	minimumGeneration, err := strconv.ParseUint(os.Getenv("TCOUNT_READER_MIN_GENERATION"), 10, 64)
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := cache.NewLocationResolverAt(base)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := cache.NewFileStore(resolver).Load(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Generation < minimumGeneration || len(snapshot.Entries) == 0 {
		t.Fatalf("reader observed generation %d with %d entries", snapshot.Generation, len(snapshot.Entries))
	}
}

func TestConcurrentReadersObserveCompleteGenerationsInContainer(t *testing.T) {
	base, root, store, _ := newLifecycleFixture(t)
	cacheBase := filepath.Join(base, "cache")
	if err := store.Commit(context.Background(), root, 0, cache.UpdateSet{"stable.txt": atomicTestEntry(1)}); err != nil {
		t.Fatal(err)
	}
	ready := filepath.Join(base, "writer.ready")
	release := filepath.Join(base, "writer.release")
	writer := startBarrierWriter(t, cacheBase, root, "new.txt", 2, ready, release)
	waitForFile(t, ready)
	readers := make([]*atomicWorker, 0, 4)
	for range 4 {
		readers = append(readers, startReader(t, cacheBase, root, 1))
	}
	waitAtomicWorkers(t, readers)
	if err := os.WriteFile(release, []byte("release"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitAtomicWorkers(t, []*atomicWorker{writer})
	snapshot, err := store.Load(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Generation != 2 || len(snapshot.Entries) != 2 {
		t.Fatalf("final snapshot = generation %d, entries %d", snapshot.Generation, len(snapshot.Entries))
	}
}

func TestLockCancellationReturnsWhileWriterIsBlockedInContainer(t *testing.T) {
	base, root, store, _ := newLifecycleFixture(t)
	cacheBase := filepath.Join(base, "cache")
	if err := store.Commit(context.Background(), root, 0, cache.UpdateSet{"stable.txt": atomicTestEntry(1)}); err != nil {
		t.Fatal(err)
	}
	ready := filepath.Join(base, "writer.ready")
	release := filepath.Join(base, "writer.release")
	writer := startBarrierWriter(t, cacheBase, root, "held.txt", 2, ready, release)
	waitForFile(t, ready)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	started := time.Now()
	err := store.Commit(ctx, root, 1, cache.UpdateSet{"cancelled.txt": atomicTestEntry(3)})
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("blocked commit error = %v, want deadline exceeded", err)
	}
	if elapsed := time.Since(started); elapsed > time.Second {
		t.Fatalf("lock cancellation took %s", elapsed)
	}
	if err := os.WriteFile(release, []byte("release"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitAtomicWorkers(t, []*atomicWorker{writer})
	assertValidSnapshot(t, store, root, 2, 2)
}

func TestKilledTemporaryWriterLeavesOldOrNewGenerationInContainer(t *testing.T) {
	base, root, store, location := newLifecycleFixture(t)
	if err := store.Commit(context.Background(), root, 0, cache.UpdateSet{"stable.txt": atomicTestEntry(1)}); err != nil {
		t.Fatal(err)
	}
	ready := filepath.Join(base, "writer.ready")
	writer := startKilledWriter(t, filepath.Join(base, "cache"), root, ready)
	waitForFile(t, ready)
	if err := writer.cmd.Process.Kill(); err != nil {
		t.Fatal(err)
	}
	if err := writer.cmd.Wait(); err == nil {
		t.Fatal("killed writer exited successfully")
	}
	assertValidSnapshot(t, store, root, 1, 1)
	if matches, err := filepath.Glob(filepath.Join(location.Directory, ".manifest-*")); err != nil || len(matches) == 0 {
		t.Fatalf("killed writer temporary files = %v, error %v; expected cleanup on next commit", matches, err)
	}
	if err := store.Commit(context.Background(), root, 1, cache.UpdateSet{"after-kill.txt": atomicTestEntry(2)}); err != nil {
		t.Fatal(err)
	}
	if matches, err := filepath.Glob(filepath.Join(location.Directory, ".manifest-*")); err != nil || len(matches) != 0 {
		t.Fatalf("temporary files after cleanup = %v, error %v", matches, err)
	}
	assertValidSnapshot(t, store, root, 2, 2)
}

func TestClearAndWriteRaceLeavesValidStateInContainer(t *testing.T) {
	base, root, store, _ := newLifecycleFixture(t)
	cacheBase := filepath.Join(base, "cache")
	if err := store.Commit(context.Background(), root, 0, cache.UpdateSet{"stable.txt": atomicTestEntry(1)}); err != nil {
		t.Fatal(err)
	}
	ready := filepath.Join(base, "writer.ready")
	release := filepath.Join(base, "writer.release")
	writer := startBarrierWriter(t, cacheBase, root, "raced.txt", 2, ready, release)
	waitForFile(t, ready)
	clearResult := make(chan error, 1)
	go func() { clearResult <- store.Clear(context.Background(), root) }()
	if err := os.WriteFile(release, []byte("release"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitAtomicWorkers(t, []*atomicWorker{writer})
	if err := <-clearResult; err != nil {
		t.Fatal(err)
	}
	status, err := store.Status(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if status.Present {
		assertValidSnapshot(t, store, root, status.Generation, status.Entries)
	}
}

func TestRepeatedDistinctProcessStressPreservesCompleteGenerationsInContainer(t *testing.T) {
	for iteration := 0; iteration < 4; iteration++ {
		base := t.TempDir()
		root := filepath.Join(base, "repository")
		cacheBase := filepath.Join(base, "cache")
		if err := os.Mkdir(root, 0o755); err != nil {
			t.Fatal(err)
		}
		workers := make([]*atomicWorker, 0, 6)
		for worker := 0; worker < 6; worker++ {
			workers = append(workers, startAtomicWorker(t, cacheBase, root, fmt.Sprintf("file-%d.txt", worker), worker+1))
		}
		waitAtomicWorkers(t, workers)
		resolver, err := cache.NewLocationResolverAt(cacheBase)
		if err != nil {
			t.Fatal(err)
		}
		assertValidSnapshot(t, cache.NewFileStore(resolver), root, 6, 6)
	}
}

func startBarrierWriter(t *testing.T, cacheBase, root, path string, identity int, ready, release string) *atomicWorker {
	t.Helper()
	return startProcess(t, "^TestBarrierWriterProcess$", map[string]string{
		barrierWriterEnvironment:   "1",
		"TCOUNT_ATOMIC_CACHE_BASE": cacheBase,
		"TCOUNT_ATOMIC_ROOT":       root,
		"TCOUNT_ATOMIC_PATH":       path,
		"TCOUNT_ATOMIC_IDENTITY":   strconv.Itoa(identity),
		"TCOUNT_BARRIER_READY":     ready,
		"TCOUNT_BARRIER_RELEASE":   release,
	})
}

func startReader(t *testing.T, cacheBase, root string, generation uint64) *atomicWorker {
	t.Helper()
	return startProcess(t, "^TestCacheReaderProcess$", map[string]string{
		readerEnvironment:              "1",
		"TCOUNT_ATOMIC_CACHE_BASE":     cacheBase,
		"TCOUNT_ATOMIC_ROOT":           root,
		"TCOUNT_READER_MIN_GENERATION": strconv.FormatUint(generation, 10),
	})
}

func startKilledWriter(t *testing.T, cacheBase, root, ready string) *atomicWorker {
	t.Helper()
	return startProcess(t, "^TestKilledWriterProcess$", map[string]string{
		"TCOUNT_KILLED_WRITER":     "1",
		"TCOUNT_ATOMIC_CACHE_BASE": cacheBase,
		"TCOUNT_ATOMIC_ROOT":       root,
		"TCOUNT_ATOMIC_PATH":       "killed.txt",
		"TCOUNT_ATOMIC_IDENTITY":   "2",
		"TCOUNT_BARRIER_READY":     ready,
	})
}

func TestKilledWriterProcess(t *testing.T) {
	if os.Getenv("TCOUNT_KILLED_WRITER") != "1" {
		return
	}
	resolver, err := cache.NewLocationResolverAt(os.Getenv("TCOUNT_ATOMIC_CACHE_BASE"))
	if err != nil {
		t.Fatal(err)
	}
	cache.SetManifestTestHook(func(_ context.Context, _, temporaryPath string) error {
		if err := os.WriteFile(os.Getenv("TCOUNT_BARRIER_READY"), []byte(temporaryPath), 0o600); err != nil {
			return err
		}
		select {}
	})
	defer cache.SetManifestTestHook(nil)
	store := cache.NewFileStore(resolver)
	if err := store.Commit(context.Background(), os.Getenv("TCOUNT_ATOMIC_ROOT"), 1, cache.UpdateSet{
		os.Getenv("TCOUNT_ATOMIC_PATH"): atomicTestEntry(2),
	}); err != nil {
		t.Fatal(err)
	}
}

func startProcess(t *testing.T, pattern string, values map[string]string) *atomicWorker {
	t.Helper()
	worker := &atomicWorker{}
	worker.cmd = exec.Command(os.Args[0], "-test.run="+pattern, "-test.v")
	worker.cmd.Env = append(os.Environ(), "TCOUNT_PROCESS_WORKER=1")
	for key, value := range values {
		worker.cmd.Env = append(worker.cmd.Env, key+"="+value)
	}
	worker.cmd.Stdout = &worker.output
	worker.cmd.Stderr = &worker.output
	if err := worker.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	return worker
}

func waitForFile(t *testing.T, path string) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for time.Now().Before(deadline) {
		if _, err := os.Stat(path); err == nil {
			return
		} else if !errors.Is(err, os.ErrNotExist) {
			t.Fatal(err)
		}
		time.Sleep(5 * time.Millisecond)
	}
	t.Fatalf("timed out waiting for %s", path)
}

func assertValidSnapshot(t *testing.T, store *cache.FileStore, root string, generation uint64, entries int) {
	t.Helper()
	snapshot, err := store.Load(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Generation != generation || len(snapshot.Entries) != entries {
		t.Fatalf("snapshot = generation %d, entries %d; want generation %d, entries %d", snapshot.Generation, len(snapshot.Entries), generation, entries)
	}
}
