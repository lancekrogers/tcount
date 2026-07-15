package cache

import (
	"fmt"
	"sort"
)

// InvalidationRequest contains the current cache compatibility inputs. Root
// membership is supplied separately because the current walk is authoritative.
type InvalidationRequest struct {
	Root          string
	SchemaVersion uint32
	Mode          ValidationMode
	Contracts     []ContractKey
}

// DecisionKind describes how one current path should be scheduled.
type DecisionKind string

const (
	DecisionHit        DecisionKind = "hit"
	DecisionPartialHit DecisionKind = "partial_hit"
	DecisionMiss       DecisionKind = "miss"
	DecisionStale      DecisionKind = "stale"
)

// InvalidationReason is a structured metric/diagnostic reason, not user-facing
// prose. New reasons should be added when a new invalidation input is added.
type InvalidationReason string

const (
	ReasonEntryMissing          InvalidationReason = "entry_missing"
	ReasonSchemaMismatch        InvalidationReason = "schema_mismatch"
	ReasonRootMismatch          InvalidationReason = "root_mismatch"
	ReasonPathChanged           InvalidationReason = "path_changed"
	ReasonSizeChanged           InvalidationReason = "size_changed"
	ReasonModTimeChanged        InvalidationReason = "modtime_changed"
	ReasonContentChanged        InvalidationReason = "content_changed"
	ReasonClassificationChanged InvalidationReason = "classification_changed"
	ReasonContractMissing       InvalidationReason = "contract_missing"
	ReasonIdentityMatch         InvalidationReason = "identity_match"
	ReasonMetadataAssumed       InvalidationReason = "metadata_assumed"
	ReasonVerifiedMatch         InvalidationReason = "verified_match"
	ReasonStaleEntry            InvalidationReason = "stale_entry"
)

// FileDecision is the complete per-path scheduling decision. ReusableMethods
// are safe to aggregate; MissingMethods must be counted before aggregation.
// Stale decisions never represent current membership and must not aggregate.
type FileDecision struct {
	Path            string
	Kind            DecisionKind
	Reason          InvalidationReason
	ReusableMethods []ContractKey
	MissingMethods  []ContractKey
}

// InvalidationPlan is deterministic by path and contains one decision for
// every current path plus stale entries from a compatible prior snapshot.
type InvalidationPlan struct {
	Decisions []FileDecision
}

// PlanInvalidation compares the authoritative current membership and
// observations with a cached snapshot. A nil snapshot is a cold cache. The
// function performs no I/O and never allows a stale entry to become current.
func PlanInvalidation(request InvalidationRequest, current map[string]FileIdentity, snapshot *Snapshot) (InvalidationPlan, error) {
	contracts, err := normalizedContracts(request.Contracts)
	if err != nil {
		return InvalidationPlan{}, err
	}
	if err := validateRequest(request); err != nil {
		return InvalidationPlan{}, err
	}
	for path, identity := range current {
		if err := validateCurrentIdentity(path, identity); err != nil {
			return InvalidationPlan{}, err
		}
	}

	compatible := snapshot != nil && snapshot.SchemaVersion == request.SchemaVersion && snapshot.Root == request.Root
	if compatible {
		if err := validateManifest(*snapshot); err != nil {
			return InvalidationPlan{}, fmt.Errorf("%w: cached snapshot: %v", ErrInvalidationRequest, err)
		}
	}

	paths := make([]string, 0, len(current))
	for path := range current {
		paths = append(paths, path)
	}
	sort.Strings(paths)
	decisions := make([]FileDecision, 0, len(current)+snapshotEntryCount(snapshot, compatible))
	for _, path := range paths {
		identity := current[path]
		if !compatible {
			reason := ReasonSchemaMismatch
			if snapshot != nil && snapshot.Root != request.Root {
				reason = ReasonRootMismatch
			}
			decisions = append(decisions, missDecision(path, reason, contracts))
			continue
		}
		cached, exists := snapshot.Entries[path]
		if !exists {
			decisions = append(decisions, missDecision(path, ReasonEntryMissing, contracts))
			continue
		}
		decisions = append(decisions, decideCurrentFile(request.Mode, path, identity, cached, contracts))
	}

	if compatible {
		stalePaths := make([]string, 0, len(snapshot.Entries))
		for path := range snapshot.Entries {
			if _, exists := current[path]; !exists {
				stalePaths = append(stalePaths, path)
			}
		}
		sort.Strings(stalePaths)
		for _, path := range stalePaths {
			decisions = append(decisions, FileDecision{Path: path, Kind: DecisionStale, Reason: ReasonStaleEntry})
		}
	}

	return InvalidationPlan{Decisions: decisions}, nil
}

