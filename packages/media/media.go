package media

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
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

// TranslateChecker is a Pipeline that can check only what translating needs, which is all a job translating a done film again (core.Job.Redo) uses, so such a job does not fail for a download or speech tool it does not run.
type TranslateChecker interface {
	CheckTranslate() error
}

// Pipeline is the cross-platform boundary for media processing. Implementations keep OS-specific details out of the core package.
type Pipeline interface {
	// Check reports missing tools or settings before a job spends time downloading.
	Check() error
	Download(ctx context.Context, source string) (string, error)
	Transcribe(ctx context.Context, videoPath string) ([]core.Segment, error)
	// Translate returns the segments translated into language. When it left some lines untranslated, with an empty Text, it returns every segment with an *UntranslatedError, and the job completes with them.
	Translate(ctx context.Context, segments []core.Segment, language string) ([]core.Segment, error)
	// Export writes the subtitles next to the video and returns the bilingual subtitle path.
	Export(ctx context.Context, videoPath string, original, translated []core.Segment) (string, error)
}

// Run drives job through every stage of p. report receives the job each time its stage changes, ending with StageComplete or StageFailed, and each time a stage reports through the context's Trace how far it has gone (job.Progress) or what the video is called (job.Title, job.Thumbnail); the final job is returned. Progress starts again at each stage and is cleared once the job is done; Untranslated counts the lines this run's translation left untranslated. A local video (a file:// source) is not downloaded, so its job goes from StageQueued straight to StageTranscribing; p.Download still runs to check the file and give its path.
//
// A job run again resumes after the stages an earlier run finished. A link whose video is still at job.VideoPath is not downloaded again, so the retry works offline or after the site took the video down. Once transcribed, the speech is saved beside the video before the translation starts and job.Transcribed set; a job that comes back with it set reads that transcript back instead of calling p.Transcribe, and goes straight to StageTranslating. A job whose Transcribed is false, as a done film processed again, or whose video had to be downloaded again, is transcribed anew.
//
// A job whose Redo is set translates a done film again from the subtitles its earlier run exported beside job.VideoPath (exportSubtitles): it neither downloads nor transcribes, so it goes from StageQueued straight to StageTranslating, and a pipeline that is a TranslateChecker checks only what translating needs. RedoTranslation translates every line of the transcript again; RedoMissing sends p.Translate only the lines the bilingual subtitles left untranslated and keeps the rest. A film whose transcript is gone fails with ErrNoTranscript, and one whose bilingual subtitles no longer match it, as ones edited by hand, fails a RedoMissing with ErrSubtitlesChanged rather than translating every line over them. Until the new subtitles are exported the job keeps the film's Language, Untranslated and CompletedAt, so a redo that fails still describes the subtitles on disk; once it is complete its Language is language and Redo and RedoLanguage are cleared.
func Run(ctx context.Context, p Pipeline, job core.Job, language string, report func(core.Job)) core.Job {
	enter := func(stage core.Stage) {
		job.Stage = stage
		job.Progress = core.Progress{}
		report(job)
	}
	ctx = WithTrace(ctx, &Trace{
		Progress: func(progress core.Progress) {
			job.Progress = progress
			report(job)
		},
		Info: func(info Info) {
			if info.Title == "" || (info.Title == job.Title && info.Thumbnail == job.Thumbnail) {
				return
			}
			job.Title, job.Thumbnail = info.Title, info.Thumbnail
			report(job)
		},
	})
	if job.Redo == "" {
		job.Untranslated = 0
	}
	fail := func(step string, err error) core.Job {
		job.Err = fmt.Errorf("%s: %w", step, err)
		enter(core.StageFailed)
		return job
	}

	check := p.Check
	if tc, ok := p.(TranslateChecker); ok && job.Redo != "" {
		check = tc.CheckTranslate
	}
	if err := check(); err != nil {
		return fail("check", err)
	}

	var original, earlier []core.Segment
	if job.Redo == "" {
		_, local := localVideoPath(job.Source)
		video := job.VideoPath
		if local || !downloaded(video) {
			if !local {
				enter(core.StageDownloading)
			}
			var err error
			video, err = p.Download(ctx, job.Source)
			if err != nil {
				job.FailKind = ClassifyDownload(err.Error())
				// A site that still wants a login while the browser's is borrowed has let that login run out.
				if job.Cookies != "" && (job.FailKind == core.FailLogin || job.FailKind == core.FailAge) {
					job.FailKind = core.FailLoginExpired
				}
				return fail("download", err)
			}
			// A link downloaded again lands on the same path as before, so the transcript beside it may have been made from the file this download replaced, such as a silent track that never merged.
			if !local {
				job.Transcribed = false
			}
		}
		// A transcript saved beside another file is not this video's.
		if video != job.VideoPath {
			job.Transcribed = false
		}
		job.VideoPath = video

		if job.Transcribed {
			// A transcript that is gone or unreadable is made again, as if it had never been saved.
			original, _ = loadTranscript(video)
		}
		if len(original) == 0 {
			job.Transcribed = false
			enter(core.StageTranscribing)
			var err error
			original, err = p.Transcribe(ctx, video)
			if err != nil {
				return fail("transcribe", err)
			}
			if len(original) == 0 {
				return fail("transcribe", ErrNoSpeech)
			}
			// The transcript is kept before the translation starts, so a translation that fails does not cost the minutes of transcribing again. A transcript that cannot be saved only means a retry transcribes again; Export, which writes beside the video too, says why the folder cannot be written.
			job.Transcribed = saveTranscript(video, original) == nil
		}
	} else {
		var err error
		original, earlier, err = readExported(job.VideoPath, job.Redo == core.RedoMissing)
		if err != nil {
			return fail("translate", err)
		}
	}

	enter(core.StageTranslating)
	var translated []core.Segment
	var untranslated int
	var err error
	if job.Redo == core.RedoMissing {
		translated, untranslated, err = translateMissing(ctx, p, original, earlier, language)
	} else {
		translated, err = p.Translate(ctx, original, language)
		// Lines the translation could not do after trying them again stay untranslated; the rest of the film is not lost for them.
		var partial *UntranslatedError
		if errors.As(err, &partial) && len(translated) == len(original) {
			untranslated, err = partial.Lines, nil
		}
	}
	if err != nil {
		return fail("translate", err)
	}

	enter(core.StageExporting)
	subtitles, err := p.Export(ctx, job.VideoPath, original, translated)
	if err != nil {
		return fail("export", err)
	}
	job.SubtitlePath = subtitles
	job.CompletedAt = time.Now()
	job.Language, job.Untranslated = language, untranslated
	job.Redo, job.RedoLanguage = "", ""

	enter(core.StageComplete)
	return job
}

