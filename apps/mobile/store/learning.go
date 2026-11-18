package store

import (
	"errors"
	"io/fs"
	"net/url"
	"path/filepath"
	"strings"
	"time"

	"kando/apps/mobile/study"
	"kando/packages/sync"
)

// learningDir keeps the libraries' learning state, a file per library under a folder per desktop: learning/<desktop>/<library>.json.
const learningDir = "learning"

// learningPath is the file of library lib of desktop, its ID escaped so it names one file whatever it holds.
func (s *Store) learningPath(desktop, lib string) string {
	return filepath.Join(s.dir, learningDir, url.PathEscape(desktop), url.PathEscape(lib)+".json")
}

// Learning is library lib of desktop's learning state as the phone keeps it, the zero Library before any was kept.
func (s *Store) Learning(desktop, lib string) (study.Library, error) {
	var l study.Library
	err := readJSON(s.learningPath(desktop, lib), &l)
	if errors.Is(err, fs.ErrNotExist) {
		return study.Library{}, nil
	}
	return l, err
}

// SaveLearning keeps l as library lib of desktop's learning state.
func (s *Store) SaveLearning(desktop, lib string, l study.Library) error {
	return writeJSON(s.learningPath(desktop, lib), l)
}

// lookupsFile keeps the words the phone looked up through its computers, so a word looked up once shows again at once, also away from them.
const lookupsFile = "lookups.json"

// MaxLookups is how many words the phone keeps; past that the ones looked up longest ago go first.
const MaxLookups = 2000

// Lookup is a word the phone looked up: what the computer answered, and when.
type Lookup struct {
	Entry sync.WordEntry `json:"entry"`
	At    time.Time      `json:"at"`
}

// LookupKey names a lookup of word in line of a film spoken in spoken, as the computer answers a word anew for another line.
func LookupKey(word, spoken, line string) string {
	return strings.ToLower(strings.TrimSpace(word)) + "\x00" + strings.ToLower(strings.TrimSpace(spoken)) + "\x00" + strings.TrimSpace(line)
}

// Lookups are the words looked up, by LookupKey, none before any were kept.
func (s *Store) Lookups() (map[string]Lookup, error) {
	out := map[string]Lookup{}
	err := readJSON(filepath.Join(s.dir, lookupsFile), &out)
	if errors.Is(err, fs.ErrNotExist) {
		return map[string]Lookup{}, nil
	}
	return out, err
}

// SaveLookups keeps the words looked up, the MaxLookups looked up last of them.
func (s *Store) SaveLookups(m map[string]Lookup) error {
	for len(m) > MaxLookups {
		var oldest string
		for k, v := range m {
			if oldest == "" || v.At.Before(m[oldest].At) {
				oldest = k
			}
		}
		delete(m, oldest)
	}
	return writeJSON(filepath.Join(s.dir, lookupsFile), m)
}
