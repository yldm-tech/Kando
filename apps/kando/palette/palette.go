// Package palette draws the command palette that ⌘K opens over the window: a search field, the subtitle lines and the commands it finds, and the answer to a question about the open film. It only draws and hands back what was typed, pressed and clicked (Event); what the lines and commands are, and what choosing one does, is the app's.
package palette

import (
	"slices"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Line is a subtitle line the palette found: where it starts, as the subtitle list writes it ("0:30.4"), its translation over its original, and the film it is in when the palette lists lines of several films.
type Line struct {
	At                    string
	Translation, Original string
	Film                  string
}

// Command is a command the palette lists, with the shortcut that runs it outside the palette, a key in NSGo's form ("mod+shift+n", "space"); "" shows none.
type Command struct {
	Label string
	Key   string
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
	Placeholder string
	Footer      string
	Answer      *Answer
	Lines       []Line
	Commands    []Command
	// Empty is the note shown in place of the results when the query found nothing; "" shows none.
	Empty string
	// Selected is the highlighted row, counting the lines first and the commands after them; -1 highlights none.
	Selected int
}

// Event is what happened in the palette: one of its fields is set.
type Event struct {
	// Typed is set when the query changed.
	Typed bool
	// Move is -1 or 1 when ↑ or ↓ was pressed, else 0.
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
	// Close is set when Escape or a click outside the palette asked to close it.
	Close bool
}

// App is the palette's state and what it reports. The palette reads it inside its bindings through live.Func and reports through PaletteEvent, after which it calls Changed.
type App interface {
	live.State
	// PaletteOpen reports whether the palette shows.
	PaletteOpen() bool
	// PaletteQuery is the search field's text, which the field edits.
	PaletteQuery() *string
	// PaletteView is what the palette shows now, its words in the language the app shows under cx.
	PaletteView(cx *core.Scope) View
	// PaletteEvent does what happened in the palette.
	PaletteEvent(ev Event)
}

// The palette's measures, as the prototype's: 46 below the window's top, 640 wide and at most 32 narrower than the window, at most 130 shorter than the window under the title bar's 44, corners of 24. The field's row is padded 14 18 with 10 between its parts, the list 8 with rows 2 apart; headings are padded 8 10 4, command rows 34 high padded 0 10, line rows padded 8 10 with 12 between the time, 40 wide, and the text. The answer card is padded 12 14 with corners of 14 and its chips 24 high; the footer is padded 9 18.
const (
	panelTop      = 46
	panelWidth    = 640
	panelInset    = 32
	panelRadius   = 24
	pageTop       = 44
	pageMargin    = 130
	rowRadius     = 10
	commandHeight = 34
	timeWidth     = 40
	chipHeight    = 24
	cardRadius    = 14
	fieldHeight   = 30
)

var (
	// scrim dims the window under the palette.
	scrim = core.RGBA(0x000000, 0.30)
	// glassTint is the dark glass over what the blur shows through; rim its hairline edge and shadow the shadow it casts.
	glassTint = core.RGBA(0x1C1C20, 0.88)
	rim       = core.RGBA(0xFFFFFF, 0.2)
	shadow    = core.RGBA(0x000000, 0.5)
	// hairline separates the field and the footer from the list.
	hairline = core.RGBA(0xFFFFFF, 0.12)
	ink      = core.RGB(0xFFFFFF)
	// selected is the highlighted row, Kando's blue.
	selected = core.RGB(0x0A66D8)
	// cardFill is the answer card, chipFill its chips.
	cardFill = core.RGBA(0xFFFFFF, 0.08)
	chipFill = core.RGBA(0x64AAFF, 0.28)
	// placeholder is the field's empty text.
	placeholder = core.RGBA(0xFFFFFF, 0.45)
)

// Palette draws the palette over the window in the dark theme while the app's PaletteOpen holds: the window dimmed, and near its top dark glass with the field, the answer, the lines and the commands. Escape or a click outside it report Close, ⌘K reports Toggle.
type Palette struct{ App App }

// Render implements core.Node.
func (p Palette) Render(cx *core.Scope) core.Node {
	a := p.App
	theme.Dark(cx)
	send := func(fill func(*Event)) {
		ev := Event{Pick: -1, Ref: -1}
		fill(&ev)
		a.PaletteEvent(ev)
		a.Changed()
	}
	return base.Dialog{
		Open: live.Func(a, a.PaletteOpen),
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				send(func(ev *Event) { ev.Close = true })
			}
		},
		TestID:        "palette",
		BackdropStyle: core.Style{Background: scrim},
		LayerStyle:    core.Style{JustifyContent: core.JustifyStart, Padding: core.Edges{Top: panelTop}},
		PopupStyleFunc: func() core.Style {
			window := cx.Env().Window()
			return core.Style{Width: core.Pt(max(0, min(panelWidth, window.Width-panelInset))), MaxHeight: core.Pt(max(0, window.Height+pageTop-pageMargin-panelTop)),
				CornerRadius: panelRadius, BackdropFilters: []core.Filter{core.Blur(36)}, Shadows: []core.BoxShadow{{Offset: core.Point{Y: 30}, Blur: 80, Color: shadow}}}
		},
		Children: []core.Node{panel{a: a, send: send}},
	}
}

