package commands

import (
	"context"
	"errors"
	"io"
	"sync"
	"testing"
	"time"
)

type blockingReadCloser struct {
	closed  chan struct{}
	started chan struct{}
	once    sync.Once
	start   sync.Once
}

func newBlockingReadCloser() *blockingReadCloser {
	return &blockingReadCloser{
		closed:  make(chan struct{}),
		started: make(chan struct{}),
	}
}

func (r *blockingReadCloser) Read([]byte) (int, error) {
	r.start.Do(func() { close(r.started) })
	<-r.closed
	return 0, io.ErrClosedPipe
}

func (r *blockingReadCloser) Close() error {
	r.once.Do(func() { close(r.closed) })
	return nil
}

func TestReadStdinCancellationUnblocksRead(t *testing.T) {
	original := stdinReader
	reader := newBlockingReadCloser()
	stdinReader = reader
	t.Cleanup(func() { stdinReader = original })

	ctx, cancel := context.WithCancel(context.Background())
	done := make(chan error, 1)
	go func() {
		_, err := readStdin(ctx)
		done <- err
	}()

	select {
	case <-reader.started:
	case <-time.After(time.Second):
		t.Fatal("readStdin never started reading")
	}
	cancel()
	select {
	case err := <-done:
		if !errors.Is(err, context.Canceled) {
			t.Fatalf("readStdin() error = %v, want context.Canceled", err)
		}
	case <-time.After(time.Second):
		t.Fatal("readStdin did not unblock after context cancellation")
	}

	select {
	case <-reader.closed:
	default:
		t.Fatal("readStdin did not close the blocked source")
	}
}
