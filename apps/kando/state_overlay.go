package main

import (
	"slices"

	"kando/apps/kando/cheatsheet"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/core"
)

// overlayUI is what is drawn over the window's pages: the shortcut cheat sheet, and later the command palette. The first-run guide, drawn over them too, keeps its state in onboardUI (state_onboarding.go).
type overlayUI struct {
	cheatsheet bool
	// modal counts the modal dialogs up over the window: a system file panel, whose answer comes back to a callback (dialogs.go), a system alert (core.Scope.ShowAlert), or a dialog an area draws. AppKit keeps the menu bar's items enabled under them, so the menu's commands wait while modalOpen is true. Change it only through holdModal.
	modal int
}

func newOverlayUI() overlayUI { return overlayUI{} }

// overlays is whatever opens over the pages, in the order they stack. view puts it last, so it sits above the sidebar and the content. Each area that draws over the window gives its own node from a function in its own file, built once under cx, the main view's scope: a core.Show on the area's open flag read through live.Func around its dialog (base.Dialog, ui.Dialog), or its own layer, so nothing is built while it is closed and what it holds keeps its focus while it is open. The cheat sheet's and the first-run guide's are in state_overlay.go and state_onboarding.go, the feedback dialog's in state_feedback.go, the sites' in state_sites.go, the palette's in palette_app.go, the quit confirmation's in queue_quit.go, and the settings' where they open over the main window in state_settings.go.
func (a *app) overlays(cx *core.Scope) core.Node {
	return core.Group{
		a.cheatsheetOverlay(cx),
		a.onboardingOverlay(cx),
		a.feedbackOverlay(cx),
		a.sitesOverlay(cx),
		a.paletteOverlay(cx),
		a.queueQuitOverlay(cx),
		a.settingsOverlay(cx),
	}
}

func (a *app) toggleCheatsheet() { a.overlay.cheatsheet = !a.overlay.cheatsheet }

// cheatsheetOverlay is the shortcut cheat sheet over the window while it is open (overlayUI.cheatsheet), in the player's dark theme: ⌘/ opens and closes it (overlayKeys), and Escape, its close button or a click outside it close it. Its groups follow the shortcut table and the language, and the sheet is built again only when they change (cheatsheetGroupsEqual), not after every change of the app's state.
func (a *app) cheatsheetOverlay(cx *core.Scope) core.Node {
	return core.Show{
		When: live.Func(a, func() bool { return a.overlay.cheatsheet }),
		Then: func(cx *core.Scope) core.Node {
			groups := core.NewMemoFunc(cx, live.Func(a, func() []cheatsheet.Group { return a.cheatsheetGroups(cx) }), cheatsheetGroupsEqual)
			return cheatsheet.View{Groups: groups.Get, OnClose: live.Do(a, func() { a.overlay.cheatsheet = false })}
		},
	}
}

// cheatsheetGroupsEqual reports whether the cheat sheet would list x and y alike.
func cheatsheetGroupsEqual(x, y []cheatsheet.Group) bool {
	return slices.EqualFunc(x, y, func(g, h cheatsheet.Group) bool {
		return g.Title == h.Title && slices.EqualFunc(g.Keys, h.Keys, func(k, l cheatsheet.Key) bool {
			return k.Label == l.Label && slices.Equal(k.Combos, l.Combos)
		})
	})
}

// holdModal marks a modal dialog as up over the window until release runs. A dialog calls it as it opens and runs release as it closes: a file panel or a system alert in its answer's callback, an area's own dialog in its OnOpenChange or wherever it clears its open flag. Dialogs nest, as a file panel opened from a sheet does, and release counts only once however often it runs.
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

