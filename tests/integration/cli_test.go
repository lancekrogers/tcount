package integration_test

import (
	"encoding/json"
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

func TestIntegrationCLI_CostEstimates(t *testing.T) {
	file := fixturesDir(t) + "/sample.txt"
	stdout, _, exitCode := runTcount(t, "--cost", "--model", "gpt-4o", file)

	if exitCode != 0 {
		t.Fatalf("expected exit code 0, got %d", exitCode)
	}
	if !strings.Contains(stdout, "Cost Estimates") {
		t.Errorf("expected 'Cost Estimates' in output:\n%s", stdout)
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
