package main

import (
	"fmt"
	"path/filepath"
	"runtime"
	"slices"
	"strings"

	"kando/apps/kando/desktop"
	"kando/apps/kando/films"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
)

// menuOrder is the order of the menu bar's menus. A shortcut's Group names the menu it is listed in, so the menu bar, the cheat sheet and wireKeys read the same table. A group not named here gets a menu of its own before 窗口.
var menuOrder = []string{groupApp, groupFile, groupEdit, groupPlayback, groupSubtitles, groupView, groupStudy, groupWindow, groupHelp}

// rememberFilm keeps the film the window shows, for 窗口 › 播放器 to go back to after the window went to another page. menuHooks.afterChange calls it after every change, once the pages have opened or closed a film.
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

// installMenu gives the app its own menu bar on macOS, where the menu bar belongs to the app, under app, the app's scope; run calls it once the main window is open. From then on the menu's shortcuts run the ⌘ rows and wireKeys leaves them alone. Linux and Windows put an app's menu bar inside each window, which would push the window's own title bar down, so there the window keeps binding every shortcut itself. The menu follows the app's state and its language: an effect builds it after every change, and syncMenu installs it when it shows something else.
func (a *app) installMenu(app *core.Scope) {
	if !appMenuBar {
		return
	}
	a.menu.menuBar = true
	// holder owns the installed menu's handlers, outside the effect, whose next run would otherwise take the menu away with them.
	holder := app.Child()
	app.Effect(func() {
		a.Track()
		items := a.menuBar(app)
		app.Untrack(func() { a.syncMenu(holder, items) })
	})
}

// syncMenu installs items as the menu bar when they differ from the menu installed, so a change that alters none of it leaves the menu bar alone, even while one of its menus is open. Each menu is installed under a scope of its own in holder, and the one before goes once the new one is in: the items' handlers run under that scope, and the menu bar goes back to the platform's default when it goes. Both hops are posts, as code an effect runs owns what it makes.
func (a *app) syncMenu(holder *core.Scope, items []nsdesktop.MenuItem) {
	sig := menuSignature(items)
	if sig == a.menu.menuSig {
		return
	}
	a.menu.menuSig = sig
	holder.Post(func() {
		next := holder.Child()
		next.Post(func() {
			nsdesktop.SetApplicationMenu(next, items)
			if a.menu.installed != nil {
				a.menu.installed.Dispose()
			}
			a.menu.installed = next
		})
	})
	// TODO(nsgo): give the Dock icon a menu of the libraries, as File › 切换资料库 lists them (menuLibraries) and MyGo's App.Dock.SetMenu did; NSGo has no Dock or taskbar menu
}

