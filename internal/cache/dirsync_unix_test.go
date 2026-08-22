//go:build unix

package cache

import (
	"syscall"
	"testing"
)

func TestUnsupportedDirectorySyncErrorsAreBenign(t *testing.T) {
	for _, err := range []error{syscall.EINVAL, syscall.ENOTSUP, syscall.ENOSYS, syscall.EOPNOTSUPP} {
		if !isUnsupportedDirectorySync(err) {
			t.Fatalf("isUnsupportedDirectorySync(%v) = false, want true", err)
		}
	}
	if isUnsupportedDirectorySync(syscall.EIO) {
		t.Fatal("I/O errors must not be treated as unsupported directory sync")
	}
}
