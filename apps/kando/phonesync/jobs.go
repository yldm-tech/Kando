package phonesync

import (
	"context"
	"errors"
	"strings"

	"kando/packages/core"
	"kando/packages/media"
	psync "kando/packages/sync"
)

// JobFacts is what the app says of a job beside the job itself: the name its card shows, the protocol's ID of the library it saves into, and for a failed job the reason its card gives, in English, and whether a setting on the desktop fixes it (a card that offers 打开设置).
type JobFacts struct {
	Title, Library string
	Reason         string
	Setup          bool
}

// JobOf is job as protocol v2 tells it (psync.Job): its state, each step's, and for one that failed, why, typed by what yt-dlp said (core.FailKind) or the step that failed.
func JobOf(job core.Job, facts JobFacts) psync.Job {
	out := psync.Job{ID: job.ID, Link: job.Source, Title: facts.Title, Library: facts.Library, Device: job.Device}
	if strings.HasPrefix(job.Source, "file:") {
		out.Link = ""
	}
	canceled := errors.Is(job.Err, context.Canceled)
	// at is the index of the step the job is at: running, failed or stopped in; -1 before the first, len(steps) past the last.
	at := -1
	switch job.Stage {
	case core.StageQueued:
		out.State = psync.JobWaiting
	case core.StageDownloading:
		out.State, at = psync.JobRunning, 0
	case core.StageTranscribing:
		out.State, at = psync.JobRunning, 1
	case core.StageTranslating, core.StageExporting:
		out.State, at = psync.JobRunning, 2
	case core.StageComplete:
		out.State, at = psync.JobDone, 3
		// A film that keeps no video (core.Job.SubtitlesOnly) is served too, by its subtitles.
		if job.VideoPath != "" {
			out.Film = job.ID
		}
	case core.StageFailed:
		out.State, at = psync.JobFailed, failedStep(job)
		if canceled {
			out.State = psync.JobCanceled
		}
	}
	// A job a phone paused waits held until it is let go on (core.Job.PausedBy), and so does one paused while it ran, which stops where it is.
	if job.PausedBy != "" && (out.State == psync.JobWaiting || out.State == psync.JobRunning) {
		out.State = psync.JobPaused
	}
	names := []string{psync.StepDownload, psync.StepTranscribe, psync.StepTranslate}
	// A job downloading the video of a film that keeps none (core.RedoVideo) neither transcribes nor translates it.
	video := job.Redo == core.RedoVideo
	skipped := []bool{
		strings.HasPrefix(job.Source, "file:") || job.Redo.TranslatesOnly(),
		job.Redo.TranslatesOnly() || video,
		job.Stage == core.StageComplete && job.SameLanguage || video,
	}
	// A step that failed for a reason found before it began, as a tool missing (the pipeline's check), is the first the job needs.
	if job.Stage == core.StageFailed {
		for at < 2 && at >= 0 && skipped[at] {
			at++
		}
	}
	for i, name := range names {
		step := psync.Step{Name: name, State: psync.StepWaiting}
		switch {
		case skipped[i]:
			step.State = psync.StepSkipped
		case i < at:
			step.State, step.Progress = psync.StepDone, 1
		case i == at && out.State == psync.JobRunning:
			step.State, step.Progress = psync.StepRunning, -1
			if job.Progress.Known {
				step.Progress = job.Progress.Fraction
			}
		case i == at && out.State == psync.JobFailed:
			step.State = psync.StepFailed
		}
		out.Steps = append(out.Steps, step)
	}
	// A done film that translated none of its lines failed its translation, though the film is done: its subtitles are the original lines alone.
	if job.Stage == core.StageComplete && job.TranslationErr != "" {
		out.Steps[2] = psync.Step{Name: psync.StepTranslate, State: psync.StepFailed}
	}
	if out.State == psync.JobFailed {
		out.Failure = failureOf(job, names[min(max(at, 0), 2)], facts)
	}
	return out
}

// failedStep is the index of the step job failed in, as media.Make names it (media.StageError): the check of the tools before any step counts as the first, and the export as the translation's.
func failedStep(job core.Job) int {
	var failed *media.StageError
	if !errors.As(job.Err, &failed) {
		return 0
	}
	switch failed.Stage {
	case media.StageTranscribe:
		return 1
	case media.StageTranslate, media.StageExport:
		return 2
	}
	return 0
}

// failKinds are the failure codes of what yt-dlp said (core.FailKind).
var failKinds = map[core.FailKind]string{
	core.FailOutdated:     psync.FailOutdated,
	core.FailLoginExpired: psync.FailLoginExpired,
	core.FailMembers:      psync.FailMembers,
	core.FailAge:          psync.FailAge,
	core.FailRegion:       psync.FailRegion,
	core.FailPrivate:      psync.FailPrivate,
	core.FailDRM:          psync.FailDRM,
	core.FailUnsupported:  psync.FailUnsupported,
	core.FailLogin:        psync.FailLogin,
	core.FailRateLimit:    psync.FailRateLimited,
	core.FailNetwork:      psync.FailNetwork,
	core.FailHost:         psync.FailHost,
	core.FailCookies:      psync.FailCookies,
	core.FailBadLink:      psync.FailBadLink,
	core.FailHiddenVideo:  psync.FailHiddenVideo,
}

// failureOf is why job failed in step.
func failureOf(job core.Job, step string, facts JobFacts) *psync.Failure {
	f := &psync.Failure{Step: step, Code: psync.FailOther, Message: facts.Reason}
	if f.Message == "" && job.Err != nil {
		f.Message = job.Err.Error()
	}
	code, known := failKinds[job.FailKind]
	switch {
	case known:
		f.Code = code
	case facts.Setup:
		f.Code = psync.FailDesktopSetup
	case job.Err != nil && strings.Contains(job.Err.Error(), media.ErrNoSpeech.Error()):
		f.Code = psync.FailNoSpeech
	case step == psync.StepTranslate:
		f.Code = psync.FailTranslation
	}
	switch f.Code {
	case psync.FailOutdated:
		f.Remedy = psync.RemedyUpdateYTDLP
	case psync.FailNetwork, psync.FailRateLimited, psync.FailHost, psync.FailOther, psync.FailTranslation:
		f.Remedy = psync.RemedyRetry
	case psync.FailLogin, psync.FailLoginExpired, psync.FailCookies, psync.FailMembers, psync.FailAge, psync.FailHiddenVideo, psync.FailDesktopSetup:
		f.Remedy = psync.RemedyDesktop
	}
	return f
}
