//go:build unix

package integration_test

import (
	"bytes"
	"fmt"
	"io"
	"os"
	"os/exec"
	"path/filepath"
	"regexp"
	"strings"
	"testing"
	"time"

	"github.com/creack/pty"
)

var cursorUpRE = regexp.MustCompile(`\x1b\[[0-9]*A`)

func TestIntegrationCLI_DirectoryProgressDoesNotClipTable(t *testing.T) {
	dir := t.TempDir()
	for i := range 80 {
		var b strings.Builder
		for range 200 {
			fmt.Fprintf(&b, "hello world this is file %d\n", i)
		}
		path := filepath.Join(dir, fmt.Sprintf("f%03d.txt", i))
		if err := os.WriteFile(path, []byte(b.String()), 0o644); err != nil {
			t.Fatalf("write fixture: %v", err)
		}
	}

	cmd := exec.Command(binaryPath, "--no-color", "-d", dir)
	cmd.Dir = dir
	cmd.Env = append(os.Environ(), "TERM=xterm-256color", "COLUMNS=120", "LINES=40")

	ptmx, err := pty.StartWithSize(cmd, &pty.Winsize{Rows: 40, Cols: 120})
	if err != nil {
		t.Fatalf("pty start: %v", err)
	}
	t.Cleanup(func() { _ = ptmx.Close() })

	var buf bytes.Buffer
	done := make(chan error, 1)
	go func() {
		_, copyErr := io.Copy(&buf, ptmx)
		waitErr := cmd.Wait()
		if waitErr != nil {
			done <- waitErr
			return
		}
		done <- copyErr
	}()

	select {
	case err := <-done:
		if err != nil {
			t.Fatalf("tcount pty run: %v\noutput:\n%s", err, buf.String())
		}
	case <-time.After(45 * time.Second):
		_ = cmd.Process.Kill()
		t.Fatalf("tcount timed out\noutput:\n%s", buf.String())
	}

	out := buf.String()
	if !strings.Contains(out, "Whitespace split") {
		t.Fatalf("report missing last method row:\n%s", out)
	}
	bottom := strings.LastIndex(out, "╰")
	if bottom < 0 {
		t.Fatalf("table missing bottom border:\n%s", out)
	}
	if cursorUpRE.MatchString(out[bottom:]) {
		t.Fatalf("progress cursor-up ran after the table bottom border (TTY clip bug):\n%s", out)
	}
}
