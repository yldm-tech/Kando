package playlist

import (
	"kando/apps/kando/locales"

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

// Confirm is the dialog over the window while Open returns true: it says that the videos are being looked for, then how many there are, a channel's limited to its ChannelLimit most recent, and how many of them the library has, and asks before they are queued. Add queues the ones the library does not have and runs OnAdd; Cancel, Escape and closing the dialog run OnCancel, which also stops the reading.
type Confirm struct {
	Open     func() bool
	Look     func() Look
	OnAdd    func()
	OnCancel func()
}

// Render implements core.Node.
func (c Confirm) Render(cx *core.Scope) core.Node {
	// Look may follow the app's whole revision, which changes on every keystroke and progress report; the memo lets the dialog below be built again only when what it shows changes.
	look := core.NewMemo(cx, c.Look)
	cancel := func() {
		if c.OnCancel != nil {
			c.OnCancel()
		}
	}
	return core.Show{When: c.Open, Then: func(cx *core.Scope) core.Node {
		return core.Dynamic(func(cx *core.Scope) core.Node {
			l := look.Get()
			// TODO(nsgo#145): give ui.Dialog and ui.AlertDialog TitleFunc and DescriptionFunc twins so the words follow the list and the language without building the dialog again, then drop this Dynamic https://github.com/nsgo-dev/nsgo/issues/145
			footer := []core.Node{ui.Button{Text: locales.PlaylistCancel(cx), Variant: ui.ButtonSecondary, TestID: "playlist-cancel", OnPress: cancel}}
			if n := l.Adds(); n > 0 {
				footer = append(footer, ui.Button{Text: locales.PlaylistAdd(cx, n), TestID: "playlist-add", OnPress: c.OnAdd})
			}
			return ui.Dialog{
				TestID:                 "playlist-confirm",
				Role:                   core.RoleAlertDialog,
				NoCloseOnBackdropPress: true,
				Open:                   func() bool { return true },
				Title:                  title(cx, l),
				Children:               body(cx, l),
				OnOpenChange: func(open bool, _ base.Reason) {
					if !open {
						cancel()
					}
				},
				Footer: footer,
			}
		})
	}}
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
func body(cx *core.Scope, l Look) []core.Node {
	colors := ui.ColorsOf(cx)
	line := func(text, id string) core.Node {
		return core.Text{Content: text, TestID: id, Style: ui.TypeBody, StyleFunc: ui.Colored(colors.Muted)}
	}
	switch {
	case l.Loading:
		return []core.Node{core.View{Style: core.Style{AlignItems: core.AlignCenter, Padding: core.Symmetric(8, 0)}, Children: []core.Node{
			ui.Spinner{Size: 24, Label: locales.PlaylistReading(cx), TestID: "playlist-spinner"},
		}}}
	case l.Err != "":
		return []core.Node{line(l.Err, "playlist-error")}
	case l.Videos == 0:
		return []core.Node{line(locales.PlaylistEmpty(cx), "playlist-count")}
	}
	lines := []core.Node{line(locales.PlaylistVideos(cx, l.Videos), "playlist-count")}
	if l.Channels > 0 {
		lines = append(lines, line(locales.PlaylistChannelLimit(cx, ChannelLimit), "playlist-limit"))
	}
	if l.Have > 0 {
		lines = append(lines, line(locales.PlaylistHave(cx, l.Have), "playlist-have"))
	}
	return []core.Node{core.View{Style: core.Style{Gap: 4}, Children: lines}}
}
