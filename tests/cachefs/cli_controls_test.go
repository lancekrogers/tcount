//go:build container

package cachefs

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"testing"

	"github.com/lancekrogers/tcount/internal/cache"
	"github.com/lancekrogers/tcount/tokenizer"
)

const testCacheDirEnvironment = "TCOUNT_CACHE_DIR"

func TestDirectoryCacheVerboseDiagnosticsInContainer(t *testing.T) {
	root := t.TempDir()
	cacheBase := t.TempDir()
	writeTextFile(t, root, "one.txt", "alpha beta\n")
	writeTextFile(t, root, "two.txt", "gamma delta\n")

	first := runContainerTcount(t, cacheBase, "--no-color", "--verbose", "--json", "--directory", "--cache", root)
	if first.exitCode != 0 {
		t.Fatalf("cold cached count exit code = %d, stderr: %s", first.exitCode, first.stderr)
	}
	var firstResult tokenizer.CountResult
	if err := json.Unmarshal(first.stdout, &firstResult); err != nil {
		t.Fatalf("cold cached JSON = %q: %v", first.stdout, err)
	}
	if strings.Contains(string(first.stdout), "Cache diagnostics") {
		t.Fatalf("cold cached diagnostics leaked into stdout: %s", first.stdout)
	}
	for _, field := range []string{"Cache diagnostics: mode=metadata", "misses=2", "incompatibilities=2", "tokenizer_calls=", "stages=walk:"} {
		if !strings.Contains(first.stderr, field) {
			t.Errorf("cold cached stderr missing %q:\n%s", field, first.stderr)
		}
	}

	warm := runContainerTcount(t, cacheBase, "--no-color", "--verbose", "--json", "--directory", "--cache", root)
	if warm.exitCode != 0 {
		t.Fatalf("warm cached count exit code = %d, stderr: %s", warm.exitCode, warm.stderr)
	}
	var warmResult tokenizer.CountResult
	if err := json.Unmarshal(warm.stdout, &warmResult); err != nil {
		t.Fatalf("warm cached JSON = %q: %v", warm.stdout, err)
	}
	if !strings.Contains(warm.stderr, "hits=2") || !strings.Contains(warm.stderr, "misses=0") || !strings.Contains(warm.stderr, "reused_bytes=") || !strings.Contains(warm.stderr, "tokenizer_calls=0") {
		t.Fatalf("warm cached diagnostics = %s", warm.stderr)
	}
	if string(first.stdout) != string(warm.stdout) {
		t.Fatalf("cached stdout changed between cold and warm runs:\ncold=%s\nwarm=%s", first.stdout, warm.stdout)
	}
}

