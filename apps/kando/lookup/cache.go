package lookup

import (
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// FileName is the cache's file, beside settings.json: the words looked up, which Settings › 隐私 › 清除缓存 leaves alone, as the AI service may not be there to answer again, offline or without credit.
const FileName = "lookups.json"

// MaxEntries is how many words the cache keeps; past that the ones looked up longest ago go first.
const MaxEntries = 5000

// Cache is the words looked up on this computer, by the word as it was clicked and the language its meaning is in. The zero Cache keeps them in memory only.
type Cache struct {
	path    string
	entries map[string]cached
}

type cached struct {
	Entry Entry     `json:"entry"`
	At    time.Time `json:"at"`
}

// stored is FileName.
type stored struct {
	Version int               `json:"version"`
	Entries map[string]cached `json:"entries"`
}

// Load reads the cache kept at path, empty when there is none yet or it cannot be read, which the next Save writes over: it holds nothing that cannot be looked up again.
func Load(path string) *Cache {
	c := &Cache{path: path, entries: map[string]cached{}}
	if path == "" {
		return c
	}
	data, err := os.ReadFile(path)
	if err != nil {
		return c
	}
	var s stored
	if json.Unmarshal(data, &s) == nil && s.Entries != nil {
		c.entries = s.Entries
	}
	return c
}

// key names word, looked up for a meaning in language, in the cache.
func key(word, language string) string {
	return strings.ToLower(strings.TrimSpace(word)) + "\x00" + strings.TrimSpace(language)
}

// Get is what the cache keeps of word, as clicked, in language.
func (c *Cache) Get(word, language string) (Entry, bool) {
	if c == nil {
		return Entry{}, false
	}
	e, ok := c.entries[key(word, language)]
	return e.Entry, ok
}

// Put keeps e as what word, as clicked, means in language, looked up at at, dropping the oldest once the cache holds more than MaxEntries.
func (c *Cache) Put(word, language string, e Entry, at time.Time) {
	if c.entries == nil {
		c.entries = map[string]cached{}
	}
	c.entries[key(word, language)] = cached{Entry: e, At: at}
	for len(c.entries) > MaxEntries {
		oldest, first := "", time.Time{}
		for k, v := range c.entries {
			if oldest == "" || v.At.Before(first) {
				oldest, first = k, v.At
			}
		}
		delete(c.entries, oldest)
	}
}

// Len is how many words the cache keeps.
func (c *Cache) Len() int { return len(c.entries) }

// Save writes the cache to its file atomically; a Cache without one keeps it in memory.
func (c *Cache) Save() error {
	if c.path == "" {
		return nil
	}
	data, err := json.Marshal(stored{Version: 1, Entries: c.entries})
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(c.path), 0o755); err != nil {
		return err
	}
	tmp := c.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	if err := os.Rename(tmp, c.path); err != nil {
		return errors.Join(err, os.Remove(tmp))
	}
	return nil
}
