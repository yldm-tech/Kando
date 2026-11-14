// Package form draws the pieces the settings window's pages are laid out with, as the prototype's settings sheet draws them: section labels, white cards of rows on the window's gray, separators and footnotes. The controls inside the rows are NSGo's (package ui). The settings pages and the 账号与上传 page in package account both use it, so it imports neither.
//
// A page is built once and again when the interface's language or its category changes (settings.Window), so the words a piece takes as a plain string are read as it is built; what changes with the app's state between those builds, such as a version or a path, the pieces take as a func, which a binding reads.
package form

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// The prototype's settings palette.
var (
	// Window is the settings window's background, under the header and the pages.
	Window = core.RGB(0xF5F5F7)
	// Text is the primary text, Secondary the subtitles, footnotes, section labels and trailing values.
	Text      = core.RGB(0x1D1D1F)
	Secondary = core.RGB(0x6A6A70)
	// AccentText is the blue of tinted buttons, badges and chips, SuccessText the green of a working connection.
	AccentText  = core.RGB(0x0052B0)
	SuccessText = core.RGB(0x1B6B30)
	// Line is the hairline between rows and under the header.
	Line = core.RGBA(0x000000, 0.08)
	// OK and Idle are the status dots: a tool found or a connection that works, and one not tried.
	OK   = core.RGB(0x30D158)
	Idle = core.RGB(0xC7C7CC)
	// Tint is the fill of a badge or a chip in the accent.
	Tint = core.RGBA(0x0A66D8, 0.12)
	// card is a card's fill, white.
	card = core.RGB(0xFFFFFF)
)

// Page is the column a category's page is built in: the content's width, its blocks 6 points apart.
func Page(children ...core.Node) core.Node {
	return core.View{Style: core.Style{Width: core.Pct(100), Gap: 6, AlignItems: core.AlignStretch}, Children: children}
}

// Label is a section's label above its card: 11-point bold in the secondary color, 12 points in. top is the room above it: 6 for a page's first label, 12 after a card. suffix, when given, is a control at the end of its line, such as 重新检测.
func Label(text string, top core.Points, suffix ...core.Node) core.Node {
	title := core.Text{Content: text, Role: core.RoleHeader,
		Style: core.Style{FontSize: 11, LineHeight: 16, FontWeight: ui.Bold, Color: Secondary, FlexGrow: 1, FlexShrink: 1}}
	children := []core.Node{title}
	if len(suffix) > 0 {
		children = append(children, core.View{Style: core.Style{Direction: core.Row, FlexShrink: 0, AlignItems: core.AlignCenter}, Children: suffix})
	}
	return core.View{Style: core.Style{Direction: core.Row, FlexShrink: 0, AlignItems: core.AlignCenter, Padding: core.Edges{Top: top, Right: 12, Left: 12}},
		Children: children}
}

// Card is a white card of rows, with corners of 12 and a hairline ring. It is a View, so a page can give it a margin.
func Card(rows ...core.Node) core.View {
	return core.View{Style: core.Style{FlexShrink: 0, AlignItems: core.AlignStretch, Background: card, CornerRadius: 12,
		Shadows: []core.BoxShadow{{Spread: 0.5, Color: core.RGBA(0x000000, 0.07)}}}, Children: rows}
}

// Separator is the hairline between two rows of a card. inset starts it 14 points in, under the rows' text; otherwise it crosses the card, as between the items of a list.
func Separator(inset bool) core.Node {
	// A line of one point: at the prototype's 0.5 a line between pixels on a 1x display fades away.
	style := core.Style{Height: core.Pt(1), FlexShrink: 0, Background: Line}
	if inset {
		style.Margin = core.Edges{Left: 14}
	}
	return core.View{Style: style}
}

// Row is a row of a card: the title over the subtitle on the left, Trailing at the right end. It is 40 points high with one line and 46 with a subtitle. Assistive technology sees a group named by the title and described by the subtitle. Subtitle is a subtitle that stays while the page shows; SubtitleFunc one that changes with the app's state, which shows while it is not "".
type Row struct {
	Title, Subtitle string
	SubtitleFunc    func() string
	Trailing        []core.Node
	// NoGroup leaves the row unnamed, for a row whose control assistive technology already reads by the title.
	NoGroup bool
	TestID  string
}

// Render implements core.Node.
func (r Row) Render(*core.Scope) core.Node {
	children := []core.Node{Titles{Title: r.Title, Subtitle: r.Subtitle, SubtitleFunc: r.SubtitleFunc}}
	if len(r.Trailing) > 0 {
		children = append(children, core.View{Style: core.Style{Direction: core.Row, FlexShrink: 0, AlignItems: core.AlignCenter, Gap: 8}, Children: r.Trailing})
	}
	row := core.View{TestID: r.TestID, Style: core.Style{Direction: core.Row, FlexShrink: 0, MinHeight: core.Pt(40), Padding: core.Symmetric(6, 14), Gap: 10, AlignItems: core.AlignCenter},
		Children: children}
	if !r.NoGroup {
		// TODO(nsgo): give core.View an AccessibilityDescriptionFunc twin, so a row whose subtitle changes with the app's state (SubtitleFunc) is described by it as it changes, as MyGo's Description was each frame; until then assistive technology reads that subtitle as the row's text only
		row.Role, row.Label, row.AccessibilityDescription = core.RoleGroup, r.Title, r.Subtitle
	}
	return row
}

// Titles is a row's title over its subtitle, taking the room the row leaves.
type Titles struct {
	Title, Subtitle string
	SubtitleFunc    func() string
}

