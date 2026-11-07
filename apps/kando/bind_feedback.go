package main

import (
	"strings"

	"kando/apps/kando/feedback"

	"github.com/egoist/mygo/ui"
)

// The 反馈问题 dialog's reads and writes (feedback.App), and the player's way to open it (film.App's FeedbackSubtitles).

// feedbackOverlay draws the dialog while it is open; overlays calls it.
func (a *app) feedbackOverlay(c *ui.Context, t *ui.Theme) {
	if a.fb.open {
		feedback.Show(c, t, a, &a.fb.open)
	}
}

func (a *app) FeedbackForm() *feedback.Form { return &a.fb.form }

// FeedbackView works out what the dialog shows from the form and what the app knows now: what each row of 附带信息 attaches, and the issue, scrubbed and fitted to the link.
func (a *app) FeedbackView() feedback.View {
	facts := a.feedbackFacts()
	issue := a.fb.form.Report(facts).Issue(a.feedbackScrubber())
	view := feedback.View{
		Issue:   issue,
		Copied:  a.fb.copied != "" && a.fb.copied == issue.Full,
		Sending: a.fb.sending,
		Err:     a.fb.err,
	}
	view.Env = feedback.Attachment{Detail: facts.App + " · " + facts.OS, Enabled: true}
	tools := make([]string, len(facts.Tools))
	for i, tool := range facts.Tools {
		tools[i] = tool.String()
	}
	speech := facts.Speech
	if speech == "" {
		speech = "未选模型"
	}
	view.Tools = feedback.Attachment{Detail: strings.Join(tools, " · ") + "；识别 " + speech + " · 翻译 " + facts.Translate + "（不含 Key）", Enabled: true}
	if facts.Film != nil {
		view.Film = feedback.Attachment{Detail: "来源、处理进度与错误信息，不含文件路径和链接", Enabled: true}
	} else {
		view.Film = feedback.Attachment{Detail: "当前不在播放页，没有可附带的影片"}
	}
	if facts.Excerpt != nil {
		view.Subs = feedback.Attachment{Detail: "会公开到 GitHub；只附带播放位置的这一句（" + feedback.Clock(facts.Excerpt.At) + "）", Enabled: true}
	} else {
		view.Subs = feedback.Attachment{Detail: "打开一部有字幕的影片后才能附带"}
	}
	return view
}

// FeedbackCopied remembers the body 拷贝正文 put on the clipboard, so the button reads 已拷贝 until the body changes.
func (a *app) FeedbackCopied() { a.fb.copied = a.FeedbackView().Issue.Full }

// SendFeedback asks the browser to open the prefilled new-issue page off the UI thread, and closes the dialog once it did. If the browser cannot be opened, the dialog stays open and says why, so nothing typed is lost.
func (a *app) SendFeedback() {
	if a.fb.sending || strings.TrimSpace(a.fb.form.Title) == "" {
		return
	}
	link := a.FeedbackView().Issue.URL
	a.fb.sending, a.fb.err = true, ""
	gen, update := a.fb.gen, a.update
	go func() {
		err := feedbackOpenURL(link)
		update(func() {
			if a.fb.gen != gen {
				return
			}
			a.fb.sending = false
			if err != nil {
				a.fb.err = "没能打开浏览器：" + err.Error() + "。可以用「拷贝正文」拷贝内容，到 github.com/" + feedback.Repo + "/issues/new 里粘贴。"
				return
			}
			a.fb.open = false
		})
	}()
}

// FeedbackSubtitles opens the dialog for the open film's subtitles, as the player's ⋯ menu offers.
func (a *app) FeedbackSubtitles() { a.openFeedback(feedback.KindSubs) }
