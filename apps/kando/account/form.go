package account

import (
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// The pieces the page is laid out with, as the prototype's settings sheet draws them and the settings pages' form does: section labels, cards of rows on the window's ground, separators and footnotes, in the theme's colors (ui.ColorsOf and theme.Of), which follow the window's appearance.

// label is a section's label above its card: 11-point bold in the secondary color, 12 points in. top is the room above it: 6 for a page's first label, 12 after a card.
func label(text func() string, top core.Points) core.Node {
	return core.View{
		Role:               core.RoleHeader,
		AccessibilityLevel: func() int { return 2 },
		LabelFunc:          text,
		Style:              core.Style{FlexShrink: 0, Padding: core.Edges{Top: top, Right: 12, Left: 12}},
		Children: []core.Node{core.Func(func(cx *core.Scope) core.Node {
			return core.Text{ContentFunc: text, Style: core.Style{FontSize: 11, LineHeight: 16, FontWeight: ui.Bold}, StyleFunc: ui.Colored(ui.ColorsOf(cx).Muted)}
		})},
	}
}

// card is a card of rows, white on the light window and a step lighter than the dark one, with corners of 12 and a hairline ring.
func card(test string, rows ...core.Node) core.Node {
	return core.Func(func(cx *core.Scope) core.Node {
		k := theme.Of(cx)
		return core.View{
			TestID: test,
			Style:  core.Style{FlexShrink: 0, AlignItems: core.AlignStretch, CornerRadius: 12},
			StyleFunc: func() core.Style {
				return core.Style{Background: k.Card(), Shadows: []core.BoxShadow{{Spread: 0.5, Color: k.Shade(0.07)}}}
			},
			Children: rows,
		}
	})
}

// separator is the hairline between two rows of a card, starting 14 points in, under the rows' text. A line of one point: at the prototype's 0.5 a line between pixels on a 1x display fades away.
func separator() core.Node {
	return core.Func(func(cx *core.Scope) core.Node {
		k := theme.Of(cx)
		return core.View{Style: core.Style{Height: core.Pt(1), FlexShrink: 0, Margin: core.Edges{Left: 14}}, StyleFunc: func() core.Style { return core.Style{Background: k.Shade(0.08)} }}
	})
}

// row is a row of a card: the title over the subtitle (none when it says "") on the left, trailing at the right end. It is 40 points high with one line and 46 with a subtitle. Assistive technology sees a group named by the title and described by the subtitle.
func row(title, subtitle func() string, trailing core.Node) core.Node {
	return core.View{
		Role:                         core.RoleGroup,
		LabelFunc:                    title,
		AccessibilityDescriptionFunc: subtitle,
		Style: core.Style{Direction: core.Row, FlexShrink: 0, MinHeight: core.Pt(40), Padding: core.Symmetric(6, 14), Gap: 10,
			AlignItems: core.AlignCenter},
		Children: []core.Node{
			titles(title, subtitle),
			core.View{Style: core.Style{Direction: core.Row, FlexShrink: 0, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{trailing}},
		},
	}
}

// titles is a row's title over its subtitle, taking the room the row leaves.
func titles(title, subtitle func() string) core.Node {
	return core.Func(func(cx *core.Scope) core.Node {
		colors := ui.ColorsOf(cx)
		return core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, Children: []core.Node{
			core.Text{ContentFunc: title, Style: core.Style{FontSize: 13, LineHeight: 18}, StyleFunc: ui.Colored(colors.Text)},
			core.Show{When: func() bool { return subtitle() != "" }, Then: func(*core.Scope) core.Node {
				return core.Text{ContentFunc: subtitle, Style: core.Style{FontSize: 11, LineHeight: 16}, StyleFunc: ui.Colored(colors.Muted)}
			}},
		}}
	})
}

// note is a footnote under a card: 11 points in the secondary color, 12 in from the card's edges, with bottom points below it before what follows.
func note(text func() string, bottom core.Points) core.Node {
	return core.Func(func(cx *core.Scope) core.Node {
		return core.Text{ContentFunc: text, Style: core.Style{FontSize: 11, LineHeight: 15, FlexShrink: 0, Padding: core.Edges{Right: 12, Bottom: bottom, Left: 12}},
			StyleFunc: ui.Colored(ui.ColorsOf(cx).Muted)}
	})
}

// none is a row's subtitle when it has none.
func none() string { return "" }

// field is a text field of a row, in the prototype's column of fields: 300 points wide and 26 high, in Face's type, with ui.Input's rim, darker under the pointer and the accent while it has the keyboard. ui.Input keeps to its own 28 points and 13-point type, so the field is drawn on base.Input.
type field struct {
	Input base.Input
	Face  core.Style
}

// Render implements core.Node.
func (f field) Render(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	in := f.Input
	focused := core.NewSignal(cx, false)
	in.OnFocus = func() { focused.Set(true) }
	in.OnBlur = func() { focused.Set(false) }
	in.Style = core.Style{Width: core.Pt(300), Height: core.Pt(26), Padding: core.Symmetric(0, 10), BorderWidth: 1, CornerRadius: ui.RadiusControl}.Merge(f.Face)
	in.StyleFunc = func() core.Style {
		s := core.Style{Color: colors.Text(), Background: colors.Field(), BorderColor: colors.FieldBorder(),
			PlaceholderColor: colors.Faint(), SelectionColor: cx.Palette().Selection, CaretColor: colors.Accent(),
			Hover: &core.Style{BorderColor: ui.Mix(colors.FieldBorder(), colors.Text(), 0.25)}}
		if focused.Get() {
			s.BorderColor, s.Hover = colors.Accent(), nil
		}
		return s
	}
	return in
}
