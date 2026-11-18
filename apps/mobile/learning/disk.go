package learning

import (
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"time"
)

// The file the time films played on the phone is kept in, under the app's data folder, beside the libraries' learning state, which the store keeps a folder per computer of (store.Learning).
const (
	dirName     = "learning"
	watchedFile = "watched.json"
)

// Disk is the time watched's file under the app's data folder.
type Disk struct{ dir string }

// Open is the time watched kept under dir, the app's data folder.
func Open(dir string) Disk { return Disk{dir: filepath.Join(dir, dirName)} }

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
