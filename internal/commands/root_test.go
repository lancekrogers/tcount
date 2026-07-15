package commands

import (
	"strings"
	"testing"

	"github.com/spf13/cobra"

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
	flags := []string{"model", "vocab-file", "provider", "all", "json", "models", "recursive", "cache", "no-cache", "cache-verify", "no-color", "verbose"}
	for _, flag := range flags {
		if cmd.Flags().Lookup(flag) == nil && cmd.PersistentFlags().Lookup(flag) == nil {
			t.Errorf("Flag --%s not found", flag)
		}
	}
}

func TestValidateCacheFlags(t *testing.T) {
	tests := []struct {
		name string
		opts countOptions
		want string
	}{
		{name: "cache and no-cache", opts: countOptions{cache: true, noCache: true}, want: "cannot be used together"},
		{name: "verify without cache", opts: countOptions{cacheVerify: true}, want: "requires --cache"},
		{name: "valid cache", opts: countOptions{cache: true}},
		{name: "valid cold bypass", opts: countOptions{noCache: true}},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			err := validateCacheFlags(&test.opts)
			if test.want == "" {
				if err != nil {
					t.Fatalf("validateCacheFlags() error = %v", err)
				}
				return
			}
			if err == nil || !strings.Contains(err.Error(), test.want) {
				t.Fatalf("validateCacheFlags() error = %v, want substring %q", err, test.want)
			}
		})
	}
}

func TestValidateCacheTarget(t *testing.T) {
	if err := validateCacheTarget(&countOptions{cache: true}, false); err == nil || !strings.Contains(err.Error(), "recursive directory") {
		t.Fatalf("file cache target error = %v", err)
	}
	if err := validateCacheTarget(&countOptions{cache: true}, true); err != nil {
		t.Fatalf("directory cache target error = %v", err)
	}
	if err := validateCacheTarget(&countOptions{noCache: true}, false); err != nil {
		t.Fatalf("single-file cold bypass error = %v", err)
	}
}

func TestCacheCommandComposition(t *testing.T) {
	cmd := newRootCmd("test")
	var cacheCmdFound bool
	for _, child := range cmd.Commands() {
		if child.Name() != "cache" {
			continue
		}
		cacheCmdFound = true
		if child.Commands() == nil || len(child.Commands()) != 2 {
			t.Fatalf("cache subcommands = %d, want status and clear", len(child.Commands()))
		}
		var status, clear *cobra.Command
		for _, subcommand := range child.Commands() {
			switch subcommand.Name() {
			case "status":
				status = subcommand
			case "clear":
				clear = subcommand
			}
		}
		if status == nil || clear == nil {
			t.Fatalf("cache subcommands = %v, want status and clear", child.Commands())
		}
		if status.Name() != "status" || status.Flags().Lookup("json") == nil {
			t.Fatalf("status command or --json flag missing")
		}
		if clear.Name() != "clear" || clear.Flags().Lookup("all") == nil {
			t.Fatalf("clear command or --all flag missing")
		}
	}
	if !cacheCmdFound {
		t.Fatal("cache command missing from root command")
	}
}

func TestCacheDiagnosticsHelpers(t *testing.T) {
	if got := cacheDiagnosticsMode(&countOptions{}); got != "disabled" {
		t.Fatalf("cold diagnostics mode = %q, want disabled", got)
	}
	if got := cacheDiagnosticsMode(&countOptions{cache: true}); got != "metadata" {
		t.Fatalf("metadata diagnostics mode = %q, want metadata", got)
	}
	if got := cacheDiagnosticsMode(&countOptions{cache: true, cacheVerify: true}); got != "verified" {
		t.Fatalf("verified diagnostics mode = %q, want verified", got)
	}
	if got := tokenizerCalls(map[string]int64{"bpe": 2, "claude": 3}); got != 5 {
		t.Fatalf("tokenizer calls = %d, want 5", got)
	}
	reasons := map[string]int64{
		"schema_mismatch":  2,
		"content_changed":  1,
		"metadata_assumed": 4,
	}
	if got := cacheIncompatibilities(reasons); got != 3 {
		t.Fatalf("incompatibilities = %d, want 3", got)
	}
	if got := formatCacheReasons(reasons); got != "content_changed=1,metadata_assumed=4,schema_mismatch=2" {
		t.Fatalf("formatted cache reasons = %q", got)
	}
}
