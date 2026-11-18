// Package store is what the phone keeps on its disk, in the app's data folder: the desktops it is paired with, and the films it has downloaded from them, each in a folder of its own with its subtitles, its poster, its video and where it was left playing. Nothing here touches the interface; the session reads and writes it off the engine's goroutine.
package store

import (
	"encoding/json"
	"errors"
	"io/fs"
	"net/netip"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"kando/packages/sync"
)

// Desktop is a desktop Kando the phone is paired with.
type Desktop struct {
	// ID names the desktop on the phone: its certificate's fingerprint in short (DesktopID), so pairing again with the same desktop replaces it.
	ID   string `json:"id"`
	Name string `json:"name"`
	// Addr is where it answered at pairing.
	Addr netip.AddrPort `json:"addr"`
	// Fingerprint is its certificate's SHA-256, which every request pins.
	Fingerprint []byte `json:"fingerprint"`
	// Token is the device token it gave, which the session keeps in the platform's secure store under TokenAccount, never in desktops.json.
	Token    string    `json:"-"`
	DeviceID string    `json:"device_id"`
	Paired   time.Time `json:"paired"`
}

// TokenAccount is the account the device token of desktop id is kept under in the platform's secure store.
func TokenAccount(id string) string { return "desktop-token." + id }

// DesktopID is the ID of the desktop whose certificate has fingerprint.
func DesktopID(fingerprint []byte) string {
	return strings.ToLower(strings.ReplaceAll(sync.ShortFingerprint(fingerprint), " ", ""))
}

// Film is a film the phone downloads or has downloaded: the desktop and library it is from, what the desktop says of it, and how far its files are.
type Film struct {
	Desktop string    `json:"desktop"`
	Library string    `json:"library"`
	Film    sync.Film `json:"film"`
	// ETag names the video as the part being downloaded was asked for, so a resumed download goes on from it only if the file has not changed on the desktop.
	ETag string `json:"etag"`
	// Video is the name of the video file in the film's folder once it is whole, "" until then; Poster the poster's, "" for none.
	Video  string `json:"video"`
	Poster string `json:"poster"`
	// PositionMS is where it was left playing, and PositionAt when, zero for a film left before the phone kept when (yldm-tech/Kando#221).
	PositionMS int64     `json:"position_ms"`
	PositionAt time.Time `json:"position_at,omitzero"`
	Added      time.Time `json:"added"`
}

// Key names a film on the phone: its desktop's ID and its own.
func Key(desktop, film string) string { return desktop + "/" + film }

// Key is the film's.
func (f Film) Key() string { return Key(f.Desktop, f.Film.ID) }

// Done reports whether the video is whole.
func (f Film) Done() bool { return f.Video != "" }

// The files of the store, under its folder: the paired desktops, and a folder per film under films/<desktop>/<film>, holding film.json, subtitles.json, the poster, and video.part while the video downloads.
const (
	desktopsFile  = "desktops.json"
	filmsDir      = "films"
	filmFile      = "film.json"
	subtitlesFile = "subtitles.json"
	// PartFile is the video while it downloads.
	PartFile = "video.part"
)

// Store is the phone's files under one folder.
type Store struct{ dir string }

// Open is the store in dir, the app's data folder.
func Open(dir string) *Store { return &Store{dir: dir} }

// Desktops are the paired desktops, in the order they were paired, without their tokens.
func (s *Store) Desktops() ([]Desktop, error) {
	var out []Desktop
	err := readJSON(filepath.Join(s.dir, desktopsFile), &out)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, nil
	}
	return out, err
}

// SaveDesktops keeps desktops, without their tokens, readable by the app alone.
func (s *Store) SaveDesktops(desktops []Desktop) error {
	return writeJSON(filepath.Join(s.dir, desktopsFile), desktops)
}

// FilmDir is the folder of film id of desktop.
func (s *Store) FilmDir(desktop, id string) string {
	return filepath.Join(s.dir, filmsDir, desktop, id)
}

// Films are every film the phone keeps, downloading ones too, the latest added first.
func (s *Store) Films() ([]Film, error) {
	paths, err := filepath.Glob(filepath.Join(s.dir, filmsDir, "*", "*", filmFile))
	if err != nil {
		return nil, err
	}
	var out []Film
	for _, p := range paths {
		var f Film
		if err := readJSON(p, &f); err != nil {
			// A film whose record was cut short, as by the app being killed while writing it, is left out rather than hiding every other.
			continue
		}
		out = append(out, f)
	}
	slices.SortStableFunc(out, func(a, b Film) int { return b.Added.Compare(a.Added) })
	return out, nil
}

// SaveFilm keeps f's record in its folder, making the folder.
func (s *Store) SaveFilm(f Film) error {
	dir := s.FilmDir(f.Desktop, f.Film.ID)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return writeJSON(filepath.Join(dir, filmFile), f)
}

// DeleteFilm removes the film's folder and everything in it.
func (s *Store) DeleteFilm(desktop, id string) error {
	return os.RemoveAll(s.FilmDir(desktop, id))
}

// SaveCues keeps the film's subtitles.
func (s *Store) SaveCues(desktop, id string, cues []sync.Cue) error {
	dir := s.FilmDir(desktop, id)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return err
	}
	return writeJSON(filepath.Join(dir, subtitlesFile), cues)
}

// Cues are the film's subtitles, and false when they are not kept yet.
func (s *Store) Cues(desktop, id string) ([]sync.Cue, bool, error) {
	var cues []sync.Cue
	err := readJSON(filepath.Join(s.FilmDir(desktop, id), subtitlesFile), &cues)
	if errors.Is(err, fs.ErrNotExist) {
		return nil, false, nil
	}
	return cues, err == nil, err
}

// Used is how many bytes the films take on the phone.
func (s *Store) Used() (int64, error) {
	var total int64
	err := filepath.WalkDir(filepath.Join(s.dir, filmsDir), func(_ string, d fs.DirEntry, err error) error {
		if err != nil {
			if errors.Is(err, fs.ErrNotExist) {
				return nil
			}
			return err
		}
		if d.Type().IsRegular() {
			info, err := d.Info()
			if err != nil {
				return err
			}
			total += info.Size()
		}
		return nil
	})
	return total, err
}

func readJSON(path string, v any) error {
	data, err := os.ReadFile(path)
	if err != nil {
		return err
	}
	return json.Unmarshal(data, v)
}

// writeJSON writes v to path through a file beside it, so a record is never left half written. Each write has a file of its own, so two goroutines saving the same record at once do not write into or move each other's; which of them lands last is the caller's to order, as the session does with a film's saves.
func writeJSON(path string, v any) error {
	data, err := json.MarshalIndent(v, "", "  ")
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
		err = replace(tmp.Name(), path)
	}
	if err != nil {
		_ = os.Remove(tmp.Name())
	}
	return err
}
