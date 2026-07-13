package fileops

import "time"

// BinaryStatsCollector receives optional instrumentation from binary detection.
// It is deliberately small so the normal path can pass nil without creating a
// dependency on the tokenizer package.
type BinaryStatsCollector interface {
	RecordBinarySniffOpen()
	RecordBinarySniffBytes(int64)
	RecordValidationReadDuration(time.Duration)
}

// WalkStatsCollector receives optional instrumentation from directory walking.
type WalkStatsCollector interface {
	BinaryStatsCollector
	RecordEntryVisited()
	RecordEligibleFile()
	RecordWalkDuration(time.Duration)
}
