//go:build container

package cachefs

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/lancekrogers/tcount/internal/cache"
	"github.com/lancekrogers/tcount/tokenizer"
	"github.com/lancekrogers/tcount/tokenizer/fileops"
)

func TestPersistentCacheReuseAndSelectiveMissesInContainer(t *testing.T) {
	root := t.TempDir()
	cacheBase := t.TempDir()
	writeTextFile(t, root, "one.txt", "alpha beta\n")
	writeTextFile(t, root, "two.txt", "gamma delta\n")
	ctx := context.Background()

	resolver, err := cache.NewLocationResolverAt(cacheBase)
	if err != nil {
		t.Fatal(err)
	}
	store := cache.NewFileStore(resolver)
	stats := tokenizer.NewStats()
	counter, err := tokenizer.NewCounter(tokenizer.CounterOptions{Stats: stats})
	if err != nil {
		t.Fatal(err)
	}

	walk, err := fileops.WalkDirectory(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	first, err := counter.CountFilesWithCache(ctx, root, walk.Files, "gpt-5", false, store, cache.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	afterFirst := stats.Snapshot()
	if afterFirst.FilesTokenizedByMethod["bpe_gpt_5"] != int64(len(walk.Files)) {
		t.Fatalf("cold tokenizer calls = %v, want %d", afterFirst.FilesTokenizedByMethod, len(walk.Files))
	}

	warmWalk, err := fileops.WalkDirectory(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	warm, err := counter.CountFilesWithCache(ctx, root, warmWalk.Files, "gpt-5", false, store, cache.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	afterWarm := stats.Snapshot()
	if afterWarm.FilesTokenizedByMethod["bpe_gpt_5"] != afterFirst.FilesTokenizedByMethod["bpe_gpt_5"] {
		t.Fatalf("warm tokenizer calls changed from %d to %d", afterFirst.FilesTokenizedByMethod["bpe_gpt_5"], afterWarm.FilesTokenizedByMethod["bpe_gpt_5"])
	}
	if !reflect.DeepEqual(first, warm) {
		t.Fatalf("warm result differs from cold result:\n got %#v\nwant %#v", warm, first)
	}

	verifiedResolver, err := cache.NewLocationResolverAt(filepath.Join(cacheBase, "verified"))
	if err != nil {
		t.Fatal(err)
	}
	verifiedStore := cache.NewFileStore(verifiedResolver)
	beforeVerified := stats.Snapshot()
	if _, err := counter.CountFilesWithCache(ctx, root, warmWalk.Files, "gpt-5", false, verifiedStore, cache.Verified); err != nil {
		t.Fatal(err)
	}
	afterVerified := stats.Snapshot()
	if delta := afterVerified.FullFileOpens - beforeVerified.FullFileOpens; delta != int64(len(walk.Files)) {
		t.Fatalf("verified cold opens = %d, want %d (one validation read per file)", delta, len(walk.Files))
	}
	if delta := afterVerified.FilesTokenizedByMethod["bpe_gpt_5"] - beforeVerified.FilesTokenizedByMethod["bpe_gpt_5"]; delta != int64(len(walk.Files)) {
		t.Fatalf("verified cold tokenizer calls = %d, want %d", delta, len(walk.Files))
	}

	writeTextFile(t, root, "two.txt", "gamma delta epsilon\n")
	changedWalk, err := fileops.WalkDirectory(ctx, root)
	if err != nil {
		t.Fatal(err)
	}
	changed, err := counter.CountFilesWithCache(ctx, root, changedWalk.Files, "gpt-5", false, store, cache.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	afterChanged := stats.Snapshot()
	if delta := afterChanged.FilesTokenizedByMethod["bpe_gpt_5"] - afterVerified.FilesTokenizedByMethod["bpe_gpt_5"]; delta != 1 {
		t.Fatalf("changed-file tokenizer calls = %d, want 1", delta)
	}

	coldCounter, err := tokenizer.NewCounter(tokenizer.CounterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	cold, err := coldCounter.CountFiles(ctx, changedWalk.Files, "gpt-5", false)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(changed, cold) {
		t.Fatalf("changed cached result differs from forced cold result:\n got %#v\nwant %#v", changed, cold)
	}

	beforeVerifiedChange := stats.Snapshot()
	verifiedChanged, err := counter.CountFilesWithCache(ctx, root, changedWalk.Files, "gpt-5", false, verifiedStore, cache.Verified)
	if err != nil {
		t.Fatal(err)
	}
	afterVerifiedChange := stats.Snapshot()
	if delta := afterVerifiedChange.FullFileOpens - beforeVerifiedChange.FullFileOpens; delta != int64(len(changedWalk.Files)) {
		t.Fatalf("verified changed opens = %d, want %d (no second miss read)", delta, len(changedWalk.Files))
	}
	if delta := afterVerifiedChange.FilesTokenizedByMethod["bpe_gpt_5"] - beforeVerifiedChange.FilesTokenizedByMethod["bpe_gpt_5"]; delta != 1 {
		t.Fatalf("verified changed tokenizer calls = %d, want 1", delta)
	}
	if !reflect.DeepEqual(verifiedChanged, cold) {
		t.Fatalf("verified changed result differs from forced cold result:\n got %#v\nwant %#v", verifiedChanged, cold)
	}
}

func writeTextFile(t *testing.T, root, name, content string) {
	t.Helper()
	if err := os.WriteFile(filepath.Join(root, name), []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}
}
