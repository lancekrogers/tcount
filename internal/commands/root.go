package commands

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"slices"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/lipgloss/table"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"

	"github.com/lancekrogers/tcount/internal/cache"
	"github.com/lancekrogers/tcount/internal/errors"
	"github.com/lancekrogers/tcount/internal/ui"
	"github.com/lancekrogers/tcount/tokenizer"
	"github.com/lancekrogers/tcount/tokenizer/fileops"
)

type countOptions struct {
	model         string
	vocabFile     string
	provider      string
	all           bool
	jsonOutput    bool
	showModels    bool
	recursive     bool
	cache         bool
	noCache       bool
	cacheVerify   bool
	noColor       bool
	verbose       bool
	stats         *tokenizer.Stats
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
  - Counts each text file in parallel and returns summed totals
  - Enables experimental persistence only when --cache is explicitly supplied
  - Hashes file contents before reuse when --cache-verify is supplied`,
		Example: `  tcount document.md                                       # Count tokens in a file
  tcount --model gpt-4o doc.md                             # Use GPT-4o tokenizer
  tcount --model gpt-5 doc.md                              # Use GPT-5 tokenizer
  tcount --model claude-sonnet-4.6 doc.md                   # Use Claude Sonnet 4.6
  tcount --model gemini-2.5-pro doc.md                      # Use Gemini 2.5 Pro (approx)
  tcount --model llama-3.1-8b --vocab-file tokenizer.model doc.md  # SentencePiece
  tcount --all doc.md                                      # Show all counting methods
  tcount --json doc.md                                     # Output as JSON
  tcount -r ./src                                          # Count all files in directory
  tcount -d --cache ./src                                  # Opt into experimental directory caching
  TCOUNT_CACHE_DIR=/tmp/tcount-cache tcount -d --cache ./src
  tcount -r --models ./project                             # Show encoding→model lookup`,
		Args: cobra.ExactArgs(1),
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if opts.noColor {
				lipgloss.SetColorProfile(termenv.Ascii)
			}
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			return runCount(cmd.Context(), args[0], opts)
		},
		SilenceUsage:  true,
		SilenceErrors: true,
	}

	registerFlags(cmd, opts)
	cmd.AddCommand(newCacheCommand())

	return cmd
}

// registerFlags binds every root-command flag to the countOptions fields.
func registerFlags(cmd *cobra.Command, opts *countOptions) {
	cmd.PersistentFlags().BoolVar(&opts.noColor, "no-color", false, "disable color output")
	cmd.PersistentFlags().BoolVar(&opts.verbose, "verbose", false, "enable verbose output")

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
	cmd.Flags().BoolVar(&opts.cache, "cache", false, "enable experimental persistent cache for recursive directories")
	cmd.Flags().BoolVar(&opts.noCache, "no-cache", false, "force cold counting without reading or writing cache state")
	cmd.Flags().BoolVar(&opts.cacheVerify, "cache-verify", false, "hash file contents before reusing cached directory results (requires --cache)")
	cmd.Flags().Float64Var(&opts.charsPerToken, "chars-per-token", tokenizer.DefaultCharsPerToken, "characters per token ratio")
	cmd.Flags().Float64Var(&opts.wordsPerToken, "words-per-token", tokenizer.DefaultWordsPerToken, "words per token ratio")
}

// isValidModel checks if a model name is valid using the tokenizer registry.
func isValidModel(model string) bool {
	return model == "" || slices.Contains(tokenizer.ListModels(), model)
}

// sentencePieceVocabURLs maps model prefixes to their HuggingFace vocab download URLs.
var sentencePieceVocabURLs = map[string]string{
	"llama-3.1": "https://huggingface.co/meta-llama/Llama-3.1-8B/blob/main/original/tokenizer.model",
	"llama-4":   "https://huggingface.co/meta-llama/Llama-4-Scout-17B-16E/blob/main/tokenizer.model",
}

