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

// Cache is the words looked up on this computer, each by what it was asked with (Query). The zero Cache keeps them in memory only.
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

// version is the version of FileName this package writes; a file of another version is read as empty, as its entries are named another way.
const version = 2

// Query is what a word is looked up with: the word as clicked, the language of the film it was clicked in (Spoken, as whisper's code, "" when not known), the language its meaning is written in, and the line it was clicked in, as what it means is what it means there. A word is looked up again for another line or another language, so "die" of a German film does not show the meaning of "die" of an English one.
type Query struct {
	Word, Spoken, Language, Line string
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
	if json.Unmarshal(data, &s) == nil && s.Version == version && s.Entries != nil {
		c.entries = s.Entries
	}
	return c
}

// key names q in the cache.
func (q Query) key() string {
	return strings.Join([]string{strings.ToLower(strings.TrimSpace(q.Word)), strings.ToLower(strings.TrimSpace(q.Spoken)), strings.TrimSpace(q.Language), strings.TrimSpace(q.Line)}, "\x00")
}

// Get is what the cache keeps of the word looked up as q.
func (c *Cache) Get(q Query) (Entry, bool) {
	if c == nil {
		return Entry{}, false
	}
	e, ok := c.entries[q.key()]
	return e.Entry, ok
}

// Put keeps e as what the word looked up as q means, looked up at at, dropping the oldest once the cache holds more than MaxEntries.
func (c *Cache) Put(q Query, e Entry, at time.Time) {
	if c.entries == nil {
		c.entries = map[string]cached{}
	}
	c.entries[q.key()] = cached{Entry: e, At: at}
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
	data, err := json.Marshal(stored{Version: version, Entries: c.entries})
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
