package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"

	"github.com/lancekrogers/tcount/internal/errors"
	"github.com/lancekrogers/tcount/internal/ui"
	"github.com/lancekrogers/tcount/tokenizer"
	"github.com/lancekrogers/tcount/tokenizer/fileops"
)

var (
	noColor bool
	verbose bool
)

type countOptions struct {
	model         string
	vocabFile     string
	provider      string
	all           bool
	jsonOutput    bool
	showModels    bool
	recursive     bool
	charsPerToken float64
	wordsPerToken float64
}

// Execute runs the root command with the given version string.
func Execute(version string) {
	if err := newRootCmd(version).Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func newRootCmd(version string) *cobra.Command {
	opts := &countOptions{}

	cmd := &cobra.Command{
		Use:     "tcount [file|directory]",
		Version: version,
		Short:   "Count tokens in files using various LLM tokenizers",
		Long: `Count tokens in a file or directory using multiple tokenization methods.

Provides token counts using different LLM tokenizers and approximation methods,
helping you understand how much of a model's context window your text uses.

Supports all modern OpenAI models (GPT-5.x, GPT-4.1, GPT-4o, o-series),
Anthropic Claude models (Opus 4.6, Sonnet 4.6, Haiku 4.5, and earlier), and
Google Gemini models.

When counting a directory with --recursive, the command:
  - Respects .gitignore files
  - Skips binary files automatically
  - Returns aggregated totals for all text files`,
		Example: `  tcount document.md                                       # Count tokens in a file
  tcount --model gpt-4o doc.md                             # Use GPT-4o tokenizer
  tcount --model gpt-5 doc.md                              # Use GPT-5 tokenizer
  tcount --model claude-sonnet-4.6 doc.md                   # Use Claude Sonnet 4.6
  tcount --model gemini-2.5-pro doc.md                      # Use Gemini 2.5 Pro (approx)
  tcount --model llama-3.1-8b --vocab-file tokenizer.model doc.md  # SentencePiece
  tcount --all doc.md                                      # Show all counting methods
  tcount --json doc.md                                     # Output as JSON
  tcount -r ./src                                          # Count all files in directory
  tcount -r --models ./project                             # Show encoding→model lookup`,
		Args: cobra.ExactArgs(1),
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if noColor {
				lipgloss.SetColorProfile(termenv.Ascii)
			}
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCount(cmd.Context(), args[0], opts)
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	cmd.PersistentFlags().BoolVar(&noColor, "no-color", false, "disable color output")
	cmd.PersistentFlags().BoolVar(&verbose, "verbose", false, "enable verbose output")

	cmd.Flags().StringVar(&opts.model, "model", "", `specific model to use

OpenAI Models:
  GPT-5 series:     gpt-5, gpt-5-mini, gpt-5-nano, gpt-5.1, gpt-5.2
  GPT-4.1 series:   gpt-4.1, gpt-4.1-mini, gpt-4.1-nano
  GPT-4o series:    gpt-4o, gpt-4o-mini
  o-series:         o3, o3-mini, o4-mini
  Legacy:           gpt-4, gpt-4-turbo, gpt-3.5-turbo

Anthropic Models:
  Opus:             claude-opus-4.6, claude-opus-4.5, claude-opus-4.1, claude-opus-4
  Sonnet:           claude-sonnet-4.6, claude-sonnet-4.5, claude-sonnet-4
  Haiku:            claude-haiku-4.5, claude-haiku-3.5, claude-haiku-3
  Legacy:           claude-opus-3

Google Models (character approximation):
  Gemini:           gemini-2.5-pro, gemini-2.5-flash, gemini-2.5-flash-lite

Open Source Models (BPE approximation):
  Llama:            llama-3.1-8b, llama-3.1-70b, llama-3.1-405b, llama-4-scout, llama-4-maverick
  DeepSeek:         deepseek-v2, deepseek-v3, deepseek-coder-v2
  Qwen:             qwen-2.5-7b, qwen-2.5-14b, qwen-2.5-72b, qwen-3-72b
  Phi:              phi-3-mini, phi-3-small, phi-3-medium`)
	cmd.Flags().StringVar(&opts.vocabFile, "vocab-file", "", `path to SentencePiece .model file for exact tokenization
Required for models that use SentencePiece (e.g., llama-3.1-8b)
Download vocab files from HuggingFace (see error messages for URLs)`)
	cmd.Flags().StringVar(&opts.provider, "provider", "all", `filter models by provider (openai, anthropic, google, meta, deepseek, alibaba, microsoft, all)`)
	cmd.Flags().BoolVar(&opts.all, "all", false, "show all counting methods")
	cmd.Flags().BoolVar(&opts.jsonOutput, "json", false, "output in JSON format")
	cmd.Flags().BoolVarP(&opts.showModels, "models", "m", false, "show encoding-to-model lookup table")
	cmd.Flags().BoolVarP(&opts.recursive, "recursive", "r", false, "recursively count tokens in directory")
	cmd.Flags().BoolVarP(&opts.recursive, "directory", "d", false, "alias for --recursive")
	cmd.Flags().Float64Var(&opts.charsPerToken, "chars-per-token", 4.0, "characters per token ratio")
	cmd.Flags().Float64Var(&opts.wordsPerToken, "words-per-token", 0.75, "words per token ratio")

	return cmd
}

// isValidModel checks if a model name is valid using the tokenizer registry.
func isValidModel(model string) bool {
	if model == "" {
		return true
	}
	for _, valid := range tokenizer.ListModels() {
		if model == valid {
			return true
		}
	}
	return false
}

// sentencePieceVocabURLs maps model prefixes to their HuggingFace vocab download URLs.
var sentencePieceVocabURLs = map[string]string{
	"llama-3.1": "https://huggingface.co/meta-llama/Llama-3.1-8B/blob/main/original/tokenizer.model",
	"llama-4":   "https://huggingface.co/meta-llama/Llama-4-Scout-17B-16E/blob/main/tokenizer.model",
}

// isValidProvider checks if a provider name is valid.
func isValidProvider(provider string) bool {
	for _, valid := range validProviders {
		if provider == valid {
			return true
		}
	}
	return false
}

// requiresSentencePiece checks if a model can use SentencePiece tokenization
// and returns the download URL for the vocab file.
func requiresSentencePiece(model string) (bool, string) {
	for prefix, url := range sentencePieceVocabURLs {
		if strings.HasPrefix(model, prefix) {
			return true, url
		}
	}
	return false, ""
}

// validProviders lists accepted values for the --provider flag.
var validProviders = []string{"openai", "anthropic", "google", "meta", "deepseek", "alibaba", "microsoft", "all"}

func runCount(ctx context.Context, path string, opts *countOptions) error {
	display := ui.New(noColor, verbose)

	if !isValidProvider(opts.provider) {
		return fmt.Errorf("invalid provider %q, valid options: %s", opts.provider, strings.Join(validProviders, ", "))
	}

	if !isValidModel(opts.model) {
		display.Warning("Unknown model '%s', using approximation methods", opts.model)
	}

	info, err := os.Stat(path)
	if err != nil {
		return errors.IO("accessing path", err).WithField("path", path)
	}

	var content []byte
	var walkFiles []string
	isDirectory := info.IsDir()

	if isDirectory {
		if !opts.recursive {
			return errors.Validation("path is a directory — use --recursive flag to count tokens in all files").WithField("path", path)
		}

		walkResult, err := fileops.WalkDirectory(ctx, path)
		if err != nil {
			return errors.IO("walking directory", err).WithField("path", path)
		}

		if len(walkResult.Files) == 0 {
			return errors.NotFound("text files in directory").WithField("path", path)
		}

		if verbose {
			display.Info("Found %d text files (skipped %d binary, %d ignored)",
				len(walkResult.Files), walkResult.SkippedBinary, walkResult.SkippedIgnore)
		}

		walkFiles = walkResult.Files
	} else {
		content, err = os.ReadFile(path)
		if err != nil {
			return errors.IO("reading file", err).WithField("path", path)
		}
	}

	// Check if model requires SentencePiece and validate vocab-file flag
	if needsSP, downloadURL := requiresSentencePiece(opts.model); needsSP && opts.vocabFile == "" {
		return fmt.Errorf(
			"model %s requires a SentencePiece vocab file\n\n"+
				"Download the tokenizer.model file from:\n"+
				"  %s\n\n"+
				"Then run:\n"+
				"  tcount --model %s --vocab-file /path/to/tokenizer.model <input>",
			opts.model, downloadURL, opts.model,
		)
	}

	counter, err := tokenizer.NewCounter(tokenizer.CounterOptions{
		CharsPerToken: opts.charsPerToken,
		WordsPerToken: opts.wordsPerToken,
		VocabFile:     opts.vocabFile,
		Provider:      tokenizer.Provider(opts.provider),
	})
	if err != nil {
		return errors.Wrap(err, "creating token counter")
	}

	var result *tokenizer.CountResult
	if isDirectory {
		result, err = counter.CountFiles(ctx, walkFiles, opts.model, opts.all)
	} else {
		result, err = counter.Count(ctx, string(content), opts.model, opts.all)
	}
	if err != nil {
		return errors.Wrap(err, "counting tokens")
	}

	result.FilePath = path
	result.IsDirectory = isDirectory
	if !isDirectory {
		result.FileSize = len(content)
	}

	if opts.jsonOutput {
		return outputJSON(result)
	}

	return outputTable(display, result, opts.showModels)
}

func outputJSON(result *tokenizer.CountResult) error {
	encoder := json.NewEncoder(os.Stdout)
	encoder.SetIndent("", "  ")
	return encoder.Encode(result)
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

func outputTable(_ *ui.UI, result *tokenizer.CountResult, showModels bool) error {
	titleStyle, sectionStyle, labelStyle, valStyle := styles()

	// Title
	path := result.FilePath
	if result.IsDirectory {
		path += " (directory)"
	}
	fmt.Println(titleStyle.Render("Token Count Report for: " + path))
	fmt.Println()

	// Basic Statistics
	fmt.Println(sectionStyle.Render("Basic Statistics"))
	if result.IsDirectory {
		fmt.Printf("  %s %s\n", labelStyle.Render("Files:"), valStyle.Render(formatInt(result.FileCount)))
	}
	fmt.Printf("  %s %s\n", labelStyle.Render("Characters:"), valStyle.Render(formatInt(result.Characters)))
	fmt.Printf("  %s %s\n", labelStyle.Render("Words:"), valStyle.Render(formatInt(result.Words)))
	fmt.Printf("  %s %s\n", labelStyle.Render("Lines:"), valStyle.Render(formatInt(result.Lines)))
	fmt.Println()

	// Context usage is only shown when a specific --model attaches a context
	// window to its method(s).
	showContext := false
	for _, method := range result.Methods {
		if method.ContextWindow > 0 {
			showContext = true
			break
		}
	}

	// Build token table rows
	rows := make([][]string, 0, len(result.Methods))
	for _, method := range result.Methods {
		accuracy := "Approx"
		if method.IsExact {
			accuracy = "Exact"
		} else if method.Name == "claude_3_approx" {
			accuracy = "Estimated"
		}
		row := []string{method.DisplayName, formatInt(method.Tokens), accuracy}
		if showContext {
			row = append(row, formatContextUsage(method.Tokens, method.ContextWindow))
		}
		rows = append(rows, row)
	}

	// Styled table
	purple := lipgloss.Color("99")
	headerStyle := lipgloss.NewStyle().Bold(true).Foreground(purple).Align(lipgloss.Center)
	cellStyle := lipgloss.NewStyle().PaddingLeft(1).PaddingRight(1)
	tokenCellStyle := cellStyle.Align(lipgloss.Right)

	headers := []string{"Method", "Tokens", "Accuracy"}
	if showContext {
		headers = append(headers, "Context Usage")
	}

	t := table.New().
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

	fmt.Println(sectionStyle.Render("Token Counts by Method"))
	fmt.Println(t)

	// Model lookup
	if showModels {
		fmt.Println()
		outputModelLookup(sectionStyle, labelStyle)
	}

	return nil
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

	order := []string{"o200k_base", "cl100k_base", "claude_approx", "gemini_approx"}
	for _, enc := range order {
		models, ok := byEncoding[enc]
		if !ok {
			continue
		}
		fmt.Printf("  %s %s\n", labelStyle.Render(enc+":"), strings.Join(models, ", "))
	}
}
