package tokenizer

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/lancekrogers/tcount/internal/cache"
	"github.com/lancekrogers/tcount/tokenizer/fileops"
)

// CountFilesWithCache counts the current directory membership while reusing
// valid per-file values from store. The files argument must come from the
// caller's successful current walk; the manifest never supplies membership.
// The cache is deliberately an explicit caller choice; CountFiles remains the
// cold-path oracle.
func (c *Counter) CountFilesWithCache(ctx context.Context, root string, files []string, model string, all bool, store cache.Store, mode cache.ValidationMode) (*CountResult, error) {
	if store == nil {
		return c.CountFiles(ctx, files, model, all)
	}
	if err := validateCacheRequest(ctx, files, mode); err != nil {
		return nil, err
	}
	plans, includeApprox, err := c.planMethods(model, all)
	if err != nil {
		return nil, fmt.Errorf("counting tokens for model %q: %w", model, err)
	}
	allMode := all || model == ""
	contracts, contractIndexes, cacheable := cacheContracts(plans)
	if !cacheable {
		return c.CountFiles(ctx, files, model, all)
	}
	state, err := c.prepareCacheCount(ctx, root, files, contracts, mode, store)
	if err != nil {
		c.stats.RecordCacheWarning()
		return c.CountFiles(ctx, files, model, all)
	}
	missPaths, selected := c.scheduleCacheMisses(state, contractIndexes, len(files))
	freshResults, err := c.countFileResults(ctx, missPaths, plans, allMode, selected, state.validated)
	if err != nil {
		return nil, err
	}
	results, updates, err := materializeCacheResults(state, plans, contracts, missPaths, freshResults)
	if err != nil {
		c.stats.RecordCacheWarning()
		return c.CountFiles(ctx, files, model, all)
	}
	result, err := c.aggregateFileResults(ctx, results, plans, includeApprox)
	if err != nil {
		return nil, err
	}
	if err := c.commitCacheUpdates(ctx, store, state, updates); err != nil {
		return nil, err
	}
	return result, nil
}

type cacheCountState struct {
	root          string
	pathRoot      string
	snapshot      *cache.Snapshot
	plan          cache.InvalidationPlan
	current       map[string]cache.FileIdentity
	absolutePaths map[string]string
	validated     map[string]cacheValidatedFile
}

type cacheValidatedFile struct {
	content     []byte
	digest      [sha256.Size]byte
	digestKnown bool
}

func validateCacheRequest(ctx context.Context, files []string, mode cache.ValidationMode) error {
	if err := ctx.Err(); err != nil {
		return err
	}
	if len(files) == 0 {
		return fmt.Errorf("no files to count")
	}
	if mode != cache.Metadata && mode != cache.Verified {
		return fmt.Errorf("unsupported cache validation mode %d", mode)
	}
	return nil
}

func (c *Counter) prepareCacheCount(ctx context.Context, root string, files []string, contracts []cache.ContractKey, mode cache.ValidationMode, store cache.Store) (*cacheCountState, error) {
	canonicalRoot, err := cache.CanonicalRoot(root)
	if err != nil {
		return nil, err
	}
	snapshot, err := c.loadCacheSnapshot(ctx, store, canonicalRoot)
	if err != nil {
		return nil, err
	}
	current, absolutePaths, validated, err := c.currentCacheIdentities(ctx, root, canonicalRoot, files, contracts, mode, snapshot)
	if err != nil {
		return nil, err
	}
	plan, err := cache.PlanInvalidation(cache.InvalidationRequest{
		Root:          canonicalRoot,
		SchemaVersion: cache.CurrentSchemaVersion,
		Mode:          mode,
		Contracts:     contracts,
	}, current, snapshot)
	if err != nil {
		return nil, err
	}
	return &cacheCountState{root: canonicalRoot, pathRoot: root, snapshot: snapshot, plan: plan, current: current, absolutePaths: absolutePaths, validated: validated}, nil
}

