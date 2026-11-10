package films

import (
	"kando/apps/kando/locales"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// noLibrary is the page before there is a library: what a library is, 新建资料库… and 打开已有资料库….
func noLibrary(c *ui.Context, a App) {
	statusPage(c, libraryArt, locales.FilmsNoLibraryTitle(), locales.FilmsNoLibraryDescription(breeze.PlatformOf(c).String()), func() {
		c.Key("empty-new")
		if statusButton(breeze.PrimaryButton(c, locales.FilmsNewLibraryEllipsis())).Clicked() {
			a.BeginNew()
		}
		c.Key("empty-open")
		if statusButton(breeze.Button(c, locales.FilmsOpenLibraryEllipsis())).Clicked() {
			a.PickLibrary()
		}
	})
}

// emptyLibrary is the page of a library that holds no film yet: how to add one, 选择本地视频…, 打开已有资料库… and 支持的网站, which lists the sites a link can come from.
func emptyLibrary(c *ui.Context, a App) {
	statusPage(c, startArt, locales.FilmsEmptyTitle(), locales.FilmsEmptyDescription(), func() {
		c.Key("empty-pick")
		if statusButton(breeze.PrimaryButton(c, locales.FilmsPickVideosEllipsis())).Clicked() {
			a.PickVideos()
		}
		c.Key("empty-open")
		if statusButton(breeze.Button(c, locales.FilmsOpenLibraryEllipsis())).Clicked() {
			a.PickLibrary()
		}
		c.Key("empty-sites")
		if statusButton(breeze.Button(c, locales.FilmsSupportedSites())).Clicked() {
			a.OpenSites()
		}
	})
}

// emptySource is what a source of the sidebar shows when no film of the library comes from it.
func emptySource(c *ui.Context, t *ui.Theme) {
	ui.Column(c).Grow(1).AlignItems(ui.Center).Padding(80, 28).Children(func() {
		breeze.Text(c, locales.FilmsSourceEmpty(), breeze.TextSecondary)
	})
}

// statusPage is an empty page as the prototype draws it: the colored illustration, the title at 20 points, the description centered in gray, and the actions in a row, in a column 420 wide, 90 points under the message bar.
func statusPage(c *ui.Context, art *ui.SVG, title, description string, actions func()) {
	t := c.Theme()
	ui.Column(c).AlignItems(ui.Center).Padding(90, 28).Children(func() {
		ui.Column(c).MaxWidth(420).AlignItems(ui.Center).Gap(10).Children(func() {
			w, h := art.Size()
			ui.Image(c, art).Size(w, h).Shrink(0)
			ui.Text(c, title).FontSize(20).Bold().Margin(6, 0, 0, 0).Role(ui.RoleHeading).Level(2)
			ui.Text(c, description).FontSize(13).FixedLineHeight(20).TextColor(t.TextMuted).TextAlign(ui.Center).Wrap()
			ui.Row(c).Gap(10).Wrap().Justify(ui.Center).Margin(8, 0, 0, 0).Children(actions)
		})
	})
}

// statusButton sizes an empty page's button as the prototype's, 30 points high and padded 16 at its sides; the platform draws its face.
func statusButton(b ui.Element) ui.Element { return b.Height(30).PaddingX(16) }