func validateRequest(request InvalidationRequest) error {
	if request.Root == "" {
		return fmt.Errorf("%w: root is empty", ErrInvalidationRequest)
	}
	if request.SchemaVersion == 0 {
		return fmt.Errorf("%w: schema version is zero", ErrInvalidationRequest)
	}
	if request.Mode != Metadata && request.Mode != Verified {
		return fmt.Errorf("%w: unsupported validation mode %d", ErrInvalidationRequest, request.Mode)
	}
	return nil
}

func normalizedContracts(contracts []ContractKey) ([]ContractKey, error) {
	if len(contracts) > int(MaxManifestMethods) {
		return nil, fmt.Errorf("%w: requested contract count exceeds %d", ErrInvalidationRequest, MaxManifestMethods)
	}
	seen := make(map[ContractKey]struct{}, len(contracts))
	ordered := make([]ContractKey, 0, len(contracts))
	for _, contract := range contracts {
		if err := validateContractKey(contract); err != nil {
			return nil, fmt.Errorf("%w: contract: %v", ErrInvalidationRequest, err)
		}
		if _, exists := seen[contract]; exists {
			continue
		}
		seen[contract] = struct{}{}
		ordered = append(ordered, contract)
	}
	sort.Slice(ordered, func(i, j int) bool { return contractKeyLess(ordered[i], ordered[j]) })
	return ordered, nil
}

func validateContractKey(contract ContractKey) error {
	for _, value := range []string{contract.Method, contract.Encoding, contract.Implementation, contract.VocabularyDigest, contract.NormalizationPolicy, contract.SpecialTokenPolicy} {
		if uint64(len(value)) > uint64(MaxManifestString) {
			return fmt.Errorf("contract field exceeds %d bytes", MaxManifestString)
		}
	}
	return nil
}

func validateCurrentIdentity(path string, identity FileIdentity) error {
	if err := validatePath(path); err != nil {
		return fmt.Errorf("%w: path %q: %v", ErrInvalidationRequest, path, err)
	}
	if identity.RelativePath != path {
		return fmt.Errorf("%w: identity path %q does not match map path %q", ErrInvalidationRequest, identity.RelativePath, path)
	}
	if identity.Size < 0 {
		return fmt.Errorf("%w: path %q has negative size", ErrInvalidationRequest, path)
	}
	if identity.Classification != ClassificationText && identity.Classification != ClassificationBinary {
		return fmt.Errorf("%w: path %q has invalid classification %d", ErrInvalidationRequest, path, identity.Classification)
	}
	return nil
}

func decideCurrentFile(mode ValidationMode, path string, current FileIdentity, cached FileResult, contracts []ContractKey) FileDecision {
	if current.RelativePath != path {
		return missDecision(path, ReasonPathChanged, contracts)
	}
	if current.Classification != cached.Classification {
		return missDecision(path, ReasonClassificationChanged, contracts)
	}
	if current.Size != cached.Size {
		return missDecision(path, ReasonSizeChanged, contracts)
	}
	if current.ModTimeNS != cached.ModTimeNS {
		return missDecision(path, ReasonModTimeChanged, contracts)
	}
	if mode == Verified && current.ContentDigest != cached.ContentDigest {
		return missDecision(path, ReasonContentChanged, contracts)
	}

	reusable, missing := splitContracts(cached.Methods, contracts)
	if len(missing) == 0 {
		reason := ReasonVerifiedMatch
		if mode == Metadata {
			reason = ReasonMetadataAssumed
		}
		return FileDecision{Path: path, Kind: DecisionHit, Reason: reason, ReusableMethods: reusable}
	}
	if len(reusable) == 0 {
		return FileDecision{Path: path, Kind: DecisionMiss, Reason: ReasonContractMissing, MissingMethods: missing}
	}
	return FileDecision{Path: path, Kind: DecisionPartialHit, Reason: ReasonContractMissing, ReusableMethods: reusable, MissingMethods: missing}
}

func missDecision(path string, reason InvalidationReason, contracts []ContractKey) FileDecision {
	return FileDecision{Path: path, Kind: DecisionMiss, Reason: reason, MissingMethods: append([]ContractKey(nil), contracts...)}
}

func splitContracts(methods map[ContractKey]int, requested []ContractKey) ([]ContractKey, []ContractKey) {
	reusable := make([]ContractKey, 0, len(requested))
	missing := make([]ContractKey, 0, len(requested))
	for _, contract := range requested {
		if _, exists := methods[contract]; exists {
			reusable = append(reusable, contract)
		} else {
			missing = append(missing, contract)
		}
	}
	return reusable, missing
}

func snapshotEntryCount(snapshot *Snapshot, compatible bool) int {
	if snapshot == nil || !compatible {
		return 0
	}
	return len(snapshot.Entries)
}
