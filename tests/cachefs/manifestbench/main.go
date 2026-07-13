package main

import (
	"context"
	"crypto/sha256"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"time"

	"github.com/lancekrogers/tcount/internal/cache"
)

func main() {
	entries := flag.Int("entries", 0, "number of logical entries")
	samples := flag.Int("samples", 3, "number of samples")
	flag.Parse()
	if *entries < 1 || *samples < 1 {
		fail("positive entries and samples are required")
	}

	manifest := buildManifest(*entries)
	encoded, encodeDuration := encode(manifest)
	decodeSeconds := make([]float64, 0, *samples)
	mergeSeconds := make([]float64, 0, *samples)
	writeSeconds := make([]float64, 0, *samples)
	allocBytes := make([]uint64, 0, *samples)
	temporary, err := os.MkdirTemp("", "tcount-manifest-bench-")
	if err != nil {
		fail(err.Error())
	}
	defer func() { _ = os.RemoveAll(temporary) }()
	manifestPath := filepath.Join(temporary, "manifest")
	updates := buildUpdates(manifest)

	for sample := 1; sample <= *samples; sample++ {
		before := runtimeMem()
		started := time.Now()
		decoded, err := cache.DecodeManifest(encoded)
		decodeDuration := time.Since(started)
		after := runtimeMem()
		if err != nil {
			fail(err.Error())
		}
		started = time.Now()
		if _, err := cache.MergeEntries(decoded, updates); err != nil {
			fail(err.Error())
		}
		mergeDuration := time.Since(started)
		started = time.Now()
		if err := cache.WriteManifestAtomic(context.Background(), manifestPath, manifest); err != nil {
			fail(err.Error())
		}
		writeDuration := time.Since(started)

		decodeSeconds = append(decodeSeconds, decodeDuration.Seconds())
		mergeSeconds = append(mergeSeconds, mergeDuration.Seconds())
		writeSeconds = append(writeSeconds, writeDuration.Seconds())
		allocBytes = append(allocBytes, after-before)
		fmt.Printf("manifest entries=%d sample=%d bytes=%d decode_seconds=%.6f decode_alloc_bytes=%d merge_seconds=%.6f encode_seconds=%.6f atomic_write_seconds=%.6f updates=%d\n", *entries, sample, len(encoded), decodeDuration.Seconds(), after-before, mergeDuration.Seconds(), encodeDuration.Seconds(), writeDuration.Seconds(), len(updates))
	}

	fmt.Printf("manifest summary entries=%d bytes=%d decode_median_seconds=%.6f decode_p95_seconds=%.6f decode_alloc_median_bytes=%d merge_median_seconds=%.6f atomic_write_median_seconds=%.6f\n", *entries, len(encoded), median(decodeSeconds), p95(decodeSeconds), medianUint(allocBytes), median(mergeSeconds), median(writeSeconds))
}

func buildManifest(count int) cache.Manifest {
	methods := map[cache.ContractKey]int{
		{Method: "bpe_gpt_5", Encoding: "o200k_base", Implementation: "bpe-v1", NormalizationPolicy: "default", SpecialTokenPolicy: "default"}:     10,
		{Method: "bpe_gpt_4o", Encoding: "o200k_base", Implementation: "bpe-v1", NormalizationPolicy: "default", SpecialTokenPolicy: "default"}:    11,
		{Method: "whitespace_split", Encoding: "approx", Implementation: "builtin-v1", NormalizationPolicy: "default", SpecialTokenPolicy: "none"}: 3,
	}
	entries := make(map[string]cache.FileEntry, count)
	for i := 0; i < count; i++ {
		path := fmt.Sprintf("pkg/file-%06d.txt", i)
		digest := sha256.Sum256([]byte(path))
		entries[path] = cache.FileEntry{Size: 128, ModTimeNS: int64(i), ContentDigest: digest, Classification: cache.ClassificationText, Characters: 128, Words: 20, Lines: 4, Methods: methods}
	}
	return cache.Manifest{SchemaVersion: cache.CurrentSchemaVersion, Root: "/workspace/fixture", Generation: 7, Entries: entries}
}

func buildUpdates(manifest cache.Manifest) cache.UpdateSet {
	updates := make(cache.UpdateSet)
	for path, entry := range manifest.Entries {
		if len(updates) >= max(1, len(manifest.Entries)/100) {
			break
		}
		entry.Characters++
		updates[path] = entry
	}
	return updates
}

func encode(manifest cache.Manifest) ([]byte, time.Duration) {
	started := time.Now()
	encoded, err := cache.EncodeManifest(manifest)
	if err != nil {
		fail(err.Error())
	}
	return encoded, time.Since(started)
}

func runtimeMem() uint64 {
	var stats runtime.MemStats
	runtime.ReadMemStats(&stats)
	return stats.TotalAlloc
}

func median(values []float64) float64 {
	ordered := append([]float64(nil), values...)
	sort.Float64s(ordered)
	return ordered[(len(ordered)-1)/2]
}

func p95(values []float64) float64 {
	ordered := append([]float64(nil), values...)
	sort.Float64s(ordered)
	return ordered[((len(ordered)*95+99)/100)-1]
}

func medianUint(values []uint64) uint64 {
	ordered := append([]uint64(nil), values...)
	sort.Slice(ordered, func(i, j int) bool { return ordered[i] < ordered[j] })
	return ordered[(len(ordered)-1)/2]
}

func max(left, right int) int {
	if left > right {
		return left
	}
	return right
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
