package feedback

import (
	"strings"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
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

// App is the feedback dialog's state and the clicks it reports. The dialog reads it inside its bindings through live.Func and calls its actions through live.Do and live.On.
type App interface {
	live.State
	// FeedbackOpen reports whether the dialog shows.
	FeedbackOpen() bool
	FeedbackForm() *Form
	// FeedbackView is what the dialog shows for the form now, its words in the language the app shows under cx.
	FeedbackView(cx *core.Scope) View
	// FeedbackCopied is told that 拷贝正文 put the body on the clipboard.
	FeedbackCopied()
	// SendFeedback opens the issue in the browser and closes the dialog once it did.
	SendFeedback()
	// CloseFeedback closes the dialog: 取消, Escape.
	CloseFeedback()
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
	tile = core.RGB(0x24292F)
	// previewFill is the preview's well, the prototype's #F2F2F5 over the dialog.
	previewFill = core.RGBA(0x000000, 0.045)
)

// Dialog shows the dialog over the window while the app's FeedbackOpen holds. A click outside it does not close it, so nothing typed is lost; Escape and 取消 close it, and 在 GitHub 中打开 asks the app to open the issue, which closes it once the browser has the link.
type Dialog struct{ App App }

// Render implements core.Node.
func (d Dialog) Render(cx *core.Scope) core.Node {
	a := d.App
	// TODO(nsgo): give ui.Dialog (base.Dialog) a LabelFunc so the dialog's accessible name follows the interface language as it changes; it is read once as the dialog is built
	return ui.Dialog{
		TestID:                 "feedback",
		Label:                  locales.FeedbackTitle(cx),
		Open:                   live.Func(a, a.FeedbackOpen),
		NoCloseOnBackdropPress: true,
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				a.CloseFeedback()
				a.Changed()
			}
		},
		Style:    core.Style{Width: core.Pt(width)},
		Children: []core.Node{content{a}},
	}
}

// content is what the dialog holds, built while it is open.
type content struct{ a App }

// Render implements core.Node.
func (c content) Render(cx *core.Scope) core.Node {
	a := c.a
	// FeedbackView follows the app's whole revision, which changes on every keystroke and progress report; the memo lets the dialog follow only what it shows.
	view := live.Memo(cx, a, func() View { return a.FeedbackView(cx) })
	form := a.FeedbackForm()
	// Escape closes the dialog wherever the keyboard focus is in it, before the window's own Escape, which would close the open film behind it.
	cx.OnKeyWith("escape", core.KeyOptions{When: core.KeyAlways}, live.Do(a, a.CloseFeedback))
	return core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0), Gap: 12, AlignItems: core.AlignStretch}, Children: []core.Node{
		header(cx),
		core.Scroll{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)}, Children: []core.Node{
			core.View{Style: core.Style{Gap: 12, AlignItems: core.AlignStretch, Padding: core.Symmetric(0, 2)}, Children: []core.Node{
				kinds(cx, a, form),
				fields(cx, a, form),
				section(cx, func() string { return locales.FeedbackAttachments(cx) }),
				card(cx,
					attachment(cx, a, "feedback-env", &form.Env, func() string { return locales.FeedbackAttachEnv(cx) }, func() Attachment { return view().Env }),
					attachment(cx, a, "feedback-tools", &form.Tools, func() string { return locales.FeedbackAttachTools(cx) }, func() Attachment { return view().Tools }),
					attachment(cx, a, "feedback-film", &form.Film, func() string { return locales.FeedbackAttachFilm(cx) }, func() Attachment { return view().Film }),
					attachment(cx, a, "feedback-subs", &form.Subs, func() string { return locales.FeedbackAttachSubs(cx) }, func() Attachment { return view().Subs }),
				),
				section(cx, func() string { return locales.FeedbackPreview(cx) }),
				preview(cx, view),
			}},
		}},
		footer(cx, a, form, view),
	}}
}

