package session

import (
	"context"

	"kando/apps/mobile/catalog"
	"kando/packages/sync"
)

// UnlockLibrary unlocks a password-protected library on paired computer desktop using password, refreshes that computer's shelf before calling done on the UI thread so the unlocked library's films are ready immediately, and reports any error.
func (s *Session) UnlockLibrary(desktop, libID, password string, done func(error)) {
	d, ok := s.Desktop(desktop)
	c := s.Client(desktop)
	if !ok || c == nil {
		if done != nil {
			done(sync.Errorf(sync.CodeFailed, "computer not paired"))
		}
		return
	}
	s.work.Go(func() {
		ctx, cancel := context.WithTimeout(context.Background(), listFor)
		defer cancel()
		if _, err := c.UnlockLibrary(ctx, libID, password); err != nil {
			if done != nil {
				s.cx.Post(func() { done(err) })
			}
			return
		}
		l, err := catalog.Load(ctx, c, d.DeviceID)
		s.cx.Post(func() {
			if err == nil {
				s.setShelf(desktop, func(Shelf) Shelf { return Shelf{Listing: l, Listed: true} })
				s.keepListed(desktop, l)
				s.readSubscriptions(desktop)
			} else {
				s.RefreshDesktop(desktop)
			}
			if done != nil {
				done(nil)
			}
		})
	})
}

// LockLibrary locks a password-protected library on paired computer desktop again for this device, refreshes that computer's shelf immediately so its films leave the list, and calls done on the UI thread.
func (s *Session) LockLibrary(desktop, libID string, done func(error)) {
	d, ok := s.Desktop(desktop)
	c := s.Client(desktop)
	if !ok || c == nil {
		if done != nil {
			done(sync.Errorf(sync.CodeFailed, "computer not paired"))
		}
		return
	}
	s.work.Go(func() {
		ctx, cancel := context.WithTimeout(context.Background(), listFor)
		defer cancel()
		if _, err := c.LockLibrary(ctx, libID); err != nil {
			if done != nil {
				s.cx.Post(func() { done(err) })
			}
			return
		}
		l, err := catalog.Load(ctx, c, d.DeviceID)
		s.cx.Post(func() {
			if err == nil {
				s.setShelf(desktop, func(Shelf) Shelf { return Shelf{Listing: l, Listed: true} })
				s.keepListed(desktop, l)
				s.readSubscriptions(desktop)
			} else {
				s.RefreshDesktop(desktop)
			}
			if done != nil {
				done(nil)
			}
		})
	})
}
