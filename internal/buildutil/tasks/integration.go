// internal/buildutil/tasks/integration.go
package tasks

import (
	"bufio"
	"encoding/json"
	"fmt"
	"os/exec"
	"strings"
	"time"

	"github.com/lancekrogers/tcount/internal/buildutil/ui"
)

// Integration runs integration tests (tests with "Integration" in their name)
func Integration(verbose bool) error {
	ui.Section("Running Integration Tests")

	start := time.Now()

	// Run all tests matching "Integration" pattern
	cmd := exec.Command("go", "test", "-json", "-run", "Integration", "-timeout", "60s", "./...")
	output, _ := cmd.Output()
	duration := time.Since(start)

	// Parse JSON output to count tests
	passed, failed := parseIntegrationOutput(output, verbose)
	totalTests := passed + failed

	// No integration tests found
	if totalTests == 0 {
		ui.Status("No integration tests found", true)
		return nil
	}

	ui.ClearProgress()

	// Display summary
	totalStatus := fmt.Sprintf("%d/%d tests passed", passed, totalTests)
	if ui.ColourEnabled() {
		if failed > 0 {
			totalStatus = ui.Red + totalStatus + ui.Reset
		} else {
			totalStatus = ui.Green + totalStatus + ui.Reset
		}
	}

	rows := [][]string{
		{"Metric", "Value"},
		{"Tests Passed", fmt.Sprintf("%d", passed)},
		{"Tests Failed", fmt.Sprintf("%d", failed)},
		{"Total", totalStatus},
	}

	success := failed == 0
	successMsg := fmt.Sprintf("✓ ALL %d INTEGRATION TESTS PASSED", passed)
	failMsg := fmt.Sprintf("✗ %d/%d INTEGRATION TESTS FAILED", failed, totalTests)

	ui.SummaryCardWithStatus("Integration Test Summary", rows, fmt.Sprintf("%.2fs", duration.Seconds()), success, successMsg, failMsg)

	if failed > 0 {
		return fmt.Errorf("%d integration tests failed", failed)
	}

	return nil
}

// parseIntegrationOutput parses go test -json output for integration tests
func parseIntegrationOutput(output []byte, verbose bool) (passed, failed int) {
	scanner := bufio.NewScanner(strings.NewReader(string(output)))

	for scanner.Scan() {
		line := scanner.Text()
		if line == "" {
			continue
		}

		var event testEvent
		if err := json.Unmarshal([]byte(line), &event); err != nil {
			continue
		}

		// Only count actual test results (not package-level or sub-tests)
		if event.Test != "" && !strings.Contains(event.Test, "/") {
			switch event.Action {
			case "pass":
				passed++
			case "fail":
				failed++
				if verbose {
					fmt.Printf("  FAIL: %s\n", event.Test)
				}
			}
		}
	}

	return passed, failed
}
