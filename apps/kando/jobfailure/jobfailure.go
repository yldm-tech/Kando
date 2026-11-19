// Package jobfailure says why a job failed, in the user's words: what its error means for them and the settings section that fixes it, from the step media.Make failed at (media.StageError) and what its tool said. It imports no interface package; the cards, the message bar, the phone and the control socket all read it.
package jobfailure

import (
	"errors"
	"regexp"
	"strings"

	"kando/apps/kando/locales"
	"kando/apps/kando/plugins"
	"kando/apps/kando/settings/sectionid"
	"kando/packages/core"
	"kando/packages/media"
	"kando/packages/track"
)

// Of is the step job failed at and why (media.StageError), nil for a job that did not fail or whose error names no step.
func Of(job core.Job) *media.StageError {
	var failed *media.StageError
	if job.Stage != core.StageFailed || !errors.As(job.Err, &failed) {
		return nil
	}
	return failed
}

// Stage is the step of media.Make job's error names (media.StageError), "" for an error that names none.
func Stage(job core.Job) media.Stage {
	var failed *media.StageError
	if !errors.As(job.Err, &failed) {
		return ""
	}
	return failed.Stage
}

// TranslationHint says, in the language of m, why a done film translated none of its lines, err being core.Job.TranslationErr and source core.Job.TranslationSource, as Hint says why a translation failed, and the settings section that fixes it: the film page and the card say it, and the control socket says it in English.
func TranslationHint(m locales.Messages, err, source string) (reason, section string) {
	failed := &media.StageError{Stage: media.StageTranslate, Source: source, Err: errors.New(err)}
	return Hint(m, m.QueueStatusFailed(failed.Error()), failed)
}

// Hint says in the user's words, in the language of m, which is the one the card's detail is in, what a failed job's error means for them, and the settings section that fixes it ("" when none does). detail is what the card says of the job (its status line) and failed the step it failed at (Of). A translation source of its own that failed (media.Tools.Engine, failed.Source) is named with what it said, as its settings are not Kando's AI service's and its words are not the AI service's; otherwise a known reason in the user's words, or else the step that failed and what its tool said, without the tool's prefixes. A job that was stopped rather than failed, as one whose library was switched, already says so in the app's words, which are kept as they are.
func Hint(m locales.Messages, detail string, failed *media.StageError) (reason, section string) {
	detail, isError := strings.CutPrefix(detail, failedPrefix(m))
	if !isError {
		return detail, ""
	}
	// The step media.Make failed at, and its error without the step's name.
	var stage media.Stage
	rest := detail
	if failed != nil {
		stage, rest = failed.Stage, failed.Detail()
		if failed.Source != "" {
			return SourceHint(m, failed.Source, rest)
		}
	}
	switch {
	case strings.Contains(detail, "KANDO_WHISPER_MODEL"), strings.Contains(detail, "whisper model"):
		return m.PrecheckNoSpeechModel(), sectionid.Speech
	case strings.Contains(detail, "is not installed"):
		return m.PrecheckNoTools(), sectionid.General
	case strings.Contains(detail, media.ErrEveryAPIMissing.Error()):
		return m.PrecheckEveryAPIMissing(), sectionid.AI
	// Before media.ErrEveryAPIMissing, Kando said this, followed by "or set EVERYAPI_API_KEY", also when the everyapi program was missing; such a failed job kept in library.json reads as signed out.
	case strings.Contains(detail, media.ErrEveryAPISignedOut.Error()):
		return m.PrecheckEveryAPISignedOut(), sectionid.AI
	case strings.Contains(detail, "KANDO_LLM_MODEL"):
		return m.PrecheckNoLLMModel(), sectionid.AI
	case strings.Contains(detail, media.ErrNoSpeech.Error()):
		return m.PrecheckNoSpeech(), ""
	// A job that failed before media.ErrNoAudio kept ffmpeg's own words in library.json.
	case strings.Contains(detail, media.ErrNoAudio.Error()), media.NoAudioStream(detail):
		return m.PrecheckNoAudio(), ""
	case strings.Contains(detail, media.ErrNoTranscript.Error()):
		return m.PrecheckNoTranscript(), ""
	case strings.Contains(detail, media.ErrSubtitlesChanged.Error()):
		return m.PrecheckSubtitlesChanged(), ""
	case strings.Contains(detail, track.ErrNewer.Error()):
		return m.PrecheckNewerKando(), ""
	case strings.Contains(detail, media.ErrNoLength.Error()):
		return m.PrecheckNoLength(), ""
	case strings.Contains(detail, media.ErrOffline.Error()) && stage == media.StageDownload:
		return m.PrivacyNoDownload(), sectionid.Privacy
	case strings.Contains(detail, media.ErrOffline.Error()):
		return m.PrivacyNoCloudAI(), sectionid.AI
	}
	if stage == media.StageTranslate {
		switch media.ClassifyTranslate(rest) {
		case media.TranslateQuota:
			return m.PrecheckTranslateQuota(), sectionid.AI
		case media.TranslateAuth:
			return m.PrecheckTranslateAuth(), sectionid.AI
		case media.TranslateRejected:
			return m.PrecheckTranslateRejected(), sectionid.AI
		case media.TranslateMalformed:
			return m.PrecheckTranslateMalformed(), sectionid.AI
		case media.TranslateTimeout:
			return m.PrecheckTranslateTimeout(), ""
		case media.TranslateNetwork:
			return m.PrecheckTranslateNetwork(), ""
		case media.TranslateBusy:
			return m.PrecheckTranslateBusy(), ""
		}
	}
	if msg, known := stageFailed(m, stage, shorten(ToolMessage(rest), 72)); known {
		return msg, ""
	}
	return m.PrecheckFailedProcess(shorten(ToolMessage(detail), 72)), ""
}