// menuBar builds the menu bar from the shortcut table and the app's state now, its words in the language the app shows under cx. Each row of the table is an item of the menu its Group names, enabled while its When holds; the rows without a key come last in their menu, after a separator. A ⌘ row carries its key as the item's shortcut, which runs it. A row without ⌘ shows no shortcut in the menu, because the menu would take its key before a focused text field could; its key is on the cheat sheet. While the cheat sheet is open, only the overlays' own rows stay enabled, as the window's shortcuts wait behind it; while a modal dialog such as a file panel is up, none of the rows are (holds).
func (a *app) menuBar(cx *core.Scope) []nsdesktop.MenuItem {
	table := a.shortcuts(cx)
	order, rows := grouped(table)
	carrier := menuSharedKeyCarriers(table)
	own := map[string]bool{}
	for _, s := range a.overlayKeys(cx) {
		own[s.Key] = true
	}
	separator := nsdesktop.MenuItem{Separator: true}
	// then adds a separator before more, when there is something on both sides of it.
	then := func(head []nsdesktop.MenuItem, more ...nsdesktop.MenuItem) []nsdesktop.MenuItem {
		if len(head) == 0 || len(more) == 0 {
			return append(head, more...)
		}
		return append(append(head, separator), more...)
	}
	role := func(label string, r nsdesktop.MenuRole) nsdesktop.MenuItem { return nsdesktop.MenuItem{Label: label, Role: r} }
	// items are the group's rows, the ones with a key first and the menu's own commands without one after a separator. extra are items of the menu alone that go with the rows that have a key, before that separator.
	items := func(group string, extra ...nsdesktop.MenuItem) []nsdesktop.MenuItem {
		var keyed, plain []nsdesktop.MenuItem
		for _, s := range rows[group] {
			it := a.menuItem(s, s.Key != "" && own[s.Key])
			if id, shared := carrier[s.Key]; shared && id != menuRowID(s) {
				it.Shortcut = ""
			}
			if s.Key == "" {
				plain = append(plain, it)
			} else {
				keyed = append(keyed, it)
			}
		}
		return then(append(keyed, extra...), plain...)
	}
	var bar []nsdesktop.MenuItem
	for _, group := range order {
		var sub []nsdesktop.MenuItem
		switch group {
		case groupApp:
			sub = then([]nsdesktop.MenuItem{role(locales.MenuAbout(cx), nsdesktop.MenuAbout)}, items(group)...)
			// TODO(nsgo#118): put the system's Services submenu here, as MyGo's RoleServices did; NSGo has no Services role https://github.com/nsgo-dev/nsgo/issues/118
			sub = then(sub, role(locales.MenuHide(cx), nsdesktop.MenuHide), role(locales.MenuHideOthers(cx), nsdesktop.MenuHideOthers), role(locales.MenuShowAll(cx), nsdesktop.MenuShowAll))
			// Quit is the app's own item, not MenuQuit's, so a running job can ask first (requestQuit).
			sub = then(sub, nsdesktop.MenuItem{Label: locales.MenuQuit(cx), Shortcut: "mod+q", OnSelect: live.Do(a, a.requestQuit)})
		case groupFile:
			reveal := nsdesktop.MenuItem{Label: locales.MenuRevealLibrary(cx, desktop.Platform(cx)), Disabled: a.holds(false) || a.snap().LibraryDir == "", OnSelect: a.menuSelect(false, func() { a.reveal(a.snap().LibraryDir) })}
			library := []nsdesktop.MenuItem{reveal}
			if libs := a.menuLibraries(); len(libs) > 0 {
				library = append([]nsdesktop.MenuItem{{Label: locales.MenuSwitchLibrary(cx), Items: libs}}, library...)
			}
			// 关闭窗口 keeps ⌘W, which the default menu bar has in its File menu.
			sub = then(then(items(group), library...), role(locales.MenuCloseWindow(cx), nsdesktop.MenuClose))
		case groupEdit:
			sub = []nsdesktop.MenuItem{role(locales.MenuUndo(cx), nsdesktop.MenuUndo), role(locales.MenuRedo(cx), nsdesktop.MenuRedo)}
			// TODO(nsgo#118): add 粘贴并匹配样式, 删除 and the 语音 submenu's 开始朗读 and 停止朗读, as MyGo's RolePasteAndMatchStyle, RoleDelete, RoleStartSpeaking and RoleStopSpeaking gave the Edit menu; NSGo has no such roles https://github.com/nsgo-dev/nsgo/issues/118
			sub = then(sub, role(locales.MenuCut(cx), nsdesktop.MenuCut), role(locales.MenuCopy(cx), nsdesktop.MenuCopy), role(locales.MenuPaste(cx), nsdesktop.MenuPaste), role(locales.MenuSelectAll(cx), nsdesktop.MenuSelectAll))
			sub = then(sub, items(group)...)
		case groupView:
			// 切换全屏幕 keeps ⌃⌘F on every page, as the default menu bar's View menu has it. The table's 全屏 is the player's single key F, which works only while a film is open.
			sub = then(items(group), role(locales.MenuToggleFullScreen(cx), nsdesktop.MenuToggleFullScreen))
		case groupWindow:
			sub = then([]nsdesktop.MenuItem{role(locales.MenuMinimize(cx), nsdesktop.MenuMinimize), role(locales.MenuZoom(cx), nsdesktop.MenuZoom)}, items(group)...)
			// TODO(nsgo#118): end the Window menu with 前置全部窗口 and make it the system's Window menu, which lists the open windows, as MyGo's RoleFront and RoleWindow did https://github.com/nsgo-dev/nsgo/issues/118
		case groupHelp:
			// With a menu bar, 重新查看入门引导 is the menu's own item (state_onboarding.go), placed in the prototype's order: 快捷键速查, 重新查看入门引导, then after a separator the plain rows, the website and release-notes links (state_menu.go) and then the feedback items.
			// TODO(nsgo#118): make it the system's Help menu, with its search field, as MyGo's RoleHelp did https://github.com/nsgo-dev/nsgo/issues/118
			var guide []nsdesktop.MenuItem
			if appMenuBar {
				guide = append(guide, nsdesktop.MenuItem{Label: locales.MenuReopenGuide(cx), Disabled: a.holds(false), OnSelect: a.menuSelect(false, a.reopenOnboarding)})
			}
			sub = items(group, guide...)
		default:
			sub = items(group)
		}
		if len(sub) > 0 {
			bar = append(bar, nsdesktop.MenuItem{Label: groupTitle(cx, group), Items: sub})
		}
	}
	return bar
}

