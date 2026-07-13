package tokenizer

import (
	"context"
	"fmt"
	"maps"
	"os"
	"runtime"
	"slices"
	"sort"
	"strings"
	"sync"
	"time"
	"unicode"

	"github.com/lancekrogers/tcount/tokenizer/fileops"
)

// Counter handles token counting.
type Counter struct {
	charsPerToken float64
	wordsPerToken float64
	vocabFile     string
	provider      Provider
	stats         *Stats

	mu         sync.Mutex
	tokenizers map[string]Tokenizer
}

// NewCounter creates a new token counter.
// Returns an error if the BPE tokenizers fail to initialize.
func NewCounter(opts CounterOptions) (*Counter, error) {
	if opts.CharsPerToken == 0 {
		opts.CharsPerToken = DefaultCharsPerToken
	}
	if opts.WordsPerToken == 0 {
		opts.WordsPerToken = DefaultWordsPerToken
	}

	c := &Counter{
		charsPerToken: opts.CharsPerToken,
		wordsPerToken: opts.WordsPerToken,
		vocabFile:     opts.VocabFile,
		provider:      opts.Provider,
		stats:         opts.Stats,
		tokenizers:    make(map[string]Tokenizer),
	}

	if err := c.initializeTokenizers(); err != nil {
		return nil, fmt.Errorf("initializing tokenizers: %w", err)
	}

	return c, nil
}

// Count performs token counting using specified methods.
func (c *Counter) Count(ctx context.Context, text string, model string, all bool) (*CountResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	chars := len(text)
	words := countWords(text)

	result := &CountResult{
		Characters: chars,
		Words:      words,
		Lines:      countLines(text),
		Methods:    []MethodResult{},
	}

	plans, includeApprox, err := c.planMethods(model, all)
	if err != nil {
		return nil, fmt.Errorf("counting tokens for model %q: %w", model, err)
	}

	methods, err := applyPlans(plans, text, all || model == "")
	if err != nil {
		return nil, fmt.Errorf("counting tokens for model %q: %w", model, err)
	}
	result.Methods = methods
	if includeApprox {
		result.Methods = append(result.Methods, c.approximationsFromTotals(chars, words)...)
	}

	return result, nil
}

