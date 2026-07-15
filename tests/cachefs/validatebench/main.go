package main

import (
	"context"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"
	"time"

	"github.com/lancekrogers/tcount/internal/cache"
)

func main() {
	root := flag.String("root", "", "directory containing the generated fixture")
	modeName := flag.String("mode", "", "metadata or verified")
	samples := flag.Int("samples", 5, "number of validation samples")
	flag.Parse()
	if *root == "" || *samples < 1 {
		fail("root and positive samples are required")
	}

	mode, err := parseMode(*modeName)
	if err != nil {
		fail(err.Error())
	}
	validator, err := cache.NewValidator(mode)
	if err != nil {
		fail(err.Error())
	}
	paths, err := fixtureFiles(*root)
	if err != nil {
		fail(err.Error())
	}
	entries := make([]cache.Entry, len(paths))
	for i, path := range paths {
		entries[i], err = cache.CaptureEntry(context.Background(), *root, path)
		if err != nil {
			fail(err.Error())
		}
	}

	elapsed := make([]float64, 0, *samples)
	for sample := 1; sample <= *samples; sample++ {
		stats := &cache.ValidationStats{}
		started := time.Now()
		for i, path := range paths {
			if _, err := validator.Validate(context.Background(), *root, path, entries[i], stats); err != nil {
				fail(err.Error())
			}
		}
		seconds := time.Since(started).Seconds()
		elapsed = append(elapsed, seconds)
		snapshot := stats.Snapshot()
		fmt.Printf("validation mode=%s sample=%d elapsed_seconds=%.3f files=%d hits=%d misses=%d full_reads=%d bytes_read=%d digest_seconds=%.6f tokenizer_calls_avoided=%d\n", mode, sample, seconds, snapshot.FilesChecked, snapshot.Hits, snapshot.Misses, snapshot.FullReads, snapshot.BytesRead, snapshot.DigestDuration.Seconds(), snapshot.Hits)
	}

	sort.Float64s(elapsed)
	median := elapsed[(*samples-1)/2]
	p95 := elapsed[((*samples*95+99)/100)-1]
	fmt.Printf("validation summary mode=%s median_elapsed_seconds=%.3f p95_elapsed_seconds=%.3f\n", mode, median, p95)
}

func parseMode(name string) (cache.ValidationMode, error) {
	switch strings.ToLower(name) {
	case "metadata":
		return cache.Metadata, nil
	case "verified":
		return cache.Verified, nil
	default:
		return 0, fmt.Errorf("unknown validation mode %q", name)
	}
}

func fixtureFiles(root string) ([]string, error) {
	paths := make([]string, 0)
	err := filepath.Walk(root, func(path string, info os.FileInfo, err error) error {
		if err != nil {
			return err
		}
		if info.IsDir() {
			if info.Name() == ".git" {
				return filepath.SkipDir
			}
			return nil
		}
		if !info.Mode().IsRegular() {
			return nil
		}
		paths = append(paths, path)
		return nil
	})
	if err != nil {
		return nil, fmt.Errorf("walking fixture: %w", err)
	}
	sort.Strings(paths)
	if len(paths) == 0 {
		return nil, fmt.Errorf("no regular files found under %q", root)
	}
	return paths, nil
}

func fail(message string) {
	fmt.Fprintln(os.Stderr, message)
	os.Exit(1)
}
