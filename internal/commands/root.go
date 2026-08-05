package commands

import (
	"context"
	"fmt"
	"os"
	"strings"

	"github.com/charmbracelet/lipgloss"
	"github.com/muesli/termenv"
	"github.com/spf13/cobra"

	"github.com/lancekrogers/tcount/internal/cache"
	"github.com/lancekrogers/tcount/internal/errors"
	"github.com/lancekrogers/tcount/internal/ui"
	"github.com/lancekrogers/tcount/tokenizer"
)

type countOptions struct {
	model         string
	vocabFile     string
	provider      string
	all           bool
	jsonOutput    bool
	tokensOnly    bool
	showModels    bool
	recursive     bool
	cache         bool
	noCache       bool
	cacheVerify   bool
	noProgress    bool
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
		Use:     "tcount [file|directory|-]",
		Version: version,
		Short:   "Count tokens in files using various LLM tokenizers",
		Long: `Count tokens in a file, directory, or stdin using multiple tokenization methods.

Provides token counts using different LLM tokenizers and approximation methods,
helping you understand how much of a model's context window your text uses.

Supports all modern OpenAI models (GPT-5.x, GPT-4.1, GPT-4o, o-series),
Anthropic Claude models (Opus 4.6, Sonnet 4.6, Haiku 4.5, and earlier), and
Google Gemini models.

When no path is given, or when the path is "-", tcount reads standard input
like a Unix filter (wc, cat). Use --tokens to print only the token count for
scripting pipelines.

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
  cat prompt.md | tcount --model gpt-4o --tokens           # Unix filter: number only
  tcount - --model gpt-4o --json < prompt.md               # Explicit stdin + JSON
  tcount -r ./src                                          # Count all files in directory
  tcount -d --cache ./src                                  # Opt into experimental directory caching
  TCOUNT_CACHE_DIR=/tmp/tcount-cache tcount -d --cache ./src
  tcount -r --models ./project                             # Show encoding→model lookup`,
		Args: cobra.MaximumNArgs(1),
		PersistentPreRun: func(cmd *cobra.Command, args []string) {
			if opts.noColor {
				lipgloss.SetColorProfile(termenv.Ascii)
			}
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			path := stdinMarker
			if len(args) == 1 {
				path = args[0]
			}
			return runCount(cmd.Context(), path, opts)
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
	cmd.Flags().BoolVar(&opts.tokensOnly, "tokens", false, "print only the token count (scripting / filter mode)")
	cmd.Flags().BoolVarP(&opts.showModels, "models", "m", false, "show encoding-to-model lookup table")
	cmd.Flags().BoolVarP(&opts.recursive, "recursive", "r", false, "recursively count tokens in directory")
	cmd.Flags().BoolVarP(&opts.recursive, "directory", "d", false, "alias for --recursive")
	cmd.Flags().BoolVar(&opts.cache, "cache", false, "enable experimental persistent cache for recursive directories")
	cmd.Flags().BoolVar(&opts.noCache, "no-cache", false, "force cold counting without reading or writing cache state")
	cmd.Flags().BoolVar(&opts.cacheVerify, "cache-verify", false, "hash file contents before reusing cached directory results (requires --cache)")
	cmd.Flags().BoolVar(&opts.noProgress, "no-progress", false, "disable live counting progress on the terminal")
	cmd.Flags().Float64Var(&opts.charsPerToken, "chars-per-token", tokenizer.DefaultCharsPerToken, "characters per token ratio")
	cmd.Flags().Float64Var(&opts.wordsPerToken, "words-per-token", tokenizer.DefaultWordsPerToken, "words per token ratio")
}

func runCount(ctx context.Context, path string, opts *countOptions) error {
	display := ui.New(opts.noColor, opts.verbose)
	if opts.verbose {
		opts.stats = tokenizer.NewStats()
	}
	if err := validateOutputFlags(opts); err != nil {
		return err
	}
	if err := validateCacheFlags(opts); err != nil {
		return err
	}
	if err := validateStdinFlags(path, opts); err != nil {
		return err
	}

	if !isValidProvider(opts.provider) {
		return errors.Validation(fmt.Sprintf("invalid provider %q, valid options: %s",
			opts.provider, strings.Join(validProviders, ", "))).WithField("provider", opts.provider)
	}

	if !isValidModel(opts.model) {
		display.Warning("Unknown model '%s', using approximation methods", opts.model)
	}

	startDirectoryProgress, stopProgress, attachDirectoryProgress := armDirectoryProgress(path, opts)
	defer stopProgress()

	content, walkFiles, isDirectory, err := resolveInput(ctx, path, opts, display, startDirectoryProgress)
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

	result, err := countResult(ctx, counter, path, content, walkFiles, isDirectory, opts, attachDirectoryProgress)
	if err != nil {
		return err
	}

	// Sole success-path bridge from counting → user-facing output. Always
	// clears progress inside presentCountResult before any report I/O.
	return presentCountResult(result, path, content, isDirectory, opts, display, stopProgress)
}

// presentCountResult clears live directory progress, then writes the final
// count output. All success-path reporting must go through here so Stop cannot
// run after the report on the shared TTY cursor (large-dir table clip bug).
//
// Order is structural: stop runs before countOutputWriter; callers cannot emit
// JSON/table/tokens without clearing first. defer stopProgress remains for
// error paths only (stop is idempotent).
func presentCountResult(
	result *tokenizer.CountResult,
	path string,
	content []byte,
	isDirectory bool,
	opts *countOptions,
	display *ui.UI,
	stopProgress func(),
) error {
	return emitAfterProgress(stopProgress, func() error {
		result.FilePath = path
		result.IsDirectory = isDirectory
		if !isDirectory {
			result.FileSize = len(content)
		} else if opts.stats != nil {
			outputStats(display, opts.stats.Snapshot(), cacheDiagnosticsMode(opts))
		}
		return countOutputWriter(result, opts)
	})
}

// emitAfterProgress clears the progress frame, then runs emit. Extracted so
// tests can lock stop-before-output without depending on TTY/progress paint.
func emitAfterProgress(stop func(), emit func() error) error {
	if stop != nil {
		stop()
	}
	return emit()
}

// countOutputWriter is the final report sink used by presentCountResult.
// Production points at writeCountOutput; tests swap it to assert ordering.
var countOutputWriter = writeCountOutput

// writeCountOutput renders the final count result. Call only after progress is
// cleared (see presentCountResult).
func writeCountOutput(result *tokenizer.CountResult, opts *countOptions) error {
	if opts.jsonOutput {
		return outputJSON(result)
	}
	if opts.tokensOnly {
		return outputTokensOnly(result)
	}
	return outputTable(result, opts.showModels)
}

func countResult(
	ctx context.Context,
	counter *tokenizer.Counter,
	path string,
	content []byte,
	walkFiles []string,
	isDirectory bool,
	opts *countOptions,
	attachDirectoryProgress func([]string, *tokenizer.CountFilesOptions),
) (*tokenizer.CountResult, error) {
	if !isDirectory {
		result, err := counter.Count(ctx, string(content), opts.model, opts.all)
		if err != nil {
			return nil, errors.Wrap(err, "counting tokens")
		}
		return result, nil
	}

	countOpts := tokenizer.CountFilesOptions{
		Model: opts.model,
		All:   opts.all,
	}
	if attachDirectoryProgress != nil {
		attachDirectoryProgress(walkFiles, &countOpts)
	}

	var (
		result *tokenizer.CountResult
		err    error
	)
	if opts.cache {
		store, storeErr := newCacheStore()
		if storeErr != nil {
			return nil, errors.Wrap(storeErr, "creating cache store")
		}
		mode := cache.Metadata
		if opts.cacheVerify {
			mode = cache.Verified
		}
		result, err = counter.CountFilesWithCacheOptions(ctx, path, walkFiles, countOpts, store, mode)
	} else {
		result, err = counter.CountFilesWithOptions(ctx, walkFiles, countOpts)
	}
	if err != nil {
		return nil, errors.Wrap(err, "counting tokens")
	}
	return result, nil
}

// armDirectoryProgress returns lifecycle hooks for recursive directory counts.
// start is invoked once resolveInput confirms a recursive walk; attach ends
// discovery with the walked file total and wires OnProgress; stop clears any
// painted frame (safe to call more than once).
//
// On the success path, stop is invoked only via presentCountResult (before
// report I/O). Defer stop for error returns; do not write the final report
// outside presentCountResult while a frame may still be painted.
func armDirectoryProgress(path string, opts *countOptions) (start, stop func(), attach func([]string, *tokenizer.CountFilesOptions)) {
	var progress *ui.Progress
	stop = func() {
		if progress != nil {
			progress.Stop()
			progress = nil
		}
	}
	start = func() {
		if !ui.ShouldShowProgress(true, opts.jsonOutput || opts.tokensOnly, opts.noProgress, os.Stderr) {
			return
		}
		progress = ui.NewProgress(ui.ProgressOptions{
			Root:    path,
			Model:   opts.model,
			NoColor: opts.noColor,
		})
		progress.Arm()
	}
	attach = func(walkFiles []string, countOpts *tokenizer.CountFilesOptions) {
		if progress == nil || countOpts == nil {
			return
		}
		// Ends discovery immediately so the UI does not claim scanning after the walk.
		progress.SetFilesTotal(len(walkFiles))
		countOpts.OnProgress = progress.OnProgress
	}
	return start, stop, attach
}
