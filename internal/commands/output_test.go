package commands

import (
	"bytes"
	"regexp"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/lancekrogers/tcount/internal/ui"
	"github.com/lancekrogers/tcount/tokenizer"
)

var cursorUpRE = regexp.MustCompile(`\x1b\[[0-9]*A`)

type synchronizedBuffer struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (b *synchronizedBuffer) Write(data []byte) (int, error) {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.Write(data)
}

func (b *synchronizedBuffer) String() string {
	b.mu.Lock()
	defer b.mu.Unlock()
	return b.buf.String()
}

func typicalDirectoryResult() *tokenizer.CountResult {
	return &tokenizer.CountResult{
		FilePath:    ".",
		IsDirectory: true,
		FileCount:   80,
		Characters:  446_000,
		Words:       96_000,
		Lines:       16_000,
		Methods: []tokenizer.MethodResult{
			{Name: tokenizer.EncodingCL100kBase, DisplayName: "cl100k_base", Tokens: 128_000, IsExact: true},
			{Name: tokenizer.NameClaudeApprox, DisplayName: "Claude (approx)", Tokens: 117_320, IsExact: false},
			{Name: "gemini_approx", DisplayName: "Gemini (approx)", Tokens: 111_500, IsExact: false},
			{Name: tokenizer.EncodingO200kBase, DisplayName: "o200k_base", Tokens: 128_000, IsExact: true},
			{Name: "char_div4", DisplayName: "Character-based (÷4.0)", Tokens: 111_500, IsExact: false},
			{Name: "word_based_mul133", DisplayName: "Word-based (×1.33)", Tokens: 128_000, IsExact: false},
			{Name: "whitespace_split", DisplayName: "Whitespace split", Tokens: 96_000, IsExact: false},
		},
	}
}

func TestRenderMethodTableIncludesBottomBorder(t *testing.T) {
	t.Parallel()

	rows, showContext := methodRows(typicalDirectoryResult())
	got := renderMethodTable(rows, showContext).String()
	if !strings.Contains(got, "Whitespace split") {
		t.Fatalf("table missing last method row:\n%s", got)
	}
	if !strings.Contains(got, "╰") || !strings.Contains(got, "╯") {
		t.Fatalf("table missing bottom border:\n%s", got)
	}
}

func TestPresentCountResultKeepsBottomBorderOnSharedStream(t *testing.T) {
	var buf synchronizedBuffer
	oldWriter := reportWriter
	t.Cleanup(func() { reportWriter = oldWriter })
	reportWriter = &buf

	p := ui.NewProgress(ui.ProgressOptions{
		Out:        &buf,
		Root:       ".",
		FilesTotal: 80,
		PaintDelay: 5 * time.Millisecond,
		NoColor:    true,
	})
	p.Arm()
	p.OnProgress(tokenizer.ProgressUpdate{
		FilesTotal: 80,
		FilesDone:  48,
		Characters: 266_800,
		Words:      57_600,
		Lines:      9_600,
		LastPath:   "f047.txt",
	})
	time.Sleep(40 * time.Millisecond)

	err := presentCountResult(
		typicalDirectoryResult(),
		".",
		nil,
		true,
		&countOptions{},
		ui.New(true, false),
		p.Stop,
	)
	if err != nil {
		t.Fatalf("presentCountResult: %v", err)
	}

	out := buf.String()
	if !strings.Contains(out, "Whitespace split") {
		t.Fatalf("report missing last method row:\n%s", out)
	}
	bottom := strings.LastIndex(out, "╰")
	if bottom < 0 {
		t.Fatalf("report missing table bottom border:\n%s", out)
	}
	if cursorUpRE.MatchString(out[bottom:]) {
		t.Fatalf("progress cursor-up ran after the table bottom border (TTY clip bug):\n%s", out)
	}
}

func TestLateProgressStopClipsTableBottomOnSharedStream(t *testing.T) {
	var buf synchronizedBuffer
	oldWriter := reportWriter
	t.Cleanup(func() { reportWriter = oldWriter })
	reportWriter = &buf

	p := ui.NewProgress(ui.ProgressOptions{
		Out:        &buf,
		Root:       ".",
		FilesTotal: 80,
		PaintDelay: 5 * time.Millisecond,
		NoColor:    true,
	})
	p.Arm()
	p.OnProgress(tokenizer.ProgressUpdate{
		FilesTotal: 80,
		FilesDone:  48,
		Characters: 10,
		Words:      2,
		Lines:      1,
		LastPath:   "a.txt",
	})
	time.Sleep(40 * time.Millisecond)

	if err := writeCountOutput(typicalDirectoryResult(), &countOptions{}); err != nil {
		t.Fatalf("writeCountOutput: %v", err)
	}
	p.Stop()

	out := buf.String()
	bottom := strings.LastIndex(out, "╰")
	if bottom < 0 {
		t.Fatal("expected table bottom border in the byte stream before the late clear")
	}
	if !cursorUpRE.MatchString(out[bottom:]) {
		t.Fatalf("expected late Stop to emit cursor-up after the table (documents the TTY clip bug):\n%s", out)
	}
}
