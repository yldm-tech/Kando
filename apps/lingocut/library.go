package main

import (
	"context"
	"encoding/json"
	"errors"
	"os"
	"path/filepath"

	"lingocut/packages/core"
)

const storedCanceled = "canceled"

// library is the videos this machine has processed, newest first.
type library struct {
	path string
	jobs []core.Job
}

func defaultLibraryPath() string {
	dir, err := os.UserConfigDir()
	if err != nil {
		dir = os.TempDir()
	}
	return filepath.Join(dir, "LingoCut", "library.json")
}

// loadLibrary reads the library at path. A missing file is an empty library. A corrupt file is empty too, and the error says why.
func loadLibrary(path string) (library, error) {
	lib := library{path: path}
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
	lib.jobs = make([]core.Job, len(stored))
	for i, s := range stored {
		lib.jobs[i] = s.job()
	}
	return lib, nil
}

func (l *library) upsert(job core.Job) {
	for i := range l.jobs {
		if l.jobs[i].ID == job.ID {
			l.jobs[i] = job
			return
		}
	}
	l.jobs = append([]core.Job{job}, l.jobs...)
}

func (l *library) find(id string) (core.Job, bool) {
	for _, job := range l.jobs {
		if job.ID == id {
			return job, true
		}
	}
	return core.Job{}, false
}

// save writes the library atomically. An empty path keeps the library in memory, which tests use.
func (l library) save() error {
	if l.path == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(l.path), 0o700); err != nil {
		return err
	}
	stored := make([]storedJob, len(l.jobs))
	for i, job := range l.jobs {
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
