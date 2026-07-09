# tcount

[![Go Reference](https://pkg.go.dev/badge/github.com/lancekrogers/tcount.svg)](https://pkg.go.dev/github.com/lancekrogers/tcount)
[![Go Report Card](https://goreportcard.com/badge/github.com/lancekrogers/tcount)](https://goreportcard.com/report/github.com/lancekrogers/tcount)
[![Release](https://img.shields.io/github/v/release/lancekrogers/tcount)](https://github.com/lancekrogers/tcount/releases/latest)
[![npm](https://img.shields.io/npm/v/@obedience-corp/tcount)](https://www.npmjs.com/package/@obedience-corp/tcount)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A fast, zero-network token counter for LLM workflows. Count tokens in files and directories using exact OpenAI tokenizers, Claude and Gemini approximations, SentencePiece vocabularies, and generic estimation — all from a single CLI.

## Features

- **Exact BPE tokenization** — offline, no network calls. Supports GPT-5, GPT-4.1, GPT-4o, o-series, and legacy GPT-4/3.5.
- **Claude approximation** calibrated for Anthropic models
- **Gemini approximation** for Google Gemini models
- **SentencePiece** exact tokenization for Llama and other open-source models (bring your own `.model` file)
- **Context window usage** — see what percentage of a model's context you're consuming (shown when you pass `--model`)
- **Provider filtering** — compare models from a specific provider
- **Directory scanning** with `.gitignore` support and binary file detection
- **JSON output** for scripting and pipelines

## Install

### npm / pnpm / bun (macOS & Linux)

```bash
npm install -g @obedience-corp/tcount
# or
pnpm add -g @obedience-corp/tcount
# or
bun add -g @obedience-corp/tcount
```

The npm package downloads the official release binary for your platform (with checksum verification) on first install.

### Homebrew (macOS & Linux)

```bash
brew install lancekrogers/tap/tcount
```

### Go

```bash
go install github.com/lancekrogers/tcount/cmd/tcount@latest
```

### From source

```bash
git clone https://github.com/lancekrogers/tcount.git
cd tcount
go build -o bin/tcount ./cmd/tcount
```

### Binary releases

Pre-built binaries for macOS, Linux, and Windows are available on the [releases page](https://github.com/lancekrogers/tcount/releases).

## Quick Start

```bash
# Count tokens in a file
tcount myfile.txt

# Specific model (shows context-window usage)
tcount --model gpt-5 prompt.md

# All counting methods
tcount --all prompt.md

# Filter by provider
tcount --provider openai prompt.md

# Recursive directory count
tcount -r ./src

# JSON output for scripting
tcount --json document.md
```

## Supported Models

### OpenAI
| Model | Encoding | Context |
|-------|----------|---------|
| `gpt-5`, `gpt-5-mini`, `gpt-5-nano` | o200k_base | 400K |
| `gpt-5.1`, `gpt-5.2` | o200k_base | 400K |
| `gpt-4.1`, `gpt-4.1-mini`, `gpt-4.1-nano` | o200k_base | 1M |
| `gpt-4o`, `gpt-4o-mini` | o200k_base | 128K |
| `o3`, `o3-mini`, `o4-mini` | o200k_base | 200K |
| `gpt-4`, `gpt-4-turbo` | cl100k_base | 8K–128K |
| `gpt-3.5-turbo` | cl100k_base | 16K |

### Anthropic
| Model | Method | Context |
|-------|--------|---------|
| `claude-opus-4.6`, `claude-opus-4.5` | Approximation | 1M |
| `claude-opus-4.1`, `claude-opus-4` | Approximation | 200K |
| `claude-sonnet-4.6`, `claude-sonnet-4.5` | Approximation | 1M |
| `claude-sonnet-4` | Approximation | 200K |
| `claude-haiku-4.5`, `claude-haiku-3.5`, `claude-haiku-3` | Approximation | 200K |
| `claude-opus-3` (deprecated) | Approximation | 200K |

### Google
| Model | Method | Context |
|-------|--------|---------|
| `gemini-2.5-pro`, `gemini-2.5-flash`, `gemini-2.5-flash-lite` | Approximation | 1M |

Gemini uses its own SentencePiece tokenizer. Without a `--vocab-file`, tcount approximates at ~4 characters per token.

### Meta (Llama)
| Model | Method | Context |
|-------|--------|---------|
| `llama-4-scout` | tiktoken approx / SentencePiece | 10M |
| `llama-4-maverick` | tiktoken approx / SentencePiece | 1M |
| `llama-3.1-8b`, `llama-3.1-70b`, `llama-3.1-405b` | tiktoken approx / SentencePiece | 128K |

### DeepSeek
| Model | Method | Context |
|-------|--------|---------|
| `deepseek-v2`, `deepseek-v3`, `deepseek-coder-v2` | tiktoken approx | 128K |

### Alibaba (Qwen)
| Model | Method | Context |
|-------|--------|---------|
| `qwen-2.5-7b`, `qwen-2.5-14b`, `qwen-2.5-72b` | tiktoken approx | 128K |
| `qwen-3-72b` | tiktoken approx | 128K |

### Microsoft (Phi)
| Model | Method | Context |
|-------|--------|---------|
| `phi-3-mini`, `phi-3-small`, `phi-3-medium` | tiktoken approx | 128K |

## Tokenization Methods

| Method | Accuracy | When Used |
|--------|----------|-----------|
| tiktoken (o200k_base) | Exact | GPT-5.x, GPT-4.1, GPT-4o, o3, o4-mini |
| tiktoken (cl100k_base) | Exact | GPT-4, GPT-3.5 |
| Claude approximation | Estimated | All Claude models (÷3.8 char ratio) |
| Gemini approximation | Approximate | All Gemini models (÷4.0 char ratio) |
| SentencePiece | Exact | Llama with `--vocab-file` |
| tiktoken approximation | Approximate | Llama, DeepSeek, Qwen, Phi (no vocab file) |
| Character-based | Approximate | Any (chars ÷ configurable ratio, default 4.0) |
| Word-based | Approximate | Any (words × configurable multiplier, default 1.33) |
| Whitespace split | Approximate | Any (raw word count as lower bound) |

## Usage

```
tcount [file|directory] [flags]
```

### Flags

| Flag | Short | Description |
|------|-------|-------------|
| `--model` | | Specific model tokenizer (adds a context-usage column) |
| `--models` | `-m` | Show encoding-to-model lookup table |
| `--provider` | | Filter by provider: `openai`, `anthropic`, `google`, `meta`, `deepseek`, `alibaba`, `microsoft`, `all` |
| `--vocab-file` | | Path to SentencePiece `.model` file for exact Llama tokenization |
| `--all` | | Show all counting methods |
| `--json` | | JSON output |
| `--recursive` | `-r` | Recursively count files in a directory |
| `--directory` | `-d` | Alias for `--recursive` |
| `--chars-per-token` | | Character/token ratio for approximation (default: 4.0) |
| `--words-per-token` | | Words/token ratio for approximation (default: 0.75) |
| `--verbose` | | Show additional details |
| `--no-color` | | Disable color output |

## Examples

### Single model

Passing `--model` shows the exact (or approximate) count for that model plus
how much of its context window the text uses:

```
$ tcount --model gpt-5 tokenizer.go

Token Count Report for: tokenizer.go

Basic Statistics
  Characters: 7,397
  Words: 912
  Lines: 242

Token Counts by Method
╭────────────────────┬────────┬──────────┬───────────────╮
│       Method       │ Tokens │ Accuracy │ Context Usage │
├────────────────────┼────────┼──────────┼───────────────┤
│ o200k_base (gpt-5) │  1,839 │ Exact    │ 0.46% of 400K │
╰────────────────────┴────────┴──────────┴───────────────╯
```

### All methods

```
$ tcount --all tokenizer.go

Token Count Report for: tokenizer.go

Basic Statistics
  Characters: 7,397
  Words: 912
  Lines: 242

Token Counts by Method
╭────────────────────────┬────────┬───────────╮
│         Method         │ Tokens │ Accuracy  │
├────────────────────────┼────────┼───────────┤
│ cl100k_base            │  1,835 │ Exact     │
│ Claude (approx)        │  1,946 │ Estimated │
│ Gemini (approx)        │  1,849 │ Approx    │
│ o200k_base             │  1,839 │ Exact     │
│ Character-based (÷4.0) │  1,849 │ Approx    │
│ Word-based (×1.33)     │  1,216 │ Approx    │
│ Whitespace split       │    912 │ Approx    │
╰────────────────────────┴────────┴───────────╯
```

### SentencePiece for exact Llama tokenization

```bash
# Download tokenizer.model from HuggingFace (requires auth):
# https://huggingface.co/meta-llama/Llama-3.1-8B/blob/main/original/tokenizer.model

tcount --model llama-3.1-8b --vocab-file /path/to/tokenizer.model document.md
```

Without `--vocab-file`, Llama models use a tiktoken-based approximation.

### Directory scanning

```
$ tcount -r --verbose tokenizer/

Found 11 text files (skipped 4 binary, 0 ignored)
Token Count Report for: tokenizer/ (directory)

Basic Statistics
  Files: 11
  Characters: 49,279
  Words: 6,511
  Lines: 1,841

Token Counts by Method
╭────────────────────────┬────────┬───────────╮
│         Method         │ Tokens │ Accuracy  │
├────────────────────────┼────────┼───────────┤
│ cl100k_base            │ 14,301 │ Exact     │
│ Claude (approx)        │ 12,968 │ Estimated │
│ Gemini (approx)        │ 12,319 │ Approx    │
│ o200k_base             │ 14,242 │ Exact     │
│ Character-based (÷4.0) │ 12,319 │ Approx    │
│ Word-based (×1.33)     │  8,681 │ Approx    │
│ Whitespace split       │  6,511 │ Approx    │
╰────────────────────────┴────────┴───────────╯
```

When scanning directories, tcount respects `.gitignore` rules, skips binary files and `.git` directories, and aggregates all text files into a combined count. Use `--verbose` to see file and skip statistics.

### JSON output

```
$ tcount --json --model gpt-5 tokenizer.go
{
  "file_path": "tokenizer.go",
  "file_size": 7397,
  "characters": 7397,
  "words": 912,
  "lines": 242,
  "methods": [
    {
      "name": "bpe_gpt_5",
      "display_name": "o200k_base (gpt-5)",
      "tokens": 1839,
      "is_exact": true,
      "context_window": 400000
    }
  ]
}
```

```bash
# Extract a specific count
tcount --json myfile.txt | jq '.methods[] | select(.name == "bpe_gpt_5") | .tokens'

# Batch count all markdown files
for f in docs/*.md; do tcount --json "$f"; done | jq -s '.'
```

## Library Usage

tcount can be used as a Go library in your own projects.

### Installation

```bash
go get github.com/lancekrogers/tcount/tokenizer
```

### Basic Token Counting

```go
package main

import (
    "context"
    "fmt"
    "log"

    "github.com/lancekrogers/tcount/tokenizer"
)

func main() {
    counter, err := tokenizer.NewCounter(tokenizer.CounterOptions{})
    if err != nil {
        log.Fatal(err)
    }

    ctx := context.Background()
    result, err := counter.Count(ctx, "Hello, world!", "gpt-4o", false)
    if err != nil {
        log.Fatal(err)
    }

    for _, m := range result.Methods {
        if m.IsExact {
            fmt.Printf("Tokens: %d (exact, %s)\n", m.Tokens, m.DisplayName)
        }
    }
}
```

### File and Directory Counting

```go
ctx := context.Background()

// Count tokens in a single file
result, err := counter.CountFile(ctx, "document.md", "gpt-4o", false)

// Count tokens across a directory (respects .gitignore, skips binaries)
result, err := counter.CountDirectory(ctx, "./src", "", true)
fmt.Printf("Files: %d, Tokens: %d\n", result.FileCount, result.Methods[0].Tokens)
```

### Direct BPE Tokenizer Access

```go
tok, err := tokenizer.NewBPETokenizer("gpt-4o")
if err != nil {
    log.Fatal(err)
}

count, _ := tok.CountTokens("Hello, world!")
fmt.Printf("Tokens: %d, Exact: %v\n", count, tok.IsExact())
```

### Model Discovery

```go
// Get metadata for a specific model
meta := tokenizer.GetModelMetadata("gpt-4o")
fmt.Printf("Encoding: %s, Context: %d\n", meta.Encoding, meta.ContextWindow)

// List all registered models
models := tokenizer.ListModels()

// List models by provider
openaiModels := tokenizer.ListModelsByProvider(tokenizer.ProviderOpenAI)
```

## Development

Requires [just](https://github.com/casey/just) for the build system.

```bash
just                       # List all recipes
just build                 # Build (with fmt + vet)
just test all              # Run all tests
just test unit             # Unit tests only
just test integration      # Integration tests only
just test coverage         # Coverage report
just test bench            # Benchmarks
just release all            # Cross-compile for all platforms
```

## License

MIT License. See [LICENSE](LICENSE) for details.
