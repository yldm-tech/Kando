package learning

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"
)

// The files the learning state is kept in, under the app's data folder: a file per paired computer with its libraries' mirrors, and the time films played on the phone.
const (
	dirName     = "learning"
	watchedFile = "watched.json"
)

// Disk is the learning state's files under the app's data folder.
type Disk struct{ dir string }

// Open is the learning state kept under dir, the app's data folder.
func Open(dir string) Disk { return Disk{dir: filepath.Join(dir, dirName)} }

// desktopFile is the file of computer desktop's mirrors.
func (d Disk) desktopFile(desktop string) string { return filepath.Join(d.dir, desktop+".json") }

// Mirrors are every mirror kept, by computer and library; a file that cannot be read is left out rather than hide the others.
func (d Disk) Mirrors() ([]Mirror, error) {
	paths, err := filepath.Glob(filepath.Join(d.dir, "*.json"))
	if err != nil {
		return nil, err
	}
	var out []Mirror
	for _, p := range paths {
		if filepath.Base(p) == watchedFile {
			continue
		}
		var list []Mirror
		if readJSON(p, &list) != nil {
			continue
		}
		out = append(out, list...)
	}
	slices.SortStableFunc(out, func(a, b Mirror) int { return strings.Compare(a.Key(), b.Key()) })
	return out, nil
}

// SaveDesktop keeps mirrors, computer desktop's, in place of those kept of it.
func (d Disk) SaveDesktop(desktop string, mirrors []Mirror) error {
	return writeJSON(d.desktopFile(desktop), mirrors)
}

// DeleteDesktop forgets computer desktop's mirrors.
func (d Disk) DeleteDesktop(desktop string) error {
	err := os.Remove(d.desktopFile(desktop))
	if errors.Is(err, fs.ErrNotExist) {
		return nil
	}
	return err
}

// storedWatched is Watched as kept, in seconds.
type storedWatched map[string]float64

// Watched is how long films played on the phone each day, none before any played.
func (d Disk) Watched() (Watched, error) {
	var s storedWatched
	if err := readJSON(filepath.Join(d.dir, watchedFile), &s); err != nil && !errors.Is(err, fs.ErrNotExist) {
		return Watched{}, err
	}
	out := Watched{}
	for day, sec := range s {
		out[day] = time.Duration(sec * float64(time.Second))
	}
	return out, nil
}

// SaveWatched keeps w.
func (d Disk) SaveWatched(w Watched) error {
	s := storedWatched{}
	for day, t := range w {
		s[day] = t.Seconds()
	}
	return writeJSON(filepath.Join(d.dir, watchedFile), s)
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// writeJSON writes v to path through a file beside it, so a file is never left half written.
func writeJSON(path string, v any) error {
	data, err := json.Marshal(v)
	if err != nil {
		return err
	}
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return err
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), filepath.Base(path)+".*.tmp")
	if err != nil {
		return err
	}
	_, err = tmp.Write(data)
	if cerr := tmp.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(tmp.Name(), path)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
	}
	return err
}