// panel is the glass's content, built while the palette is open.
type panel struct {
	a    App
	send func(fill func(*Event))
}

// Render implements core.Node.
func (p panel) Render(cx *core.Scope) core.Node {
	a, send := p.a, p.send
	// PaletteView follows the app's whole revision, which changes on every keystroke and progress report; the memos let the rows follow only what they show.
	view := core.NewMemoFunc(cx, live.Func(a, func() View { return a.PaletteView(cx) }), equalView)
	lines := core.NewMemoFunc(cx, func() []Line { return view.Get().Lines }, slices.Equal[[]Line])
	commands := core.NewMemoFunc(cx, func() []Command { return view.Get().Commands }, slices.Equal[[]Command])
	sel := core.NewMemo(cx, func() int { return view.Get().Selected })
	list := &rows{lines: map[int]*core.Ref{}, commands: map[int]*core.Ref{}, scroll: &core.Ref{}, content: &core.Ref{}}

	// The keys work wherever the keyboard focus is in the palette, before the window's own, which would act on the film behind it: ↑ and ↓ move the highlight and bring it into view, Escape closes the palette, and ⌘K, which opened it, closes it again.
	move := func(by int) {
		send(func(ev *Event) { ev.Move = by })
		list.reveal(core.Untracked(cx, sel.Get), len(core.Untracked(cx, lines.Get)))
	}
	cx.OnKeyWith("down", core.KeyOptions{When: core.KeyAlways, Repeat: true}, func() { move(1) })
	cx.OnKeyWith("up", core.KeyOptions{When: core.KeyAlways, Repeat: true}, func() { move(-1) })
	cx.OnKeyWith("escape", core.KeyOptions{When: core.KeyAlways}, func() { send(func(ev *Event) { ev.Close = true }) })
	cx.OnKeyWith("mod+k", core.KeyOptions{When: core.KeyAlways}, func() { send(func(ev *Event) { ev.Toggle = true }) })

	results := core.View{Ref: list.content, Style: core.Style{Padding: core.All(8), Gap: 2, AlignItems: core.AlignStretch}, Children: []core.Node{
		answer{view: view, send: send},
		core.Show{When: func() bool { return view.Get().Empty != "" }, Then: func(cx *core.Scope) core.Node {
			return core.Text{ContentFunc: func() string { return view.Get().Empty }, Style: core.Style{FontSize: 13, Opacity: core.Alpha(0.55), Padding: core.Symmetric(8, 10)}}
		}},
		core.Show{When: func() bool { return len(lines.Get()) > 0 }, Then: func(cx *core.Scope) core.Node {
			return heading(func() string { return locales.PaletteHeadingLines(cx) }, 6)
		}},
		core.Each(func() []int { return count(len(lines.Get())) }, func(i int) int { return i }, func(cx *core.Scope, item func() int, _ func() int) core.Node {
			i := core.Untracked(cx, item)
			line := func() Line {
				if l := lines.Get(); i < len(l) {
					return l[i]
				}
				return Line{}
			}
			return lineRow(cx, list.ref(cx, list.lines, i), line, func() bool { return sel.Get() == i }, func() { send(func(ev *Event) { ev.Pick = i }) })
		}),
		core.Show{When: func() bool { return len(commands.Get()) > 0 }, Then: func(cx *core.Scope) core.Node {
			return heading(func() string { return locales.PaletteHeadingCommands(cx) }, 8)
		}},
		core.Each(func() []int { return count(len(commands.Get())) }, func(i int) int { return i }, func(cx *core.Scope, item func() int, _ func() int) core.Node {
			i := core.Untracked(cx, item)
			command := func() Command {
				if c := commands.Get(); i < len(c) {
					return c[i]
				}
				return Command{}
			}
			at := func() int { return len(lines.Get()) + i }
			return commandRow(cx, list.ref(cx, list.commands, i), command, func() bool { return sel.Get() == at() }, func() { send(func(ev *Event) { ev.Pick = core.Untracked(cx, at) }) })
		}),
	}}
	return core.View{
		Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0), AlignItems: core.AlignStretch, Overflow: core.Hidden, CornerRadius: panelRadius,
			Background: glassTint, Shadows: []core.BoxShadow{{Spread: 0.5, Color: rim, Inset: true}}, Color: ink},
		Children: []core.Node{
			field(cx, a, view, send),
			core.View{Style: core.Style{Height: core.Pt(0.5), Background: hairline}},
			core.Scroll{Ref: list.scroll, Style: core.Style{FlexShrink: 1, MinHeight: core.Pt(0)}, Children: []core.Node{results}},
			core.View{Style: core.Style{Height: core.Pt(0.5), Background: hairline}},
			core.Text{ContentFunc: func() string { return view.Get().Footer }, Style: core.Style{FontSize: 11, Opacity: core.Alpha(0.6), Padding: core.Symmetric(9, 18)}},
		},
	}
}

