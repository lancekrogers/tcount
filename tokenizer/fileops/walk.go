// Package fileops provides file system operations for token counting,
// including directory traversal with .gitignore support and binary detection.
package fileops

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	gitignore "github.com/sabhiram/go-gitignore"
)

// WalkResult contains information about walked files.
type WalkResult struct {
	Files         []string
	TotalFiles    int
	SkippedBinary int
	SkippedIgnore int
}

// WalkDirectory recursively walks a directory, respecting .gitignore files
// and filtering out binary files.
func WalkDirectory(ctx context.Context, rootPath string, collectors ...WalkStatsCollector) (*WalkResult, error) {
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

	result := &WalkResult{
		Files: []string{},
	}

	gitignoreFile := filepath.Join(rootPath, ".gitignore")
	var gi *gitignore.GitIgnore
	if _, err := os.Stat(gitignoreFile); err == nil {
		gi, err = gitignore.CompileIgnoreFile(gitignoreFile)
		if err != nil {
			return nil, fmt.Errorf("parsing .gitignore %s: %w", gitignoreFile, err)
		}
	}

	err := filepath.Walk(rootPath, func(path string, info os.FileInfo, err error) error {
		if collector != nil && info != nil {
			collector.RecordEntryVisited()
		}
		if ctxErr := ctx.Err(); ctxErr != nil {
			return ctxErr
		}

		if err != nil {
			return err
		}

		if info.IsDir() {
			if info.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}

		result.TotalFiles++

		relPath, err := filepath.Rel(rootPath, path)
		if err != nil {
			return err
		}

		if gi != nil && gi.MatchesPath(relPath) {
			result.SkippedIgnore++
			return nil
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

		result.Files = append(result.Files, path)
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