// isValidProvider checks if a provider name is valid.
func isValidProvider(provider string) bool {
	return slices.Contains(validProviders, provider)
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
	display := ui.New(opts.noColor, opts.verbose)
	if opts.verbose {
		opts.stats = tokenizer.NewStats()
	}
	if err := validateCacheFlags(opts); err != nil {
		return err
	}

	if !isValidProvider(opts.provider) {
		return errors.Validation(fmt.Sprintf("invalid provider %q, valid options: %s",
			opts.provider, strings.Join(validProviders, ", "))).WithField("provider", opts.provider)
	}

	if !isValidModel(opts.model) {
		display.Warning("Unknown model '%s', using approximation methods", opts.model)
	}

	content, walkFiles, isDirectory, err := resolveInput(ctx, path, opts, display)
	if err != nil {
		return err
	}
	if err := validateCacheTarget(opts, isDirectory); err != nil {
		return err
	}

	if err := sentencePieceGuard(opts); err != nil {
		return err
	}

	counter, err := tokenizer.NewCounter(tokenizer.CounterOptions{
		CharsPerToken: opts.charsPerToken,
		WordsPerToken: opts.wordsPerToken,
		VocabFile:     opts.vocabFile,
		Provider:      tokenizer.Provider(opts.provider),
		Stats:         opts.stats,
	})
	if err != nil {
		return errors.Wrap(err, "creating token counter")
	}

	var result *tokenizer.CountResult
	if isDirectory {
		if opts.cache {
			store, storeErr := newCacheStore()
			if storeErr != nil {
				return errors.Wrap(storeErr, "creating cache store")
			}
			mode := cache.Metadata
			if opts.cacheVerify {
				mode = cache.Verified
			}
			result, err = counter.CountFilesWithCache(ctx, path, walkFiles, opts.model, opts.all, store, mode)
		} else {
			result, err = counter.CountFiles(ctx, walkFiles, opts.model, opts.all)
		}
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
	} else if opts.stats != nil {
		outputStats(display, opts.stats.Snapshot(), cacheDiagnosticsMode(opts))
	}

	if opts.jsonOutput {
		return outputJSON(result)
	}

	return outputTable(result, opts.showModels)
}

func validateCacheFlags(opts *countOptions) error {
	if opts.cache && opts.noCache {
		return errors.Validation("--cache and --no-cache cannot be used together")
	}
	if opts.cacheVerify && !opts.cache {
		return errors.Validation("--cache-verify requires --cache")
	}
	return nil
}

func validateCacheTarget(opts *countOptions, isDirectory bool) error {
	if opts.cache && !isDirectory {
		return errors.Validation("--cache is only supported for recursive directory counts")
	}
	return nil
}

const cacheDirEnvironment = "TCOUNT_CACHE_DIR"

func newCacheStore() (*cache.FileStore, error) {
	baseDir := os.Getenv(cacheDirEnvironment)
	if baseDir != "" {
		info, err := os.Stat(baseDir)
		if err == nil && !info.IsDir() {
			return nil, fmt.Errorf("%s must name a directory: %s", cacheDirEnvironment, baseDir)
		}
		if err != nil && !os.IsNotExist(err) {
			return nil, fmt.Errorf("checking %s=%q: %w", cacheDirEnvironment, baseDir, err)
		}
		resolver, err := cache.NewLocationResolverAt(baseDir)
		if err != nil {
			return nil, fmt.Errorf("using %s=%q: %w", cacheDirEnvironment, baseDir, err)
		}
		return cache.NewFileStore(resolver), nil
	}

	store, err := cache.NewDefaultFileStore()
	if err != nil {
		return nil, err
	}
	return store, nil
}

