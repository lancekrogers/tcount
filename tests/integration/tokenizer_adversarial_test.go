package integration_test

import (
	"strings"
	"testing"

	"github.com/lancekrogers/tcount/tokenizer"
)

// Expected counts are pinned reference values captured from the
// implementation at main commit 31ca4d6, before any encode-path changes.
// If one of these fails, the tokenizer's standardized output has drifted;
// fix the code, do not regenerate the fixtures from the current build.
func TestIntegrationTokenizer_AdversarialExactCounts(t *testing.T) {
	tests := []struct {
		name   string
		text   string
		o200k  int
		cl100k int
	}{
		{"plain_prose_and_code", "normal prose with numbers 12,345 and code func main() {}\n", 14, 14},
		{"literal_special_tokens", "text containing <|endoftext|> and <|fim_prefix|> literal special tokens <|endofprompt|>\n", 26, 24},
		{"unicode_cjk_emoji_combining", "日本語のテキスト 🎉🚀 émojis and ñ açcénts мир s̈\n", 23, 29},
		{"crlf_line_endings", "crlf line one\r\nline two\r\n", 8, 9},
		{"no_trailing_newline", "no trailing newline", 3, 3},
		{"only_newline", "\n", 1, 1},
		{"large_repeated", strings.Repeat("lorem ipsum dolor sit amet ", 2000) + "\n", 10002, 10002},
	}

	o200k, err := tokenizer.NewBPETokenizerByEncoding("o200k_base")
	if err != nil {
		t.Fatal(err)
	}
	cl100k, err := tokenizer.NewBPETokenizerByEncoding("cl100k_base")
	if err != nil {
		t.Fatal(err)
	}

	for _, tc := range tests {
		t.Run(tc.name, func(t *testing.T) {
			got, err := o200k.CountTokens(tc.text)
			if err != nil {
				t.Fatalf("o200k CountTokens() error: %v", err)
			}
			if got != tc.o200k {
				t.Errorf("o200k_base tokens = %d, want %d", got, tc.o200k)
			}

			got, err = cl100k.CountTokens(tc.text)
			if err != nil {
				t.Fatalf("cl100k CountTokens() error: %v", err)
			}
			if got != tc.cl100k {
				t.Errorf("cl100k_base tokens = %d, want %d", got, tc.cl100k)
			}
		})
	}
}
