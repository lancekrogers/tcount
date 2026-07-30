package ui

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"github.com/charmbracelet/lipgloss"
	"github.com/mattn/go-isatty"

	"github.com/lancekrogers/tcount/tokenizer"
)

// Default paint delay before the first progress frame (Freeze 4).
const ProgressPaintDelay = 100 * time.Millisecond

const progressPaintInterval = time.Second / 12

var spinnerFrames = []string{"⠋", "⠙", "⠹", "⠸", "⠼", "⠴", "⠦", "⠧", "⠇", "⠏"}

// ProgressOptions configures directory-count live progress on stderr.
type ProgressOptions struct {
	Out        io.Writer // defaults to os.Stderr
	NoColor    bool
	Root       string
	FilesTotal int
	Model      string // when set, show labeled live tokens (Freeze 3)
	PaintDelay time.Duration
}

// Progress paints a multi-line in-place status frame while directory counting runs.
type Progress struct {
	out        io.Writer
	noColor    bool
	root       string
	filesTotal int
	model      string
	paintDelay time.Duration

	start time.Time

	mu       sync.Mutex
	latest   tokenizer.ProgressUpdate
	hasData  bool
	painted  bool
	stopped  bool
	stopCh   chan struct{}
	doneCh   chan struct{}
	spinIdx  int
	lineCount int
}

// NewProgress builds a progress controller. Call Arm before counting; Stop when done.
func NewProgress(opts ProgressOptions) *Progress {
	out := opts.Out
	if out == nil {
		out = os.Stderr
	}
	delay := opts.PaintDelay
	if delay <= 0 {
		delay = ProgressPaintDelay
	}
	return &Progress{
		out:        out,
		noColor:    opts.NoColor,
		root:       opts.Root,
		filesTotal: opts.FilesTotal,
		model:      opts.Model,
		paintDelay: delay,
		stopCh:     make(chan struct{}),
		doneCh:     make(chan struct{}),
	}
}

// ShouldShowProgress reports whether live progress should run (Freezes 1–2).
func ShouldShowProgress(isDirectory, jsonOutput, noProgress bool, stderr *os.File) bool {
	if !isDirectory || jsonOutput || noProgress || stderr == nil {
		return false
	}
	return isatty.IsTerminal(stderr.Fd()) || isatty.IsCygwinTerminal(stderr.Fd())
}

// Arm starts the paint loop. Safe to call once.
func (p *Progress) Arm() {
	if p == nil {
		return
	}
	p.start = time.Now()
	go p.loop()
}

// OnProgress implements tokenizer.ProgressFunc.
func (p *Progress) OnProgress(u tokenizer.ProgressUpdate) {
	if p == nil {
		return
	}
	p.mu.Lock()
	p.latest = u
	p.hasData = true
	p.mu.Unlock()
}

// Stop ends the paint loop and clears the frame if it was painted.
// Clear state is read only after the paint loop has fully exited so a first
// paint that races Stop still leaves a correct lineCount for erase.
func (p *Progress) Stop() {
	if p == nil {
		return
	}
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return
	}
	p.stopped = true
	p.mu.Unlock()

	close(p.stopCh)
	<-p.doneCh

	p.mu.Lock()
	painted := p.painted
	lines := p.lineCount
	p.mu.Unlock()

	if painted {
		p.clear(lines)
	}
}

func (p *Progress) loop() {
	defer close(p.doneCh)

	// Wait for deferred paint delay (Freeze 4), then paint immediately.
	timer := time.NewTimer(p.paintDelay)
	select {
	case <-p.stopCh:
		if !timer.Stop() {
			select {
			case <-timer.C:
			default:
			}
		}
		return
	case <-timer.C:
		p.tick()
	}

	ticker := time.NewTicker(progressPaintInterval)
	defer ticker.Stop()
	for {
		select {
		case <-p.stopCh:
			return
		case <-ticker.C:
			p.tick()
		}
	}
}

func (p *Progress) tick() {
	p.mu.Lock()
	if p.stopped {
		p.mu.Unlock()
		return
	}
	elapsed := time.Since(p.start)
	if elapsed < p.paintDelay {
		p.mu.Unlock()
		return
	}
	if !p.hasData && p.filesTotal == 0 {
		p.mu.Unlock()
		return
	}
	u := p.latest
	if u.FilesTotal == 0 {
		u.FilesTotal = p.filesTotal
	}
	p.spinIdx = (p.spinIdx + 1) % len(spinnerFrames)
	spin := spinnerFrames[p.spinIdx]
	p.mu.Unlock()

	// Write first, then publish painted/lineCount so Stop never clears with a
	// stale lineCount of 0 after a completed first frame.
	lines := p.render(spin, elapsed, u)
	p.mu.Lock()
	p.painted = true
	p.lineCount = lines
	p.mu.Unlock()
}

