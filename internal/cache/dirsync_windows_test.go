//go:build windows

package cache

import (
	"errors"
	"os"
	"path/filepath"
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsUnsupportedDirectorySyncErrorsAreBenign(t *testing.T) {
	if !isUnsupportedDirectorySync(windows.ERROR_INVALID_FUNCTION) {
		t.Fatal("ERROR_INVALID_FUNCTION should be treated as unsupported directory sync")
	}
	if !isUnsupportedDirectorySync(windows.ERROR_NOT_SUPPORTED) {
		t.Fatal("ERROR_NOT_SUPPORTED should be treated as unsupported directory sync")
	}
	if isUnsupportedDirectorySync(windows.ERROR_ACCESS_DENIED) {
		t.Fatal("ACCESS_DENIED must not be treated as unsupported directory sync")
	}
}

func TestWindowsSyncDirectoryAfterRename(t *testing.T) {
	directory := t.TempDir()
	temporary := filepath.Join(directory, "manifest.tmp")
	published := filepath.Join(directory, "manifest")
	if err := os.WriteFile(temporary, []byte("tcount-cache"), 0o600); err != nil {
		t.Fatal(err)
	}
	if err := os.Rename(temporary, published); err != nil {
		t.Fatal(err)
	}
	if err := syncDirectory(directory); err != nil {
		t.Fatalf("syncDirectory() after rename = %v", err)
	}
}

func TestWindowsSyncDirectoryReportsMissingDirectory(t *testing.T) {
	missing := filepath.Join(t.TempDir(), "missing")
	err := syncDirectory(missing)
	if err == nil {
		t.Fatal("syncDirectory() of a missing directory succeeded")
	}
	if isUnsupportedDirectorySync(err) {
		t.Fatalf("missing directory was treated as unsupported sync: %v", err)
	}
}

func TestWindowsReadOnlyDirectoryHandleCannotSatisfyFlush(t *testing.T) {
	directory := t.TempDir()
	readonly, err := os.Open(directory)
	if err != nil {
		t.Fatal(err)
	}
	readonlyErr := readonly.Sync()
	_ = readonly.Close()
	if err := syncDirectory(directory); err != nil {
		t.Fatalf("write-capable syncDirectory() = %v", err)
	}
	if readonlyErr == nil {
		return
	}
	if !errors.Is(readonlyErr, windows.ERROR_ACCESS_DENIED) && !isUnsupportedDirectorySync(readonlyErr) {
		t.Fatalf("unexpected os.Open+Sync error: %v", readonlyErr)
	}
}
