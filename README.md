# tcount

[![Go Reference](https://pkg.go.dev/badge/github.com/lancekrogers/tcount.svg)](https://pkg.go.dev/github.com/lancekrogers/tcount)
[![Go Report Card](https://goreportcard.com/badge/github.com/lancekrogers/tcount)](https://goreportcard.com/report/github.com/lancekrogers/tcount)
[![Release](https://img.shields.io/github/v/release/lancekrogers/tcount)](https://github.com/lancekrogers/tcount/releases/latest)
[![npm](https://img.shields.io/npm/v/@obedience-corp/tcount)](https://www.npmjs.com/package/@obedience-corp/tcount)
[![License: MIT](https://img.shields.io/badge/License-MIT-yellow.svg)](LICENSE)

A fast, zero-network token counter for LLM workflows. Count tokens in files and directories using exact OpenAI tokenizers, Claude approximations, SentencePiece vocabularies, and generic estimation — all from a single CLI.

## Features

- **Exact BPE tokenization** — offline, no network calls. Supports GPT-5, GPT-4.1, GPT-4o, o-series, and legacy GPT-4/3.5.
- **Claude approximation** calibrated for Anthropic models
- **SentencePiece** exact tokenization for Llama and other open-source models (bring your own `.model` file)
- **Context window usage** — see what percentage of a model's context you're consuming
- **Cost estimates** with per-1M-token pricing via `--cost`
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

# Specific model
tcount --model gpt-5 prompt.md

# All methods with cost estimates
tcount --all --cost prompt.md

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
| `claude-opus-4.6`, `claude-opus-4.5` | Approximation | 200K |
| `claude-opus-4.1`, `claude-opus-4` | Approximation | 200K |
| `claude-sonnet-4.6`, `claude-sonnet-4.5`, `claude-sonnet-4` | Approximation | 200K |
| `claude-haiku-4.5`, `claude-haiku-3.5`, `claude-haiku-3` | Approximation | 200K |
| `claude-opus-3` (deprecated) | Approximation | 200K |

### Meta (Llama)
| Model | Method | Context |
|-------|--------|---------|
| `llama-4-scout`, `llama-4-maverick` | tiktoken approx / SentencePiece | 128K |
| `llama-3.1-8b`, `llama-3.1-70b`, `llama-3.1-405b` | tiktoken approx / SentencePiece | 128K |

### DeepSeek
| Model | Method | Context |
|-------|--------|---------|
| `deepseek-v2`, `deepseek-v3`, `deepseek-coder-v2` | tiktoken approx | 128K |

### Alibaba (Qwen)
| Model | Method | Context |
|-------|--------|---------|
| `qwen-2.5-7b`, `qwen-2.5-14b`, `qwen-2.5-72b` | tiktoken approx | 32K |
| `qwen-3-72b` | tiktoken approx | 32K |

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
| `--model` | | Specific model tokenizer |
| `--models` | `-m` | Show encoding-to-model lookup table |
| `--provider` | | Filter by provider: `openai`, `anthropic`, `meta`, `deepseek`, `alibaba`, `microsoft`, `all` |
| `--vocab-file` | | Path to SentencePiece `.model` file for exact Llama tokenization |
| `--all` | | Show all counting methods |
| `--json` | | JSON output |
| `--cost` | | Include cost estimates (per 1M tokens) |
| `--recursive` | `-r` | Recursively count files in a directory |
| `--directory` | `-d` | Alias for `--recursive` |
| `--chars-per-token` | | Character/token ratio for approximation (default: 4.0) |
| `--words-per-token` | | Words/token ratio for approximation (default: 0.75) |
| `--verbose` | | Show additional details |
| `--no-color` | | Disable color output |

## Examples

### Single model

```
$ tcount --model gpt-5 document.md

Token Count Report for: document.md
═══════════════════════════════════════════════════════

Basic Statistics:
  Characters:     5451
  Words:          662
  Lines:          222

Token Counts by Method:
  ┌─────────────────────────┬──────────┬────────────┬──────────────────┐
  │ Method                  │ Tokens   │ Accuracy   │ Context Usage    │
  ├─────────────────────────┼──────────┼────────────┼──────────────────┤
  │ GPT (gpt-5)             │ 1445     │ Exact      │ 0.7% of 200K     │
  └─────────────────────────┴──────────┴────────────┴──────────────────┘
```

### All methods with costs

```
$ tcount --all --cost document.md

Token Count Report for: document.md
═══════════════════════════════════════════════════════

Basic Statistics:
  Characters:     5451
  Words:          662
  Lines:          222

Token Counts by Method:
  ┌─────────────────────────┬──────────┬────────────┬──────────────────┐
  │ Method                  │ Tokens   │ Accuracy   │ Context Usage    │
  ├─────────────────────────┼──────────┼────────────┼──────────────────┤
  │ GPT (gpt-5)             │ 1445     │ Exact      │ 0.7% of 200K     │
  │ GPT (gpt-4o)            │ 1445     │ Exact      │ 1.1% of 128K     │
  │ Claude (approx)         │ 1434     │ Estimated  │ 0.7% of 200K     │
  │ Llama (llama-3.1-8b)    │ 1445     │ Exact      │ 1.1% of 128K     │
  │ Character-based (÷4.0)  │ 1362     │ Approx     │                  │
  │ Word-based (×1.33)      │ 882      │ Approx     │                  │
  │ Whitespace split        │ 662      │ Approx     │                  │
  └─────────────────────────┴──────────┴────────────┴──────────────────┘

Cost Estimates (Input):
  gpt-5:           $0.0018 ($1.25/1M tokens)
  gpt-4o:          $0.0036 ($2.50/1M tokens)
  claude-sonnet-4.6: $0.0043 ($3.00/1M tokens)
  claude-sonnet-4.5: $0.0043 ($3.00/1M tokens)
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

Found 4 text files (skipped 0 binary, 0 ignored)
Token Count Report for: tokenizer/ (directory)
═══════════════════════════════════════════════════════

Basic Statistics:
  Files:          4
  Characters:     14929
  Words:          1906
  Lines:          612

Token Counts by Method:
  ┌─────────────────────────┬──────────┬────────────┬──────────────────┐
  │ Method                  │ Tokens   │ Accuracy   │ Context Usage    │
  ├─────────────────────────┼──────────┼────────────┼──────────────────┤
  │ GPT (gpt-5)             │ 4206     │ Exact      │ 2.1% of 200K     │
  │ Claude (approx)         │ 3928     │ Estimated  │ 2.0% of 200K     │
  │ Character-based (÷4.0)  │ 3732     │ Approx     │                  │
  │ Word-based (×1.33)      │ 2541     │ Approx     │                  │
  │ Whitespace split        │ 1906     │ Approx     │                  │
  └─────────────────────────┴──────────┴────────────┴──────────────────┘
```

When scanning directories, tcount respects `.gitignore` rules, skips binary files and `.git` directories, and aggregates all text files into a combined count. Use `--verbose` to see file and skip statistics.

### JSON output

```
$ tcount --json --model gpt-5 document.md
{
  "file_path": "document.md",
  "file_size": 5451,
  "characters": 5451,
  "words": 662,
  "lines": 222,
  "methods": [
    {
      "name": "tiktoken_gpt_5",
      "display_name": "GPT (gpt-5)",
      "tokens": 1445,
      "is_exact": true,
      "context_window": 200000
    }
  ]
}
```

```bash
# Extract a specific count
tcount --json myfile.txt | jq '.methods[] | select(.name == "tiktoken_gpt_5") | .tokens'

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

### Cost Estimation

```go
ctx := context.Background()
result, _ := counter.Count(ctx, text, "gpt-4o", false)
costs := tokenizer.CalculateCosts(result.Methods)
for _, c := range costs {
    fmt.Printf("%s: $%.4f\n", c.Model, c.Cost)
}
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
