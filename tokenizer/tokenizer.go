// Package tokenizer provides token counting for LLM models.
//
// It supports exact BPE tokenization for OpenAI models, character-based
// approximation for Claude and Gemini models, and SentencePiece tokenization
// for open-source models like Llama and Mistral.
package tokenizer

import (
	"fmt"
	"os"
	"strings"

	sentencepiece "github.com/eliben/go-sentencepiece"
	"github.com/lancekrogers/tcount/tokenizer/bpe"
)

// Encoding identifiers shared across the tokenizer and CLI layers.
const (
	EncodingO200kBase    = bpe.EncodingO200kBase
	EncodingCL100kBase   = bpe.EncodingCL100kBase
	EncodingClaudeApprox = "claude_approx"
	EncodingGeminiApprox = "gemini_approx"
	EncodingSPM          = "spm"
)

// NameClaudeApprox is the machine-readable identifier the Claude
// approximator reports; consumers key accuracy labeling off it.
const NameClaudeApprox = "claude_3_approx"

// Default approximation ratios applied when CounterOptions leaves them zero.
const (
	DefaultCharsPerToken = 4.0
	DefaultWordsPerToken = 0.75
)

// Tokenizer counts tokens in text using a specific tokenization method.
type Tokenizer interface {
	// CountTokens returns the token count for the given text.
	CountTokens(text string) (int, error)

	// Name returns the tokenizer's machine-readable identifier.
	Name() string

	// DisplayName returns the tokenizer's human-readable name.
	DisplayName() string

	// IsExact returns true if this tokenizer produces exact counts
	// (as opposed to approximations).
	IsExact() bool
}

// BPETokenizerWrapper implements exact tokenization using a BPE encoding.
type BPETokenizerWrapper struct {
	encodingName string
	tokenizer    *bpe.BPETokenizer
}

// NewBPETokenizer creates an exact tokenizer for the given model name.
// Supports OpenAI models (gpt-4o, gpt-5, o3, o4-mini, etc.) and
// open-source models that use BPE-compatible encodings.
func NewBPETokenizer(model string) (Tokenizer, error) {
	encodingName, _ := getEncodingForModel(model)
	return NewBPETokenizerByEncoding(encodingName)
}

// NewBPETokenizerByEncoding creates a tokenizer for a specific BPE encoding.
// Supported encodings: o200k_base, cl100k_base, p50k_base, r50k_base.
func NewBPETokenizerByEncoding(encodingName string) (Tokenizer, error) {
	tokenizer, err := bpe.NewEncoderByName(encodingName)
	if err != nil {
		return nil, fmt.Errorf("getting encoding %q: %w", encodingName, err)
	}

	return &BPETokenizerWrapper{
		encodingName: encodingName,
		tokenizer:    tokenizer,
	}, nil
}

// CountTokens counts tokens using BPE tokenization. Counting never allows
// special tokens, so it takes the ordinary encode path, which skips the
// special-token scan and produces identical counts.
func (t *BPETokenizerWrapper) CountTokens(text string) (int, error) {
	return len(t.tokenizer.EncodeOrdinary(text)), nil
}

// Name returns the machine-readable tokenizer identifier.
func (t *BPETokenizerWrapper) Name() string {
	return fmt.Sprintf("bpe_%s", t.encodingName)
}

// DisplayName returns the human-readable tokenizer name.
func (t *BPETokenizerWrapper) DisplayName() string {
	return t.encodingName
}

// IsExact returns true for BPE tokenizers.
func (t *BPETokenizerWrapper) IsExact() bool {
	return true
}

// getEncodingForModel maps model names to encoding types.
// The second return value indicates whether the model was recognized.
// Unrecognized models fall back to o200k_base.
func getEncodingForModel(model string) (string, bool) {
	model = strings.ToLower(model)

	if strings.HasPrefix(model, "gpt-5") {
		return "o200k_base", true
	}
	if strings.HasPrefix(model, "gpt-4.1") {
		return "o200k_base", true
	}
	if strings.HasPrefix(model, "gpt-4o") {
		return "o200k_base", true
	}
	if strings.HasPrefix(model, "o3") || strings.HasPrefix(model, "o4") {
		return "o200k_base", true
	}

	if strings.HasPrefix(model, "gpt-4") || strings.HasPrefix(model, "gpt-3.5") {
		return "cl100k_base", true
	}

	if strings.HasPrefix(model, "llama-") ||
		strings.HasPrefix(model, "deepseek-") ||
		strings.HasPrefix(model, "qwen-") ||
		strings.HasPrefix(model, "phi-") {
		return "cl100k_base", true
	}

	if strings.Contains(model, "davinci") || strings.Contains(model, "curie") {
		return "p50k_base", true
	}

	return "o200k_base", false
}

