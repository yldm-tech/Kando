// Package palette draws the command palette that ⌘K opens over the window: a search field, the subtitle lines and the commands it finds, and the answer to a question about the open film. It only draws and hands back what was typed, pressed and clicked (Event); what the lines and commands are, and what choosing one does, is the app's.
package palette

import (
	"github.com/egoist/mygo/plugins/glass"
	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// Line is a subtitle line the palette found: where it starts, as the subtitle list writes it ("0:30.4"), its translation over its original, and the film it is in when the palette lists lines of several films.
type Line struct {
	At                    string
	Translation, Original string
	Film                  string
}

// Command is a command the palette lists, with the shortcut that runs it outside the palette; a Key of ui.KeyUnknown shows none.
type Command struct {
	Label string
	Mods  ui.Modifiers
	Key   ui.Key
}

// Answer is the card above the results: the answer to the question asked, or what stands in for it while it is being asked or when it cannot be.
type Answer struct {
	// Busy shows Text as a note that the question is on its way, without chips.
	Busy bool
	Text string
	// Refs are the chips under the text, each a moment of the film the answer points at ("0:30").
	Refs []string
	// Note is the footnote under the chips, 11 at 55%.
	Note string
	// Action is a button under the text, such as one that opens the settings that make answers possible; "" for none.
	Action string
}

// View is what the palette shows.
type View struct {
	// Query is the search field's text, which the field edits.
	Query       *string
	Placeholder string
	Footer      string
	Answer      *Answer
	Lines       []Line
	Commands    []Command
	// Empty is the note shown in place of the results when the query found nothing; "" shows none.
	Empty string
	// Selected is the highlighted row, counting the lines first and the commands after them; -1 highlights none.
	Selected int
	// Reveal scrolls the highlighted row into view in this frame, as after it was moved to with the arrow keys.
	Reveal bool
}

// Event is what happened in the palette in a frame.
type Event struct {
	// Typed is set when the query changed.
	Typed bool
	// Move is -1 or 1 when ↑ or ↓ was pressed in the field, else 0.
	Move int
	// Submit is set when Return was pressed in the field.
	Submit bool
	// Pick is the row clicked, counted as View.Selected counts them, or -1.
	Pick int
	// Ref is the answer's chip clicked, or -1.
	Ref int
	// Action is set when the answer's button was clicked.
	Action bool
	// Toggle is set when ⌘K was pressed while the palette was open, which closes it.
	Toggle bool
}

// The palette's measures, as the prototype's: 46 below the window's top, 640 wide and at most 32 narrower than the window, at most 130 shorter than the window under the title bar's 44, corners of 24. The field's row is padded 14 18 with 10 between its parts, the list 8 with rows 2 apart; headings are padded 8 10 4, command rows 34 high padded 0 10, line rows padded 8 10 with 12 between the time, 40 wide, and the text. The answer card is padded 12 14 with corners of 14 and its chips 24 high; the footer is padded 9 18.
const (
	panelTop    = 46
	panelWidth  = 640
	panelInset  = 32
	panelRadius = 24
	pageTop     = 44
	pageMargin  = 130
	rowRadius   = 10
	commandRow  = 34
	timeWidth   = 40
	chipHeight  = 24
	cardRadius  = 14
	fieldHeight = 30
)

var (
	// scrim dims the window under the palette.
	scrim = ui.RGBA(0, 0, 0, 0.30)
	// glassTint is the dark glass over what the blur shows through; rim its hairline edge and shadow the shadow it casts.
	glassTint = ui.RGBA(28, 28, 32, 0.88)
	rim       = ui.RGBA(255, 255, 255, 0.2)
	shadow    = ui.RGBA(0, 0, 0, 0.5)
	// hairline separates the field and the footer from the list.
	hairline = ui.RGBA(255, 255, 255, 0.12)
	ink      = ui.RGBA(255, 255, 255, 1)
	// selected is the highlighted row, Kando's blue.
	selected = ui.Hex("#0A66D8")
	// cardFill is the answer card, chipFill its chips.
	cardFill = ui.RGBA(255, 255, 255, 0.08)
	chipFill = ui.RGBA(100, 170, 255, 0.28)
	// placeholder is the field's empty text.
	placeholder = ui.RGBA(255, 255, 255, 0.45)
)

// Show draws the palette over the window in the dark theme while *open: the window dimmed, and near its top dark glass with the field, the answer, the lines and the commands. Escape or a click outside it sets *open to false. It returns what was typed, pressed and clicked in this frame.
func Show(c *ui.Context, dark *ui.Theme, open *bool, v View) Event {
	ev := Event{Pick: -1, Ref: -1}
	ui.Themed(c, dark).Absolute().Children(func() {
		w, h := c.Size()
		width := min(panelWidth, w-panelInset)
		panel := ui.DialogBase(c, open, func(back, p ui.Element) {
			back.Background(scrim).Justify(ui.Start).Padding(panelTop, 0, 0, 0)
			// The glass: the panel blurs what is under it, and the column inside tints the blur dark.
			p.Width(width).MaxHeight(h+pageTop-pageMargin-panelTop).Padding(0).Radius(panelRadius).Material(glass.Blur{Radius: 36}).Shadow(0, 30, 80, 0, shadow)
			ui.Column(c).Grow(1).Shrink(1).MinHeight(0).AlignItems(ui.Stretch).Radius(panelRadius).Background(glassTint).InsetShadow(0, 0, 0, 0.5, rim).TextColor(ink).Children(func() {
				field(c, v, &ev)
				ui.Box(c).Height(0.5).Background(hairline)
				ui.Scroll(c).Shrink(1).MinHeight(0).Children(func() {
					ui.Column(c).Padding(8).Gap(2).AlignItems(ui.Stretch).Children(func() {
						if v.Answer != nil {
							answer(c, *v.Answer, &ev)
						}
						if v.Empty != "" {
							ui.Text(c, v.Empty).FontSize(13).Opacity(0.55).Padding(8, 10)
						}
						if len(v.Lines) > 0 {
							heading(c, "台词", 6)
						}
						for i, l := range v.Lines {
							if line(c, l, i == v.Selected, v.Reveal).Clicked() {
								ev.Pick = i
							}
						}
						if len(v.Commands) > 0 {
							heading(c, "命令", 8)
						}
						for i, cmd := range v.Commands {
							at := len(v.Lines) + i
							if command(c, cmd, at == v.Selected, v.Reveal).Clicked() {
								ev.Pick = at
							}
						}
					})
				})
				ui.Box(c).Height(0.5).Background(hairline)
				ui.Text(c, v.Footer).FontSize(11).Opacity(0.6).Padding(9, 18)
			})
		})
		// ⌘K, which opened the palette, closes it again: the window's own ⌘K waits behind the dialog while it is open.
		if panel.OverlayShortcut(ui.Cmd, ui.KeyK) {
			ev.Toggle = true
		}
	})
	return ev
}

// field is the search row: the magnifier, the field 17 high in a row of 30 that has the keyboard as the palette opens, and Esc.
func field(c *ui.Context, v View, ev *Event) {
	ui.Row(c).Padding(14, 18).Gap(10).AlignItems(ui.Center).Children(func() {
		ui.Icon(c, iconSearch).Size(16, 16).Shrink(0).Opacity(0.7)
		in := ui.TextInputBase(c, v.Query).Grow(1).MinWidth(0).Height(fieldHeight).FontSize(17).TextColor(ink).
			Placeholder(v.Placeholder).PlaceholderColor(placeholder).Label("命令、台词或问题").AutoFocus()
		if in.Changed() {
			ev.Typed = true
		}
		switch {
		case in.Shortcut(0, ui.KeyDown):
			ev.Move = 1
		case in.Shortcut(0, ui.KeyUp):
			ev.Move = -1
		}
		if in.Submitted() {
			ev.Submit = true
		}
		ui.Text(c, "Esc").FontSize(11).Opacity(0.5).Shrink(0)
	})
}

// heading is a group's title, 11 bold at 55%, top above it.
func heading(c *ui.Context, title string, top float32) {
	ui.Text(c, title).FontSize(11).Bold().Opacity(0.55).Padding(top, 10, 4, 10)
}

// row is a result's button: corners of 10, Kando's blue while highlighted, scrolled into view when reveal asks for it.
func row(c *ui.Context, label string, on, reveal bool) ui.Element {
	b := ui.ButtonBase(c).Radius(rowRadius).Label(label).TextColor(ink)
	if on {
		b.Background(selected)
		if reveal {
			b.ScrollIntoView()
		}
	}
	return b
}

// command is a command's row: its label 13 and its shortcut 11 at 55%, 34 high.
func command(c *ui.Context, cmd Command, on, reveal bool) ui.Element {
	b := row(c, cmd.Label, on, reveal).Height(commandRow).Padding(0, 10)
	b.Children(func() {
		ui.Row(c).Grow(1).AlignItems(ui.Center).Children(func() {
			ui.Text(c, cmd.Label).FontSize(13).Grow(1).MinWidth(0)
			if keys := keyText(c, cmd); keys != "" {
				ui.Text(c, keys).FontSize(11).Opacity(0.55).Shrink(0)
			}
		})
	})
	return b
}

// keyText is the command's shortcut as the platform's menus write it, and Esc for Escape alone, as the cheat sheet writes it.
func keyText(c *ui.Context, cmd Command) string {
	if cmd.Key == ui.KeyUnknown {
		return ""
	}
	if cmd.Key == ui.KeyEscape && cmd.Mods == 0 {
		return "Esc"
	}
	return breeze.ShortcutText(c, cmd.Mods, cmd.Key)
}

// line is a subtitle line's row: its time 11 at 70% in a column of 40, then the translation 13 over the original 12 at 70%, and the film's name after them when there is one.
func line(c *ui.Context, l Line, on, reveal bool) ui.Element {
	b := row(c, l.At+" "+l.Translation+" "+l.Original, on, reveal).Padding(8, 10)
	b.Children(func() {
		ui.Row(c).Grow(1).Gap(12).AlignItems(ui.Start).Children(func() {
			ui.Text(c, l.At).FontSize(11).Opacity(0.7).FontFeatures("tnum").Width(timeWidth).Shrink(0).Padding(2, 0, 0, 0)
			ui.Column(c).Grow(1).MinWidth(0).Gap(2).Children(func() {
				if l.Translation != "" {
					ui.Text(c, l.Translation).FontSize(13)
				}
				if l.Original != "" {
					ui.Text(c, l.Original).FontSize(12).Opacity(0.7)
				}
			})
			if l.Film != "" {
				ui.Text(c, l.Film).FontSize(11).Opacity(0.55).Shrink(0).MaxWidth(160).Padding(2, 0, 0, 0)
			}
		})
	})
	return b
}

// answer is the 回答 heading and the card under it: the text 14 on lines of 21, the chips that jump to the moments it names, the button that fixes what kept it from answering, and the footnote.
func answer(c *ui.Context, a Answer, ev *Event) {
	ui.Text(c, "回答").FontSize(11).Bold().Opacity(0.55).Padding(4, 10)
	ui.Column(c).Margin(0, 2, 6, 2).Padding(12, 14).Radius(cardRadius).Background(cardFill).Gap(8).AlignItems(ui.Stretch).Children(func() {
		text := ui.Text(c, a.Text).FontSize(14).LineHeight(21.0 / 14)
		if a.Busy {
			text.Opacity(0.7)
		}
		if len(a.Refs) > 0 {
			ui.Row(c).Wrap().Gap(6).Children(func() {
				for i, ref := range a.Refs {
					chip := ui.ButtonBase(c).Height(chipHeight).Padding(0, 9).Radius(chipHeight / 2).Background(chipFill).TextColor(ink).Label("跳到 " + ref)
					chip.Children(func() {
						ui.Text(c, "▶ "+ref).FontSize(12).FontFeatures("tnum")
					})
					if chip.Clicked() {
						ev.Ref = i
					}
				}
			})
		}
		if a.Action != "" {
			ui.Row(c).Children(func() {
				if breeze.PrimaryButton(c, a.Action).Clicked() {
					ev.Action = true
				}
			})
		}
		if a.Note != "" {
			ui.Text(c, a.Note).FontSize(11).Opacity(0.55)
		}
	})
}