func (p *Progress) render(spin string, elapsed time.Duration, u tokenizer.ProgressUpdate) int {
	purple := lipgloss.NewStyle().Foreground(lipgloss.Color("99"))
	dim := lipgloss.NewStyle().Foreground(lipgloss.Color("245"))
	bold := lipgloss.NewStyle().Bold(true)
	if p.noColor {
		purple = lipgloss.NewStyle()
		dim = lipgloss.NewStyle()
		bold = lipgloss.NewStyle().Bold(true)
	}

	root := p.root
	if root == "" {
		root = "."
	}
	// Keep header on one logical line on narrow TTYs (matches last-path clip).
	rootDisplay := truncateMiddle(root, 40)
	done := u.FilesDone
	total := u.FilesTotal
	if total == 0 {
		total = p.filesTotal
	}

	header := fmt.Sprintf(
		"  %s counting  %s  %s  %d/%d files  %s  %s",
		purple.Render(spin),
		bold.Render(rootDisplay),
		dim.Render("·"),
		done,
		total,
		dim.Render("·"),
		fmt.Sprintf("%.1fs", elapsed.Seconds()),
	)

	var stats string
	if p.model != "" {
		tokens := 0
		if len(u.MethodTokens) > 0 {
			tokens = u.MethodTokens[0]
		}
		stats = fmt.Sprintf(
			"  tokens (%s)  %s   chars  %s   words  %s   lines  %s",
			p.model,
			formatProgressInt(tokens),
			formatProgressInt(u.Characters),
			formatProgressInt(u.Words),
			formatProgressInt(u.Lines),
		)
	} else {
		stats = fmt.Sprintf(
			"  chars  %s   words  %s   lines  %s",
			formatProgressInt(u.Characters),
			formatProgressInt(u.Words),
			formatProgressInt(u.Lines),
		)
	}

	last := u.LastPath
	if last != "" {
		last = shortenPath(last, root, 48)
	} else {
		last = "—"
	}
	lastLine := fmt.Sprintf("  %s     %s", dim.Render("last"), last)

	frame := []string{header, stats, lastLine}
	p.writeFrame(frame)
	return len(frame)
}

func (p *Progress) writeFrame(lines []string) {
	var b strings.Builder
	p.mu.Lock()
	prev := p.lineCount
	p.mu.Unlock()
	// Subsequent frames rewrite in place; first frame has prev == 0.
	if prev > 0 {
		b.WriteString(fmt.Sprintf("\033[%dA", prev))
	}
	for _, line := range lines {
		b.WriteString("\033[2K")
		b.WriteString(line)
		b.WriteByte('\n')
	}
	_, _ = io.WriteString(p.out, b.String())
}

func (p *Progress) clear(lines int) {
	if lines <= 0 {
		return
	}
	var b strings.Builder
	b.WriteString(fmt.Sprintf("\033[%dA", lines))
	for i := 0; i < lines; i++ {
		b.WriteString("\033[2K\n")
	}
	b.WriteString(fmt.Sprintf("\033[%dA", lines))
	_, _ = io.WriteString(p.out, b.String())
}

func shortenPath(path, root string, maxLen int) string {
	if rel, err := filepath.Rel(root, path); err == nil && rel != "" && !strings.HasPrefix(rel, "..") {
		path = rel
	}
	return truncateMiddle(path, maxLen)
}

// truncateMiddle shortens a display path with an ellipsis in the middle so
// fixed-line ANSI rewrites do not leave wrap residue on narrow terminals.
func truncateMiddle(path string, maxLen int) string {
	path = filepath.ToSlash(path)
	if maxLen <= 1 || utf8.RuneCountInString(path) <= maxLen {
		return path
	}
	runes := []rune(path)
	keep := maxLen - 1
	left := keep / 2
	right := keep - left
	return string(runes[:left]) + "…" + string(runes[len(runes)-right:])
}

func formatProgressInt(n int) string {
	if n < 0 {
		return "-" + formatProgressInt(-n)
	}
	s := fmt.Sprintf("%d", n)
	if len(s) <= 3 {
		return s
	}
	var b strings.Builder
	remainder := len(s) % 3
	if remainder > 0 {
		b.WriteString(s[:remainder])
	}
	for i := remainder; i < len(s); i += 3 {
		if b.Len() > 0 {
			b.WriteByte(',')
		}
		b.WriteString(s[i : i+3])
	}
	return b.String()
}
