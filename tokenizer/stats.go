package tokenizer

import (
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"github.com/lancekrogers/tcount/tokenizer/fileops"
)

// Stats collects optional, benchmark-oriented measurements for a count run.
// A nil *Stats disables instrumentation and preserves the normal counting path.
type Stats struct {
	entriesVisited      atomic.Int64
	eligibleFiles       atomic.Int64
	binarySniffOpens    atomic.Int64
	binarySniffBytes    atomic.Int64
	fullFileOpens       atomic.Int64
	fullFileBytes       atomic.Int64
	walkNanos           atomic.Int64
	validationReadNanos atomic.Int64
	tokenizationNanos   atomic.Int64
	aggregationNanos    atomic.Int64
	persistenceNanos    atomic.Int64
	peakHeapAlloc       atomic.Uint64
	cacheHits           atomic.Int64
	cachePartialHits    atomic.Int64
	cacheMisses         atomic.Int64
	cacheMethodsAvoided atomic.Int64
	cacheWarnings       atomic.Int64

	tokenizedMu       sync.Mutex
	tokenizedByMethod map[string]int64
	cacheReasonsMu    sync.Mutex
	cacheReasons      map[string]int64
}

// StatsSnapshot is the immutable view of one Stats collector.
type StatsSnapshot struct {
	EntriesVisited           int64
	EligibleFiles            int64
	BinarySniffOpens         int64
	BinarySniffBytes         int64
	FullFileOpens            int64
	FullFileBytes            int64
	FilesTokenizedByMethod   map[string]int64
	WalkDuration             time.Duration
	ValidationReadDuration   time.Duration
	TokenizationDuration     time.Duration
	AggregationDuration      time.Duration
	PersistenceReadyDuration time.Duration
	PeakHeapAllocBytes       uint64
	CacheHits                int64
	CachePartialHits         int64
	CacheMisses              int64
	CacheMethodsAvoided      int64
	CacheWarnings            int64
	CacheReasons             map[string]int64
}

// NewStats creates an enabled instrumentation collector.
func NewStats() *Stats {
	return &Stats{tokenizedByMethod: make(map[string]int64), cacheReasons: make(map[string]int64)}
}

// Snapshot returns a race-free copy of the measurements collected so far.
func (s *Stats) Snapshot() StatsSnapshot {
	if s == nil {
		return StatsSnapshot{}
	}

	s.tokenizedMu.Lock()
	tokenized := make(map[string]int64, len(s.tokenizedByMethod))
	for method, count := range s.tokenizedByMethod {
		tokenized[method] = count
	}
	s.tokenizedMu.Unlock()

	s.cacheReasonsMu.Lock()
	reasons := make(map[string]int64, len(s.cacheReasons))
	for reason, count := range s.cacheReasons {
		reasons[reason] = count
	}
	s.cacheReasonsMu.Unlock()

	return StatsSnapshot{
		EntriesVisited:           s.entriesVisited.Load(),
		EligibleFiles:            s.eligibleFiles.Load(),
		BinarySniffOpens:         s.binarySniffOpens.Load(),
		BinarySniffBytes:         s.binarySniffBytes.Load(),
		FullFileOpens:            s.fullFileOpens.Load(),
		FullFileBytes:            s.fullFileBytes.Load(),
		FilesTokenizedByMethod:   tokenized,
		WalkDuration:             time.Duration(s.walkNanos.Load()),
		ValidationReadDuration:   time.Duration(s.validationReadNanos.Load()),
		TokenizationDuration:     time.Duration(s.tokenizationNanos.Load()),
		AggregationDuration:      time.Duration(s.aggregationNanos.Load()),
		PersistenceReadyDuration: time.Duration(s.persistenceNanos.Load()),
		PeakHeapAllocBytes:       s.peakHeapAlloc.Load(),
		CacheHits:                s.cacheHits.Load(),
		CachePartialHits:         s.cachePartialHits.Load(),
		CacheMisses:              s.cacheMisses.Load(),
		CacheMethodsAvoided:      s.cacheMethodsAvoided.Load(),
		CacheWarnings:            s.cacheWarnings.Load(),
		CacheReasons:             reasons,
	}
}

