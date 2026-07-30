package tokenizer

import (
	"context"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"testing"
)

func TestCountFilesWithOptionsReportsProgressPerFile(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	paths := []string{
		writeTempFile(t, dir, "a.txt", "hello world one"),
		writeTempFile(t, dir, "b.txt", "hello world two three"),
		writeTempFile(t, dir, "c.txt", "four five"),
	}

	counter, err := NewCounter(CounterOptions{})
	if err != nil {
		t.Fatalf("NewCounter: %v", err)
	}

	var (
		mu      sync.Mutex
		updates []ProgressUpdate
	)
	result, err := counter.CountFilesWithOptions(context.Background(), paths, CountFilesOptions{
		Model: "gpt-5",
		OnProgress: func(u ProgressUpdate) {
			mu.Lock()
			updates = append(updates, u)
			mu.Unlock()
		},
	})
	if err != nil {
		t.Fatalf("CountFilesWithOptions: %v", err)
	}
	if result == nil || len(result.Methods) == 0 {
		t.Fatal("expected method results")
	}

	mu.Lock()
	defer mu.Unlock()
	if len(updates) != len(paths) {
		t.Fatalf("progress updates = %d, want %d", len(updates), len(paths))
	}

	seen := make(map[int]bool)
	var lastChars, lastTokens int
	for i, u := range updates {
		if u.FilesTotal != len(paths) {
			t.Fatalf("update %d FilesTotal = %d, want %d", i, u.FilesTotal, len(paths))
		}
		if u.FilesDone < 1 || u.FilesDone > len(paths) {
			t.Fatalf("update %d FilesDone = %d out of range", i, u.FilesDone)
		}
		if seen[u.FilesDone] {
			t.Fatalf("duplicate FilesDone %d", u.FilesDone)
		}
		seen[u.FilesDone] = true
		if u.Characters < lastChars {
			t.Fatalf("characters decreased: %d -> %d", lastChars, u.Characters)
		}
		lastChars = u.Characters
		if len(u.MethodTokens) == 0 {
			t.Fatalf("update %d missing MethodTokens", i)
		}
		if u.MethodTokens[0] < lastTokens {
			t.Fatalf("tokens decreased: %d -> %d", lastTokens, u.MethodTokens[0])
		}
		lastTokens = u.MethodTokens[0]
		if u.LastPath == "" {
			t.Fatalf("update %d empty LastPath", i)
		}
	}
	if !seen[len(paths)] {
		t.Fatal("never saw FilesDone == total")
	}
	if lastChars != result.Characters {
		t.Fatalf("final progress chars = %d, result chars = %d", lastChars, result.Characters)
	}
	if lastTokens != result.Methods[0].Tokens {
		t.Fatalf("final progress tokens = %d, result tokens = %d", lastTokens, result.Methods[0].Tokens)
	}
}

func TestCountFilesNilProgressMatchesPlain(t *testing.T) {
	t.Parallel()

	dir := t.TempDir()
	paths := []string{
		writeTempFile(t, dir, "a.txt", "alpha beta gamma"),
		writeTempFile(t, dir, "b.txt", "delta epsilon"),
	}
	counter, err := NewCounter(CounterOptions{})
	if err != nil {
		t.Fatalf("NewCounter: %v", err)
	}

	plain, err := counter.CountFiles(context.Background(), paths, "gpt-5", false)
	if err != nil {
		t.Fatalf("CountFiles: %v", err)
	}
	withNil, err := counter.CountFilesWithOptions(context.Background(), paths, CountFilesOptions{
		Model:      "gpt-5",
		OnProgress: nil,
	})
	if err != nil {
		t.Fatalf("CountFilesWithOptions: %v", err)
	}
	if plain.Characters != withNil.Characters || plain.Words != withNil.Words || plain.Lines != withNil.Lines {
		t.Fatalf("stats mismatch plain=%+v withNil=%+v", plain, withNil)
	}
	if len(plain.Methods) != len(withNil.Methods) {
		t.Fatalf("method count mismatch")
	}
	for i := range plain.Methods {
		if plain.Methods[i].Tokens != withNil.Methods[i].Tokens {
			t.Fatalf("method %d tokens plain=%d withNil=%d", i, plain.Methods[i].Tokens, withNil.Methods[i].Tokens)
		}
	}
}

func TestProgressTrackerConcurrentAdd(t *testing.T) {
	t.Parallel()

	var calls atomic.Int64
	tracker := newProgressTracker(func(ProgressUpdate) {
		calls.Add(1)
	}, 100, 2)

	var wg sync.WaitGroup
	for i := 0; i < 100; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			r := perFileResult{
				FileSize:      10,
				Characters:    10,
				Words:         2,
				Lines:         1,
				Methods:       []MethodResult{{Tokens: 3}, {Tokens: 4}},
				MethodPresent: []bool{true, true},
			}
			tracker.add(filepath.Join("f", string(rune('a'+i%26))), r)
		}(i)
	}
	wg.Wait()
	if got := calls.Load(); got != 100 {
		t.Fatalf("callbacks = %d, want 100", got)
	}
}

func writeTempFile(t *testing.T, dir, name, content string) string {
	t.Helper()
	path := filepath.Join(dir, name)
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("write %s: %v", path, err)
	}
	return path
}
