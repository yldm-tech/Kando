package shelf

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"maps"
	"math"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kando/packages/core"
)

// The error of a stopped job is stored as one of these words so it reads back as the same error; any other error is stored as its text.
const (
	storedCanceled        = "canceled"
	storedLibrarySwitched = "library-switched"
)

// Library is the videos this machine has processed, newest first, where each video stopped playing, how its subtitles were moved against it and over it, and the titles the user gave videos in Kando.
type Library struct {
	path string
	Jobs []core.Job
	// Positions is where the video at each path stopped playing last time, so it can resume there.
	Positions map[string]time.Duration
	// Furthest is how far the video at each path has ever played. Unlike its resume position it stays once the video was watched to the end, so what has been seen of a film is known for good.
	Furthest map[string]time.Duration
	// Played is when the video at each path last played, kept beside its resume position, so the library can offer first the film watched last.
	Played map[string]time.Time
	// Delays is how far the subtitles of the video at each path were moved against its picture (Z and X), so they stay in step when it is opened again. A video whose subtitles were not moved has none.
	Delays map[string]time.Duration
	// Lifts is how far the subtitles of the video at each path were moved up over its picture from the place the subtitle style puts them (dragged, or ⌥↑ and ⌥↓), as a share of the picture's height, negative for down, so they stay off the film's own burned-in text when it is opened again. A video whose subtitles are in their place has none.
	Lifts map[string]float32
	// Names is the title the user gave the video at each path in Kando (Rename), shown in place of the title its site gives it or the name of its file or folder. Renaming never touches the files on disk, and a title kept here follows its video when the video moves (MoveVideos).
	Names map[string]string
	// Locks is the password of each library that has one, by the library folder's cleaned path (Lock). library.json is the machine's, so the lock stays with the library on this machine, follows it when its folder is renamed in Finder (MoveLock), and stays when the library is taken off the list.
	Locks map[string]Lock
	// unread is why Load could not read the file in full. While it is set Save does not write, as the file may hold jobs this library lacks; AllowSave clears it once a copy of the file is kept.
	unread error
}

// ErrUnread is what Save returns for a library whose file Load could not read in full, until AllowSave.
var ErrUnread = errors.New("library.json could not be read, so it is not written over")

// DamagedError is a library.json that is there but is not a library Load can read in full, such as one cut short. Data is the file as it was read, for the caller to keep a copy of.
type DamagedError struct {
	Data []byte
	Err  error
}

func (e *DamagedError) Error() string { return e.Err.Error() }

func (e *DamagedError) Unwrap() error { return e.Err }

func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "Kando", "library.json")
}

// Path is the library.json file this shelf persists.
func (l Library) Path() string { return l.path }

// Load reads the library at path. A missing file is an empty library. A file that is there but cannot be read is empty too, and the error says why. A file that is not a library in full returns a *DamagedError with the file's bytes, and the library keeps what could be read: a value of another type leaves only that field out, as encoding/json goes on with the rest, and a file that is not JSON at all reads as empty. After either error Save returns ErrUnread rather than write over the file, until AllowSave. A job the app was still working on when it quit is read back as queued, so the queue runs it again from the start. A file written before positions were kept, a bare list of jobs, still reads.
func Load(path string) (Library, error) {
	lib := Library{path: path, Positions: map[string]time.Duration{}, Furthest: map[string]time.Duration{}, Played: map[string]time.Time{}, Delays: map[string]time.Duration{}, Lifts: map[string]float32{}, Names: map[string]string{}, Locks: map[string]Lock{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return lib, nil
		}
		lib.unread = err
		return lib, err
	}
	var stored storedLibrary
	if trimmed := bytes.TrimSpace(data); len(trimmed) > 0 && trimmed[0] == '[' {
		err = json.Unmarshal(data, &stored.Jobs)
	} else {
		err = json.Unmarshal(data, &stored)
	}
	if err != nil {
		var typeErr *json.UnmarshalTypeError
		if !errors.As(err, &typeErr) {
			stored = storedLibrary{}
		}
	}
	lib.Jobs = make([]core.Job, len(stored.Jobs))
	for i, s := range stored.Jobs {
		lib.Jobs[i] = s.job()
	}
	for path, seconds := range stored.Positions {
		if seconds > 0 {
			lib.Positions[path] = time.Duration(seconds * float64(time.Second))
		}
	}
	for path, seconds := range stored.Furthest {
		if seconds > 0 {
			lib.Furthest[path] = time.Duration(seconds * float64(time.Second))
		}
	}
	for path, at := range stored.Played {
		if !at.IsZero() {
			lib.Played[path] = at
		}
	}
	for path, seconds := range stored.Delays {
		// A delay is earlier or later, so it may be below 0. It is read to the millisecond, as seconds such as 2.3 are not exact in binary and would otherwise read back a nanosecond short.
		if at := time.Duration(math.Round(seconds*1000)) * time.Millisecond; at != 0 {
			lib.Delays[path] = at
		}
	}
	for path, lift := range stored.Lifts {
		// A place past the picture's edge, as a hand-edited file may hold, is dropped rather than show the lines off the picture.
		if lift != 0 && lift > -1 && lift < 1 {
			lib.Lifts[path] = lift
		}
	}
	for path, name := range stored.Names {
		if name = strings.TrimSpace(name); name != "" {
			lib.Names[path] = name
		}
	}
	// A lock is kept as it was read, even one Matches cannot check, so a damaged lock keeps its library locked rather than open.
	for dir, lock := range stored.Locks {
		if dir = strings.TrimSpace(dir); dir != "" && lock.Algorithm != "" {
			lib.Locks[filepath.Clean(dir)] = lock
		}
	}
	if err != nil {
		lib.unread = err
		return lib, &DamagedError{Data: data, Err: err}
	}
	return lib, nil
}