func TestDirectoryCacheControlsInContainer(t *testing.T) {
	root := t.TempDir()
	cacheBase := t.TempDir()
	writeTextFile(t, root, "one.txt", "alpha beta\n")
	writeTextFile(t, root, "two.txt", "gamma delta\n")

	cached := runContainerTcount(t, cacheBase, "--no-color", "--json", "--directory", "--cache", root)
	if cached.exitCode != 0 {
		t.Fatalf("cached count exit code = %d, stderr: %s", cached.exitCode, cached.stderr)
	}
	var cachedResult tokenizer.CountResult
	if err := json.Unmarshal(cached.stdout, &cachedResult); err != nil {
		t.Fatalf("cached JSON = %q: %v", cached.stdout, err)
	}
	if !cachedResult.IsDirectory || cachedResult.FileCount != 2 {
		t.Fatalf("cached result = %+v", cachedResult)
	}
	resolver, err := cache.NewLocationResolverAt(cacheBase)
	if err != nil {
		t.Fatal(err)
	}
	status, err := cache.NewFileStore(resolver).Status(context.Background(), root)
	if err != nil || !status.Present || status.Entries != 2 {
		t.Fatalf("cache status after opt-in count = %+v, error %v", status, err)
	}

	statusCLI := runContainerTcount(t, cacheBase, "--no-color", "cache", "status", "--json", root)
	if statusCLI.exitCode != 0 {
		t.Fatalf("cache status exit code = %d, stderr: %s", statusCLI.exitCode, statusCLI.stderr)
	}
	var statusReport struct {
		Root       string `json:"root"`
		Present    bool   `json:"present"`
		Entries    int    `json:"entries"`
		Generation uint64 `json:"generation"`
	}
	if err := json.Unmarshal(statusCLI.stdout, &statusReport); err != nil {
		t.Fatalf("cache status JSON = %q: %v", statusCLI.stdout, err)
	}
	canonicalRoot, err := cache.CanonicalRoot(root)
	if err != nil {
		t.Fatal(err)
	}
	if statusReport.Root != canonicalRoot || !statusReport.Present || statusReport.Entries != 2 || statusReport.Generation == 0 {
		t.Fatalf("cache status report = %+v", statusReport)
	}

	clearCLI := runContainerTcount(t, cacheBase, "--no-color", "cache", "clear", root)
	if clearCLI.exitCode != 0 || !strings.Contains(string(clearCLI.stdout), canonicalRoot) {
		t.Fatalf("cache clear result: exit=%d stdout=%q stderr=%q", clearCLI.exitCode, clearCLI.stdout, clearCLI.stderr)
	}
	statusAfterClear := runContainerTcount(t, cacheBase, "--no-color", "cache", "status", "--json", root)
	if statusAfterClear.exitCode != 0 {
		t.Fatalf("status after clear exit code = %d, stderr: %s", statusAfterClear.exitCode, statusAfterClear.stderr)
	}
	if err := json.Unmarshal(statusAfterClear.stdout, &statusReport); err != nil {
		t.Fatalf("status after clear JSON = %q: %v", statusAfterClear.stdout, err)
	}
	if statusReport.Present {
		t.Fatalf("status after clear = %+v, want absent", statusReport)
	}
	statusHuman := runContainerTcount(t, cacheBase, "--no-color", "cache", "status", root)
	if statusHuman.exitCode != 0 || !strings.Contains(string(statusHuman.stdout), "Cache Status") || !strings.Contains(string(statusHuman.stdout), "Present: false") {
		t.Fatalf("human cache status result: exit=%d stdout=%q stderr=%q", statusHuman.exitCode, statusHuman.stdout, statusHuman.stderr)
	}

	verified := runContainerTcount(t, cacheBase, "--no-color", "--json", "--directory", "--cache", "--cache-verify", root)
	if verified.exitCode != 0 {
		t.Fatalf("verified count exit code = %d, stderr: %s", verified.exitCode, verified.stderr)
	}

	// A regular file is an invalid cache override. The true bypass must not
	// inspect or create cache state, so it still succeeds with that override.
	bypassed := runContainerTcount(t, "/workspace/bin/tcount", "--no-color", "--json", "--directory", "--no-cache", root)
	if bypassed.exitCode != 0 {
		t.Fatalf("cold bypass exit code = %d, stderr: %s", bypassed.exitCode, bypassed.stderr)
	}
	var bypassedResult tokenizer.CountResult
	if err := json.Unmarshal(bypassed.stdout, &bypassedResult); err != nil {
		t.Fatalf("bypass JSON = %q: %v", bypassed.stdout, err)
	}
	if bypassedResult.Methods[0].Tokens != cachedResult.Methods[0].Tokens {
		t.Fatalf("bypass result = %+v, cached result = %+v", bypassedResult, cachedResult)
	}

	assertContainerCLIError(t, cacheBase, "--directory", "--cache", "--no-cache", root, "cannot be used together")
	assertContainerCLIError(t, cacheBase, "--directory", "--cache-verify", root, "requires --cache")
	assertContainerCLIError(t, cacheBase, "--cache", filepath.Join(root, "one.txt"), "recursive directory")
	assertContainerCLIError(t, cacheBase, "--directory", "--cache", filepath.Join(root, "missing"), "accessing path")
	assertContainerCLIError(t, "/workspace/bin/tcount", "--directory", "--cache", root, "must name a directory")
	assertContainerCLIError(t, cacheBase, "cache", "clear", "--all", root, "cannot be combined")

	secondRoot := t.TempDir()
	writeTextFile(t, secondRoot, "other.txt", "other cache root\n")
	second := runContainerTcount(t, cacheBase, "--no-color", "--json", "--directory", "--cache", secondRoot)
	if second.exitCode != 0 {
		t.Fatalf("second cached count exit code = %d, stderr: %s", second.exitCode, second.stderr)
	}
	clearAll := runContainerTcount(t, cacheBase, "--no-color", "cache", "clear", "--all")
	if clearAll.exitCode != 0 || !strings.Contains(string(clearAll.stdout), "all tcount cache state") {
		t.Fatalf("cache clear --all result: exit=%d stdout=%q stderr=%q", clearAll.exitCode, clearAll.stdout, clearAll.stderr)
	}
	secondStatus := runContainerTcount(t, cacheBase, "--no-color", "cache", "status", "--json", secondRoot)
	if secondStatus.exitCode != 0 {
		t.Fatalf("status after clear --all exit code = %d, stderr: %s", secondStatus.exitCode, secondStatus.stderr)
	}
	if err := json.Unmarshal(secondStatus.stdout, &statusReport); err != nil {
		t.Fatalf("status after clear --all JSON = %q: %v", secondStatus.stdout, err)
	}
	if statusReport.Present {
		t.Fatalf("status after clear --all = %+v, want absent", statusReport)
	}
}

type containerCLIResult struct {
	stdout   []byte
	stderr   string
	exitCode int
}

func runContainerTcount(t *testing.T, cacheDir string, args ...string) containerCLIResult {
	t.Helper()
	cmd := exec.Command("/workspace/bin/tcount", args...)
	cmd.Env = envWithCacheDir(cacheDir)
	var stdout, stderr bytes.Buffer
	cmd.Stdout = &stdout
	cmd.Stderr = &stderr
	err := cmd.Run()
	result := containerCLIResult{stdout: stdout.Bytes(), stderr: stderr.String()}
	if err == nil {
		return result
	}
	var exitErr *exec.ExitError
	if errors.As(err, &exitErr) {
		result.exitCode = exitErr.ExitCode()
		return result
	}
	t.Fatal(err)
	return result
}

func assertContainerCLIError(t *testing.T, cacheDir string, args ...string) {
	t.Helper()
	want := args[len(args)-1]
	args = args[:len(args)-1]
	result := runContainerTcount(t, cacheDir, args...)
	if result.exitCode == 0 {
		t.Fatalf("expected CLI error for %v, stdout: %s", args, result.stdout)
	}
	if !strings.Contains(strings.ToLower(result.stderr), strings.ToLower(want)) {
		t.Fatalf("CLI stderr for %v = %q, want %q", args, result.stderr, want)
	}
}

func envWithCacheDir(cacheDir string) []string {
	env := make([]string, 0, len(os.Environ())+1)
	for _, value := range os.Environ() {
		if strings.HasPrefix(value, testCacheDirEnvironment+"=") {
			continue
		}
		env = append(env, value)
	}
	return append(env, testCacheDirEnvironment+"="+cacheDir)
}
