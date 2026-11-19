package main

import (
	"context"
	"errors"
	"os"
	"strings"
	"time"

	"kando/apps/kando/locales"
	"kando/apps/kando/settings"
	"kando/apps/kando/sidebar"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
)

// readiness is what the sidebar's status line last learned about whether a job can start. Everything but the EveryAPI sign-in is read on the UI thread when it changes; the sign-in asks the everyapi CLI, which can take up to 15 seconds, so it runs off the UI thread and its answer is kept until the tools are detected again or 测试 in 「AI 服务」 reaches the model through the CLI's key.
type readiness struct {
	// checked is set once checkReadiness has run; status is what it found.
	checked bool
	status  sidebar.Readiness
	// signInDone is set once a sign-in check finished, with its error in signInErr. gen numbers the sign-in checks, so the answer of one started before another is dropped. asking is set while the last one runs, so the changes that do not ask again (signIn false), as each keystroke in 「AI 服务」, wait for its answer instead of starting another.
	signInDone bool
	signInErr  error
	gen        int
	asking     bool
}

// The status line's messages, said in the language the app shows under the sidebar's scope each time its binding reads them (sidebar.Readiness.Message). Where they say what the library's message bar says, they are its words (locales.Precheck…).
var (
	readyMessage      = locales.ReadinessReady
	noLibrary         = locales.PrecheckNoLibrary
	noSpeechModel     = locales.PrecheckNoSpeechModel
	noLLMModel        = locales.PrecheckNoLLMModel
	everyAPIMissing   = locales.PrecheckEveryAPIMissing
	everyAPISignedOut = locales.PrecheckEveryAPISignedOut
	everyAPIFailed    = locales.PrecheckEveryAPIFailed
	checkingSignIn    = locales.ReadinessCheckingSignIn
	checkingTools     = locales.ReadinessCheckingTools
)

// readiness is the status line the sidebar shows. A missing library is read from the shelf as it draws, and so are the programs Kando is downloading (state_kits.go); the rest is what checkReadiness last found.
func (a *app) readiness() sidebar.Readiness {
	if strings.TrimSpace(a.snap().LibraryDir) == "" {
		return sidebar.Readiness{State: sidebar.Missing, Message: noLibrary, Section: settings.SectionGeneral}
	}
	if names := a.kitsDownloading(); len(names) > 0 {
		return sidebar.Readiness{State: sidebar.Checking, Message: func(cx *ns.Scope) string {
			return locales.ReadinessDownloadingTools(cx, strings.Join(names, locales.PrecheckToolSeparator(cx)))
		}, Section: settings.SectionGeneral}
	}
	if !a.side.ready.checked {
		return sidebar.Readiness{State: sidebar.Checking, Message: checkingTools}
	}
	return a.privacyReadiness(a.side.ready.status)
}

// checkReadiness finds the first thing a job would miss, in the order the pipeline needs them: the tools, a copy of one that does not start (toolsBroken) before one that is missing, the speech model, the translation model and the EveryAPI sign-in. It runs at startup, after 重新检测 and when the speech model or the AI service changes. signIn asks the everyapi CLI again even when an earlier answer is kept or a check is running; without it, a running check's answer is waited for.
func (a *app) checkReadiness(signIn bool) {
	a.side.ready.checked = true
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		// Another pipeline, as a test's, has only its own check.
		if err := a.pipeline.Check(); err != nil {
			msg := err.Error()
			a.side.ready.status = sidebar.Readiness{State: sidebar.Missing, Message: func(*ns.Scope) string { return msg }, Section: settings.SectionGeneral}
			return
		}
		a.side.ready.status = sidebar.Readiness{State: sidebar.Ready, Message: readyMessage}
		return
	}
	// A copy that does not start is as good as none, and Settings › 通用 says why and offers Kando's own.
	if names := a.toolsBroken(); len(names) > 0 {
		a.side.ready.status = sidebar.Readiness{State: sidebar.Missing, Message: func(cx *ns.Scope) string {
			return locales.ReadinessBrokenTools(cx, strings.Join(names, locales.PrecheckToolSeparator(cx)))
		}, Section: settings.SectionGeneral}
		return
	}
	if r, missing := toolsMissing(tools, a.pluginsTranslating()); missing {
		a.side.ready.status = r
		return
	}
	// A script plugin's translator chosen whose plugin cannot run fails every translation, which its details in Settings › 插件 say why.
	if name := a.pluginsTranslateBroken(); name != "" {
		a.side.ready.status = sidebar.Readiness{State: sidebar.Missing, Message: func(cx *ns.Scope) string { return locales.PrecheckSourceNotRunning(cx, name) }, Section: settings.SectionPlugins}
		return
	}
	credential := tools.Translator.Credential
	// Asking the everyapi CLI asks EveryAPI's servers, which offline mode does not (privacyReadiness says why the service will not translate).
	if credential == nil || a.privacy.offline {
		a.side.ready.status = sidebar.Readiness{State: sidebar.Ready, Message: readyMessage}
		return
	}
	checking := sidebar.Readiness{State: sidebar.Checking, Message: checkingSignIn, Section: settings.SectionAI}
	if a.side.ready.asking && !signIn {
		a.side.ready.status = checking
		return
	}
	if a.side.ready.signInDone && !signIn {
		a.side.ready.status = signInStatus(a.side.ready.signInErr)
		return
	}
	if a.update == nil {
		return
	}
	a.side.ready.gen++
	a.side.ready.asking = true
	gen := a.side.ready.gen
	a.side.ready.status = checking
	update := a.update
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 15*time.Second)
		_, err := credential(ctx, false)
		cancel()
		update(func() {
			if gen != a.side.ready.gen {
				return
			}
			a.signedIn(err)
		})
	}()
}

