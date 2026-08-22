//go:build unix

package cache

import (
	"testing"

	"golang.org/x/sys/unix"
)

func TestUnixWriterLockRetryClassification(t *testing.T) {
	if writerLockRetry(unix.EINTR) != lockRetryImmediate {
		t.Fatal("EINTR should retry immediately")
	}
	if writerLockRetry(unix.EWOULDBLOCK) != lockRetryWait {
		t.Fatal("EWOULDBLOCK should wait and retry")
	}
	if writerLockRetry(unix.EAGAIN) != lockRetryWait {
		t.Fatal("EAGAIN should wait and retry")
	}
	if writerLockRetry(unix.EIO) != lockFail {
		t.Fatal("EIO should fail the lock attempt")
	}
}
