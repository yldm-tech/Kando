package playlist

import (
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Look is what the dialog shows: the videos being looked for, why they could not be, or how many were found and how many of them the library has.
type Look struct {
	// Loading is set while the links are read.
	Loading bool
	// Err says why they could not be read, in the interface's language; "" when they were.
	Err string
	// Title is the name of the one list read, "" when it has none or several were read; Lists is how many were, and Channels how many of them are channels, whose ChannelLimit most recent videos are listed.
	Title    string
	Lists    int
	Channels int
	// Videos is how many videos were found, each once, and Have how many of them the library has already, waiting, being processed or done, which are not added again.
	Videos int
	Have   int
}

// Adds is how many videos Add queues: those found that the library does not have yet.
func (l Look) Adds() int {
	if l.Loading || l.Err != "" {
		return 0
	}
	return l.Videos - l.Have
}

// Confirm is the dialog over the window while Open returns true: it says that the videos are being looked for, then how many there are, a channel's limited to its ChannelLimit most recent, and how many of them the library has, and asks before they are queued. Once one list was read, a switch subscribes to it as the videos are added (Subscribe, OnSubscribe), so its new videos are added as they come out; for one playlist, another has its films share a glossary as one series (Series, OnSeries); for one channel, a choice says how many of its most recent videos are added now (Recent, OnRecent). Add queues the ones the library does not have, Adding of them when set, else Look.Adds, and runs OnAdd, as it does with none to add while the subscribe switch is on; Cancel, Escape and closing the dialog run OnCancel, which also stops the reading.
type Confirm struct {
	Open        func() bool
	Look        func() Look
	Subscribe   func() bool
	OnSubscribe func(on bool)
	Series      func() bool
	OnSeries    func(on bool)
	Recent      func() int
	OnRecent    func(n int)
	// Adding is how many videos Add queues, which a channel's Recent limits; nil for Look.Adds.
	Adding   func() int
	OnAdd    func()
	OnCancel func()
}

// RecentChoices are how many of a channel's most recent videos the dialog offers to add now, as the prototype has it: none, the latest 3 or the latest 10; RecentDefault is the one it starts at.
var RecentChoices = []int{0, 3, 10}

// RecentDefault is how many of a channel's most recent videos the dialog adds unless the user chooses otherwise.
const RecentDefault = 3

// kind is what the one list read is, as PlaylistSubscribe's select names it: "channel" or "playlist".
func (l Look) kind() string {
	if l.Channels > 0 {
		return "channel"
	}
	return "playlist"
}

// Subscribable reports whether the dialog offers to subscribe to what was read: one list, read.
func (l Look) Subscribable() bool {
	return !l.Loading && l.Err == "" && l.Lists == 1
}

// Serial reports whether the films of what was read can be one series: one playlist with a name and at least two videos, as the app names the series after it.
func (l Look) Serial() bool {
	return l.Subscribable() && l.Channels == 0 && l.Title != "" && l.Videos >= 2
}

// OneChannel reports whether what was read is one channel, of whose most recent videos the dialog asks how many to add now.
func (l Look) OneChannel() bool {
	return l.Subscribable() && l.Channels == 1 && l.Videos > 0
}

// Render implements core.Node.
func (c Confirm) Render(cx *core.Scope) core.Node {
	// Look may follow the app's whole revision, which changes on every keystroke and progress report; the memo lets what the dialog shows be worked out again only when it changes.
	look := core.NewMemo(cx, c.Look)
	cancel := func() {
		if c.OnCancel != nil {
			c.OnCancel()
		}
	}
	return core.Show{When: c.Open, Then: func(cx *core.Scope) core.Node {
		// What the dialog shows under its title is built again only when its shape changes, which lines and rows there are; their words and numbers follow the look and the language through bindings, so a switch or the drop-down keeps its focus while the videos are counted or the language changes.
		shape := core.NewMemo(cx, func() shapeOf { return shapeFor(look.Get(), c) })
		content := core.Dynamic(func(cx *core.Scope) core.Node {
			s := shape.Get()
			children := body(cx, s, look.Get)
			var rows []core.Node
			add := func(row core.Node) {
				if len(rows) > 0 {
					rows = append(rows, form.Separator(true))
				}
				rows = append(rows, row)
			}
			if s.recent {
				add(recentRow(cx, c))
			}
			if s.subscribe {
				add(form.SwitchRow{TitleFunc: func() string { return locales.PlaylistSubscribe(cx, look.Get().kind()) },
					SubtitleFunc: func() string { return locales.PlaylistSubscribeHint(cx) }, TestID: "playlist-subscribe",
					On: c.Subscribe, OnChange: c.OnSubscribe})
			}
			if s.series {
				add(form.SwitchRow{TitleFunc: func() string { return locales.PlaylistSeries(cx) },
					SubtitleFunc: func() string { return locales.PlaylistSeriesHint(cx) }, TestID: "playlist-series",
					On: c.Series, OnChange: c.OnSeries})
			}
			if len(rows) > 0 {
				children = append(children, form.Card(rows...))
			}
			return core.Group(children)
		})
		return ui.Dialog{
			TestID:                 "playlist-confirm",
			Role:                   core.RoleAlertDialog,
			NoCloseOnBackdropPress: true,
			Open:                   func() bool { return true },
			TitleFunc:              func() string { return title(cx, look.Get()) },
			Children:               []core.Node{content},
			OnOpenChange: func(open bool, _ base.Reason) {
				if !open {
					cancel()
				}
			},
			Footer: []core.Node{
				ui.Button{TextFunc: func() string { return locales.PlaylistCancel(cx) }, Variant: ui.ButtonSecondary, TestID: "playlist-cancel", OnPress: cancel},
				c.addButton(cx, look.Get),
			},
		}
	}}
}

// addButton is Add, which says how many videos it queues, or, with none to queue, Subscribe, which is on while the subscribe switch is; with neither, there is no button. Its words follow the number, as a channel's Recent changes it, and the look.
func (c Confirm) addButton(cx *core.Scope, look func() Look) core.Node {
	n := core.NewMemo(cx, func() int {
		if l := look(); c.Adding != nil && !l.Loading && l.Err == "" {
			return c.Adding()
		}
		return look().Adds()
	})
	subscribeOnly := func() bool { return n.Get() == 0 && look().Subscribable() && c.Subscribe != nil }
	return core.Show{When: func() bool { return n.Get() > 0 || subscribeOnly() }, Then: func(cx *core.Scope) core.Node {
		return ui.Button{TestID: "playlist-add", OnPress: c.OnAdd,
			TextFunc: func() string {
				if n := n.Get(); n > 0 {
					return locales.PlaylistAdd(cx, n)
				}
				return locales.PlaylistSubscribeOnly(cx)
			},
			Disabled: func() bool { return subscribeOnly() && !c.Subscribe() }}
	}}
}

// recentRow is 现在添加: how many of the channel's most recent videos Add queues now, none, the latest 3 or the latest 10 (RecentChoices).
func recentRow(cx *core.Scope, c Confirm) core.Node {
	options := func() []ui.Option[int] {
		out := make([]ui.Option[int], 0, len(RecentChoices))
		for _, n := range RecentChoices {
			label := locales.PlaylistRecentNone(cx)
			if n > 0 {
				label = locales.PlaylistRecentLatest(cx, n)
			}
			out = append(out, ui.Option[int]{Value: n, Label: label})
		}
		return out
	}
	recent := func() string { return locales.PlaylistRecent(cx) }
	return form.Row{TitleFunc: recent, SubtitleFunc: func() string { return locales.PlaylistRecentHint(cx) }, NoGroup: true, TestID: "playlist-recent-row",
		Trailing: []core.Node{ui.Select[int]{OptionsFunc: options, LabelFunc: recent, TestID: "playlist-recent",
			Value: c.Recent, OnChange: c.OnRecent, Style: core.Style{Width: core.Pt(130)}}}}
}

// shapeOf is which lines and rows the dialog shows under its title, all that building them again depends on; their words and numbers are bound.
type shapeOf struct {
	loading, failed, empty, channels, have bool
	recent, subscribe, series              bool
}

// shapeFor is the shape of what the dialog shows for l, with the rows c has callbacks for.
func shapeFor(l Look, c Confirm) shapeOf {
	return shapeOf{
		loading: l.Loading, failed: !l.Loading && l.Err != "", empty: !l.Loading && l.Err == "" && l.Videos == 0,
		channels: l.Channels > 0, have: l.Have > 0,
		recent:    l.OneChannel() && c.Recent != nil && c.OnRecent != nil,
		subscribe: l.Subscribable() && c.Subscribe != nil && c.OnSubscribe != nil,
		series:    l.Serial() && c.Series != nil && c.OnSeries != nil,
	}
}

// title is the dialog's title for l: that the videos are being looked for, the one list's name, what it is when it has none, or how many lists were read.
func title(cx *core.Scope, l Look) string {
	switch {
	case l.Loading:
		return locales.PlaylistReading(cx)
	case l.Lists > 1:
		return locales.PlaylistSeveral(cx, l.Lists)
	case l.Title != "":
		return l.Title
	case l.Channels > 0:
		return locales.PlaylistUntitledChannel(cx)
	}
	return locales.PlaylistUntitled(cx)
}

// body is what the dialog says under its title, a sentence a line: a spinner while the videos are looked for, why they could not be, or how many there are, that a channel gives only its most recent, and how many the library has.
func body(cx *core.Scope, s shapeOf, look func() Look) []core.Node {
	colors := ui.ColorsOf(cx)
	line := func(text func() string, id string) core.Node {
		return core.Text{ContentFunc: text, TestID: id, Style: ui.TypeBody, StyleFunc: ui.Colored(colors.Muted)}
	}
	switch {
	case s.loading:
		// TODO(nsgo#1861): ui.Spinner takes its name for assistive technology as a string only (Label, ui/spinner.go), so it is built again in the interface's language https://github.com/nsgo-dev/nsgo/issues/1861
		spinner := core.Dynamic(func(cx *core.Scope) core.Node {
			return ui.Spinner{Size: 24, Label: locales.PlaylistReading(cx), TestID: "playlist-spinner"}
		})
		return []core.Node{core.View{Style: core.Style{AlignItems: core.AlignCenter, Padding: core.Symmetric(8, 0)}, Children: []core.Node{spinner}}}
	case s.failed:
		return []core.Node{line(func() string { return look().Err }, "playlist-error")}
	case s.empty:
		return []core.Node{line(func() string { return locales.PlaylistEmpty(cx) }, "playlist-count")}
	}
	lines := []core.Node{line(func() string { return locales.PlaylistVideos(cx, look().Videos) }, "playlist-count")}
	if s.channels {
		lines = append(lines, line(func() string { return locales.PlaylistChannelLimit(cx, ChannelLimit) }, "playlist-limit"))
	}
	if s.have {
		lines = append(lines, line(func() string { return locales.PlaylistHave(cx, look().Have) }, "playlist-have"))
	}
	return []core.Node{core.View{Style: core.Style{Gap: 4}, Children: lines}}
}
