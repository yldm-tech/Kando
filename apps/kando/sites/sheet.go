// Package sites draws 支持的网站: the websites Kando downloads from through yt-dlp, with their logos, in a sheet over the window that the search narrows down.
package sites

import (
	"hash/fnv"
	"strings"
	"unicode"
	"unicode/utf8"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/sites/catalog"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// App is the sheet's state, which the app keeps: the search, and the logos read so far. The sheet reads it inside its bindings through live.Func and changes the search through live.On, so what it changes shows at once (package live).
type App interface {
	live.State
	Logos
	SitesQuery() *string
	// OpenSite opens s in the browser (浏览), as a tile does when pressed, closing the sheet.
	OpenSite(s catalog.Site)
}

// Logos is where a tile's logo comes from, the app, which reads each logo once: SiteLogo is the bytes of s's logo, the same slice each time, or nil when it has none and the tile draws its initial.
type Logos interface {
	SiteLogo(s catalog.Site) []byte
}

// The sheet's measures: 720 wide and at most 640 high, the grid three tiles to a row, each 52 high, the tiles gap apart, the logos 28 square with corners of 7.
const (
	width     core.Points = 720
	maxHeight core.Points = 640
	columns               = 3
	tileH     core.Points = 52
	gap       core.Points = 8
	logoSize  core.Points = 28
	logoR     core.Points = 7
)

var (
	// logoPlate is the white a logo sits on, as favicons are drawn for a light page, with a hairline inside its edge.
	logoPlate    = core.RGB(0xFFFFFF)
	logoHairline = core.RGBA(0x000000, 0.12)
	initialText  = core.RGB(0xFFFFFF)
	// searchFill is the search field's gray, which has no border.
	searchFill = core.RGBA(0x000000, 0.06)
)

// Sheet is the sheet, built while it is open: the title and what the list is, the search, the grid of the sites it finds, and where the list comes from with 完成. 完成, Escape once the search is empty, and closing the sheet run OnClose, which closes it.
type Sheet struct {
	App     App
	OnClose func()
}

// Render implements core.Node.
func (s Sheet) Render(cx *core.Scope) core.Node {
	a := s.App
	cat := catalog.Load()
	query := live.Memo(cx, a, func() string { return *a.SitesQuery() })
	// The sites the search finds, which the grid and the footer read; they change only with the search.
	found := core.NewMemoFunc(cx, func() []catalog.Site { return cat.Filter(query()) }, sameSites)
	body := core.View{
		Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0), Gap: 12, AlignItems: core.AlignStretch, Padding: core.All(20)},
		Children: []core.Node{
			header(cx, cat),
			search{a: a, query: query},
			core.Show{
				When: func() bool { return len(found.Get()) == 0 },
				Then: func(cx *core.Scope) core.Node { return notFound(cx, query) },
				Else: func(cx *core.Scope) core.Node { return grid(cx, a, found) },
			},
			footer(cx, cat, found, query, s.OnClose),
		},
	}
	// TODO(nsgo#145): give base.Dialog (ui.Dialog) LabelFunc and AccessibilityDescriptionFunc twins, so the sheet's accessible name follows the interface language as it changes while it is open; it is read as the sheet opens https://github.com/nsgo-dev/nsgo/issues/145
	return ui.Dialog{
		TestID:                 "sites",
		Label:                  locales.SitesTitle(cx),
		Open:                   func() bool { return true },
		NoCloseOnBackdropPress: true,
		Bare:                   true,
		Style:                  core.Style{Width: core.Pt(width), Height: core.Pt(maxHeight)},
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open && s.OnClose != nil {
				s.OnClose()
			}
		},
		Children: []core.Node{body},
	}
}

// sameSites reports whether two searches found the same sites, in the same order.
func sameSites(x, y []catalog.Site) bool {
	if len(x) != len(y) {
		return false
	}
	for i := range x {
		if x[i].Domain != y[i].Domain {
			return false
		}
	}
	return true
}

// header is the title and what the list is.
func header(cx *core.Scope, cat catalog.Catalog) core.Node {
	colors := ui.ColorsOf(cx)
	title := func() string { return locales.SitesTitle(cx) }
	line := func(text func() string) core.Node {
		return core.Text{ContentFunc: text, Style: core.Style{FontSize: 12, LineHeight: 17}, StyleFunc: ui.Colored(colors.Muted)}
	}
	return core.View{Style: core.Style{Gap: 2, FlexShrink: 0}, Children: []core.Node{
		core.View{Role: core.RoleHeader, AccessibilityLevel: func() int { return 1 }, LabelFunc: title, Children: []core.Node{
			core.Text{ContentFunc: title, Style: core.Style{FontSize: 15, LineHeight: 20, FontWeight: ui.Bold}, StyleFunc: ui.Colored(colors.Text)},
		}},
		line(func() string { return locales.SitesIntro(cx, len(cat.Sites)) }),
		line(func() string { return locales.SitesCaveat(cx) }),
	}}
}

