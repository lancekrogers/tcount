package cache

import (
	"context"
	"errors"
	"io/fs"
	"time"
)

// FileClassification records the classification associated with the stored bytes.
type FileClassification uint8

const (
	ClassificationText FileClassification = iota + 1
	ClassificationBinary
)

// ContractKey identifies the exact tokenizer contract represented by a method
// value. Model aliases intentionally do not appear here when they resolve to
// the same tokenizer contract.
type ContractKey struct {
	Method              string
	Encoding            string
	Implementation      string
	VocabularyDigest    string
	NormalizationPolicy string
	SpecialTokenPolicy  string
}

// FileIdentity is the observed identity that must still match before a stored
// file result can be reused. RelativePath is slash-normalized and relative to
// the canonical root; callers own canonical-root normalization.
type FileIdentity struct {
	RelativePath   string
	Size           int64
	ModTimeNS      int64
	ContentDigest  [32]byte
	Classification FileClassification
}

// FileResult contains the reducible per-file values. Characters, words, and
// lines support approximation methods without rereading the file; Methods can
// contain only the contracts computed by one run so later updates can be
// merged without discarding reusable method values.
type FileResult struct {
	Size           int64
	ModTimeNS      int64
	ContentDigest  [32]byte
	Classification FileClassification
	Characters     int
	Words          int
	Lines          int
	Methods        map[ContractKey]int
}

// Identity returns the identity represented by a stored result at path.
func (result FileResult) Identity(path string) FileIdentity {
	return FileIdentity{
		RelativePath:   path,
		Size:           result.Size,
		ModTimeNS:      result.ModTimeNS,
		ContentDigest:  result.ContentDigest,
		Classification: result.Classification,
	}
}

// Snapshot is a complete generation for one canonical root. The current
// directory walk remains authoritative; entries absent from that walk never
// contribute to an aggregate even if they remain in this snapshot.
type Snapshot struct {
	SchemaVersion uint32
	Root          string
	Generation    uint64
	Entries       map[string]FileResult
}

// UpdateSet contains per-file results. Methods may be a partial set when a
// run requests an additional tokenizer contract.
type UpdateSet map[string]FileResult

// These aliases keep the Sequence 01 manifest prototype source-compatible
// while the domain names above become the persistence-independent contract.
type FileEntry = FileResult
type Manifest = Snapshot

// Status is the non-persistent store status for one canonical root.
type Status struct {
	Root          string
	Present       bool
	Failure       CacheFailureKind
	SchemaVersion uint32
	Generation    uint64
	Entries       int
	Bytes         int64
	ModifiedAt    time.Time
	Age           time.Duration
}

// CacheFailureKind is the diagnostic class for a cache operation. None of
// these failures should replace a correctly computed count with an error.
type CacheFailureKind string

const (
	FailureNone         CacheFailureKind = ""
	FailureAbsent       CacheFailureKind = "absent"
	FailureCorrupt      CacheFailureKind = "corrupt"
	FailureIncompatible CacheFailureKind = "incompatible"
	FailurePermission   CacheFailureKind = "permission"
	FailureLock         CacheFailureKind = "lock"
	FailurePersistence  CacheFailureKind = "persistence"
)

var (
	ErrCacheAbsent       = errors.New("cache state absent")
	ErrCacheCorrupt      = errors.New("cache state corrupt")
	ErrCacheIncompatible = errors.New("cache state incompatible")
	ErrCachePermission   = errors.New("cache permission failure")
	ErrCacheLock         = errors.New("cache lock failure")
	ErrCachePersistence  = errors.New("cache persistence failure")
	ErrPruneNotApproved  = errors.New("cache prune requires a successful full walk")
)

// CacheError preserves the underlying error while exposing a stable
// diagnostic category to callers and future CLI reporting.
type CacheError struct {
	Category  CacheFailureKind
	Operation string
	Path      string
	Err       error
}

func (err *CacheError) Error() string {
	if err == nil {
		return "<nil>"
	}
	return string(err.Category) + " cache " + err.Operation + " at " + err.Path + ": " + err.Err.Error()
}

func (err *CacheError) Unwrap() error { return err.Err }

func (err *CacheError) Is(target error) bool {
	if target == failureSentinel(err.Category) {
		return true
	}
	return errors.Is(err.Err, target)
}

// CacheFailureOf extracts a stable diagnostic class without requiring callers
// to depend on the concrete persistence error type.
func CacheFailureOf(err error) CacheFailureKind {
	var cacheErr *CacheError
	if errors.As(err, &cacheErr) {
		return cacheErr.Category
	}
	return FailureNone
}

func failureSentinel(category CacheFailureKind) error {
	switch category {
	case FailureAbsent:
		return ErrCacheAbsent
	case FailureCorrupt:
		return ErrCacheCorrupt
	case FailureIncompatible:
		return ErrCacheIncompatible
	case FailurePermission:
		return ErrCachePermission
	case FailureLock:
		return ErrCacheLock
	case FailurePersistence:
		return ErrCachePersistence
	default:
		return nil
	}
}

func classifyCacheFailure(operation, path string, err error) error {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var cacheErr *CacheError
	if errors.As(err, &cacheErr) {
		return err
	}
	category := FailurePersistence
	switch {
	case errors.Is(err, fs.ErrNotExist), errors.Is(err, ErrSnapshotNotFound):
		category = FailureAbsent
	case errors.Is(err, ErrCacheIncompatible):
		category = FailureIncompatible
	case errors.Is(err, ErrCacheCorrupt), errors.Is(err, ErrLocationCollision):
		category = FailureCorrupt
	case errors.Is(err, fs.ErrPermission):
		category = FailurePermission
	case errors.Is(err, ErrLockUnsupported):
		category = FailureLock
	}
	return &CacheError{Category: category, Operation: operation, Path: path, Err: err}
}

