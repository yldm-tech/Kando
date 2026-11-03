package main

import (
	"fmt"
	"path/filepath"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

func (a *app) sidebar(c *ui.Context, t *ui.Theme, count int) {
	components.Sidebar(c).Width(sidebarWidth).Shrink(0).Children(func() {
		a.librarySwitcher(c, t, count)
		components.SidebarSection(c, "影片", func() {
			a.nav(c, pageFilms, "全部影片", fmt.Sprintf("%d", count))
		})
		components.SidebarSection(c, "来源", func() {
			a.nav(c, pageYouTube, "YouTube", "")
		})
		components.SidebarSection(c, "设置", func() {
			a.nav(c, pageGeneral, "通用", "")
			a.nav(c, pageSpeech, "语音识别", "")
			a.nav(c, pageAI, "AI 服务", "")
			a.nav(c, pageTranslate, "翻译", "")
		})
		if a.saveErr != "" {
			ui.Text(c, a.saveErr).TextColor(t.Danger).Wrap().Padding(8, 8, 0, 8)
		}
	})
}

// librarySwitcher is the current library. Its menu lists the others and the actions that used to sit in the sidebar.
func (a *app) librarySwitcher(c *ui.Context, t *ui.Theme, count int) {
	title := "选择资料库"
	if dir := a.snap().LibraryDir; dir != "" {
		title = libraryTitle(dir)
	}
	head := ui.Row(c).Padding(4, 8, 14, 8).Gap(10).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label("资料库菜单")
	head.PopoverMenuButton(func(m *ui.Menu) { a.libraryMenu(m) })
	head.Children(func() {
		ui.Image(c, appLogo).Size(28, 28).Shrink(0)
		ui.Column(c).Gap(1).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, title).FontSize(15).Bold()
			ui.Text(c, countLabel(count)).TextColor(t.TextMuted).FontSize(12)
		})
		components.Icon(c, lucide.ChevronDown).Size(14, 14).TextColor(t.TextMuted).Shrink(0)
	})
}

func (a *app) libraryMenu(m *ui.Menu) {
	currentDir := a.snap().LibraryDir
	for _, dir := range a.knownLibraries() {
		dir := dir
		current := filepath.Clean(currentDir) == filepath.Clean(dir)
		if m.Item(libraryTitle(dir)).Radio(current).Chosen() {
			a.edit(func(s *session) { s.MakingLibrary = false })
			a.page = pageFilms
			a.switchLibrary(dir)
		}
	}
	if m.Item("新建资料库...").Chosen() {
		a.beginNewLibrary()
	}
	if m.Item("打开资料库...").Chosen() {
		a.pickLibrary()
	}
	m.Separator()
	if m.Item("在 Finder 中显示").Disabled(currentDir == "").Chosen() {
		a.reveal(currentDir)
	}
	if len(a.knownLibraries()) > 1 {
		if m.Item("从列表移出").Chosen() {
			a.forgetLibrary(currentDir)
		}
	}
}

func (a *app) nav(c *ui.Context, page, label, badge string) {
	var trailing func(*ui.Context, float32)
	if badge != "" {
		trailing = func(c *ui.Context, _ float32) {
			components.Text(c, badge).TextColor(c.Theme().TextMuted)
		}
	}
	item := components.SidebarItemWithSlots(c, label, a.page == page, func(c *ui.Context, size float32) {
		components.Icon(c, navLucide(page)).Size(size, size)
	}, trailing)
	if item.Clicked() {
		a.page = page
		a.openKey = ""
		a.stopPlayback()
	}
}

func navLucide(page string) *ui.SVG {
	switch page {
	case pageYouTube:
		return lucide.TvMinimalPlay()
	case pageGeneral:
		return lucide.Settings()
	case pageSpeech:
		return lucide.AudioLines()
	case pageAI:
		return lucide.Sparkles()
	case pageTranslate:
		return lucide.Languages()
	default:
		return lucide.Clapperboard()
	}
}

func countLabel(n int) string {
	return fmt.Sprintf("%d 部影片", n)
}