func (c *Counter) loadCacheSnapshot(ctx context.Context, store cache.Store, root string) (*cache.Snapshot, error) {
	snapshot, err := store.Load(ctx, root)
	if err == nil {
		return snapshot, nil
	}
	if contextErr := ctx.Err(); contextErr != nil {
		return nil, contextErr
	}
	if !errors.Is(err, cache.ErrSnapshotNotFound) && !errors.Is(err, cache.ErrCacheAbsent) {
		c.stats.RecordCacheWarning()
	}
	return nil, nil
}

func (c *Counter) scheduleCacheMisses(state *cacheCountState, indexes map[cache.ContractKey]int, capacity int) ([]string, map[string][]int) {
	missPaths := make([]string, 0, capacity)
	selected := make(map[string][]int)
	for _, decision := range state.plan.Decisions {
		if decision.Kind == cache.DecisionStale {
			continue
		}
		if decision.Kind == cache.DecisionHit {
			c.stats.RecordCacheHit(string(decision.Reason), len(decision.ReusableMethods))
			c.stats.RecordCacheBytesReused(state.current[decision.Path].Size)
			continue
		}
		if decision.Kind == cache.DecisionPartialHit {
			c.stats.RecordCachePartialHit(string(decision.Reason), len(decision.ReusableMethods), len(decision.MissingMethods))
			c.stats.RecordCacheBytesReused(state.current[decision.Path].Size)
		} else {
			c.stats.RecordCacheMiss(string(decision.Reason))
		}
		indices := make([]int, 0, len(decision.MissingMethods))
		for _, contract := range decision.MissingMethods {
			indices = append(indices, indexes[contract])
		}
		path := state.absolutePaths[decision.Path]
		selected[path] = indices
		missPaths = append(missPaths, path)
	}
	return missPaths, selected
}

func materializeCacheResults(state *cacheCountState, plans []methodPlan, contracts []cache.ContractKey, missPaths []string, freshResults []perFileResult) ([]perFileResult, cache.UpdateSet, error) {
	freshByPath := make(map[string]perFileResult, len(freshResults))
	for index, path := range missPaths {
		relative, err := cacheRelativePath(state.pathRoot, path)
		if err != nil {
			return nil, nil, err
		}
		freshByPath[relative] = freshResults[index]
	}
	results := make([]perFileResult, 0, len(state.current))
	updates := make(cache.UpdateSet, len(missPaths))
	for _, decision := range state.plan.Decisions {
		if decision.Kind == cache.DecisionStale {
			continue
		}
		if decision.Kind == cache.DecisionHit {
			result, err := cacheEntryToPerFileResult(state.snapshot.Entries[decision.Path], plans, contracts, decision.ReusableMethods)
			if err != nil {
				return nil, nil, err
			}
			results = append(results, result)
			continue
		}
		result := freshByPath[decision.Path]
		if len(decision.ReusableMethods) > 0 && state.snapshot != nil {
			cached, err := cacheEntryToPerFileResult(state.snapshot.Entries[decision.Path], plans, contracts, decision.ReusableMethods)
			if err != nil {
				return nil, nil, err
			}
			copyReusableMethods(&result, &cached)
		}
		updates[decision.Path] = perFileResultToCacheResult(result, state.current[decision.Path], contracts)
		results = append(results, result)
	}
	return results, updates, nil
}

func copyReusableMethods(destination, source *perFileResult) {
	for index, present := range source.MethodPresent {
		if present {
			destination.Methods[index] = source.Methods[index]
			destination.MethodPresent[index] = true
		}
	}
}

