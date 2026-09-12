// Package fileops provides file system operations for token counting,
// including directory traversal with .gitignore support and binary detection.
package fileops

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	gitignore "github.com/sabhiram/go-gitignore"
)

// Directory entries that traversal treats specially.
const (
	gitDirName        = ".git"
	gitignoreFileName = ".gitignore"
)

// WalkResult contains information about walked files.
type WalkResult struct {
	Files      []string
	TotalFiles int
	// SkippedBinary counts files excluded by binary detection.
	SkippedBinary int
	// SkippedIgnore counts entries excluded by a .gitignore rule. An ignored
	// directory counts once and its contents are never visited, so they do not
	// appear in TotalFiles either.
	SkippedIgnore int
	// SkippedLarge counts files excluded by WalkOptions.MaxFileSize.
	SkippedLarge int
}

// WalkOptions tunes directory traversal.
type WalkOptions struct {
	// MaxFileSize skips any file larger than this many bytes. Zero or negative
	// means no limit. A symlink is measured by the file it points at, so a
	// link to an oversized file is skipped too.
	MaxFileSize int64
}

// ignoreScope pairs a compiled .gitignore with the directory that owns it.
// Git anchors a pattern at its own .gitignore, so a scope matches paths
// expressed relative to that directory rather than to the walk root.
type ignoreScope struct {
	dir     string
	matcher *gitignore.GitIgnore
}

// WalkDirectory recursively walks a directory, respecting .gitignore files
// and filtering out binary files. It is WalkDirectoryWithOptions with no
// size cap.
func WalkDirectory(ctx context.Context, rootPath string, collectors ...WalkStatsCollector) (*WalkResult, error) {
	return WalkDirectoryWithOptions(ctx, rootPath, WalkOptions{}, collectors...)
}

