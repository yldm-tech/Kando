package feedback

import (
	"strings"

	"kando/apps/kando/locales"

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
					breeze.Segmented(c, &form.Kind, kindLabels()...).AlignSelf(ui.Start).Label(locales.FeedbackKindsLabel())
					fields(c, form)
					section(c, locales.FeedbackAttachments())
					breeze.Card(c, func() {
						attachment(c, "feedback-env", &form.Env, locales.FeedbackAttachEnv(), view.Env)
						attachment(c, "feedback-tools", &form.Tools, locales.FeedbackAttachTools(), view.Tools)
						attachment(c, "feedback-film", &form.Film, locales.FeedbackAttachFilm(), view.Film)
						attachment(c, "feedback-subs", &form.Subs, locales.FeedbackAttachSubs(), view.Subs)
					})
					section(c, locales.FeedbackPreview())
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
			breeze.Text(c, locales.FeedbackTitle(), breeze.TextHeading).FontSize(15).Bold().Role(ui.RoleHeading).Level(1)
			breeze.Text(c, locales.FeedbackIntro(), breeze.TextCaption, breeze.TextSecondary).FontSize(12).FixedLineHeight(17).Wrap()
		})
	})
}

// fields are the title and the description, in a card. The dialog opens with the keyboard in the title, or in the description when it opens with a title already, as for a film's subtitles.
func fields(c *ui.Context, form *Form) {
	titled := strings.TrimSpace(form.Title) != ""
	breeze.Card(c, func() {
		breeze.ContentRow(c, locales.FeedbackFieldTitle(), func() {
			c.Key("feedback-title")
			title := breeze.TextInput(c, &form.Title).Label(locales.FeedbackFieldTitle()).Placeholder(titleHint(Kind(form.Kind)))
			if !titled {
				title.AutoFocus()
			}
		})
		breeze.ContentRow(c, locales.FeedbackFieldDescription(), func() {
			c.Key("feedback-description")
			description := breeze.TextArea(c, &form.Description).Label(locales.FeedbackFieldDescription()).Placeholder(descriptionHint(Kind(form.Kind))).Height(92)
			if titled {
				description.AutoFocus()
			}
		})
	})
}

// kindLabels are the segmented control's labels, by Kind, in the interface's language. The issue names the kind with Kind.Name, which stays in Chinese.
func kindLabels() []string {
	return []string{locales.FeedbackKindBug(), locales.FeedbackKindIdea(), locales.FeedbackKindSubs()}
}

func titleHint(k Kind) string {
	if k == KindIdea {
		return locales.FeedbackTitleHintIdea()
	}
	return locales.FeedbackTitleHint()
}

func descriptionHint(k Kind) string {
	switch k {
	case KindIdea:
		return locales.FeedbackDescriptionHintIdea()
	case KindSubs:
		return locales.FeedbackDescriptionHintSubs()
	}
	return locales.FeedbackDescriptionHint()
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
		breeze.Text(c, locales.FeedbackTruncated(is.Cut), breeze.TextCaption).FontSize(11).TextColor(t.Warning).Wrap().Padding(0, 4)
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
		if breeze.Button(c, locales.FeedbackCancel()).Clicked() {
			*open = false
		}
		c.Key("feedback-copy")
		copyLabel := locales.FeedbackCopy()
		if view.Copied {
			copyLabel = locales.FeedbackCopied()
		}
		if breeze.Button(c, copyLabel).Label(locales.FeedbackCopy()).Clicked() {
			c.WriteClipboard(view.Issue.Full)
			a.FeedbackCopied()
		}
		c.Key("feedback-send")
		ready := strings.TrimSpace(form.Title) != "" && !view.Sending
		if breeze.PrimaryButton(c, locales.FeedbackOpen()).Disabled(!ready).Clicked() && ready {
			a.SendFeedback()
		}
	})
}
