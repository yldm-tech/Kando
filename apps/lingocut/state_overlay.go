package main

import (
	"slices"

	"lingocut/apps/lingocut/cheatsheet"

	"github.com/egoist/mygo/ui"
)

// overlayUI is what is drawn over the window's pages: the shortcut cheat sheet, and later the first-run guide and the command palette.
type overlayUI struct {
	cheatsheet bool
	// onboarded is saved once the first-run guide has been seen.
	onboarded bool
	// modal counts the modal dialogs up over the window: a system file panel (dialogs.go), or a sheet or alert an area draws. AppKit keeps the menu bar's items enabled under a window's sheet, so the menu's commands wait while modalOpen is true. Change it only through holdModal.
	modal int
}

func newOverlayUI() overlayUI { return overlayUI{} }

// overlays draws whatever is open over the pages. view calls it last, so it sits above the sidebar and the content. t is the window's theme and dark the player's.
func (a *app) overlays(c *ui.Context, t, dark *ui.Theme) {
	if a.overlay.cheatsheet {
		cheatsheet.Show(c, dark, &a.overlay.cheatsheet, a.cheatsheetGroups())
	}
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

// cheatsheetLayout is the cheat sheet's own arrangement of the shortcut table, the prototype's: its groups in order, and lines that name rows of the table by their keys, joining several under one label, as 后退 / 前进 5 秒 shows ← →. A line shows only once the table has all its keys; an empty label takes the label of its one row. The prototype's lines for shortcuts the app does not have yet are left out until their areas add them.
var cheatsheetLayout = []struct {
	title string
	lines []cheatsheetLine
}{
	{"播放", []cheatsheetLine{
		{"播放 / 暂停", []cheatsheet.Combo{{Key: ui.KeySpace}}},
		{"后退 / 前进 5 秒", []cheatsheet.Combo{{Key: ui.KeyLeft}, {Key: ui.KeyRight}}},
		{"后退 / 前进 30 秒", []cheatsheet.Combo{{Mods: ui.Cmd, Key: ui.KeyLeft}, {Mods: ui.Cmd, Key: ui.KeyRight}}},
		{"", []cheatsheet.Combo{{Key: ui.KeyF}}},
	}},
	{"字幕", []cheatsheetLine{
		{"显示 / 隐藏", []cheatsheet.Combo{{Key: ui.KeyV}}},
		{"切换显示方式", []cheatsheet.Combo{{Key: ui.KeyS}}},
		{"字幕延迟 −/+ 0.1 秒", []cheatsheet.Combo{{Key: ui.KeyZ}, {Key: ui.KeyX}}},
		{"侧栏", []cheatsheet.Combo{{Mods: ui.Cmd | ui.Shift, Key: ui.KeyT}}},
		{"搜索台词", []cheatsheet.Combo{{Mods: ui.Cmd, Key: ui.KeyF}}},
	}},
	{"窗口", []cheatsheetLine{
		{"", []cheatsheet.Combo{{Mods: ui.Cmd, Key: ui.KeySlash}}},
		{"设置", []cheatsheet.Combo{{Mods: ui.Cmd, Key: ui.KeyComma}}},
		{"", []cheatsheet.Combo{{Key: ui.KeyEscape}}},
	}},
}

// cheatsheetLine is a line of cheatsheetLayout.
type cheatsheetLine struct {
	label  string
	combos []cheatsheet.Combo
}

// cheatsheetGroups is the shortcut table as the cheat sheet lists it: the groups and lines of cheatsheetLayout, then every row the layout does not place, under its menu's group (grouped, menu.go), in a group of the layout of that name or in a group of its own after them. So the sheet lists the whole table, and a row an area adds shows on it before the layout names it.
func (a *app) cheatsheetGroups() []cheatsheet.Group { return cheatsheetGroupsOf(a.shortcuts()) }

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
	for _, g := range cheatsheetLayout {
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
			at[g.title] = len(groups)
			groups = append(groups, cheatsheet.Group{Title: g.title, Keys: keys})
		}
	}
	order, byGroup := grouped(rows)
	for _, title := range order {
		for _, s := range byGroup[title] {
			k := cheatsheet.Combo{Mods: s.Mods, Key: s.Key}
			if placed[k] {
				continue
			}
			placed[k] = true
			i, ok := at[title]
			if !ok {
				i = len(groups)
				at[title] = i
				groups = append(groups, cheatsheet.Group{Title: title})
			}
			groups[i].Keys = append(groups[i].Keys, cheatsheet.Key{Label: s.Label, Combos: []cheatsheet.Combo{k}})
		}
	}
	return groups
}

// savedOverlay is the overlays' part of settings.json.
type savedOverlay struct {
	Onboarded bool `json:"onboarded,omitempty"`
}

func (s *savedOverlay) clean() {}

// overlayHooks: the overlays save whether the first-run guide was seen.
func (a *app) overlayHooks() areaHooks {
	return areaHooks{
		save:    func(saved *savedSettings) { saved.Onboarded = a.overlay.onboarded },
		restore: func(saved savedSettings) { a.overlay.onboarded = saved.Onboarded },
	}
}

// overlayKeys are the shortcuts of the overlays.
func (a *app) overlayKeys() []shortcut {
	return []shortcut{
		{Group: "帮助", Label: "快捷键速查", Mods: ui.Cmd, Key: ui.KeySlash, Run: a.toggleCheatsheet},
	}
}
