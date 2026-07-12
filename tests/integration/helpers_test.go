package integration_test

import (
	"bytes"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"testing"

	"github.com/lancekrogers/tcount/tokenizer"
)

var binaryPath string

func TestMain(m *testing.M) {
	tmpDir, err := os.MkdirTemp("", "tcount-integration-*")
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to create temp dir: %v\n", err)
		os.Exit(1)
	}
	bin := filepath.Join(tmpDir, "tcount")
	if runtime.GOOS == "windows" {
		bin += ".exe"
	}

	cmd := exec.Command("go", "build", "-o", bin, "./cmd/tcount")
	cmd.Dir = projectRoot()
	out, err := cmd.CombinedOutput()
	if err != nil {
		fmt.Fprintf(os.Stderr, "failed to build tcount: %v\n%s\n", err, out)
		_ = os.RemoveAll(tmpDir)
		os.Exit(1)
	}

	binaryPath = bin
	code := m.Run()
	_ = os.RemoveAll(tmpDir)
	os.Exit(code)
}

// runTcount executes the tcount binary with the given arguments and returns
// stdout, stderr, and exit code.
func runTcount(t *testing.T, args ...string) (stdout, stderr string, exitCode int) {
	t.Helper()

	cmd := exec.Command(binaryPath, args...)
	var outBuf, errBuf bytes.Buffer
	cmd.Stdout = &outBuf
	cmd.Stderr = &errBuf

	err := cmd.Run()
	exitCode = 0
	if err != nil {
		if exitErr, ok := err.(*exec.ExitError); ok {
			exitCode = exitErr.ExitCode()
		} else {
			t.Fatalf("failed to run tcount: %v", err)
		}
	}

	return outBuf.String(), errBuf.String(), exitCode
}

// runTcountJSON executes tcount with --json and parses the result into a CountResult.
func runTcountJSON(t *testing.T, args ...string) *tokenizer.CountResult {
	t.Helper()

	fullArgs := append([]string{"--json"}, args...)
	stdout, stderr, exitCode := runTcount(t, fullArgs...)
	if exitCode != 0 {
		t.Fatalf("tcount exited with code %d\nstdout: %s\nstderr: %s", exitCode, stdout, stderr)
	}

	var result tokenizer.CountResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("failed to parse JSON output: %v\nraw: %s", err, stdout)
	}

	return &result
}

// fixturesDir returns the absolute path to the testdata directory.
func fixturesDir(t *testing.T) string {
	t.Helper()
	dir, err := filepath.Abs(filepath.Join("testdata"))
	if err != nil {
		t.Fatalf("failed to resolve testdata path: %v", err)
	}
	return dir
}

// projectRoot returns the absolute path to the project root.
func projectRoot() string {
	// tests/integration/ -> project root is two levels up
	_, filename, _, _ := runtime.Caller(0)
	return filepath.Join(filepath.Dir(filename), "..", "..")
}