// WalkDirectoryWithOptions recursively walks a directory, honoring every
// .gitignore found along the way, skipping files above opts.MaxFileSize, and
// filtering out binary files. A directory excluded by an ignore rule is not
// descended into, so an ignored tree of model weights costs one stat rather
// than a full traversal.
func WalkDirectoryWithOptions(ctx context.Context, rootPath string, opts WalkOptions, collectors ...WalkStatsCollector) (*WalkResult, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	var collector WalkStatsCollector
	if len(collectors) > 0 {
		collector = collectors[0]
	}
	var walkStarted time.Time
	if collector != nil {
		walkStarted = time.Now()
		defer func() {
			collector.RecordWalkDuration(time.Since(walkStarted))
		}()
	}

	// filepath.Walk hands the root to the callback verbatim but builds every
	// child with filepath.Join, which cleans. Walking "." therefore yields "."
	// and then bare names like "app.log", and a scope anchored at "." would
	// look like it no longer contains its own children. Resolving the root
	// once keeps every callback path prefix-consistent with the scope
	// directories derived from it.
	resolvedRoot, err := filepath.Abs(rootPath)
	if err != nil {
		return nil, fmt.Errorf("resolving walk root %s: %w", rootPath, err)
	}

	result := &WalkResult{
		Files: []string{},
	}
	var scopes []ignoreScope

	err = filepath.Walk(resolvedRoot, func(path string, info os.FileInfo, err error) error {
		if collector != nil && info != nil {
			collector.RecordEntryVisited()
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		if err != nil {
			return err
		}

		scopes = popScopes(scopes, path)

		if info.IsDir() {
			if info.Name() == gitDirName {
				return filepath.SkipDir
			}
			if matchesAnyScope(scopes, path, true) {
				result.SkippedIgnore++
				return filepath.SkipDir
			}
			scope, scopeErr := loadIgnoreScope(path)
			if scopeErr != nil {
				return scopeErr
			}
			if scope != nil {
				scopes = append(scopes, *scope)
			}
			return nil
		}

		result.TotalFiles++

		if matchesAnyScope(scopes, path, false) {
			result.SkippedIgnore++
			return nil
		}

		if opts.MaxFileSize > 0 {
			if size, known := entrySize(path, info); known && size > opts.MaxFileSize {
				result.SkippedLarge++
				return nil
			}
		}

		var isBinary bool
		if collector != nil {
			isBinary, err = IsBinaryFile(path, collector)
		} else {
			isBinary, err = IsBinaryFile(path)
		}
		if err != nil {
			result.SkippedBinary++
			return nil
		}
		if isBinary {
			result.SkippedBinary++
			return nil
		}

		callerPath, restoreErr := restoreRootForm(rootPath, resolvedRoot, path)
		if restoreErr != nil {
			return restoreErr
		}
		result.Files = append(result.Files, callerPath)
		if collector != nil {
			collector.RecordEligibleFile()
		}
		return nil
	})

	if err != nil {
		return nil, fmt.Errorf("walking directory %s: %w", rootPath, err)
	}

	return result, nil
}

// loadIgnoreScope compiles the .gitignore in dir, if there is one. A missing
// or unreadable ignore file leaves the directory without a scope, matching the
// permissive behavior traversal has always had for the walk root.
func loadIgnoreScope(dir string) (*ignoreScope, error) {
	ignorePath := filepath.Join(dir, gitignoreFileName)
	if _, err := os.Stat(ignorePath); err != nil {
		return nil, nil
	}

	matcher, err := gitignore.CompileIgnoreFile(ignorePath)
	if err != nil {
		return nil, fmt.Errorf("parsing .gitignore %s: %w", ignorePath, err)
	}
	return &ignoreScope{dir: dir, matcher: matcher}, nil
}

// popScopes drops the scopes whose directory no longer contains path.
// filepath.Walk is depth-first, so leaving a subtree means every scope pushed
// inside it is now out of range.
func popScopes(scopes []ignoreScope, path string) []ignoreScope {
	for len(scopes) > 0 && !withinDir(scopes[len(scopes)-1].dir, path) {
		scopes = scopes[:len(scopes)-1]
	}
	return scopes
}

// restoreRootForm maps a path under the resolved root back to the form the
// caller wrote their root in, so WalkResult.Files keeps the shape every
// existing consumer already sees: a relative root yields relative results and
// an absolute root yields absolute ones.
func restoreRootForm(rootPath, resolvedRoot, path string) (string, error) {
	if path == resolvedRoot {
		return rootPath, nil
	}

	relative, err := filepath.Rel(resolvedRoot, path)
	if err != nil {
		return "", fmt.Errorf("relating %s to walk root %s: %w", path, resolvedRoot, err)
	}
	return filepath.Join(rootPath, relative), nil
}

// matchesAnyScope reports whether any active .gitignore excludes path. Each
// scope sees the path relative to its own directory.
//
// Any scope that matches wins, so negation is honored only within the single
// .gitignore that declares it: a nested "!pattern" cannot re-include a file
// that an ancestor .gitignore excludes. Git behaves the same way once the
// ancestor rule names a directory, and the divergence for an ancestor rule
// naming files is deliberate, since re-including across files would mean
// evaluating every scope in order rather than stopping at the first match.
func matchesAnyScope(scopes []ignoreScope, path string, isDir bool) bool {
	for _, scope := range scopes {
		relPath, err := filepath.Rel(scope.dir, path)
		if err != nil {
			continue
		}
		if scope.matcher.MatchesPath(relPath) {
			return true
		}
		// A pattern written with a trailing slash only matches a directory
		// when the candidate carries the separator too.
		if isDir && scope.matcher.MatchesPath(relPath+"/") {
			return true
		}
	}
	return false
}

// withinDir reports whether path is dir itself or lies beneath it. Both paths
// come from the same filepath.Walk root, so they share cleaning and separators.
func withinDir(dir, path string) bool {
	if path == dir {
		return true
	}
	separator := string(filepath.Separator)
	if !strings.HasSuffix(dir, separator) {
		dir += separator
	}
	return strings.HasPrefix(path, dir)
}

// entrySize reports the size a per-file cap should measure. It is consulted
// only when a cap is active, so no symlink is resolved on the default path.
// filepath.Walk
// hands back an Lstat result, so a symlink reports the length of its target
// path rather than the size of the file it points at. A broken link reports no
// size and falls through to binary detection, which skips it as before.
func entrySize(path string, info os.FileInfo) (int64, bool) {
	if info.Mode()&os.ModeSymlink == 0 {
		return info.Size(), true
	}

	target, err := os.Stat(path)
	if err != nil {
		return 0, false
	}
	return target.Size(), true
}

// AggregateFileContents reads all files and returns combined content.
// Pre-allocates the result buffer based on file sizes to minimize allocations.
func AggregateFileContents(ctx context.Context, files []string) ([]byte, error) {
	if err := ctx.Err(); err != nil {
		return nil, err
	}

	totalSize := int64(0)
	for _, file := range files {
		if info, err := os.Stat(file); err == nil {
			totalSize += info.Size()
		}
	}

	totalContent := make([]byte, 0, totalSize)

	for _, file := range files {
		if ctxErr := ctx.Err(); ctxErr != nil {
			return nil, ctxErr
		}

		content, err := os.ReadFile(file)
		if err != nil {
			return nil, fmt.Errorf("reading file %s: %w", file, err)
		}
		totalContent = append(totalContent, content...)
	}

	return totalContent, nil
}
