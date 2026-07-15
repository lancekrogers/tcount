package tokenizer

import (
	"context"
	"errors"
	"reflect"
	"testing"

	"github.com/lancekrogers/tcount/internal/cache"
	"github.com/lancekrogers/tcount/tokenizer/fileops"
)

func TestCountFilesWithCacheReusesCompleteAndPartialResults(t *testing.T) {
	fixture := integrationFixturePath(t)
	ctx := context.Background()
	walk, err := fileops.WalkDirectory(ctx, fixture)
	if err != nil {
		t.Fatal(err)
	}

	stats := NewStats()
	counter, err := NewCounter(CounterOptions{Stats: stats})
	if err != nil {
		t.Fatal(err)
	}
	store := cache.NewMemoryStore()
	if _, err := counter.CountFilesWithCache(ctx, fixture, walk.Files, "gpt-5", false, store, cache.Metadata); err != nil {
		t.Fatal(err)
	}
	afterFirst := stats.Snapshot()
	firstBPECalls := afterFirst.FilesTokenizedByMethod["bpe_gpt_5"]
	if firstBPECalls != int64(len(walk.Files)) {
		t.Fatalf("first-run BPE calls = %d, want %d", firstBPECalls, len(walk.Files))
	}

	allResult, err := counter.CountFilesWithCache(ctx, fixture, walk.Files, "", true, store, cache.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	afterPartial := stats.Snapshot()
	if afterPartial.FilesTokenizedByMethod["bpe_o200k_base"] != 0 {
		t.Fatalf("partial-hit encoding BPE calls = %d, want 0", afterPartial.FilesTokenizedByMethod["bpe_o200k_base"])
	}
	for _, method := range []string{"bpe_cl100k_base", "claude_3_approx", "gemini_approx"} {
		if got := afterPartial.FilesTokenizedByMethod[method] - afterFirst.FilesTokenizedByMethod[method]; got != int64(len(walk.Files)) {
			t.Fatalf("partial-hit %s calls = %d, want %d", method, got, len(walk.Files))
		}
	}
	if afterPartial.CachePartialHits != int64(len(walk.Files)) || afterPartial.CacheMethodsAvoided != int64(len(walk.Files)) {
		t.Fatalf("partial cache metrics = hits=%d avoided=%d, want hits=%d avoided=%d", afterPartial.CachePartialHits, afterPartial.CacheMethodsAvoided, len(walk.Files), len(walk.Files))
	}

	coldCounter, err := NewCounter(CounterOptions{})
	if err != nil {
		t.Fatal(err)
	}
	coldResult, err := coldCounter.CountFiles(ctx, walk.Files, "", true)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(allResult, coldResult) {
		t.Fatalf("cached all-method result differs from cold result:\n got %#v\nwant %#v", allResult, coldResult)
	}

	if _, err := counter.CountFilesWithCache(ctx, fixture, walk.Files, "", true, store, cache.Metadata); err != nil {
		t.Fatal(err)
	}
	afterComplete := stats.Snapshot()
	if afterComplete.FilesTokenizedByMethod["bpe_o200k_base"] != afterPartial.FilesTokenizedByMethod["bpe_o200k_base"] {
		t.Fatalf("complete-hit BPE calls changed from %d to %d", afterPartial.FilesTokenizedByMethod["bpe_o200k_base"], afterComplete.FilesTokenizedByMethod["bpe_o200k_base"])
	}
	for _, method := range []string{"bpe_cl100k_base", "claude_3_approx", "gemini_approx"} {
		if afterComplete.FilesTokenizedByMethod[method] != afterPartial.FilesTokenizedByMethod[method] {
			t.Fatalf("complete-hit %s calls changed from %d to %d", method, afterPartial.FilesTokenizedByMethod[method], afterComplete.FilesTokenizedByMethod[method])
		}
	}
	if afterComplete.CacheHits != int64(len(walk.Files)) {
		t.Fatalf("complete cache hits = %d, want %d", afterComplete.CacheHits, len(walk.Files))
	}
}

func TestCountFilesWithCacheFailureFallsBackToColdResult(t *testing.T) {
	fixture := integrationFixturePath(t)
	ctx := context.Background()
	walk, err := fileops.WalkDirectory(ctx, fixture)
	if err != nil {
		t.Fatal(err)
	}

	stats := NewStats()
	counter, err := NewCounter(CounterOptions{Stats: stats})
	if err != nil {
		t.Fatal(err)
	}
	got, err := counter.CountFilesWithCache(ctx, fixture, walk.Files, "gpt-5", false, failingCacheStore{}, cache.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	want, err := counter.CountFiles(ctx, walk.Files, "gpt-5", false)
	if err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("cache failure changed result:\n got %#v\nwant %#v", got, want)
	}
	if stats.Snapshot().CacheWarnings == 0 {
		t.Fatal("cache failure did not record a warning")
	}
}

func TestRetainVerifiedContentOnlyForFilesThatNeedCounting(t *testing.T) {
	contract := cache.ContractKey{Method: "bpe", Encoding: "o200k_base"}
	digest := [32]byte{1}
	identity := cache.FileIdentity{
		RelativePath:   "one.txt",
		Size:           10,
		ModTimeNS:      20,
		ContentDigest:  digest,
		Classification: cache.ClassificationText,
	}
	complete := &cache.Snapshot{
		SchemaVersion: cache.CurrentSchemaVersion,
		Root:          "/root",
		Entries: map[string]cache.FileResult{
			"one.txt": {
				Size:           identity.Size,
				ModTimeNS:      identity.ModTimeNS,
				ContentDigest:  identity.ContentDigest,
				Classification: identity.Classification,
				Methods:        map[cache.ContractKey]int{contract: 10},
			},
		},
	}

	tests := []struct {
		name     string
		snapshot *cache.Snapshot
		identity cache.FileIdentity
		want     bool
	}{
		{name: "cold snapshot", want: false},
		{name: "complete hit", snapshot: complete, identity: identity, want: false},
		{name: "partial contract hit", snapshot: complete, identity: identity, want: true},
		{name: "changed digest", snapshot: complete, identity: cache.FileIdentity{RelativePath: identity.RelativePath, Size: identity.Size, ModTimeNS: identity.ModTimeNS, ContentDigest: [32]byte{2}, Classification: identity.Classification}, want: true},
		{name: "new path", snapshot: complete, identity: cache.FileIdentity{RelativePath: "new.txt", Size: identity.Size, ModTimeNS: identity.ModTimeNS, ContentDigest: identity.ContentDigest, Classification: identity.Classification}, want: true},
	}
	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			contracts := []cache.ContractKey{contract}
			if test.name == "partial contract hit" {
				partial := *complete
				partial.Entries = map[string]cache.FileResult{"one.txt": complete.Entries["one.txt"]}
				entry := partial.Entries["one.txt"]
				entry.Methods = nil
				partial.Entries["one.txt"] = entry
				test.snapshot = &partial
			}
			if got := retainVerifiedContent(test.snapshot, test.identity.RelativePath, test.identity, contracts); got != test.want {
				t.Fatalf("retainVerifiedContent() = %t, want %t", got, test.want)
			}
		})
	}
}

type failingCacheStore struct{}

func (failingCacheStore) Load(context.Context, string) (*cache.Snapshot, error) {
	return nil, errors.New("cache unavailable for test")
}

func (failingCacheStore) Commit(context.Context, string, uint64, cache.UpdateSet) error {
	return errors.New("cache commit unavailable for test")
}

func (failingCacheStore) CommitAndPrune(context.Context, string, uint64, cache.UpdateSet, cache.PruneOptions) error {
	return errors.New("cache commit unavailable for test")
}

func (failingCacheStore) Status(context.Context, string) (cache.Status, error) {
	return cache.Status{}, errors.New("cache status unavailable for test")
}

func (failingCacheStore) Clear(context.Context, string) error {
	return errors.New("cache clear unavailable for test")
}
