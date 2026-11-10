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

// ErrLibrarySwitched is the error of a job that was stopped because another library was opened while it ran. Kando no longer stops a job then; the error stays so library.json rows written by versions that did read back as the same error.
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
	// Unanswered is true while the translation waits to try again a request that got no usable answer, as one that timed out, lost its connection or came back cut off; false while it waits out a rate limit or a server error.
	Unanswered bool
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
	// FailKind is why the download failed, when yt-dlp said why; empty otherwise.
	FailKind FailKind
	// Untranslated is how many lines of a completed job's subtitles the translation left untranslated after trying them again; 0 when every line was translated.
	Untranslated int
	// Transcribed is true once this job's transcription has saved the speech it heard beside VideoPath, so running the job again, as a retry after a failed translation or after the app quit in the middle, reads it back instead of transcribing the video again. Processing a done film again clears it, so its speech is transcribed anew.
	Transcribed bool
	// Cookies is the browser whose login the download borrows (yt-dlp's --cookies-from-browser name, such as chrome); empty borrows none.
	Cookies string
	// Redo is the part of a done film's processing the job does again instead of every stage: RedoTranslation or RedoMissing. Empty runs every stage. It stays set while the job waits, runs or has failed, so a retry or a restart does the same part again, and is cleared once the job is complete.
	Redo Redo
}

// Redo is what a job made to translate a done film again translates. Both read the subtitles the film's job exported beside its video and neither downloads nor transcribes it again.
type Redo string

const (
	// RedoTranslation translates every line of the film's transcript again.
	RedoTranslation Redo = "translation"
	// RedoMissing translates only the lines the film's subtitles left untranslated and keeps the others as they are.
	RedoMissing Redo = "missing"
)

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

// FailKind is why a download failed, as yt-dlp's errors say, so the card can offer what fixes it. The empty kind is a failure no remedy is known for.
type FailKind string

const (
	// FailOutdated is an extractor that no longer understands the site: yt-dlp needs an update.
	FailOutdated FailKind = "ytdlp"
	// FailLoginExpired is a video that wants a login although the browser's login was used: it has run out.
	FailLoginExpired FailKind = "cookie"
	FailMembers      FailKind = "members"
	FailAge          FailKind = "age"
	FailRegion       FailKind = "region"
	FailPrivate      FailKind = "private"
	FailDRM          FailKind = "drm"
	FailUnsupported  FailKind = "unsupported"
	FailLogin        FailKind = "login"
	FailRateLimit    FailKind = "ratelimit"
	FailNetwork      FailKind = "network"
	// FailHost is a site whose name did not resolve: the link names a host that does not exist, or the network is down.
	FailHost FailKind = "host"
	// FailCookies is a browser whose login yt-dlp could not read: the system did not allow it, or the browser has no data here.
	FailCookies FailKind = "cookies"
	// FailBadLink is a link the site says is cut short or wrong, as a YouTube watch link whose video id lost its last characters: no retry can work, only the whole link.
	FailBadLink FailKind = "badlink"
)
