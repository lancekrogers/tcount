package integration_test

import (
	"context"
	"errors"
	"testing"

	"github.com/lancekrogers/tcount/tokenizer"
	"github.com/lancekrogers/tcount/tokenizer/fileops"
)

// cancelAfterCtx reports Canceled after its budget of Err() calls is spent,
// making mid-loop cancellation deterministic without goroutine timing.
type cancelAfterCtx struct {
	context.Context
	remaining int
}

func (c *cancelAfterCtx) Err() error {
	if c.remaining <= 0 {
		return context.Canceled
	}
	c.remaining--
	return nil
}

func TestIntegrationCancellation_WalkDirectoryPreCanceled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := fileops.WalkDirectory(ctx, fixturesDir(t)+"/walkdir"); !errors.Is(err, context.Canceled) {
		t.Fatalf("WalkDirectory() error = %v, want context.Canceled", err)
	}
}

func TestIntegrationCancellation_WalkDirectoryMidWalk(t *testing.T) {
	ctx := &cancelAfterCtx{Context: context.Background(), remaining: 2}

	if _, err := fileops.WalkDirectory(ctx, fixturesDir(t)+"/walkdir"); !errors.Is(err, context.Canceled) {
		t.Fatalf("WalkDirectory() error = %v, want context.Canceled", err)
	}
}

func TestIntegrationCancellation_AggregateFileContentsPreCanceled(t *testing.T) {
	walk, err := fileops.WalkDirectory(context.Background(), fixturesDir(t)+"/walkdir")
	if err != nil {
		t.Fatal(err)
	}
	if len(walk.Files) == 0 {
		t.Fatal("walkdir fixture returned no files")
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := fileops.AggregateFileContents(ctx, walk.Files); !errors.Is(err, context.Canceled) {
		t.Fatalf("AggregateFileContents() error = %v, want context.Canceled", err)
	}
}

func TestIntegrationCancellation_AggregateFileContentsMidRead(t *testing.T) {
	walk, err := fileops.WalkDirectory(context.Background(), fixturesDir(t)+"/walkdir")
	if err != nil {
		t.Fatal(err)
	}
	if len(walk.Files) == 0 {
		t.Fatal("walkdir fixture returned no files")
	}

	ctx := &cancelAfterCtx{Context: context.Background(), remaining: 1}

	if _, err := fileops.AggregateFileContents(ctx, walk.Files); !errors.Is(err, context.Canceled) {
		t.Fatalf("AggregateFileContents() error = %v, want context.Canceled", err)
	}
}

func TestIntegrationCancellation_CountDirectory(t *testing.T) {
	counter, err := tokenizer.NewCounter(tokenizer.CounterOptions{})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := counter.CountDirectory(ctx, fixturesDir(t)+"/walkdir", "", false); !errors.Is(err, context.Canceled) {
		t.Fatalf("CountDirectory() error = %v, want context.Canceled", err)
	}
}

func TestIntegrationCancellation_CountFile(t *testing.T) {
	counter, err := tokenizer.NewCounter(tokenizer.CounterOptions{})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := counter.CountFile(ctx, fixturesDir(t)+"/sample.txt", "", false); !errors.Is(err, context.Canceled) {
		t.Fatalf("CountFile() error = %v, want context.Canceled", err)
	}
}

func TestIntegrationCountDirectory_PerFileSum(t *testing.T) {
	ctx := context.Background()
	counter, err := tokenizer.NewCounter(tokenizer.CounterOptions{})
	if err != nil {
		t.Fatal(err)
	}

	dir := fixturesDir(t) + "/walkdir"
	walk, err := fileops.WalkDirectory(ctx, dir)
	if err != nil {
		t.Fatal(err)
	}

	dirResult, err := counter.CountDirectory(ctx, dir, "gpt-5", false)
	if err != nil {
		t.Fatal(err)
	}

	sumTokens, sumWords, sumBytes := 0, 0, 0
	for _, f := range walk.Files {
		fr, err := counter.CountFile(ctx, f, "gpt-5", false)
		if err != nil {
			t.Fatalf("CountFile(%s): %v", f, err)
		}
		sumTokens += fr.Methods[0].Tokens
		sumWords += fr.Words
		sumBytes += fr.FileSize
	}

	if got := dirResult.Methods[0].Tokens; got != sumTokens {
		t.Errorf("directory tokens = %d, want per-file sum %d", got, sumTokens)
	}
	if dirResult.Words != sumWords {
		t.Errorf("directory words = %d, want per-file sum %d", dirResult.Words, sumWords)
	}
	if dirResult.FileSize != sumBytes {
		t.Errorf("directory FileSize = %d, want per-file sum %d", dirResult.FileSize, sumBytes)
	}
	if dirResult.FileCount != len(walk.Files) {
		t.Errorf("FileCount = %d, want %d", dirResult.FileCount, len(walk.Files))
	}
}
