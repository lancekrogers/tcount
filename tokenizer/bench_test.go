package tokenizer

import (
	"context"
	"strings"
	"testing"
)

const benchSeed = "The quick brown fox jumps over 12,345 lazy dogs near the riverbank. " +
	"func Count(ctx context.Context, text string) (int, error) { return len(text) / 4, nil }\n" +
	"Tokenização é rápida; 数える tokens across scripts, naïve façade, résumé. " +
	"HTTP/2 requests: GET /api/v1/models?limit=100&offset=0 -> 200 OK\n"

func benchText(size int) string {
	var b strings.Builder
	b.Grow(size + len(benchSeed))
	for b.Len() < size {
		b.WriteString(benchSeed)
	}
	return b.String()
}

func BenchmarkNewCounter(b *testing.B) {
	b.ReportAllocs()
	for b.Loop() {
		if _, err := NewCounter(CounterOptions{}); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCounterCountAll_512KB(b *testing.B) {
	c, err := NewCounter(CounterOptions{})
	if err != nil {
		b.Fatal(err)
	}
	text := benchText(512 << 10)
	ctx := context.Background()
	if _, err := c.Count(ctx, "warmup", "", true); err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(text)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := c.Count(ctx, text, "", true); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkCounterCountModelGPT5_512KB(b *testing.B) {
	c, err := NewCounter(CounterOptions{})
	if err != nil {
		b.Fatal(err)
	}
	text := benchText(512 << 10)
	ctx := context.Background()
	if _, err := c.Count(ctx, "warmup", "gpt-5", false); err != nil {
		b.Fatal(err)
	}
	b.SetBytes(int64(len(text)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := c.Count(ctx, text, "gpt-5", false); err != nil {
			b.Fatal(err)
		}
	}
}
