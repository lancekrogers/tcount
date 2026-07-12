package tokenizer

import (
	"context"
	"fmt"
	"maps"
	"os"
	"slices"
	"sort"
	"strings"
	"sync"
	"unicode"

	"github.com/lancekrogers/tcount/tokenizer/fileops"
)

// Counter handles token counting.
type Counter struct {
	charsPerToken float64
	wordsPerToken float64
	vocabFile     string
	provider      Provider

	mu         sync.Mutex
	tokenizers map[string]Tokenizer
}

// NewCounter creates a new token counter.
// Returns an error if the BPE tokenizers fail to initialize.
func NewCounter(opts CounterOptions) (*Counter, error) {
	if opts.CharsPerToken == 0 {
		opts.CharsPerToken = 4.0
	}
	if opts.WordsPerToken == 0 {
		opts.WordsPerToken = 0.75
	}

	c := &Counter{
		charsPerToken: opts.CharsPerToken,
		wordsPerToken: opts.WordsPerToken,
		vocabFile:     opts.VocabFile,
		provider:      opts.Provider,
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

	result := &CountResult{
		Characters: len(text),
		Words:      countWords(text),
		Lines:      countLines(text),
		Methods:    []MethodResult{},
	}

	if all || model == "" {
		result.Methods = c.countAllMethods(text)
	} else {
		methods, err := c.countSpecificModel(text, model)
		if err != nil {
			return nil, fmt.Errorf("counting tokens for model %q: %w", model, err)
		}
		result.Methods = methods
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

	isBinary, err := fileops.IsBinaryFile(path)
	if err != nil {
		return nil, fmt.Errorf("checking if file is binary %q: %w", path, err)
	}
	if isBinary {
		return nil, fmt.Errorf("file %q: %w", path, ErrBinaryFile)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	content, err := os.ReadFile(path)
	if err != nil {
		return nil, fmt.Errorf("reading file %q: %w", path, err)
	}

	result, err := c.Count(ctx, string(content), model, all)
	if err != nil {
		return nil, err
	}

	result.FilePath = path
	result.FileSize = len(content)

	return result, nil
}

// CountDirectory counts tokens across all text files in a directory.
// It walks the directory respecting .gitignore rules and skipping binary files,
// aggregates all file contents, and counts tokens on the combined text.
// Context cancellation is checked between each major operation.
//
// Note: this operation loads all text file content into memory before counting.
// For very large repositories, consider processing files individually with CountFile.
func (c *Counter) CountDirectory(ctx context.Context, path string, model string, all bool) (*CountResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	walkResult, err := fileops.WalkDirectory(ctx, path)
	if err != nil {
		return nil, fmt.Errorf("walking directory %q: %w", path, err)
	}

	if len(walkResult.Files) == 0 {
		return nil, fmt.Errorf("no text files found in directory %q", path)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	content, err := fileops.AggregateFileContents(ctx, walkResult.Files)
	if err != nil {
		return nil, fmt.Errorf("reading files in %q: %w", path, err)
	}

	if err := ctx.Err(); err != nil {
		return nil, err
	}

	result, err := c.Count(ctx, string(content), model, all)
	if err != nil {
		return nil, err
	}

	result.FilePath = path
	result.FileSize = len(content)
	result.IsDirectory = true
	result.FileCount = len(walkResult.Files)

	return result, nil
}

// countAllMethods counts tokens using all available encodings (deduplicated).
func (c *Counter) countAllMethods(text string) []MethodResult {
	methods := []MethodResult{}
	seen := make(map[string]bool)

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

	for _, encoding := range keys {
		tokenizer := tokenizers[encoding]

		if c.provider != "" && c.provider != "all" {
			if !encodingMatchesProvider(encoding, c.provider) {
				continue
			}
		}

		if seen[encoding] {
			continue
		}
		seen[encoding] = true

		if count, err := tokenizer.CountTokens(text); err == nil {
			methods = append(methods, MethodResult{
				Name:        tokenizer.Name(),
				DisplayName: tokenizer.DisplayName(),
				Tokens:      count,
				IsExact:     tokenizer.IsExact(),
			})
		}
	}

	methods = append(methods, c.getApproximations(text)...)

	return methods
}

// encodingMatchesProvider checks if an encoding should be included for a provider filter.
func encodingMatchesProvider(encoding string, provider Provider) bool {
	switch encoding {
	case "o200k_base":
		return provider == ProviderOpenAI
	case "cl100k_base":
		return provider == ProviderOpenAI || provider == ProviderMeta || provider == ProviderDeepSeek || provider == ProviderAlibaba || provider == ProviderMicrosoft
	case "claude_approx":
		return provider == ProviderAnthropic
	case "gemini_approx":
		return provider == ProviderGoogle
	}
	return false
}

// countSpecificModel counts tokens for a specific model.
func (c *Counter) countSpecificModel(text string, model string) ([]MethodResult, error) {
	methods := []MethodResult{}

	meta := GetModelMetadata(model)
	if meta != nil {
		tokenizer, err := c.tokenizerForEncoding(meta.Encoding)
		if err != nil {
			return nil, err
		}
		if tokenizer != nil {
			count, err := tokenizer.CountTokens(text)
			if err != nil {
				return nil, err
			}
			methods = append(methods, MethodResult{
				Name:          fmt.Sprintf("bpe_%s", strings.ReplaceAll(model, "-", "_")),
				DisplayName:   fmt.Sprintf("%s (%s)", meta.Encoding, model),
				Tokens:        count,
				IsExact:       tokenizer.IsExact(),
				ContextWindow: meta.ContextWindow,
			})
			return methods, nil
		}
	}

	if tokenizer, err := c.tokenizerForEncoding(model); err == nil && tokenizer != nil {
		count, err := tokenizer.CountTokens(text)
		if err != nil {
			return nil, err
		}
		result := MethodResult{
			Name:        tokenizer.Name(),
			DisplayName: tokenizer.DisplayName(),
			Tokens:      count,
			IsExact:     tokenizer.IsExact(),
		}
		if meta != nil {
			result.ContextWindow = meta.ContextWindow
		}
		methods = append(methods, result)
		return methods, nil
	}

	methods = append(methods, c.getApproximations(text)...)
	return methods, nil
}

// getApproximations returns approximation-based token counts.
func (c *Counter) getApproximations(text string) []MethodResult {
	chars := len(text)
	words := countWords(text)

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
	c.tokenizers["claude_approx"] = NewClaudeApproximator()
	c.tokenizers["gemini_approx"] = NewGeminiApproximator()

	if c.vocabFile != "" {
		tok, err := NewSPMTokenizer(c.vocabFile)
		if err != nil {
			return fmt.Errorf("loading SentencePiece vocab %q: %w", c.vocabFile, err)
		}
		c.tokenizers["spm"] = tok
	}

	return nil
}

// bpeEncodings are the encodings loaded on demand by bpeTokenizer.
var bpeEncodings = []string{"o200k_base", "cl100k_base"}

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
