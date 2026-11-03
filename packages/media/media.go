package media

import (
	"context"
	"errors"
	"fmt"

	"lingocut/packages/core"
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

// Run drives job through every stage of p. report receives the job each time its stage changes, ending with StageComplete or StageFailed, and the final job is returned.
func Run(ctx context.Context, p Pipeline, job core.Job, language string, report func(core.Job)) core.Job {
	enter := func(stage core.Stage) {
		job.Stage = stage
		report(job)
	}
	fail := func(step string, err error) core.Job {
		job.Err = fmt.Errorf("%s: %w", step, err)
		enter(core.StageFailed)
		return job
	}

	if err := p.Check(); err != nil {
		return fail("check", err)
	}

	enter(core.StageDownloading)
	video, err := p.Download(ctx, job.Source)
	if err != nil {
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

	enter(core.StageComplete)
	return job
}