// field is the search row: the magnifier, the field 17 high in a row of 30 that has the keyboard as the palette opens, and Esc.
func field(cx *core.Scope, a App, view *core.Memo[View], send func(fill func(*Event))) core.Node {
	// TODO(nsgo): give core.Input a LabelFunc twin so the field's accessible name follows the interface language as it changes; it is read once as the palette opens
	input := base.Input{
		TestID:          "palette-field",
		AutoFocus:       true,
		Label:           locales.PaletteFieldLabel(cx),
		Value:           live.Func(a, func() string { return *a.PaletteQuery() }),
		PlaceholderFunc: func() string { return view.Get().Placeholder },
		OnChange: func(text string) {
			*a.PaletteQuery() = text
			send(func(ev *Event) { ev.Typed = true })
		},
		OnSubmit: func(string) { send(func(ev *Event) { ev.Submit = true }) },
		Style: core.Style{FlexGrow: 1, MinWidth: core.Pt(0), Height: core.Pt(fieldHeight), FontSize: 17, Color: ink, PlaceholderColor: placeholder,
			CaretColor: selected, SelectionColor: selected.Faded(0.4)},
	}
	return core.View{Style: core.Style{Direction: core.Row, Padding: core.Symmetric(14, 18), Gap: 10, AlignItems: core.AlignCenter}, Children: []core.Node{
		core.View{Style: core.Style{FlexShrink: 0, Opacity: core.Alpha(0.7)}, Children: []core.Node{ui.Icon(svg(iconSearch), 16, func() core.Color { return ink })}},
		input,
		core.Text{Content: "Esc", Style: core.Style{FontSize: 11, Opacity: core.Alpha(0.5), FlexShrink: 0}},
	}}
}

// heading is a group's title, 11 bold at 55%, top above it.
func heading(title func() string, top core.Points) core.Node {
	return core.Text{ContentFunc: title, Role: core.RoleHeader, Style: core.Style{FontSize: 11, FontWeight: ui.Bold, Opacity: core.Alpha(0.55), Padding: core.Edges{Top: top, Right: 10, Bottom: 4, Left: 10}}}
}

// rowStyle is a result's button: corners of 10, Kando's blue while highlighted.
func rowStyle(on func() bool) func() core.Style {
	return func() core.Style {
		if on() {
			return core.Style{Background: selected}
		}
		return core.Style{}
	}
}

