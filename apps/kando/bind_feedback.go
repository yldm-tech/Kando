package main

import (
	"strings"

	"kando/apps/kando/feedback"
	"kando/apps/kando/locales"

	"github.com/egoist/mygo/ui"
)

// The 反馈问题 dialog's reads and writes (feedback.App), and the player's way to open it (film.App's FeedbackSubtitles).

// feedbackOverlay draws the dialog while it is open; overlays calls it.
func (a *app) feedbackOverlay(c *ui.Context, t *ui.Theme) {
	if a.feedback.open {
		feedback.Show(c, t, a, &a.feedback.open)
	}
}

func (a *app) FeedbackForm() *feedback.Form { return &a.feedback.form }

// FeedbackView works out what the dialog shows from the form and what the app knows now: what each row of 附带信息 attaches, and the issue, scrubbed and fitted to the link.
func (a *app) FeedbackView() feedback.View {
	facts := a.feedbackFacts()
	issue := a.feedback.form.Report(facts).Issue(a.feedbackScrubber())
	view := feedback.View{
		Issue:   issue,
		Copied:  a.feedback.copied != "" && a.feedback.copied == issue.Full,
		Sending: a.feedback.sending,
	}
	if a.feedback.err != nil {
		view.Err = locales.FeedbackBrowserFailed(a.feedback.err.Error(), feedback.Repo)
	}
	view.Env = feedback.Attachment{Detail: locales.FeedbackEnvDetail(appVersion, facts.OS), Enabled: true}
	tools := make([]string, len(facts.Tools))
	for i, tool := range facts.Tools {
		tools[i] = feedbackToolLabel(tool)
	}
	speech := facts.Speech
	if speech == "" {
		speech = locales.FeedbackNoSpeechModel()
	}
	view.Tools = feedback.Attachment{Detail: locales.FeedbackToolsDetail(strings.Join(tools, " · "), speech, facts.Translate), Enabled: true}
	if facts.Film != nil {
		view.Film = feedback.Attachment{Detail: locales.FeedbackFilmDetail(), Enabled: true}
	} else {
		view.Film = feedback.Attachment{Detail: locales.FeedbackFilmNone()}
	}
	if facts.Excerpt != nil {
		view.Subs = feedback.Attachment{Detail: locales.FeedbackSubsDetail(feedback.Clock(facts.Excerpt.At)), Enabled: true}
	} else {
		view.Subs = feedback.Attachment{Detail: locales.FeedbackSubsNone()}
	}
	return view
}

// feedbackToolLabel is a tool and its version as the 组件与模型 row shows it, in the interface's language; the issue writes it with feedback.Tool's String, in Chinese.
func feedbackToolLabel(t feedback.Tool) string {
	switch {
	case t.Missing:
		return locales.FeedbackToolMissing(t.Name)
	case t.Version == "":
		return locales.FeedbackToolNoVersion(t.Name)
	}
	return t.Name + " " + t.Version
}

// FeedbackCopied remembers the body 拷贝正文 put on the clipboard, so the button reads 已拷贝 until the body changes.
func (a *app) FeedbackCopied() { a.feedback.copied = a.FeedbackView().Issue.Full }

// SendFeedback asks the browser to open the prefilled new-issue page off the UI thread, and closes the dialog once it did. If the browser cannot be opened, the dialog stays open and says why, so nothing typed is lost.
func (a *app) SendFeedback() {
	if a.feedback.sending || strings.TrimSpace(a.feedback.form.Title) == "" {
		return
	}
	link := a.FeedbackView().Issue.URL
	a.feedback.sending, a.feedback.err = true, nil
	gen, update := a.feedback.gen, a.update
	go func() {
		err := feedbackOpenURL(link)
		update(func() {
			if a.feedback.gen != gen {
				return
			}
			a.feedback.sending = false
			if err != nil {
				a.feedback.err = err
				return
			}
			a.feedback.open = false
		})
	}()
}

// FeedbackSubtitles opens the dialog for the open film's subtitles, as the player's ⋯ menu offers.
func (a *app) FeedbackSubtitles() { a.openFeedback(feedback.KindSubs) }