func (c *Counter) commitCacheUpdates(ctx context.Context, store cache.Store, state *cacheCountState, updates cache.UpdateSet) error {
	if len(updates) == 0 {
		return ctx.Err()
	}
	if err := ctx.Err(); err != nil {
		return err
	}
	baseGeneration := uint64(0)
	if state.snapshot != nil {
		baseGeneration = state.snapshot.Generation
	}
	if err := store.Commit(ctx, state.root, baseGeneration, updates); err != nil {
		if contextErr := ctx.Err(); contextErr != nil {
			return contextErr
		}
		if !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			c.stats.RecordCacheWarning()
		}
		return nil
	}
	return nil
}

func cacheContracts(plans []methodPlan) ([]cache.ContractKey, map[cache.ContractKey]int, bool) {
	contracts := make([]cache.ContractKey, len(plans))
	indexes := make(map[cache.ContractKey]int, len(plans))
	for index, plan := range plans {
		identity, ok := ContractOf(plan.tok)
		if !ok {
			return nil, nil, false
		}
		contract := cacheContract(identity)
		contracts[index] = contract
		indexes[contract] = index
	}
	return contracts, indexes, true
}

func cacheContract(identity ContractIdentity) cache.ContractKey {
	digest := ""
	if identity.VocabularyDigest != ([sha256.Size]byte{}) {
		digest = hex.EncodeToString(identity.VocabularyDigest[:])
	}
	return cache.ContractKey{
		Method:              identity.Method,
		Encoding:            identity.Encoding,
		Implementation:      identity.Implementation,
		VocabularyDigest:    digest,
		NormalizationPolicy: identity.NormalizationPolicy,
		SpecialTokenPolicy:  identity.SpecialTokenPolicy,
	}
}

func (c *Counter) currentCacheIdentities(ctx context.Context, root, canonicalRoot string, files []string, contracts []cache.ContractKey, mode cache.ValidationMode, snapshot *cache.Snapshot) (map[string]cache.FileIdentity, map[string]string, map[string]cacheValidatedFile, error) {
	current := make(map[string]cache.FileIdentity, len(files))
	absolutePaths := make(map[string]string, len(files))
	validated := make(map[string]cacheValidatedFile)
	verifiedSnapshot := compatibleCacheSnapshot(snapshot, canonicalRoot)
	for _, path := range files {
		if err := ctx.Err(); err != nil {
			return nil, nil, nil, err
		}
		relative, err := cacheRelativePath(root, path)
		if err != nil {
			return nil, nil, nil, err
		}
		if _, exists := current[relative]; exists {
			return nil, nil, nil, fmt.Errorf("duplicate current file path %q", relative)
		}
		info, err := os.Stat(path)
		if err != nil {
			return nil, nil, nil, fmt.Errorf("stating file %q: %w", path, err)
		}
		identity := cache.FileIdentity{
			RelativePath:   relative,
			Size:           info.Size(),
			ModTimeNS:      info.ModTime().UnixNano(),
			Classification: cache.ClassificationText,
		}
		if mode == cache.Verified && verifiedSnapshot {
			if c.stats != nil {
				c.stats.RecordFullFileOpen()
			}
			var readStarted time.Time
			if c.stats != nil {
				readStarted = time.Now()
			}
			content, readErr := os.ReadFile(path)
			if c.stats != nil {
				c.stats.RecordValidationReadDuration(time.Since(readStarted))
				c.stats.RecordFullFileBytes(int64(len(content)))
				c.stats.ObserveMemory()
			}
			if readErr != nil {
				return nil, nil, nil, fmt.Errorf("reading file %q for cache verification: %w", path, readErr)
			}
			identity.ContentDigest = sha256.Sum256(content)
			if fileops.IsBinaryContent(path, content) {
				delete(validated, path)
				continue
			}
			if retainVerifiedContent(snapshot, relative, identity, contracts) {
				validated[path] = cacheValidatedFile{content: content, digest: identity.ContentDigest, digestKnown: true}
			}
		}
		current[relative] = identity
		absolutePaths[relative] = path
	}
	return current, absolutePaths, validated, nil
}

func compatibleCacheSnapshot(snapshot *cache.Snapshot, root string) bool {
	return snapshot != nil && snapshot.SchemaVersion == cache.CurrentSchemaVersion && snapshot.Root == root
}

