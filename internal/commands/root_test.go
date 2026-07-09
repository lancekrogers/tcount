package commands

import (
	"testing"

	"github.com/lancekrogers/tcount/tokenizer"
)

func TestIsValidModel(t *testing.T) {
	tests := []struct {
		model string
		valid bool
	}{
		{"", true},
		{"gpt-5", true},
		{"gpt-5-mini", true},
		{"gpt-4.1", true},
		{"gpt-4.1-mini", true},
		{"gpt-4.1-nano", true},
		{"gpt-4o", true},
		{"gpt-4o-mini", true},
		{"o3", true},
		{"o3-mini", true},
		{"o4-mini", true},
		{"gpt-4", true},
		{"gpt-4-turbo", true},
		{"gpt-3.5-turbo", true},
		{"claude-opus-4.6", true},
		{"claude-opus-4.5", true},
		{"claude-opus-4.1", true},
		{"claude-opus-4", true},
		{"claude-sonnet-4.6", true},
		{"claude-sonnet-4.5", true},
		{"claude-sonnet-4", true},
		{"claude-haiku-4.5", true},
		{"claude-haiku-3.5", true},
		{"claude-haiku-3", true},
		{"claude-opus-3", true},
		{"llama-3.1-8b", true},
		{"llama-4-scout", true},
		{"llama-4-maverick", true},
		{"deepseek-v2", true},
		{"deepseek-v3", true},
		{"deepseek-coder-v2", true},
		{"qwen-2.5-7b", true},
		{"qwen-3-72b", true},
		{"phi-3-mini", true},
		{"phi-3-small", true},
		{"phi-3-medium", true},
		{"nonexistent-model", false},
		{"GPT-5", false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			if got := isValidModel(tt.model); got != tt.valid {
				t.Errorf("isValidModel(%q) = %v, want %v", tt.model, got, tt.valid)
			}
		})
	}
}

func TestIsValidProvider(t *testing.T) {
	tests := []struct {
		provider string
		valid    bool
	}{
		{"openai", true},
		{"anthropic", true},
		{"meta", true},
		{"deepseek", true},
		{"alibaba", true},
		{"microsoft", true},
		{"all", true},
		{"google", true},
		{"invalid", false},
		{"", false},
	}

	for _, tt := range tests {
		t.Run(tt.provider, func(t *testing.T) {
			if got := isValidProvider(tt.provider); got != tt.valid {
				t.Errorf("isValidProvider(%q) = %v, want %v", tt.provider, got, tt.valid)
			}
		})
	}
}

func TestRequiresSentencePiece(t *testing.T) {
	tests := []struct {
		model    string
		requires bool
		hasURL   bool
	}{
		{"llama-3.1-8b", true, true},
		{"llama-3.1-70b", true, true},
		{"llama-3.1-405b", true, true},
		{"llama-4-scout", true, true},
		{"llama-4-maverick", true, true},
		{"gpt-5", false, false},
		{"claude-opus-4.6", false, false},
		{"deepseek-v3", false, false},
	}

	for _, tt := range tests {
		t.Run(tt.model, func(t *testing.T) {
			requires, url := requiresSentencePiece(tt.model)
			if requires != tt.requires {
				t.Errorf("requiresSentencePiece(%q) requires = %v, want %v", tt.model, requires, tt.requires)
			}
			if tt.hasURL && url == "" {
				t.Errorf("requiresSentencePiece(%q) expected a URL, got empty", tt.model)
			}
			if !tt.hasURL && url != "" {
				t.Errorf("requiresSentencePiece(%q) expected no URL, got %s", tt.model, url)
			}
		})
	}
}

func TestListModelsContainsKeyModels(t *testing.T) {
	models := tokenizer.ListModels()
	if len(models) == 0 {
		t.Fatal("tokenizer.ListModels() returned empty list")
	}

	// Check key models are present
	required := []string{"gpt-5", "gpt-4o", "claude-opus-4.6", "claude-sonnet-4.6", "llama-4-scout", "deepseek-v3", "qwen-3-72b"}
	for _, model := range required {
		found := false
		for _, m := range models {
			if m == model {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("Required model %q not in tokenizer.ListModels()", model)
		}
	}
}

func TestNewRootCmd(t *testing.T) {
	cmd := newRootCmd("test")
	if cmd == nil {
		t.Fatal("newRootCmd() returned nil")
	}

	if cmd.Use != "tcount [file|directory]" {
		t.Errorf("Unexpected Use: %s", cmd.Use)
	}

	// Verify flags exist
	flags := []string{"model", "vocab-file", "provider", "all", "json", "models", "recursive", "no-color", "verbose"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil && cmd.PersistentFlags().Lookup(flag) == nil {
			t.Errorf("Flag --%s not found", flag)
		}
	}
}
