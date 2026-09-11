package integration_test

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"sort"
	"strings"
	"testing"

	"github.com/lancekrogers/tcount/tokenizer"
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

// writeWalkFile creates a file under root, making parent directories as needed.
func writeWalkFile(t *testing.T, root, relative, content string) string {
	t.Helper()
	path := filepath.Join(root, filepath.FromSlash(relative))
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatalf("failed to create directory for %q: %v", relative, err)
	}
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatalf("failed to write %q: %v", relative, err)
	}
	return path
}

// walkRelativeFiles returns the walk results as slash-separated paths relative
// to root, so assertions read like the fixture layout.
func walkRelativeFiles(t *testing.T, root string, files []string) []string {
	t.Helper()
	relatives := make([]string, 0, len(files))
	for _, file := range files {
		relative, err := filepath.Rel(root, file)
		if err != nil {
			t.Fatalf("failed to relativize %q: %v", file, err)
		}
		relatives = append(relatives, filepath.ToSlash(relative))
	}
	sort.Strings(relatives)
	return relatives
}

// nestedIgnoreTree builds a directory whose subdirectory carries its own
// .gitignore, including a negation and a rule that excludes a whole directory.
func nestedIgnoreTree(t *testing.T) string {
	t.Helper()
	root := t.TempDir()

	writeWalkFile(t, root, ".gitignore", "*.log\n")
	writeWalkFile(t, root, "keep.txt", "root text\n")
	writeWalkFile(t, root, "app.log", "root log\n")

	writeWalkFile(t, root, "sub/.gitignore", "models/\n*.test\n!keep.test\n")
	writeWalkFile(t, root, "sub/sub.txt", "sub text\n")
	writeWalkFile(t, root, "sub/keep.test", "negated back in\n")
	writeWalkFile(t, root, "sub/drop.test", "excluded by nested rule\n")
	writeWalkFile(t, root, "sub/nested.log", "excluded by root rule\n")
	writeWalkFile(t, root, "sub/models/weights.txt", "never visited\n")

	writeWalkFile(t, root, "other/other.test", "nested rule does not reach here\n")

	return root
}

func TestIntegrationFilesystem_NestedGitignoreScopesRules(t *testing.T) {
	ctx := context.Background()
	root := nestedIgnoreTree(t)

	result, err := fileops.WalkDirectory(ctx, root)
	if err != nil {
		t.Fatalf("WalkDirectory() error: %v", err)
	}

	got := walkRelativeFiles(t, root, result.Files)
	want := []string{
		".gitignore",
		"keep.txt",
		"other/other.test",
		"sub/.gitignore",
		"sub/keep.test",
		"sub/sub.txt",
	}
	if !slices.Equal(got, want) {
		t.Fatalf("walked files = %v, want %v", got, want)
	}
}

func TestIntegrationFilesystem_NestedGitignoreSkipsIgnoredDirectory(t *testing.T) {
	ctx := context.Background()
	root := nestedIgnoreTree(t)

	result, err := fileops.WalkDirectory(ctx, root)
	if err != nil {
		t.Fatalf("WalkDirectory() error: %v", err)
	}

	// sub/models is excluded by the nested rule, so the walk must never
	// descend into it: its contents are absent from TotalFiles entirely.
	// Visited files are app.log, keep.txt, .gitignore, sub/.gitignore,
	// sub/sub.txt, sub/keep.test, sub/drop.test, sub/nested.log,
	// other/other.test.
	const wantTotal = 9
	if result.TotalFiles != wantTotal {
		t.Errorf("TotalFiles = %d, want %d (an ignored directory must not be descended into)",
			result.TotalFiles, wantTotal)
	}

	// app.log, sub/drop.test, sub/nested.log, and the sub/models directory.
	const wantSkippedIgnore = 4
	if result.SkippedIgnore != wantSkippedIgnore {
		t.Errorf("SkippedIgnore = %d, want %d", result.SkippedIgnore, wantSkippedIgnore)
	}
}

func TestIntegrationFilesystem_MaxFileSizeSkipsLargeFiles(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()

	const sizeCap = 1024
	writeWalkFile(t, root, "small.txt", "well under the cap\n")
	writeWalkFile(t, root, "big.txt", strings.Repeat("oversized content\n", 200))

	result, err := fileops.WalkDirectoryWithOptions(ctx, root, fileops.WalkOptions{MaxFileSize: sizeCap})
	if err != nil {
		t.Fatalf("WalkDirectoryWithOptions() error: %v", err)
	}

	got := walkRelativeFiles(t, root, result.Files)
	if !slices.Equal(got, []string{"small.txt"}) {
		t.Fatalf("walked files = %v, want [small.txt]", got)
	}
	if result.SkippedLarge != 1 {
		t.Errorf("SkippedLarge = %d, want 1", result.SkippedLarge)
	}
	if result.TotalFiles != 2 {
		t.Errorf("TotalFiles = %d, want 2", result.TotalFiles)
	}
}

