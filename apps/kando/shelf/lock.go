package shelf

import (
	"crypto/pbkdf2"
	"crypto/rand"
	"crypto/sha256"
	"crypto/subtle"
	"path/filepath"
)

// A library's password is an app lock, not encryption: Kando asks for it before it shows the library, and its files stay ordinary files on disk. library.json keeps a salted PBKDF2-SHA256 key of the password, never the password itself, with the parameters it was made with, so a later version can raise them for new passwords and still check the old ones.
const (
	// LockAlgorithm is the only key derivation Matches knows.
	LockAlgorithm = "pbkdf2-sha256"
	// LockIterations is how many rounds NewLock makes a key with, OWASP's figure for PBKDF2-HMAC-SHA256 (2023).
	LockIterations = 600_000
	lockSaltSize   = 16
	lockKeySize    = 32
	// lockMaxIterations and lockMaxKeySize bound what Matches derives for a lock read from a file, which a hand edit may have set to something that would take hours or all the memory.
	lockMaxIterations = 100_000_000
	lockMaxKeySize    = 64
)

// Lock is the password of a library: its key derived with Algorithm from the password and Salt in Iterations rounds. Salt and Key are written to library.json in base64.
type Lock struct {
	Algorithm  string `json:"algorithm"`
	Iterations int    `json:"iterations"`
	Salt       []byte `json:"salt"`
	Key        []byte `json:"key"`
}

// NewLock makes the lock of password with a fresh random salt and LockIterations rounds. It never keeps the password.
func NewLock(password string) (Lock, error) {
	salt := make([]byte, lockSaltSize)
	if _, err := rand.Read(salt); err != nil {
		return Lock{}, err
	}
	key, err := pbkdf2.Key(sha256.New, password, salt, LockIterations, lockKeySize)
	if err != nil {
		return Lock{}, err
	}
	return Lock{Algorithm: LockAlgorithm, Iterations: LockIterations, Salt: salt, Key: key}, nil
}

// Matches reports whether password is the one the lock was made from, comparing the keys in constant time. A lock whose parameters it does not know, or that are out of bounds, matches no password, so a damaged lock keeps its library locked rather than open.
func (l Lock) Matches(password string) bool {
	if l.Algorithm != LockAlgorithm || l.Iterations < 1 || l.Iterations > lockMaxIterations || len(l.Salt) == 0 || len(l.Key) == 0 || len(l.Key) > lockMaxKeySize {
		return false
	}
	key, err := pbkdf2.Key(sha256.New, password, l.Salt, l.Iterations, len(l.Key))
	if err != nil {
		return false
	}
	defer clear(key)
	return subtle.ConstantTimeCompare(key, l.Key) == 1
}

// LockOf is the lock of the library at dir, by its cleaned path, and whether it has one.
func (l Library) LockOf(dir string) (Lock, bool) {
	if dir == "" {
		return Lock{}, false
	}
	lock, ok := l.Locks[filepath.Clean(dir)]
	return lock, ok
}

// SetLock gives the library at dir the lock, or takes its lock away when lock is the zero Lock.
func (l *Library) SetLock(dir string, lock Lock) {
	dir = filepath.Clean(dir)
	if lock.Algorithm == "" {
		delete(l.Locks, dir)
		return
	}
	if l.Locks == nil {
		l.Locks = map[string]Lock{}
	}
	l.Locks[dir] = lock
}

// MoveLock keys the lock of the library at old by next, as when the library's folder was renamed in Finder, and reports whether there was one to move.
func (l *Library) MoveLock(old, next string) bool {
	old, next = filepath.Clean(old), filepath.Clean(next)
	lock, ok := l.Locks[old]
	if !ok || old == next {
		return false
	}
	delete(l.Locks, old)
	l.Locks[next] = lock
	return true
}
