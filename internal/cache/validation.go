// Package cache contains the experimental validation contracts used to measure
// directory-cache identity before the production store is selected.
package cache

import (
	"context"
	"crypto/sha256"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"time"
)

// ValidationMode controls how an existing entry is considered reusable.
type ValidationMode uint8

const (
	// Metadata accepts a hit when normalized path, size, and nanosecond mtime match.
	Metadata ValidationMode = iota + 1
	// Verified reads and hashes the file, accepting a hit only when its digest matches.
	Verified
)

func (m ValidationMode) String() string {
	switch m {
	case Metadata:
		return "metadata"
	case Verified:
		return "verified"
	default:
		return "unknown"
	}
}

// FileObservation is the filesystem identity stored with a cache entry.
type FileObservation struct {
	RelativePath string
	Size         int64
	ModTimeNS    int64
}

// Entry is the minimal per-file value needed by the validation prototype.
type Entry struct {
	Observation FileObservation
	Digest      [sha256.Size]byte
}

// ValidationResult reports whether an entry can be reused and the work needed
// to reach that decision.
type ValidationResult struct {
	Hit            bool
	Observation    FileObservation
	Digest         [sha256.Size]byte
	BytesRead      int64
	DigestDuration time.Duration
}

// ValidationStats collects work measurements for one validation run.
type ValidationStats struct {
	filesChecked atomic.Int64
	hits         atomic.Int64
	misses       atomic.Int64
	fullReads    atomic.Int64
	bytesRead    atomic.Int64
	digestNanos  atomic.Int64
}

// ValidationStatsSnapshot is an immutable view of ValidationStats.
type ValidationStatsSnapshot struct {
	FilesChecked   int64
	Hits           int64
	Misses         int64
	FullReads      int64
	BytesRead      int64
	DigestDuration time.Duration
}

// Snapshot returns a race-free copy of the current measurements.
func (s *ValidationStats) Snapshot() ValidationStatsSnapshot {
	if s == nil {
		return ValidationStatsSnapshot{}
	}
	return ValidationStatsSnapshot{
		FilesChecked:   s.filesChecked.Load(),
		Hits:           s.hits.Load(),
		Misses:         s.misses.Load(),
		FullReads:      s.fullReads.Load(),
		BytesRead:      s.bytesRead.Load(),
		DigestDuration: time.Duration(s.digestNanos.Load()),
	}
}

// Validator validates entries without changing the production count path.
type Validator struct {
	mode ValidationMode
}

// NewValidator creates a validator for an experimental validation mode.
func NewValidator(mode ValidationMode) (Validator, error) {
	if mode != Metadata && mode != Verified {
		return Validator{}, fmt.Errorf("unsupported validation mode %d", mode)
	}
	return Validator{mode: mode}, nil
}

// CaptureEntry reads a file once and records its current observation and digest.
func CaptureEntry(ctx context.Context, root, path string) (Entry, error) {
	if err := ctx.Err(); err != nil {
		return Entry{}, err
	}
	observation, absolute, err := observeFile(root, path)
	if err != nil {
		return Entry{}, err
	}
	content, err := os.ReadFile(absolute)
	if err != nil {
		return Entry{}, fmt.Errorf("reading %q: %w", path, err)
	}
	if err := ctx.Err(); err != nil {
		return Entry{}, err
	}
	return Entry{Observation: observation, Digest: sha256.Sum256(content)}, nil
}

// Validate checks one current file against a previously captured entry.
// Metadata mode intentionally permits timestamp-preserving false hits; only
// Verified mode provides content identity for that adversarial case.
func (v Validator) Validate(ctx context.Context, root, path string, entry Entry, stats *ValidationStats) (ValidationResult, error) {
	if err := ctx.Err(); err != nil {
		return ValidationResult{}, err
	}
	if stats != nil {
		stats.filesChecked.Add(1)
	}

	observation, absolute, err := observeFile(root, path)
	if err != nil {
		return ValidationResult{}, err
	}
	if observation.RelativePath != entry.Observation.RelativePath || observation.Size != entry.Observation.Size {
		return v.miss(stats, observation, entry.Digest), nil
	}

	if v.mode == Metadata {
		if observation.ModTimeNS == entry.Observation.ModTimeNS {
			if stats != nil {
				stats.hits.Add(1)
			}
			return ValidationResult{Hit: true, Observation: observation, Digest: entry.Digest}, nil
		}
		return v.miss(stats, observation, entry.Digest), nil
	}

	readStarted := time.Now()
	content, err := os.ReadFile(absolute)
	if err != nil {
		return ValidationResult{}, fmt.Errorf("reading %q for verification: %w", path, err)
	}
	digest := sha256.Sum256(content)
	readDuration := time.Since(readStarted)
	if stats != nil {
		stats.fullReads.Add(1)
		stats.bytesRead.Add(int64(len(content)))
		stats.digestNanos.Add(int64(readDuration))
	}
	if err := ctx.Err(); err != nil {
		return ValidationResult{}, err
	}
	if digest == entry.Digest {
		if stats != nil {
			stats.hits.Add(1)
		}
		return ValidationResult{Hit: true, Observation: observation, Digest: digest, BytesRead: int64(len(content)), DigestDuration: readDuration}, nil
	}
	return v.missWithDigest(stats, observation, digest, int64(len(content)), readDuration), nil
}

func (v Validator) miss(stats *ValidationStats, observation FileObservation, digest [sha256.Size]byte) ValidationResult {
	if stats != nil {
		stats.misses.Add(1)
	}
	return ValidationResult{Observation: observation, Digest: digest}
}

func (v Validator) missWithDigest(stats *ValidationStats, observation FileObservation, digest [sha256.Size]byte, bytesRead int64, digestDuration time.Duration) ValidationResult {
	if stats != nil {
		stats.misses.Add(1)
	}
	return ValidationResult{Observation: observation, Digest: digest, BytesRead: bytesRead, DigestDuration: digestDuration}
}

func observeFile(root, path string) (FileObservation, string, error) {
	rootAbsolute, err := filepath.Abs(root)
	if err != nil {
		return FileObservation{}, "", fmt.Errorf("resolving root %q: %w", root, err)
	}
	absolute, err := filepath.Abs(path)
	if err != nil {
		return FileObservation{}, "", fmt.Errorf("resolving path %q: %w", path, err)
	}
	relative, err := filepath.Rel(rootAbsolute, absolute)
	if err != nil {
		return FileObservation{}, "", fmt.Errorf("relating path %q to root %q: %w", path, root, err)
	}
	relative = filepath.ToSlash(filepath.Clean(relative))
	if relative == "." || relative == ".." || strings.HasPrefix(relative, "../") {
		return FileObservation{}, "", errors.New("path is outside validation root")
	}
	info, err := os.Stat(absolute)
	if err != nil {
		return FileObservation{}, "", fmt.Errorf("stating %q: %w", path, err)
	}
	if info.IsDir() {
		return FileObservation{}, "", fmt.Errorf("path %q is a directory", path)
	}
	return FileObservation{
		RelativePath: relative,
		Size:         info.Size(),
		ModTimeNS:    info.ModTime().UnixNano(),
	}, absolute, nil
}
