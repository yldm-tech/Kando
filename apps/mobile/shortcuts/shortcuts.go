// Package shortcuts is the phone app's keyboard shortcuts for a hardware keyboard on an iPad, an Android tablet or a phone (yldm-tech/Kando#225): the keys the player and the shell bind, in NSGo's form (core.Scope.OnKey), and the list Settings › Keyboard Shortcuts shows on a tablet, as the design's iPad artboard has them.
package shortcuts

import (
	"strconv"
	"strings"

	"kando/apps/mobile/locales"

	"github.com/nsgo-dev/nsgo/core"
)

// The keys of the player, which act on the film and the line the card shows.
const (
	PlayPause = "space"
	PrevLine  = "left"
	NextLine  = "right"
	Replay    = "r"
	Loop      = "l"
	Save      = "s"
	Explain   = "e"
	Subtitles = "m"
)

// The keys of the shell, which work wherever a section shows.
const (
	// Search goes to the Library's search.
	Search = "mod+k"
	// Add opens the Add sheet.
	Add = "mod+n"
)

// Section is the key that shows the shell's section i, counting from 0: ⌘1 (Ctrl+1 on Android) for the first.
func Section(i int) string { return "mod+" + strconv.Itoa(i+1) }

// Row is a shortcut as the list shows it: what it does, its testable ID and its keys as the platform writes them. Read both in a binding, so they follow the language.
type Row struct {
	ID         string
	What, Keys func() string
}

// List is the shortcuts as Settings lists them, sections being the names of the shell's sections in their order, which ⌘1 and on show.
func List(cx *core.Scope, sections []func() string) []Row {
	key := func(keys ...string) func() string {
		return func() string {
			labels := make([]string, len(keys))
			for i, k := range keys {
				labels[i] = Label(cx, k)
			}
			return strings.Join(labels, " / ")
		}
	}
	rows := []Row{
		{ID: "play", What: func() string { return locales.ShortcutsPlayPause(cx) }, Keys: key(PlayPause)},
		{ID: "lines", What: func() string { return locales.ShortcutsLines(cx) }, Keys: key(PrevLine, NextLine)},
		{ID: "replay", What: func() string { return locales.ShortcutsReplay(cx) }, Keys: key(Replay)},
		{ID: "loop", What: func() string { return locales.ShortcutsLoop(cx) }, Keys: key(Loop)},
		{ID: "save", What: func() string { return locales.ShortcutsSave(cx) }, Keys: key(Save)},
		{ID: "explain", What: func() string { return locales.ShortcutsExplain(cx) }, Keys: key(Explain)},
		{ID: "subtitles", What: func() string { return locales.ShortcutsSubtitles(cx) }, Keys: key(Subtitles)},
		{ID: "search", What: func() string { return locales.ShortcutsSearch(cx) }, Keys: key(Search)},
		{ID: "add", What: func() string { return locales.ShortcutsAdd(cx) }, Keys: key(Add)},
	}
	if len(sections) > 0 {
		rows = append(rows, Row{ID: "sections",
			What: func() string {
				names := make([]string, len(sections))
				for i, name := range sections {
					names[i] = name()
				}
				return strings.Join(names, locales.ShortcutsJoin(cx))
			},
			Keys: func() string { return Label(cx, Section(0)) + " – " + Label(cx, Section(len(sections)-1)) }})
	}
	return rows
}

// Label is key as the platform writes it (core.Scope.KeyLabel), the space bar named in the app's language.
func Label(cx *core.Scope, key string) string {
	if key == PlayPause {
		return locales.ShortcutsSpace(cx)
	}
	return cx.KeyLabel(key)
}
