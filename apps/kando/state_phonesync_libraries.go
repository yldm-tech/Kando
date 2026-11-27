package main

import (
	"context"
	"errors"
	"hash/fnv"
	"log"
	"path/filepath"
	"slices"
	"time"

	"kando/apps/kando/remote"
	"kando/apps/kando/shelf"
	"kando/apps/kando/vault"
	psync "kando/packages/sync"
)

// Per-device unlock for password-protected libraries over Phone Sync (psync.LibraryService):
// a paired phone can unlock a password-protected library for its own session without exposing it on the desktop window or to other paired devices. On macOS its encrypted image stays attached (lockNeeds) while any phone holds it unlocked, and detaches (lockSettle) once locked again, revoked, or stopped.

const (
	phoneUnlockLimit  = 5
	phoneUnlockWindow = time.Minute
)

// phoneLibraries answers OpLibraryUnlock and OpLibraryLock over protocol v2.
type phoneLibraries struct{ a *app }

// phoneLibraryLocked reports whether the library at dir is locked for paired device deviceID: locked on the desktop and not unlocked by deviceID. With deviceID == "" (such as background cloud relay turns), a desktop-locked library is always locked. UI thread only.
func (a *app) phoneLibraryLocked(dir, deviceID string) bool {
	if !a.LibraryLocked(dir) {
		return false
	}
	if deviceID == "" {
		return true
	}
	return !a.phone.unlocked[deviceID][filepath.Clean(dir)]
}

// phoneHasUnlocked reports whether any paired phone currently holds the library at dir unlocked. UI thread only.
func (a *app) phoneHasUnlocked(dir string) bool {
	dir = filepath.Clean(dir)
	for _, libs := range a.phone.unlocked {
		if libs[dir] {
			return true
		}
	}
	return false
}

// phoneClearUnlocked revokes every phone's unlock of the library at dir (or all libraries when dir == ""). UI thread only.
func (a *app) phoneClearUnlocked(dir string) {
	if dir == "" {
		clear(a.phone.unlocked)
		return
	}
	dir = filepath.Clean(dir)
	for dev, libs := range a.phone.unlocked {
		delete(libs, dir)
		if len(libs) == 0 {
			delete(a.phone.unlocked, dev)
		}
	}
}

// phoneClearDeviceUnlocked revokes every library unlock held by deviceID, as when the device is unpaired. UI thread only.
func (a *app) phoneClearDeviceUnlocked(deviceID string) {
	delete(a.phone.unlocked, deviceID)
	delete(a.phone.unlockFails, deviceID)
	a.lockSettle()
}

// phoneMoveUnlocked re-keys per-device library unlocks when a library folder is renamed in Finder. UI thread only.
func (a *app) phoneMoveUnlocked(old, next string) {
	old, next = filepath.Clean(old), filepath.Clean(next)
	if old == next {
		return
	}
	for _, libs := range a.phone.unlocked {
		if libs[old] {
			delete(libs, old)
			libs[next] = true
		}
	}
}

// phoneUnlockAllowed reports whether deviceID is within the wrong-password rate limit at now. UI thread only.
func (a *app) phoneUnlockAllowed(deviceID string, now time.Time) bool {
	list := a.phone.unlockFails[deviceID]
	kept := list[:0]
	for _, t := range list {
		if now.Sub(t) < phoneUnlockWindow {
			kept = append(kept, t)
		}
	}
	if len(kept) == 0 {
		delete(a.phone.unlockFails, deviceID)
		return true
	}
	a.phone.unlockFails[deviceID] = kept
	return len(kept) < phoneUnlockLimit
}

// phoneUnlockFailed records a failed password attempt by deviceID at now. UI thread only.
func (a *app) phoneUnlockFailed(deviceID string, now time.Time) {
	if a.phone.unlockFails == nil {
		a.phone.unlockFails = map[string][]time.Time{}
	}
	a.phone.unlockFails[deviceID] = append(a.phone.unlockFails[deviceID], now)
}

// phoneLockSign sums up which known libraries have passwords and which are unlocked on the desktop, independent of order, so desktop lock/unlock changes wake connected phones. UI thread only.
func (a *app) phoneLockSign() uint64 {
	var sign uint64
	for _, dir := range a.knownLibraries() {
		clean := filepath.Clean(dir)
		h := fnv.New64a()
		_, _ = h.Write([]byte(clean))
		var state byte
		if a.LibraryHasPassword(clean) {
			state |= 1
		}
		if a.LibraryLocked(clean) {
			state |= 2
		}
		_, _ = h.Write([]byte{state})
		sign += h.Sum64()
	}
	return sign
}

// phoneSyncPublishLocks notifies phones waiting for events when any known library's desktop lock state changes (unlocked, locked, password added/removed). UI thread only.
func (a *app) phoneSyncPublishLocks() {
	srv := a.phone.server
	if srv == nil {
		return
	}
	sign := a.phoneLockSign()
	if sign == a.phone.lockSign {
		return
	}
	a.phone.lockSign = sign
	srv.PublishFilms("")
}