func TestIntegrationFilesystem_MaxFileSizeFollowsSymlinks(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()

	const sizeCap = 1024
	writeWalkFile(t, root, "small.txt", "well under the cap\n")
	target := writeWalkFile(t, root, "big.txt", strings.Repeat("oversized content\n", 200))

	link := filepath.Join(root, "link.txt")
	if err := os.Symlink(target, link); err != nil {
		t.Skipf("symlinks unavailable on this platform: %v", err)
	}

	result, err := fileops.WalkDirectoryWithOptions(ctx, root, fileops.WalkOptions{MaxFileSize: sizeCap})
	if err != nil {
		t.Fatalf("WalkDirectoryWithOptions() error: %v", err)
	}

	got := walkRelativeFiles(t, root, result.Files)
	if !slices.Equal(got, []string{"small.txt"}) {
		t.Fatalf("walked files = %v, want [small.txt]; a symlink must be measured by its target", got)
	}
	if result.SkippedLarge != 2 {
		t.Errorf("SkippedLarge = %d, want 2 (big.txt and the link to it)", result.SkippedLarge)
	}
}

func TestIntegrationFilesystem_ZeroMaxFileSizeMeansNoLimit(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()

	writeWalkFile(t, root, "small.txt", "well under any cap\n")
	writeWalkFile(t, root, "big.txt", strings.Repeat("oversized content\n", 200))

	result, err := fileops.WalkDirectoryWithOptions(ctx, root, fileops.WalkOptions{})
	if err != nil {
		t.Fatalf("WalkDirectoryWithOptions() error: %v", err)
	}

	got := walkRelativeFiles(t, root, result.Files)
	if !slices.Equal(got, []string{"big.txt", "small.txt"}) {
		t.Fatalf("walked files = %v, want [big.txt small.txt]", got)
	}
	if result.SkippedLarge != 0 {
		t.Errorf("SkippedLarge = %d, want 0", result.SkippedLarge)
	}
}

func TestIntegrationFilesystem_ModelWeightsAreSkippedAsBinary(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()

	// Protobuf framing interleaved with tensor names, carrying no null byte.
	header := "\x08\x07\x12\x07pytorch\x1a\x052.6.0\x3a\xc9\xe3\xa2\x9b\x01\x0a\x8b\x01\x0a\x37" +
		"kmodel.decoder.generator.noise_res."
	writeWalkFile(t, root, "README.md", "# Model directory\n")
	writeWalkFile(t, root, "model.onnx", strings.Repeat(header, 64))
	writeWalkFile(t, root, "weights.data", strings.Repeat(header, 64))

	result, err := fileops.WalkDirectory(ctx, root)
	if err != nil {
		t.Fatalf("WalkDirectory() error: %v", err)
	}

	got := walkRelativeFiles(t, root, result.Files)
	if !slices.Equal(got, []string{"README.md"}) {
		t.Fatalf("walked files = %v, want [README.md]; model weights must be skipped as binary", got)
	}
	if result.SkippedBinary != 2 {
		t.Errorf("SkippedBinary = %d, want 2", result.SkippedBinary)
	}
}

func TestIntegrationFilesystem_CountDirectoryHonorsMaxFileSize(t *testing.T) {
	ctx := context.Background()
	root := t.TempDir()

	const sizeCap = 1024
	writeWalkFile(t, root, "small.txt", "alpha beta gamma\n")
	writeWalkFile(t, root, "big.txt", strings.Repeat("oversized content\n", 200))

	counter, err := tokenizer.NewCounter(tokenizer.CounterOptions{})
	if err != nil {
		t.Fatalf("NewCounter() error: %v", err)
	}

	capped, err := counter.CountDirectoryWithOptions(ctx, root, tokenizer.CountDirectoryOptions{
		Model:       "gpt-4o",
		MaxFileSize: sizeCap,
	})
	if err != nil {
		t.Fatalf("CountDirectoryWithOptions() error: %v", err)
	}

	uncapped, err := counter.CountDirectory(ctx, root, "gpt-4o", false)
	if err != nil {
		t.Fatalf("CountDirectory() error: %v", err)
	}

	if capped.Characters != len("alpha beta gamma\n") {
		t.Errorf("capped Characters = %d, want %d", capped.Characters, len("alpha beta gamma\n"))
	}
	if uncapped.Characters <= capped.Characters {
		t.Errorf("uncapped Characters = %d, want more than the capped %d",
			uncapped.Characters, capped.Characters)
	}
	if capped.FileCount != 1 {
		t.Errorf("capped FileCount = %d, want 1", capped.FileCount)
	}
	if uncapped.FileCount != 2 {
		t.Errorf("uncapped FileCount = %d, want 2", uncapped.FileCount)
	}
}