// SourceHint says in the user's words, in the language of m, what the failure of the translation source named source (media.Tools.Engine, media.StageError.Source) means, detail being what it said (media.StageError.Detail), and the settings section that fixes it ("" when none does). A plugin's translator that failed with a code Kando knows (plugins.TranslateCode) is told in words of its own: its key and its plugin's switch are in 插件, the target language in 翻译. Any other failure is told with what the source said, as its words are not the AI service's.
func SourceHint(m locales.Messages, source, detail string) (reason, section string) {
	code, _ := plugins.TranslateCode(strings.TrimSpace(detail))
	switch code {
	case plugins.TranslateQuota:
		return m.PrecheckSourceQuota(source), ""
	case plugins.TranslateAuth:
		return m.PrecheckSourceAuth(source), sectionid.Plugins
	case plugins.TranslateNoCredential:
		return m.PrecheckSourceNoCredential(source), sectionid.Plugins
	case plugins.TranslateLanguage:
		return m.PrecheckSourceLanguage(source), sectionid.Translate
	case plugins.TranslateRateLimit:
		return m.PrecheckSourceRateLimit(source), ""
	case plugins.TranslateUnavailable:
		return m.PrecheckSourceUnavailable(source), ""
	case plugins.TranslateNotRunning:
		return m.PrecheckSourceNotRunning(source), sectionid.Plugins
	case plugins.TranslateBadReply:
		return m.PrecheckSourceBadReply(source), ""
	}
	return m.PrecheckFailedSource(source, shorten(ToolMessage(detail), 72)), ""
}

// failedPrefix is what a failed job's status line says before its error in the language of m (QueueStatusFailed), which Hint takes off the card's detail to read the error.
func failedPrefix(m locales.Messages) string {
	prefix, _, _ := strings.Cut(m.QueueStatusFailed("\x00"), "\x00")
	return prefix
}

// stageFailed says that the step media.Make says failed did, with what its tool said, for an error no reason is known for, in the language of m; known is false for a stage it does not know.
func stageFailed(m locales.Messages, stage media.Stage, detail string) (msg string, known bool) {
	switch stage {
	case media.StageCheck:
		return m.PrecheckFailedProcess(detail), true
	case media.StageDownload:
		return m.PrecheckFailedDownload(detail), true
	case media.StageTranscribe:
		return m.PrecheckFailedTranscribe(detail), true
	case media.StageTranslate:
		return m.PrecheckFailedTranslate(detail), true
	case media.StageExport:
		return m.PrecheckFailedExport(detail), true
	}
	return "", false
}

// toolPrefix is what comes before what a tool says went wrong in its error: the tool and how it exited (yt-dlp: exit status 1:, or whisper-cli: signal: killed: for one the system stopped), and in yt-dlp's ERROR line the word ERROR and the extractor with the video id ([youtube] abc:).
var toolPrefix = regexp.MustCompile(`^(?:[\w.+-]+: (?:exit status [^:]*|signal: [^:]*): )?(?:ERROR: )?(?:\[[^\]]+\] (?:[^\s:]+: )?)?`)

// ToolMessage is the part of a stage's error that says what went wrong, in its first line, without the tool's prefixes.
func ToolMessage(err string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(err), "\n")
	if msg := strings.TrimSpace(toolPrefix.ReplaceAllString(line, "")); msg != "" {
		return msg
	}
	return line
}

// shorten is s cut to n characters with an ellipsis, as films.Shorten cuts a card's text; this package does not import the films interface package.
func shorten(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}
