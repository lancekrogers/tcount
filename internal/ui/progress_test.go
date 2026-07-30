package ui

import (
	"bytes"
	"os"
	"strings"
	"testing"
	"time"

	"github.com/lancekrogers/tcount/tokenizer"
)

func TestShouldShowProgress(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name        string
		isDirectory bool
		json        bool
		noProgress  bool
		want        bool
	}{
		{name: "directory ok", isDirectory: true, want: true},
		{name: "file", isDirectory: false, want: false},
		{name: "json", isDirectory: true, json: true, want: false},
		{name: "no-progress", isDirectory: true, noProgress: true, want: false},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Non-nil *os.File that is not a TTY: force false when gates pass.
			// When gates fail, result is false regardless of TTY.
			got := ShouldShowProgress(tc.isDirectory, tc.json, tc.noProgress, nil)
			if got {
				t.Fatalf("with nil stderr want false, got true")
			}
			if !tc.isDirectory || tc.json || tc.noProgress {
				if ShouldShowProgress(tc.isDirectory, tc.json, tc.noProgress, os.Stderr) && tc.want {
					// may be true only if stderr is a TTY and gates pass
				}
				if tc.json || tc.noProgress || !tc.isDirectory {
					if ShouldShowProgress(tc.isDirectory, tc.json, tc.noProgress, os.Stderr) {
						t.Fatalf("expected false for blocked gates")
					}
				}
			}
		})
	}
}

func TestProgressDeferredPaintNoFlash(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	p := NewProgress(ProgressOptions{
		Out:        &buf,
		Root:       "./src",
		FilesTotal: 10,
		PaintDelay: 50 * time.Millisecond,
	})
	p.Arm()
	p.OnProgress(tokenizer.ProgressUpdate{
		FilesTotal: 10,
		FilesDone:  10,
		Characters: 100,
		Words:      20,
		Lines:      5,
		LastPath:   "a.go",
	})
	// Finish before paint delay elapses.
	time.Sleep(10 * time.Millisecond)
	p.Stop()
	if buf.Len() != 0 {
		t.Fatalf("expected no paint for sub-delay run, got %q", buf.String())
	}
}

func TestProgressPaintsAfterDelay(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	p := NewProgress(ProgressOptions{
		Out:        &buf,
		Root:       "./src",
		FilesTotal: 3,
		Model:      "gpt-5",
		PaintDelay: 20 * time.Millisecond,
		NoColor:    true,
	})
	p.Arm()
	p.OnProgress(tokenizer.ProgressUpdate{
		FilesTotal:   3,
		FilesDone:    1,
		Characters:   50,
		Words:        10,
		Lines:        2,
		LastPath:     "cmd/main.go",
		MethodTokens: []int{12},
	})
	time.Sleep(80 * time.Millisecond)
	p.OnProgress(tokenizer.ProgressUpdate{
		FilesTotal:   3,
		FilesDone:    3,
		Characters:   150,
		Words:        30,
		Lines:        6,
		LastPath:     "tokenizer/counter.go",
		MethodTokens: []int{40},
	})
	time.Sleep(40 * time.Millisecond)
	p.Stop()

	out := buf.String()
	if !strings.Contains(out, "counting") {
		t.Fatalf("expected counting frame, got %q", out)
	}
	if !strings.Contains(out, "tokens (gpt-5)") {
		t.Fatalf("expected labeled tokens, got %q", out)
	}
	if !strings.Contains(out, "chars") {
		t.Fatalf("expected chars, got %q", out)
	}
}

func TestProgressMultiMethodOmitsTokens(t *testing.T) {
	t.Parallel()

	var buf bytes.Buffer
	p := NewProgress(ProgressOptions{
		Out:        &buf,
		Root:       "./src",
		FilesTotal: 2,
		PaintDelay: 10 * time.Millisecond,
		NoColor:    true,
	})
	p.Arm()
	p.OnProgress(tokenizer.ProgressUpdate{
		FilesTotal: 2,
		FilesDone:  1,
		Characters: 10,
		Words:      2,
		Lines:      1,
		LastPath:   "a.txt",
	})
	time.Sleep(50 * time.Millisecond)
	p.Stop()

	out := buf.String()
	if strings.Contains(out, "tokens") {
		t.Fatalf("multi-method frame should omit tokens, got %q", out)
	}
	if !strings.Contains(out, "chars") {
		t.Fatalf("expected chars line, got %q", out)
	}
}

func TestFormatProgressInt(t *testing.T) {
	t.Parallel()
	if got := formatProgressInt(1234567); got != "1,234,567" {
		t.Fatalf("got %q", got)
	}
}
