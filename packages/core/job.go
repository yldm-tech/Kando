package core

import (
	"errors"
	"net/url"
	"strings"
)

var ErrEmptySource = errors.New("video source is empty")

type Stage string

const (
	StageQueued       Stage = "queued"
	StageDownloading  Stage = "downloading"
	StageTranscribing Stage = "transcribing"
	StageTranslating  Stage = "translating"
	StageComplete     Stage = "complete"
	StageFailed       Stage = "failed"
)

type Job struct {
	Source string
	Stage  Stage
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
