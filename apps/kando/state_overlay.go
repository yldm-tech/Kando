package main

import (
	"slices"

	"kando/apps/kando/cheatsheet"
	"kando/apps/kando/locales"
	"kando/apps/kando/onboarding"

	"github.com/egoist/mygo/ui"
)

// overlayUI is what is drawn over the window's pages: the shortcut cheat sheet, and later the command palette. The first-run guide, drawn over them too, keeps its state in onboardUI (state_onboarding.go).
type overlayUI struct {
	cheatsheet bool
	// modal counts the modal dialogs up over the window: a system file panel (dialogs.go), or a sheet or alert an area draws. AppKit keeps the menu bar's items enabled under a window's sheet, so the menu's commands wait while modalOpen is true. Change it only through holdModal.
	modal int
}

func newOverlayUI() overlayUI { return overlayUI{} }

// overlays draws whatever is open over the pages. view calls it last, so it sits above the sidebar and the content. t is the window's theme and dark the player's.
func (a *app) overlays(c *ui.Context, t, dark *ui.Theme) {
	if a.overlay.cheatsheet {
		cheatsheet.Show(c, dark, &a.overlay.cheatsheet, a.cheatsheetGroups())
	}
	onboarding.Show(c, a)
	a.feedbackOverlay(c, t)
	a.sitesOverlay(c, t)
	a.paletteOverlay(c, dark)
	a.queueQuitOverlay(c)
}

func (a *app) toggleCheatsheet() { a.overlay.cheatsheet = !a.overlay.cheatsheet }

// holdModal marks a modal dialog as up over the window until release runs. A dialog calls it as it opens and runs release as it closes; an area that draws a sheet or alert keeps release in its state and runs it once its open flag is false again (its afterFrame is a good place). Dialogs nest, as a file panel opened from a sheet does, and release counts only once however often it runs.
func (a *app) holdModal() (release func()) {
	a.overlay.modal++
	released := false
	return func() {
		if !released {
			released = true
			a.overlay.modal--
		}
	}
}

// modalOpen reports whether a modal dialog is up over the window (holdModal). The cheat sheet is not one of them: it lets its own ⌘/ through.
func (a *app) modalOpen() bool { return a.overlay.modal > 0 }

// cheatsheetLayout is the cheat sheet's own arrangement of the shortcut table, the prototype's: its groups in order, and lines that name rows of the table by their keys, joining several under one label, as 后退 / 前进 5 秒 shows ← →. A line shows only once the table has all its keys; an empty label takes the label of its one row. The prototype's lines for shortcuts the app does not have yet are left out until their areas add them. It is built each time the sheet is shown, so its labels are in the interface's language then.
func cheatsheetLayout() []cheatsheetGroup {
	return []cheatsheetGroup{
		{groupPlayback, []cheatsheetLine{
			{locales.CheatsheetPlayPause(), []cheatsheet.Combo{{Key: ui.KeySpace}}},
			{locales.CheatsheetSkip5(), []cheatsheet.Combo{{Key: ui.KeyLeft}, {Key: ui.KeyRight}}},
			{locales.CheatsheetSkip30(), []cheatsheet.Combo{{Mods: ui.Cmd, Key: ui.KeyLeft}, {Mods: ui.Cmd, Key: ui.KeyRight}}},
			{"", []cheatsheet.Combo{{Key: ui.KeyF}}},
			{locales.CheatsheetVolume(), []cheatsheet.Combo{{Key: ui.KeyUp}, {Key: ui.KeyDown}}},
			{"", []cheatsheet.Combo{{Key: ui.KeyM}}},
		}},
		{groupSubtitles, []cheatsheetLine{
			{locales.CheatsheetSubtitlesShowHide(), []cheatsheet.Combo{{Key: ui.KeyV}}},
			{locales.CheatsheetSubtitlesMode(), []cheatsheet.Combo{{Key: ui.KeyS}}},
			{locales.CheatsheetSubtitlesDelay(), []cheatsheet.Combo{{Key: ui.KeyZ}, {Key: ui.KeyX}}},
			{locales.CheatsheetSidebar(), []cheatsheet.Combo{{Mods: ui.Cmd | ui.Shift, Key: ui.KeyT}}},
			{locales.CheatsheetSearchLines(), []cheatsheet.Combo{{Mods: ui.Cmd, Key: ui.KeyF}}},
		}},
		{groupWindow, []cheatsheetLine{
			{locales.CheatsheetPalette(), []cheatsheet.Combo{{Mods: ui.Cmd, Key: ui.KeyK}}},
			{"", []cheatsheet.Combo{{Mods: ui.Cmd, Key: ui.KeySlash}}},
			{locales.CheatsheetSettings(), []cheatsheet.Combo{{Mods: ui.Cmd, Key: ui.KeyComma}}},
			{"", []cheatsheet.Combo{{Key: ui.KeyEscape}}},
		}},
	}
}

