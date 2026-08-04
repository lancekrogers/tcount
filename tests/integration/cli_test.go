package integration_test

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"testing"

	"github.com/lancekrogers/tcount/tokenizer"
)

func TestIntegrationCLI_SingleFile(t *testing.T) {
	file := fixturesDir(t) + "/sample.txt"
	stdout, _, exitCode := runTcount(t, file)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, "Token Count Report") {
		t.Errorf("expected 'Token Count Report' header in output:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Characters") {
		t.Errorf("expected 'Characters' in output:\n%s", stdout)
	}
	if !strings.Contains(stdout, "Method") {
		t.Errorf("expected 'Method' table header in output:\n%s", stdout)
	}
}

func TestIntegrationCLI_CacheControlsAppearInHelp(t *testing.T) {
	stdout, stderr, exitCode := runTcount(t, "--help")
	if exitCode != 0 {
		t.Fatalf("expected help exit code 0, got %d; stderr: %s", exitCode, stderr)
	}
	for _, control := range []string{"--cache", "--no-cache", "--cache-verify", "TCOUNT_CACHE_DIR"} {
		if !strings.Contains(stdout, control) {
			t.Errorf("help output missing %q:\n%s", control, stdout)
		}
	}
}

func TestIntegrationCLI_JSONOutput(t *testing.T) {
	file := fixturesDir(t) + "/sample.txt"
	result := runTcountJSON(t, file)

	if result.FilePath != file {
		t.Errorf("expected file_path %q, got %q", file, result.FilePath)
	}
	if result.Characters != 152 {
		t.Errorf("expected 152 characters, got %d", result.Characters)
	}
	if result.Lines != 3 {
		t.Errorf("expected 3 lines, got %d", result.Lines)
	}
	if len(result.Methods) == 0 {
		t.Error("expected at least one method in results")
	}

	hasExact := false
	for _, m := range result.Methods {
		if m.IsExact {
			hasExact = true
			break
		}
	}
	if !hasExact {
		t.Error("expected at least one exact tokenizer method")
	}
}

func TestIntegrationCLI_SpecificModel(t *testing.T) {
	file := fixturesDir(t) + "/sample.txt"
	stdout, _, exitCode := runTcount(t, "--model", "gpt-4o", file)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, "o200k_base") {
		t.Errorf("expected 'o200k_base' encoding in output for gpt-4o:\n%s", stdout)
	}
}

func TestIntegrationCLI_RecursiveDir(t *testing.T) {
	dir := fixturesDir(t) + "/walkdir"
	result := runTcountJSON(t, "-r", dir)

	if !result.IsDirectory {
		t.Error("expected is_directory to be true")
	}
	if result.FileCount < 3 {
		t.Errorf("expected at least 3 files counted, got %d", result.FileCount)
	}
	if result.Characters == 0 {
		t.Error("expected non-zero character count")
	}
}

func TestIntegrationCLI_VerboseKeepsJSONOnStdout(t *testing.T) {
	dir := fixturesDir(t) + "/walkdir"
	stdout, stderr, exitCode := runTcount(t, "--verbose", "--json", "-r", dir)
	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d; stderr: %s", exitCode, stderr)
	}
	var result tokenizer.CountResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("verbose JSON output = %q: %v", stdout, err)
	}
	if strings.Contains(stdout, "Cache diagnostics") || strings.Contains(stdout, "Instrumentation") {
		t.Fatalf("verbose diagnostics leaked into stdout: %s", stdout)
	}
	for _, field := range []string{"Cache diagnostics: mode=disabled", "tokenizer_calls=", "stages=walk:"} {
		if !strings.Contains(stderr, field) {
			t.Errorf("verbose stderr missing %q:\n%s", field, stderr)
		}
	}
}

func TestIntegrationCLI_ProviderFilter(t *testing.T) {
	file := fixturesDir(t) + "/sample.txt"
	stdout, _, exitCode := runTcount(t, "--json", "--provider", "openai", "--all", file)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}

	var result tokenizer.CountResult
	if err := json.Unmarshal([]byte(stdout), &result); err != nil {
		t.Fatalf("failed to parse JSON: %v", err)
	}

	for _, m := range result.Methods {
		if m.IsExact && strings.Contains(m.Name, "claude") {
			t.Errorf("provider=openai should not include claude methods, found: %s", m.Name)
		}
	}
}

func TestIntegrationCLI_ModelsFlag(t *testing.T) {
	file := fixturesDir(t) + "/sample.txt"
	stdout, _, exitCode := runTcount(t, "-r", "--models", fixturesDir(t)+"/walkdir")
	_ = file

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, "Model Lookup") {
		t.Errorf("expected 'Model Lookup' in output:\n%s", stdout)
	}
	if !strings.Contains(stdout, "o200k_base") {
		t.Errorf("expected 'o200k_base' in model lookup output:\n%s", stdout)
	}
}

