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

// cheatsheetOverlay is the cheat sheet over the window while it is open (overlays). Its groups follow the shortcut table and the language as they change.
func (a *app) cheatsheetOverlay(cx *core.Scope) core.Node {
	return cheatsheet.Sheet{
		Open:    live.Func(a, func() bool { return a.overlay.cheatsheet }),
		Groups:  live.Func(a, func() []cheatsheet.Group { return a.cheatsheetGroups(cx) }),
		OnClose: live.Do(a, func() { a.overlay.cheatsheet = false }),
	}
}

// cheatsheetLayout is the cheat sheet's own arrangement of the shortcut table, the prototype's: its groups in order, and lines that name rows of the table by their keys, joining several under one label, as 后退 / 前进 5 秒 shows ← →. A line shows only once the table has all its keys; an empty label takes the label of its one row. The prototype's lines for shortcuts the app does not have yet are left out until their areas add them. Its labels are in the language the app shows under cx.
func cheatsheetLayout(cx *core.Scope) []cheatsheetGroup {
	return []cheatsheetGroup{
		{groupPlayback, []cheatsheetLine{
			{locales.CheatsheetPlayPause(cx), []string{"space"}},
			{locales.CheatsheetSkip5(cx), []string{"left", "right"}},
			{locales.CheatsheetSkip30(cx), []string{"mod+left", "mod+right"}},
			{locales.CheatsheetPrevNextLine(cx), []string{"a", "d"}},
			{"", []string{"r"}},
			{"", []string{"l"}},
			{"", []string{"f"}},
			{locales.CheatsheetVolume(cx), []string{"up", "down"}},
			{"", []string{"m"}},
			{locales.CheatsheetSpeed(cx), []string{"shift+,", "shift+."}},
		}},
		{groupSubtitles, []cheatsheetLine{
			{locales.CheatsheetSubtitlesShowHide(cx), []string{"v"}},
			{locales.CheatsheetSubtitlesMode(cx), []string{"s"}},
			{locales.CheatsheetSubtitlesDelay(cx), []string{"z", "x"}},
			{locales.CheatsheetSidebar(cx), []string{"mod+shift+t"}},
			{locales.CheatsheetSearchLines(cx), []string{"mod+f"}},
		}},
		{groupWindow, []cheatsheetLine{
			{locales.CheatsheetPalette(cx), []string{"mod+k"}},
			{"", []string{cheatsheetKey}},
			{locales.CheatsheetSettings(cx), []string{"mod+,"}},
			{"", []string{"escape"}},
		}},
	}
}

// cheatsheetGroup is a group of cheatsheetLayout: its group ID and its lines.
type cheatsheetGroup struct {
	group string
	lines []cheatsheetLine
}

// cheatsheetLine is a line of cheatsheetLayout: its label and the keys of its rows, in NSGo's form as the shortcut table writes them (shortcut.Key).
type cheatsheetLine struct {
	label  string
	combos []string
}

// cheatsheetGroups is the shortcut table's rows with a key as the cheat sheet lists them, in the language the app shows under cx: the groups and lines of cheatsheetLayout, then every row the layout does not place, under its menu's group (grouped, menu.go), in a group of the layout of that name or in a group of its own after them. So the sheet lists the whole table, and a row an area adds shows on it before the layout names it.
func (a *app) cheatsheetGroups(cx *core.Scope) []cheatsheet.Group {
	rows := slices.DeleteFunc(a.shortcuts(cx), func(s shortcut) bool { return s.Key == "" })
	return cheatsheetGroupsOf(cx, rows)
}

// cheatsheetGroupsOf arranges rows of the shortcut table as cheatsheetGroups describes.
func cheatsheetGroupsOf(cx *core.Scope, rows []shortcut) []cheatsheet.Group {
	labels := map[string]string{}
	for _, s := range rows {
		if _, ok := labels[s.Key]; !ok {
			labels[s.Key] = s.Label
		}
	}
	placed := map[string]bool{}
	at := map[string]int{}
	var groups []cheatsheet.Group
	for _, g := range cheatsheetLayout(cx) {
		var keys []cheatsheet.Key
		for _, l := range g.lines {
			if slices.ContainsFunc(l.combos, func(k string) bool { _, ok := labels[k]; return !ok }) {
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
			if placed[s.Key] {
				continue
			}
			placed[s.Key] = true
			i, ok := at[group]
			if !ok {
				i = len(groups)
				at[group] = i
				groups = append(groups, cheatsheet.Group{Title: groupTitle(cx, group)})
			}
			groups[i].Keys = append(groups[i].Keys, cheatsheet.Key{Label: s.Label, Combos: []string{s.Key}})
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
		{Group: groupHelp, Label: locales.CheatsheetMenuItem(cx), Key: cheatsheetKey, Run: a.toggleCheatsheet},
	}
}

// cheatsheetKey is ⌘/, which opens the cheat sheet and closes it again; the palette lists its row first while nothing is typed (paletteFound).
const cheatsheetKey = "mod+/"
