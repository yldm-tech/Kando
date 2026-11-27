package sync

import (
	"context"
	"net/http"
)

// Password-protected libraries over Phone Sync: a paired phone that asks PathLibraries with WithLocked learns each locked library's ID and name (with Locked set and Films 0), never its films or learning state, and may unlock it for its own session over the local network with OpLibraryUnlock (and lock it back with OpLibraryLock). Unlocking for a phone never exposes the library on the desktop's own window or to other paired devices, and locked libraries are never unlocked through the storage relay. Neither op is Mutates, so a read-only phone may unlock its own viewing session (while staying read-only for writes) and passwords are never kept in ReplyMemory.

// Operations for unlocking and locking a password-protected library for the calling phone.
const (
	// OpLibraryUnlock takes LibraryUnlock and answers LibraryLockState.
	OpLibraryUnlock = "library.unlock"
	// OpLibraryLock takes LibraryRef and answers LibraryLockState.
	OpLibraryLock = "library.lock"
)

// LibraryUnlock asks the desktop to unlock library Library for the calling device using Password.
type LibraryUnlock struct {
	Library  string `json:"library"`
	Password string `json:"password"`
}

// LibraryRef names a library by its protocol ID.
type LibraryRef struct {
	Library string `json:"library"`
}

// LibraryLockState reports whether the library is unlocked for the calling device after the call.
type LibraryLockState struct {
	Unlocked bool `json:"unlocked"`
}

// LibraryService is what a desktop does for OpLibraryUnlock and OpLibraryLock, over the local network. Its errors are as Service's (CodeWrongPassword for a wrong password, CodeTooMany when throttled, CodeNotFound when the library does not exist or has no password).
type LibraryService interface {
	Unlock(ctx context.Context, caller Caller, req LibraryUnlock) (LibraryLockState, error)
	Lock(ctx context.Context, caller Caller, req LibraryRef) (LibraryLockState, error)
}

// answerLibraries runs OpLibraryUnlock and OpLibraryLock.
func (d *Desk) answerLibraries(ctx context.Context, caller Caller, call Call) (any, error) {
	if d.Libraries == nil {
		return nil, Errorf(CodeUnsupported, "this desktop does not unlock libraries for phones")
	}
	if caller.Relay {
		return nil, Errorf(CodeUnsupported, "locked libraries are unlocked on the local network only")
	}
	switch call.Op {
	case OpLibraryUnlock:
		var req LibraryUnlock
		if err := args(call, &req); err != nil {
			return nil, err
		}
		if req.Library == "" {
			return nil, Errorf(CodeInvalid, "%s names no library", call.Op)
		}
		return d.Libraries.Unlock(ctx, caller, req)
	case OpLibraryLock:
		var req LibraryRef
		if err := args(call, &req); err != nil {
			return nil, err
		}
		if req.Library == "" {
			return nil, Errorf(CodeInvalid, "%s names no library", call.Op)
		}
		return d.Libraries.Lock(ctx, caller, req)
	}
	return nil, Errorf(CodeUnsupported, "unknown op %q", call.Op)
}

// LibrariesWithLocked are the desktop's libraries, including locked password-protected ones (Library.Locked, Library.HasPassword) on desktops that support WithLocked. Older desktops ignore the query parameter and return only unlocked libraries as before.
func (c *Client) LibrariesWithLocked(ctx context.Context) ([]Library, error) {
	var out Libraries
	err := c.getJSON(ctx, http.MethodGet, PathLibraries+"?"+QueryWith+"="+WithLocked, nil, &out)
	return out.Libraries, err
}

// UnlockLibrary unlocks library lib for this paired phone using password.
func (c *Client) UnlockLibrary(ctx context.Context, lib, password string) (LibraryLockState, error) {
	var out LibraryLockState
	err := c.Call(ctx, OpLibraryUnlock, LibraryUnlock{Library: lib, Password: password}, &out)
	return out, err
}

// LockLibrary locks library lib again for this paired phone.
func (c *Client) LockLibrary(ctx context.Context, lib string) (LibraryLockState, error) {
	var out LibraryLockState
	err := c.Call(ctx, OpLibraryLock, LibraryRef{Library: lib}, &out)
	return out, err
}