// Unlock verifies req.Password for req.Library and unlocks it for caller.DeviceID.
func (p phoneLibraries) Unlock(ctx context.Context, caller psync.Caller, req psync.LibraryUnlock) (psync.LibraryLockState, error) {
	a := p.a
	type check struct {
		dir      string
		lock     shelf.Lock
		hasKey   bool
		hasVault bool
		noPass   bool
	}
	c, err := remote.OnUI(ctx, a.update, func() (check, error) {
		now := a.phoneStateNow()
		if !a.phoneUnlockAllowed(caller.DeviceID, now) {
			return check{}, psync.Errorf(psync.CodeTooMany, "too many wrong passwords; wait a minute")
		}
		var dir string
		for _, d := range a.knownLibraries() {
			if phoneLibraryID(d) == req.Library {
				dir = filepath.Clean(d)
				break
			}
		}
		if dir == "" {
			return check{}, psync.Errorf(psync.CodeNotFound, "no library %q", req.Library)
		}
		if !a.LibraryHasPassword(dir) {
			return check{dir: dir, noPass: true}, nil
		}
		if a.lock.vault.work == dir {
			return check{}, psync.Errorf(psync.CodeNotReady, "the library's encryption is changing; try again in a moment")
		}
		l, hasKey := a.library.LockOf(dir)
		return check{dir: dir, lock: l, hasKey: hasKey, hasVault: vault.Has(dir)}, nil
	})
	if err != nil {
		return psync.LibraryLockState{}, err
	}
	if c.noPass {
		return psync.LibraryLockState{Unlocked: true}, nil
	}

	ok := !c.hasKey || c.lock.Matches(req.Password)
	var verr error
	if ok && c.hasVault && !vault.Mounted(c.dir) {
		verr = vault.Open(ctx, c.dir, req.Password)
		if !c.hasKey && errors.Is(verr, vault.ErrNotOpened) {
			ok, verr = false, nil
		}
	}

	return remote.OnUI(ctx, a.update, func() (psync.LibraryLockState, error) {
		if c.hasVault && vault.Mounted(c.dir) {
			a.lockVaultOpened(c.dir)
		}
		if !ok {
			a.phoneUnlockFailed(caller.DeviceID, a.phoneStateNow())
			a.lockSettle()
			return psync.LibraryLockState{}, psync.Errorf(psync.CodeWrongPassword, "wrong password")
		}
		if verr != nil {
			a.lockSettle()
			log.Printf("phonesync: unlock %s: %v", filepath.Base(c.dir), verr)
			return psync.LibraryLockState{}, psync.Errorf(psync.CodeFailed, "could not open the encrypted library")
		}
		if a.lock.vault.work == c.dir {
			a.lockSettle()
			return psync.LibraryLockState{}, psync.Errorf(psync.CodeNotReady, "the library's encryption is changing; try again in a moment")
		}
		// Ensure the library's password did not change while we checked it off the UI thread.
		l2, hasKey2 := a.library.LockOf(c.dir)
		if hasKey2 != c.hasKey || !slices.Equal(l2.Key, c.lock.Key) {
			a.lockSettle()
			return psync.LibraryLockState{}, psync.Errorf(psync.CodeWrongPassword, "wrong password")
		}
		delete(a.phone.unlockFails, caller.DeviceID)
		if a.phone.unlocked == nil {
			a.phone.unlocked = map[string]map[string]bool{}
		}
		if a.phone.unlocked[caller.DeviceID] == nil {
			a.phone.unlocked[caller.DeviceID] = map[string]bool{}
		}
		a.phone.unlocked[caller.DeviceID][c.dir] = true
		if srv := a.phone.server; srv != nil {
			srv.PublishFilms(req.Library)
		}
		return psync.LibraryLockState{Unlocked: true}, nil
	})
}

// Lock locks req.Library again for caller.DeviceID (and on the desktop if the desktop had it open and caller is not read-only), flushing its ledger before any encrypted image detaches.
func (p phoneLibraries) Lock(ctx context.Context, caller psync.Caller, req psync.LibraryRef) (psync.LibraryLockState, error) {
	a := p.a
	return remote.OnUI(ctx, a.update, func() (psync.LibraryLockState, error) {
		var dir string
		for _, d := range a.knownLibraries() {
			if phoneLibraryID(d) == req.Library {
				dir = filepath.Clean(d)
				break
			}
		}
		if dir == "" {
			return psync.LibraryLockState{}, psync.Errorf(psync.CodeNotFound, "no library %q", req.Library)
		}
		if !a.LibraryHasPassword(dir) {
			return psync.LibraryLockState{Unlocked: true}, nil
		}
		if libs := a.phone.unlocked[caller.DeviceID]; libs != nil {
			delete(libs, dir)
			if len(libs) == 0 {
				delete(a.phone.unlocked, caller.DeviceID)
			}
		}
		if !caller.ReadOnly && a.lock.unlocked == dir && filepath.Clean(a.snap().LibraryDir) == dir {
			a.LockNow()
		}
		if a.LibraryLocked(dir) && !a.phoneHasUnlocked(dir) && !a.lockFilesAway(dir) {
			if err := a.phoneStateKeepLedger(dir); err != nil {
				log.Printf("phonesync: keep ledger before locking %s: %v", filepath.Base(dir), err)
			}
			delete(a.phone.ledgers, dir)
		}
		a.lockSettle()
		if srv := a.phone.server; srv != nil {
			srv.PublishFilms(req.Library)
		}
		return psync.LibraryLockState{Unlocked: !a.phoneLibraryLocked(dir, caller.DeviceID)}, nil
	})
}
