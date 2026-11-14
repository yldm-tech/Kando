package account

import (
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// The pieces the page is laid out with, as the prototype's settings sheet draws them and the settings pages' form does: section labels, white cards of rows on the window's gray, separators and footnotes, in the prototype's settings palette.
var (
	// formText is the primary text, formSecondary the subtitles, footnotes and section labels.
	formText      = core.RGB(0x1D1D1F)
	formSecondary = core.RGB(0x6A6A70)
	// formLine is the hairline between rows, formRing the one around a card.
	formLine = core.RGBA(0x000000, 0.08)
	formRing = core.RGBA(0x000000, 0.07)
	formCard = core.RGB(0xFFFFFF)
	// formIdle is the gray of the signed-out avatar.
	formIdle = core.RGB(0xC7C7CC)
)

// label is a section's label above its card: 11-point bold in the secondary color, 12 points in. top is the room above it: 6 for a page's first label, 12 after a card.
func label(text func() string, top core.Points) core.Node {
	return core.View{
		Role:               core.RoleHeader,
		AccessibilityLevel: func() int { return 2 },
		LabelFunc:          text,
		Style:              core.Style{FlexShrink: 0, Padding: core.Edges{Top: top, Right: 12, Left: 12}},
		Children: []core.Node{core.Text{ContentFunc: text, Style: core.Style{FontSize: 11, LineHeight: 16, FontWeight: ui.Bold, Color: formSecondary}}},
	}
}

// card is a white card of rows, with corners of 12 and a hairline ring.
func card(test string, rows ...core.Node) core.Node {
	return core.View{
		TestID: test,
		Style: core.Style{FlexShrink: 0, AlignItems: core.AlignStretch, Background: formCard, CornerRadius: 12,
			Shadows: []core.BoxShadow{{Spread: 0.5, Color: formRing}}},
		Children: rows,
	}
}

// separator is the hairline between two rows of a card, starting 14 points in, under the rows' text. A line of one point: at the prototype's 0.5 a line between pixels on a 1x display fades away.
func separator() core.Node {
	return core.View{Style: core.Style{Height: core.Pt(1), FlexShrink: 0, Background: formLine, Margin: core.Edges{Left: 14}}}
}

// row is a row of a card: the title over the subtitle (none when it says "") on the left, trailing at the right end. It is 40 points high with one line and 46 with a subtitle. Assistive technology sees a group named by the title and described by the subtitle.
func row(title, subtitle func() string, trailing core.Node) core.Node {
	// TODO(nsgo): give core.View an AccessibilityDescriptionFunc twin, so the row's description follows the interface language as it changes, as LabelFunc does its name; it is read as the row is built
	return core.View{
		Role:                     core.RoleGroup,
		LabelFunc:                title,
		AccessibilityDescription: subtitle(),
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
	return core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, Children: []core.Node{
		core.Text{ContentFunc: title, Style: core.Style{FontSize: 13, LineHeight: 18, Color: formText}},
		core.Show{When: func() bool { return subtitle() != "" }, Then: func(*core.Scope) core.Node {
			return core.Text{ContentFunc: subtitle, Style: core.Style{FontSize: 11, LineHeight: 16, Color: formSecondary}}
		}},
	}}
}

// note is a footnote under a card: 11 points in the secondary color, 12 in from the card's edges, with bottom points below it before what follows.
func note(text func() string, bottom core.Points) core.Node {
	return core.Text{ContentFunc: text, Style: core.Style{FontSize: 11, LineHeight: 15, Color: formSecondary, FlexShrink: 0,
		Padding: core.Edges{Right: 12, Bottom: bottom, Left: 12}}}
}

// none is a row's subtitle when it has none.
func none() string { return "" }
