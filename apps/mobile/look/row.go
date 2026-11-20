package look

import (
	"kando/apps/mobile/theme"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Tone is the color a row's text or a tile takes: the slot's own (the text's for a title, the muted text's for a detail), the accent, danger's, the muted text's, or the warning's.
type Tone uint8

const (
	ToneDefault Tone = iota
	ToneAccent
	ToneDanger
	ToneMuted
	ToneWarn
)

// color is t's color, fallback for ToneDefault.
func (t Tone) color(colors ui.Colors, warnText, fallback func() core.Color) core.Color {
	switch t {
	case ToneAccent:
		return colors.Accent()
	case ToneDanger:
		return colors.Danger()
	case ToneMuted:
		return colors.Muted()
	case ToneWarn:
		return warnText()
	}
	return fallback()
}

// Row is a row of a list in a Group: a press target the width of the card with a leading view, a title, a note under it and a view below them, a value and a trailing view at its end, and a chevron where it leads to a page; each is optional. A row after the first draws the hairline over it, the card's width unless Inset says where it starts.
type Row struct {
	Leading   core.Node
	Title     func() string
	TitleTone Tone
	// TitleLines is how many lines the title takes at most, 2 when 0.
	TitleLines int
	// TitleStyle is set over the title's 16-point Body, as a film's row sets its 15-point semibold one.
	TitleStyle core.Style
	Detail     func() string
	// DetailTone is the detail's color, muted when nil.
	DetailTone func() Tone
	Below      core.Node
	// Value is a short value at the row's end, as a count.
	Value    func() string
	Trailing core.Node
	Chevron  bool
	// Index is the row's place in its group: rows after the first draw the hairline over them.
	Index func() int
	// Inset is where the hairline starts from the card's leading edge, at the edge when 0.
	Inset core.Points
	// Checked makes the row a radio button of its group, checked while it reports true.
	Checked func() bool
	// Padding is the room around the row's content, 8 by 16 when nil.
	Padding *core.Edges
	OnPress func()
	TestID  string
}

// Render implements core.Node.
func (r Row) Render(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	warnText := theme.Of(cx).WarnText
	var texts []core.Node
	if r.Title != nil {
		lines := r.TitleLines
		if lines == 0 {
			lines = 2
		}
		texts = append(texts, core.Text{ContentFunc: r.Title, MaxLines: lines, Style: Body.Merge(r.TitleStyle),
			StyleFunc: func() core.Style { return core.Style{Color: r.TitleTone.color(colors, warnText, colors.Text)} }})
	}
	if r.Detail != nil {
		texts = append(texts, core.Show{When: func() bool { return r.Detail() != "" }, Then: func(cx *core.Scope) core.Node {
			return core.Text{ContentFunc: r.Detail, MaxLines: 2, Style: Footnote, StyleFunc: func() core.Style {
				tone := ToneDefault
				if r.DetailTone != nil {
					tone = r.DetailTone()
				}
				return core.Style{Color: tone.color(colors, warnText, colors.Muted)}
			}}
		}})
	}
	if r.Below != nil {
		texts = append(texts, r.Below)
	}
	var children []core.Node
	if r.Index != nil {
		children = append(children, core.View{Style: core.Style{Position: core.Absolute, Top: core.Pt(0), Left: core.Pt(r.Inset), Right: core.Pt(0), Height: core.Pt(0.5)},
			StyleFunc: func() core.Style {
				s := core.Style{Background: colors.Line()}
				if r.Index() <= 0 {
					s.Display = core.DisplayNone
				}
				return s
			}})
	}
	if r.Leading != nil {
		children = append(children, r.Leading)
	}
	children = append(children, core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: texts})
	if r.Value != nil {
		children = append(children, core.Text{ContentFunc: r.Value, MaxLines: 1, Style: Subhead, StyleFunc: ui.Colored(colors.Muted)})
	}
	if r.Trailing != nil {
		children = append(children, r.Trailing)
	}
	if r.Chevron {
		// The chevron is the same light grey in both appearances, as the system's is.
		children = append(children, core.Icon{SVG: Chevron(), Style: core.Style{Width: core.Pt(8), Height: core.Pt(13), FlexShrink: 0, Color: theme.Chevron}})
	}
	pad := core.Symmetric(8, 16)
	if r.Padding != nil {
		pad = *r.Padding
	}
	v := core.View{TestID: r.TestID, Children: children,
		Style:     core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Padding: pad, MinHeight: core.Pt(52)},
		StyleFunc: func() core.Style { return core.Style{Active: &core.Style{Background: colors.Pressed()}} }}
	if r.OnPress != nil {
		v.Role = core.RoleButton
		v.OnPress = func(core.PressEvent) { r.OnPress() }
	}
	if r.Checked != nil {
		v.Role, v.Checked = core.RoleRadio, r.Checked
	}
	return v
}