func classifyLockFailure(operation, path string, err error) error {
	if err == nil || errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	var cacheErr *CacheError
	if errors.As(err, &cacheErr) {
		return err
	}
	if errors.Is(err, fs.ErrPermission) || errors.Is(err, ErrLockUnsupported) {
		return classifyCacheFailure(operation, path, err)
	}
	return &CacheError{Category: FailureLock, Operation: operation, Path: path, Err: err}
}

// Store is the narrow persistence boundary used by counting code. A cache
// failure must never replace a correctly computed count with an error; callers
// may treat load or commit-and-prune failures as cold-path or diagnostic
// conditions while explicit management operations can surface them directly.
type Store interface {
	Load(context.Context, string) (*Snapshot, error)
	Commit(context.Context, string, uint64, UpdateSet) error
	CommitAndPrune(context.Context, string, uint64, UpdateSet, PruneOptions) error
	Status(context.Context, string) (Status, error)
	Clear(context.Context, string) error
}

// Stable error categories let the count path distinguish a cold cache from a
// generation conflict or invalid caller input without depending on storage.
var (
	ErrInvalidRoot         = errors.New("invalid canonical root")
	ErrSnapshotNotFound    = errors.New("cache snapshot not found")
	ErrGenerationConflict  = errors.New("cache generation conflict")
	ErrInvalidSnapshot     = errors.New("invalid cache snapshot")
	ErrCacheUnavailable    = errors.New("cache unavailable")
	ErrInvalidationRequest = errors.New("invalid invalidation request")
	ErrAggregateOverflow   = errors.New("aggregate exceeds numeric limits")
	ErrInvalidAggregate    = errors.New("invalid aggregate input")
	ErrLocationCollision   = errors.New("cache location root mismatch")
	ErrLockUnsupported     = errors.New("cache writer lock unsupported on this platform")
)

func validateCanonicalRoot(root string) error {
	if root == "" {
		return ErrInvalidRoot
	}
	return nil
}

func cloneFileResult(result FileResult) FileResult {
	if result.Methods == nil {
		return result
	}
	methods := make(map[ContractKey]int, len(result.Methods))
	for key, tokens := range result.Methods {
		methods[key] = tokens
	}
	result.Methods = methods
	return result
}

func cloneSnapshot(snapshot Snapshot) Snapshot {
	clone := snapshot
	clone.Entries = make(map[string]FileResult, len(snapshot.Entries))
	for path, result := range snapshot.Entries {
		clone.Entries[path] = cloneFileResult(result)
	}
	return clone
}

func sameFileIdentity(left, right FileResult) bool {
	return left.Size == right.Size &&
		left.ModTimeNS == right.ModTimeNS &&
		left.ContentDigest == right.ContentDigest &&
		left.Classification == right.Classification
}

func mergeFileResult(base, update FileResult) FileResult {
	merged := cloneFileResult(update)
	if !sameFileIdentity(base, update) {
		return merged
	}
	methods := make(map[ContractKey]int, len(base.Methods)+len(update.Methods))
	for key, tokens := range base.Methods {
		methods[key] = tokens
	}
	for key, tokens := range update.Methods {
		methods[key] = tokens
	}
	merged.Methods = methods
	return merged
}

// AggregateResult contains only values reducible across file boundaries.
// Identity fields intentionally stay per-file so directory membership and
// validation cannot be bypassed by a stored total.
type AggregateResult struct {
	FileCount  int
	FileSize   int64
	Characters int
	Words      int
	Lines      int
	Methods    map[ContractKey]int
}

// AggregateFileResults proves that fresh and reusable per-file values use the
// same summation semantics. It performs no membership or cache validation.
func AggregateFileResults(results []FileResult) (AggregateResult, error) {
	aggregate := AggregateResult{Methods: make(map[ContractKey]int, len(results))}
	for _, result := range results {
		if err := validateFileEntry(result); err != nil {
			return AggregateResult{}, ErrInvalidAggregate
		}
		if result.Size < 0 || result.Characters < 0 || result.Words < 0 || result.Lines < 0 {
			return AggregateResult{}, ErrAggregateOverflow
		}
		var err error
		aggregate.FileSize, err = addInt64(aggregate.FileSize, result.Size)
		if err != nil {
			return AggregateResult{}, err
		}
		aggregate.Characters, err = addInt(aggregate.Characters, result.Characters)
		if err != nil {
			return AggregateResult{}, err
		}
		aggregate.Words, err = addInt(aggregate.Words, result.Words)
		if err != nil {
			return AggregateResult{}, err
		}
		aggregate.Lines, err = addInt(aggregate.Lines, result.Lines)
		if err != nil {
			return AggregateResult{}, err
		}
		for contract, tokens := range result.Methods {
			if tokens < 0 {
				return AggregateResult{}, ErrAggregateOverflow
			}
			current, err := addInt(aggregate.Methods[contract], tokens)
			if err != nil {
				return AggregateResult{}, err
			}
			aggregate.Methods[contract] = current
		}
		aggregate.FileCount++
	}
	return aggregate, nil
}

func addInt64(left, right int64) (int64, error) {
	if right > 0 && left > int64(^uint64(0)>>1)-right {
		return 0, ErrAggregateOverflow
	}
	return left + right, nil
}

func addInt(left, right int) (int, error) {
	maxInt := int(^uint(0) >> 1)
	if right > 0 && left > maxInt-right {
		return 0, ErrAggregateOverflow
	}
	return left + right, nil
}
