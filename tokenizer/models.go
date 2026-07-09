package tokenizer

import "sort"

// Provider represents an LLM provider.
type Provider string

const (
	ProviderOpenAI    Provider = "openai"    // OpenAI (GPT, o-series)
	ProviderAnthropic Provider = "anthropic" // Anthropic (Claude)
	ProviderMeta      Provider = "meta"      // Meta (Llama)
	ProviderDeepSeek  Provider = "deepseek"  // DeepSeek
	ProviderAlibaba   Provider = "alibaba"   // Alibaba (Qwen)
	ProviderMicrosoft Provider = "microsoft" // Microsoft (Phi)
	ProviderGoogle    Provider = "google"    // Google (Gemini)
)

// ModelMetadata contains comprehensive information about an LLM model.
type ModelMetadata struct {
	Name          string   // Model identifier (e.g., "gpt-4o", "claude-sonnet-4.6")
	Provider      Provider // Provider who created the model
	Encoding      string   // BPE encoding name (e.g., "o200k_base", "cl100k_base")
	ContextWindow int      // Maximum context window size in tokens
}

// modelRegistry is the central registry of all supported models.
// Model data last updated: 2026-07-09.
// Context windows reflect each model's standard API tier.
var modelRegistry = map[string]ModelMetadata{
	// OpenAI Models - GPT-5 series (o200k_base)
	"gpt-5": {
		Name: "gpt-5", Provider: ProviderOpenAI, Encoding: "o200k_base",
		ContextWindow: 400000,
	},
	"gpt-5-mini": {
		Name: "gpt-5-mini", Provider: ProviderOpenAI, Encoding: "o200k_base",
		ContextWindow: 400000,
	},
	"gpt-5-nano": {
		Name: "gpt-5-nano", Provider: ProviderOpenAI, Encoding: "o200k_base",
		ContextWindow: 400000,
	},

	// OpenAI Models - GPT-5.1/5.2 series (o200k_base)
	"gpt-5.1": {
		Name: "gpt-5.1", Provider: ProviderOpenAI, Encoding: "o200k_base",
		ContextWindow: 400000,
	},
	"gpt-5.2": {
		Name: "gpt-5.2", Provider: ProviderOpenAI, Encoding: "o200k_base",
		ContextWindow: 400000,
	},

	// OpenAI Models - GPT-4.1 series (o200k_base, 1M context)
	"gpt-4.1": {
		Name: "gpt-4.1", Provider: ProviderOpenAI, Encoding: "o200k_base",
		ContextWindow: 1047576,
	},
	"gpt-4.1-mini": {
		Name: "gpt-4.1-mini", Provider: ProviderOpenAI, Encoding: "o200k_base",
		ContextWindow: 1047576,
	},
	"gpt-4.1-nano": {
		Name: "gpt-4.1-nano", Provider: ProviderOpenAI, Encoding: "o200k_base",
		ContextWindow: 1047576,
	},

	// OpenAI Models - GPT-4o series (o200k_base)
	"gpt-4o": {
		Name: "gpt-4o", Provider: ProviderOpenAI, Encoding: "o200k_base",
		ContextWindow: 128000,
	},
	"gpt-4o-mini": {
		Name: "gpt-4o-mini", Provider: ProviderOpenAI, Encoding: "o200k_base",
		ContextWindow: 128000,
	},

	// OpenAI Models - o-series (o200k_base)
	"o3": {
		Name: "o3", Provider: ProviderOpenAI, Encoding: "o200k_base",
		ContextWindow: 200000,
	},
	"o3-mini": {
		Name: "o3-mini", Provider: ProviderOpenAI, Encoding: "o200k_base",
		ContextWindow: 200000,
	},
	"o4-mini": {
		Name: "o4-mini", Provider: ProviderOpenAI, Encoding: "o200k_base",
		ContextWindow: 200000,
	},

	// OpenAI Models - Legacy (cl100k_base)
	"gpt-4": {
		Name: "gpt-4", Provider: ProviderOpenAI, Encoding: "cl100k_base",
		ContextWindow: 8192,
	},
	"gpt-4-turbo": {
		Name: "gpt-4-turbo", Provider: ProviderOpenAI, Encoding: "cl100k_base",
		ContextWindow: 128000,
	},
	"gpt-3.5-turbo": {
		Name: "gpt-3.5-turbo", Provider: ProviderOpenAI, Encoding: "cl100k_base",
		ContextWindow: 16385,
	},

	// Anthropic Models - Claude Opus (approximation).
	// Current generation (4.5/4.6) ships a 1M context window at standard tier.
	"claude-opus-4.6": {
		Name: "claude-opus-4.6", Provider: ProviderAnthropic, Encoding: "claude_approx",
		ContextWindow: 1000000,
	},
	"claude-opus-4.5": {
		Name: "claude-opus-4.5", Provider: ProviderAnthropic, Encoding: "claude_approx",
		ContextWindow: 1000000,
	},
	"claude-opus-4.1": {
		Name: "claude-opus-4.1", Provider: ProviderAnthropic, Encoding: "claude_approx",
		ContextWindow: 200000,
	},
	"claude-opus-4": {
		Name: "claude-opus-4", Provider: ProviderAnthropic, Encoding: "claude_approx",
		ContextWindow: 200000,
	},

	// Anthropic Models - Claude Sonnet (approximation).
	// Current generation (4.5/4.6) ships a 1M context window at standard tier.
	"claude-sonnet-4.6": {
		Name: "claude-sonnet-4.6", Provider: ProviderAnthropic, Encoding: "claude_approx",
		ContextWindow: 1000000,
	},
	"claude-sonnet-4.5": {
		Name: "claude-sonnet-4.5", Provider: ProviderAnthropic, Encoding: "claude_approx",
		ContextWindow: 1000000,
	},
	"claude-sonnet-4": {
		Name: "claude-sonnet-4", Provider: ProviderAnthropic, Encoding: "claude_approx",
		ContextWindow: 200000,
	},

	// Anthropic Models - Claude Haiku (approximation)
	"claude-haiku-4.5": {
		Name: "claude-haiku-4.5", Provider: ProviderAnthropic, Encoding: "claude_approx",
		ContextWindow: 200000,
	},
	"claude-haiku-3.5": {
		Name: "claude-haiku-3.5", Provider: ProviderAnthropic, Encoding: "claude_approx",
		ContextWindow: 200000,
	},
	"claude-haiku-3": {
		Name: "claude-haiku-3", Provider: ProviderAnthropic, Encoding: "claude_approx",
		ContextWindow: 200000,
	},

	// Anthropic Models - Legacy (deprecated)
	"claude-opus-3": {
		Name: "claude-opus-3", Provider: ProviderAnthropic, Encoding: "claude_approx",
		ContextWindow: 200000,
	},

	// Google Models - Gemini series (character-based approximation).
	// Gemini uses its own SentencePiece tokenizer; without a vocab file tcount
	// approximates at ~4 characters per token per Google's guidance.
	"gemini-2.5-pro": {
		Name: "gemini-2.5-pro", Provider: ProviderGoogle, Encoding: "gemini_approx",
		ContextWindow: 1000000,
	},
	"gemini-2.5-flash": {
		Name: "gemini-2.5-flash", Provider: ProviderGoogle, Encoding: "gemini_approx",
		ContextWindow: 1000000,
	},
	"gemini-2.5-flash-lite": {
		Name: "gemini-2.5-flash-lite", Provider: ProviderGoogle, Encoding: "gemini_approx",
		ContextWindow: 1000000,
	},

	// Meta Models - Llama series (cl100k_base BPE approximation)
	"llama-3.1-8b": {
		Name: "llama-3.1-8b", Provider: ProviderMeta, Encoding: "cl100k_base",
		ContextWindow: 128000,
	},
	"llama-3.1-70b": {
		Name: "llama-3.1-70b", Provider: ProviderMeta, Encoding: "cl100k_base",
		ContextWindow: 128000,
	},
	"llama-3.1-405b": {
		Name: "llama-3.1-405b", Provider: ProviderMeta, Encoding: "cl100k_base",
		ContextWindow: 128000,
	},
	"llama-4-scout": {
		Name: "llama-4-scout", Provider: ProviderMeta, Encoding: "cl100k_base",
		ContextWindow: 10000000,
	},
	"llama-4-maverick": {
		Name: "llama-4-maverick", Provider: ProviderMeta, Encoding: "cl100k_base",
		ContextWindow: 1000000,
	},

	// DeepSeek Models (cl100k_base BPE approximation)
	"deepseek-v2": {
		Name: "deepseek-v2", Provider: ProviderDeepSeek, Encoding: "cl100k_base",
		ContextWindow: 128000,
	},
	"deepseek-v3": {
		Name: "deepseek-v3", Provider: ProviderDeepSeek, Encoding: "cl100k_base",
		ContextWindow: 128000,
	},
	"deepseek-coder-v2": {
		Name: "deepseek-coder-v2", Provider: ProviderDeepSeek, Encoding: "cl100k_base",
		ContextWindow: 128000,
	},

	// Alibaba Models - Qwen 2/3 series (cl100k_base BPE compatible)
	"qwen-2.5-7b": {
		Name: "qwen-2.5-7b", Provider: ProviderAlibaba, Encoding: "cl100k_base",
		ContextWindow: 128000,
	},
	"qwen-2.5-14b": {
		Name: "qwen-2.5-14b", Provider: ProviderAlibaba, Encoding: "cl100k_base",
		ContextWindow: 128000,
	},
	"qwen-2.5-72b": {
		Name: "qwen-2.5-72b", Provider: ProviderAlibaba, Encoding: "cl100k_base",
		ContextWindow: 128000,
	},
	"qwen-3-72b": {
		Name: "qwen-3-72b", Provider: ProviderAlibaba, Encoding: "cl100k_base",
		ContextWindow: 128000,
	},

	// Microsoft Models - Phi-3 series (cl100k_base BPE compatible)
	"phi-3-mini": {
		Name: "phi-3-mini", Provider: ProviderMicrosoft, Encoding: "cl100k_base",
		ContextWindow: 128000,
	},
	"phi-3-small": {
		Name: "phi-3-small", Provider: ProviderMicrosoft, Encoding: "cl100k_base",
		ContextWindow: 128000,
	},
	"phi-3-medium": {
		Name: "phi-3-medium", Provider: ProviderMicrosoft, Encoding: "cl100k_base",
		ContextWindow: 128000,
	},
}

