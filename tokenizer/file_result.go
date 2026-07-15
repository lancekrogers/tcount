package tokenizer

import (
	"context"
	"crypto/sha256"
	"fmt"
	"os"
	"runtime"
	"sync"
	"time"
)

type fileClassification uint8

const fileClassificationText fileClassification = 1

// perFileResult is the cache-neutral reducible value shared by fresh and
// reusable directory results. Method slots stay aligned with the resolved
// plans so an all-method failure can be omitted consistently at aggregation.
type perFileResult struct {
	FileSize       int
	Characters     int
	Words          int
	Lines          int
	Classification fileClassification
	ContentDigest  [sha256.Size]byte
	Methods        []MethodResult
	MethodPresent  []bool
}

func (c *Counter) countFileResult(ctx context.Context, path string, plans []methodPlan, allMode bool) (perFileResult, error) {
	return c.countFileResultForIndices(ctx, path, plans, allMode, allMethodIndices(len(plans)), false, nil)
}

func (c *Counter) countFileResultForCache(ctx context.Context, path string, plans []methodPlan, allMode bool, indices []int, validated *cacheValidatedFile) (perFileResult, error) {
	return c.countFileResultForIndices(ctx, path, plans, allMode, indices, true, validated)
}

func (c *Counter) countFileResultForIndices(ctx context.Context, path string, plans []methodPlan, allMode bool, indices []int, captureDigest bool, validated *cacheValidatedFile) (perFileResult, error) {
	if err := ctx.Err(); err != nil {
		return perFileResult{}, err
	}
	var content []byte
	if validated != nil {
		content = validated.content
	} else {
		if c.stats != nil {
			c.stats.RecordFullFileOpen()
		}
		var readStarted time.Time
		if c.stats != nil {
			readStarted = time.Now()
		}
		var err error
		content, err = os.ReadFile(path)
		if c.stats != nil {
			c.stats.RecordValidationReadDuration(time.Since(readStarted))
		}
		if err != nil {
			return perFileResult{}, fmt.Errorf("reading file %q: %w", path, err)
		}
	}
	if c.stats != nil && validated == nil {
		c.stats.RecordFullFileBytes(int64(len(content)))
		c.stats.ObserveMemory()
	}

	text := string(content)
	result := perFileResult{
		FileSize:       len(content),
		Characters:     len(text),
		Words:          countWords(text),
		Lines:          countLines(text),
		Classification: fileClassificationText,
		Methods:        make([]MethodResult, len(plans)),
		MethodPresent:  make([]bool, len(plans)),
	}
	if captureDigest {
		if validated != nil && validated.digestKnown {
			result.ContentDigest = validated.digest
		} else {
			result.ContentDigest = sha256.Sum256(content)
		}
	}
	recordTokenization := c.stats.startTokenization()
	if recordTokenization != nil {
		defer recordTokenization()
	}
	for _, i := range indices {
		plan := plans[i]
		if err := ctx.Err(); err != nil {
			return perFileResult{}, err
		}
		count, err := plan.tok.CountTokens(text)
		if err != nil {
			if allMode {
				continue
			}
			return perFileResult{}, err
		}
		result.Methods[i] = MethodResult{
			Name:          plan.name,
			DisplayName:   plan.displayName,
			Tokens:        count,
			IsExact:       plan.isExact,
			ContextWindow: plan.contextWindow,
		}
		result.MethodPresent[i] = true
		if c.stats != nil {
			c.stats.RecordTokenizedFile(plan.name)
		}
	}
	return result, nil
}

func allMethodIndices(count int) []int {
	indices := make([]int, count)
	for i := range indices {
		indices[i] = i
	}
	return indices
}

func (c *Counter) countFileResults(ctx context.Context, files []string, plans []methodPlan, allMode bool, selected map[string][]int, validated map[string]cacheValidatedFile) ([]perFileResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return []perFileResult{}, nil
	}

	workers := min(runtime.GOMAXPROCS(0), 8, len(files))
	if spmPlanned(plans) {
		// go-sentencepiece does not document Processor thread safety.
		workers = 1
	}

	cctx, cancelFiles := context.WithCancel(ctx)
	defer cancelFiles()
	results := make([]perFileResult, len(files))
	var (
		wg       sync.WaitGroup
		mu       sync.Mutex
		firstErr error
	)
	fail := func(err error) {
		mu.Lock()
		if firstErr == nil {
			firstErr = err
		}
		mu.Unlock()
		cancelFiles()
	}

	sem := make(chan struct{}, workers)
dispatch:
	for index, file := range files {
		if cctx.Err() != nil {
			break
		}
		select {
		case sem <- struct{}{}:
		case <-cctx.Done():
			break dispatch
		}
		wg.Add(1)
		go func(index int, path string) {
			defer wg.Done()
			defer func() { <-sem }()
			if cctx.Err() != nil {
				return
			}

			indices := allMethodIndices(len(plans))
			if selected != nil {
				indices = selected[path]
			}
			var validatedFile *cacheValidatedFile
			if validated != nil {
				if value, ok := validated[path]; ok {
					validatedFile = &value
				}
			}
			var (
				result perFileResult
				err    error
			)
			if selected == nil {
				result, err = c.countFileResult(cctx, path, plans, allMode)
			} else {
				result, err = c.countFileResultForCache(cctx, path, plans, allMode, indices, validatedFile)
			}
			if err != nil {
				fail(err)
				return
			}
			results[index] = result
		}(index, file)
	}
	wg.Wait()
	if firstErr != nil {
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

func (c *Counter) aggregateFileResults(ctx context.Context, results []perFileResult, plans []methodPlan, includeApprox bool) (*CountResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	started := time.Now()
	bytesTotal, chars, words, lines := 0, 0, 0, 0
	methodSums := make([]int, len(plans))
	methodAvailable := make([]bool, len(plans))
	for i := range methodAvailable {
		methodAvailable[i] = true
	}
	for _, file := range results {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		if len(file.Methods) != len(plans) || len(file.MethodPresent) != len(plans) {
			return nil, fmt.Errorf("invalid per-file result method shape")
		}
		bytesTotal += file.FileSize
		chars += file.Characters
		words += file.Words
		lines += file.Lines
		for i := range plans {
			if !file.MethodPresent[i] {
				methodAvailable[i] = false
				continue
			}
			methodSums[i] += file.Methods[i].Tokens
		}
	}

	methods := make([]MethodResult, 0, len(plans))
	for i, plan := range plans {
		if !methodAvailable[i] {
			continue
		}
		methods = append(methods, MethodResult{
			Name:          plan.name,
			DisplayName:   plan.displayName,
			Tokens:        methodSums[i],
			IsExact:       plan.isExact,
			ContextWindow: plan.contextWindow,
		})
	}
	if includeApprox {
		methods = append(methods, c.approximationsFromTotals(chars, words)...)
	}
	if c.stats != nil {
		c.stats.RecordAggregationDuration(time.Since(started))
	}
	return &CountResult{Characters: chars, Words: words, Lines: lines, Methods: methods, FileSize: bytesTotal, FileCount: len(results)}, nil
}
