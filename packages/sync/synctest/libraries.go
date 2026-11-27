package synctest

import (
	"bytes"
	"context"
	"net/http"

	"kando/packages/sync"
)

// locks holds password-protected libraries on the stand-in desktop: passwords by library ID, and which libraries each paired device unlocked.
type locks struct {
	passwords map[string]string
	unlocked  map[string]map[string]bool
	// desktopUnlocked is the libraries unlocked on the desktop itself (visible to every phone).
	desktopUnlocked map[string]bool
}

// AddLockedLibrary adds a password-protected library with id, name and password, locked until unlocked by a phone (OpLibraryUnlock) or on the desktop (SetLibraryUnlocked).
func (d *Desktop) AddLockedLibrary(id, name, password string) {
	d.mu.Lock()
	defer d.mu.Unlock()
	if d.locks.passwords == nil {
		d.locks.passwords = map[string]string{}
	}
	d.locks.passwords[id] = password
	d.libs = append(d.libs, sync.Library{ID: id, Name: name})
}

// SetLibraryUnlocked unlocks or locks library id on the desktop itself; locking it clears every phone's unlock of it too, as desktop LockNow does, and publishes EventFilms.
func (d *Desktop) SetLibraryUnlocked(id string, unlocked bool) {
	d.mu.Lock()
	if unlocked {
		if d.locks.desktopUnlocked == nil {
			d.locks.desktopUnlocked = map[string]bool{}
		}
		d.locks.desktopUnlocked[id] = true
	} else {
		delete(d.locks.desktopUnlocked, id)
		for dev := range d.locks.unlocked {
			delete(d.locks.unlocked[dev], id)
		}
	}
	d.mu.Unlock()
	d.desk.Journal.PublishFilms(id)
}

// requestDeviceID returns the DeviceID of the bearer token on r.
func requestDeviceID(r *http.Request) string {
	token, _ := bytes.CutPrefix([]byte(r.Header.Get("Authorization")), []byte("Bearer "))
	return "dev-" + string(token[:min(6, len(token))])
}

// libLockedLocked reports whether library id is locked for deviceID. Caller holds d.mu.
func (d *Desktop) libLockedLocked(id, deviceID string) bool {
	if _, has := d.locks.passwords[id]; !has {
		return false
	}
	if d.locks.desktopUnlocked[id] {
		return false
	}
	return !d.locks.unlocked[deviceID][id]
}

// libHasPasswordLocked reports whether library id has a password. Caller holds d.mu.
func (d *Desktop) libHasPasswordLocked(id string) bool {
	_, has := d.locks.passwords[id]
	return has
}

type libraryService struct{ d *Desktop }

func (s libraryService) Unlock(_ context.Context, caller sync.Caller, req sync.LibraryUnlock) (sync.LibraryLockState, error) {
	d := s.d
	d.mu.Lock()
	want, ok := d.locks.passwords[req.Library]
	if !ok {
		d.mu.Unlock()
		return sync.LibraryLockState{}, sync.Errorf(sync.CodeNotFound, "no locked library %q", req.Library)
	}
	if req.Password != want {
		d.mu.Unlock()
		return sync.LibraryLockState{}, sync.Errorf(sync.CodeWrongPassword, "wrong password")
	}
	if d.locks.unlocked == nil {
		d.locks.unlocked = map[string]map[string]bool{}
	}
	if d.locks.unlocked[caller.DeviceID] == nil {
		d.locks.unlocked[caller.DeviceID] = map[string]bool{}
	}
	d.locks.unlocked[caller.DeviceID][req.Library] = true
	d.mu.Unlock()
	d.desk.Journal.PublishFilms(req.Library)
	return sync.LibraryLockState{Unlocked: true}, nil
}

func (s libraryService) Lock(_ context.Context, caller sync.Caller, req sync.LibraryRef) (sync.LibraryLockState, error) {
	d := s.d
	d.mu.Lock()
	if _, ok := d.locks.passwords[req.Library]; !ok {
		d.mu.Unlock()
		return sync.LibraryLockState{}, sync.Errorf(sync.CodeNotFound, "no locked library %q", req.Library)
	}
	delete(d.locks.unlocked[caller.DeviceID], req.Library)
	d.mu.Unlock()
	d.desk.Journal.PublishFilms(req.Library)
	return sync.LibraryLockState{Unlocked: false}, nil
}