// search is the field the sheet opens with the keyboard in: gray without a border, a magnifying glass before the text and a clear button after it. Escape empties it while it holds something, and closes the sheet once it is empty.
type search struct {
	a     App
	query func() string
}

// Render implements core.Node.
func (s search) Render(cx *core.Scope) core.Node {
	a := s.a
	colors := ui.ColorsOf(cx)
	clear := live.Do(a, func() { *a.SitesQuery() = "" })
	focused := core.NewSignal(cx, false)
	escape := core.Show{When: func() bool { return focused.Get() && s.query() != "" }, Then: func(cx *core.Scope) core.Node {
		cx.OnKeyWith("escape", core.KeyOptions{When: core.KeyAlways}, clear)
		return nil
	}}
	// TODO(nsgo#145): give core.Input (base.Input, ui.Input) LabelFunc and AccessibilityDescriptionFunc twins, so the field's accessible name follows the interface language as it changes; it is read once here, and a Dynamic would take the focus and the typed text with it https://github.com/nsgo-dev/nsgo/issues/145
	// TODO(nsgo#144): give core.Input a search role, so assistive technology reads the field as a search field as MyGo's SearchField was https://github.com/nsgo-dev/nsgo/issues/144
	input := base.Input{
		TestID:          "sites-search",
		Label:           locales.SitesSearchLabel(cx),
		AutoFocus:       true,
		Value:           s.query,
		PlaceholderFunc: func() string { return locales.SitesSearchPlaceholder(cx) },
		OnChange:        live.On(a, func(text string) { *a.SitesQuery() = text }),
		OnFocus:         func() { focused.Set(true) },
		OnBlur:          func() { focused.Set(false) },
		Style: core.Style{FlexGrow: 1, MinWidth: core.Pt(0), Height: core.Pt(30), CornerRadius: 8, Background: searchFill, FontSize: 13,
			Padding: core.Edges{Left: 30, Right: 30}},
		StyleFunc: func() core.Style {
			return core.Style{Color: colors.Text(), PlaceholderColor: colors.Faint(), CaretColor: colors.Accent(), SelectionColor: cx.Palette().Selection}
		},
	}
	clearButton := core.Show{When: func() bool { return s.query() != "" }, Then: func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{Position: core.Absolute, Top: core.Pt(5), Right: core.Pt(5)}, Children: []core.Node{
			ui.IconButton{Icon: lucide.X, LabelFunc: func() string { return locales.SitesSearchClear(cx) }, Size: 20, OnPress: clear, TestID: "sites-search-clear"},
		}}
	}}
	glass := core.View{PointerEvents: core.PointerEventsNone, Style: core.Style{Position: core.Absolute, Top: core.Pt(8), Left: core.Pt(10)},
		Children: []core.Node{ui.Icon(lucide.Search, 14, colors.Muted)}}
	return core.View{Style: core.Style{Direction: core.Row, FlexShrink: 0}, Children: []core.Node{
		core.View{Style: core.Style{FlexGrow: 1, MinWidth: core.Pt(0)}, Children: []core.Node{input, glass, clearButton, escape}},
	}}
}

// grid lists the sites found as tiles, three to a row, built only as they scroll into view; a tile's cell shows another site as the list scrolls (core.List).
func grid(cx *core.Scope, a App, found *core.Memo[[]catalog.Site]) core.Node {
	list := core.List{
		TestID:        "sites-grid",
		Count:         func() int { return len(found.Get()) },
		Key:           func(i int) any { return found.Get()[i].Domain },
		NumColumns:    columns,
		EstimatedSize: tileH,
		Gap:           gap,
		ColumnGap:     gap,
		Style:         core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)},
		Row: func(cx *core.Scope, row core.ListRow) core.Node {
			site := func() catalog.Site {
				if sites, i := found.Get(), row.Index(); i < len(sites) {
					return sites[i]
				}
				return catalog.Site{}
			}
			return Tile(cx, a, site, live.Do(a, func() { a.OpenSite(site()) }))
		},
	}
	return core.View{
		Role:      core.RoleGroup,
		LabelFunc: func() string { return locales.SitesGridLabel(cx) },
		Style:     core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0), AlignItems: core.AlignStretch},
		Children:  []core.Node{list},
	}
}