// cheatsheetLayout is the cheat sheet's own arrangement of the shortcut table, the prototype's: its groups in order, and lines that name rows of the table by their keys, in NSGo's form as the rows have them (shortcut.Key), joining several under one label, as 后退 / 前进 5 秒 shows ← →. A line shows only once the table has all its keys; an empty label takes the label of its one row. The prototype's lines for shortcuts the app does not have yet are left out until their areas add them. Its labels are in the language the app shows under cx, which the sheet's binding follows.
func cheatsheetLayout(cx *core.Scope) []cheatsheetGroup {
	keys := func(keys ...string) []cheatsheet.Combo {
		combos := make([]cheatsheet.Combo, len(keys))
		for i, k := range keys {
			combos[i] = cheatsheet.Combo{Key: k}
		}
		return combos
	}
	return []cheatsheetGroup{
		{groupPlayback, []cheatsheetLine{
			{locales.CheatsheetPlayPause(cx), keys("space")},
			{locales.CheatsheetSkip5(cx), keys("left", "right")},
			{locales.CheatsheetSkip30(cx), keys("mod+left", "mod+right")},
			{locales.CheatsheetPrevNextLine(cx), keys("a", "d")},
			{"", keys("r")},
			{"", keys("l")},
			{"", keys("f")},
			{locales.CheatsheetVolume(cx), keys("up", "down")},
			{"", keys("m")},
			{locales.CheatsheetSpeed(cx), keys("shift+,", "shift+.")},
		}},
		{groupSubtitles, []cheatsheetLine{
			{locales.CheatsheetSubtitlesShowHide(cx), keys("v")},
			{locales.CheatsheetSubtitlesMode(cx), keys("s")},
			{locales.CheatsheetSubtitlesDelay(cx), keys("z", "x")},
			{locales.CheatsheetSidebar(cx), keys("mod+shift+t")},
			{locales.CheatsheetSearchLines(cx), keys("mod+f")},
		}},
		{groupWindow, []cheatsheetLine{
			{locales.CheatsheetPalette(cx), keys("mod+k")},
			{"", keys("mod+/")},
			{locales.CheatsheetSettings(cx), keys("mod+,")},
			{"", keys("escape")},
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

// cheatsheetGroups is the shortcut table's rows with a key as the cheat sheet lists them, in the language the app shows under cx: the groups and lines of cheatsheetLayout, then every row the layout does not place, under its menu's group (grouped, menu.go), in a group of the layout of that name or in a group of its own after them. So the sheet lists the whole table, and a row an area adds shows on it before the layout names it.
func (a *app) cheatsheetGroups(cx *core.Scope) []cheatsheet.Group {
	rows := slices.DeleteFunc(a.shortcuts(cx), func(s shortcut) bool { return s.Key == "" })
	return cheatsheetGroupsOf(cx, rows)
}

// cheatsheetGroupsOf arranges rows of the shortcut table as cheatsheetGroups describes.
func cheatsheetGroupsOf(cx *core.Scope, rows []shortcut) []cheatsheet.Group {
	labels := map[cheatsheet.Combo]string{}
	for _, s := range rows {
		k := cheatsheet.Combo{Key: s.Key}
		if _, ok := labels[k]; !ok {
			labels[k] = s.Label
		}
	}
	placed := map[cheatsheet.Combo]bool{}
	at := map[string]int{}
	var groups []cheatsheet.Group
	for _, g := range cheatsheetLayout(cx) {
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
			groups = append(groups, cheatsheet.Group{Title: groupTitle(cx, g.group), Keys: keys})
		}
	}
	order, byGroup := grouped(rows)
	for _, group := range order {
		for _, s := range byGroup[group] {
			k := cheatsheet.Combo{Key: s.Key}
			if placed[k] {
				continue
			}
			placed[k] = true
			i, ok := at[group]
			if !ok {
				i = len(groups)
				at[group] = i
				groups = append(groups, cheatsheet.Group{Title: groupTitle(cx, group)})
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
func (a *app) overlayKeys(cx *core.Scope) []shortcut {
	return []shortcut{
		{Group: groupHelp, Label: locales.CheatsheetMenuItem(cx), Key: "mod+/", Run: a.toggleCheatsheet},
	}
}