// Unread is why Load could not read the file in full while Save does not write it (ErrUnread), nil once it may.
func (l Library) Unread() error { return l.unread }

// AllowSave lets Save write over a file Load could not read in full, once the caller has kept a copy of it.
func (l *Library) AllowSave() { l.unread = nil }

// Upsert puts job on the shelf in place of the job with its ID, or first when there is none. When the job's video is now at another path, as when processing it again downloads it anew, the title given to it in Kando goes with it, unless the new path has one of its own.
func (l *Library) Upsert(job core.Job) {
	for i := range l.Jobs {
		if l.Jobs[i].ID == job.ID {
			if old := l.Jobs[i].VideoPath; old != "" && job.VideoPath != "" && old != job.VideoPath && l.Names[old] != "" && l.Names[job.VideoPath] == "" {
				l.Names[job.VideoPath] = l.Names[old]
				delete(l.Names, old)
			}
			l.Jobs[i] = job
			return
		}
	}
	l.Jobs = append([]core.Job{job}, l.Jobs...)
}

func (l *Library) Find(id string) (core.Job, bool) {
	for _, job := range l.Jobs {
		if job.ID == id {
			return job, true
		}
	}
	return core.Job{}, false
}

// Rename gives the video at path the title name in Kando, trimmed of surrounding space; an empty name drops the title the user gave it, so it shows the one its site or its file gives it again. It reports whether the title changed.
func (l *Library) Rename(path, name string) bool {
	name = strings.TrimSpace(name)
	if path == "" || l.Names[path] == name {
		return false
	}
	if name == "" {
		delete(l.Names, path)
		return true
	}
	if l.Names == nil {
		l.Names = map[string]string{}
	}
	l.Names[path] = name
	return true
}

// MoveVideos keys the resume positions, furthest points, times last played, subtitle delays and places and the titles given in Kando by the new paths of videos that moved on disk: move gives a video's new path for its old one, and reports false for one that did not move. It reports whether any moved.
func (l *Library) MoveVideos(move func(path string) (string, bool)) bool {
	positions := MoveKeys(l.Positions, move)
	furthest := MoveKeys(l.Furthest, move)
	played := MoveKeys(l.Played, move)
	delays := MoveKeys(l.Delays, move)
	lifts := MoveKeys(l.Lifts, move)
	names := MoveKeys(l.Names, move)
	return positions || furthest || played || delays || lifts || names
}

// MoveKeys keys what m holds for each video that moved, by path, under the new path move gives for it. It reports whether any moved.
func MoveKeys[V any](m map[string]V, move func(path string) (string, bool)) bool {
	moved := map[string]V{}
	for path, at := range m {
		if next, ok := move(path); ok && next != path {
			moved[next] = at
			delete(m, path)
		}
	}
	maps.Copy(m, moved)
	return len(moved) > 0
}

