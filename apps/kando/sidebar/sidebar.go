package sidebar

import (
	"path/filepath"
	"strings"

	"kando/packages/core"
	"kando/packages/media"

	"github.com/egoist/mygo/ui"
)

const (
	PageFilms     = ""
	PageYouTube   = "youtube"
	PageGeneral   = "general"
	PageSpeech    = "speech"
	PageAI        = "ai"
	PageTranslate = "translate"
	PageAccount   = "account"

	// Width is the sidebar's column: the card and the gutter of the window's material around it.
	Width        float32 = 236
	PagePadX     float32 = 32
	CardMinWidth float32 = 280
	CardGap      float32 = 18
)

// Sources the library can be filtered by. Apart from SourceAll they are the platforms core.ParseLink reads from a film's source; a film without a source it reads is a local file.
const (
	SourceAll      = "all"
	SourceYouTube  = string(core.PlatformYouTube)
	SourceBilibili = string(core.PlatformBilibili)
	SourceWeb      = string(core.PlatformWeb)
	SourceLocal    = string(core.PlatformLocal)
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
	// FilmCount is how many films the open library holds.
	FilmCount() int
	// LibraryCount is how many films the library at dir holds, for a library that is not open; ok is false until the count has been read.
	LibraryCount(dir string) (n int, ok bool)
	// SourceFilter is the source the library page shows, SourceAll or another Source value.
	SourceFilter() string
	// SetSourceFilter shows the library page filtered by source and closes the open film.
	SetSourceFilter(source string)
	// SourceCount is how many films of the open library come from source.
	SourceCount(source string) int
	// OpenSettings shows the settings at section, one of the settings package's Section values; "" opens 通用.
	OpenSettings(section string)
	// Readiness is what the status line at the foot of the sidebar says.
	Readiness() Readiness
}

// The card the sidebar is drawn on, floating over the window's material: the prototype's white at 42% with a white inner hairline, a dark outer one and a soft shadow, 8 points in from the window's edges.
const (
	cardMargin float32 = 8
	cardRadius float32 = 14
	// insetStart and insetEnd are how far the switcher, the sources and the status line stand in from the card's edges. In the prototype the card is 231 wide and runs 3 points under the content, so they end 8 points before that hidden edge, 5 before the one that shows.
	insetStart float32 = 8
	insetEnd   float32 = 5
	// LightsRow is the empty row at the top of the card that the traffic lights sit in.
	LightsRow float32 = 48
)

var (
	cardFill      = ui.RGBA(255, 255, 255, 0.42)
	cardHighlight = ui.RGBA(255, 255, 255, 0.85)
	cardHairline  = ui.RGBA(0, 0, 0, 0.06)
	cardShadow    = ui.RGBA(30, 50, 90, 0.08)
	// gutterFill stands in for the window's material where the window has none (Tester, a window without vibrancy): the prototype's frosted strip over a white page.
	gutterFill = ui.Hex("#F0F2F6")
)

// Show draws the sidebar: a column Width wide holding the card with the library switcher, the sources and the status line. Its top row clears the traffic lights of a window with a hidden title bar and drags the window.
func Show(c *ui.Context, t *ui.Theme, a App) {
	col := ui.Column(c).Width(Width).Shrink(0).AlignItems(ui.Stretch).Padding(cardMargin, 0, cardMargin, cardMargin)
	// With the window's material on, the gutter around the card shows it; without it, the gutter is painted.
	if !c.Vibrancy() {
		col.Background(gutterFill)
	}
	col.Children(func() {
		// The hairlines are rings of half a point that take no room, as the prototype's box-shadows: white just inside the edge, dark just outside it.
		card := ui.Column(c).Grow(1).MinHeight(0).AlignItems(ui.Stretch).Radius(cardRadius).Background(cardFill)
		card.InsetShadow(0, 0, 0, 0.5, cardHighlight).Shadow(0, 0, 0, 0.5, cardHairline).Shadow(0, 4, 14, 0, cardShadow)
		card.Children(func() {
			top := float32(10)
			if c.TitleBar().Height > 0 {
				top = LightsRow
			}
			ui.Row(c).Height(top).Shrink(0).DragWindow()
			switcher(c, t, a)
			sources(c, a)
			if a.SaveErr() != "" {
				ui.Text(c, a.SaveErr()).TextColor(t.Danger).Wrap().Padding(8, 16, 0, 16)
			}
			footer(c, t, a)
		})
	})
}

// Menu is the library menu of the settings page's 切换 button: the known libraries, new and open, reveal, and forgetting the open one.
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