func retainVerifiedContent(snapshot *cache.Snapshot, path string, identity cache.FileIdentity, contracts []cache.ContractKey) bool {
	if snapshot == nil {
		return false
	}
	cached, exists := snapshot.Entries[path]
	if !exists || cached.Size != identity.Size || cached.ModTimeNS != identity.ModTimeNS || cached.ContentDigest != identity.ContentDigest || cached.Classification != identity.Classification {
		return true
	}
	for _, contract := range contracts {
		if _, exists := cached.Methods[contract]; !exists {
			return true
		}
	}
	return false
}

func cacheRelativePath(root, path string) (string, error) {
	rootAbsolute, err := filepath.Abs(root)
	if err != nil {
		return "", fmt.Errorf("resolving cache root %q: %w", root, err)
	}
	pathAbsolute, err := filepath.Abs(path)
	if err != nil {
		return "", fmt.Errorf("resolving cache path %q: %w", path, err)
	}
	relative, err := filepath.Rel(rootAbsolute, pathAbsolute)
	if err != nil {
		return "", fmt.Errorf("relating cache path %q to root %q: %w", path, root, err)
	}
	relative = filepath.ToSlash(filepath.Clean(relative))
	if relative == "." || relative == ".." || strings.HasPrefix(relative, "../") {
		return "", fmt.Errorf("cache path %q is outside root %q", path, root)
	}
	return relative, nil
}

func cacheEntryToPerFileResult(entry cache.FileResult, plans []methodPlan, contracts []cache.ContractKey, reusable []cache.ContractKey) (perFileResult, error) {
	size, ok := int64ToInt(entry.Size)
	if !ok || entry.Characters < 0 || entry.Words < 0 || entry.Lines < 0 {
		return perFileResult{}, fmt.Errorf("cached file result has invalid numeric fields")
	}
	result := perFileResult{
		FileSize:       size,
		Characters:     entry.Characters,
		Words:          entry.Words,
		Lines:          entry.Lines,
		Classification: fileClassificationText,
		ContentDigest:  entry.ContentDigest,
		Methods:        make([]MethodResult, len(plans)),
		MethodPresent:  make([]bool, len(plans)),
	}
	indexes := make(map[cache.ContractKey]int, len(contracts))
	for index, contract := range contracts {
		indexes[contract] = index
	}
	for _, contract := range reusable {
		index, ok := indexes[contract]
		if !ok {
			continue
		}
		tokens, ok := entry.Methods[contract]
		if !ok || tokens < 0 {
			return perFileResult{}, fmt.Errorf("cached file result is missing reusable contract")
		}
		result.Methods[index] = MethodResult{
			Name:          plans[index].name,
			DisplayName:   plans[index].displayName,
			Tokens:        tokens,
			IsExact:       plans[index].isExact,
			ContextWindow: plans[index].contextWindow,
		}
		result.MethodPresent[index] = true
	}
	return result, nil
}

func perFileResultToCacheResult(result perFileResult, identity cache.FileIdentity, contracts []cache.ContractKey) cache.FileResult {
	methods := make(map[cache.ContractKey]int)
	for index, present := range result.MethodPresent {
		if present {
			methods[contracts[index]] = result.Methods[index].Tokens
		}
	}
	digest := result.ContentDigest
	if digest == ([sha256.Size]byte{}) {
		digest = identity.ContentDigest
	}
	return cache.FileResult{
		Size:           int64(result.FileSize),
		ModTimeNS:      identity.ModTimeNS,
		ContentDigest:  digest,
		Classification: cache.ClassificationText,
		Characters:     result.Characters,
		Words:          result.Words,
		Lines:          result.Lines,
		Methods:        methods,
	}
}

func int64ToInt(value int64) (int, bool) {
	converted := int(value)
	return converted, value >= 0 && int64(converted) == value
}
