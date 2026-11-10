package media

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"kando/packages/core"
)

var ErrNoSpeech = errors.New("no speech found in the video")

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
// A job run again resumes after the stages an earlier run finished. A link whose video is still at job.VideoPath is not downloaded again, so the retry works offline or after the site took the video down. Once transcribed, the speech is saved beside the video before the translation starts and job.Transcribed set; a job that comes back with it set reads that transcript back instead of calling p.Transcribe, and goes straight to StageTranslating. A job whose Transcribed is false, as a done film processed again, is transcribed anew.
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
	job.Untranslated = 0
	fail := func(step string, err error) core.Job {
		job.Err = fmt.Errorf("%s: %w", step, err)
		enter(core.StageFailed)
		return job
	}

	if err := p.Check(); err != nil {
		return fail("check", err)
	}

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
	}
	// A transcript saved beside another file is not this video's.
	if video != job.VideoPath {
		job.Transcribed = false
	}
	job.VideoPath = video

	var original []core.Segment
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

	enter(core.StageTranslating)
	translated, err := p.Translate(ctx, original, language)
	// Lines the translation could not do after trying them again stay untranslated; the rest of the film is not lost for them.
	var partial *UntranslatedError
	if errors.As(err, &partial) && len(translated) == len(original) {
		job.Untranslated, err = partial.Lines, nil
	}
	if err != nil {
		return fail("translate", err)
	}

	enter(core.StageExporting)
	subtitles, err := p.Export(ctx, video, original, translated)
	if err != nil {
		return fail("export", err)
	}
	job.SubtitlePath = subtitles
	job.CompletedAt = time.Now()

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
