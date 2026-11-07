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

// ErrLibrarySwitched is the error of a job that was stopped because another library was opened while it ran.
var ErrLibrarySwitched = errors.New("stopped because another library was opened")

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

// Progress is how far a job has gone in the stage it is in, as the tool doing the work reports it. The zero value is a stage that has reported nothing yet.
type Progress struct {
	// Fraction is how much of the stage is done, from 0 to 1. Known is false until the tool has said; a stage can report the other fields before it knows the fraction.
	Fraction float64
	Known    bool
	// Speed is bytes a second and ETA the time yt-dlp expects the download still needs; both are 0 when it has not said.
	Speed float64
	ETA   time.Duration
	// Batch is the translation batch being sent, from 1, of Batches, each of up to BatchSize lines.
	Batch, Batches, BatchSize int
	// RetryIn is how long the translation waits before trying a rate-limited or failed request again, as the API's Retry-After asked or the backoff chose; Attempt is the retry about to be made, of Attempts. RetryIn is 0 while no retry is waiting.
	RetryIn           time.Duration
	Attempt, Attempts int
}

// Fraction reports f as a known fraction, clamped to [0, 1].
func Fraction(f float64) Progress {
	return Progress{Fraction: min(max(f, 0), 1), Known: true}
}

type Job struct {
	ID       string
	Source   string
	Language string
	Stage    Stage
	// Progress is how far the job has gone in Stage. It is not saved: a job read back after a restart starts its stage again.
	Progress Progress
	// Title and Thumbnail are the video's title and the address of its thumbnail as the site gives them, known once the download has looked the video up; both stay empty for a local video and for a link whose site said nothing.
	Title        string
	Thumbnail    string
	VideoPath    string
	SubtitlePath string
	// LibraryDir is the library that owns this job. Empty means it was saved before libraries were separate.
	LibraryDir string
	// CompletedAt is when the job reached StageComplete; zero until then.
	CompletedAt time.Time
	Err         error
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