// header is the GitHub tile, the title and what will happen.
func header(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{Style: core.Style{Direction: core.Row, Gap: 12, AlignItems: core.AlignStart}, Children: []core.Node{
		core.View{Style: core.Style{Width: core.Pt(tileSize), Height: core.Pt(tileSize), FlexShrink: 0, CornerRadius: tileRadius, Background: tile,
			JustifyContent: core.JustifyCenter, AlignItems: core.AlignCenter},
			Children: []core.Node{core.Icon{SVG: iconGitHub, Style: core.Style{Width: core.Pt(20), Height: core.Pt(20)}}}},
		core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: []core.Node{
			// The title is the dialog's heading of level 1, as MyGo's Level(1) made it; core.Text carries no level, so a view around it is the heading and the text inside it is read through it.
			core.View{TestID: "feedback-heading", Role: core.RoleHeader, AccessibilityLevel: headingLevel, LabelFunc: func() string { return locales.FeedbackTitle(cx) }, Children: []core.Node{
				core.Text{ContentFunc: func() string { return locales.FeedbackTitle(cx) }, Role: core.RoleNone,
					Style: core.Style{FontSize: 15, FontWeight: ui.Bold}, StyleFunc: ui.Colored(colors.Text)},
			}},
			core.Text{ContentFunc: func() string { return locales.FeedbackIntro(cx) }, Style: core.Style{FontSize: 12, LineHeight: 17}, StyleFunc: ui.Colored(colors.Muted)},
		}},
	}}
}

// headingLevel is the title's level among the dialog's headings.
func headingLevel() int { return 1 }

// titled builds a control whose title NSGo's ui controls take once, again whenever title changes, which only another language or another state of the control does: the control holds no text being typed, so building it again loses nothing but the keyboard focus on it.
func titled(cx *core.Scope, title func() string, build func(title string) core.Node) core.Node {
	// TODO(nsgo): give ui.Button and ui.Tabs TextFunc and LabelFunc twins, so their titles follow the language and the state without building the control again, then drop this Dynamic
	memo := core.NewMemo(cx, title)
	return core.Dynamic(func(*core.Scope) core.Node { return build(memo.Get()) })
}

// kinds is the segmented control of the feedback's kind, by Kind, its labels in the interface's language. The issue names the kind with Kind.Name, which stays in Chinese.
func kinds(cx *core.Scope, a App, form *Form) core.Node {
	labels := func() string {
		return strings.Join([]string{locales.FeedbackKindsLabel(cx), locales.FeedbackKindBug(cx), locales.FeedbackKindIdea(cx), locales.FeedbackKindSubs(cx)}, "\x00")
	}
	return core.View{TestID: "feedback-kind", Style: core.Style{AlignSelf: core.AlignStart}, Children: []core.Node{titled(cx, labels, func(joined string) core.Node {
		l := strings.Split(joined, "\x00")
		return ui.Tabs[int]{
			Label:    l[0],
			Options:  []ui.Option[int]{{Value: int(KindBug), Label: l[1]}, {Value: int(KindIdea), Label: l[2]}, {Value: int(KindSubs), Label: l[3]}},
			Value:    live.Func(a, func() int { return form.Kind }),
			OnChange: live.On(a, func(kind int) { form.Kind = kind }),
		}
	})}}
}

// card is a white card of rows, a hairline between them.
func card(cx *core.Scope, rows ...core.Node) core.Node {
	colors := ui.ColorsOf(cx)
	var children []core.Node
	for i, row := range rows {
		if i > 0 {
			children = append(children, core.View{Style: core.Style{Height: core.Pt(1), Margin: core.Edges{Left: 12}}, StyleFunc: func() core.Style { return core.Style{Background: colors.Line()} }})
		}
		children = append(children, row)
	}
	return core.View{Style: core.Style{AlignItems: core.AlignStretch, Overflow: core.Hidden}, StyleFunc: func() core.Style { return ui.CardStyle(cx) }, Children: children}
}

