package tokenizer_test

import (
	"context"
	"fmt"
	"os"

	"github.com/lancekrogers/tcount/tokenizer"
)

func ExampleNewCounter() {
	counter, err := tokenizer.NewCounter(tokenizer.CounterOptions{})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	ctx := context.Background()
	result, err := counter.Count(ctx, "Hello, world!", "gpt-4o", false)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	for _, m := range result.Methods {
		if m.IsExact {
			fmt.Printf("Tokens: %d (exact)\n", m.Tokens)
		}
	}
	// Output: Tokens: 4 (exact)
}

func ExampleCounter_Count() {
	counter, err := tokenizer.NewCounter(tokenizer.CounterOptions{})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	ctx := context.Background()
	result, err := counter.Count(ctx, "The quick brown fox jumps over the lazy dog.", "", true)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("Characters: %d\n", result.Characters)
	fmt.Printf("Words: %d\n", result.Words)
	fmt.Printf("Methods: %d\n", len(result.Methods))
	// Output:
	// Characters: 44
	// Words: 9
	// Methods: 7
}

func ExampleCounter_CountFile() {
	f, err := os.CreateTemp("", "tcount-example-*.txt")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	defer os.Remove(f.Name())

	if _, err := f.WriteString("Hello, world!"); err != nil {
		fmt.Println("error:", err)
		return
	}
	f.Close()

	counter, err := tokenizer.NewCounter(tokenizer.CounterOptions{})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	ctx := context.Background()
	result, err := counter.CountFile(ctx, f.Name(), "gpt-4o", false)
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	for _, m := range result.Methods {
		if m.IsExact {
			fmt.Printf("Tokens: %d\n", m.Tokens)
		}
	}
	// Output: Tokens: 4
}

func ExampleCounter_CountFile_error() {
	counter, err := tokenizer.NewCounter(tokenizer.CounterOptions{})
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	ctx := context.Background()
	_, err = counter.CountFile(ctx, "nonexistent.txt", "gpt-4o", false)
	if err != nil {
		fmt.Println("File not found (expected)")
	}
	// Output: File not found (expected)
}

func ExampleNewBPETokenizer() {
	tok, err := tokenizer.NewBPETokenizer("gpt-4o")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	count, err := tok.CountTokens("Hello, world!")
	if err != nil {
		fmt.Println("error:", err)
		return
	}
	fmt.Printf("Tokens: %d\n", count)
	fmt.Printf("Exact: %v\n", tok.IsExact())
	// Output:
	// Tokens: 4
	// Exact: true
}

func ExampleGetModelMetadata() {
	meta := tokenizer.GetModelMetadata("gpt-4o")
	if meta != nil {
		fmt.Printf("Model: %s\n", meta.Name)
		fmt.Printf("Provider: %s\n", meta.Provider)
		fmt.Printf("Encoding: %s\n", meta.Encoding)
		fmt.Printf("Context: %d\n", meta.ContextWindow)
	}
	// Output:
	// Model: gpt-4o
	// Provider: openai
	// Encoding: o200k_base
	// Context: 128000
}
