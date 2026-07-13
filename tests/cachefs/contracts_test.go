//go:build container

package cachefs

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/lancekrogers/tcount/internal/cache"
	"github.com/lancekrogers/tcount/tokenizer"
	"google.golang.org/protobuf/encoding/protowire"
)

func TestCountCacheContractMutationOracleInContainer(t *testing.T) {
	root, store := newCountFixture(t)
	writeTextFile(t, root, "one.txt", "alpha beta\n")
	writeTextFile(t, root, "two.txt", "gamma delta\n")
	stats := tokenizer.NewStats()
	counter, err := tokenizer.NewCounter(tokenizer.CounterOptions{Stats: stats})
	if err != nil {
		t.Fatal(err)
	}
	countOracle(t, root, counter, stats, store, cache.Metadata, "gpt-5", false)

	before := stats.Snapshot()
	alias := countOracle(t, root, counter, stats, store, cache.Metadata, "gpt-5-mini", false)
	assertTokenDelta(t, before, alias.stats, "bpe_gpt_5_mini", 0)

	before = stats.Snapshot()
	changedEncoding := countOracle(t, root, counter, stats, store, cache.Metadata, "gpt-4", false)
	assertTokenDelta(t, before, changedEncoding.stats, "bpe_gpt_4", int64(len(changedEncoding.files)))
}

func TestCountCacheRatioChangeReusesReducibleTotalsInContainer(t *testing.T) {
	root, store := newCountFixture(t)
	writeTextFile(t, root, "one.txt", "alpha beta gamma\n")
	writeTextFile(t, root, "two.txt", "delta epsilon zeta\n")
	firstStats := tokenizer.NewStats()
	first, err := tokenizer.NewCounter(tokenizer.CounterOptions{
		CharsPerToken: 4,
		WordsPerToken: 0.75,
		Stats:         firstStats,
	})
	if err != nil {
		t.Fatal(err)
	}
	countOracle(t, root, first, firstStats, store, cache.Verified, "", true)

	secondStats := tokenizer.NewStats()
	second, err := tokenizer.NewCounter(tokenizer.CounterOptions{
		CharsPerToken: 2,
		WordsPerToken: 0.5,
		Stats:         secondStats,
	})
	if err != nil {
		t.Fatal(err)
	}
	before := secondStats.Snapshot()
	run := countOracle(t, root, second, secondStats, store, cache.Verified, "", true)
	for method, calls := range run.stats.FilesTokenizedByMethod {
		if calls != before.FilesTokenizedByMethod[method] {
			t.Fatalf("ratio-only cache run tokenized %s: %d calls, want 0", method, calls-before.FilesTokenizedByMethod[method])
		}
	}
}

func TestCountCacheVocabularyMutationInvalidatesContractInContainer(t *testing.T) {
	root, store := newCountFixture(t)
	writeTextFile(t, root, "one.txt", "ab ab\n")
	modelA := filepath.Join(filepath.Dir(root), "vocab-a.model")
	modelB := filepath.Join(filepath.Dir(root), "vocab-b.model")
	if err := os.WriteFile(modelA, tinyBPEModel(true), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(modelB, tinyBPEModel(false), 0o600); err != nil {
		t.Fatal(err)
	}

	firstStats := tokenizer.NewStats()
	first, err := tokenizer.NewCounter(tokenizer.CounterOptions{VocabFile: modelA, Stats: firstStats})
	if err != nil {
		t.Fatal(err)
	}
	countOracle(t, root, first, firstStats, store, cache.Verified, "spm", false)

	secondStats := tokenizer.NewStats()
	second, err := tokenizer.NewCounter(tokenizer.CounterOptions{VocabFile: modelB, Stats: secondStats})
	if err != nil {
		t.Fatal(err)
	}
	before := secondStats.Snapshot()
	run := countOracle(t, root, second, secondStats, store, cache.Verified, "spm", false)
	assertTokenDelta(t, before, run.stats, "spm", int64(len(run.files)))
}

func tinyBPEModel(includeMerge bool) []byte {
	model := make([]byte, 0, 128)
	model = appendMessage(model, 1, sentencePiece("<unk>", 2))
	model = appendMessage(model, 1, sentencePiece("a", 1))
	model = appendMessage(model, 1, sentencePiece("b", 1))
	if includeMerge {
		model = appendMessage(model, 1, sentencePiece("ab", 1))
	}

	trainer := make([]byte, 0, 4)
	trainer = appendVarintField(trainer, 3, 2)
	model = appendMessage(model, 2, trainer)
	normalizer := make([]byte, 0, 8)
	normalizer = appendVarintField(normalizer, 3, 0)
	normalizer = appendVarintField(normalizer, 4, 0)
	return appendMessage(model, 3, normalizer)
}

func sentencePiece(piece string, kind uint64) []byte {
	message := make([]byte, 0, len(piece)+8)
	message = appendStringField(message, 1, piece)
	return appendVarintField(message, 3, kind)
}

func appendMessage(dst []byte, field protowire.Number, message []byte) []byte {
	dst = protowire.AppendTag(dst, field, protowire.BytesType)
	return protowire.AppendBytes(dst, message)
}

func appendStringField(dst []byte, field protowire.Number, value string) []byte {
	dst = protowire.AppendTag(dst, field, protowire.BytesType)
	return protowire.AppendString(dst, value)
}

func appendVarintField(dst []byte, field protowire.Number, value uint64) []byte {
	dst = protowire.AppendTag(dst, field, protowire.VarintType)
	return protowire.AppendVarint(dst, value)
}