// Save writes the library atomically. An empty path keeps the library in memory, which tests use. A library whose file Load could not read in full is not written, and Save says so with ErrUnread.
func (l Library) Save() error {
	if l.path == "" {
		return nil
	}
	if l.unread != nil {
		return fmt.Errorf("%w: %v", ErrUnread, l.unread)
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o700); err != nil {
		return err
	}
	stored := storedLibrary{Jobs: make([]storedJob, len(l.Jobs))}
	for i, job := range l.Jobs {
		stored.Jobs[i] = storeJob(job)
	}
	for path, at := range l.Positions {
		if at > 0 {
			if stored.Positions == nil {
				stored.Positions = map[string]float64{}
			}
			stored.Positions[path] = at.Seconds()
		}
	}
	for path, at := range l.Furthest {
		if at > 0 {
			if stored.Furthest == nil {
				stored.Furthest = map[string]float64{}
			}
			stored.Furthest[path] = at.Seconds()
		}
	}
	for path, at := range l.Played {
		if !at.IsZero() {
			if stored.Played == nil {
				stored.Played = map[string]time.Time{}
			}
			stored.Played[path] = at
		}
	}
	for path, d := range l.Delays {
		if d != 0 {
			if stored.Delays == nil {
				stored.Delays = map[string]float64{}
			}
			stored.Delays[path] = d.Seconds()
		}
	}
	for path, lift := range l.Lifts {
		if lift != 0 {
			if stored.Lifts == nil {
				stored.Lifts = map[string]float32{}
			}
			stored.Lifts[path] = lift
		}
	}
	for path, name := range l.Names {
		if name != "" {
			if stored.Names == nil {
				stored.Names = map[string]string{}
			}
			stored.Names[path] = name
		}
	}
	if len(l.Locks) > 0 {
		stored.Locks = maps.Clone(l.Locks)
	}
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := l.path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, l.path)
}

// storedLibrary is library.json. Positions, Furthest and Delays are in seconds; Played is Library.Played, as RFC 3339 times; Lifts, Names and Locks are Library.Lifts, Library.Names and Library.Locks.
type storedLibrary struct {
	Jobs      []storedJob          `json:"jobs"`
	Positions map[string]float64   `json:"positions,omitempty"`
	Furthest  map[string]float64   `json:"furthest,omitempty"`
	Played    map[string]time.Time `json:"played,omitempty"`
	Delays    map[string]float64   `json:"delays,omitempty"`
	Lifts     map[string]float32   `json:"lifts,omitempty"`
	Names     map[string]string    `json:"names,omitempty"`
	Locks     map[string]Lock      `json:"locks,omitempty"`
}

type storedJob struct {
	ID           string     `json:"id"`
	Source       string     `json:"source"`
	Language     string     `json:"language,omitempty"`
	Stage        core.Stage `json:"stage"`
	Title        string     `json:"title,omitempty"`
	Thumbnail    string     `json:"thumbnail,omitempty"`
	VideoPath    string     `json:"videoPath,omitempty"`
	SubtitlePath string     `json:"subtitlePath,omitempty"`
	LibraryDir   string     `json:"libraryDir,omitempty"`
	CompletedAt  time.Time  `json:"completedAt,omitzero"`
	Error        string     `json:"error,omitempty"`
	// FailKind is why the download failed (core.FailKind), and Cookies the browser whose login the job borrows.
	FailKind core.FailKind `json:"failKind,omitempty"`
	Cookies  string        `json:"cookies,omitempty"`
	// Untranslated is how many lines a completed job's translation left untranslated (core.Job.Untranslated).
	Untranslated int `json:"untranslated,omitempty"`
	// TranslationError is why a completed job translated none of its lines (core.Job.TranslationErr).
	TranslationError string `json:"translationError,omitempty"`
	// Transcribed is whether the job saved its transcript beside its video (core.Job.Transcribed).
	Transcribed bool `json:"transcribed,omitempty"`
	// Proofread is whether the job's transcript has been proofread (core.Job.Proofread), and SameLanguage whether the film needed no translation (core.Job.SameLanguage).
	Proofread    bool `json:"proofread,omitempty"`
	SameLanguage bool `json:"sameLanguage,omitempty"`
	// Redo is the part of a done film's processing a job translating it again does (core.Job.Redo).
	Redo core.Redo `json:"redo,omitempty"`
	// RedoLanguage is the language a job translating every line again translates into (core.Job.RedoLanguage).
	RedoLanguage string `json:"redoLanguage,omitempty"`
	// SpokenLanguage is the language the transcription heard (core.Job.SpokenLanguage).
	SpokenLanguage string `json:"spokenLanguage,omitempty"`
	// SpeechLanguage is the language the user said the job's video is spoken in (core.Job.SpeechLanguage).
	SpeechLanguage string `json:"speechLanguage,omitempty"`
	// FromFolder is whether the job processes a video found in the library's folder that no job held (core.Job.FromFolder).
	FromFolder bool `json:"fromFolder,omitempty"`
	// Quality is the picture the job's download asks for, and SubtitleSource where it takes the film's original lines from (core.Job.Quality, core.Job.SubtitleSource); both are empty in a library written before they were chosen, which the queue then gives the settings' ones.
	Quality        core.Quality        `json:"quality,omitempty"`
	SubtitleSource core.SubtitleSource `json:"subtitleSource,omitempty"`
	// PlatformSubtitles is whether the film's original lines are its site's subtitles, and LocalLines how many of their lines the speech replaced (core.Job.PlatformSubtitles).
	PlatformSubtitles bool `json:"platformSubtitles,omitempty"`
	LocalLines        int  `json:"localLines,omitempty"`
	// Device is the paired phone that sent the job's link (core.Job.Device).
	Device string `json:"device,omitempty"`
	// Series is the series the film is part of (core.Job.Series).
	Series string `json:"series,omitempty"`
}

