package main

import (
	"lingocut/apps/lingocut/cheatsheet"

	"github.com/egoist/mygo/ui"
)

// overlayUI is what is drawn over the window's pages: the shortcut cheat sheet, and later the first-run guide and the command palette.
type overlayUI struct {
	cheatsheet bool
	// onboarded is saved once the first-run guide has been seen.
	onboarded bool
}

// overlays draws whatever is open over the pages. view calls it last, so it sits above the sidebar and the content. t is the window's theme and dark the player's.
func (a *app) overlays(c *ui.Context, t, dark *ui.Theme) {
	if a.overlay.cheatsheet {
		cheatsheet.Show(c, dark, &a.overlay.cheatsheet, a.cheatsheetGroups())
	}
}

func (a *app) toggleCheatsheet() { a.overlay.cheatsheet = !a.overlay.cheatsheet }

// cheatsheetGroups is the shortcut table grouped for the cheat sheet, in the order the groups first appear.
func (a *app) cheatsheetGroups() []cheatsheet.Group {
	var groups []cheatsheet.Group
	index := map[string]int{}
	for _, s := range a.shortcuts() {
		i, ok := index[s.Group]
		if !ok {
			i = len(groups)
			index[s.Group] = i
			groups = append(groups, cheatsheet.Group{Title: s.Group})
		}
		groups[i].Keys = append(groups[i].Keys, cheatsheet.Key{Label: s.Label, Mods: s.Mods, Key: s.Key})
	}
	return groups
}

// overlayKeys are the shortcuts of the overlays.
func (a *app) overlayKeys() []shortcut {
	return []shortcut{
		{Group: "帮助", Label: "键盘快捷键", Mods: ui.Cmd, Key: ui.KeySlash, Run: a.toggleCheatsheet},
	}
}
