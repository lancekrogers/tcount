package bpe

import (
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

func benchmarkEncodeOrdinary(b *testing.B, encoding string, size int) {
	tok, err := NewEncoderByName(encoding)
	if err != nil {
		b.Fatal(err)
	}
	text := benchText(size)
	b.SetBytes(int64(len(text)))
	b.ReportAllocs()
	for b.Loop() {
		tok.EncodeOrdinary(text)
	}
}

func benchmarkEncode(b *testing.B, encoding string, size int) {
	tok, err := NewEncoderByName(encoding)
	if err != nil {
		b.Fatal(err)
	}
	text := benchText(size)
	b.SetBytes(int64(len(text)))
	b.ReportAllocs()
	for b.Loop() {
		if _, err := tok.Encode(text, nil, nil); err != nil {
			b.Fatal(err)
		}
	}
}

func BenchmarkEncodeOrdinary_O200k_4KB(b *testing.B) {
	benchmarkEncodeOrdinary(b, EncodingO200kBase, 4<<10)
}

func BenchmarkEncodeOrdinary_O200k_512KB(b *testing.B) {
	benchmarkEncodeOrdinary(b, EncodingO200kBase, 512<<10)
}

func BenchmarkEncodeOrdinary_CL100k_512KB(b *testing.B) {
	benchmarkEncodeOrdinary(b, EncodingCL100kBase, 512<<10)
}

func BenchmarkEncode_O200k_4KB(b *testing.B) {
	benchmarkEncode(b, EncodingO200kBase, 4<<10)
}

func BenchmarkEncode_O200k_512KB(b *testing.B) {
	benchmarkEncode(b, EncodingO200kBase, 512<<10)
}

func BenchmarkNewEncoderByName_O200k(b *testing.B) {
	if _, err := NewEncoderByName(EncodingO200kBase); err != nil {
		b.Fatal(err)
	}
	b.ReportAllocs()
	for b.Loop() {
		if _, err := NewEncoderByName(EncodingO200kBase); err != nil {
			b.Fatal(err)
		}
	}
}