func (s *Stats) RecordEntryVisited() {
	if s != nil {
		s.entriesVisited.Add(1)
	}
}

func (s *Stats) RecordEligibleFile() {
	if s != nil {
		s.eligibleFiles.Add(1)
	}
}

func (s *Stats) RecordBinarySniffOpen() {
	if s != nil {
		s.binarySniffOpens.Add(1)
	}
}

func (s *Stats) RecordBinarySniffBytes(bytes int64) {
	if s != nil {
		s.binarySniffBytes.Add(bytes)
	}
}

func (s *Stats) RecordFullFileOpen() {
	if s != nil {
		s.fullFileOpens.Add(1)
	}
}

func (s *Stats) RecordFullFileBytes(bytes int64) {
	if s != nil {
		s.fullFileBytes.Add(bytes)
	}
}

func (s *Stats) RecordWalkDuration(duration time.Duration) {
	if s != nil {
		s.walkNanos.Add(int64(duration))
	}
}

func (s *Stats) RecordValidationReadDuration(duration time.Duration) {
	if s != nil {
		s.validationReadNanos.Add(int64(duration))
	}
}

func (s *Stats) RecordTokenizationDuration(duration time.Duration) {
	if s != nil {
		s.tokenizationNanos.Add(int64(duration))
	}
}

func (s *Stats) RecordAggregationDuration(duration time.Duration) {
	if s != nil {
		s.aggregationNanos.Add(int64(duration))
	}
}

func (s *Stats) RecordPersistenceReadyDuration(duration time.Duration) {
	if s != nil {
		s.persistenceNanos.Add(int64(duration))
	}
}

func (s *Stats) RecordTokenizedFile(method string) {
	if s == nil {
		return
	}
	s.tokenizedMu.Lock()
	if s.tokenizedByMethod == nil {
		s.tokenizedByMethod = make(map[string]int64)
	}
	s.tokenizedByMethod[method]++
	s.tokenizedMu.Unlock()
}

func (s *Stats) RecordCacheHit(reason string, methods int) {
	if s == nil {
		return
	}
	s.cacheHits.Add(1)
	if methods > 0 {
		s.cacheMethodsAvoided.Add(int64(methods))
	}
	s.recordCacheReason(reason)
}

func (s *Stats) RecordCachePartialHit(reason string, reusableMethods, missingMethods int) {
	if s == nil {
		return
	}
	s.cachePartialHits.Add(1)
	if reusableMethods > 0 {
		s.cacheMethodsAvoided.Add(int64(reusableMethods))
	}
	s.recordCacheReason(reason)
	if missingMethods > 0 {
		s.cacheMisses.Add(1)
	}
}

func (s *Stats) RecordCacheMiss(reason string) {
	if s == nil {
		return
	}
	s.cacheMisses.Add(1)
	s.recordCacheReason(reason)
}

func (s *Stats) RecordCacheWarning() {
	if s != nil {
		s.cacheWarnings.Add(1)
	}
}

func (s *Stats) recordCacheReason(reason string) {
	if s == nil || reason == "" {
		return
	}
	s.cacheReasonsMu.Lock()
	if s.cacheReasons == nil {
		s.cacheReasons = make(map[string]int64)
	}
	s.cacheReasons[reason]++
	s.cacheReasonsMu.Unlock()
}

func (s *Stats) startTokenization() func() {
	if s == nil {
		return nil
	}

	started := time.Now()
	stopped := false
	return func() {
		if stopped {
			return
		}
		s.RecordTokenizationDuration(time.Since(started))
		stopped = true
	}
}

func (s *Stats) ObserveMemory() {
	if s == nil {
		return
	}

	var memory runtime.MemStats
	runtime.ReadMemStats(&memory)
	for {
		current := s.peakHeapAlloc.Load()
		if memory.HeapAlloc <= current || s.peakHeapAlloc.CompareAndSwap(current, memory.HeapAlloc) {
			return
		}
	}
}

var _ fileops.WalkStatsCollector = (*Stats)(nil)
