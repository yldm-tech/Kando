package main

import (
	"context"
	"time"
)

// aiAsk runs ask off the engine's goroutine with a context that ends after timeout, and hands what it returned to done back on the engine's goroutine (app.update), unless current, asked there first, reports that what asked for it was left meanwhile, as a card closed or a newer request made: the answer is then dropped. The cancel it returns stops ask early; ask's context is released once it returns either way. Explain Line, the word looked up, the recap, the palette's question, the other translations of a line and the plugins' tasks ask the AI service this way, each keeping its own generation and cancel.
func aiAsk[T any](a *app, timeout time.Duration, ask func(ctx context.Context) (T, error), current func() bool, done func(T, error)) context.CancelFunc {
	ctx, cancel := context.WithTimeout(context.Background(), timeout)
	update := a.update
	go func() {
		defer cancel()
		v, err := ask(ctx)
		update(func() {
			if current() {
				done(v, err)
			}
		})
	}()
	return cancel
}
