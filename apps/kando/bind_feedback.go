package main

import (
	"strings"

	"kando/apps/kando/desktop"
	"kando/apps/kando/feedback"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/core"
)

// The 反馈问题 dialog's reads and writes (feedback.App), and the player's way to open it (film.App's FeedbackSubtitles).

// feedbackOverlay is the dialog over the window while it is open (state_overlay.go).
func (a *app) feedbackOverlay(*core.Scope) core.Node { return feedback.Dialog{App: a} }

func (a *app) FeedbackOpen() bool { return a.feedback.open }

func (a *app) FeedbackForm() *feedback.Form { return &a.feedback.form }

// feedbackIssue is the issue the form makes now from what the app knows, scrubbed and fitted to the link.
func (a *app) feedbackIssue(facts feedback.Facts) feedback.Issue {
	return a.feedback.form.Report(facts).Issue(a.feedbackScrubber())
}

// FeedbackView works out what the dialog shows from the form and what the app knows now, its words in the language the app shows under cx: what each row of 附带信息 attaches, and the issue, scrubbed and fitted to the link.
func (a *app) FeedbackView(cx *core.Scope) feedback.View {
	facts := a.feedbackFacts()
	issue := a.feedbackIssue(facts)
	view := feedback.View{
		Issue:   issue,
		Copied:  a.feedback.copied != "" && a.feedback.copied == issue.Full,
		Sending: a.feedback.sending,
	}
	if a.feedback.failed {
		view.Err = locales.FeedbackBrowserFailed(cx, feedback.Repo)
	}
	view.Env = feedback.Attachment{Detail: locales.FeedbackEnvDetail(cx, appVersion, facts.OS), Enabled: true}
	tools := make([]string, len(facts.Tools))
	for i, tool := range facts.Tools {
		tools[i] = feedbackToolLabel(cx, tool)
	}
	speech := facts.Speech
	if speech == "" {
		speech = locales.FeedbackNoSpeechModel(cx)
	}
	view.Tools = feedback.Attachment{Detail: locales.FeedbackToolsDetail(cx, strings.Join(tools, " · "), speech, facts.Translate), Enabled: true}
	if facts.Film != nil {
		view.Film = feedback.Attachment{Detail: locales.FeedbackFilmDetail(cx), Enabled: true}
	} else {
		view.Film = feedback.Attachment{Detail: locales.FeedbackFilmNone(cx)}
	}
	if facts.Excerpt != nil {
		view.Subs = feedback.Attachment{Detail: locales.FeedbackSubsDetail(cx, feedback.Clock(facts.Excerpt.At)), Enabled: true}
	} else {
		view.Subs = feedback.Attachment{Detail: locales.FeedbackSubsNone(cx)}
	}
	return view
}

// feedbackToolLabel is a tool and its version as the 组件与模型 row shows it, in the language the app shows under cx; the issue writes it with feedback.Tool's String, in Chinese.
func feedbackToolLabel(cx *core.Scope, t feedback.Tool) string {
	switch {
	case t.Missing:
		return locales.FeedbackToolMissing(cx, t.Name)
	case t.Version == "":
		return locales.FeedbackToolNoVersion(cx, t.Name)
	}
	return t.Name + " " + t.Version
}

// FeedbackCopied remembers the body 拷贝正文 put on the clipboard, so the button reads 已拷贝 until the body changes.
func (a *app) FeedbackCopied() { a.feedback.copied = a.feedbackIssue(a.feedbackFacts()).Full }

// SendFeedback asks the system to open the prefilled new-issue page in the browser, and closes the dialog once it did. If no app opened it, the dialog stays open and says so, so nothing typed is lost. The answer comes back on the engine's goroutine, so it tells the revision itself.
func (a *app) SendFeedback() {
	if a.feedback.sending || strings.TrimSpace(a.feedback.form.Title) == "" || a.cx == nil {
		return
	}
	link := a.feedbackIssue(a.feedbackFacts()).URL
	a.feedback.sending, a.feedback.failed = true, false
	gen := a.feedback.gen
	desktop.OpenURL(a.cx, link, live.On(a, func(opened bool) {
		if a.feedback.gen != gen {
			return
		}
		a.feedback.sending = false
		if !opened {
			a.feedback.failed = true
			return
		}
		a.feedbackClose()
	}))
}

// CloseFeedback closes the dialog: 取消 or Escape.
func (a *app) CloseFeedback() { a.feedbackClose() }

// FeedbackSubtitles opens the dialog for the open film's subtitles, as the player's ⋯ menu offers.
func (a *app) FeedbackSubtitles() { a.openFeedback(feedback.KindSubs) }
