package tokenizer

import (
	"context"
	"path/filepath"
	"reflect"
	"runtime"
	"testing"

	"github.com/lancekrogers/tcount/tokenizer/fileops"
)

func TestStatsSnapshotRecordsStagesAndMethods(t *testing.T) {
	stats := NewStats()
	stats.RecordEntryVisited()
	stats.RecordEligibleFile()
	stats.RecordBinarySniffOpen()
	stats.RecordBinarySniffBytes(12)
	stats.RecordFullFileOpen()
	stats.RecordFullFileBytes(24)
	stats.RecordTokenizedFile("bpe_o200k_base")
	stats.RecordTokenizedFile("bpe_o200k_base")
	stats.RecordCacheBytesReused(24)
	stats.ObserveMemory()

	snapshot := stats.Snapshot()
	if snapshot.EntriesVisited != 1 || snapshot.EligibleFiles != 1 {
		t.Fatalf("snapshot entries/files = %d/%d, want 1/1", snapshot.EntriesVisited, snapshot.EligibleFiles)
	}
	if snapshot.BinarySniffOpens != 1 || snapshot.BinarySniffBytes != 12 {
		t.Fatalf("snapshot sniff stats = %d/%d, want 1/12", snapshot.BinarySniffOpens, snapshot.BinarySniffBytes)
	}
	if snapshot.FullFileOpens != 1 || snapshot.FullFileBytes != 24 {
		t.Fatalf("snapshot full-read stats = %d/%d, want 1/24", snapshot.FullFileOpens, snapshot.FullFileBytes)
	}
	if snapshot.FilesTokenizedByMethod["bpe_o200k_base"] != 2 {
		t.Fatalf("tokenized files = %v, want bpe_o200k_base=2", snapshot.FilesTokenizedByMethod)
	}
	if snapshot.CacheBytesReused != 24 {
		t.Fatalf("cache bytes reused = %d, want 24", snapshot.CacheBytesReused)
	}
	if snapshot.PeakHeapAllocBytes == 0 {
		t.Fatal("peak heap allocation was not recorded")
	}
}

func TestCountFilesStatsMatchReadAndTokenizationWork(t *testing.T) {
	fixture := integrationFixturePath(t)
	ctx := context.Background()

	stats := NewStats()
	counter, err := NewCounter(CounterOptions{Stats: stats})
	if err != nil {
		t.Fatal(err)
	}
	walk, err := fileops.WalkDirectory(ctx, fixture, stats)
	if err != nil {
		t.Fatal(err)
	}
	got, err := counter.CountFiles(ctx, walk.Files, "gpt-5", false)
	if err != nil {
		t.Fatal(err)
	}

	snapshot := stats.Snapshot()
	if snapshot.EligibleFiles != int64(len(walk.Files)) {
		t.Fatalf("eligible files = %d, want %d", snapshot.EligibleFiles, len(walk.Files))
	}
	if snapshot.FullFileOpens != int64(len(walk.Files)) {
		t.Fatalf("full file opens = %d, want %d", snapshot.FullFileOpens, len(walk.Files))
	}
	if snapshot.FullFileBytes != int64(got.FileSize) {
		t.Fatalf("full file bytes = %d, want %d", snapshot.FullFileBytes, got.FileSize)
	}
	if snapshot.FilesTokenizedByMethod["bpe_gpt_5"] != int64(len(walk.Files)) {
		t.Fatalf("tokenized files = %v, want bpe_gpt_5=%d", snapshot.FilesTokenizedByMethod, len(walk.Files))
	}
	if snapshot.WalkDuration <= 0 || snapshot.ValidationReadDuration <= 0 || snapshot.TokenizationDuration <= 0 || snapshot.AggregationDuration <= 0 || snapshot.PersistenceReadyDuration <= 0 {
		t.Fatalf("stage durations = %+v, want all positive", snapshot)
	}

	plainCounter, err := NewCounter(CounterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	plain, err := plainCounter.CountFiles(ctx, walk.Files, "gpt-5", false)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, plain) {
		t.Fatalf("instrumented result changed count output:\n got %#v\nwant %#v", got, plain)
	}
}

func integrationFixturePath(t *testing.T) string {
	t.Helper()
	_, filename, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("runtime.Caller failed")
	}
	return filepath.Join(filepath.Dir(filename), "..", "tests", "integration", "testdata", "walkdir")
}
