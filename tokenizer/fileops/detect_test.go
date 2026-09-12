package fileops

import (
	"bytes"
	"strings"
	"testing"
)

// onnxHeader is the first 64 bytes of a real ONNX model: protobuf field tags
// and varint lengths interleaved with tensor names, without a single null
// byte. Content like this reached the tokenizer before model extensions and
// the non-text scan existed.
var onnxHeader = []byte{
	0x08, 0x07, 0x12, 0x07, 0x70, 0x79, 0x74, 0x6f, 0x72, 0x63, 0x68, 0x1a,
	0x05, 0x32, 0x2e, 0x36, 0x2e, 0x30, 0x3a, 0xc9, 0xe3, 0xa2, 0x9b, 0x01,
	0x0a, 0x8b, 0x01, 0x0a, 0x37, 0x6b, 0x6d, 0x6f, 0x64, 0x65, 0x6c, 0x2e,
	0x64, 0x65, 0x63, 0x6f, 0x64, 0x65, 0x72, 0x2e, 0x67, 0x65, 0x6e, 0x65,
	0x72, 0x61, 0x74, 0x6f, 0x72, 0x2e, 0x6e, 0x6f, 0x69, 0x73, 0x65, 0x5f,
	0x72, 0x65, 0x73, 0x2e,
}

// machOHeader is the leading fields of a 64-bit Mach-O executable.
var machOHeader = []byte{
	0xcf, 0xfa, 0xed, 0xfe, 0x0c, 0x00, 0x00, 0x01, 0x00, 0x00, 0x00, 0x00,
	0x02, 0x00, 0x00, 0x00, 0x11, 0x00, 0x00, 0x00, 0xe0, 0x06, 0x00, 0x00,
	0x85, 0x00, 0x20, 0x00, 0x00, 0x00, 0x00, 0x00,
}

// onnxSniffWindow fills a whole sniff window with protobuf framing by
// repeating the real header, mirroring how tensor metadata fills the start of
// a model file.
func onnxSniffWindow() []byte {
	repeats := binarySniffBytes/len(onnxHeader) + 1
	return bytes.Repeat(onnxHeader, repeats)[:binarySniffBytes]
}

// latin1Paragraph returns prose encoded in Latin-1, where each accented
// character is a lone high byte that is not valid UTF-8.
func latin1Paragraph() []byte {
	const accent = 0xe9 // é in Latin-1
	parts := []string{
		"Le d", "veloppement du syst", "me a ", "tÿ",
		" achev", " apr", "s une p", "riode de tests d", "taill",
		"s par l'équipe.\n",
	}
	var buf bytes.Buffer
	for _, part := range parts {
		buf.WriteString(part)
		buf.WriteByte(accent)
	}
	return buf.Bytes()
}

// truncatedRuneWindow returns content whose sniff window ends part-way through
// a three-byte rune, so the boundary itself must not be read as evidence.
func truncatedRuneWindow() []byte {
	const cjk = "世"
	buf := bytes.NewBufferString(strings.Repeat("a", binarySniffBytes-2))
	buf.WriteString(cjk)
	buf.WriteString(strings.Repeat(cjk, 4))
	return buf.Bytes()
}

func TestIsBinaryContentPreservesExtensionAndPrefixRules(t *testing.T) {
	contentAfterPrefix := make([]byte, binarySniffBytes+1)
	for index := range contentAfterPrefix {
		contentAfterPrefix[index] = 'a'
	}
	contentAfterPrefix[binarySniffBytes] = 0
	tests := []struct {
		name    string
		path    string
		content []byte
		want    bool
	}{
		{name: "binary extension", path: "image.png", content: []byte("plain text"), want: true},
		{name: "null in prefix", path: "data.txt", content: []byte{'a', 0, 'b'}, want: true},
		{name: "ordinary text", path: "readme.md", content: []byte("ordinary text\n"), want: false},
		{name: "null after prefix", path: "large.txt", content: contentAfterPrefix, want: false},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsBinaryContent(test.path, test.content); got != test.want {
				t.Fatalf("IsBinaryContent(%q) = %t, want %t", test.path, got, test.want)
			}
		})
	}
}

func TestIsBinaryContentSeparatesSerializedDataFromText(t *testing.T) {
	tests := []struct {
		name    string
		path    string
		content []byte
		want    bool
	}{
		{
			name:    "onnx protobuf header without nulls",
			path:    "model.data",
			content: onnxSniffWindow(),
			want:    true,
		},
		{
			name:    "onnx header alone",
			path:    "weights.unknown",
			content: onnxHeader,
			want:    true,
		},
		{
			name:    "mach-o executable header",
			path:    "tcount",
			content: machOHeader,
			want:    true,
		},
		{
			name:    "onnx extension with ascii content",
			path:    "ascii.onnx",
			content: []byte("this file is entirely printable ASCII\n"),
			want:    true,
		},
		{
			name:    "go source",
			path:    "main.go",
			content: []byte("package main\n\nimport \"fmt\"\n\nfunc main() {\n\tfmt.Println(\"hello\")\n}\n"),
			want:    false,
		},
		{
			name:    "utf-8 with emoji and cjk",
			path:    "notes.md",
			content: []byte("# 世界 notes\n\nShipped 🌍 today — 日本語 and français both render.\n"),
			want:    false,
		},
		{
			name:    "latin-1 prose",
			path:    "rapport.txt",
			content: latin1Paragraph(),
			want:    false,
		},
		{
			name:    "markdown with ansi escape sequences",
			path:    "transcript.md",
			content: []byte("# Run log\n\n\x1b[1;32mPASS\x1b[0m ok\tgithub.com/lancekrogers/tcount\t0.4s\n\x1b[31mFAIL\x1b[0m tokenizer\n"),
			want:    false,
		},
		{
			name:    "empty file",
			path:    "empty.txt",
			content: []byte{},
			want:    false,
		},
		{
			name:    "rune truncated by the sniff window",
			path:    "cjk.txt",
			content: truncatedRuneWindow(),
			want:    false,
		},
	}

	for _, test := range tests {
		t.Run(test.name, func(t *testing.T) {
			if got := IsBinaryContent(test.path, test.content); got != test.want {
				counts := scanNonText(test.content[:min(len(test.content), binarySniffBytes)])
				t.Fatalf("IsBinaryContent(%q) = %t, want %t (control=%d non-text=%d window=%d)",
					test.path, got, test.want, counts.control, counts.total,
					min(len(test.content), binarySniffBytes))
			}
		})
	}
}

func TestBinaryExtensionsCoverModelAndSerializedFormats(t *testing.T) {
	extensions := []string{
		".onnx", ".safetensors", ".gguf", ".ggml", ".pt", ".pth", ".ckpt",
		".h5", ".hdf5", ".npy", ".npz", ".pb", ".tflite", ".mlmodel", ".ort",
		".parquet", ".arrow", ".feather", ".sqlite", ".sqlite3", ".db",
		".wasm", ".pkl", ".pickle", ".joblib", ".msgpack", ".bson",
	}

	for _, extension := range extensions {
		t.Run(extension, func(t *testing.T) {
			if !IsBinaryContent("weights"+extension, []byte("printable\n")) {
				t.Fatalf("extension %q should be treated as binary", extension)
			}
			if !IsBinaryContent("WEIGHTS"+strings.ToUpper(extension), []byte("printable\n")) {
				t.Fatalf("extension %q should be treated as binary regardless of case", extension)
			}
		})
	}
}
