//go:build container

package cachefs

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"reflect"
	"testing"

	"github.com/lancekrogers/tcount/internal/cache"
	"github.com/lancekrogers/tcount/tokenizer"
	"github.com/lancekrogers/tcount/tokenizer/fileops"
)

func TestCountCacheMutationOracleInContainer(t *testing.T) {
	root, store := newCountFixture(t)
	writeTextFile(t, root, "one.txt", "alpha beta\n")
	writeTextFile(t, root, "two.txt", "gamma delta\n")
	stats := tokenizer.NewStats()
	counter, err := tokenizer.NewCounter(tokenizer.CounterOptions{Stats: stats})
	if err != nil {
		t.Fatal(err)
	}

	before := stats.Snapshot()
	files := countOracle(t, root, counter, stats, store, cache.Verified, "gpt-5", false)
	assertTokenDelta(t, before, files.stats, "bpe_gpt_5", int64(len(files.files)))

	before = files.finalStats
	files = countOracle(t, root, counter, stats, store, cache.Verified, "gpt-5", false)
	assertTokenDelta(t, before, files.stats, "bpe_gpt_5", 0)
	if got := files.stats.FullFileOpens - before.FullFileOpens; got != int64(len(files.files)) {
		t.Fatalf("unchanged verified validation opens = %d, want %d", got, len(files.files))
	}

	writeTextFile(t, root, "two.txt", "gamma delta epsilon\n")
	before = files.finalStats
	files = countOracle(t, root, counter, stats, store, cache.Verified, "gpt-5", false)
	assertTokenDelta(t, before, files.stats, "bpe_gpt_5", 1)

	writeTextFile(t, root, "three.txt", "new file\n")
	before = files.finalStats
	files = countOracle(t, root, counter, stats, store, cache.Verified, "gpt-5", false)
	assertTokenDelta(t, before, files.stats, "bpe_gpt_5", 1)

	if err := os.Remove(filepath.Join(root, "one.txt")); err != nil {
		t.Fatal(err)
	}
	before = files.finalStats
	files = countOracle(t, root, counter, stats, store, cache.Verified, "gpt-5", false)
	assertTokenDelta(t, before, files.stats, "bpe_gpt_5", 0)

	if err := os.Rename(filepath.Join(root, "two.txt"), filepath.Join(root, "renamed.txt")); err != nil {
		t.Fatal(err)
	}
	before = files.finalStats
	files = countOracle(t, root, counter, stats, store, cache.Verified, "gpt-5", false)
	assertTokenDelta(t, before, files.stats, "bpe_gpt_5", 1)

	writeTextFile(t, root, "ignored.txt", "will be ignored\n")
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	before = files.finalStats
	files = countOracle(t, root, counter, stats, store, cache.Verified, "gpt-5", false)
	assertTokenDelta(t, before, files.stats, "bpe_gpt_5", 2)
	if err := os.WriteFile(filepath.Join(root, ".gitignore"), []byte("ignored.txt\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before = files.finalStats
	files = countOracle(t, root, counter, stats, store, cache.Verified, "gpt-5", false)
	assertTokenDelta(t, before, files.stats, "bpe_gpt_5", 1)
	if files.fileCount == 0 {
		t.Fatal("root-ignore mutation left no current files")
	}

	writeTextFile(t, root, "toggle.txt", "ordinary text\n")
	before = files.finalStats
	files = countOracle(t, root, counter, stats, store, cache.Verified, "gpt-5", false)
	assertTokenDelta(t, before, files.stats, "bpe_gpt_5", 1)
	if err := os.WriteFile(filepath.Join(root, "toggle.txt"), []byte{'a', 0, 'b'}, 0o644); err != nil {
		t.Fatal(err)
	}
	before = files.finalStats
	files = countOracle(t, root, counter, stats, store, cache.Verified, "gpt-5", false)
	assertTokenDelta(t, before, files.stats, "bpe_gpt_5", 0)
	if err := os.WriteFile(filepath.Join(root, "toggle.txt"), []byte("visible again\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	before = files.finalStats
	files = countOracle(t, root, counter, stats, store, cache.Verified, "gpt-5", false)
	assertTokenDelta(t, before, files.stats, "bpe_gpt_5", 1)
}

func TestTimestampPreservingCountMutationMetadataIsNotExactInContainer(t *testing.T) {
	root, store := newCountFixture(t)
	path := filepath.Join(root, "sample.txt")
	writeTextFile(t, root, "sample.txt", "alpha\n")
	stats := tokenizer.NewStats()
	counter, err := tokenizer.NewCounter(tokenizer.CounterOptions{Stats: stats})
	if err != nil {
		t.Fatal(err)
	}
	first := countOracle(t, root, counter, stats, store, cache.Metadata, "gpt-5", false)
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("bravo\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}

	metadata := countCached(t, root, counter, stats, store, cache.Metadata, "gpt-5", false)
	cold, err := counter.CountFiles(context.Background(), metadata.files, "gpt-5", false)
	if err != nil {
		t.Fatal(err)
	}
	if reflect.DeepEqual(metadata.result, cold) {
		t.Fatal("metadata mode unexpectedly treated timestamp-preserving rewrite as exact")
	}
	if first.result == nil {
		t.Fatal("initial metadata oracle did not produce a result")
	}

	beforeVerified := stats.Snapshot()
	verified := countOracle(t, root, counter, stats, store, cache.Verified, "gpt-5", false)
	assertJSONEqual(t, verified.result, cold)
	if verified.stats.FilesTokenizedByMethod["bpe_gpt_5"]-beforeVerified.FilesTokenizedByMethod["bpe_gpt_5"] != 1 {
		t.Fatalf("verified rewrite tokenizer calls = %d, want 1", verified.stats.FilesTokenizedByMethod["bpe_gpt_5"]-beforeVerified.FilesTokenizedByMethod["bpe_gpt_5"])
	}
}

type oracleRun struct {
	result     *tokenizer.CountResult
	stats      tokenizer.StatsSnapshot
	finalStats tokenizer.StatsSnapshot
	files      []string
	fileCount  int
}

func countOracle(t *testing.T, root string, counter *tokenizer.Counter, stats *tokenizer.Stats, store cache.Store, mode cache.ValidationMode, model string, all bool) oracleRun {
	t.Helper()
	run := countCached(t, root, counter, stats, store, mode, model, all)
	cold, err := counter.CountFiles(context.Background(), run.files, model, all)
	if err != nil {
		t.Fatal(err)
	}
	assertJSONEqual(t, run.result, cold)
	run.finalStats = stats.Snapshot()
	return run
}

func countCached(t *testing.T, root string, counter *tokenizer.Counter, stats *tokenizer.Stats, store cache.Store, mode cache.ValidationMode, model string, all bool) oracleRun {
	t.Helper()
	walk, err := fileops.WalkDirectory(context.Background(), root)
	if err != nil {
		t.Fatal(err)
	}
	if len(walk.Files) == 0 {
		t.Fatal("mutation fixture has no eligible files")
	}
	result, err := counter.CountFilesWithCache(context.Background(), root, walk.Files, model, all, store, mode)
	if err != nil {
		t.Fatal(err)
	}
	return oracleRun{result: result, stats: stats.Snapshot(), files: walk.Files, fileCount: len(walk.Files)}
}

func assertJSONEqual(t *testing.T, got, want *tokenizer.CountResult) {
	t.Helper()
	gotJSON, err := json.Marshal(got)
	if err != nil {
		t.Fatal(err)
	}
	wantJSON, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) || !reflect.DeepEqual(gotJSON, wantJSON) {
		t.Fatalf("cached and cold results differ:\n cached=%s\n cold=%s", gotJSON, wantJSON)
	}
}

func assertTokenDelta(t *testing.T, before, after tokenizer.StatsSnapshot, method string, want int64) {
	t.Helper()
	got := after.FilesTokenizedByMethod[method] - before.FilesTokenizedByMethod[method]
	if got != want {
		t.Fatalf("%s tokenizer delta = %d, want %d", method, got, want)
	}
}

func newCountFixture(t *testing.T) (string, *cache.FileStore) {
	t.Helper()
	base := t.TempDir()
	root := filepath.Join(base, "repository")
	if err := os.Mkdir(root, 0o755); err != nil {
		t.Fatal(err)
	}
	resolver, err := cache.NewLocationResolverAt(filepath.Join(base, "cache"))
	if err != nil {
		t.Fatal(err)
	}
	return root, cache.NewFileStore(resolver)
}
