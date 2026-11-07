package feedback

import (
	"strconv"
	"strings"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// Form is what the dialog edits: the kind, as the index of its segmented control, the title, the description and which facts to attach.
type Form struct {
	Kind               int
	Title, Description string
	Env, Tools, Film   bool
	// Subs attaches the subtitle line at the playhead. It is off whenever the dialog opens: the line goes to a public issue.
	Subs bool
}

// Report is the form as a report of the facts given.
func (f Form) Report(facts Facts) Report {
	return Report{Kind: Kind(f.Kind), Title: f.Title, Description: f.Description, Env: f.Env, Tools: f.Tools, Film: f.Film, Subs: f.Subs, Facts: facts}
}

// Attachment is how a row of 附带信息 reads: the detail under its label, and whether it can be ticked now.
type Attachment struct {
	Detail  string
	Enabled bool
}

// View is what the app worked out for the dialog to show.
type View struct {
	Env, Tools, Film, Subs Attachment
	// Issue is the issue the form makes, scrubbed and fitted to the link, which the preview shows and 在 GitHub 中打开 opens.
	Issue Issue
	// Copied is set once 拷贝正文 copied the body, until the form changes. Sending is set while the browser is being asked to open the link; Err says why it could not.
	Copied, Sending bool
	Err             string
}

// App is the feedback dialog's state and the clicks it reports.
type App interface {
	FeedbackForm() *Form
	FeedbackView() View
	// FeedbackCopied is told that 拷贝正文 put the body on the clipboard.
	FeedbackCopied()
	// SendFeedback opens the issue in the browser and closes the dialog once it did.
	SendFeedback()
}

// The dialog's measures, as the prototype's: 620 wide, the header's GitHub tile 34 square with corners of 9, the preview at most 180 high.
const (
	width       = 620
	tileSize    = 34
	tileRadius  = 9
	previewMaxH = 180
)

var (
	// tile is the GitHub tile's #24292F.
	tile = ui.RGBA(36, 41, 47, 1)
	// previewFill is the preview's well, the prototype's #F2F2F5 over the dialog.
	previewFill = ui.RGBA(0, 0, 0, 0.045)
)

// Show draws the dialog as a sheet over the window while *open. Escape and 取消 close it; 在 GitHub 中打开 asks the app to open the issue, which closes it once the browser has the link.
func Show(c *ui.Context, t *ui.Theme, a App, open *bool) {
	form := a.FeedbackForm()
	view := a.FeedbackView()
	panel := breeze.Sheet(c, open, func() {
		ui.Column(c).Grow(1).Shrink(1).MinHeight(0).Gap(12).AlignItems(ui.Stretch).Children(func() {
			header(c)
			ui.Scroll(c).Grow(1).Shrink(1).MinHeight(0).Children(func() {
				ui.Column(c).Gap(12).AlignItems(ui.Stretch).Padding(0, 2).Children(func() {
					c.Key("feedback-kind")
					breeze.Segmented(c, &form.Kind, Kinds...).AlignSelf(ui.Start).Label("反馈类型")
					fields(c, form)
					section(c, "附带信息")
					breeze.Card(c, func() {
						attachment(c, "feedback-env", &form.Env, "应用与系统版本", view.Env)
						attachment(c, "feedback-tools", &form.Tools, "组件与模型", view.Tools)
						attachment(c, "feedback-film", &form.Film, "当前影片的处理状态", view.Film)
						attachment(c, "feedback-subs", &form.Subs, "相关字幕原文与译文", view.Subs)
					})
					section(c, "Issue 预览")
					preview(c, t, view.Issue)
				})
			})
			footer(c, t, a, form, view, open)
		})
	})
	panel.Width(width)
}

// header is the GitHub tile, the title and what will happen.
func header(c *ui.Context) {
	ui.Row(c).Gap(12).AlignItems(ui.Start).Children(func() {
		ui.Row(c).Size(tileSize, tileSize).Shrink(0).Radius(tileRadius).Background(tile).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
			ui.Image(c, iconGitHub).Size(20, 20)
		})
		ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
			breeze.Text(c, "反馈问题", breeze.TextHeading).FontSize(15).Bold().Role(ui.RoleHeading).Level(1)
			breeze.Text(c, "会在浏览器里打开 GitHub 的新建 Issue 页面，内容已经填好，你确认后再提交。Issue 是公开的。", breeze.TextCaption, breeze.TextSecondary).FontSize(12).FixedLineHeight(17).Wrap()
		})
	})
}

