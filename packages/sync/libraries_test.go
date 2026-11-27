package sync

import (
	"context"
	"encoding/json"
	"net/http"
	"testing"
)

type fakeLibraryService struct {
	passwords map[string]string
	unlocked  map[string]map[string]bool
	calls     int
}

func (f *fakeLibraryService) Unlock(_ context.Context, caller Caller, req LibraryUnlock) (LibraryLockState, error) {
	f.calls++
	want, ok := f.passwords[req.Library]
	if !ok {
		return LibraryLockState{}, Errorf(CodeNotFound, "no locked library %q", req.Library)
	}
	if req.Password != want {
		return LibraryLockState{}, Errorf(CodeWrongPassword, "wrong password")
	}
	if f.unlocked == nil {
		f.unlocked = map[string]map[string]bool{}
	}
	if f.unlocked[caller.DeviceID] == nil {
		f.unlocked[caller.DeviceID] = map[string]bool{}
	}
	f.unlocked[caller.DeviceID][req.Library] = true
	return LibraryLockState{Unlocked: true}, nil
}

func (f *fakeLibraryService) Lock(_ context.Context, caller Caller, req LibraryRef) (LibraryLockState, error) {
	if _, ok := f.passwords[req.Library]; !ok {
		return LibraryLockState{}, Errorf(CodeNotFound, "no locked library %q", req.Library)
	}
	if f.unlocked != nil {
		delete(f.unlocked[caller.DeviceID], req.Library)
	}
	return LibraryLockState{Unlocked: false}, nil
}

func TestDeskAnswersLibraryUnlockAndLock(t *testing.T) {
	if StatusOf(CodeWrongPassword) != http.StatusForbidden {
		t.Fatalf("StatusOf(CodeWrongPassword) = %d, want 403", StatusOf(CodeWrongPassword))
	}
	if Mutates(OpLibraryUnlock) || Mutates(OpLibraryLock) {
		t.Fatal("library unlock/lock must not be Mutates so read-only devices can view and passwords are not cached in ReplyMemory")
	}

	svc := &fakeLibraryService{passwords: map[string]string{"priv": "secret"}}
	desk := &Desk{Libraries: svc, Journal: NewJournal()}
	ctx := context.Background()
	phone := Caller{DeviceID: "p1", Name: "Phone", ReadOnly: true}

	call := func(caller Caller, id, op string, arg any) Reply {
		raw, _ := json.Marshal(arg)
		return desk.Answer(ctx, caller, Call{ID: id, V: CallVersion, Op: op, Args: raw})
	}

	// Relay is refused.
	r := call(Caller{DeviceID: "p1", Relay: true}, "c1", OpLibraryUnlock, LibraryUnlock{Library: "priv", Password: "secret"})
	if r.Error == nil || r.Error.Code != CodeUnsupported {
		t.Fatalf("relay unlock = %+v, want unsupported", r.Error)
	}

	// Wrong password is refused with CodeWrongPassword.
	r = call(phone, "c2", OpLibraryUnlock, LibraryUnlock{Library: "priv", Password: "wrong"})
	if r.Error == nil || r.Error.Code != CodeWrongPassword {
		t.Fatalf("wrong password = %+v, want wrong_password", r.Error)
	}

	// Right password succeeds even for a read-only device, and same call ID is not cached.
	r = call(phone, "c2", OpLibraryUnlock, LibraryUnlock{Library: "priv", Password: "secret"})
	if r.Error != nil {
		t.Fatalf("right password error: %+v", r.Error)
	}
	var st LibraryLockState
	if err := json.Unmarshal(r.Result, &st); err != nil || !st.Unlocked {
		t.Fatalf("unlock result = %s (%v), want unlocked=true", r.Result, err)
	}

	// Lock locks it again for this device.
	r = call(phone, "c3", OpLibraryLock, LibraryRef{Library: "priv"})
	if r.Error != nil {
		t.Fatalf("lock error: %+v", r.Error)
	}
	if err := json.Unmarshal(r.Result, &st); err != nil || st.Unlocked {
		t.Fatalf("lock result = %s (%v), want unlocked=false", r.Result, err)
	}
	if svc.unlocked["p1"]["priv"] {
		t.Fatal("library still marked unlocked after Lock")
	}
}
