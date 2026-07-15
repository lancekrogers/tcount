//go:build container

package cachefs

import (
	"context"
	"encoding/binary"
	"errors"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"github.com/lancekrogers/tcount/internal/cache"
	"github.com/lancekrogers/tcount/tokenizer"
	"github.com/lancekrogers/tcount/tokenizer/fileops"
)

func TestCountCacheCancellationDuringValidationLeavesGenerationUnchangedInContainer(t *testing.T) {
	root, store := newCountFixture(t)
	for index := 0; index < 8; index++ {
		name := "file-" + string(rune('a'+index)) + ".txt"
		writeTextFile(t, root, name, "validation fixture\n")
	}
	stats := tokenizer.NewStats()
	counter, err := tokenizer.NewCounter(tokenizer.CounterOptions{Stats: stats})
	if err != nil {
		t.Fatal(err)
	}
	walk, err := fileops.WalkDirectory(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := counter.CountFilesWithCache(context.Background(), root, walk.Files, "gpt-5", false, store, cache.Verified); err != nil {
		t.Fatal(err)
	}
	statusBefore, err := store.Status(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}

	ctx := newCancelAfterErrContext(3)
	if _, err := counter.CountFilesWithCache(ctx, root, walk.Files, "gpt-5", false, store, cache.Verified); !errors.Is(err, context.Canceled) {
		t.Fatalf("validation cancellation error = %v, want context.Canceled", err)
	}
	statusAfter, err := store.Status(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if statusAfter.Generation != statusBefore.Generation || statusAfter.Entries != statusBefore.Entries {
		t.Fatalf("validation cancellation published generation: before=%+v after=%+v", statusBefore, statusAfter)
	}
}

func TestCountCacheCancellationDuringWorkerSchedulingLeavesNoGenerationInContainer(t *testing.T) {
	root, store := newCountFixture(t)
	for index := 0; index < 48; index++ {
		writeTextFile(t, root, "worker-"+string(rune('a'+index%26))+"-"+string(rune('a'+index/26))+".txt", "worker scheduling fixture\n")
	}
	walk, err := fileops.WalkDirectory(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	counter, err := tokenizer.NewCounter(tokenizer.CounterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := newCancelAfterErrContext(int64(len(walk.Files) + 9))
	if _, err := counter.CountFilesWithCache(ctx, root, walk.Files, "gpt-5", false, store, cache.Metadata); !errors.Is(err, context.Canceled) {
		t.Fatalf("worker scheduling cancellation error = %v, want context.Canceled", err)
	}
	status, err := store.Status(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if status.Present {
		t.Fatalf("worker scheduling cancellation published cache: %+v", status)
	}
}

func TestCountCacheCancellationDuringTokenizerWorkLeavesNoGenerationInContainer(t *testing.T) {
	root, store := newCountFixture(t)
	for index := 0; index < 32; index++ {
		writeTextFile(t, root, "tokenizer-"+string(rune('a'+index%26))+"-"+string(rune('a'+index/26))+".txt", "tokenizer cancellation fixture\n")
	}
	walk, err := fileops.WalkDirectory(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	counter, err := tokenizer.NewCounter(tokenizer.CounterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	ctx := newCancelAfterErrContext(int64(len(walk.Files) + 15))
	if _, err := counter.CountFilesWithCache(ctx, root, walk.Files, "", true, store, cache.Metadata); !errors.Is(err, context.Canceled) {
		t.Fatalf("tokenizer cancellation error = %v, want context.Canceled", err)
	}
	status, err := store.Status(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if status.Present {
		t.Fatalf("tokenizer cancellation published cache: %+v", status)
	}
}

func TestCountCacheCancellationWhileWaitingForWriterLeavesCountUpdateUnpublishedInContainer(t *testing.T) {
	base := t.TempDir()
	root := filepath.Join(base, "repository")
	cacheBase := filepath.Join(base, "cache")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	writeTextFile(t, root, "stable.txt", "stable\n")
	resolver, err := cache.NewLocationResolverAt(cacheBase)
	if err != nil {
		t.Fatal(err)
	}
	store := cache.NewFileStore(resolver)
	counter, err := tokenizer.NewCounter(tokenizer.CounterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	walk, err := fileops.WalkDirectory(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := counter.CountFilesWithCache(context.Background(), root, walk.Files, "gpt-5", false, store, cache.Metadata); err != nil {
		t.Fatal(err)
	}
	writeTextFile(t, root, "stable.txt", "changed content\n")

	ready := filepath.Join(base, "writer.ready")
	release := filepath.Join(base, "writer.release")
	writer := startBarrierWriter(t, cacheBase, root, "held.txt", 2, ready, release)
	waitForFile(t, ready)
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Millisecond)
	defer cancel()
	changedWalk, err := fileops.WalkDirectory(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := counter.CountFilesWithCache(ctx, root, changedWalk.Files, "gpt-5", false, store, cache.Metadata); !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("lock-wait cancellation error = %v, want deadline exceeded", err)
	}
	if err := os.WriteFile(release, []byte("release"), 0o600); err != nil {
		t.Fatal(err)
	}
	waitAtomicWorkers(t, []*atomicWorker{writer})
	snapshot, err := store.Load(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if _, exists := snapshot.Entries["stable.txt"]; !exists {
		t.Fatal("writer generation removed the existing stable entry")
	}
	if _, exists := snapshot.Entries["held.txt"]; !exists {
		t.Fatal("writer generation was not published")
	}
	if snapshot.Entries["stable.txt"].Size == int64(len("changed content\n")) {
		t.Fatal("canceled count update was published over the writer generation")
	}
}

func TestCountCacheCancellationBeforePreCommitLeavesPreviousGenerationInContainer(t *testing.T) {
	root, store := newCountFixture(t)
	writeTextFile(t, root, "stable.txt", "before\n")
	counter, err := tokenizer.NewCounter(tokenizer.CounterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	walk, err := fileops.WalkDirectory(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := counter.CountFilesWithCache(context.Background(), root, walk.Files, "gpt-5", false, store, cache.Metadata); err != nil {
		t.Fatal(err)
	}
	writeTextFile(t, root, "stable.txt", "after\n")
	ready := make(chan struct{})
	cache.SetManifestTestHook(func(ctx context.Context, _, _ string) error {
		close(ready)
		<-ctx.Done()
		return ctx.Err()
	})
	defer cache.SetManifestTestHook(nil)
	ctx, cancel := context.WithTimeout(context.Background(), time.Second)
	defer cancel()
	resultCh := make(chan error, 1)
	go func() {
		changedWalk, walkErr := fileops.WalkDirectory(context.Background(), root)
		if walkErr != nil {
			resultCh <- walkErr
			return
		}
		_, countErr := counter.CountFilesWithCache(ctx, root, changedWalk.Files, "gpt-5", false, store, cache.Metadata)
		resultCh <- countErr
	}()
	select {
	case <-ready:
		cancel()
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for pre-commit hook")
	}
	if err := <-resultCh; !errors.Is(err, context.Canceled) {
		t.Fatalf("pre-commit cancellation error = %v, want context.Canceled", err)
	}
	snapshot, err := store.Load(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if snapshot.Generation != 1 || snapshot.Entries["stable.txt"].Size != int64(len("before\n")) {
		t.Fatalf("pre-commit cancellation changed snapshot: %+v", snapshot)
	}
}

func TestCountCacheCorruptIncompatibleAndUnwritableFallbackInContainer(t *testing.T) {
	root, store := newCountFixture(t)
	writeTextFile(t, root, "stable.txt", "fallback fixture\n")
	stats := tokenizer.NewStats()
	counter, err := tokenizer.NewCounter(tokenizer.CounterOptions{Stats: stats})
	if err != nil {
		t.Fatal(err)
	}
	walk, err := fileops.WalkDirectory(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if _, err := counter.CountFilesWithCache(context.Background(), root, walk.Files, "gpt-5", false, store, cache.Metadata); err != nil {
		t.Fatal(err)
	}
	resolver, err := cache.NewLocationResolverAt(filepath.Join(filepath.Dir(root), "cache"))
	if err != nil {
		t.Fatal(err)
	}
	location, err := resolver.Resolve(root)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(location.ManifestPath, []byte("corrupt"), 0o600); err != nil {
		t.Fatal(err)
	}
	beforeWarnings := stats.Snapshot().CacheWarnings
	result, err := counter.CountFilesWithCache(context.Background(), root, walk.Files, "gpt-5", false, store, cache.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	cold, err := counter.CountFiles(context.Background(), walk.Files, "gpt-5", false)
	if err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, result, cold)
	if stats.Snapshot().CacheWarnings <= beforeWarnings {
		t.Fatalf("corrupt fallback result/warning mismatch: result=%+v cold=%+v stats=%+v", result, cold, stats.Snapshot())
	}

	snapshot, err := store.Load(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	data, err := cache.EncodeManifest(*snapshot)
	if err != nil {
		t.Fatal(err)
	}
	binary.LittleEndian.PutUint32(data[4:8], cache.CurrentSchemaVersion+1)
	if err := os.WriteFile(location.ManifestPath, data, 0o600); err != nil {
		t.Fatal(err)
	}
	incompatible, err := counter.CountFilesWithCache(context.Background(), root, walk.Files, "gpt-5", false, store, cache.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, incompatible, cold)

	unwritableResolver, err := cache.NewLocationResolverAt("/workspace")
	if err != nil {
		t.Fatal(err)
	}
	unwritable := cache.NewFileStore(unwritableResolver)
	result, err = counter.CountFilesWithCache(context.Background(), root, walk.Files, "gpt-5", false, unwritable, cache.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, result, cold)
	if result.Methods[0].Tokens != cold.Methods[0].Tokens {
		t.Fatalf("unwritable fallback result = %+v, cold = %+v", result, cold)
	}
	if stats.Snapshot().CacheWarnings <= beforeWarnings {
		t.Fatal("unwritable cache failure did not record a warning")
	}
}

type cancelAfterErrContext struct {
	context.Context
	remaining atomic.Int64
}

func (ctx *cancelAfterErrContext) Err() error {
	if ctx.remaining.Add(-1) < 0 {
		return context.Canceled
	}
	return nil
}

func newCancelAfterErrContext(remaining int64) *cancelAfterErrContext {
	ctx := &cancelAfterErrContext{Context: context.Background()}
	ctx.remaining.Store(remaining)
	return ctx
}
