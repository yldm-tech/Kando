package main

import (
	"context"
	"crypto/pbkdf2"
	"crypto/sha256"
	"net/http"
	"path/filepath"
	"slices"
	"testing"
	"time"

	"kando/apps/kando/shelf"
	psync "kando/packages/sync"
)

// fastLock builds a valid pbkdf2-sha256 shelf.Lock with fewer rounds so -race tests do not spend seconds per Matches call.
func fastLock(t *testing.T, password string) shelf.Lock {
	t.Helper()
	salt := []byte("test-salt-16byte")
	key, err := pbkdf2.Key(sha256.New, password, salt, 1000, 32)
	if err != nil {
		t.Fatal(err)
	}
	return shelf.Lock{Algorithm: shelf.LockAlgorithm, Iterations: 1000, Salt: salt, Key: key}
}

// A paired phone sees locked password-protected libraries when asking with WithLocked, can unlock one for its own session without exposing it on the desktop window or to other paired devices, and can lock it back.
func TestPhoneCanUnlockAndLockALockedLibraryWithoutExposingDesktop(t *testing.T) {
	a, tt, updates, priv, _ := phoneSyncApp(t)
	other := t.TempDir()
	remoteWriteFile(t, filepath.Join(other, "clip.mp4"), "clip")
	a.edit(func(s *session) { s.Libraries = []string{other, priv}; s.LibraryDir = other })
	a.library.SetLock(priv, fastLock(t, "secret"))
	phoneSyncOn(t, a, tt, updates)

	// Pair two separate phones.
	c1 := phoneClient(t, a, "")
	if _, err := phonePair(t, a, tt, updates, c1, a.phone.offer.Code); err != nil {
		t.Fatal(err)
	}
	apply(tt, a, func() { a.phoneSyncNewCode() })
	c2 := phoneClient(t, a, "")
	if _, err := phonePair(t, a, tt, updates, c2, a.phone.offer.Code); err != nil {
		t.Fatal(err)
	}

	privID := phoneLibraryID(priv)

	// Old client endpoint (without ?with=locked) omits the locked library.
	plain, err := phoneCall(t, a, tt, updates, c1.Libraries)
	if err != nil || len(plain) != 1 || plain[0].ID != phoneLibraryID(other) {
		t.Fatalf("Libraries() = %+v (%v), want only unlocked library", plain, err)
	}

	// New client endpoint (?with=locked) lists both, with priv marked locked and 0 films.
	libs, err := phoneCall(t, a, tt, updates, c1.LibrariesWithLocked)
	if err != nil || len(libs) != 2 {
		t.Fatalf("LibrariesWithLocked() = %+v (%v), want 2 libraries", libs, err)
	}
	i := slices.IndexFunc(libs, func(l psync.Library) bool { return l.ID == privID })
	if i < 0 || !libs[i].Locked || !libs[i].HasPassword || libs[i].Films != 0 {
		t.Fatalf("locked library entry = %+v, want locked=true has_password=true films=0", libs[i])
	}

	// Before unlocking, films are refused with 404.
	_, err = phoneCall(t, a, tt, updates, func(ctx context.Context) ([]psync.Film, error) { return c1.Films(ctx, privID) })
	phoneWantCode(t, err, http.StatusNotFound, psync.CodeNotFound)

	// Wrong password returns 403 wrong_password.
	_, err = phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.LibraryLockState, error) {
		return c1.UnlockLibrary(ctx, privID, "wrong")
	})
	phoneWantCode(t, err, http.StatusForbidden, psync.CodeWrongPassword)

	// Right password unlocks priv for c1 only.
	st, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.LibraryLockState, error) {
		return c1.UnlockLibrary(ctx, privID, "secret")
	})
	if err != nil || !st.Unlocked {
		t.Fatalf("UnlockLibrary = %+v, %v", st, err)
	}

	// Desktop itself still considers priv locked (desktop window never exposes it).
	if !a.LibraryLocked(priv) {
		t.Fatal("unlocking from a phone exposed the library on the desktop")
	}

	// c1 sees priv unlocked and can read its films.
	libs1, err := phoneCall(t, a, tt, updates, c1.LibrariesWithLocked)
	if err != nil {
		t.Fatal(err)
	}
	i = slices.IndexFunc(libs1, func(l psync.Library) bool { return l.ID == privID })
	if i < 0 || libs1[i].Locked || !libs1[i].HasPassword || libs1[i].Films == 0 {
		t.Fatalf("c1 unlocked library = %+v, want locked=false has_password=true films>0", libs1[i])
	}
	films1, err := phoneCall(t, a, tt, updates, func(ctx context.Context) ([]psync.Film, error) { return c1.Films(ctx, privID) })
	if err != nil || len(films1) == 0 {
		t.Fatalf("c1 Films = %+v, %v", films1, err)
	}

	// c2 (the other paired phone) still sees priv locked and cannot read its films.
	libs2, err := phoneCall(t, a, tt, updates, c2.LibrariesWithLocked)
	if err != nil {
		t.Fatal(err)
	}
	j := slices.IndexFunc(libs2, func(l psync.Library) bool { return l.ID == privID })
	if j < 0 || !libs2[j].Locked || libs2[j].Films != 0 {
		t.Fatalf("c2 library = %+v, want locked=true films=0", libs2[j])
	}
	_, err = phoneCall(t, a, tt, updates, func(ctx context.Context) ([]psync.Film, error) { return c2.Films(ctx, privID) })
	phoneWantCode(t, err, http.StatusNotFound, psync.CodeNotFound)

	// c1 locks priv again.
	st, err = phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.LibraryLockState, error) {
		return c1.LockLibrary(ctx, privID)
	})
	if err != nil || st.Unlocked {
		t.Fatalf("LockLibrary = %+v, %v", st, err)
	}
	_, err = phoneCall(t, a, tt, updates, func(ctx context.Context) ([]psync.Film, error) { return c1.Films(ctx, privID) })
	phoneWantCode(t, err, http.StatusNotFound, psync.CodeNotFound)
}