// fields are the title and the description, in a card. The dialog opens with the keyboard in the title, or in the description when it opens with a title already, as for a film's subtitles.
func fields(c *ui.Context, form *Form) {
	titled := strings.TrimSpace(form.Title) != ""
	breeze.Card(c, func() {
		breeze.ContentRow(c, "标题", func() {
			c.Key("feedback-title")
			title := breeze.TextInput(c, &form.Title).Label("标题").Placeholder(titleHint(Kind(form.Kind)))
			if !titled {
				title.AutoFocus()
			}
		})
		breeze.ContentRow(c, "描述", func() {
			c.Key("feedback-description")
			description := breeze.TextArea(c, &form.Description).Label("描述").Placeholder(descriptionHint(Kind(form.Kind))).Height(92)
			if titled {
				description.AutoFocus()
			}
		})
	})
}

func titleHint(k Kind) string {
	if k == KindIdea {
		return "一句话说明想要什么"
	}
	return "一句话说明问题"
}

func descriptionHint(k Kind) string {
	switch k {
	case KindIdea:
		return "想要什么功能，用在什么场景"
	case KindSubs:
		return "哪一句、哪里不对、你觉得应该是什么"
	}
	return "做了什么、期望看到什么、实际看到什么"
}

// section is a label over a group, 11 bold in the secondary color.
func section(c *ui.Context, label string) {
	breeze.Text(c, label, breeze.TextCaption, breeze.TextSecondary).FontSize(11).Bold().Padding(0, 4)
}

// attachment is a row of 附带信息: a check box, the label and what it attaches. A click anywhere on the row toggles it, as a click on a check box's title does. A row that has nothing to attach now is unticked and cannot be ticked.
func attachment(c *ui.Context, key string, on *bool, label string, at Attachment) {
	if !at.Enabled {
		*on = false
	}
	row := breeze.PrefixRow(c, label, at.Detail, func() {
		c.Key(key)
		breeze.Checkbox(c, on, "").Label(label).Disabled(!at.Enabled)
	}, nil)
	// A click on the check box is the check box's own; the row takes the clicks elsewhere on it.
	if at.Enabled {
		row.OnClick(func() { *on = !*on })
	}
}

// preview is the issue as it will be sent, in a scrolling monospaced well, and under it why it was cut when it was.
func preview(c *ui.Context, t *ui.Theme, is Issue) {
	ui.Scroll(c).MaxHeight(previewMaxH).Shrink(0).Radius(12).Background(previewFill).Children(func() {
		ui.Text(c, is.Preview()).Font("Menlo").FontSize(12).FixedLineHeight(18).TextColor(t.Text.Alpha(0.8)).Wrap().Selectable().Padding(10, 12)
	})
	if is.Truncated {
		breeze.Text(c, "正文太长，链接里截掉了最后 "+strconv.Itoa(is.Cut)+" 个字。用「拷贝正文」拷贝完整内容，在 GitHub 页面里粘贴替换。", breeze.TextCaption).FontSize(11).TextColor(t.Warning).Wrap().Padding(0, 4)
	}
}

// footer is where the issue goes and the dialog's buttons. 在 GitHub 中打开 waits for a title.
func footer(c *ui.Context, t *ui.Theme, a App, form *Form, view View, open *bool) {
	if view.Err != "" {
		ui.Text(c, view.Err).FontSize(12).TextColor(t.Danger).Wrap()
	}
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		breeze.Text(c, strings.TrimPrefix(NewIssue, "https://"), breeze.TextCaption, breeze.TextSecondary).Font("Menlo").FontSize(11).Grow(1).Shrink(1).MinWidth(0).SingleLine()
		c.Key("feedback-cancel")
		if breeze.Button(c, "取消").Clicked() {
			*open = false
		}
		c.Key("feedback-copy")
		copyLabel := "拷贝正文"
		if view.Copied {
			copyLabel = "已拷贝"
		}
		if breeze.Button(c, copyLabel).Label("拷贝正文").Clicked() {
			c.WriteClipboard(view.Issue.Full)
			a.FeedbackCopied()
		}
		c.Key("feedback-send")
		ready := strings.TrimSpace(form.Title) != "" && !view.Sending
		if breeze.PrimaryButton(c, "在 GitHub 中打开").Disabled(!ready).Clicked() && ready {
			a.SendFeedback()
		}
	})
}
