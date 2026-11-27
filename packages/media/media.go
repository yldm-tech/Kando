package media

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kando/packages/core"
)

var ErrNoSpeech = errors.New("no speech found in the video")

// ErrNoAudio is why transcribing failed for a video without a sound track, as many screen recordings posted on X are: there is nothing to hear, so retrying cannot help.
var ErrNoAudio = errors.New("the video has no sound")

// ErrNoTranscript is why a job translating a done film again failed when the transcript its earlier run exported beside the video is gone or unreadable.
var ErrNoTranscript = errors.New("the film's transcript is no longer beside its video")

// ErrSubtitlesChanged is why a job translating only the missing lines of a done film again failed when the film's bilingual subtitles no longer match its transcript cue for cue, as after they were edited by hand: which lines are missing cannot be told, and translating every line would overwrite the edits.
var ErrSubtitlesChanged = errors.New("the film's bilingual subtitles no longer match its transcript")

// TranslateChecker is a Pipeline that checks what translating needs apart from Check: a job translating a done film again (core.Job.Redo) checks only that, so it does not fail for a download or speech tool it does not run, and a job making a film for the first time checks it before translating, so one without a translator still completes with the original lines (core.Job.TranslationErr).
type TranslateChecker interface {
	CheckTranslate() error
}

// hearer is a Pipeline that says how its Hear hears the speech of a video under ctx, as Tools does: by the speech model and in the language it is told, "" for one it detects. A redo goes on with what it heard before only when it hears by the same model (making.generation).
type hearer interface {
	hearing(ctx context.Context) (model, language string)
}

// speechHearer is a Pipeline that says whether its Hear hears only the speech of a video, as Tools does where whisper-cli can find it (Tools.hearSpeech). Cells heard that way are recorded as heard so (speechHearing), so a film begun the other way is heard again.
type speechHearer interface {
	hearsSpeech(ctx context.Context) bool
}

// speechHearing follows the language in the record of a cell heard by its speech alone (track.Making.Hearing), as "ja+speech".
const speechHearing = "+speech"

// Pipeline is the cross-platform boundary for media processing, which Make makes a film's subtitles with. Implementations keep OS-specific details out of the core package.
type Pipeline interface {
	// Check reports missing tools or settings before a job spends time downloading.
	Check() error
	Download(ctx context.Context, source string) (string, error)
	// Hear hears part of a video at a time, handing each line on as it is heard (Hearer).
	Hearer
	// Length is how long the video at videoPath is, 0 when it cannot be told.
	Length(ctx context.Context, videoPath string) time.Duration
	// Translate returns the segments translated into language. When it left some lines untranslated, with an empty Text, it returns every segment with an *UntranslatedError, and the job completes with them.
	Translate(ctx context.Context, segments []core.Segment, language string) ([]core.Segment, error)
}

// downloadFailKind is why job's download failed, as err says (ClassifyDownload): a site that still wants a login while the browser's is borrowed has let that login run out. A job that borrows none (core.CookiesNone, as the Add sheet's 未登录 chose) only wants one. A failure yt-dlp could not say better, or took for an outdated extractor, after it found no JavaScript runtime (ErrNoJSRuntime) wants one, which no update of yt-dlp gives.
func downloadFailKind(job core.Job, err error) core.FailKind {
	kind := ClassifyDownload(err.Error())
	if (kind == "" || kind == core.FailOutdated) && strings.Contains(err.Error(), ErrNoJSRuntime.Error()) {
		return core.FailNoJSRuntime
	}
	if job.Browser() != "" && (kind == core.FailLogin || kind == core.FailAge) {
		kind = core.FailLoginExpired
	}
	return kind
}

// filmTitle is what a job's video is called, for the proofreading and the translation to judge its words by: the title the site gave it, or a local video's file name, or, for one the library keeps in a folder of its own as video.<ext> (GatherVideos), the folder's name.
func filmTitle(job core.Job) string {
	if title := strings.TrimSpace(job.Title); title != "" {
		return title
	}
	if job.VideoPath == "" {
		return ""
	}
	dir, name := filepath.Split(job.VideoPath)
	return strings.TrimSuffix(displayName(filepath.Clean(dir), name), filepath.Ext(name))
}

// Downloaded reports whether a link's job already downloaded its video to path in an earlier run: the file is there, holds something, and no partial download lies beside it, which would mean it never merged and Download has to fetch it again.
func Downloaded(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return false
	}
	return !holdsDownloadLeftovers(filepath.Dir(path))
}
