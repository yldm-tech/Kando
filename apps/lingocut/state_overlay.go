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

// cheatsheetGroups is the shortcut table grouped for the cheat sheet, in the menu bar's order of the groups (grouped, menu.go).
func (a *app) cheatsheetGroups() []cheatsheet.Group {
	order, rows := grouped(a.shortcuts())
	var groups []cheatsheet.Group
	for _, title := range order {
		if len(rows[title]) == 0 {
			continue
		}
		g := cheatsheet.Group{Title: title}
		for _, s := range rows[title] {
			g.Keys = append(g.Keys, cheatsheet.Key{Label: s.Label, Mods: s.Mods, Key: s.Key})
		}
		groups = append(groups, g)
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