// signedIn keeps an answer about the EveryAPI sign-in, from the check checkReadiness started or from 测试 in 「AI 服务」 reaching the model through the CLI's key, in place of any check still running, and shows what it means.
func (a *app) signedIn(err error) {
	a.side.ready.gen++
	a.side.ready.asking = false
	a.side.ready.signInDone, a.side.ready.signInErr = true, err
	a.checkReadiness(false)
}

// toolsMissing reports the first thing tools lack that the UI thread can read at once, with the settings section that fixes it. With ownSource, a script plugin's translator chosen in Settings › 翻译 (pluginsTranslating), the films are translated without the AI service, which is then not missing.
func toolsMissing(tools media.Tools, ownSource bool) (sidebar.Readiness, bool) {
	var absent []string
	for _, tool := range []struct{ name, path string }{{"yt-dlp", tools.YTDLP}, {"ffmpeg", tools.FFmpeg}, {"whisper-cli", tools.Whisper}} {
		if tool.path == "" {
			absent = append(absent, tool.name)
		}
	}
	missing := func(message func(*ns.Scope) string, section string) (sidebar.Readiness, bool) {
		return sidebar.Readiness{State: sidebar.Missing, Message: message, Section: section}, true
	}
	if len(absent) > 0 {
		return missing(func(cx *ns.Scope) string {
			return locales.ReadinessMissingTools(cx, strings.Join(absent, locales.PrecheckToolSeparator(cx)))
		}, settings.SectionGeneral)
	}
	if tools.WhisperModel == "" {
		return missing(noSpeechModel, settings.SectionSpeech)
	}
	if info, err := os.Stat(tools.WhisperModel); err != nil || info.IsDir() {
		return missing(noSpeechModel, settings.SectionSpeech)
	}
	if ownSource {
		return sidebar.Readiness{}, false
	}
	if strings.TrimSpace(tools.Translator.Model) == "" {
		return missing(noLLMModel, settings.SectionAI)
	}
	if everyAPICLIMissing(tools.Translator) {
		return missing(everyAPIMissing, settings.SectionAI)
	}
	return sidebar.Readiness{}, false
}

func signInStatus(err error) sidebar.Readiness {
	if err != nil {
		return sidebar.Readiness{State: sidebar.Missing, Message: everyAPISignInFailed(err), Section: settings.SectionAI}
	}
	return sidebar.Readiness{State: sidebar.Ready, Message: readyMessage}
}

// everyAPICLIMissing reports whether t is EveryAPI without an API Key and the everyapi command-line program it would sign in through was not found, which leaves it without a Credential (media.Tools.UseEveryAPICLI).
func everyAPICLIMissing(t media.ChatTranslator) bool {
	return t.Credential == nil && t.BaseURL == media.EveryAPIBaseURL && t.APIKey == ""
}

// everyAPISignInFailed is what to do about an everyapi sign-in that failed: install the program when it has gone since it was found, sign in to it when nobody is, and otherwise read on the AI 服务 page what it said, as when it timed out or crashed.
func everyAPISignInFailed(err error) func(*ns.Scope) string {
	switch {
	case errors.Is(err, media.ErrEveryAPIMissing):
		return everyAPIMissing
	case errors.Is(err, media.ErrEveryAPISignedOut):
		return everyAPISignedOut
	}
	return everyAPIFailed
}
