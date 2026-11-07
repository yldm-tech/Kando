package media

import (
	"context"
	"errors"
	"fmt"
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
	Translate(ctx context.Context, segments []core.Segment, language string) ([]core.Segment, error)
	// Export writes the subtitles next to the video and returns the bilingual subtitle path.
	Export(ctx context.Context, videoPath string, original, translated []core.Segment) (string, error)
}

// Run drives job through every stage of p. report receives the job each time its stage changes, ending with StageComplete or StageFailed, and each time a stage reports through the context's Trace how far it has gone (job.Progress) or what the video is called (job.Title, job.Thumbnail); the final job is returned. Progress starts again at each stage and is cleared once the job is done. A local video (a file:// source) is not downloaded, so its job goes from StageQueued straight to StageTranscribing; p.Download still runs to check the file and give its path.
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
	fail := func(step string, err error) core.Job {
		job.Err = fmt.Errorf("%s: %w", step, err)
		enter(core.StageFailed)
		return job
	}

	if err := p.Check(); err != nil {
		return fail("check", err)
	}

	if _, local := localVideoPath(job.Source); !local {
		enter(core.StageDownloading)
	}
	video, err := p.Download(ctx, job.Source)
	if err != nil {
		job.FailKind = ClassifyDownload(err.Error())
		// A site that still wants a login while the browser's is borrowed has let that login run out.
		if job.Cookies != "" && (job.FailKind == core.FailLogin || job.FailKind == core.FailAge) {
			job.FailKind = core.FailLoginExpired
		}
		return fail("download", err)
	}
	job.VideoPath = video

	enter(core.StageTranscribing)
	original, err := p.Transcribe(ctx, video)
	if err != nil {
		return fail("transcribe", err)
	}
	if len(original) == 0 {
		return fail("transcribe", ErrNoSpeech)
	}

	enter(core.StageTranslating)
	translated, err := p.Translate(ctx, original, language)
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
