package integration_test

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lancekrogers/tcount/tokenizer/fileops"
)

func TestIntegrationFilesystem_WalkDirectory(t *testing.T) {
	ctx := context.Background()
	dir := fixturesDir(t) + "/walkdir"

	result, err := fileops.WalkDirectory(ctx, dir)
	if err != nil {
		t.Fatalf("WalkDirectory() error: %v", err)
	}

	// Should include hello.txt, main.go, nested/deep.txt
	expectedFiles := []string{"hello.txt", "main.go", filepath.Join("nested", "deep.txt")}
	for _, expected := range expectedFiles {
		found := false
		for _, f := range result.Files {
			if strings.HasSuffix(f, expected) {
				found = true
				break
			}
		}
		if !found {
			t.Errorf("expected file %q in walk results, got: %v", expected, result.Files)
		}
	}

	// TotalFiles should be greater than included files (includes ignored/binary)
	if result.TotalFiles < len(result.Files) {
		t.Errorf("TotalFiles (%d) should be >= included files (%d)", result.TotalFiles, len(result.Files))
	}
}

func TestIntegrationFilesystem_GitignoreRespect(t *testing.T) {
	ctx := context.Background()
	dir := fixturesDir(t) + "/walkdir"

	result, err := fileops.WalkDirectory(ctx, dir)
	if err != nil {
		t.Fatalf("WalkDirectory() error: %v", err)
	}

	ignoredFiles := []string{"ignored.txt", "debug.log", filepath.Join("build", "out.txt")}
	for _, ignored := range ignoredFiles {
		for _, f := range result.Files {
			if strings.HasSuffix(f, ignored) {
				t.Errorf("gitignored file %q should not be in results", ignored)
			}
		}
	}

	if result.SkippedIgnore == 0 {
		t.Error("expected SkippedIgnore > 0")
	}
}

func TestIntegrationFilesystem_BinaryDetection(t *testing.T) {
	ctx := context.Background()
	dir := fixturesDir(t) + "/walkdir"

	result, err := fileops.WalkDirectory(ctx, dir)
	if err != nil {
		t.Fatalf("WalkDirectory() error: %v", err)
	}

	for _, f := range result.Files {
		if strings.HasSuffix(f, "photo.png") {
			t.Error("binary file photo.png should not be in results")
		}
	}

	if result.SkippedBinary == 0 {
		t.Error("expected SkippedBinary > 0")
	}
}

func TestIntegrationFilesystem_GitDirSkip(t *testing.T) {
	ctx := context.Background()

	// Create temp dir with a .git subdirectory
	tmpDir := t.TempDir()
	gitDir := filepath.Join(tmpDir, ".git")
	if err := os.MkdirAll(gitDir, 0o755); err != nil {
		t.Fatalf("failed to create .git dir: %v", err)
	}

	// Put a file inside .git that should be skipped
	if err := os.WriteFile(filepath.Join(gitDir, "HEAD"), []byte("ref: refs/heads/main\n"), 0o644); err != nil {
		t.Fatalf("failed to write .git/HEAD: %v", err)
	}

	// Put a regular file that should be included
	if err := os.WriteFile(filepath.Join(tmpDir, "readme.txt"), []byte("Hello\n"), 0o644); err != nil {
		t.Fatalf("failed to write readme.txt: %v", err)
	}

	result, err := fileops.WalkDirectory(ctx, tmpDir)
	if err != nil {
		t.Fatalf("WalkDirectory() error: %v", err)
	}

	for _, f := range result.Files {
		if strings.Contains(f, ".git") {
			t.Errorf("file from .git directory should not be in results: %s", f)
		}
	}

	if len(result.Files) != 1 {
		t.Errorf("expected 1 file (readme.txt), got %d: %v", len(result.Files), result.Files)
	}
}

func TestIntegrationFilesystem_AggregateContents(t *testing.T) {
	ctx := context.Background()
	dir := fixturesDir(t) + "/walkdir"

	walkResult, err := fileops.WalkDirectory(ctx, dir)
	if err != nil {
		t.Fatalf("WalkDirectory() error: %v", err)
	}

	content, err := fileops.AggregateFileContents(ctx, walkResult.Files)
	if err != nil {
		t.Fatalf("AggregateFileContents() error: %v", err)
	}

	contentStr := string(content)

	// Should contain text from included files
	if !strings.Contains(contentStr, "Hello from the test directory") {
		t.Error("aggregated content should contain hello.txt content")
	}
	if !strings.Contains(contentStr, "Hello from walkdir") {
		t.Error("aggregated content should contain main.go content")
	}
	if !strings.Contains(contentStr, "Deeply nested file") {
		t.Error("aggregated content should contain nested/deep.txt content")
	}

	// Should NOT contain text from excluded files
	if strings.Contains(contentStr, "This should be ignored") {
		t.Error("aggregated content should NOT contain ignored.txt content")
	}
	if strings.Contains(contentStr, "Debug output") {
		t.Error("aggregated content should NOT contain debug.log content")
	}
	if strings.Contains(contentStr, "Build output") {
		t.Error("aggregated content should NOT contain build/out.txt content")
	}
}
