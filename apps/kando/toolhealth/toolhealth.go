// Package toolhealth is which copy of each program Kando runs, chosen by whether it works (media.Probe), and how a copy that does not work is said to the user. It holds no app state; the app's tools area (state_tools.go) probes the copies off the UI thread and asks Pick.
package toolhealth

import (
	"slices"

	"kando/apps/kando/locales"
	"kando/packages/media"
)

// Programs are the programs Kando runs, in the order the pipeline runs them.
var Programs = []string{"yt-dlp", "ffmpeg", "whisper-cli"}

// The copies of a program the user can choose to run (Pick's choice): Kando's own, or the computer's. Without a choice, Kando runs the copies in the order media.Candidates finds them, its own first unless its kit is only a fallback.
const (
	Own      = "own"
	Computer = "computer"
)

// Pick chooses the copy of a program to run among candidates, the copies media.Candidates found in the order Kando prefers them, by health, how each answered (a copy not in it has not answered yet). own is Kando's own copy, "" when it has none, and choice the copy the user chose (Own or Computer), which puts Kando's own first or last; "" keeps the order. The first copy that works wins; while none is known to work, the first that is not known to be broken, which may still be on its first run; and when every copy is broken, the first, which the settings show broken. other is a copy on the computer, besides Kando's own, that works, which the user may choose instead; "" when Kando has no copy of its own to choose between, or the computer none that works.
func Pick(candidates []string, own, choice string, health map[string]media.Health) (chosen, other string) {
	order := slices.Clone(candidates)
	if i := slices.Index(order, own); own != "" && i >= 0 {
		switch choice {
		case Computer:
			order = append(slices.Delete(order, i, i+1), own)
		case Own:
			order = append([]string{own}, slices.Delete(order, i, i+1)...)
		}
	}
	for _, path := range order {
		if own != "" && path != own && other == "" && health[path].State == media.HealthReady {
			other = path
		}
	}
	for _, path := range order {
		if health[path].State == media.HealthReady {
			return path, other
		}
	}
	for _, path := range order {
		if health[path].State != media.HealthBroken {
			return path, other
		}
	}
	if len(order) > 0 {
		return order[0], other
	}
	return "", other
}

// StageTools are the programs a step of media.Make runs, as a failed job names the step (media.StageError): the download runs yt-dlp, which merges with ffmpeg; the transcription has ffmpeg take the sound out for whisper-cli; the export cuts with ffmpeg; and the check before a job asks for all three.
func StageTools(stage media.Stage) []string {
	switch stage {
	case media.StageCheck:
		return Programs
	case media.StageDownload:
		return []string{"yt-dlp", "ffmpeg"}
	case media.StageTranscribe:
		return []string{"ffmpeg", "whisper-cli"}
	case media.StageExport:
		return []string{"ffmpeg"}
	}
	return nil
}

// Why says in the language of m why a copy whose health is broken does not run, as the settings' row, a failed job's card and the feedback say it.
func Why(m locales.Messages, h media.Health) string {
	switch h.Problem {
	case media.ProblemLibrary:
		if h.Library != "" {
			return m.ToolProblemLibrary(h.Library)
		}
		return m.ToolProblemLibraryUnnamed()
	case media.ProblemCrashed:
		return m.ToolProblemCrashed()
	case media.ProblemCannotStart:
		return m.ToolProblemCannotStart()
	}
	return m.ToolProblemFailed()
}
