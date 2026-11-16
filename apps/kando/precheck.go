package main

import (
	"errors"
	"regexp"
	"strings"

	"kando/apps/kando/films"
	"kando/apps/kando/films/sitename"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings"
	"kando/packages/core"
	"kando/packages/media"
)

// precheck says why sources cannot be processed yet, in the prototype's words (locales.Precheck…) in the interface's language, as the message bar and the guide's link field keep them, and the settings section that fixes it ("" when none does), in the prototype's order: an empty field, something that is not a link, a channel or a list that names no single video, no library, then the tools and settings a job needs. It reads only what the app already holds, so a click can afford it; what it cannot tell without running a program, as an EveryAPI login that has expired, still fails the job, whose card says so.
func (a *app) precheck(sources []string) (msg, section string) {
	m := a.messages()
	if len(sources) == 0 {
		return m.PrecheckEmpty(), ""
	}
	for _, source := range sources {
		source = strings.TrimSpace(source)
		if source == "" {
			return m.PrecheckEmpty(), ""
		}
		link, ok := core.ParseLink(source)
		if !ok {
			return m.PrecheckNotLink(), ""
		}
		if !link.OneVideo() {
			return precheckWhole(m, link), ""
		}
	}
	if strings.TrimSpace(a.snap().LibraryDir) == "" {
		return m.PrecheckNoLibrary(), ""
	}
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return "", ""
	}
	// It asks for every tool media.Tools.Check asks for, so a job it lets through does not fail at its first step. That includes yt-dlp for a local video, which the pipeline reads where it is without downloading, because Check still asks for yt-dlp for every job.
	var missing []string
	if tools.YTDLP == "" {
		missing = append(missing, "yt-dlp")
	}
	if tools.FFmpeg == "" {
		missing = append(missing, "ffmpeg")
	}
	if tools.Whisper == "" {
		missing = append(missing, "whisper-cli")
	}
	switch {
	case len(missing) > 0 && len(a.kitsDownloading()) > 0:
		return m.PrecheckToolsDownloading(strings.Join(missing, m.PrecheckToolSeparator())), settings.SectionGeneral
	case len(missing) > 0:
		return m.PrecheckToolsMissing(strings.Join(missing, m.PrecheckToolSeparator())), settings.SectionGeneral
	case tools.WhisperModel == "":
		return m.PrecheckNoSpeechModel(), settings.SectionSpeech
	case tools.Translator.Model == "":
		return m.PrecheckNoLLMModel(), settings.SectionAI
	case everyAPICLIMissing(tools.Translator):
		return m.PrecheckEveryAPIMissing(), settings.SectionAI
	}
	return "", ""
}

// precheckWhole says that link names a channel or a list rather than one video, which a job would download whole, naming the site when Kando knows it, in the language of m. It asks for the link of one of its videos, as the onboarding's link field does (onboarding.LinkHint, which also refuses a local file and so is not used here).
func precheckWhole(m locales.Messages, link core.Link) string {
	site := sitename.In(m, link.Site)
	switch {
	case link.Kind == core.LinkChannel && site == "":
		return m.PrecheckChannelUnnamed()
	case link.Kind == core.LinkChannel:
		return m.PrecheckChannel(site)
	case site == "":
		return m.PrecheckListUnnamed()
	}
	return m.PrecheckList(site)
}

// sourceError says in the library's words, in the language of m, why core.NewJob, through Enqueue, refused a source.
func sourceError(m locales.Messages, err error) string {
	switch {
	case errors.Is(err, core.ErrEmptySource):
		return m.PrecheckEmpty()
	case err.Error() == "video source must be a valid URL":
		return m.PrecheckNotLink()
	}
	return err.Error()
}

