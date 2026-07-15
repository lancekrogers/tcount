//go:build container

package cachefs

import (
	"context"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/lancekrogers/tcount/internal/cache"
)

func TestTimestampPreservingMutationMetadataFalseHitVerifiedMiss(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "sample.txt")
	original := []byte("alpha\n")
	mutated := []byte("bravo\n")
	if err := os.WriteFile(path, original, 0o644); err != nil {
		t.Fatal(err)
	}
	info, err := os.Stat(path)
	if err != nil {
		t.Fatal(err)
	}
	entry, err := cache.CaptureEntry(context.Background(), root, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, mutated, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, info.ModTime(), info.ModTime()); err != nil {
		t.Fatal(err)
	}

	metadata, err := cache.NewValidator(cache.Metadata)
	if err != nil {
		t.Fatal(err)
	}
	metadataResult, err := metadata.Validate(context.Background(), root, path, entry, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !metadataResult.Hit {
		t.Fatal("metadata mode did not reproduce the timestamp-preserving false hit")
	}

	verified, err := cache.NewValidator(cache.Verified)
	if err != nil {
		t.Fatal(err)
	}
	verifiedResult, err := verified.Validate(context.Background(), root, path, entry, nil)
	if err != nil {
		t.Fatal(err)
	}
	if verifiedResult.Hit {
		t.Fatal("verified mode accepted timestamp-preserving content mutation")
	}
}

func TestCoarseTimestampMutationHasSameModeTradeoff(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "coarse.txt")
	coarse := time.Unix(1_700_000_000, 0)
	if err := os.WriteFile(path, []byte("111111\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, coarse, coarse); err != nil {
		t.Fatal(err)
	}
	entry, err := cache.CaptureEntry(context.Background(), root, path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("222222\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chtimes(path, coarse, coarse); err != nil {
		t.Fatal(err)
	}

	metadata, _ := cache.NewValidator(cache.Metadata)
	metadataResult, err := metadata.Validate(context.Background(), root, path, entry, nil)
	if err != nil {
		t.Fatal(err)
	}
	verified, _ := cache.NewValidator(cache.Verified)
	verifiedResult, err := verified.Validate(context.Background(), root, path, entry, nil)
	if err != nil {
		t.Fatal(err)
	}
	if !metadataResult.Hit || verifiedResult.Hit {
		t.Fatalf("coarse timestamp results metadata=%t verified=%t", metadataResult.Hit, verifiedResult.Hit)
	}
}

func TestVerifiedModeReadsAndMetadataModeDoesNot(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "stats.txt")
	if err := os.WriteFile(path, []byte("stable content\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	entry, err := cache.CaptureEntry(context.Background(), root, path)
	if err != nil {
		t.Fatal(err)
	}

	metadata, _ := cache.NewValidator(cache.Metadata)
	metadataStats := &cache.ValidationStats{}
	if _, err := metadata.Validate(context.Background(), root, path, entry, metadataStats); err != nil {
		t.Fatal(err)
	}
	verified, _ := cache.NewValidator(cache.Verified)
	verifiedStats := &cache.ValidationStats{}
	if _, err := verified.Validate(context.Background(), root, path, entry, verifiedStats); err != nil {
		t.Fatal(err)
	}
	if got := metadataStats.Snapshot(); got.FullReads != 0 || got.BytesRead != 0 || got.Hits != 1 {
		t.Fatalf("metadata work = %+v, want one hit and no read", got)
	}
	if got := verifiedStats.Snapshot(); got.FullReads != 1 || got.BytesRead != int64(len("stable content\n")) || got.Hits != 1 {
		t.Fatalf("verified work = %+v, want one full read and one hit", got)
	}
}
