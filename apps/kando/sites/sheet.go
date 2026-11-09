// Package sites draws 支持的网站: the websites Kando downloads from through yt-dlp, with their logos, in a sheet over the window that the search narrows down.
package sites

import (
	"hash/fnv"
	"strings"
	"unicode"
	"unicode/utf8"

	"kando/apps/kando/locales"
	"kando/apps/kando/sites/catalog"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// App is the sheet's state, which the app keeps: the search, the grid's place, and the decoded logos.
type App interface {
	SitesQuery() *string
	SitesGrid() *ui.GridState
	// SiteLogo is s's logo, or nil when it has none and the sheet draws its initial.
	SiteLogo(s catalog.Site) ui.ImageSource
}

// The sheet's measures: 720 wide and at most 640 high, the grid's tiles at least 200 wide and 52 high (the grid spaces them by the theme's gap), the logos 28 square with corners of 7.
const (
	width     = 720
	maxHeight = 640
	tileWidth = 200
	tileH     = 52
	logoSize  = 28
	logoR     = 7
)

// Show draws the sheet while *open. Escape and 完成 close it.
func Show(c *ui.Context, t *ui.Theme, a App, open *bool) {
	cat := catalog.Load()
	query := a.SitesQuery()
	found := cat.Filter(*query)
	_, h := c.Size()
	panel := breeze.Sheet(c, open, func() {
		ui.Column(c).Grow(1).Shrink(1).MinHeight(0).Gap(12).AlignItems(ui.Stretch).Children(func() {
			header(c, cat)
			c.Key("sites-search")
			breeze.SearchField(c, query).Placeholder(locales.SitesSearchPlaceholder()).Label(locales.SitesSearchLabel()).AutoFocus()
			if len(found) == 0 {
				notFound(c, t, *query)
			} else {
				grid(c, t, a, found)
			}
			footer(c, cat, len(found), *query, open)
		})
	})
	panel.Width(width).Height(min(maxHeight, h-40))
}

// header is the title and what the list is.
func header(c *ui.Context, cat catalog.Catalog) {
	ui.Column(c).Gap(2).Children(func() {
		breeze.Text(c, locales.SitesTitle(), breeze.TextHeading).FontSize(15).Bold().Role(ui.RoleHeading).Level(1)
		for _, line := range []string{
			locales.SitesIntro(len(cat.Sites)),
			locales.SitesCaveat(),
		} {
			breeze.Text(c, line, breeze.TextCaption, breeze.TextSecondary).FontSize(12).FixedLineHeight(17).Wrap()
		}
	})
}

// grid lists found as tiles, as many to a row as fit, built only as they scroll into view.
func grid(c *ui.Context, t *ui.Theme, a App, found []catalog.Site) {
	state := a.SitesGrid()
	state.Key = func(i int) any { return found[i].Domain }
	c.Key("sites-grid")
	ui.GridView(c, state, len(found), tileWidth, tileH, func(i int) {
		tile(c, t, a, found[i])
	}).Grow(1).Shrink(1).MinHeight(0).Label(locales.SitesGridLabel())
}

// tile is a site: its logo, its name, and its domain under it.
func tile(c *ui.Context, t *ui.Theme, a App, s catalog.Site) {
	ui.Row(c).Grow(1).Gap(10).AlignItems(ui.Center).Padding(0, 10).Radius(10).Background(t.Surface).Children(func() {
		logo(c, a, s)
		ui.Column(c).Grow(1).MinWidth(0).Gap(1).Children(func() {
			ui.Text(c, s.Name).FontSize(13).FontWeight(600).SingleLine()
			ui.Text(c, s.Domain).FontSize(11).TextColor(t.TextMuted).SingleLine()
		})
	}).Label(locales.SitesTileLabel(s.Name, s.Domain))
}

// logo is s's logo on a white plate, as favicons are drawn for a light page, or its initial on a plate of a color of its own when it has none.
func logo(c *ui.Context, a App, s catalog.Site) {
	if img := a.SiteLogo(s); img != nil {
		ui.Box(c).Size(logoSize, logoSize).Shrink(0).Radius(logoR).Background(ui.RGB(255, 255, 255)).InsetShadow(0, 0, 0, 0.5, ui.RGBA(0, 0, 0, 0.12)).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
			ui.Image(c, img).Size(logoSize-6, logoSize-6).Fit(ui.Contain)
		})
		return
	}
	ui.Box(c).Size(logoSize, logoSize).Shrink(0).Radius(logoR).Background(plate(s.Domain)).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
		ui.Text(c, Initial(s.Name)).FontSize(14).Bold().TextColor(ui.RGB(255, 255, 255))
	})
}

// Initial is the letter a site without a logo shows: the first letter or digit of its name, upper case.
func Initial(name string) string {
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			return string(unicode.ToUpper(r))
		}
	}
	if r, _ := utf8.DecodeRuneInString(name); r != utf8.RuneError {
		return string(r)
	}
	return "?"
}

// plates are the colors of the plates under initials, mid tones that white letters read on.
var plates = []ui.Color{
	ui.RGB(0x4C, 0x6E, 0xF5), ui.RGB(0x12, 0xB8, 0x86), ui.RGB(0xF0, 0x8C, 0x00), ui.RGB(0xE6, 0x49, 0x80),
	ui.RGB(0x79, 0x50, 0xF2), ui.RGB(0x15, 0xAA, 0xBF), ui.RGB(0xFA, 0x52, 0x52), ui.RGB(0x5C, 0x7C, 0x2E),
}

// plate is domain's plate color, the same each time.
func plate(domain string) ui.Color {
	h := fnv.New32a()
	h.Write([]byte(domain))
	return plates[h.Sum32()%uint32(len(plates))]
}

// notFound says the search found no site, and that a site off the list may still work.
func notFound(c *ui.Context, t *ui.Theme, query string) {
	ui.Column(c).Grow(1).AlignItems(ui.Center).Justify(ui.Center).Gap(6).Padding(24).Children(func() {
		breeze.Text(c, locales.SitesNotFound(strings.TrimSpace(query)), breeze.TextSubtitle).FontSize(14).Bold().TextAlign(ui.Center).Wrap()
		ui.Text(c, locales.SitesNotFoundHint()).FontSize(12).FixedLineHeight(18).TextColor(t.TextMuted).TextAlign(ui.Center).MaxWidth(420).Wrap()
	})
}

// footer is where the list comes from, how many sites the search found, and 完成.
func footer(c *ui.Context, cat catalog.Catalog, found int, query string, open *bool) {
	ui.Row(c).Gap(8).AlignItems(ui.Center).Children(func() {
		note := locales.SitesSource(cat.YTDLP)
		if strings.TrimSpace(query) != "" {
			note = locales.SitesFound(found, note)
		}
		breeze.Text(c, note, breeze.TextCaption, breeze.TextSecondary).FontSize(11).Grow(1).Shrink(1).MinWidth(0).SingleLine()
		c.Key("sites-done")
		if breeze.PrimaryButton(c, locales.SitesDone()).Clicked() {
			*open = false
		}
	})
}
