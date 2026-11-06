package main

import (
	"context"
	"os"
	"strings"
	"time"

	"kando/apps/kando/settings"
	"kando/apps/kando/sidebar"
	"kando/packages/media"
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

const (
	readyMessage   = "已就绪，可以开始处理"
	noLibrary      = "先新建或打开一个资料库"
	noSpeechModel  = "先在「语音识别」里选一个模型"
	noLLMModel     = "先在「AI 服务」里选一个模型"
	notSignedIn    = "先登录 EveryAPI，或在「AI 服务」里填 Key"
	checkingSignIn = "正在检查 EveryAPI 登录…"
	checkingTools  = "正在检查组件…"
)

// readiness is the status line the sidebar shows. A missing library is read from the shelf as it draws; the rest is what checkReadiness last found.
func (a *app) readiness() sidebar.Readiness {
	if strings.TrimSpace(a.snap().LibraryDir) == "" {
		return sidebar.Readiness{State: sidebar.Missing, Message: noLibrary, Section: settings.SectionGeneral}
	}
	if !a.side.ready.checked {
		return sidebar.Readiness{State: sidebar.Checking, Message: checkingTools}
	}
	return a.side.ready.status
}

// checkReadiness finds the first thing a job would miss, in the order the pipeline needs them: the tools, the speech model, the translation model and the EveryAPI sign-in. It runs at startup, after 重新检测 and when the speech model or the AI service changes. signIn asks the everyapi CLI again even when an earlier answer is kept or a check is running; without it, a running check's answer is waited for.
func (a *app) checkReadiness(signIn bool) {
	a.side.ready.checked = true
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		// Another pipeline, as a test's, has only its own check.
		if err := a.pipeline.Check(); err != nil {
			a.side.ready.status = sidebar.Readiness{State: sidebar.Missing, Message: err.Error(), Section: settings.SectionGeneral}
			return
		}
		a.side.ready.status = sidebar.Readiness{State: sidebar.Ready, Message: readyMessage}
		return
	}
	if r, missing := toolsMissing(tools); missing {
		a.side.ready.status = r
		return
	}
	credential := tools.Translator.Credential
	if credential == nil {
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

// toolsMissing reports the first thing tools lack that the UI thread can read at once, with the settings section that fixes it.
func toolsMissing(tools media.Tools) (sidebar.Readiness, bool) {
	var absent []string
	for _, tool := range []struct{ name, path string }{{"yt-dlp", tools.YTDLP}, {"ffmpeg", tools.FFmpeg}, {"whisper-cli", tools.Whisper}} {
		if tool.path == "" {
			absent = append(absent, tool.name)
		}
	}
	missing := func(message, section string) (sidebar.Readiness, bool) {
		return sidebar.Readiness{State: sidebar.Missing, Message: message, Section: section}, true
	}
	if len(absent) > 0 {
		return missing("缺少组件："+strings.Join(absent, "、"), settings.SectionGeneral)
	}
	if tools.WhisperModel == "" {
		return missing(noSpeechModel, settings.SectionSpeech)
	}
	if info, err := os.Stat(tools.WhisperModel); err != nil || info.IsDir() {
		return missing(noSpeechModel, settings.SectionSpeech)
	}
	if strings.TrimSpace(tools.Translator.Model) == "" {
		return missing(noLLMModel, settings.SectionAI)
	}
	if tools.Translator.Credential == nil && tools.Translator.BaseURL == media.EveryAPIBaseURL && tools.Translator.APIKey == "" {
		return missing(notSignedIn, settings.SectionAI)
	}
	return sidebar.Readiness{}, false
}

func signInStatus(err error) sidebar.Readiness {
	if err != nil {
		return sidebar.Readiness{State: sidebar.Missing, Message: notSignedIn, Section: settings.SectionAI}
	}
	return sidebar.Readiness{State: sidebar.Ready, Message: readyMessage}
}
