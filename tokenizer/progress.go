package tokenizer

import "sync"

// ProgressUpdate is a pure fact snapshot after one or more files complete.
// The CLI owns elapsed time, spinner, labels, and paint rate.
type ProgressUpdate struct {
	FilesTotal   int
	FilesDone    int
	LastPath     string
	Bytes        int64
	Characters   int
	Words        int
	Lines        int
	MethodTokens []int // running sums aligned with planned methods for this run
}

// ProgressFunc receives per-file progress facts. Nil means no reporting.
type ProgressFunc func(ProgressUpdate)

// CountFilesOptions configures a multi-file count, including optional progress.
type CountFilesOptions struct {
	Model      string
	All        bool
	OnProgress ProgressFunc
}

// progressTracker accumulates completed file facts and invokes OnProgress.
// Nil receiver and nil OnProgress are no-ops (near-zero cost when unused).
type progressTracker struct {
	on    ProgressFunc
	total int
	plans int

	mu           sync.Mutex
	done         int
	bytes        int64
	chars        int
	words        int
	lines        int
	methodTokens []int
	lastPath     string
}

func newProgressTracker(on ProgressFunc, total, planCount int) *progressTracker {
	if on == nil {
		return nil
	}
	t := &progressTracker{on: on, total: total, plans: planCount}
	if planCount > 0 {
		t.methodTokens = make([]int, planCount)
	}
	return t
}

func (t *progressTracker) add(path string, result perFileResult) {
	if t == nil || t.on == nil {
		return
	}
	t.mu.Lock()
	t.done++
	t.bytes += int64(result.FileSize)
	t.chars += result.Characters
	t.words += result.Words
	t.lines += result.Lines
	t.lastPath = path
	if len(t.methodTokens) == len(result.MethodPresent) {
		for i, present := range result.MethodPresent {
			if present {
				t.methodTokens[i] += result.Methods[i].Tokens
			}
		}
	}
	update := ProgressUpdate{
		FilesTotal:   t.total,
		FilesDone:    t.done,
		LastPath:     t.lastPath,
		Bytes:        t.bytes,
		Characters:   t.chars,
		Words:        t.words,
		Lines:        t.lines,
		MethodTokens: append([]int(nil), t.methodTokens...),
	}
	on := t.on
	t.mu.Unlock()
	on(update)
}
