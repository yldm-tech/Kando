package shelf

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"
	"time"

	"kando/packages/core"
)

// The error of a stopped job is stored as one of these words so it reads back as the same error; any other error is stored as its text.
const (
	storedCanceled        = "canceled"
	storedLibrarySwitched = "library-switched"
)

// Library is the videos this machine has processed, newest first, and where each video stopped playing.
type Library struct {
	path string
	Jobs []core.Job
	// Positions is where the video at each path stopped playing last time, so it can resume there.
	Positions map[string]time.Duration
}

func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "Kando", "library.json")
}

// Load reads the library at path. A missing file is an empty library. A corrupt file is empty too, and the error says why. A job the app was still working on when it quit is read back as queued, so the queue runs it again from the start. A file written before positions were kept, a bare list of jobs, still reads.
func Load(path string) (Library, error) {
	lib := Library{path: path, Positions: map[string]time.Duration{}}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return lib, nil
		}
		return lib, err
	}
	var stored storedLibrary
	if trimmed := bytes.TrimSpace(data); len(trimmed) > 0 && trimmed[0] == '[' {
		err = json.Unmarshal(data, &stored.Jobs)
	} else {
		err = json.Unmarshal(data, &stored)
	}
	if err != nil {
		return lib, err
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
	return lib, nil
}

func (l *Library) Upsert(job core.Job) {
	for i := range l.Jobs {
		if l.Jobs[i].ID == job.ID {
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

// Save writes the library atomically. An empty path keeps the library in memory, which tests use.
func (l Library) Save() error {
	if l.path == "" {
		return nil
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

// storedLibrary is library.json. Positions are in seconds.
type storedLibrary struct {
	Jobs      []storedJob        `json:"jobs"`
	Positions map[string]float64 `json:"positions,omitempty"`
}

type storedJob struct {
	ID           string     `json:"id"`
	Source       string     `json:"source"`
	Language     string     `json:"language,omitempty"`
	Stage        core.Stage `json:"stage"`
	VideoPath    string     `json:"videoPath,omitempty"`
	SubtitlePath string     `json:"subtitlePath,omitempty"`
	LibraryDir   string     `json:"libraryDir,omitempty"`
	CompletedAt  time.Time  `json:"completedAt,omitzero"`
	Error        string     `json:"error,omitempty"`
}

func storeJob(job core.Job) storedJob {
	stored := storedJob{
		ID: job.ID, Source: job.Source, Language: job.Language, Stage: job.Stage,
		VideoPath: job.VideoPath, SubtitlePath: job.SubtitlePath, LibraryDir: job.LibraryDir,
		CompletedAt: job.CompletedAt,
	}
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
		VideoPath: s.VideoPath, SubtitlePath: s.SubtitlePath, LibraryDir: s.LibraryDir,
		CompletedAt: s.CompletedAt,
	}
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
	}
	return job
}