// GetModelMetadata retrieves metadata for a given model name.
// Returns nil if model is not found in the registry.
func GetModelMetadata(modelName string) *ModelMetadata {
	if meta, ok := modelRegistry[modelName]; ok {
		return &meta
	}
	return nil
}

// ListModels returns all registered model names in sorted order.
func ListModels() []string {
	models := make([]string, 0, len(modelRegistry))
	for name := range modelRegistry {
		models = append(models, name)
	}
	sort.Strings(models)
	return models
}

// ListModelsByProvider returns all models from a specific provider, sorted by name.
func ListModelsByProvider(provider Provider) []ModelMetadata {
	models := make([]ModelMetadata, 0)
	for _, meta := range modelRegistry {
		if meta.Provider == provider {
			models = append(models, meta)
		}
	}
	sort.Slice(models, func(i, j int) bool {
		return models[i].Name < models[j].Name
	})
	return models
}

// GetProviderForModel returns the provider for a given model name.
// Returns empty string if model is not registered.
func GetProviderForModel(modelName string) Provider {
	if meta := GetModelMetadata(modelName); meta != nil {
		return meta.Provider
	}
	return ""
}

// IsOpenSourceModel returns true if the model is from an open-source provider
// (not OpenAI, Anthropic, or Google).
func IsOpenSourceModel(modelName string) bool {
	provider := GetProviderForModel(modelName)
	return provider != "" &&
		provider != ProviderOpenAI &&
		provider != ProviderAnthropic &&
		provider != ProviderGoogle
}

// ModelsByEncoding returns a map of encoding name to sorted model names.
func ModelsByEncoding() map[string][]string {
	result := make(map[string][]string)
	for name, meta := range modelRegistry {
		result[meta.Encoding] = append(result[meta.Encoding], name)
	}
	for enc := range result {
		sort.Strings(result[enc])
	}
	return result
}
