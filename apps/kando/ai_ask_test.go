package main

import (
	"context"
	"errors"
	"testing"
	"time"
)

// TestAIAskHandsTheAnswerBackUnlessItWasLeft checks the shared way the AI service is asked off the engine's goroutine: the answer comes back through update and reaches done while current holds, is dropped once it does not, and the cancel returned stops the question, whose context ends after the timeout too.
func TestAIAskHandsTheAnswerBackUnlessItWasLeft(t *testing.T) {
	updates := make(chan func(), 1)
	a := newApp(&fakePipeline{}, func(fn func()) { updates <- fn })
	apply := func() {
		t.Helper()
		select {
		case fn := <-updates:
			fn()
		case <-time.After(5 * time.Second):
			t.Fatal("no answer came back")
		}
	}

	var got string
	aiAsk(a, time.Minute, func(context.Context) (string, error) { return "yes", nil }, func() bool { return true }, func(v string, err error) { got = v })
	apply()
	if got != "yes" {
		t.Fatalf("done got %q", got)
	}

	called := false
	aiAsk(a, time.Minute, func(context.Context) (string, error) { return "late", nil }, func() bool { return false }, func(string, error) { called = true })
	apply()
	if called {
		t.Fatal("an answer to a question left was handed on")
	}

	var stopped error
	cancel := aiAsk(a, time.Minute, func(ctx context.Context) (int, error) { <-ctx.Done(); return 0, ctx.Err() }, func() bool { return true }, func(_ int, err error) { stopped = err })
	cancel()
	apply()
	if !errors.Is(stopped, context.Canceled) {
		t.Fatalf("cancel left the question running: %v", stopped)
	}

	aiAsk(a, time.Millisecond, func(ctx context.Context) (int, error) { <-ctx.Done(); return 0, ctx.Err() }, func() bool { return true }, func(_ int, err error) { stopped = err })
	apply()
	if !errors.Is(stopped, context.DeadlineExceeded) {
		t.Fatalf("the timeout did not end the question: %v", stopped)
	}
}
