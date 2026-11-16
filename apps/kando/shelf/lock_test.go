package shelf

import (
	"bytes"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

// A new lock holds the parameters it was made with, at least OWASP's 600,000 rounds of PBKDF2-SHA256, a random salt of its own and a 32-byte key, and matches its password only.
func TestNewLockMatchesItsPasswordOnly(t *testing.T) {
	lock, err := NewLock("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if lock.Algorithm != "pbkdf2-sha256" || lock.Iterations < 600_000 || len(lock.Salt) != 16 || len(lock.Key) != 32 {
		t.Fatalf("lock = %s, %d rounds, %d-byte salt, %d-byte key", lock.Algorithm, lock.Iterations, len(lock.Salt), len(lock.Key))
	}
	if !lock.Matches("correct horse") {
		t.Error("the lock does not match its password")
	}
	for _, wrong := range []string{"", "correct horse ", "Correct horse", "correct hors"} {
		if lock.Matches(wrong) {
			t.Errorf("the lock matches %q", wrong)
		}
	}
	other, err := NewLock("correct horse")
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Equal(other.Salt, lock.Salt) || bytes.Equal(other.Key, lock.Key) {
		t.Error("two locks of one password share their salt or key")
	}
}

// A lock whose parameters Matches does not know or will not derive, as after a hand edit, matches no password, so its library stays locked.
func TestDamagedLockMatchesNothing(t *testing.T) {
	lock, err := NewLock("pw")
	if err != nil {
		t.Fatal(err)
	}
	for name, damage := range map[string]func(*Lock){
		"another algorithm": func(l *Lock) { l.Algorithm = "md5" },
		"no rounds":         func(l *Lock) { l.Iterations = 0 },
		"too many rounds":   func(l *Lock) { l.Iterations = lockMaxIterations + 1 },
		"fewer rounds":      func(l *Lock) { l.Iterations-- },
		"no salt":           func(l *Lock) { l.Salt = nil },
		"no key":            func(l *Lock) { l.Key = nil },
		"a huge key":        func(l *Lock) { l.Key = make([]byte, lockMaxKeySize+1) },
	} {
		damaged := lock
		damaged.Salt, damaged.Key = bytes.Clone(lock.Salt), bytes.Clone(lock.Key)
		damage(&damaged)
		if damaged.Matches("pw") {
			t.Errorf("a lock with %s matches its password", name)
		}
	}
}

// library.json keeps each library's lock by its folder, with the parameters it was made with, and never the password; a lock read back matches as it did, and one Matches cannot check is kept rather than dropped.
func TestLibraryKeepsLocks(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	lib, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	const password = "hunter2-private-films"
	lock, err := NewLock(password)
	if err != nil {
		t.Fatal(err)
	}
	lib.SetLock("/movies/Private.kando/", lock)
	lib.SetLock("/movies/Odd.kando", Lock{Algorithm: "scrypt", Iterations: 1, Salt: []byte{1}, Key: []byte{2}})
	if err := lib.Save(); err != nil {
		t.Fatal(err)
	}
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if bytes.Contains(data, []byte(password)) {
		t.Fatalf("library.json holds the password:\n%s", data)
	}
	var raw struct {
		Locks map[string]map[string]any `json:"locks"`
	}
	if err := json.Unmarshal(data, &raw); err != nil {
		t.Fatal(err)
	}
	stored := raw.Locks[filepath.Clean("/movies/Private.kando")]
	if stored["algorithm"] != "pbkdf2-sha256" || stored["iterations"] != float64(LockIterations) || stored["salt"] == "" || stored["key"] == "" {
		t.Fatalf("stored lock = %v", stored)
	}
	loaded, err := Load(path)
	if err != nil {
		t.Fatal(err)
	}
	got, ok := loaded.LockOf("/movies/Private.kando")
	if !ok || !got.Matches(password) || got.Matches("hunter2") {
		t.Fatalf("lock read back = %+v, %t", got, ok)
	}
	if odd, ok := loaded.LockOf("/movies/Odd.kando"); !ok || odd.Algorithm != "scrypt" {
		t.Fatalf("a lock of an unknown algorithm read back as %+v, %t", odd, ok)
	}
	if _, ok := loaded.LockOf("/movies/Open.kando"); ok {
		t.Fatal("a library without a password has a lock")
	}
}

// Taking a lock away, and moving one with its renamed folder, are kept.
func TestLibrarySetAndMoveLock(t *testing.T) {
	var lib Library
	lock := Lock{Algorithm: LockAlgorithm, Iterations: 1, Salt: []byte{1}, Key: []byte{2}}
	lib.SetLock("/a.kando", lock)
	if !lib.MoveLock("/a.kando", "/b.kando") {
		t.Fatal("the lock did not move")
	}
	if _, ok := lib.LockOf("/a.kando"); ok {
		t.Fatal("the old path keeps the lock")
	}
	if _, ok := lib.LockOf("/b.kando"); !ok {
		t.Fatal("the new path has no lock")
	}
	if lib.MoveLock("/c.kando", "/d.kando") {
		t.Fatal("a library without a lock moved one")
	}
	lib.SetLock("/b.kando", Lock{})
	if len(lib.Locks) != 0 {
		t.Fatalf("locks after removing = %v", lib.Locks)
	}
}