// fields are the title and the description, in a card. The dialog opens with the keyboard in the title, or in the description when it opens with a title already, as for a film's subtitles.
func fields(cx *core.Scope, a App, form *Form) core.Node {
	hasTitle := strings.TrimSpace(form.Title) != ""
	kind := live.Func(a, func() Kind { return Kind(form.Kind) })
	// TODO(nsgo): give core.Input a LabelFunc twin so the fields' accessible names follow the interface language as it changes; they are read once as the dialog opens
	title := ui.Input{
		TestID:          "feedback-title",
		Label:           locales.FeedbackFieldTitle(cx),
		AutoFocus:       !hasTitle,
		Value:           live.Func(a, func() string { return form.Title }),
		PlaceholderFunc: func() string { return titleHint(cx, kind()) },
		OnChange:        live.On(a, func(text string) { form.Title = text }),
		Style:           core.Style{FlexGrow: 1, MinWidth: core.Pt(0)},
	}
	description := ui.Input{
		TestID:          "feedback-description",
		Label:           locales.FeedbackFieldDescription(cx),
		AutoFocus:       hasTitle,
		Multiline:       true,
		Value:           live.Func(a, func() string { return form.Description }),
		PlaceholderFunc: func() string { return descriptionHint(cx, kind()) },
		OnChange:        live.On(a, func(text string) { form.Description = text }),
		Style:           core.Style{FlexGrow: 1, MinWidth: core.Pt(0), Height: core.Pt(92)},
	}
	return card(cx,
		fieldRow(cx, func() string { return locales.FeedbackFieldTitle(cx) }, core.AlignCenter, title),
		fieldRow(cx, func() string { return locales.FeedbackFieldDescription(cx) }, core.AlignStart, description),
	)
}

// fieldRow is a row of the fields' card: its label, then its field.
func fieldRow(cx *core.Scope, label func() string, align core.Align, field core.Node) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{Style: core.Style{Direction: core.Row, Gap: 12, AlignItems: align, Padding: core.Symmetric(8, 12)}, Children: []core.Node{
		core.Text{ContentFunc: label, Style: core.Style{FontSize: 13, Width: core.Pt(72), FlexShrink: 0, Padding: core.Edges{Top: alignTop(align)}}, StyleFunc: ui.Colored(colors.Text)},
		field,
	}}
}

// alignTop moves a label at the top of its row down to the first line of a field.
func alignTop(align core.Align) core.Points {
	if align == core.AlignStart {
		return 6
	}
	return 0
}

func titleHint(cx *core.Scope, k Kind) string {
	if k == KindIdea {
		return locales.FeedbackTitleHintIdea(cx)
	}
	return locales.FeedbackTitleHint(cx)
}

func descriptionHint(cx *core.Scope, k Kind) string {
	switch k {
	case KindIdea:
		return locales.FeedbackDescriptionHintIdea(cx)
	case KindSubs:
		return locales.FeedbackDescriptionHintSubs(cx)
	}
	return locales.FeedbackDescriptionHint(cx)
}

// section is a label over a group, 11 bold in the secondary color.
func section(cx *core.Scope, label func() string) core.Node {
	colors := ui.ColorsOf(cx)
	return core.Text{ContentFunc: label, Style: core.Style{FontSize: 11, FontWeight: ui.Bold, Padding: core.Symmetric(0, 4)}, StyleFunc: ui.Colored(colors.Muted)}
}