// downloaded reports whether a link's job already downloaded its video to path in an earlier run: the file is there, holds something, and no partial download lies beside it, which would mean it never merged and Download has to fetch it again.
func downloaded(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	if err != nil || !info.Mode().IsRegular() || info.Size() == 0 {
		return false
	}
	return !holdsDownloadLeftovers(filepath.Dir(path))
}

// translateMissing translates the lines of original whose translation in earlier is empty and keeps the others, returning every line's translation and how many are still untranslated. Only the missing lines go to p.Translate, so a film whose translation failed for a few batches does not pay for the rest again.
func translateMissing(ctx context.Context, p Pipeline, original, earlier []core.Segment, language string) ([]core.Segment, int, error) {
	out := slices.Clone(earlier)
	var missing []core.Segment
	var at []int
	for i, s := range earlier {
		if strings.TrimSpace(s.Text) == "" {
			missing = append(missing, original[i])
			at = append(at, i)
		}
	}
	if len(missing) == 0 {
		return out, 0, nil
	}
	got, err := p.Translate(ctx, missing, language)
	left := 0
	var partial *UntranslatedError
	if errors.As(err, &partial) && len(got) == len(missing) {
		left, err = partial.Lines, nil
	}
	if err != nil {
		return nil, 0, err
	}
	if len(got) != len(missing) {
		return nil, 0, fmt.Errorf("%d translated lines for %d untranslated lines", len(got), len(missing))
	}
	for k, i := range at {
		out[i].Text = got[k].Text
	}
	return out, left, nil
}