// cheatsheetGroup is a group of cheatsheetLayout: its group ID and its lines.
type cheatsheetGroup struct {
	group string
	lines []cheatsheetLine
}

// cheatsheetLine is a line of cheatsheetLayout.
type cheatsheetLine struct {
	label  string
	combos []cheatsheet.Combo
}

// cheatsheetGroups is the shortcut table's rows with a key as the cheat sheet lists them: the groups and lines of cheatsheetLayout, then every row the layout does not place, under its menu's group (grouped, menu.go), in a group of the layout of that name or in a group of its own after them. So the sheet lists the whole table, and a row an area adds shows on it before the layout names it.
func (a *app) cheatsheetGroups() []cheatsheet.Group {
	rows := slices.DeleteFunc(a.shortcuts(), func(s shortcut) bool { return s.Key == 0 })
	return cheatsheetGroupsOf(rows)
}

// cheatsheetGroupsOf arranges rows of the shortcut table as cheatsheetGroups describes.
func cheatsheetGroupsOf(rows []shortcut) []cheatsheet.Group {
	labels := map[cheatsheet.Combo]string{}
	for _, s := range rows {
		k := cheatsheet.Combo{Mods: s.Mods, Key: s.Key}
		if _, ok := labels[k]; !ok {
			labels[k] = s.Label
		}
	}
	placed := map[cheatsheet.Combo]bool{}
	at := map[string]int{}
	var groups []cheatsheet.Group
	for _, g := range cheatsheetLayout() {
		var keys []cheatsheet.Key
		for _, l := range g.lines {
			if slices.ContainsFunc(l.combos, func(k cheatsheet.Combo) bool { _, ok := labels[k]; return !ok }) {
				continue
			}
			label := l.label
			if label == "" {
				label = labels[l.combos[0]]
			}
			keys = append(keys, cheatsheet.Key{Label: label, Combos: l.combos})
			for _, k := range l.combos {
				placed[k] = true
			}
		}
		if len(keys) > 0 {
			at[g.group] = len(groups)
			groups = append(groups, cheatsheet.Group{Title: groupTitle(g.group), Keys: keys})
		}
	}
	order, byGroup := grouped(rows)
	for _, group := range order {
		for _, s := range byGroup[group] {
			k := cheatsheet.Combo{Mods: s.Mods, Key: s.Key}
			if placed[k] {
				continue
			}
			placed[k] = true
			i, ok := at[group]
			if !ok {
				i = len(groups)
				at[group] = i
				groups = append(groups, cheatsheet.Group{Title: groupTitle(group)})
			}
			groups[i].Keys = append(groups[i].Keys, cheatsheet.Key{Label: s.Label, Combos: []cheatsheet.Combo{k}})
		}
	}
	return groups
}

// savedOverlay is the overlays' part of settings.json. Whether the first-run guide was seen is the guide's own part (savedOnboarding).
type savedOverlay struct{}

func (s *savedOverlay) clean() {}

// overlayHooks: the overlays keep nothing in settings.json and do nothing at the shared moments.
func (a *app) overlayHooks() areaHooks { return areaHooks{} }

// overlayKeys are the shortcuts of the overlays.
func (a *app) overlayKeys() []shortcut {
	return []shortcut{
		{Group: groupHelp, Label: locales.CheatsheetMenuItem(), Mods: ui.Cmd, Key: ui.KeySlash, Run: a.toggleCheatsheet},
	}
}
