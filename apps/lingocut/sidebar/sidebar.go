package sidebar

import (
	"fmt"
	"path/filepath"
	"strings"

	"lingocut/packages/media"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
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
	ui.Column(c).Width(Width).Shrink(0).Grow(1).Children(func() {
		switcher(c, t, count, logo, a)
		// The sidebar writes a click or an arrow key on another item into sel and Select runs when the page changes; a click on the item already chosen leaves sel alone, so nav calls Select itself to close the open film and go back to the page's grid.
		sel := a.Page()
		breeze.Sidebar(c, &sel, func() {
			breeze.SidebarSection(c, "影片", nil, func() {
				nav(c, a, PageFilms, "全部影片", fmt.Sprintf("%d", count))
			})
			breeze.SidebarSection(c, "来源", nil, func() {
				nav(c, a, PageYouTube, "YouTube", "")
			})
			breeze.SidebarSection(c, "设置", nil, func() {
				nav(c, a, PageGeneral, "通用", "")
				nav(c, a, PageSpeech, "语音识别", "")
				nav(c, a, PageAI, "AI 服务", "")
				nav(c, a, PageTranslate, "翻译", "")
				nav(c, a, PageAccount, "账号", "")
			})
		}).Background(ui.Transparent).Grow(1)
		if sel != a.Page() {
			a.Select(sel)
		}
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
		ui.Icon(c, iconChevronDown).Size(14, 14).TextColor(t.TextMuted).Shrink(0)
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
	item := breeze.SidebarItem(c, page, navIcon(page), label)
	if item.Clicked() && page == a.Page() {
		a.Select(page)
	}
	if badge != "" {
		item.Children(func() {
			breeze.Text(c, badge, breeze.TextSecondary)
		})
	}
}

func navIcon(page string) *ui.SVG {
	switch page {
	case PageYouTube:
		return iconTvMinimalPlay
	case PageGeneral:
		return iconSettings
	case PageSpeech:
		return iconAudioLines
	case PageAI:
		return iconSparkles
	case PageTranslate:
		return iconLanguages
	case PageAccount:
		return iconUser
	default:
		return iconClapperboard
	}
}