// Render implements core.Node.
func (t Titles) Render(*core.Scope) core.Node {
	children := []core.Node{core.Text{Content: t.Title, Style: core.Style{FontSize: 13, LineHeight: 18, Color: Text}}}
	subtitle := func(text func() string) core.Node {
		return core.Text{ContentFunc: text, Style: core.Style{FontSize: 11, LineHeight: 16, Color: Secondary}}
	}
	switch {
	case t.SubtitleFunc != nil:
		children = append(children, core.Show{When: func() bool { return t.SubtitleFunc() != "" }, Then: func(*core.Scope) core.Node { return subtitle(t.SubtitleFunc) }})
	case t.Subtitle != "":
		children = append(children, subtitle(func() string { return t.Subtitle }))
	}
	return core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, Children: children}
}

// Value is a row's trailing value, such as a version or a path, in the secondary color, on one line, which the user can select and copy.
func Value(text func() string, size core.Points) core.Text {
	return core.Text{ContentFunc: text, MaxLines: 1, EllipsizeMode: core.EllipsizeMiddle, Selectable: core.SelectableOn,
		Style: core.Style{FontSize: size, LineHeight: size + 5, Color: Secondary, FlexShrink: 1, MinWidth: core.Pt(0)}}
}

// Note is a footnote under a card: 11 points in the secondary color, 12 in from the card's edges, with bottom points below it before what follows.
func Note(text func() string, bottom core.Points) core.Node {
	return core.Text{ContentFunc: text, Style: core.Style{FontSize: 11, LineHeight: 15, Color: Secondary, Padding: core.Edges{Right: 12, Bottom: bottom, Left: 12}}}
}

// Dot is a status's dot, 7 points across, before its word, in the color color returns.
func Dot(color func() core.Color) core.Node {
	return core.View{Style: core.Style{Width: core.Pt(7), Height: core.Pt(7), CornerRadius: 3.5, FlexShrink: 0}, StyleFunc: ui.Filled(color)}
}

// Status is a dot and a word after it, as a tool's version or a connection's state; the word can be selected and copied, as an error it says.
func Status(dot, color func() core.Color, text func() string) core.Node {
	word := core.Text{ContentFunc: text, Selectable: core.SelectableOn, Style: core.Style{FontSize: 12, LineHeight: 17, MaxWidth: core.Pt(240), FlexShrink: 1}, StyleFunc: ui.Colored(color)}
	return core.View{Style: core.Style{Direction: core.Row, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 6, AlignItems: core.AlignCenter}, Children: []core.Node{Dot(dot), word}}
}

// Badge is a small tag in the accent, as 推荐 after a title.
func Badge(text string) core.Node {
	return core.Text{Content: text, MaxLines: 1, Role: core.RoleNone,
		Style: core.Style{FontSize: 10, FontWeight: ui.Bold, LineHeight: 16, Color: AccentText, Background: Tint, CornerRadius: 8, Padding: core.Symmetric(0, 7), FlexShrink: 0}}
}

// SwitchRow is a row of a card with a switch at its end: the title over the subtitle, padded 8 points above and below and 12 before the switch. The switch is named by the title, and a press on the title or the subtitle turns it too, as on a label of a check box. On reads the switch's state and OnChange hears it turned.
type SwitchRow struct {
	Title, Subtitle string
	On              func() bool
	OnChange        func(on bool)
	TestID          string
}

// Render implements core.Node.
func (r SwitchRow) Render(*core.Scope) core.Node {
	texts := []core.Node{core.Text{Content: r.Title, Style: core.Style{FontSize: 13, LineHeight: 18, Color: Text}}}
	if r.Subtitle != "" {
		texts = append(texts, core.Text{Content: r.Subtitle, Style: core.Style{FontSize: 11, LineHeight: 15, Color: Secondary}})
	}
	// The labels are pressed with the pointer only: the switch is the control the keyboard reaches and assistive technology reads.
	labels := core.View{NoFocus: true, OnPress: func(core.PressEvent) { r.OnChange(!r.On()) },
		Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: texts}
	toggle := ui.Switch{Checked: r.On, OnChange: r.OnChange, Label: r.Title, TestID: r.TestID}
	return core.View{Style: core.Style{Direction: core.Row, FlexShrink: 0, MinHeight: core.Pt(48), Padding: core.Symmetric(8, 14), Gap: 12, AlignItems: core.AlignCenter},
		Children: []core.Node{labels, toggle}}
}

// Button is NSGo's push button at the prototype's size for a button in a row, 24 points high with a 12-point label as 重新检测 and 恢复默认 are. Text is its label; TextFunc, when set, a label that changes with the app's state, as 下载 turns to 重试. Label names it for assistive technology when it says something else than its label, as 已拷贝 does.
type Button struct {
	Text     string
	TextFunc func() string
	Label    string
	Disabled func() bool
	OnPress  func()
	TestID   string
}

// Render implements core.Node.
func (b Button) Render(cx *core.Scope) core.Node {
	button := func(text string) ui.Button {
		return ui.Button{Text: text, Label: b.Label, Variant: ui.ButtonSecondary, Size: ui.SizeSmall, Disabled: b.Disabled, OnPress: b.OnPress, TestID: b.TestID}
	}
	if b.TextFunc == nil {
		return button(b.Text)
	}
	text := core.NewMemo(cx, b.TextFunc)
	// TODO(nsgo): give ui.Button a TextFunc twin, so a label that changes with the app's state, as 拷贝配置 turning to 已拷贝, does not build the button again, which takes the keyboard focus off it
	return core.Dynamic(func(*core.Scope) core.Node { return button(text.Get()) })
}