// commandRow is a command's row: its label 13 and its shortcut 11 at 55%, 34 high.
func commandRow(cx *core.Scope, ref *core.Ref, cmd func() Command, on func() bool, press func()) core.Node {
	keys := func() string { return keyText(cx, cmd().Key) }
	return core.View{Ref: ref, Children: []core.Node{base.Button{
		LabelFunc: func() string { return cmd().Label },
		OnPress:   func(core.PressEvent) { press() },
		Style:     core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Height: core.Pt(commandHeight), Padding: core.Symmetric(0, 10), CornerRadius: rowRadius, Color: ink},
		StyleFunc: rowStyle(on),
		Children: []core.Node{
			core.Text{ContentFunc: func() string { return cmd().Label }, MaxLines: 1, EllipsizeMode: core.EllipsizeTail, Style: core.Style{FontSize: 13, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}},
			core.Show{When: func() bool { return keys() != "" }, Then: func(*core.Scope) core.Node {
				return core.Text{ContentFunc: keys, Style: core.Style{FontSize: 11, Opacity: core.Alpha(0.55), FlexShrink: 0}}
			}},
		},
	}}}
}

// keyText is a command's shortcut as the platform's menus write it, and Esc for Escape alone, as the cheat sheet writes it.
func keyText(cx *core.Scope, key string) string {
	switch key {
	case "":
		return ""
	case "escape":
		return "Esc"
	}
	return cx.KeyLabel(key)
}

// lineRow is a subtitle line's row: its time 11 at 70% in a column of 40, then the translation 13 over the original 12 at 70%, and the film's name after them when there is one.
func lineRow(cx *core.Scope, ref *core.Ref, line func() Line, on func() bool, press func()) core.Node {
	text := func(get func(Line) string, style core.Style) core.Node {
		return core.Show{When: func() bool { return get(line()) != "" }, Then: func(*core.Scope) core.Node {
			return core.Text{ContentFunc: func() string { return get(line()) }, Style: style}
		}}
	}
	return core.View{Ref: ref, Children: []core.Node{base.Button{
		LabelFunc: func() string { l := line(); return l.At + " " + l.Translation + " " + l.Original },
		OnPress:   func(core.PressEvent) { press() },
		Style:     core.Style{Direction: core.Row, Gap: 12, AlignItems: core.AlignStart, Padding: core.Symmetric(8, 10), CornerRadius: rowRadius, Color: ink},
		StyleFunc: rowStyle(on),
		Children: []core.Node{
			core.Text{ContentFunc: func() string { return line().At }, Style: core.Style{FontSize: 11, Opacity: core.Alpha(0.7), FontVariant: core.FontTabularNums,
				Width: core.Pt(timeWidth), FlexShrink: 0, Padding: core.Edges{Top: 2}}},
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: []core.Node{
				text(func(l Line) string { return l.Translation }, core.Style{FontSize: 13}),
				text(func(l Line) string { return l.Original }, core.Style{FontSize: 12, Opacity: core.Alpha(0.7)}),
			}},
			text(func(l Line) string { return l.Film }, core.Style{FontSize: 11, Opacity: core.Alpha(0.55), FlexShrink: 0, MaxWidth: core.Pt(160), Padding: core.Edges{Top: 2}}),
		},
	}}}
}

// answer is the 回答 heading and the card under it, while there is an answer: the text 14 on lines of 21, the chips that jump to the moments it names, the button that fixes what kept it from answering, and the footnote.
type answer struct {
	view *core.Memo[View]
	send func(fill func(*Event))
}

