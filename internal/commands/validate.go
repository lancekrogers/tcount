package commands

import (
	"fmt"
	"slices"
	"strings"

	"github.com/lancekrogers/tcount/internal/errors"
	"github.com/lancekrogers/tcount/tokenizer"
)

// validProviders lists accepted values for the --provider flag.
var validProviders = []string{"openai", "anthropic", "google", "meta", "deepseek", "alibaba", "microsoft", "all"}

// sentencePieceVocabURLs maps model prefixes to their HuggingFace vocab download URLs.
var sentencePieceVocabURLs = map[string]string{
	"llama-3.1": "https://huggingface.co/meta-llama/Llama-3.1-8B/blob/main/original/tokenizer.model",
	"llama-4":   "https://huggingface.co/meta-llama/Llama-4-Scout-17B-16E/blob/main/tokenizer.model",
}

// isValidModel checks if a model name is valid using the tokenizer registry.
func isValidModel(model string) bool {
	return model == "" || slices.Contains(tokenizer.ListModels(), model)
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

func validateOutputFlags(opts *countOptions) error {
	if opts.tokensOnly && opts.jsonOutput {
		return errors.Validation("--tokens and --json cannot be used together")
	}
	if opts.tokensOnly && opts.showModels {
		return errors.Validation("--tokens and --models cannot be used together")
	}
	if opts.tokensOnly && opts.all {
		return errors.Validation("--tokens and --all cannot be used together; select one model with --model")
	}
	return nil
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