// claudeCharsPerToken is the approximate character-to-token ratio for Claude models.
// Based on Anthropic's documentation of ~3.8 characters per token for English text.
const claudeCharsPerToken = 3.8

// ClaudeApproximator provides approximation for Claude models.
type ClaudeApproximator struct{}

// NewClaudeApproximator creates a character-based approximator tuned for
// Claude models. Uses a 3.8 characters per token ratio.
func NewClaudeApproximator() Tokenizer {
	return &ClaudeApproximator{}
}

// CountTokens approximates token count for Claude.
func (c *ClaudeApproximator) CountTokens(text string) (int, error) {
	tokens := int(float64(len(text)) / claudeCharsPerToken)
	return tokens, nil
}

// Name returns the machine-readable tokenizer identifier.
func (c *ClaudeApproximator) Name() string {
	return NameClaudeApprox
}

// DisplayName returns the human-readable tokenizer name.
func (c *ClaudeApproximator) DisplayName() string {
	return "Claude (approx)"
}

// IsExact returns false for approximations.
func (c *ClaudeApproximator) IsExact() bool {
	return false
}

// geminiCharsPerToken is the approximate character-to-token ratio for Gemini models.
// Based on Google's guidance of ~4 characters per token for English text.
const geminiCharsPerToken = 4.0

// GeminiApproximator provides approximation for Google Gemini models.
// Gemini uses its own SentencePiece tokenizer; for exact counts supply the
// vocab file via --vocab-file. Without it, this character-based estimate applies.
type GeminiApproximator struct{}

// NewGeminiApproximator creates a character-based approximator tuned for
// Gemini models. Uses a 4.0 characters per token ratio.
func NewGeminiApproximator() Tokenizer {
	return &GeminiApproximator{}
}

// CountTokens approximates token count for Gemini.
func (g *GeminiApproximator) CountTokens(text string) (int, error) {
	tokens := int(float64(len(text)) / geminiCharsPerToken)
	return tokens, nil
}

// Name returns the machine-readable tokenizer identifier.
func (g *GeminiApproximator) Name() string {
	return EncodingGeminiApprox
}

// DisplayName returns the human-readable tokenizer name.
func (g *GeminiApproximator) DisplayName() string {
	return "Gemini (approx)"
}

// IsExact returns false for approximations.
func (g *GeminiApproximator) IsExact() bool {
	return false
}

// SPMTokenizerWrapper uses a .model vocab file for exact tokenization.
type SPMTokenizerWrapper struct {
	processor *sentencepiece.Processor
	modelPath string
}

// NewSPMTokenizer creates a SentencePiece tokenizer from a .model vocab file.
// Supports Llama, Mistral, Gemma, and other SPM-based models.
func NewSPMTokenizer(modelPath string) (Tokenizer, error) {
	if modelPath == "" {
		return nil, ErrVocabFileRequired
	}

	if _, err := os.Stat(modelPath); err != nil {
		if os.IsNotExist(err) {
			return nil, fmt.Errorf("vocab file not found: %s", modelPath)
		}
		return nil, fmt.Errorf("accessing vocab file: %w", err)
	}

	processor, err := sentencepiece.NewProcessorFromPath(modelPath)
	if err != nil {
		return nil, fmt.Errorf("loading SentencePiece model: %w", err)
	}

	return &SPMTokenizerWrapper{
		processor: processor,
		modelPath: modelPath,
	}, nil
}

// CountTokens returns the token count using the SentencePiece model.
func (t *SPMTokenizerWrapper) CountTokens(text string) (int, error) {
	tokens := t.processor.Encode(text)
	return len(tokens), nil
}

// Name returns the machine-readable tokenizer identifier.
func (t *SPMTokenizerWrapper) Name() string {
	return EncodingSPM
}

// DisplayName returns the human-readable tokenizer name.
func (t *SPMTokenizerWrapper) DisplayName() string {
	return "SentencePiece"
}

// IsExact returns true because SentencePiece provides exact token counts.
func (t *SPMTokenizerWrapper) IsExact() bool {
	return true
}
