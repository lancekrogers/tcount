package tokenizer

import (
	"context"
	"errors"
	"reflect"
	"testing"
)

func TestAggregateFileResultsPreservesPerFileTotalsAndMethodOrder(t *testing.T) {
	counter := &Counter{charsPerToken: 4, wordsPerToken: 0.75}
	plans := []methodPlan{
		{name: "first", displayName: "First", isExact: true, contextWindow: 128},
		{name: "second", displayName: "Second", isExact: false, contextWindow: 256},
	}
	results := []perFileResult{
		{
			FileSize:       10,
			Characters:     8,
			Words:          2,
			Lines:          1,
			Classification: fileClassificationText,
			Methods: []MethodResult{
				{Name: "first", Tokens: 3},
				{Name: "second", Tokens: 4},
			},
			MethodPresent: []bool{true, true},
		},
		{
			FileSize:       12,
			Characters:     8,
			Words:          3,
			Lines:          2,
			Classification: fileClassificationText,
			Methods: []MethodResult{
				{Name: "first", Tokens: 5},
				{Name: "second", Tokens: 7},
			},
			MethodPresent: []bool{true, true},
		},
	}

	got, err := counter.aggregateFileResults(context.Background(), results, plans, true)
	if err != nil {
		t.Fatal(err)
	}
	if got.FileSize != 22 || got.Characters != 16 || got.Words != 5 || got.Lines != 3 || got.FileCount != 2 {
		t.Fatalf("totals = %+v, want size=22 chars=16 words=5 lines=3 files=2", got)
	}
	wantMethods := []MethodResult{
		{Name: "first", DisplayName: "First", Tokens: 8, IsExact: true, ContextWindow: 128},
		{Name: "second", DisplayName: "Second", Tokens: 11, IsExact: false, ContextWindow: 256},
		{Name: "character_based_div4", DisplayName: "Character-based (÷4.0)", Tokens: 4, IsExact: false},
		{Name: "word_based_mul133", DisplayName: "Word-based (×1.33)", Tokens: 6, IsExact: false},
		{Name: "whitespace_split", DisplayName: "Whitespace split", Tokens: 5, IsExact: false},
	}
	if !reflect.DeepEqual(got.Methods, wantMethods) {
		t.Fatalf("methods = %#v, want %#v", got.Methods, wantMethods)
	}
}

func TestAggregateFileResultsOmitsMethodWithPerFileFailure(t *testing.T) {
	plans := []methodPlan{
		{name: "stable", displayName: "Stable", isExact: true},
		{name: "sometimes-fails", displayName: "Sometimes fails"},
	}
	results := []perFileResult{
		{FileSize: 4, Characters: 4, Words: 1, Lines: 1, Methods: []MethodResult{{Tokens: 2}, {}}, MethodPresent: []bool{true, false}},
		{FileSize: 5, Characters: 5, Words: 1, Lines: 1, Methods: []MethodResult{{Tokens: 3}, {Tokens: 9}}, MethodPresent: []bool{true, true}},
	}

	got, err := (&Counter{}).aggregateFileResults(context.Background(), results, plans, false)
	if err != nil {
		t.Fatal(err)
	}
	want := []MethodResult{{Name: "stable", DisplayName: "Stable", Tokens: 5, IsExact: true}}
	if !reflect.DeepEqual(got.Methods, want) {
		t.Fatalf("methods = %#v, want %#v", got.Methods, want)
	}
}

func TestAggregateFileResultsRejectsCancellationAndMalformedMethodShape(t *testing.T) {
	canceled, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := (&Counter{}).aggregateFileResults(canceled, nil, nil, false); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled aggregation error = %v, want context.Canceled", err)
	}

	plans := []methodPlan{{name: "one"}}
	_, err := (&Counter{}).aggregateFileResults(context.Background(), []perFileResult{{Methods: nil, MethodPresent: nil}}, plans, false)
	if err == nil {
		t.Fatal("malformed per-file result unexpectedly succeeded")
	}
}
