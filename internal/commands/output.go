package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"

	"github.com/lancekrogers/tcount/internal/errors"
	"github.com/lancekrogers/tcount/tokenizer"
)

func outputJSON(result *tokenizer.CountResult) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
}

// outputTokensOnly prints a single integer token count for filter/pipeline use.
// Prefers the first exact method, then falls back to the first method overall.
func outputTokensOnly(result *tokenizer.CountResult) error {
	tokens, err := primaryTokenCount(result)
	if err != nil {
		return err
	}
	fmt.Println(tokens)
	return nil
}

// primaryTokenCount selects the method used by --tokens: first exact method,
// otherwise the first listed method.
func primaryTokenCount(result *tokenizer.CountResult) (int, error) {
	if result == nil || len(result.Methods) == 0 {
		return 0, errors.Validation("no token methods available to report")
	}
	for _, method := range result.Methods {
		if method.IsExact {
			return method.Tokens, nil
		}
	}
	return result.Methods[0].Tokens, nil
}

// displayPath returns a human-facing path label. Stdin stays "-" in JSON
// (FilePath) but renders as "stdin" in the interactive report.
func displayPath(result *tokenizer.CountResult) string {
	if isStdinSource(result.FilePath) {
		return "stdin"
	}
	path := result.FilePath
	if result.IsDirectory {
		path += " (directory)"
	}
	return path
}

// styles returns lipgloss styles for output rendering.
func styles() (title, section, label, valStyle lipgloss.Style) {
	purple := lipgloss.Color("99")
	dim := lipgloss.Color("245")

	title = lipgloss.NewStyle().Bold(true).Foreground(purple)
	section = lipgloss.NewStyle().Bold(true).Foreground(purple)
	label = lipgloss.NewStyle().Foreground(dim)
	valStyle = lipgloss.NewStyle()
	return
}

func outputTable(result *tokenizer.CountResult, showModels bool) error {
	_, sectionStyle, labelStyle, _ := styles()

	printReportHeader(result)

	rows, showContext := methodRows(result)
	fmt.Println(sectionStyle.Render("Token Counts by Method"))
	fmt.Println(renderMethodTable(rows, showContext))

	if showModels {
		fmt.Println()
		outputModelLookup(sectionStyle, labelStyle)
	}

	return nil
}

// printReportHeader prints the report title and the basic statistics block.
func printReportHeader(result *tokenizer.CountResult) {
	titleStyle, sectionStyle, labelStyle, valStyle := styles()

	fmt.Println(titleStyle.Render("Token Count Report for: " + displayPath(result)))
	fmt.Println()

	fmt.Println(sectionStyle.Render("Basic Statistics"))
	if result.IsDirectory {
		fmt.Printf("  %s %s\n", labelStyle.Render("Files:"), valStyle.Render(formatInt(result.FileCount)))
	}
	fmt.Printf("  %s %s\n", labelStyle.Render("Characters:"), valStyle.Render(formatInt(result.Characters)))
	fmt.Printf("  %s %s\n", labelStyle.Render("Words:"), valStyle.Render(formatInt(result.Words)))
	fmt.Printf("  %s %s\n", labelStyle.Render("Lines:"), valStyle.Render(formatInt(result.Lines)))
	fmt.Println()
}

// methodRows builds the token table rows. The second return reports whether
// any method carries a context window, which adds the Context Usage column.
func methodRows(result *tokenizer.CountResult) ([][]string, bool) {
	showContext := false
	for _, method := range result.Methods {
		if method.ContextWindow > 0 {
			showContext = true
			break
		}
	}

	rows := make([][]string, 0, len(result.Methods))
	for _, method := range result.Methods {
		accuracy := "Approx"
		if method.IsExact {
			accuracy = "Exact"
		} else if method.Name == tokenizer.NameClaudeApprox {
			accuracy = "Estimated"
		}
		row := []string{method.DisplayName, formatInt(method.Tokens), accuracy}
		if showContext {
			row = append(row, formatContextUsage(method.Tokens, method.ContextWindow))
		}
		rows = append(rows, row)
	}

	return rows, showContext
}

// renderMethodTable renders the styled token table.
func renderMethodTable(rows [][]string, showContext bool) *table.Table {
	purple := lipgloss.Color("99")
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(purple).Align(lipgloss.Center)
	cellStyle := lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1)
	tokenCellStyle := cellStyle.Align(lipgloss.Right)

	headers := []string{"Method", "Tokens", "Accuracy"}
	if showContext {
		headers = append(headers, "Context Usage")
	}

	return table.New().
		Border(lipgloss.RoundedBorder()).
		BorderStyle(lipgloss.NewStyle().Foreground(purple)).
		Headers(headers...).
		Rows(rows...).
		StyleFunc(func(row, col int) lipgloss.Style {
			if row == table.HeaderRow {
				return headerStyle
			}
			// Tokens column: right-aligned
			if col == 1 {
				return tokenCellStyle
			}
			// Accuracy column: color-coded
			if col == 2 && row >= 0 && row < len(rows) {
				switch rows[row][2] {
				case "Exact":
					return cellStyle.Foreground(lipgloss.Color("10"))
				case "Estimated":
					return cellStyle.Foreground(lipgloss.Color("11"))
				default:
					return cellStyle.Foreground(lipgloss.Color("245"))
				}
			}
			return cellStyle
		})
}

// formatInt formats an integer with comma thousand separators.
func formatInt(n int) string {
	if n < 0 {
		return "-" + formatInt(-n)
	}
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	remainder := len(s) % 3
	if remainder > 0 {
		b.WriteString(s[:remainder])
	}
	for i := remainder; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}

// formatContextUsage returns a "<pct> of <window>" string describing how much
// of a model's context window the token count consumes. Returns empty if the
// window is unknown.
func formatContextUsage(tokens, window int) string {
	if window <= 0 {
		return ""
	}
	pct := float64(tokens) / float64(window) * 100
	var pctStr string
	switch {
	case pct >= 10:
		pctStr = fmt.Sprintf("%.0f%%", pct)
	case pct >= 1:
		pctStr = fmt.Sprintf("%.1f%%", pct)
	case pct >= 0.1:
		pctStr = fmt.Sprintf("%.2f%%", pct)
	default:
		pctStr = "<0.1%"
	}
	return fmt.Sprintf("%s of %s", pctStr, formatWindow(window))
}

// formatWindow renders a context-window size compactly (e.g. 1M, 400K, 128K).
func formatWindow(n int) string {
	switch {
	case n >= 1_000_000:
		m := float64(n) / 1_000_000
		if m == float64(int(m)) {
			return fmt.Sprintf("%dM", int(m))
		}
		return fmt.Sprintf("%.1fM", m)
	case n >= 1000:
		return fmt.Sprintf("%dK", n/1000)
	default:
		return fmt.Sprintf("%d", n)
	}
}

// outputModelLookup prints the encoding→model mapping.
func outputModelLookup(sectionStyle, labelStyle lipgloss.Style) {
	fmt.Println(sectionStyle.Render("Model Lookup"))

	byEncoding := tokenizer.ModelsByEncoding()

	order := []string{tokenizer.EncodingO200kBase, tokenizer.EncodingCL100kBase, tokenizer.EncodingClaudeApprox, tokenizer.EncodingGeminiApprox}
	for _, enc := range order {
		models, ok := byEncoding[enc]
		if !ok {
			continue
		}
		fmt.Printf("  %s %s\n", labelStyle.Render(enc+":"), strings.Join(models, ", "))
	}
}