// Wrong unlock passwords are rate-limited per device and accepted again once the window passes.
func TestPhoneUnlockRateLimitsWrongPasswords(t *testing.T) {
	a, tt, updates, dir, clock := phoneSyncApp(t)
	a.library.SetLock(dir, fastLock(t, "secret"))
	phoneSyncOn(t, a, tt, updates)
	c := phoneClient(t, a, "")
	if _, err := phonePair(t, a, tt, updates, c, a.phone.offer.Code); err != nil {
		t.Fatal(err)
	}
	libID := phoneLibraryID(dir)

	for range phoneUnlockLimit {
		_, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.LibraryLockState, error) {
			return c.UnlockLibrary(ctx, libID, "wrong")
		})
		phoneWantCode(t, err, http.StatusForbidden, psync.CodeWrongPassword)
	}
	_, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.LibraryLockState, error) {
		return c.UnlockLibrary(ctx, libID, "secret")
	})
	phoneWantCode(t, err, http.StatusTooManyRequests, psync.CodeTooMany)

	clock.Add(phoneUnlockWindow + time.Second)
	st, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.LibraryLockState, error) {
		return c.UnlockLibrary(ctx, libID, "secret")
	})
	if err != nil || !st.Unlocked {
		t.Fatalf("after cooldown UnlockLibrary = %+v, %v", st, err)
	}
}

// Desktop unlocking or locking a library emits EventFilms so paired phones refresh automatically, and LockNow revokes phone unlocks too.
func TestDesktopLockChangeNotifiesPhonesAndRevokesUnlocks(t *testing.T) {
	a, tt, updates, dir, c := phonePaired(t)
	p := phoneTT{a: a, tt: tt, updates: updates}
	apply(tt, a, func() { a.library.SetLock(dir, fastLock(t, "secret")) })
	libID := phoneLibraryID(dir)

	st, err := phoneAsk(t, p, func(ctx context.Context) (psync.LibraryLockState, error) {
		return c.UnlockLibrary(ctx, libID, "secret")
	})
	if err != nil || !st.Unlocked {
		t.Fatalf("UnlockLibrary = %+v, %v", st, err)
	}

	// Desktop unlocks and then calls LockNow: phone hears EventFilms and loses access.
	apply(tt, a, func() { a.lock.unlocked = filepath.Clean(dir) })
	list, err := phoneAsk(t, p, c.Jobs)
	if err != nil {
		t.Fatal(err)
	}
	apply(tt, a, func() { a.LockNow() })
	ev, err := phoneAsk(t, p, func(ctx context.Context) (psync.Events, error) {
		return c.Events(ctx, list.Epoch, list.Rev, 200*time.Millisecond)
	})
	if err != nil {
		t.Fatal(err)
	}
	if !slices.ContainsFunc(ev.Events, func(e psync.Event) bool { return e.Kind == psync.EventFilms }) {
		t.Fatalf("LockNow did not emit EventFilms: %+v", ev.Events)
	}
	_, err = phoneAsk(t, p, func(ctx context.Context) ([]psync.Film, error) { return c.Films(ctx, libID) })
	phoneWantCode(t, err, http.StatusNotFound, psync.CodeNotFound)
}
