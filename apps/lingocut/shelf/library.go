package shelf

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"lingocut/packages/core"
)

const storedCanceled = "canceled"

// Library is the videos this machine has processed, newest first.
type Library struct {
	path string
	Jobs []core.Job
}

func DefaultPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "LingoCut", "library.json")
}

// Load reads the library at path. A missing file is an empty library. A corrupt file is empty too, and the error says why.
func Load(path string) (Library, error) {
	lib := Library{path: path}
	data, err := os.ReadFile(path)
	if err != nil {
		if errors.Is(err, os.ErrNotExist) {
			return lib, nil
		}
		return lib, err
	}
	var stored []storedJob
	if err := json.Unmarshal(data, &stored); err != nil {
		return lib, err
	}
	lib.Jobs = make([]core.Job, len(stored))
	for i, s := range stored {
		lib.Jobs[i] = s.job()
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
	stored := make([]storedJob, len(l.Jobs))
	for i, job := range l.Jobs {
		stored[i] = storeJob(job)
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

type storedJob struct {
	ID           string     `json:"id"`
	Source       string     `json:"source"`
	Language     string     `json:"language,omitempty"`
	Stage        core.Stage `json:"stage"`
	VideoPath    string     `json:"videoPath,omitempty"`
	SubtitlePath string     `json:"subtitlePath,omitempty"`
	LibraryDir   string     `json:"libraryDir,omitempty"`
	Error        string     `json:"error,omitempty"`
}

func storeJob(job core.Job) storedJob {
	stored := storedJob{
		ID: job.ID, Source: job.Source, Language: job.Language, Stage: job.Stage,
		VideoPath: job.VideoPath, SubtitlePath: job.SubtitlePath, LibraryDir: job.LibraryDir,
	}
	if errors.Is(job.Err, context.Canceled) {
		stored.Error = storedCanceled
	} else if job.Err != nil {
		stored.Error = job.Err.Error()
	}
	return stored
}

func (s storedJob) job() core.Job {
	job := core.Job{
		ID: s.ID, Source: s.Source, Language: s.Language, Stage: s.Stage,
		VideoPath: s.VideoPath, SubtitlePath: s.SubtitlePath, LibraryDir: s.LibraryDir,
	}
	switch s.Error {
	case "":
	case storedCanceled:
		job.Err = context.Canceled
	default:
		job.Err = errors.New(s.Error)
	}
	return job
}
