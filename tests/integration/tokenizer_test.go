package integration_test

import (
	"context"
	"os"
	"testing"

	"github.com/lancekrogers/tcount/tokenizer"
)

func TestIntegrationTokenizer_O200kBase(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"hello_world", "Hello, world!", 4},
		{"pangram", "The quick brown fox jumps over the lazy dog.", 10},
		{"sample_txt", readFixture(t, "sample.txt"), 30},
		{"sample_go", readFixture(t, "sample.go"), 19},
		{"unicode", readFixture(t, "unicode.txt"), 18},
	}

	counter, err := tokenizer.NewCounter(tokenizer.CounterOptions{})
	if err != nil {
		t.Fatalf("NewCounter() error: %v", err)
	}
	ctx := context.Background()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := counter.Count(ctx, tc.text, "gpt-4o", false)
			if err != nil {
				t.Fatalf("Count() error: %v", err)
			}

			var tiktokenCount int
			for _, m := range result.Methods {
				if m.IsExact {
					tiktokenCount = m.Tokens
					break
				}
			}

			if tiktokenCount != tc.expected {
				t.Errorf("o200k_base token count: got %d, want %d", tiktokenCount, tc.expected)
			}
		})
	}
}

func TestIntegrationTokenizer_Cl100kBase(t *testing.T) {
	tests := []struct {
		name     string
		text     string
		expected int
	}{
		{"hello_world", "Hello, world!", 4},
		{"pangram", "The quick brown fox jumps over the lazy dog.", 10},
		{"sample_txt", readFixture(t, "sample.txt"), 30},
		{"sample_go", readFixture(t, "sample.go"), 19},
		{"unicode", readFixture(t, "unicode.txt"), 24},
	}

	counter, err := tokenizer.NewCounter(tokenizer.CounterOptions{})
	if err != nil {
		t.Fatalf("NewCounter() error: %v", err)
	}
	ctx := context.Background()

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			result, err := counter.Count(ctx, tc.text, "gpt-4", false)
			if err != nil {
				t.Fatalf("Count() error: %v", err)
			}

			var tiktokenCount int
			for _, m := range result.Methods {
				if m.IsExact {
					tiktokenCount = m.Tokens
					break
				}
			}

			if tiktokenCount != tc.expected {
				t.Errorf("cl100k_base token count: got %d, want %d", tiktokenCount, tc.expected)
			}
		})
	}
}

func TestIntegrationTokenizer_AllMethods(t *testing.T) {
	text := readFixture(t, "sample.txt")
	counter, err := tokenizer.NewCounter(tokenizer.CounterOptions{})
	if err != nil {
		t.Fatalf("NewCounter() error: %v", err)
	}
	ctx := context.Background()

	result, err := counter.Count(ctx, text, "", true)
	if err != nil {
		t.Fatalf("Count() error: %v", err)
	}

	if len(result.Methods) < 3 {
		t.Errorf("expected at least 3 methods (tiktoken + approximations), got %d", len(result.Methods))
	}

	for _, m := range result.Methods {
		if m.Tokens <= 0 {
			t.Errorf("method %q returned %d tokens, expected > 0", m.Name, m.Tokens)
		}
	}
}

func TestIntegrationTokenizer_Approximations(t *testing.T) {
	text := readFixture(t, "sample.txt")
	counter, err := tokenizer.NewCounter(tokenizer.CounterOptions{})
	if err != nil {
		t.Fatalf("NewCounter() error: %v", err)
	}
	ctx := context.Background()

	result, err := counter.Count(ctx, text, "", true)
	if err != nil {
		t.Fatalf("Count() error: %v", err)
	}

	// Find exact tiktoken count for reference
	var exactCount int
	for _, m := range result.Methods {
		if m.IsExact {
			exactCount = m.Tokens
			break
		}
	}

	if exactCount == 0 {
		t.Fatal("no exact tokenizer method found")
	}

	// Check approximations are within 5x of exact count (generous range for approximations)
	for _, m := range result.Methods {
		if m.IsExact {
			continue
		}
		ratio := float64(m.Tokens) / float64(exactCount)
		if ratio < 0.2 || ratio > 5.0 {
			t.Errorf("approximation %q (%d tokens) is outside reasonable range of exact count (%d), ratio=%.2f",
				m.Name, m.Tokens, exactCount, ratio)
		}
	}
}

// readFixture reads a file from the testdata directory.
func readFixture(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(fixturesDir(t) + "/" + name)
	if err != nil {
		t.Fatalf("failed to read fixture %q: %v", name, err)
	}
	return string(data)
}