func storeJob(job core.Job) storedJob {
	stored := storedJob{
		ID: job.ID, Source: job.Source, Language: job.Language, Stage: job.Stage,
		Title: job.Title, Thumbnail: job.Thumbnail,
		VideoPath: job.VideoPath, SubtitlePath: job.SubtitlePath, LibraryDir: job.LibraryDir,
		CompletedAt: job.CompletedAt, FailKind: job.FailKind, Cookies: job.Cookies, Untranslated: job.Untranslated, TranslationError: job.TranslationErr,
		Transcribed: job.Transcribed, Proofread: job.Proofread, SameLanguage: job.SameLanguage, Redo: job.Redo, RedoLanguage: job.RedoLanguage, SpokenLanguage: job.SpokenLanguage, SpeechLanguage: job.SpeechLanguage, FromFolder: job.FromFolder, Device: job.Device,
	}
	stored.Quality, stored.SubtitleSource = job.Quality, job.SubtitleSource
	stored.PlatformSubtitles, stored.LocalLines = job.PlatformSubtitles, job.LocalLines
	stored.Series = job.Series
	switch {
	case errors.Is(job.Err, context.Canceled):
		stored.Error = storedCanceled
	case errors.Is(job.Err, core.ErrLibrarySwitched):
		stored.Error = storedLibrarySwitched
	case job.Err != nil:
		stored.Error = job.Err.Error()
	}
	return stored
}

func (s storedJob) job() core.Job {
	job := core.Job{
		ID: s.ID, Source: s.Source, Language: s.Language, Stage: s.Stage,
		Title: s.Title, Thumbnail: s.Thumbnail,
		VideoPath: s.VideoPath, SubtitlePath: s.SubtitlePath, LibraryDir: s.LibraryDir,
		CompletedAt: s.CompletedAt, FailKind: s.FailKind, Cookies: s.Cookies, Untranslated: s.Untranslated, TranslationErr: s.TranslationError,
		Transcribed: s.Transcribed, Proofread: s.Proofread, SameLanguage: s.SameLanguage, Redo: s.Redo, RedoLanguage: s.RedoLanguage, SpokenLanguage: s.SpokenLanguage, SpeechLanguage: s.SpeechLanguage, FromFolder: s.FromFolder, Device: s.Device,
	}
	job.Quality, job.SubtitleSource = s.Quality, s.SubtitleSource
	job.PlatformSubtitles, job.LocalLines = s.PlatformSubtitles, s.LocalLines
	job.Series = s.Series
	switch s.Error {
	case "":
	case storedCanceled:
		job.Err = context.Canceled
	case storedLibrarySwitched:
		job.Err = core.ErrLibrarySwitched
	default:
		job.Err = errors.New(s.Error)
	}
	if job.Stage != "" && !job.Stage.Done() {
		job.Stage = core.StageQueued
		job.Err = nil
		job.FailKind = ""
	}
	return job
}
