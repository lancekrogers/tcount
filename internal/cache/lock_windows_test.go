//go:build windows

package cache

import (
	"testing"

	"golang.org/x/sys/windows"
)

func TestWindowsWriterLockRetryClassification(t *testing.T) {
	if writerLockRetry(windows.ERROR_LOCK_VIOLATION) != lockRetryWait {
		t.Fatal("ERROR_LOCK_VIOLATION should wait and retry")
	}
	if writerLockRetry(windows.ERROR_ACCESS_DENIED) != lockFail {
		t.Fatal("ERROR_ACCESS_DENIED should fail the lock attempt")
	}
}