// Render implements core.Node.
func (c answer) Render(cx *core.Scope) core.Node {
	card := core.NewMemoFunc(cx, func() *Answer { return c.view.Get().Answer }, equalAnswer)
	get := func() Answer {
		if a := card.Get(); a != nil {
			return *a
		}
		return Answer{}
	}
	refs := core.NewMemoFunc(cx, func() []string { return get().Refs }, slices.Equal[[]string])
	action := core.NewMemo(cx, func() string { return get().Action })
	return core.Show{When: func() bool { return card.Get() != nil }, Then: func(cx *core.Scope) core.Node {
		chips := core.Each(func() []int { return count(len(refs.Get())) }, func(i int) int { return i }, func(cx *core.Scope, item func() int, _ func() int) core.Node {
			i := core.Untracked(cx, item)
			ref := func() string {
				if r := refs.Get(); i < len(r) {
					return r[i]
				}
				return ""
			}
			return base.Button{
				LabelFunc: func() string { return locales.PaletteJumpTo(cx, ref()) },
				OnPress:   func(core.PressEvent) { c.send(func(ev *Event) { ev.Ref = i }) },
				Style: core.Style{Height: core.Pt(chipHeight), Padding: core.Symmetric(0, 9), CornerRadius: chipHeight / 2, Background: chipFill, Color: ink,
					AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
				Children: []core.Node{core.Text{ContentFunc: func() string { return "▶ " + ref() }, Style: core.Style{FontSize: 12, FontVariant: core.FontTabularNums}}},
			}
		})
		return core.Group{
			heading(func() string { return locales.PaletteHeadingAnswer(cx) }, 4),
			core.View{Style: core.Style{Margin: core.Edges{Right: 2, Bottom: 6, Left: 2}, Padding: core.Symmetric(12, 14), CornerRadius: cardRadius, Background: cardFill, Gap: 8, AlignItems: core.AlignStretch},
				Children: []core.Node{
					core.Text{ContentFunc: func() string { return get().Text }, Style: core.Style{FontSize: 14, LineHeight: 21},
						StyleFunc: func() core.Style {
							if get().Busy {
								return core.Style{Opacity: core.Alpha(0.7)}
							}
							return core.Style{}
						}},
					core.Show{When: func() bool { return len(refs.Get()) > 0 }, Then: func(*core.Scope) core.Node {
						return core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, Gap: 6}, Children: []core.Node{chips}}
					}},
					core.Show{When: func() bool { return action.Get() != "" }, Then: func(*core.Scope) core.Node {
						// ui.Button takes its title once, so the button is built again when the title changes, which only another language does.
						return core.View{Style: core.Style{Direction: core.Row}, Children: []core.Node{core.Dynamic(func(*core.Scope) core.Node {
							return ui.Button{Text: action.Get(), TestID: "palette-action", OnPress: func() { c.send(func(ev *Event) { ev.Action = true }) }}
						})}}
					}},
					core.Show{When: func() bool { return get().Note != "" }, Then: func(*core.Scope) core.Node {
						return core.Text{ContentFunc: func() string { return get().Note }, Style: core.Style{FontSize: 11, Opacity: core.Alpha(0.55)}}
					}},
				}},
		}
	}}
}

// rows are the refs of the result rows the palette shows, by their place in the lines and in the commands, the scroll view they are in and its content, so ↑ and ↓ can bring the highlighted row into view.
type rows struct {
	lines, commands map[int]*core.Ref
	scroll, content *core.Ref
}

// ref is the ref of row i of group, which it holds while the row is built under cx.
func (r *rows) ref(cx *core.Scope, group map[int]*core.Ref, i int) *core.Ref {
	ref := &core.Ref{}
	group[i] = ref
	cx.Cleanup(func() {
		if group[i] == ref {
			delete(group, i)
		}
	})
	return ref
}

// reveal scrolls the list so row sel, counted as View.Selected counts the rows over lines lines, shows whole once the highlight has moved there.
func (r *rows) reveal(sel, lines int) {
	group, i := r.lines, sel
	if sel >= lines {
		group, i = r.commands, sel-lines
	}
	row, ok := group[i]
	if !ok {
		return
	}
	row.Measure(func(_, at core.Bounds, ok bool) {
		port, shown := r.scroll.Bounds()
		content, placed := r.content.Bounds()
		if !ok || !shown || !placed {
			return
		}
		offset := port.Origin.Y - content.Origin.Y
		switch top, bottom := at.Origin.Y, at.Origin.Y+at.Size.Height; {
		case top < port.Origin.Y:
			r.scroll.ScrollTo(core.Point{Y: offset - (port.Origin.Y - top)}, false)
		case bottom > port.Origin.Y+port.Size.Height:
			r.scroll.ScrollTo(core.Point{Y: offset + bottom - (port.Origin.Y + port.Size.Height)}, false)
		}
	})
}

// count is the indices of n rows.
func count(n int) []int {
	out := make([]int, n)
	for i := range out {
		out[i] = i
	}
	return out
}

// equalView reports whether a and b show the same.
func equalView(a, b View) bool {
	return a.Placeholder == b.Placeholder && a.Footer == b.Footer && a.Empty == b.Empty && a.Selected == b.Selected &&
		equalAnswer(a.Answer, b.Answer) && slices.Equal(a.Lines, b.Lines) && slices.Equal(a.Commands, b.Commands)
}

// equalAnswer reports whether a and b are the same card, or both none.
func equalAnswer(a, b *Answer) bool {
	if a == nil || b == nil {
		return a == b
	}
	return a.Busy == b.Busy && a.Text == b.Text && a.Note == b.Note && a.Action == b.Action && slices.Equal(a.Refs, b.Refs)
}