// failureHint says in the user's words, in the language of m, which is the one the card's detail is in, what a failed job's error means for them, and the settings section that fixes it ("" when none does): a known reason in the user's words, or else the stage that failed and what its tool said, without the tool's prefixes. A job that was stopped rather than failed, as one whose library was switched, already says so in the app's words (statusText), which are kept as they are.
func failureHint(m locales.Messages, item films.Film) (reason, section string) {
	detail, isError := strings.CutPrefix(item.Detail, failedPrefix(m))
	if !isError {
		return item.Detail, ""
	}
	switch {
	case strings.Contains(detail, "KANDO_WHISPER_MODEL"), strings.Contains(detail, "whisper model"):
		return m.PrecheckNoSpeechModel(), settings.SectionSpeech
	case strings.Contains(detail, "is not installed"):
		return m.PrecheckNoTools(), settings.SectionGeneral
	case strings.Contains(detail, media.ErrEveryAPIMissing.Error()):
		return m.PrecheckEveryAPIMissing(), settings.SectionAI
	// Before media.ErrEveryAPIMissing, Kando said this, followed by "or set EVERYAPI_API_KEY", also when the everyapi program was missing; such a failed job kept in library.json reads as signed out.
	case strings.Contains(detail, media.ErrEveryAPISignedOut.Error()):
		return m.PrecheckEveryAPISignedOut(), settings.SectionAI
	case strings.Contains(detail, "KANDO_LLM_MODEL"):
		return m.PrecheckNoLLMModel(), settings.SectionAI
	case strings.Contains(detail, media.ErrNoSpeech.Error()):
		return m.PrecheckNoSpeech(), ""
	// A job that failed before media.ErrNoAudio kept ffmpeg's own words in library.json.
	case strings.Contains(detail, media.ErrNoAudio.Error()), media.NoAudioStream(detail):
		return m.PrecheckNoAudio(), ""
	case strings.Contains(detail, media.ErrNoTranscript.Error()):
		return m.PrecheckNoTranscript(), ""
	case strings.Contains(detail, media.ErrSubtitlesChanged.Error()):
		return m.PrecheckSubtitlesChanged(), ""
	}
	// media.Run names the stage that failed before its error.
	stage, rest, ok := strings.Cut(detail, ": ")
	if !ok {
		stage, rest = "", detail
	}
	if stage == "translate" {
		switch media.ClassifyTranslate(rest) {
		case media.TranslateQuota:
			return m.PrecheckTranslateQuota(), settings.SectionAI
		case media.TranslateAuth:
			return m.PrecheckTranslateAuth(), settings.SectionAI
		case media.TranslateRejected:
			return m.PrecheckTranslateRejected(), settings.SectionAI
		case media.TranslateMalformed:
			return m.PrecheckTranslateMalformed(), settings.SectionAI
		case media.TranslateTimeout:
			return m.PrecheckTranslateTimeout(), ""
		case media.TranslateNetwork:
			return m.PrecheckTranslateNetwork(), ""
		case media.TranslateBusy:
			return m.PrecheckTranslateBusy(), ""
		}
	}
	if failed, known := stageFailed(m, stage, films.Shorten(toolMessage(rest), 72)); known {
		return failed, ""
	}
	return m.PrecheckFailedProcess(films.Shorten(toolMessage(detail), 72)), ""
}

// failedPrefix is what statusText says before a failed job's error in the language of m, which failureHint takes off the card's detail to read the error.
func failedPrefix(m locales.Messages) string {
	prefix, _, _ := strings.Cut(m.QueueStatusFailed("\x00"), "\x00")
	return prefix
}

// stageFailed says that the stage media.Run says failed did, with what its tool said, for an error no reason is known for, in the language of m; known is false for a stage it does not know.
func stageFailed(m locales.Messages, stage, detail string) (msg string, known bool) {
	switch stage {
	case "check":
		return m.PrecheckFailedProcess(detail), true
	case "download":
		return m.PrecheckFailedDownload(detail), true
	case "transcribe":
		return m.PrecheckFailedTranscribe(detail), true
	case "translate":
		return m.PrecheckFailedTranslate(detail), true
	case "export":
		return m.PrecheckFailedExport(detail), true
	}
	return "", false
}

// toolPrefix is what comes before what a tool says went wrong in its error: the tool and how it exited (yt-dlp: exit status 1:, or whisper-cli: signal: killed: for one the system stopped), and in yt-dlp's ERROR line the word ERROR and the extractor with the video id ([youtube] abc:).
var toolPrefix = regexp.MustCompile(`^(?:[\w.+-]+: (?:exit status [^:]*|signal: [^:]*): )?(?:ERROR: )?(?:\[[^\]]+\] (?:[^\s:]+: )?)?`)

// toolMessage is the part of a stage's error that says what went wrong, in its first line, without the tool's prefixes.
func toolMessage(err string) string {
	line, _, _ := strings.Cut(strings.TrimSpace(err), "\n")
	if msg := strings.TrimSpace(toolPrefix.ReplaceAllString(line, "")); msg != "" {
		return msg
	}
	return line
}
