//go:build windows

package cache

import (
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
