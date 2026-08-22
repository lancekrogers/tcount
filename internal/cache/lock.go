package cache

import (
	"context"
	"errors"
	"fmt"
	"time"
)

const defaultLockPollInterval = 10 * time.Millisecond

type lockRetry int

const (
	lockFail lockRetry = iota
	lockRetryImmediate
	lockRetryWait
)

func waitForLock(ctx context.Context, try func() error, retry func(error) lockRetry) error {
	for {
		if err := ctx.Err(); err != nil {
			return err
		}
		err := try()
		if err == nil {
			return nil
		}
		switch retry(err) {
		case lockRetryImmediate:
			continue
		case lockRetryWait:
			timer := time.NewTimer(defaultLockPollInterval)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return ctx.Err()
			case <-timer.C:
			}
		default:
			return err
		}
	}
}

func wrapLockAcquireError(err error) error {
	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return err
	}
	return fmt.Errorf("acquiring cache writer lock: %w", err)
}
