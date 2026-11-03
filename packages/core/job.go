package core

import (
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
	Source       string
	Stage        Stage
	VideoPath    string
	SubtitlePath string
	Err          error
}

func NewJob(source string) (Job, error) {
	source = strings.TrimSpace(source)
	if source == "" {
		return Job{}, ErrEmptySource
	}

	parsed, err := url.ParseRequestURI(source)
	if err != nil || parsed.Scheme == "" || parsed.Host == "" {
		return Job{}, errors.New("video source must be a valid URL")
	}

	return Job{Source: source, Stage: StageQueued}, nil
}
