package main

import (
	"errors"
	"strings"

	"kando/apps/kando/films/sitename"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings"
	"kando/apps/kando/settings/sectionid"
	"kando/apps/kando/sidebar"
	"kando/packages/core"
	"kando/packages/media"
)

// precheck says why sources cannot be processed yet, in the prototype's words (locales.Precheck…) in the interface's language, as the message bar and the guide's link field keep them, and the settings section that fixes it ("" when none does), in the prototype's order: an empty field, something that is not a link, a channel, a list or a page that names no single video, then what precheckReady finds missing to start; a missing AI service is not one of them (untranslatable). It reads only what the app already holds, so a click can afford it; what it cannot tell without running a program, as an EveryAPI login that has expired, still fails the job, whose card says so.
func (a *app) precheck(sources []string) (msg, section string) {
	return a.precheckIn(a.messages(), sources)
}

// precheckIn is precheck in the language of m, as the control socket says it in English.
func (a *app) precheckIn(m locales.Messages, sources []string) (msg, section string) {
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
		if a.privacy.offline && link.Platform != core.PlatformLocal && link.Platform != core.PlatformSource {
			return m.PrivacyNoDownload(), sectionid.Privacy
		}
		if !link.OneVideo() {
			return precheckWhole(m, link), ""
		}
	}
	return a.precheckReadyIn(m)
}

// precheckReady is precheckReadyIn in the interface's language.
func (a *app) precheckReady() (msg, section string) { return a.precheckReadyIn(a.messages()) }

// precheckReadyIn is what precheck says of the app rather than of the sources: no library, then the tools and settings a job needs. A message bar that says it closes once it says nothing, and follows it once it says something else (libraryFollowNotice, browseFollowNotice), as the problem was fixed in the meantime.
func (a *app) precheckReadyIn(m locales.Messages) (msg, section string) {
	if strings.TrimSpace(a.snap().LibraryDir) == "" {
		return m.PrecheckNoLibrary(), ""
	}
	if a.lockShut() {
		return m.LockLibraryIsLocked(sidebar.Title(a.snap().LibraryDir)), ""
	}
	// No AI service does not stop a job: its film's subtitles are the original lines until it is translated (core.Job.TranslationErr), as the message bar says once it is added (untranslatable).
	if say := a.precheckTools(); say != nil {
		return say(m)
	}
	return "", ""
}

// precheckTools is what precheckReadyIn finds missing among the tools and settings a job needs, whichever library it is for, nil when nothing is: found now, and said in the language of the Messages it is given, so a message kept to show later, as a subscription's page keeps why its videos were not queued, is said in the language shown then.
func (a *app) precheckTools() func(m locales.Messages) (msg, section string) {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return nil
	}
	// It asks for every tool media.Tools.Check asks for, so a job it lets through does not fail at its first step, and holds a job back while a tool's copy is known not to start (toolsBroken), which would fail it at the stage that runs it, or give yt-dlp an ffmpeg that cannot merge. That includes yt-dlp for a local video, which the pipeline reads where it is without downloading, because Check still asks for yt-dlp for every job.
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
	downloading := len(a.kitsDownloading()) > 0
	broken := a.toolsBroken()
	switch {
	case len(broken) > 0 && !downloading:
		return func(m locales.Messages) (string, string) {
			return m.PrecheckToolsBroken(strings.Join(broken, m.PrecheckToolSeparator())), settings.SectionGeneral
		}
	case len(broken) > 0:
		missing = append(missing, broken...)
		return func(m locales.Messages) (string, string) {
			return m.PrecheckToolsDownloading(strings.Join(missing, m.PrecheckToolSeparator())), settings.SectionGeneral
		}
	case len(missing) > 0 && downloading:
		return func(m locales.Messages) (string, string) {
			return m.PrecheckToolsDownloading(strings.Join(missing, m.PrecheckToolSeparator())), settings.SectionGeneral
		}
	case len(missing) > 0:
		return func(m locales.Messages) (string, string) {
			return m.PrecheckToolsMissing(strings.Join(missing, m.PrecheckToolSeparator())), settings.SectionGeneral
		}
	case tools.WhisperModel == "":
		return func(m locales.Messages) (string, string) { return m.PrecheckNoSpeechModel(), settings.SectionSpeech }
	}
	return nil
}

// precheckAboutApp reports whether msg, which precheck said, is what precheckReady says: a problem of the app rather than of the sources, which a message bar that shows it follows until it is fixed (precheckFollow).
func (a *app) precheckAboutApp(msg string) bool {
	ready, _ := a.precheckReady()
	return msg != "" && msg == ready
}

// precheckFollow brings a message bar that shows what precheckReady said, its text, section and open flag, up to date with what it says now: the bar closes once nothing is missing any more, and says what is missing now when that changed, as when the tools are in but no speech model is chosen yet. It reports whether it changed the bar.
func (a *app) precheckFollow(text, section *string, open *bool) bool {
	msg, sec := a.precheckReady()
	switch {
	case msg == *text && sec == *section:
		return false
	case msg == "":
		*open = false
	default:
		*text, *section = msg, sec
	}
	return true
}

// untranslatable reports whether a job added now would have no translator: no script plugin's translator is chosen in Settings › 翻译 (pluginsTranslating), and no model is chosen, or EveryAPI has neither an API Key nor a sign-in, Kando's own or the everyapi program's. What only running a program can tell, as an expired sign-in, is left to the job.
func (a *app) untranslatable() bool {
	tools, ok := a.pipeline.(media.Tools)
	return ok && !a.pluginsTranslating() && (strings.TrimSpace(tools.Translator.Model) == "" || tools.Translator.EveryAPIUnsigned())
}

// precheckWhole says that link names a channel, a list or a page that lists videos rather than one video, which a job would download whole or pick a video of, naming the site when Kando knows it, in the language of m. It asks for its videos to be listed first, which the playlist dialog does where the pipeline can list them (playlistSplit).
func precheckWhole(m locales.Messages, link core.Link) string {
	site := sitename.In(m, link.Site)
	switch {
	case link.Kind == core.LinkPage:
		return m.PrecheckPage()
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
