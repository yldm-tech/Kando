// Package form draws the pieces the settings window's pages are laid out with, as the prototype's settings sheet draws them: section labels, white cards of rows on the window's gray, separators and footnotes. The controls inside the rows are breeze's. The settings pages and the 账号与上传 page in package account both use it, so it imports neither.
package form

import (
	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// The prototype's settings palette.
var (
	// Window is the settings window's background, under the header and the pages.
	Window = ui.Hex("#F5F5F7")
	// Text is the primary text, Secondary the subtitles, footnotes, section labels and trailing values.
	Text      = ui.Hex("#1D1D1F")
	Secondary = ui.Hex("#6A6A70")
	// AccentText is the blue of tinted buttons, badges and chips, SuccessText the green of a working connection.
	AccentText  = ui.Hex("#0052B0")
	SuccessText = ui.Hex("#1B6B30")
	// Line is the hairline between rows and under the header.
	Line = ui.RGBA(0, 0, 0, 0.08)
	// OK and Idle are the status dots: a tool found or a connection that works, and one not tried.
	OK   = ui.Hex("#30D158")
	Idle = ui.Hex("#C7C7CC")
	// Tint is the fill of a badge or a chip in the accent.
	Tint = ui.RGBA(10, 102, 216, 0.12)
)

// Page is the column a category's page is built in: the content's width, its blocks 6 points apart.
func Page(c *ui.Context, fn func()) ui.Element {
	return ui.Column(c).WidthPercent(100).Gap(6).AlignItems(ui.Stretch).Children(fn)
}

// Label is a section's label above its card: 11-point bold in the secondary color, 12 points in. top is the room above it: 6 for a page's first label, 12 after a card. suffix, when not nil, builds a control at the end of its line, such as 重新检测.
func Label(c *ui.Context, text string, top float32, suffix func()) ui.Element {
	row := ui.Row(c).Shrink(0).AlignItems(ui.Center).Padding(top, 12, 0, 12)
	row.Children(func() {
		ui.Text(c, text).FontSize(11).FixedLineHeight(16).FontWeight(700).TextColor(Secondary).Grow(1).Role(ui.RoleHeading)
		if suffix != nil {
			ui.Row(c).Shrink(0).AlignItems(ui.Center).Children(suffix)
		}
	})
	return row
}

// Card is a white card of rows, with corners of 12 and a hairline ring.
func Card(c *ui.Context, fn func()) ui.Element {
	return ui.Column(c).Shrink(0).AlignItems(ui.Stretch).Background(ui.Hex("#FFFFFF")).Radius(12).Shadow(0, 0, 0, 0.5, ui.RGBA(0, 0, 0, 0.07)).Children(fn)
}

// Separator is the hairline between two rows of a card. inset starts it 14 points in, under the rows' text; otherwise it crosses the card, as between the items of a list.
func Separator(c *ui.Context, inset bool) {
	// A line of one point: at the prototype's 0.5 a line between pixels on a 1x display fades away.
	line := ui.Box(c).Height(1).Shrink(0).Background(Line)
	if inset {
		line.MarginStart(14)
	}
}

// Row is a row of a card: the title over the subtitle (none when empty) on the left, what trailing builds (nil for nothing) at the right end. It is 40 points high with one line and 46 with a subtitle. Assistive technology sees a group named by the title and described by the subtitle, as breeze's ActionRow.
func Row(c *ui.Context, title, subtitle string, trailing func()) ui.Element {
	row := ui.Row(c).Shrink(0).MinHeight(40).Padding(6, 14).Gap(10).AlignItems(ui.Center).Role(ui.RoleGroup).Label(title)
	if subtitle != "" {
		row.Description(subtitle)
	}
	row.Children(func() {
		Titles(c, title, subtitle)
		if trailing != nil {
			ui.Row(c).Shrink(0).AlignItems(ui.Center).Gap(8).Children(trailing)
		}
	})
	return row
}

// Titles is a row's title over its subtitle, taking the room the row leaves.
func Titles(c *ui.Context, title, subtitle string) ui.Element {
	return ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Children(func() {
		ui.Text(c, title).FontSize(13).FixedLineHeight(18).TextColor(Text).Wrap()
		if subtitle != "" {
			ui.Text(c, subtitle).FontSize(11).FixedLineHeight(16).TextColor(Secondary).Wrap()
		}
	})
}

// Value is a row's trailing value, such as a version or a path, in the secondary color.
func Value(c *ui.Context, text string, size float32) ui.Element {
	return ui.Text(c, text).FontSize(size).FixedLineHeight(size + 5).TextColor(Secondary).SingleLine()
}

// Note is a footnote under a card: 11 points in the secondary color, 12 in from the card's edges, with bottom points below it before what follows.
func Note(c *ui.Context, text string, bottom float32) ui.Element {
	return ui.Text(c, text).FontSize(11).FixedLineHeight(15).TextColor(Secondary).Wrap().Padding(0, 12, bottom, 12)
}

// Dot is a status's dot, 7 points across, before its word.
func Dot(c *ui.Context, color ui.Color) {
	ui.Box(c).Size(7, 7).Radius(3.5).Shrink(0).Background(color)
}

// Status is a dot and a word after it, as a tool's version or a connection's state.
func Status(c *ui.Context, dot, color ui.Color, text string) ui.Element {
	return ui.Row(c).Shrink(0).Gap(6).AlignItems(ui.Center).Children(func() {
		Dot(c, dot)
		ui.Text(c, text).FontSize(12).FixedLineHeight(17).TextColor(color).MaxWidth(240).Wrap().Selectable()
	})
}

// Badge is a small tag in the accent, as 推荐 after a title.
func Badge(c *ui.Context, text string) ui.Element {
	return ui.Text(c, text).FontSize(10).FontWeight(700).TextColor(AccentText).Background(Tint).Radius(8).Padding(0, 7).FixedLineHeight(16).Shrink(0).SingleLine()
}

// SwitchRow is a row of a card with a switch at its end toggling *on: the title over the subtitle, padded 8 points above and below and 12 before the switch. The switch is named by the title, and a click on the title or the subtitle turns it too, as on a label of a check box. It reports whether *on changed.
func SwitchRow(c *ui.Context, title, subtitle string, on *bool) bool {
	changed := false
	ui.Row(c).Shrink(0).MinHeight(48).Padding(8, 14).Gap(12).AlignItems(ui.Center).Children(func() {
		labels := ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
			ui.Text(c, title).FontSize(13).FixedLineHeight(18).TextColor(Text).Wrap()
			if subtitle != "" {
				ui.Text(c, subtitle).FontSize(11).FixedLineHeight(15).TextColor(Secondary).Wrap()
			}
		})
		if labels.Clicked() {
			*on = !*on
			changed = true
		}
		if breeze.Switch(c, on).Label(title).Changed() {
			changed = true
		}
	})
	return changed
}

// Button is breeze's push button at the prototype's size for a button in a row: 26 points high with a 12-point label. small makes it 24 high, as 重新检测 and 恢复默认.
func Button(c *ui.Context, label string, small bool) ui.Element {
	b := breeze.Button(c, label).FontSize(12).Height(26)
	if small {
		b.Height(24)
	}
	return b
}

// Select is breeze's drop-down at the prototype's size: 26 points high and as wide as its choice.
func Select(c *ui.Context, selected *string, options []string) ui.Element {
	return breeze.Select(c, selected, options).Height(26).MinWidth(0)
}
