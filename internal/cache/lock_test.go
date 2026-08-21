package cache

import (
	"context"
	"errors"
	"os"
	"os/exec"
	"runtime"
	"strings"
	"testing"
	"time"
)

var errLockBusy = errors.New("lock busy")
var errLockPermanent = errors.New("lock permanent")

func TestWaitForLockReturnsCanceledContext(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	called := false
	err := waitForLock(ctx, func() error {
		called = true
		return nil
	}, func(error) lockRetry { return lockFail })
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("waitForLock() error = %v, want context.Canceled", err)
	}
	if called {
		t.Fatal("try ran despite canceled context")
	}
}

func TestWaitForLockRetriesThenSucceeds(t *testing.T) {
	attempts := 0
	err := waitForLock(context.Background(), func() error {
		attempts++
		if attempts < 3 {
			return errLockBusy
		}
		return nil
	}, func(err error) lockRetry {
		if errors.Is(err, errLockBusy) {
			return lockRetryWait
		}
		return lockFail
	})
	if err != nil {
		t.Fatalf("waitForLock() error = %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
}

func TestWaitForLockImmediateRetryDoesNotTreatBusyAsFatal(t *testing.T) {
	attempts := 0
	err := waitForLock(context.Background(), func() error {
		attempts++
		if attempts == 1 {
			return errLockBusy
		}
		return nil
	}, func(err error) lockRetry {
		if errors.Is(err, errLockBusy) {
			return lockRetryImmediate
		}
		return lockFail
	})
	if err != nil {
		t.Fatalf("waitForLock() error = %v", err)
	}
	if attempts != 2 {
		t.Fatalf("attempts = %d, want 2", attempts)
	}
}

func TestWaitForLockReturnsPermanentError(t *testing.T) {
	err := waitForLock(context.Background(), func() error {
		return errLockPermanent
	}, func(error) lockRetry { return lockFail })
	if !errors.Is(err, errLockPermanent) {
		t.Fatalf("waitForLock() error = %v, want %v", err, errLockPermanent)
	}
}

func TestWaitForLockHonorsDeadlineWhileWaiting(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 15*time.Millisecond)
	defer cancel()
	err := waitForLock(ctx, func() error {
		return errLockBusy
	}, func(error) lockRetry { return lockRetryWait })
	if !errors.Is(err, context.DeadlineExceeded) {
		t.Fatalf("waitForLock() error = %v, want context.DeadlineExceeded", err)
	}
}

func TestWrapLockAcquireErrorPreservesContext(t *testing.T) {
	if err := wrapLockAcquireError(context.Canceled); !errors.Is(err, context.Canceled) {
		t.Fatalf("canceled wrap = %v", err)
	}
	if err := wrapLockAcquireError(errLockPermanent); !errors.Is(err, errLockPermanent) {
		t.Fatalf("permanent wrap = %v", err)
	}
}

func TestWindowsCachePackageCrossCompiles(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("native Windows build already covers the lock implementation")
	}
	cmd := exec.Command("go", "build", "github.com/lancekrogers/tcount/internal/cache")
	cmd.Env = append(osEnvironWithoutGoOSArch(), "GOOS=windows", "GOARCH=amd64", "CGO_ENABLED=0")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("GOOS=windows go build ./internal/cache: %v\n%s", err, out)
	}
}

func osEnvironWithoutGoOSArch() []string {
	env := os.Environ()
	filtered := make([]string, 0, len(env))
	for _, item := range env {
		if strings.HasPrefix(item, "GOOS=") || strings.HasPrefix(item, "GOARCH=") {
			continue
		}
		filtered = append(filtered, item)
	}
	return filtered
}
