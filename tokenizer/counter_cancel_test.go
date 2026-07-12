package tokenizer

import (
	"context"
	"errors"
	"testing"
)

func TestCountCanceledContext(t *testing.T) {
	c, err := NewCounter(CounterOptions{})
	if err != nil {
		t.Fatal(err)
	}

	ctx, cancel := context.WithCancel(context.Background())
	cancel()

	if _, err := c.Count(ctx, "hello world", "", false); !errors.Is(err, context.Canceled) {
		t.Fatalf("Count() error = %v, want context.Canceled", err)
	}
	if _, err := c.Count(ctx, "hello world", "gpt-5", false); !errors.Is(err, context.Canceled) {
		t.Fatalf("Count() with model error = %v, want context.Canceled", err)
	}
}
