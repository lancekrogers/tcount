package commands

import (
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/lancekrogers/tcount/internal/cache"
	"github.com/lancekrogers/tcount/internal/ui"
	"github.com/lancekrogers/tcount/tokenizer"
)

const cacheDirEnvironment = "TCOUNT_CACHE_DIR"

func newCacheStore() (*cache.FileStore, error) {
	baseDir := os.Getenv(cacheDirEnvironment)
	if baseDir != "" {
		info, err := os.Stat(baseDir)
		if err == nil && !info.IsDir() {
			return nil, fmt.Errorf("%s must name a directory: %s", cacheDirEnvironment, baseDir)
		}
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("checking %s=%q: %w", cacheDirEnvironment, baseDir, err)
		}
		resolver, err := cache.NewLocationResolverAt(baseDir)
		if err != nil {
			return nil, fmt.Errorf("using %s=%q: %w", cacheDirEnvironment, baseDir, err)
		}
		return cache.NewFileStore(resolver), nil
	}

	store, err := cache.NewDefaultFileStore()
	if err != nil {
		return nil, err
	}
	return store, nil
}

func cacheDiagnosticsMode(opts *countOptions) string {
	if !opts.cache {
		return "disabled"
	}
	if opts.cacheVerify {
		return cache.Verified.String()
	}
	return cache.Metadata.String()
}

func outputStats(display *ui.UI, stats tokenizer.StatsSnapshot, validationMode string) {
	display.Diagnostic("Cache diagnostics: mode=%s files=%d hits=%d partial_hits=%d misses=%d incompatibilities=%d methods_avoided=%d reused_bytes=%d read_bytes=%d tokenizer_calls=%d warnings=%d stages=walk:%s,validation_read:%s,tokenization:%s,aggregation:%s,persistence:%s reasons=%s",
		validationMode,
		stats.EligibleFiles,
		stats.CacheHits,
		stats.CachePartialHits,
		stats.CacheMisses,
		cacheIncompatibilities(stats.CacheReasons),
		stats.CacheMethodsAvoided,
		stats.CacheBytesReused,
		stats.FullFileBytes,
		tokenizerCalls(stats.FilesTokenizedByMethod),
		stats.CacheWarnings,
		stats.WalkDuration,
		stats.ValidationReadDuration,
		stats.TokenizationDuration,
		stats.AggregationDuration,
		stats.PersistenceReadyDuration,
		formatCacheReasons(stats.CacheReasons),
	)
}

func tokenizerCalls(byMethod map[string]int64) int64 {
	var calls int64
	for _, count := range byMethod {
		calls += count
	}
	return calls
}

func cacheIncompatibilities(reasons map[string]int64) int64 {
	var total int64
	for reason, count := range reasons {
		switch cache.InvalidationReason(reason) {
		case cache.ReasonSchemaMismatch,
			cache.ReasonRootMismatch,
			cache.ReasonPathChanged,
			cache.ReasonSizeChanged,
			cache.ReasonModTimeChanged,
			cache.ReasonContentChanged,
			cache.ReasonClassificationChanged,
			cache.ReasonContractMissing:
			total += count
		}
	}
	return total
}

func formatCacheReasons(reasons map[string]int64) string {
	keys := make([]string, 0, len(reasons))
	for reason, count := range reasons {
		if count > 0 {
			keys = append(keys, reason)
		}
	}
	if len(keys) == 0 {
		return "none"
	}
	slices.Sort(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", key, reasons[key]))
	}
	return strings.Join(parts, ",")
}
