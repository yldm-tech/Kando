package media

import (
	"errors"
	"strings"
)

// Stage is a step of making a film's subtitles (Make), which a failed job names as the one it failed at (StageError).
type Stage string

// The steps Make names a failure by. The check is of what the job needs before any step runs (Pipeline.Check, TranslateChecker); the download fetches the link's sound and video; the transcription reads the video's length and hears its speech; the translation drafts, proofreads and reviews the lines; the export writes the film's subtitles beside the video.
const (
	StageCheck      Stage = "check"
	StageDownload   Stage = "download"
	StageTranscribe Stage = "transcribe"
	StageTranslate  Stage = "translate"
	StageExport     Stage = "export"
)

// stages are the steps a failure's saved text may start with (ReadStageError).
var stages = []Stage{StageCheck, StageDownload, StageTranscribe, StageTranslate, StageExport}

// StageError is why a job failed: the step it failed at and the error that stopped it. Its text is the step's name, ": " and the error's text, as a failed job's text has always read, so a library saved with it reads the same in an older Kando. errors.Is and errors.As see through it to Err.
type StageError struct {
	Stage Stage
	// Source is the name of the translation source (Tools.Engine, as Tools.EngineName names it) whose failure stopped the job, "" for one Kando's own tools or AI service failed.
	Source string
	Err    error
}

func (e *StageError) Error() string { return string(e.Stage) + ": " + e.Err.Error() }

func (e *StageError) Unwrap() error { return e.Err }

// failedAt is err as the failure of stage, with the translation source that failed when the error came from one (engineError).
func failedAt(stage Stage, err error) *StageError {
	failed := &StageError{Stage: stage, Err: err}
	var engine *engineError
	if errors.As(err, &engine) {
		failed.Source = engine.name
	}
	return failed
}

// ReadStageError is the error a failed job's saved text stands for when the library kept no step beside it, as a library saved before the step was kept apart: a *StageError when the text starts with the name of a step and ": ", as Make wrote it, with the rest as its error's text; otherwise the text as an error of its own. Neither tells the error it once wrapped (errors.Is), as no saved text does.
func ReadStageError(text string) error {
	for _, stage := range stages {
		if rest, ok := strings.CutPrefix(text, string(stage)+": "); ok {
			return &StageError{Stage: stage, Err: errors.New(rest)}
		}
	}
	return errors.New(text)
}

// engineError is the failure of a translation source other than Kando's own AI service (Tools.Engine), named as Tools.EngineName names it. Its text is the source's error's alone, so what reads a failure's text reads it as before; the step that failed takes the name (StageError.Source).
type engineError struct {
	name string
	err  error
}

func (e *engineError) Error() string { return e.err.Error() }

func (e *engineError) Unwrap() error { return e.err }