func TestIntegrationCLI_ErrorCases(t *testing.T) {
	tests := []struct {
		name           string
		args           []string
		expectExitCode int
		expectStderr   string
	}{
		{
			name:           "missing file",
			args:           []string{"/nonexistent/file.txt"},
			expectExitCode: 1,
			expectStderr:   "no such file",
		},
		{
			name:           "directory without recursive flag",
			args:           []string{fixturesDir(t) + "/walkdir"},
			expectExitCode: 1,
			expectStderr:   "recursive",
		},
		{
			name:           "recursive with stdin",
			args:           []string{"-r", "-"},
			expectExitCode: 1,
			expectStderr:   "recursive",
		},
		{
			name:           "tokens with json",
			args:           []string{"--tokens", "--json", fixturesDir(t) + "/sample.txt"},
			expectExitCode: 1,
			expectStderr:   "--json",
		},
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			_, stderr, exitCode := runTcount(t, tc.args...)
			if exitCode != tc.expectExitCode {
				t.Errorf("expected exit code %d, got %d", tc.expectExitCode, exitCode)
			}
			if !strings.Contains(strings.ToLower(stderr), strings.ToLower(tc.expectStderr)) {
				t.Errorf("expected stderr to contain %q, got:\n%s", tc.expectStderr, stderr)
			}
		})
	}
}

func TestIntegrationCLI_StdinFilter(t *testing.T) {
	file := fixturesDir(t) + "/sample.txt"
	content, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	// Baseline from path mode.
	pathResult := runTcountJSON(t, "--model", "gpt-4o", file)
	if len(pathResult.Methods) != 1 {
		t.Fatalf("path mode methods = %d, want 1", len(pathResult.Methods))
	}
	wantTokens := pathResult.Methods[0].Tokens

	// Implicit stdin (no path arg).
	stdout, stderr, exitCode := runTcountWithStdin(t, string(content), "--json", "--model", "gpt-4o")
	if exitCode != 0 {
		t.Fatalf("implicit stdin exit=%d stderr=%s stdout=%s", exitCode, stderr, stdout)
	}
	var implicit tokenizer.CountResult
	if err := json.Unmarshal([]byte(stdout), &implicit); err != nil {
		t.Fatalf("parse implicit stdin JSON: %v\n%s", err, stdout)
	}
	if implicit.FilePath != "-" {
		t.Errorf("implicit stdin file_path = %q, want -", implicit.FilePath)
	}
	if len(implicit.Methods) != 1 || implicit.Methods[0].Tokens != wantTokens {
		t.Errorf("implicit stdin tokens = %v, want %d", implicit.Methods, wantTokens)
	}

	// Explicit "-" path.
	stdout, stderr, exitCode = runTcountWithStdin(t, string(content), "--json", "--model", "gpt-4o", "-")
	if exitCode != 0 {
		t.Fatalf("explicit stdin exit=%d stderr=%s stdout=%s", exitCode, stderr, stdout)
	}
	var explicit tokenizer.CountResult
	if err := json.Unmarshal([]byte(stdout), &explicit); err != nil {
		t.Fatalf("parse explicit stdin JSON: %v\n%s", err, stdout)
	}
	if explicit.FilePath != "-" {
		t.Errorf("explicit stdin file_path = %q, want -", explicit.FilePath)
	}
	if len(explicit.Methods) != 1 || explicit.Methods[0].Tokens != wantTokens {
		t.Errorf("explicit stdin tokens = %v, want %d", explicit.Methods, wantTokens)
	}
}

func TestIntegrationCLI_TokensOnly(t *testing.T) {
	file := fixturesDir(t) + "/sample.txt"
	content, err := os.ReadFile(file)
	if err != nil {
		t.Fatalf("read fixture: %v", err)
	}

	pathResult := runTcountJSON(t, "--model", "gpt-4o", file)
	want := fmt.Sprintf("%d\n", pathResult.Methods[0].Tokens)

	stdout, stderr, exitCode := runTcountWithStdin(t, string(content), "--model", "gpt-4o", "--tokens")
	if exitCode != 0 {
		t.Fatalf("tokens-only exit=%d stderr=%s stdout=%s", exitCode, stderr, stdout)
	}
	if stdout != want {
		t.Fatalf("tokens-only stdout = %q, want %q", stdout, want)
	}
	if strings.Contains(stdout, "Token Count Report") {
		t.Fatalf("tokens-only leaked report into stdout: %s", stdout)
	}
}

func TestIntegrationCLI_HelpMentionsFilterMode(t *testing.T) {
	stdout, _, exitCode := runTcount(t, "--help")
	if exitCode != 0 {
		t.Fatalf("help exit code = %d", exitCode)
	}
	for _, needle := range []string{"--tokens", "standard input", "stdin"} {
		if !strings.Contains(strings.ToLower(stdout), strings.ToLower(needle)) {
			t.Errorf("help missing %q:\n%s", needle, stdout)
		}
	}
}