// resolveInput stats the path and loads the single file's content or walks
// the directory for its file list.
func resolveInput(ctx context.Context, path string, opts *countOptions, display *ui.UI) (content []byte, walkFiles []string, isDirectory bool, err error) {
	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, false, errors.IO("accessing path", err).WithField("path", path)
	}

	if !info.IsDir() {
		content, err = os.ReadFile(path)
		if err != nil {
			return nil, nil, false, errors.IO("reading file", err).WithField("path", path)
		}
		return content, nil, false, nil
	}

	if !opts.recursive {
		return nil, nil, true, errors.Validation("path is a directory — use --recursive flag to count tokens in all files").WithField("path", path)
	}

	var walkResult *fileops.WalkResult
	if opts.stats != nil {
		walkResult, err = fileops.WalkDirectory(ctx, path, opts.stats)
	} else {
		walkResult, err = fileops.WalkDirectory(ctx, path)
	}
	if err != nil {
		return nil, nil, true, errors.IO("walking directory", err).WithField("path", path)
	}

	if len(walkResult.Files) == 0 {
		return nil, nil, true, errors.NotFound("text files in directory").WithField("path", path)
	}

	return nil, walkResult.Files, true, nil
}

func cacheDiagnosticsMode(opts *countOptions) string {
	if !opts.cache {
		return "disabled"
	}
	if opts.cacheVerify {
		return cache.Verified.String()
	}
	return cache.Metadata.String()
}

func outputStats(display *ui.UI, stats tokenizer.StatsSnapshot, validationMode string) {
	display.Diagnostic("Cache diagnostics: mode=%s files=%d hits=%d partial_hits=%d misses=%d incompatibilities=%d methods_avoided=%d reused_bytes=%d read_bytes=%d tokenizer_calls=%d warnings=%d stages=walk:%s,validation_read:%s,tokenization:%s,aggregation:%s,persistence:%s reasons=%s",
		validationMode,
		stats.EligibleFiles,
		stats.CacheHits,
		stats.CachePartialHits,
		stats.CacheMisses,
		cacheIncompatibilities(stats.CacheReasons),
		stats.CacheMethodsAvoided,
		stats.CacheBytesReused,
		stats.FullFileBytes,
		tokenizerCalls(stats.FilesTokenizedByMethod),
		stats.CacheWarnings,
		stats.WalkDuration,
		stats.ValidationReadDuration,
		stats.TokenizationDuration,
		stats.AggregationDuration,
		stats.PersistenceReadyDuration,
		formatCacheReasons(stats.CacheReasons),
	)
}

func tokenizerCalls(byMethod map[string]int64) int64 {
	var calls int64
	for _, count := range byMethod {
		calls += count
	}
	return calls
}

func cacheIncompatibilities(reasons map[string]int64) int64 {
	var total int64
	for reason, count := range reasons {
		switch cache.InvalidationReason(reason) {
		case cache.ReasonSchemaMismatch,
			cache.ReasonRootMismatch,
			cache.ReasonPathChanged,
			cache.ReasonSizeChanged,
			cache.ReasonModTimeChanged,
			cache.ReasonContentChanged,
			cache.ReasonClassificationChanged,
			cache.ReasonContractMissing:
			total += count
		}
	}
	return total
}

func formatCacheReasons(reasons map[string]int64) string {
	keys := make([]string, 0, len(reasons))
	for reason, count := range reasons {
		if count > 0 {
			keys = append(keys, reason)
		}
	}
	if len(keys) == 0 {
		return "none"
	}
	slices.Sort(keys)
	parts := make([]string, 0, len(keys))
	for _, key := range keys {
		parts = append(parts, fmt.Sprintf("%s=%d", key, reasons[key]))
	}
	return strings.Join(parts, ",")
}

// sentencePieceGuard rejects models that require a SentencePiece vocab file
// when --vocab-file was not provided.
func sentencePieceGuard(opts *countOptions) error {
	needsSP, downloadURL := requiresSentencePiece(opts.model)
	if !needsSP || opts.vocabFile != "" {
		return nil
	}

	return errors.Validation(fmt.Sprintf(
		"model %s requires a SentencePiece vocab file\n\n"+
			"Download the tokenizer.model file from:\n"+
			"  %s\n\n"+
			"Then run:\n"+
			"  tcount --model %s --vocab-file /path/to/tokenizer.model <input>",
		opts.model, downloadURL, opts.model,
	)).WithField("model", opts.model)
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

	path := result.FilePath
	if result.IsDirectory {
		path += " (directory)"
	}
	fmt.Println(titleStyle.Render("Token Count Report for: " + path))
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
