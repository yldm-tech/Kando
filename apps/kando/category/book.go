// Package category is the library's 分类: groups the user sorts a library's films into, as 电影 and 动漫 in a library for learning Japanese. A film may be in several or in none; a category is no folder on disk, and putting a film in one or taking it out never moves or renames a file. The categories of a library are kept in its folder (FileName), so they go wherever the library goes.
package category

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"kando/packages/study"

	"golang.org/x/text/collate"
	"golang.org/x/text/language"
)

// FileName is the file in a library's folder its categories are kept in, beside the study record (study.json).
const FileName = "categories.json"

// Category is one of a library's categories: its ID, which a film's membership names it by and which stays as it is renamed, and its name.
type Category struct {
	ID   string `json:"id"`
	Name string `json:"name"`
}

// The reasons Add and Rename refuse a name.
var (
	ErrNameEmpty = errors.New("category: the name is empty")
	ErrNameTaken = errors.New("category: another category has this name")
)

// Book is the categories of the library in its folder and the films in each. A film is named by its video, as the study record names it (study.KeyIn): its path under the library's folder, or for a video kept outside the library its full path, so a film keeps its categories as long as the shelf follows its video (MoveVideos).
type Book struct {
	dir string
	// Categories are the library's categories, in the order they were made.
	Categories []Category
	// members is the IDs of the categories each film is in, by its key, each list in the order the film was put in them. A film in none has no entry.
	members map[string][]string
	// unread is why Load could not read the file in full; while it is set Save leaves it as it is.
	unread error
}

// ErrUnread is what Save returns for a book whose file Load could not read, so the file is not written over with less than it holds.
var ErrUnread = errors.New("categories.json could not be read, so it is not written over")

type storedBook struct {
	Categories []Category          `json:"categories"`
	Members    map[string][]string `json:"members,omitempty"`
}

// Load reads the categories of the library folder dir. A library with no file has none. A file that is there but cannot be read gives the error, and a book that Save will not write until the file can be read.
func Load(dir string) (Book, error) {
	b := Book{dir: dir, members: map[string][]string{}}
	if dir == "" {
		return b, nil
	}
	data, err := os.ReadFile(filepath.Join(dir, FileName))
	if errors.Is(err, os.ErrNotExist) {
		return b, nil
	}
	if err == nil && len(bytes.TrimSpace(data)) > 0 {
		var stored storedBook
		if err = json.Unmarshal(data, &stored); err == nil {
			b.load(stored)
		}
	}
	if err != nil {
		b.unread = err
	}
	return b, err
}

// load takes in what the file holds, leaving out a category without an ID or a name, one whose ID or name another before it has, and the memberships of categories that are not there.
func (b *Book) load(stored storedBook) {
	ids, names := map[string]bool{}, map[string]bool{}
	for _, c := range stored.Categories {
		c.Name = strings.TrimSpace(c.Name)
		if c.ID == "" || c.Name == "" || ids[c.ID] || names[nameKey(c.Name)] {
			continue
		}
		ids[c.ID], names[nameKey(c.Name)] = true, true
		b.Categories = append(b.Categories, c)
	}
	for key, list := range stored.Members {
		var kept []string
		for _, id := range list {
			if ids[id] && !slices.Contains(kept, id) {
				kept = append(kept, id)
			}
		}
		if key != "" && len(kept) > 0 {
			b.members[key] = kept
		}
	}
}

// Dir is the library folder the book is of.
func (b Book) Dir() string { return b.dir }

// Unread is why Load could not read the file, nil when it read it or there was none.
func (b Book) Unread() error { return b.unread }

// Save writes the book into its library's folder, through a file beside it that then takes its name, so a write cut short leaves the file as it was. It writes nothing for a book of no folder, and returns ErrUnread for one whose file could not be read.
func (b Book) Save() error {
	if b.dir == "" {
		return nil
	}
	if b.unread != nil {
		return ErrUnread
	}
	stored := storedBook{Categories: b.Categories, Members: b.members}
	if stored.Categories == nil {
		stored.Categories = []Category{}
	}
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return err
	}
	path := filepath.Join(b.dir, FileName)
	tmp, err := os.CreateTemp(b.dir, "."+FileName+"-*")
	if err != nil {
		return err
	}
	if _, err := tmp.Write(append(data, '\n')); err != nil {
		tmp.Close()
		os.Remove(tmp.Name())
		return err
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	if err := os.Rename(tmp.Name(), path); err != nil {
		os.Remove(tmp.Name())
		return err
	}
	return nil
}

// Key is how the book names the film whose video is at path (study.KeyIn).
func (b Book) Key(path string) string { return study.KeyIn(b.dir, path) }

// Find is the category id, and whether the library has it.
func (b Book) Find(id string) (Category, bool) {
	i := slices.IndexFunc(b.Categories, func(c Category) bool { return c.ID == id })
	if i < 0 {
		return Category{}, false
	}
	return b.Categories[i], true
}

// nameKey is a name as two names are compared by: the same names but for case, as 动漫 and 动漫, or Anime and anime, are one name.
func nameKey(name string) string { return strings.ToLower(strings.TrimSpace(name)) }

