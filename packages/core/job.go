package core

import (
	"crypto/rand"
	"encoding/hex"
	"errors"
	"net/url"
	"strings"
	"time"
)

var ErrEmptySource = errors.New("video source is empty")

type Stage string

const (
	StageQueued       Stage = "queued"
	StageDownloading  Stage = "downloading"
	StageTranscribing Stage = "transcribing"
	StageTranslating  Stage = "translating"
	StageExporting    Stage = "exporting"
	StageComplete     Stage = "complete"
	StageFailed       Stage = "failed"
)

// Done reports whether a job in this stage will not change again.
func (s Stage) Done() bool {
	return s == StageComplete || s == StageFailed
}

// Segment is one timed line of speech or its translation.
type Segment struct {
	Start time.Duration
	End   time.Duration
	Text  string
}

type Job struct {
	ID           string
	Source       string
	Language     string
	Stage        Stage
	VideoPath    string
	SubtitlePath string
	// LibraryDir is the library that owns this job. Empty means it was saved before libraries were separate.
	LibraryDir string
	Err        error
}

func NewJob(source string) (Job, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return Job{}, ErrEmptySource
	}

	parsed, err := url.ParseRequestURI(source)
	if err != nil || parsed.Scheme == "" {
		return Job{}, errors.New("video source must be a valid URL")
	}
	if parsed.Scheme == "file" {
		if parsed.Path == "" {
			return Job{}, errors.New("video source must be a valid URL")
		}
	} else if parsed.Host == "" {
		return Job{}, errors.New("video source must be a valid URL")
	}

	return Job{ID: newID(), Source: source, Stage: StageQueued}, nil
}

func newID() string {
	var b [8]byte
	if _, err := rand.Read(b[:]); err != nil {
		return hex.EncodeToString([]byte(time.Now().Format(time.RFC3339Nano)))
	}
	return hex.EncodeToString(b[:])
}
