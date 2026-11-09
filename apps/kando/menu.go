package main

import (
	"fmt"
	"runtime"
	"slices"
	"strings"

	"kando/apps/kando/films"
	"kando/apps/kando/locales"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// menuOrder is the order of the menu bar's menus. A shortcut's Group names the menu it is listed in, so the menu bar, the cheat sheet and wireKeys read the same table. A group not named here gets a menu of its own before 窗口.
var menuOrder = []string{groupApp, groupFile, groupEdit, groupPlayback, groupSubtitles, groupView, groupStudy, groupWindow, groupHelp}

// rememberFilm keeps the film the window shows, for 窗口 › 播放器 to go back to after the window went to another page. menuHooks.afterFrame calls it every frame, after the pages have opened or closed a film.
func (a *app) rememberFilm() {
	if a.openKey != "" {
		a.menu.lastFilm = a.openKey
	}
}

// lastFilm is the film the window showed last, while it is still on the open library's shelf.
func (a *app) lastFilm() (films.Film, bool) {
	if a.menu.lastFilm == "" {
		return films.Film{}, false
	}
	return films.Find(a.films(), a.menu.lastFilm)
}

// reopenFilm goes back to the film the window showed last, from whatever page it shows now.
func (a *app) reopenFilm() {
	item, ok := a.lastFilm()
	if !ok || a.openKey == item.Key {
		return
	}
	a.Select(pageFilms)
	a.Open(item)
}

// appMenuBar says the app gets a menu bar of its own (installMenu): on macOS, where the menu bar belongs to the app. Commands that are items of the menu bar alone, such as 帮助 › 重新查看入门引导, need another way in where it is false. Tests set it to check the other platforms.
var appMenuBar = runtime.GOOS == "darwin"

// installMenu gives the app its own menu bar on macOS, where the menu bar belongs to the app. menuHooks.ready calls it once the window exists. From then on the menu's accelerators run the ⌘ shortcuts and wireKeys leaves them alone. Linux and Windows put an app's menu bar inside each window, which would push the window's own title bar down, so there the window keeps running every shortcut itself.
func (a *app) installMenu() {
	if !appMenuBar {
		return
	}
	a.menu.menuBar = true
	a.syncMenu()
}

// syncMenu brings the menu bar up to date with the app's state: which items are enabled, which carry a ✓, and their labels. It installs a new menu only when that differs from the one installed, so a frame that changes none of it leaves the menu bar alone, even while one of its menus is open. menuHooks.afterFrame calls it every frame, after the pages have settled the frame's state.
func (a *app) syncMenu() {
	if !a.menu.menuBar {
		return
	}
	items := a.menuBar()
	sig := menuSignature(items)
	if sig == a.menu.menuSig {
		return
	}
	a.menu.menuSig = sig
	mygo.App.SetMenu(mygo.NewMenu(items))
}

// menuBar builds the menu bar from the shortcut table and the app's state now. Each row of the table is an item of the menu its Group names, enabled while its When holds; the rows without a key come last in their menu, after a separator. A ⌘ row carries its shortcut as the item's accelerator, which runs it. A row without ⌘ shows no shortcut in the menu, because the menu would take its key before a focused text field could; its key is on the cheat sheet. While the cheat sheet is open, only the overlays' own rows stay enabled, as the window's shortcuts wait behind it; while a modal dialog such as a file panel is up as a sheet, none of the rows are (holds).
func (a *app) menuBar() []*mygo.MenuItem {
	order, rows := grouped(a.shortcuts())
	own := map[[2]int]bool{}
	for _, s := range a.overlayKeys() {
		own[[2]int{int(s.Mods), int(s.Key)}] = true
	}
	// then adds a separator before more, when there is something on both sides of it.
	then := func(head []*mygo.MenuItem, more ...*mygo.MenuItem) []*mygo.MenuItem {
		if len(head) == 0 || len(more) == 0 {
			return append(head, more...)
		}
		return append(append(head, mygo.Separator()), more...)
	}
	// items are the group's rows, the ones with a key first and the menu's own commands without one after a separator. extra are items of the menu alone that go with the rows that have a key, before that separator.
	items := func(group string, extra ...*mygo.MenuItem) []*mygo.MenuItem {
		var keyed, plain []*mygo.MenuItem
		for _, s := range rows[group] {
			it := a.menuItem(s, s.Key != 0 && own[[2]int{int(s.Mods), int(s.Key)}])
			if s.Key == 0 {
				plain = append(plain, it)
			} else {
				keyed = append(keyed, it)
			}
		}
		return then(append(keyed, extra...), plain...)
	}
	var bar []*mygo.MenuItem
	for _, group := range order {
		var menu *mygo.MenuItem
		switch group {
		case groupApp:
			sub := then([]*mygo.MenuItem{{Label: locales.MenuAbout(), Role: mygo.RoleAbout}}, items(group)...)
			sub = then(sub, &mygo.MenuItem{Label: locales.MenuServices(), Role: mygo.RoleServices})
			sub = then(sub, &mygo.MenuItem{Label: locales.MenuHide(), Role: mygo.RoleHide}, &mygo.MenuItem{Label: locales.MenuHideOthers(), Role: mygo.RoleHideOthers}, &mygo.MenuItem{Label: locales.MenuShowAll(), Role: mygo.RoleUnhide})
			sub = then(sub, &mygo.MenuItem{Label: locales.MenuQuit(), Role: mygo.RoleQuit})
			menu = &mygo.MenuItem{Label: groupTitle(group), Submenu: sub}
		case groupFile:
			reveal := &mygo.MenuItem{Label: locales.MenuRevealLibrary(), Disabled: a.holds(false) || a.snap().LibraryDir == "", Click: a.menuClick(false, func() { a.reveal(a.snap().LibraryDir) })}
			// 关闭窗口 keeps ⌘W, which the menu bar mygo installs for an app without its own has in its File menu.
			menu = &mygo.MenuItem{Label: groupTitle(group), Submenu: then(then(items(group), reveal), &mygo.MenuItem{Label: locales.MenuCloseWindow(), Role: mygo.RoleClose})}
		case groupEdit:
			sub := []*mygo.MenuItem{{Label: locales.MenuUndo(), Role: mygo.RoleUndo}, {Label: locales.MenuRedo(), Role: mygo.RoleRedo}}
			// 粘贴并匹配样式 and 语音 keep what the Edit menu of the menu bar mygo installs for an app without its own has.
			sub = then(sub, &mygo.MenuItem{Label: locales.MenuCut(), Role: mygo.RoleCut}, &mygo.MenuItem{Label: locales.MenuCopy(), Role: mygo.RoleCopy}, &mygo.MenuItem{Label: locales.MenuPaste(), Role: mygo.RolePaste}, &mygo.MenuItem{Label: locales.MenuPasteAndMatchStyle(), Role: mygo.RolePasteAndMatchStyle}, &mygo.MenuItem{Label: locales.MenuDelete(), Role: mygo.RoleDelete}, &mygo.MenuItem{Label: locales.MenuSelectAll(), Role: mygo.RoleSelectAll})
			sub = then(sub, &mygo.MenuItem{Label: locales.MenuSpeech(), Submenu: []*mygo.MenuItem{{Label: locales.MenuStartSpeaking(), Role: mygo.RoleStartSpeaking}, {Label: locales.MenuStopSpeaking(), Role: mygo.RoleStopSpeaking}}})
			menu = &mygo.MenuItem{Label: groupTitle(group), Submenu: then(sub, items(group)...)}
		case groupView:
			// 切换全屏幕 keeps ⌃⌘F on every page, as the View menu of the menu bar mygo installs for an app without its own has it. The table's 全屏 is the player's single key F, which works only while a film is open.
			menu = &mygo.MenuItem{Label: groupTitle(group), Submenu: then(items(group), &mygo.MenuItem{Label: locales.MenuToggleFullScreen(), Role: mygo.RoleToggleFullScreen})}
		case groupWindow:
			sub := then([]*mygo.MenuItem{{Label: locales.MenuMinimize(), Role: mygo.RoleMinimize}, {Label: locales.MenuZoom(), Role: mygo.RoleZoom}}, items(group)...)
			sub = then(sub, &mygo.MenuItem{Label: locales.MenuBringAllToFront(), Role: mygo.RoleFront})
			menu = &mygo.MenuItem{Label: groupTitle(group), Role: mygo.RoleWindow, Submenu: sub}
		case groupHelp:
			// With a menu bar, 重新查看入门引导 is the menu's own item (state_onboarding.go), placed in the prototype's order: 快捷键速查, 重新查看入门引导, then the feedback items after a separator.
			var guide []*mygo.MenuItem
			if appMenuBar {
				guide = append(guide, &mygo.MenuItem{Label: locales.MenuReopenGuide(), Disabled: a.holds(false), Click: a.menuClick(false, a.reopenOnboarding)})
			}
			if sub := items(group, guide...); len(sub) > 0 {
				menu = &mygo.MenuItem{Label: groupTitle(group), Role: mygo.RoleHelp, Submenu: sub}
			}
		default:
			if sub := items(group); len(sub) > 0 {
				menu = &mygo.MenuItem{Label: groupTitle(group), Submenu: sub}
			}
		}
		if menu != nil {
			bar = append(bar, menu)
		}
	}
	return bar
}

// menuItem is the menu bar's item for the shortcut s. own says s is one of the overlays' rows, which the cheat sheet leaves enabled.
func (a *app) menuItem(s shortcut, own bool) *mygo.MenuItem {
	it := &mygo.MenuItem{Label: s.Label, Disabled: a.holds(own) || (s.When != nil && !s.When())}
	if s.Mods&ui.Cmd != 0 {
		it.Accelerator = accelerator(s.Mods, s.Key)
	}
	run := s.guarded()
	if s.Checked != nil {
		it.Type, it.Checked = mygo.MenuItemCheckbox, s.Checked()
		// mygo flips a checkbox item's ✓ itself before Click; forgetting the installed menu makes the next frame set the ✓ from the app's state again.
		next := run
		run = func() {
			a.menu.menuSig = ""
			next()
		}
	}
	it.Click = a.menuClick(own, run)
	return it
}

// menuClick runs fn on the UI thread as a background job's report does, so the window draws what it changed. Whatever is open over the window by then holds fn back as a disabled item would have (holds, with own saying fn is one of the overlays'): the click can come from a menu built before it opened.
func (a *app) menuClick(own bool, fn func()) func(*mygo.MenuItem, *mygo.Window) {
	return func(*mygo.MenuItem, *mygo.Window) {
		a.update(func() {
			if a.holds(own) {
				return
			}
			fn()
		})
	}
}

// holds reports whether something open over the window holds back a menu bar command. own says the command is one of the overlays' rows: the cheat sheet holds back every command but those, so its own ⌘/ closes it, and a modal dialog (holdModal), such as a file panel up as a sheet, holds back all of them.
func (a *app) holds(own bool) bool { return a.modalOpen() || (a.overlay.cheatsheet && !own) }

// grouped returns the table's rows by group, and the groups in the menu bar's order: menuOrder, with the groups it does not name before 窗口 in the order the table first lists them.
func grouped(rows []shortcut) ([]string, map[string][]shortcut) {
	by := map[string][]shortcut{}
	var extra []string
	for _, s := range rows {
		if _, ok := by[s.Group]; !ok && !slices.Contains(menuOrder, s.Group) {
			extra = append(extra, s.Group)
		}
		by[s.Group] = append(by[s.Group], s)
	}
	at := slices.Index(menuOrder, groupWindow)
	return slices.Concat(menuOrder[:at], extra, menuOrder[at:]), by
}

// menuSignature writes down everything about items the menu bar shows, to tell whether a new menu would show anything else.
func menuSignature(items []*mygo.MenuItem) string {
	var b strings.Builder
	var walk func([]*mygo.MenuItem)
	walk = func(items []*mygo.MenuItem) {
		for _, it := range items {
			fmt.Fprintf(&b, "%s|%s|%s|%s|%t|%t|%t[", it.Type, it.Role, it.Label, it.Accelerator, it.Disabled, it.Hidden, it.Checked)
			walk(it.Submenu)
			b.WriteString("]")
		}
	}
	walk(items)
	return b.String()
}

// accelerator writes mods and key as mygo's menus take a shortcut, as in "Super+Shift+N", or "" for a key menus cannot show.
func accelerator(mods ui.Modifiers, key ui.Key) string {
	name := acceleratorKey(key)
	if name == "" {
		return ""
	}
	var b strings.Builder
	for _, m := range []struct {
		mod  ui.Modifiers
		name string
	}{{ui.Super, "Super"}, {ui.Ctrl, "Ctrl"}, {ui.Alt, "Alt"}, {ui.Shift, "Shift"}} {
		if mods&m.mod != 0 {
			b.WriteString(m.name + "+")
		}
	}
	return b.String() + name
}

func acceleratorKey(key ui.Key) string {
	switch {
	case key >= ui.KeyA && key <= ui.KeyZ:
		return string(rune('a' + key - ui.KeyA))
	case key >= ui.Key0 && key <= ui.Key9:
		return string(rune('0' + key - ui.Key0))
	case key >= ui.KeyF1 && key <= ui.KeyF12:
		return fmt.Sprintf("F%d", int(key-ui.KeyF1)+1)
	}
	switch key {
	case ui.KeyEnter:
		return "Enter"
	case ui.KeyEscape:
		return "Escape"
	case ui.KeyBackspace:
		return "Backspace"
	case ui.KeyTab:
		return "Tab"
	case ui.KeySpace:
		return "Space"
	case ui.KeyDelete:
		return "Delete"
	case ui.KeyHome:
		return "Home"
	case ui.KeyEnd:
		return "End"
	case ui.KeyPageUp:
		return "PageUp"
	case ui.KeyPageDown:
		return "PageDown"
	case ui.KeyLeft:
		return "Left"
	case ui.KeyRight:
		return "Right"
	case ui.KeyUp:
		return "Up"
	case ui.KeyDown:
		return "Down"
	case ui.KeyMinus:
		return "-"
	case ui.KeyEqual:
		return "="
	case ui.KeyComma:
		return ","
	case ui.KeyPeriod:
		return "."
	case ui.KeySlash:
		return "/"
	case ui.KeySemicolon:
		return ";"
	case ui.KeyQuote:
		return "'"
	case ui.KeyBracketLeft:
		return "["
	case ui.KeyBracketRight:
		return "]"
	case ui.KeyBackslash:
		return "\\"
	case ui.KeyBackquote:
		return "`"
	}
	return ""
}