// attachment is a row of 附带信息: a check box, the label and what it attaches. A click anywhere on the row toggles it, as a click on a check box's title does. A row that has nothing to attach now is unticked and cannot be ticked.
func attachment(cx *core.Scope, a App, id string, on *bool, label func() string, at func() Attachment) core.Node {
	colors := ui.ColorsOf(cx)
	return ui.Checkbox{
		TestID:   id,
		Checked:  live.Func(a, func() bool { return *on && at().Enabled }),
		Disabled: func() bool { return !at().Enabled },
		OnChange: live.On(a, func(checked bool) { *on = checked }),
		Style:    core.Style{Padding: core.Symmetric(9, 12), AlignItems: core.AlignCenter},
		Children: []core.Node{core.View{Style: core.Style{FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: []core.Node{
			core.Text{ContentFunc: label, Style: core.Style{FontSize: 13}, StyleFunc: ui.Colored(colors.Text)},
			core.Text{ContentFunc: func() string { return at().Detail }, Style: core.Style{FontSize: 12}, StyleFunc: ui.Colored(colors.Muted)},
		}}},
	}
}

// preview is the issue as it will be sent, in a scrolling monospaced well, and under it why it was cut when it was.
func preview(cx *core.Scope, view func() View) core.Node {
	colors := ui.ColorsOf(cx)
	return core.Group{
		core.Scroll{Style: core.Style{MaxHeight: core.Pt(previewMaxH), FlexShrink: 0, CornerRadius: 12, Background: previewFill}, Children: []core.Node{
			core.Text{TestID: "feedback-preview", ContentFunc: func() string { return view().Issue.Preview(locales.For(cx)) }, Selectable: core.SelectableOn,
				Style:     core.Style{FontFamily: ui.Mono, FontSize: 12, LineHeight: 18, Padding: core.Symmetric(10, 12)},
				StyleFunc: func() core.Style { return core.Style{Color: colors.Text().Faded(0.8)} }},
		}},
		core.Show{When: func() bool { return view().Issue.Truncated }, Then: func(cx *core.Scope) core.Node {
			return core.Text{ContentFunc: func() string { return locales.FeedbackTruncated(cx, view().Issue.Cut) }, Style: core.Style{FontSize: 11, Padding: core.Symmetric(0, 4)},
				StyleFunc: ui.Colored(colors.Warn)}
		}},
	}
}

// footer is why the browser did not open, where the issue goes and the dialog's buttons. 在 GitHub 中打开 waits for a title.
func footer(cx *core.Scope, a App, form *Form, view func() View) core.Node {
	colors := ui.ColorsOf(cx)
	ready := live.Func(a, func() bool { return strings.TrimSpace(form.Title) != "" && !view().Sending })
	copyTitle := func() string {
		if view().Copied {
			return locales.FeedbackCopied(cx)
		}
		return locales.FeedbackCopy(cx)
	}
	return core.View{Style: core.Style{Gap: 8, AlignItems: core.AlignStretch}, Children: []core.Node{
		core.Show{When: func() bool { return view().Err != "" }, Then: func(*core.Scope) core.Node {
			return core.Text{ContentFunc: func() string { return view().Err }, Style: core.Style{FontSize: 12}, StyleFunc: ui.Colored(colors.Danger)}
		}},
		core.View{Style: core.Style{Direction: core.Row, Gap: 8, AlignItems: core.AlignCenter}, Children: []core.Node{
			core.Text{Content: strings.TrimPrefix(NewIssue, "https://"), MaxLines: 1, EllipsizeMode: core.EllipsizeTail,
				Style: core.Style{FontFamily: ui.Mono, FontSize: 11, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, StyleFunc: ui.Colored(colors.Muted)},
			titled(cx, func() string { return locales.FeedbackCancel(cx) }, func(title string) core.Node {
				return ui.Button{Text: title, Variant: ui.ButtonSecondary, TestID: "feedback-cancel", OnPress: live.Do(a, a.CloseFeedback)}
			}),
			titled(cx, func() string { return copyTitle() + "\x00" + locales.FeedbackCopy(cx) }, func(titles string) core.Node {
				title, label, _ := strings.Cut(titles, "\x00")
				return ui.Button{Text: title, Label: label, Variant: ui.ButtonSecondary, TestID: "feedback-copy", OnPress: live.Do(a, func() {
					cx.SetClipboard(core.Untracked(cx, view).Issue.Full)
					a.FeedbackCopied()
				})}
			}),
			titled(cx, func() string { return locales.FeedbackOpen(cx) }, func(title string) core.Node {
				return ui.Button{Text: title, TestID: "feedback-send", Disabled: func() bool { return !ready() }, OnPress: live.Do(a, func() {
					if core.Untracked(cx, ready) {
						a.SendFeedback()
					}
				})}
			}),
		}},
	}}
}
