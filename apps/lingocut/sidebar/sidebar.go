package sidebar

import (
	"fmt"
	"path/filepath"
	"strings"

	"lingocut/packages/media"

	"github.com/egoist/mygo/ui"
	"github.com/oneo-me/onevra/source/components"
	"github.com/oneo-me/onevra/source/icons/lucide"
)

const (
	PageFilms     = ""
	PageYouTube   = "youtube"
	PageGeneral   = "general"
	PageSpeech    = "speech"
	PageAI        = "ai"
	PageTranslate = "translate"
	PageAccount   = "account"

	Width        float32 = 248
	PagePadX     float32 = 32
	CardMinWidth float32 = 280
	CardGap      float32 = 18
)

// App is the shelf and navigation the sidebar reads and reports.
type App interface {
	LibraryDir() string
	KnownLibraries() []string
	SaveErr() string
	Page() string
	Select(page string)
	Choose(dir string)
	BeginNew()
	PickLibrary()
	Reveal(path string)
	Forget(dir string)
}

func Show(c *ui.Context, t *ui.Theme, count int, logo *ui.Bitmap, a App) {
	components.Sidebar(c).Width(Width).Shrink(0).Children(func() {
		switcher(c, t, count, logo, a)
		components.SidebarSection(c, "影片", func() {
			nav(c, a, PageFilms, "全部影片", fmt.Sprintf("%d", count))
		})
		components.SidebarSection(c, "来源", func() {
			nav(c, a, PageYouTube, "YouTube", "")
		})
		components.SidebarSection(c, "设置", func() {
			nav(c, a, PageGeneral, "通用", "")
			nav(c, a, PageSpeech, "语音识别", "")
			nav(c, a, PageAI, "AI 服务", "")
			nav(c, a, PageTranslate, "翻译", "")
			nav(c, a, PageAccount, "账号", "")
		})
		if a.SaveErr() != "" {
			ui.Text(c, a.SaveErr()).TextColor(t.Danger).Wrap().Padding(8, 8, 0, 8)
		}
	})
}

// switcher is the current library. Its menu lists the others and the actions that used to sit in the sidebar.
func switcher(c *ui.Context, t *ui.Theme, count int, logo *ui.Bitmap, a App) {
	title := "选择资料库"
	if dir := a.LibraryDir(); dir != "" {
		title = Title(dir)
	}
	head := ui.Row(c).Padding(4, 8, 14, 8).Gap(10).AlignItems(ui.Center).Cursor(ui.CursorPointer).Label("资料库菜单")
	head.PopoverMenuButton(func(m *ui.Menu) { Menu(m, a) })
	head.Children(func() {
		ui.Image(c, logo).Size(28, 28).Shrink(0)
		ui.Column(c).Gap(1).Grow(1).MinWidth(0).Children(func() {
			ui.Text(c, title).FontSize(15).Bold()
			ui.Text(c, fmt.Sprintf("%d 部影片", count)).TextColor(t.TextMuted).FontSize(12)
		})
		components.Icon(c, lucide.ChevronDown).Size(14, 14).TextColor(t.TextMuted).Shrink(0)
	})
}

func Menu(m *ui.Menu, a App) {
	currentDir := a.LibraryDir()
	for _, dir := range a.KnownLibraries() {
		dir := dir
		current := filepath.Clean(currentDir) == filepath.Clean(dir)
		if m.Item(Title(dir)).Radio(current).Chosen() {
			a.Choose(dir)
		}
	}
	if m.Item("新建资料库...").Chosen() {
		a.BeginNew()
	}
	if m.Item("打开资料库...").Chosen() {
		a.PickLibrary()
	}
	m.Separator()
	if m.Item("在 Finder 中显示").Disabled(currentDir == "").Chosen() {
		a.Reveal(currentDir)
	}
	if len(a.KnownLibraries()) > 1 {
		if m.Item("从列表移出").Chosen() {
			a.Forget(currentDir)
		}
	}
}

func Title(dir string) string {
	base := filepath.Base(dir)
	base = strings.TrimSuffix(base, media.LibraryExt)
	if base == "" || base == "." || base == string(filepath.Separator) {
		return dir
	}
	return base
}

func nav(c *ui.Context, a App, page, label, badge string) {
	var trailing func(*ui.Context, float32)
	if badge != "" {
		trailing = func(c *ui.Context, _ float32) {
			components.Text(c, badge).TextColor(c.Theme().TextMuted)
		}
	}
	item := components.SidebarItemWithSlots(c, label, a.Page() == page, func(c *ui.Context, size float32) {
		components.Icon(c, navLucide(page)).Size(size, size)
	}, trailing)
	if item.Clicked() {
		a.Select(page)
	}
}

func navLucide(page string) *ui.SVG {
	switch page {
	case PageYouTube:
		return lucide.TvMinimalPlay()
	case PageGeneral:
		return lucide.Settings()
	case PageSpeech:
		return lucide.AudioLines()
	case PageAI:
		return lucide.Sparkles()
	case PageTranslate:
		return lucide.Languages()
	case PageAccount:
		return lucide.User()
	default:
		return lucide.Clapperboard()
	}
}
