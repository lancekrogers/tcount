//go:build container

package cachefs

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"testing"

	"github.com/lancekrogers/tcount/internal/cache"
)

const atomicWorkerEnvironment = "TCOUNT_ATOMIC_WORKER"

// TestAtomicStoreWorker is launched as a distinct OS process by the
// concurrency tests below. It intentionally accepts a generation conflict for
// same-path races: the caller's count result is independent of cache storage,
// and the stale update must not replace the winner on disk.
func TestAtomicStoreWorker(t *testing.T) {
	if os.Getenv(atomicWorkerEnvironment) != "1" {
		return
	}
	base := os.Getenv("TCOUNT_ATOMIC_CACHE_BASE")
	root := os.Getenv("TCOUNT_ATOMIC_ROOT")
	path := os.Getenv("TCOUNT_ATOMIC_PATH")
	identity, err := strconv.Atoi(os.Getenv("TCOUNT_ATOMIC_IDENTITY"))
	if err != nil {
		t.Fatal(err)
	}
	resolver, err := cache.NewLocationResolverAt(base)
	if err != nil {
		t.Fatal(err)
	}
	store := cache.NewFileStore(resolver)
	entry := atomicTestEntry(identity)
	err = store.Commit(context.Background(), root, 0, cache.UpdateSet{path: entry})
	if err != nil && !errors.Is(err, cache.ErrGenerationConflict) {
		t.Fatal(err)
	}
}

func TestFileStoreMergesDistinctProcessUpdatesInContainer(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "repository")
	cacheBase := filepath.Join(base, "cache")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	workers := []*atomicWorker{
		startAtomicWorker(t, cacheBase, root, "one.txt", 1),
		startAtomicWorker(t, cacheBase, root, "two.txt", 2),
	}
	waitAtomicWorkers(t, workers)

	resolver, err := cache.NewLocationResolverAt(cacheBase)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := cache.NewFileStore(resolver).Load(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Generation != 2 {
		t.Fatalf("generation = %d, want 2", snapshot.Generation)
	}
	if len(snapshot.Entries) != 2 {
		t.Fatalf("entries = %d, want 2", len(snapshot.Entries))
	}
}

func TestFileStoreRejectsStaleSamePathProcessUpdateInContainer(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "repository")
	cacheBase := filepath.Join(base, "cache")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	workers := []*atomicWorker{
		startAtomicWorker(t, cacheBase, root, "shared.txt", 1),
		startAtomicWorker(t, cacheBase, root, "shared.txt", 2),
	}
	waitAtomicWorkers(t, workers)

	resolver, err := cache.NewLocationResolverAt(cacheBase)
	if err != nil {
		t.Fatal(err)
	}
	snapshot, err := cache.NewFileStore(resolver).Load(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Generation != 1 {
		t.Fatalf("generation = %d, want 1 after same-path race", snapshot.Generation)
	}
	entry, ok := snapshot.Entries["shared.txt"]
	if !ok {
		t.Fatal("same-path entry is missing")
	}
	if len(entry.Methods) != 1 {
		t.Fatalf("same-path methods = %d, want one winning update", len(entry.Methods))
	}
}

func TestFileStoreCancellationLeavesPreviousGenerationInContainer(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "repository")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	resolver, err := cache.NewLocationResolverAt(filepath.Join(base, "cache"))
	if err != nil {
		t.Fatal(err)
	}
	store := cache.NewFileStore(resolver)
	if err := store.Commit(context.Background(), root, 0, cache.UpdateSet{"stable.txt": atomicTestEntry(1)}); err != nil {
		t.Fatal(err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if err := store.Commit(ctx, root, 1, cache.UpdateSet{"new.txt": atomicTestEntry(2)}); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled commit error = %v, want context.Canceled", err)
	}
	snapshot, err := store.Load(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Generation != 1 || len(snapshot.Entries) != 1 {
		t.Fatalf("snapshot after canceled commit = generation %d, entries %d", snapshot.Generation, len(snapshot.Entries))
	}
}

type atomicWorker struct {
	cmd    *exec.Cmd
	output bytes.Buffer
}

func startAtomicWorker(t *testing.T, cacheBase, root, path string, identity int) *atomicWorker {
	t.Helper()
	worker := &atomicWorker{}
	worker.cmd = exec.Command(os.Args[0], "-test.run=^TestAtomicStoreWorker$", "-test.v")
	worker.cmd.Env = append(os.Environ(),
		atomicWorkerEnvironment+"=1",
		"TCOUNT_ATOMIC_CACHE_BASE="+cacheBase,
		"TCOUNT_ATOMIC_ROOT="+root,
		"TCOUNT_ATOMIC_PATH="+path,
		"TCOUNT_ATOMIC_IDENTITY="+strconv.Itoa(identity),
	)
	worker.cmd.Stdout = &worker.output
	worker.cmd.Stderr = &worker.output
	if err := worker.cmd.Start(); err != nil {
		t.Fatal(err)
	}
	return worker
}

func waitAtomicWorkers(t *testing.T, workers []*atomicWorker) {
	t.Helper()
	for _, worker := range workers {
		if err := worker.cmd.Wait(); err != nil {
			t.Fatalf("worker failed: %v\n%s", err, worker.output.String())
		}
	}
}

func atomicTestEntry(identity int) cache.FileResult {
	digest := [32]byte{byte(identity)}
	return cache.FileResult{
		Size:           int64(identity),
		ModTimeNS:      int64(identity),
		ContentDigest:  digest,
		Classification: cache.ClassificationText,
		Methods: map[cache.ContractKey]int{
			{Method: "worker", Encoding: fmt.Sprintf("identity-%d", identity)}: identity,
		},
	}
}
