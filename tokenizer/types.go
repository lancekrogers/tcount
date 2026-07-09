package tokenizer

import "errors"

// Sentinel errors for common failure modes.
var (
	// ErrModelNotFound is returned when a requested model is not in the registry.
	ErrModelNotFound = errors.New("model not found")

	// ErrEncodingNotFound is returned when a BPE encoding name is not recognized.
	ErrEncodingNotFound = errors.New("encoding not found")

	// ErrVocabFileRequired is returned when a SentencePiece model path is empty.
	ErrVocabFileRequired = errors.New("vocab file path is required")

	// ErrBinaryFile is returned when attempting to count tokens in a binary file.
	ErrBinaryFile = errors.New("file is binary")
)

// CountResult represents the result of token counting.
type CountResult struct {
	FilePath    string         `json:"file_path"`
	IsDirectory bool           `json:"is_directory,omitempty"`
	FileCount   int            `json:"file_count,omitempty"`
	FileSize    int            `json:"file_size"`
	Characters  int            `json:"characters"`
	Words       int            `json:"words"`
	Lines       int            `json:"lines"`
	Methods     []MethodResult `json:"methods"`
}

// MethodResult represents token count for a specific method.
type MethodResult struct {
	Name          string `json:"name"`
	DisplayName   string `json:"display_name"`
	Tokens        int    `json:"tokens"`
	IsExact       bool   `json:"is_exact"`
	ContextWindow int    `json:"context_window,omitempty"`
}

// CounterOptions configures the counter.
type CounterOptions struct {
	CharsPerToken float64
	WordsPerToken float64
	VocabFile     string
	Provider      Provider
}