// Tile is a site, which open opens: its logo, its name, and its domain under it. The sheet lists the sites as tiles, and the browser's start the ones learners open most; open runs on the press as it is given, so a handler that changes the app's state comes wrapped in live.Do.
func Tile(cx *core.Scope, logos Logos, site func() catalog.Site, open func()) core.Node {
	colors := ui.ColorsOf(cx)
	name := func() string { return site().Name }
	domain := func() string { return site().Domain }
	return base.Button{
		TestID:    "site-tile",
		LabelFunc: func() string { return locales.SitesTileLabel(cx, name(), domain()) },
		OnPress:   func(core.PressEvent) { open() },
		Style: core.Style{Direction: core.Row, Height: core.Pt(tileH), Gap: 10, AlignItems: core.AlignCenter, Padding: core.Symmetric(0, 10),
			CornerRadius: 10},
		StyleFunc: func() core.Style {
			return core.Style{Background: colors.Surface(), Hover: &core.Style{Background: colors.Hover()}, Active: &core.Style{Background: colors.Pressed()}, FocusVisible: ui.FocusRing(cx, 2)}
		},
		Children: []core.Node{
			logo(cx, logos, site),
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 1}, Children: []core.Node{
				core.Text{ContentFunc: name, MaxLines: 1, EllipsizeMode: core.EllipsizeTail, Style: core.Style{FontSize: 13, FontWeight: ui.Semibold},
					StyleFunc: ui.Colored(colors.Text)},
				core.Text{ContentFunc: domain, MaxLines: 1, EllipsizeMode: core.EllipsizeTail, Style: core.Style{FontSize: 11}, StyleFunc: ui.Colored(colors.Muted)},
			}},
		},
	}
}

// logo is the site's logo on a white plate, or its initial on a plate of a color of its own when it has none.
func logo(cx *core.Scope, logos Logos, site func() catalog.Site) core.Node {
	plateStyle := core.Style{Width: core.Pt(logoSize), Height: core.Pt(logoSize), FlexShrink: 0, CornerRadius: logoR, JustifyContent: core.JustifyCenter, AlignItems: core.AlignCenter}
	data := func() []byte { return logos.SiteLogo(site()) }
	return core.Show{
		When: func() bool { return data() != nil },
		Then: func(*core.Scope) core.Node {
			s := plateStyle
			s.Background = logoPlate
			s.Shadows = []core.BoxShadow{{Spread: 0.5, Color: logoHairline, Inset: true}}
			return core.View{Style: s, Children: []core.Node{core.Image{
				SourceFunc: func() core.ImageSource { return core.ImageSource{Data: data()} },
				ResizeMode: core.Contain,
				Style:      core.Style{Width: core.Pt(logoSize - 6), Height: core.Pt(logoSize - 6)},
			}}}
		},
		Else: func(*core.Scope) core.Node {
			return core.View{Style: plateStyle, StyleFunc: func() core.Style { return core.Style{Background: plate(site().Domain)} }, Children: []core.Node{
				core.Text{ContentFunc: func() string { return Initial(site().Name) }, Style: core.Style{FontSize: 14, FontWeight: ui.Bold, Color: initialText}},
			}}
		},
	}
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
var plates = []core.Color{
	core.RGB(0x4C6EF5), core.RGB(0x12B886), core.RGB(0xF08C00), core.RGB(0xE64980),
	core.RGB(0x7950F2), core.RGB(0x15AABF), core.RGB(0xFA5252), core.RGB(0x5C7C2E),
}

// plate is domain's plate color, the same each time.
func plate(domain string) core.Color {
	h := fnv.New32a()
	h.Write([]byte(domain))
	return plates[h.Sum32()%uint32(len(plates))]
}

// notFound says the search found no site, and that a site off the list may still work.
func notFound(cx *core.Scope, query func() string) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{
		Style: core.Style{FlexGrow: 1, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Gap: 6, Padding: core.All(24)},
		Children: []core.Node{
			core.Text{ContentFunc: func() string { return locales.SitesNotFound(cx, strings.TrimSpace(query())) },
				Style: core.Style{FontSize: 14, FontWeight: ui.Bold, TextAlign: core.TextAlignCenter}, StyleFunc: ui.Colored(colors.Text)},
			core.Text{ContentFunc: func() string { return locales.SitesNotFoundHint(cx) },
				Style: core.Style{FontSize: 12, LineHeight: 18, TextAlign: core.TextAlignCenter, MaxWidth: core.Pt(420)}, StyleFunc: ui.Colored(colors.Muted)},
		},
	}
}

// footer is where the list comes from, how many sites the search found, and 完成.
func footer(cx *core.Scope, cat catalog.Catalog, found *core.Memo[[]catalog.Site], query func() string, done func()) core.Node {
	colors := ui.ColorsOf(cx)
	note := func() string {
		source := locales.SitesSource(cx, cat.YTDLP)
		if strings.TrimSpace(query()) != "" {
			return locales.SitesFound(cx, len(found.Get()), source)
		}
		return source
	}
	return core.View{Style: core.Style{Direction: core.Row, FlexShrink: 0, Gap: 8, AlignItems: core.AlignCenter}, Children: []core.Node{
		core.Text{ContentFunc: note, MaxLines: 1, EllipsizeMode: core.EllipsizeTail, Style: core.Style{FontSize: 11, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)},
			StyleFunc: ui.Colored(colors.Muted)},
		// The button's words are set as it is built, so it is built again for another language.
		core.Dynamic(func(cx *core.Scope) core.Node {
			return ui.Button{Text: locales.SitesDone(cx), TestID: "sites-done", OnPress: done}
		}),
	}}
}
