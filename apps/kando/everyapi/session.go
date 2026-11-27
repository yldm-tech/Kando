package everyapi

import (
	"context"
	"errors"
	"sync"
	"time"
)

// renewAhead is how long before EveryAPI says the key expires a Session renews it, so a job does not start with a key that runs out under it.
const renewAhead = 24 * time.Hour

// renewTimeout bounds a renewal, which runs apart from the request that asked for it (Session.Key).
const renewTimeout = 30 * time.Second

// Session is a sign-in in use: it hands out its key to every request, and renews it once for all the requests that were refused with it at the same time, or once it is about to expire. Each key has a generation, which a renewal moves on, so a request refused with a key of an earlier generation gets the newer key at once rather than renewing again, which would send EveryAPI a refresh token it has rotated already and make it revoke the whole sign-in.
//
// A renewal keeps the new tokens (Persist) before it hands out the new key, as the refresh token it used is gone at EveryAPI from then on: a key handed out first, and the app ending before the new refresh token is kept, would leave a sign-in that can never be renewed. When Persist fails the session still uses the new tokens, as the ones kept are spent either way, and tells the app (PersistFailed), which says the sign-in may not survive a restart.
type Session struct {
	cfg Config
	// Persist keeps the tokens a renewal got, where the app keeps the sign-in, before the new key is handed out. It runs on the goroutine of the request that renews.
	persist func(Tokens) error
	// ended is told once, when EveryAPI no longer takes the sign-in; persistFailed each time keeping a renewal failed, and nil once keeping one worked again.
	ended         func(error)
	persistFailed func(error)

	mu       sync.Mutex
	tokens   Tokens
	gen      int
	renewing chan struct{}
	err      error
	now      func() time.Time
}

// SessionHooks is what a Session tells the app: Persist keeps the tokens of a renewal before their key is handed out; Ended hears that EveryAPI no longer takes the sign-in; PersistFailed hears that keeping a renewal failed, nil once it worked again. Each runs on the goroutine of the request that renewed.
type SessionHooks struct {
	Persist       func(Tokens) error
	Ended         func(err error)
	PersistFailed func(err error)
}

// NewSession uses tokens, a sign-in Login, a Device or the app's store gave.
func NewSession(cfg Config, tokens Tokens, hooks SessionHooks) *Session {
	s := &Session{cfg: cfg.withDefaults(), tokens: tokens, persist: hooks.Persist, ended: hooks.Ended, persistFailed: hooks.PersistFailed, now: time.Now}
	if s.persist == nil {
		s.persist = func(Tokens) error { return nil }
	}
	return s
}

// Tokens is the sign-in as the session holds it now.
func (s *Session) Tokens() Tokens {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.tokens
}

// Gen is the generation of the key the session hands out: 0 for the one it started with, one more after each renewal.
func (s *Session) Gen() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.gen
}

// Key is the key to send: the one the session holds, or with rejected, a key the API refused, a renewed one. rejected that is not the key the session holds is of an earlier generation, which a renewal replaced already, and gets the key it holds. A renewal under way is waited for, by every request, so the session renews once for all of them; ctx bounds the wait, while the renewal itself goes on apart from it (renewTimeout), so a request that gives up does not leave the rotated refresh token unread. After the sign-in ended, every request gets ErrSignedOut.
func (s *Session) Key(ctx context.Context, rejected string) (Tokens, int, error) {
	s.mu.Lock()
	for s.renewing != nil {
		wait := s.renewing
		s.mu.Unlock()
		select {
		case <-wait:
		case <-ctx.Done():
			return Tokens{}, 0, ctx.Err()
		}
		s.mu.Lock()
	}
	if s.err != nil {
		err := s.err
		s.mu.Unlock()
		return Tokens{}, 0, err
	}
	stale := rejected != "" && rejected == s.tokens.Key
	expiring := !s.tokens.Expires.IsZero() && s.now().Add(renewAhead).After(s.tokens.Expires)
	if !stale && !expiring && s.tokens.Key != "" {
		tokens, gen := s.tokens, s.gen
		s.mu.Unlock()
		return tokens, gen, nil
	}
	done := make(chan struct{})
	s.renewing = done
	refresh := s.tokens.Refresh
	s.mu.Unlock()

	renewCtx, cancel := context.WithTimeout(context.WithoutCancel(ctx), renewTimeout)
	next, err := Refresh(renewCtx, s.cfg, refresh)
	cancel()
	var persistErr error
	if err == nil {
		persistErr = s.persist(next)
	}

	s.mu.Lock()
	s.renewing = nil
	close(done)
	switch {
	case errors.Is(err, ErrSignedOut):
		s.err = ErrSignedOut
		s.mu.Unlock()
		if s.ended != nil {
			s.ended(err)
		}
		return Tokens{}, 0, ErrSignedOut
	case err != nil && expiring && !stale:
		// A key about to expire that could not be renewed now still works until it does.
		tokens, gen := s.tokens, s.gen
		s.mu.Unlock()
		return tokens, gen, nil
	case err != nil:
		s.mu.Unlock()
		return Tokens{}, 0, err
	}
	s.tokens = next
	s.gen++
	tokens, gen := s.tokens, s.gen
	s.mu.Unlock()
	if s.persistFailed != nil {
		s.persistFailed(persistErr)
	}
	return tokens, gen, nil
}

// End stops the session: every request after it gets ErrSignedOut, as after signing out. A renewal under way is waited for, so its tokens are the ones End returns to revoke.
func (s *Session) End() Tokens {
	s.mu.Lock()
	for s.renewing != nil {
		wait := s.renewing
		s.mu.Unlock()
		<-wait
		s.mu.Lock()
	}
	defer s.mu.Unlock()
	s.err = ErrSignedOut
	return s.tokens
}