// CountFile counts tokens in a single file.
// It checks for context cancellation, rejects binary files, reads the file
// content, and delegates to Count. The result includes FilePath and FileSize.
func (c *Counter) CountFile(ctx context.Context, path string, model string, all bool) (*CountResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var isBinary bool
	var err error
	if c.stats != nil {
		isBinary, err = fileops.IsBinaryFile(path, c.stats)
	} else {
		isBinary, err = fileops.IsBinaryFile(path)
	}
	if err != nil {
		return nil, fmt.Errorf("checking if file is binary %q: %w", path, err)
	}
	if isBinary {
		return nil, fmt.Errorf("file %q: %w", path, ErrBinaryFile)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	if c.stats != nil {
		c.stats.RecordFullFileOpen()
	}
	var readStarted time.Time
	if c.stats != nil {
		readStarted = time.Now()
	}
	content, err := os.ReadFile(path)
	if c.stats != nil {
		c.stats.RecordValidationReadDuration(time.Since(readStarted))
	}
	if err != nil {
		return nil, fmt.Errorf("reading file %q: %w", path, err)
	}

	result, err := c.Count(ctx, string(content), model, all)
	if err != nil {
		return nil, err
	}

	result.FilePath = path
	result.FileSize = len(content)
	if c.stats != nil {
		c.stats.RecordFullFileBytes(int64(len(content)))
		c.stats.ObserveMemory()
	}

	return result, nil
}

// CountDirectory counts tokens across all text files in a directory.
// It walks the directory respecting .gitignore rules and skipping binary
// files, then counts each file individually via CountFiles, so peak memory
// tracks the largest file rather than the whole tree.
func (c *Counter) CountDirectory(ctx context.Context, path string, model string, all bool) (*CountResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var walkResult *fileops.WalkResult
	var err error
	if c.stats != nil {
		walkResult, err = fileops.WalkDirectory(ctx, path, c.stats)
	} else {
		walkResult, err = fileops.WalkDirectory(ctx, path)
	}
	if err != nil {
		return nil, fmt.Errorf("walking directory %q: %w", path, err)
	}

	if len(walkResult.Files) == 0 {
		return nil, fmt.Errorf("no text files found in directory %q", path)
	}

	result, err := c.CountFiles(ctx, walkResult.Files, model, all)
	if err != nil {
		return nil, fmt.Errorf("counting files in %q: %w", path, err)
	}

	result.FilePath = path
	result.IsDirectory = true

	return result, nil
}

// CountFiles counts tokens across the given text files. Each file is read
// exactly once, counted, and released, so peak memory tracks the largest
// files in flight rather than the combined corpus. Token counts and
// word/line statistics are computed per file and summed: tokens never merge
// across file boundaries, and word counts stay correct when a file lacks a
// trailing newline. Files are processed on a bounded worker pool; sums are
// order-independent so results are deterministic.
func (c *Counter) CountFiles(ctx context.Context, files []string, model string, all bool) (*CountResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	if len(files) == 0 {
		return nil, fmt.Errorf("no files to count")
	}

	plans, includeApprox, err := c.planMethods(model, all)
	if err != nil {
		return nil, fmt.Errorf("counting tokens for model %q: %w", model, err)
	}
	allMode := all || model == ""

	workers := min(runtime.GOMAXPROCS(0), 8, len(files))
	if spmPlanned(plans) {
		// go-sentencepiece does not document Processor thread safety.
		workers = 1
	}

	cctx, cancelFiles := context.WithCancel(ctx)
	defer cancelFiles()

	var (
		wg         sync.WaitGroup
		mu         sync.Mutex
		firstErr   error
		bytesTotal int
		chars      int
		words      int
		lines      int
	)
	tokenSums := make([]int, len(plans))
	failed := make([]bool, len(plans))

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
	for _, file := range files {
		if cctx.Err() != nil {
			break
		}
		select {
		case sem <- struct{}{}:
		case <-cctx.Done():
			break dispatch
		}
		wg.Add(1)
		go func(path string) {
			defer wg.Done()
			defer func() { <-sem }()

			if cctx.Err() != nil {
				return
			}
			if c.stats != nil {
				c.stats.RecordFullFileOpen()
			}
			var readStarted time.Time
			if c.stats != nil {
				readStarted = time.Now()
			}
			content, err := os.ReadFile(path)
			if c.stats != nil {
				c.stats.RecordValidationReadDuration(time.Since(readStarted))
			}
			if err != nil {
				fail(fmt.Errorf("reading file %q: %w", path, err))
				return
			}
			if c.stats != nil {
				c.stats.RecordFullFileBytes(int64(len(content)))
				c.stats.ObserveMemory()
			}
			text := string(content)

			recordTokenization := c.stats.startTokenization()
			if recordTokenization != nil {
				defer recordTokenization()
			}

			fileTokens := make([]int, len(plans))
			for i, p := range plans {
				if cctx.Err() != nil {
					return
				}
				count, err := p.tok.CountTokens(text)
				if err != nil {
					if allMode {
						mu.Lock()
						failed[i] = true
						mu.Unlock()
						continue
					}
					fail(err)
					return
				}
				fileTokens[i] = count
				if c.stats != nil {
					c.stats.RecordTokenizedFile(p.name)
				}
			}
			if recordTokenization != nil {
				recordTokenization()
			}

			fileWords := countWords(text)
			fileLines := countLines(text)

			var aggregationStarted time.Time
			if c.stats != nil {
				aggregationStarted = time.Now()
			}
			mu.Lock()
			bytesTotal += len(content)
			chars += len(text)
			words += fileWords
			lines += fileLines
			for i := range plans {
				tokenSums[i] += fileTokens[i]
			}
			mu.Unlock()
			if c.stats != nil {
				c.stats.RecordAggregationDuration(time.Since(aggregationStarted))
				c.stats.ObserveMemory()
			}
		}(file)
	}
	wg.Wait()

	if firstErr != nil {
		return nil, firstErr
	}
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var aggregationStarted time.Time
	if c.stats != nil {
		aggregationStarted = time.Now()
	}
	methods := make([]MethodResult, 0, len(plans))
	for i, p := range plans {
		if failed[i] {
			continue
		}
		methods = append(methods, MethodResult{
			Name:          p.name,
			DisplayName:   p.displayName,
			Tokens:        tokenSums[i],
			IsExact:       p.isExact,
			ContextWindow: p.contextWindow,
		})
	}
	if includeApprox {
		methods = append(methods, c.approximationsFromTotals(chars, words)...)
	}
	if c.stats != nil {
		c.stats.RecordAggregationDuration(time.Since(aggregationStarted))
	}

	var persistenceStarted time.Time
	if c.stats != nil {
		persistenceStarted = time.Now()
	}
	result := &CountResult{
		Characters: chars,
		Words:      words,
		Lines:      lines,
		Methods:    methods,
		FileSize:   bytesTotal,
		FileCount:  len(files),
	}
	if c.stats != nil {
		c.stats.RecordPersistenceReadyDuration(time.Since(persistenceStarted))
		c.stats.ObserveMemory()
	}
	return result, nil
}

// methodPlan is one counting method resolved ahead of execution, so a single
// selection drives both single-text and per-file counting.
type methodPlan struct {
	name          string
	displayName   string
	isExact       bool
	contextWindow int
	tok           Tokenizer
}

// planMethods resolves which counting methods apply for the given model and
// all flag. The second return reports whether the approximation methods
// (computed from character/word totals) should be appended.
func (c *Counter) planMethods(model string, all bool) ([]methodPlan, bool, error) {
	if all || model == "" {
		for _, encoding := range bpeEncodings {
			if c.provider != "" && c.provider != "all" && !encodingMatchesProvider(encoding, c.provider) {
				continue
			}
			if _, err := c.bpeTokenizer(encoding); err != nil {
				continue
			}
		}

		tokenizers := c.tokenizerSnapshot()
		keys := make([]string, 0, len(tokenizers))
		for k := range tokenizers {
			keys = append(keys, k)
		}
		sort.Strings(keys)

		plans := make([]methodPlan, 0, len(keys))
		for _, encoding := range keys {
			if c.provider != "" && c.provider != "all" && !encodingMatchesProvider(encoding, c.provider) {
				continue
			}
			tok := tokenizers[encoding]
			plans = append(plans, methodPlan{
				name:        tok.Name(),
				displayName: tok.DisplayName(),
				isExact:     tok.IsExact(),
				tok:         tok,
			})
		}
		return plans, true, nil
	}

	meta := LookupModel(model)
	if meta != nil {
		tok, err := c.tokenizerForEncoding(meta.Encoding)
		if err != nil {
			return nil, false, err
		}
		if tok != nil {
			return []methodPlan{{
				name:          fmt.Sprintf("bpe_%s", strings.ReplaceAll(model, "-", "_")),
				displayName:   fmt.Sprintf("%s (%s)", meta.Encoding, model),
				isExact:       tok.IsExact(),
				contextWindow: meta.ContextWindow,
				tok:           tok,
			}}, false, nil
		}
	}

	if tok, err := c.tokenizerForEncoding(model); err == nil && tok != nil {
		plan := methodPlan{
			name:        tok.Name(),
			displayName: tok.DisplayName(),
			isExact:     tok.IsExact(),
			tok:         tok,
		}
		if meta != nil {
			plan.contextWindow = meta.ContextWindow
		}
		return []methodPlan{plan}, false, nil
	}

	return nil, true, nil
}

// applyPlans runs each planned method against one text. When skipErrors is
// true a failing method is dropped, matching all-methods semantics;
// otherwise the first error aborts.
func applyPlans(plans []methodPlan, text string, skipErrors bool) ([]MethodResult, error) {
	methods := make([]MethodResult, 0, len(plans))
	for _, p := range plans {
		count, err := p.tok.CountTokens(text)
		if err != nil {
			if skipErrors {
				continue
			}
			return nil, err
		}
		methods = append(methods, MethodResult{
			Name:          p.name,
			DisplayName:   p.displayName,
			Tokens:        count,
			IsExact:       p.isExact,
			ContextWindow: p.contextWindow,
		})
	}
	return methods, nil
}

// spmPlanned reports whether any plan uses the SentencePiece tokenizer.
func spmPlanned(plans []methodPlan) bool {
	for _, p := range plans {
		if _, ok := p.tok.(*SPMTokenizerWrapper); ok {
			return true
		}
	}
	return false
}

// encodingMatchesProvider checks if an encoding should be included for a provider filter.
func encodingMatchesProvider(encoding string, provider Provider) bool {
	switch encoding {
	case EncodingO200kBase:
		return provider == ProviderOpenAI
	case EncodingCL100kBase:
		return provider == ProviderOpenAI || provider == ProviderMeta || provider == ProviderDeepSeek || provider == ProviderAlibaba || provider == ProviderMicrosoft
	case EncodingClaudeApprox:
		return provider == ProviderAnthropic
	case EncodingGeminiApprox:
		return provider == ProviderGoogle
	}
	return false
}

// approximationsFromTotals returns approximation-based token counts computed
// from already-summed character and word totals.
func (c *Counter) approximationsFromTotals(chars, words int) []MethodResult {
	multiplier := 1.0 / c.wordsPerToken
	multiplierStr := fmt.Sprintf("%.0f", multiplier*100)

	return []MethodResult{
		{
			Name:        fmt.Sprintf("character_based_div%.0f", c.charsPerToken),
			DisplayName: fmt.Sprintf("Character-based (÷%.1f)", c.charsPerToken),
			Tokens:      int(float64(chars) / c.charsPerToken),
			IsExact:     false,
		},
		{
			Name:        fmt.Sprintf("word_based_mul%s", multiplierStr),
			DisplayName: fmt.Sprintf("Word-based (×%.2f)", multiplier),
			Tokens:      int(float64(words) / c.wordsPerToken),
			IsExact:     false,
		},
		{
			Name:        "whitespace_split",
			DisplayName: "Whitespace split",
			Tokens:      words,
			IsExact:     false,
		},
	}
}

// initializeTokenizers sets up the cheap tokenizers eagerly. The BPE
// tokenizers parse multi-megabyte vocab tables, so they load lazily via
// bpeTokenizer the first time an encoding is actually counted with.
func (c *Counter) initializeTokenizers() error {
	c.tokenizers[EncodingClaudeApprox] = NewClaudeApproximator()
	c.tokenizers[EncodingGeminiApprox] = NewGeminiApproximator()

	if c.vocabFile != "" {
		tok, err := NewSPMTokenizer(c.vocabFile)
		if err != nil {
			return fmt.Errorf("loading SentencePiece vocab %q: %w", c.vocabFile, err)
		}
		c.tokenizers[EncodingSPM] = tok
	}

	return nil
}

// bpeEncodings are the encodings loaded on demand by bpeTokenizer.
var bpeEncodings = []string{EncodingO200kBase, EncodingCL100kBase}

// bpeTokenizer returns the tokenizer for a BPE encoding, loading and caching
// it on first use.
func (c *Counter) bpeTokenizer(encoding string) (Tokenizer, error) {
	c.mu.Lock()
	defer c.mu.Unlock()

	if tok, ok := c.tokenizers[encoding]; ok {
		return tok, nil
	}

	tok, err := NewBPETokenizerByEncoding(encoding)
	if err != nil {
		return nil, fmt.Errorf("loading %s encoding: %w", encoding, err)
	}
	c.tokenizers[encoding] = tok
	return tok, nil
}

// tokenizerForEncoding resolves any encoding name, loading BPE encodings on
// demand and returning nil (no error) for unknown encodings.
func (c *Counter) tokenizerForEncoding(encoding string) (Tokenizer, error) {
	if slices.Contains(bpeEncodings, encoding) {
		return c.bpeTokenizer(encoding)
	}

	c.mu.Lock()
	defer c.mu.Unlock()
	return c.tokenizers[encoding], nil
}

// tokenizerSnapshot returns a copy of the current tokenizer map for
// race-free iteration.
func (c *Counter) tokenizerSnapshot() map[string]Tokenizer {
	c.mu.Lock()
	defer c.mu.Unlock()

	snap := make(map[string]Tokenizer, len(c.tokenizers))
	maps.Copy(snap, c.tokenizers)
	return snap
}

// countWords counts words in text.
func countWords(text string) int {
	words := 0
	inWord := false

	for _, r := range text {
		if unicode.IsSpace(r) || unicode.IsPunct(r) {
			if inWord {
				words++
				inWord = false
			}
		} else {
			inWord = true
		}
	}

	if inWord {
		words++
	}

	return words
}

// countLines counts lines in text.
func countLines(text string) int {
	if len(text) == 0 {
		return 0
	}

	lines := strings.Count(text, "\n")
	if text[len(text)-1] != '\n' {
		lines++
	}

	return lines
}
