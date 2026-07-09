// Command tcount is a fast, offline token counter for LLM workflows.
//
// It counts tokens in files and directories using exact OpenAI tiktoken
// encoders, Claude approximations, SentencePiece vocabularies, and generic
// estimation, and reports context-window usage and per-model cost estimates.
//
// Install:
//
//	go install github.com/lancekrogers/tcount/cmd/tcount@latest
//
// Run "tcount --help" for the full flag reference.
package main

import "github.com/lancekrogers/tcount/internal/commands"

func main() {
	commands.Execute(version)
}
