package commands

import (
	"context"
	"io"
	"os"

	"github.com/lancekrogers/tcount/internal/errors"
	"github.com/lancekrogers/tcount/internal/ui"
	"github.com/lancekrogers/tcount/tokenizer/fileops"
)

// stdinMarker is the conventional path that means "read standard input".
// Used when the user passes "-" or omits the path argument entirely.
const stdinMarker = "-"

// stdinReader is the source for filter-mode input. It must be closable so
// context cancellation can interrupt a blocked pipe or FIFO read. Tests may
// replace it with another io.ReadCloser.
var stdinReader io.ReadCloser = os.Stdin

// isStdinSource reports whether path means standard input.
func isStdinSource(path string) bool {
	return path == stdinMarker
}

// validateStdinFlags rejects directory/cache flags that cannot apply to a stream.
func validateStdinFlags(path string, opts *countOptions) error {
	if !isStdinSource(path) {
		return nil
	}
	if opts.recursive {
		return errors.Validation("--recursive requires a directory path; stdin is not a directory")
	}
	if opts.cache || opts.cacheVerify {
		return errors.Validation("--cache is only supported for recursive directory counts")
	}
	return nil
}

// resolveInput loads content from stdin, a single file, or walks a directory.
func resolveInput(ctx context.Context, path string, opts *countOptions, _ *ui.UI) (content []byte, walkFiles []string, isDirectory bool, err error) {
	if isStdinSource(path) {
		content, err = readStdin(ctx)
		if err != nil {
			return nil, nil, false, err
		}
		return content, nil, false, nil
	}

	info, err := os.Stat(path)
	if err != nil {
		return nil, nil, false, errors.IO("accessing path", err).WithField("path", path)
	}

	if !info.IsDir() {
		content, err = os.ReadFile(path)
		if err != nil {
			return nil, nil, false, errors.IO("reading file", err).WithField("path", path)
		}
		return content, nil, false, nil
	}

	if !opts.recursive {
		return nil, nil, true, errors.Validation("path is a directory — use --recursive flag to count tokens in all files").WithField("path", path)
	}

	var walkResult *fileops.WalkResult
	if opts.stats != nil {
		walkResult, err = fileops.WalkDirectory(ctx, path, opts.stats)
	} else {
		walkResult, err = fileops.WalkDirectory(ctx, path)
	}
	if err != nil {
		return nil, nil, true, errors.IO("walking directory", err).WithField("path", path)
	}

	if len(walkResult.Files) == 0 {
		return nil, nil, true, errors.NotFound("text files in directory").WithField("path", path)
	}

	return nil, walkResult.Files, true, nil
}

// readStdin consumes standard input (or the test-injected stdinReader) until
// EOF. Cancellation closes the stream so a blocked Read wakes up promptly.
func readStdin(ctx context.Context) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}
	reader := stdinReader
	stopClose := context.AfterFunc(ctx, func() {
		_ = reader.Close()
	})
	defer stopClose()

	content, err := io.ReadAll(reader)
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, ctxErr
	}
	if err != nil {
		return nil, errors.IO("reading stdin", err)
	}
	return content, nil
}
