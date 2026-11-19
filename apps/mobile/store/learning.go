package store

import (
	"errors"
	"io/fs"
	"net/url"
	"os"
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

// LearningKey names library Library of desktop Desktop among the learning states kept.
type LearningKey struct{ Desktop, Library string }

// Learnings are the learning states of every library of every desktop kept on the phone; a file that cannot be read is left out rather than hide the others.
func (s *Store) Learnings() (map[LearningKey]study.Library, error) {
	out := map[LearningKey]study.Library{}
	desktops, err := os.ReadDir(filepath.Join(s.dir, learningDir))
	if errors.Is(err, fs.ErrNotExist) {
		return out, nil
	}
	if err != nil {
		return out, err
	}
	for _, d := range desktops {
		desktop, err := url.PathUnescape(d.Name())
		if !d.IsDir() || err != nil {
			continue
		}
		files, _ := os.ReadDir(filepath.Join(s.dir, learningDir, d.Name()))
		for _, f := range files {
			name, ok := strings.CutSuffix(f.Name(), ".json")
			lib, err := url.PathUnescape(name)
			if f.IsDir() || !ok || err != nil {
				continue
			}
			var l study.Library
			if readJSON(filepath.Join(s.dir, learningDir, d.Name(), f.Name()), &l) == nil {
				out[LearningKey{desktop, lib}] = l
			}
		}
	}
	return out, nil
}

// RemoveLearning forgets library lib of desktop's learning state, and the desktop's folder once it holds no other.
func (s *Store) RemoveLearning(desktop, lib string) error {
	path := s.learningPath(desktop, lib)
	if err := os.Remove(path); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return err
	}
	// Fails, as it should, while the folder holds another library's.
	_ = os.Remove(filepath.Dir(path))
	return nil
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

// SplitLookupKey is the word, the language spoken and the line LookupKey named with k, as it keeps them; ok is false for a key it did not make.
func SplitLookupKey(k string) (word, spoken, line string, ok bool) {
	word, rest, ok1 := strings.Cut(k, "\x00")
	spoken, line, ok2 := strings.Cut(rest, "\x00")
	return word, spoken, line, ok1 && ok2
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