// menuLibraries are the items of File › 切换资料库: an item for each library the app knows, named as Settings › 通用 names them, with the open one checked. Choosing one switches to it as the sidebar's switcher does.
func (a *app) menuLibraries() []nsdesktop.MenuItem {
	dirs := a.knownLibraries()
	labels := films.LibraryLabels(dirs)
	open := filepath.Clean(a.snap().LibraryDir)
	items := make([]nsdesktop.MenuItem, len(dirs))
	for i, dir := range dirs {
		// TODO(nsgo#118): make the libraries radio items, one of which is chosen, as MyGo's MenuItemRadio drew them; NSGo's menu items are only checkable https://github.com/nsgo-dev/nsgo/issues/118
		items[i] = nsdesktop.MenuItem{
			Label:     labels[i],
			Checkable: true,
			Checked:   filepath.Clean(dir) == open,
			Disabled:  a.holds(false),
			OnSelect: a.menuSelect(false, func() {
				// The menu may flip the ✓ itself; forgetting the installed menu sets it from the app's state again.
				a.menu.menuSig = ""
				a.Choose(dir)
			}),
		}
	}
	return items
}

// menuItem is the menu bar's item for the shortcut s. own says s is one of the overlays' rows, which the cheat sheet leaves enabled.
func (a *app) menuItem(s shortcut, own bool) nsdesktop.MenuItem {
	it := nsdesktop.MenuItem{Label: s.Label, Disabled: a.holds(own) || (s.When != nil && !s.When()), When: core.MenuAuto}
	if hasCommand(s.Key) {
		it.Shortcut = s.Key
	}
	run := s.guarded()
	if s.Group == groupWindow {
		// A row of the Window menu acts on the main window, so it brings the window back first when its close button put it away (state_window.go): 片库 and 播放器 would otherwise change a window no one sees, and 播放器 start a film's sound behind it.
		next := run
		run = func() {
			a.windowUnhide()
			next()
		}
	}
	if s.Checked != nil {
		it.Checkable, it.Checked = true, s.Checked()
		// The menu may flip a checkable item's ✓ itself; forgetting the installed menu sets the ✓ from the app's state again.
		next := run
		run = func() {
			a.menu.menuSig = ""
			next()
		}
	}
	it.OnSelect = a.menuSelect(own, run)
	return it
}

// menuSharedKeyCarriers picks, for each key more than one row of the table has, the row whose menu item carries it as its shortcut (by menuRowID): the first whose When holds, or the first of them while none does. Rows share a key only when they take turns, as the library's ⌘F and the player's do (TestShortcutsAreUnique), so the key runs the item it is shown by, and the menu bar never gives two items one shortcut.
func menuSharedKeyCarriers(rows []shortcut) map[string]string {
	by := map[string][]shortcut{}
	for _, s := range rows {
		if s.Key != "" {
			by[s.Key] = append(by[s.Key], s)
		}
	}
	carrier := map[string]string{}
	for k, same := range by {
		if len(same) < 2 {
			continue
		}
		pick := same[0]
		for _, s := range same {
			if s.When != nil && s.When() {
				pick = s
				break
			}
		}
		carrier[k] = menuRowID(pick)
	}
	return carrier
}

// menuRowID names a row of the shortcut table by its menu and its label, which no two rows share.
func menuRowID(s shortcut) string { return s.Group + "\x00" + s.Label }

// menuSelect is an item's OnSelect: fn, unless whatever is open over the window by then holds it back as a disabled item would have (holds, with own saying fn is one of the overlays'), as the click can come from a menu built before it opened. Either way the revision follows (package live).
func (a *app) menuSelect(own bool, fn func()) func() {
	return live.Do(a, func() {
		if a.holds(own) {
			return
		}
		fn()
	})
}

// holds reports whether something open over the window holds back a menu bar command. own says the command is one of the overlays' rows: the cheat sheet holds back every command but those, so its own ⌘/ closes it, and a modal dialog (holdModal), such as a file panel, holds back all of them.
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
func menuSignature(items []nsdesktop.MenuItem) string {
	var b strings.Builder
	var walk func([]nsdesktop.MenuItem)
	walk = func(items []nsdesktop.MenuItem) {
		for _, it := range items {
			fmt.Fprintf(&b, "%d|%s|%s|%t|%t|%t|%t[", it.Role, it.Label, it.Shortcut, it.Separator, it.Disabled, it.Checkable, it.Checked)
			walk(it.Items)
			b.WriteString("]")
		}
	}
	walk(items)
	return b.String()
}