// checkName is name with the spaces around it gone, or why it cannot be the name of a category other than except: it is empty, or another category has it.
func (b Book) checkName(name, except string) (string, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return "", ErrNameEmpty
	}
	if slices.ContainsFunc(b.Categories, func(c Category) bool { return c.ID != except && nameKey(c.Name) == nameKey(name) }) {
		return "", ErrNameTaken
	}
	return name, nil
}

// Add makes a category named name, the spaces around it left out, and returns it.
func (b *Book) Add(name string) (Category, error) {
	name, err := b.checkName(name, "")
	if err != nil {
		return Category{}, err
	}
	c := Category{ID: newID(), Name: name}
	b.Categories = append(b.Categories, c)
	return c, nil
}

// Rename gives the category id the name name, the spaces around it left out. Its films stay in it.
func (b *Book) Rename(id, name string) error {
	i := slices.IndexFunc(b.Categories, func(c Category) bool { return c.ID == id })
	if i < 0 {
		return nil
	}
	name, err := b.checkName(name, id)
	if err != nil {
		return err
	}
	b.Categories[i].Name = name
	return nil
}

// Delete takes the category id away, and its films out of it. The films stay in the library.
func (b *Book) Delete(id string) {
	b.Categories = slices.DeleteFunc(b.Categories, func(c Category) bool { return c.ID == id })
	for key, list := range b.members {
		b.setMembers(key, slices.DeleteFunc(slices.Clone(list), func(x string) bool { return x == id }))
	}
}

// Of is the IDs of the categories the film whose video is at path is in, in the order it was put in them.
func (b Book) Of(path string) []string {
	if path == "" {
		return nil
	}
	return b.members[b.Key(path)]
}

// In reports whether the film whose video is at path is in the category id.
func (b Book) In(path, id string) bool { return slices.Contains(b.Of(path), id) }

// Put puts the films whose videos are at paths in the category id, or with in false takes them out of it. It reports whether any film's categories changed.
func (b *Book) Put(id string, in bool, paths ...string) bool {
	if _, ok := b.Find(id); !ok {
		return false
	}
	changed := false
	for _, path := range paths {
		if path == "" {
			continue
		}
		key := b.Key(path)
		list := b.members[key]
		has := slices.Contains(list, id)
		switch {
		case in && !has:
			b.setMembers(key, append(slices.Clone(list), id))
			changed = true
		case !in && has:
			b.setMembers(key, slices.DeleteFunc(slices.Clone(list), func(x string) bool { return x == id }))
			changed = true
		}
	}
	return changed
}

// Move takes the films whose videos are at paths out of the category from and puts them in the category to, as moving them from 电影 to 动漫 does. Their other categories stay. It reports whether any film's categories changed.
func (b *Book) Move(from, to string, paths ...string) bool {
	if _, ok := b.Find(to); !ok || from == to {
		return false
	}
	out := b.Put(from, false, paths...)
	return b.Put(to, true, paths...) || out
}

// setMembers keeps list as the categories of the film key, none when it is empty.
func (b *Book) setMembers(key string, list []string) {
	if b.members == nil {
		b.members = map[string][]string{}
	}
	if len(list) == 0 {
		delete(b.members, key)
		return
	}
	b.members[key] = list
}

// MoveVideos keys the films whose videos moved on disk by their new paths: move gives a video's new path for its old one, and reports false for one that did not move. It reports whether any key changed.
func (b *Book) MoveVideos(move func(path string) (string, bool)) bool {
	changed := false
	for _, key := range sortedKeys(b.members) {
		path := key
		if !filepath.IsAbs(filepath.FromSlash(key)) {
			path = filepath.Join(b.dir, filepath.FromSlash(key))
		}
		next, ok := move(path)
		if !ok {
			continue
		}
		if nextKey := b.Key(next); nextKey != key {
			list := b.members[key]
			delete(b.members, key)
			b.setMembers(nextKey, mergeIDs(b.members[nextKey], list))
			changed = true
		}
	}
	return changed
}

// mergeIDs is the IDs of a, then those of b that a does not have.
func mergeIDs(a, b []string) []string {
	out := slices.Clone(a)
	for _, id := range b {
		if !slices.Contains(out, id) {
			out = append(out, id)
		}
	}
	return out
}

func sortedKeys(m map[string][]string) []string {
	keys := make([]string, 0, len(m))
	for key := range m {
		keys = append(keys, key)
	}
	slices.Sort(keys)
	return keys
}

// newID is a new category's ID: 8 random bytes in hex, which no other category of the library has.
func newID() string {
	var raw [8]byte
	_, _ = rand.Read(raw[:])
	return hex.EncodeToString(raw[:])
}

// Sorted is categories ordered by name as the language tag orders words, numbers by their value, as the sidebar lists them.
func Sorted(categories []Category, tag string) []Category {
	out := slices.Clone(categories)
	col := collate.New(language.Make(tag), collate.Numeric, collate.IgnoreCase)
	slices.SortStableFunc(out, func(x, y Category) int { return col.CompareString(x.Name, y.Name) })
	return out
}
