package films

import (
	"fmt"
	"strings"

	"kando/apps/kando/desktop"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/sidebar"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/ui"
)

// gridTracks are the columns of a grid of cards at least min wide that fill the row, as the prototype's CSS grids (auto-fill, minmax) do, so as many fit across as Columns counts.
func gridTracks(min core.Points) core.GridTracks {
	return core.GridTracks(fmt.Sprintf("repeat(auto-fill, minmax(%gpx, 1fr))", min))
}

// grid is the films that are done, as 16:9 posters across the page, in the order the sort button beside its heading chose (done is in that order already). With a single film there is nothing to order, and the button stays away.
type grid struct {
	a      App
	source func() string
	done   func() []Film
}

// Render implements core.Node.
func (g grid) Render(cx *core.Scope) core.Node {
	a := g.a
	title := func() string {
		if g.source() == sidebar.SourceAll {
			return locales.SourceAll(cx)
		}
		return locales.FilmsGridTitle(cx)
	}
	count := func() string { return locales.FilmsGridCount(cx, len(g.done())) }
	head := core.View{Style: core.Style{Direction: core.Row, Gap: 12, AlignItems: core.AlignCenter}, Children: []core.Node{
		core.View{Style: core.Style{FlexGrow: 1, MinWidth: core.Pt(0)}, Children: []core.Node{sectionTitle(cx, title, count)}},
		core.Show{When: func() bool { return len(g.done()) > 1 }, Then: func(*core.Scope) core.Node { return sortButton{a} }},
	}}
	cards := core.View{TestID: "films-grid",
		Style: core.Style{Display: core.DisplayGrid, GridTemplateColumns: gridTracks(CardMinWidth), ColumnGap: CardGapX, RowGap: CardGapY, AlignItems: core.AlignStart},
		Children: []core.Node{core.Each(g.done, func(item Film) string { return item.Key }, func(cx *core.Scope, item func() Film, _ func() int) core.Node {
			return doneCard{a: a, item: item}
		})}}
	return core.View{Style: core.Style{Margin: core.Edges{Top: 26}, Gap: 14}, Children: []core.Node{head, cards}}
}

// sectionTitle is the heading of a part of the shelf with a dimmed note after it on the same baseline.
func sectionTitle(cx *core.Scope, title, note func() string) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{Style: core.Style{Direction: core.Row, Height: core.Pt(26), Gap: 10, AlignItems: core.AlignBaseline}, Children: []core.Node{
		heading(2, core.Text{ContentFunc: title, Style: core.Style{FontSize: 19, FontWeight: ui.Bold, LetterSpacing: -0.2}, StyleFunc: ui.Colored(colors.Text)}),
		core.Show{When: func() bool { return note() != "" }, Then: func(*core.Scope) core.Node {
			return core.Text{ContentFunc: note, Style: core.Style{FontSize: 12}, StyleFunc: ui.Colored(colors.Muted)}
		}},
	}}
}

// The poster's hairline and its shadows at rest and while the pointer is over it, when it lifts toward the pointer.
var (
	posterHairline = core.RGBA(0x000000, 0.1)
	posterShadow   = []core.BoxShadow{{Offset: core.Point{Y: 4}, Blur: 12, Color: core.RGBA(0x000000, 0.12)}}
	posterLifted   = []core.BoxShadow{{Offset: core.Point{Y: 14}, Blur: 30, Color: core.RGBA(0x000000, 0.22)}}
)

// doneCard is a film in the grid: the poster opens it, and ⋯ and a right click offer 播放, 在访达中显示, 重命名…, 重新处理, 重新翻译 and 从资料库移除, and 补译未译的句子 for a film whose translation left lines untranslated, which also has that button under its line.
type doneCard struct {
	a    App
	item func() Film
}

// Render implements core.Node.
func (d doneCard) Render(cx *core.Scope) core.Node {
	a, item := d.a, d.item
	colors := ui.ColorsOf(cx)
	name := func() string { return filmName(item()) }
	duration := live.Func(a, func() string {
		if length := a.Duration(item().VideoPath); length > 0 {
			return Clock(length)
		}
		return ""
	})
	poster := base.Button{
		TestID:    "film-" + item().Key,
		LabelFunc: name,
		OnPress:   live.On(a, func(core.PressEvent) { a.Open(item()) }),
		Style: core.Style{Width: core.Pct(100), AspectRatio: 16.0 / 9, CornerRadius: 13, Overflow: core.Hidden, Background: thumbFill, BorderWidth: 0.5, BorderColor: posterHairline,
			Shadows: posterShadow, Cursor: core.CursorDefault, Hover: &core.Style{Shadows: posterLifted, Transform: core.Transform{Scale: 1.015}}},
		Children: []core.Node{
			cover(live.Func(a, func() core.ImageSource { return a.Poster(item().VideoPath) })),
			core.Show{When: func() bool { return item().Language != "" }, Then: func(*core.Scope) core.Node {
				return posterChip(func() string {
					return locales.FilmsPosterBilingual(cx, LanguageShort(locales.For(cx), item().Language))
				},
					core.Style{Top: core.Pt(9), Left: core.Pt(9), LetterSpacing: 0.4, FontWeight: ui.Bold})
			}},
			core.Show{When: func() bool { return duration() != "" }, Then: func(*core.Scope) core.Node {
				return posterChip(duration, core.Style{Right: core.Pt(9), Bottom: core.Pt(9), FontWeight: ui.Semibold, FontVariant: core.FontTabularNums})
			}},
		},
	}
	meta := core.Text{ContentFunc: live.Func(a, func() string { return doneMeta(locales.For(cx), a, item()) }), MaxLines: 2, EllipsizeMode: core.EllipsizeTail,
		Style: core.Style{FontSize: 12}, StyleFunc: ui.Colored(colors.Muted)}
	missing := core.Show{When: func() bool { return item().JobID != "" && item().Untranslated > 0 }, Then: func(*core.Scope) core.Node {
		return core.View{Style: core.Style{AlignSelf: core.AlignStart, Margin: core.Edges{Top: 5}}, Children: []core.Node{
			textButton(locales.FilmsTranslateMissing, ui.Button{Variant: ui.ButtonSecondary, Size: ui.SizeSmall, TestID: "film-translate-missing",
				OnPress: live.Do(a, func() { a.TranslateMissing(item()) })}),
		}}
	}}
	caption := core.View{Style: core.Style{Direction: core.Row, Gap: 6, AlignItems: core.AlignStart}, Children: []core.Node{
		core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: []core.Node{
			core.Text{ContentFunc: name, MaxLines: 1, EllipsizeMode: core.EllipsizeTail, Style: core.Style{FontSize: 13, FontWeight: ui.Semibold, LineHeight: 17}, StyleFunc: ui.Colored(colors.Text)},
			meta,
			missing,
		}},
		moreButton{a: a, item: item, testID: "film-more"},
	}}
	return core.View{
		Style:         core.Style{Width: core.Pct(100), MinWidth: core.Pt(0), Gap: 9},
		OnContextMenu: func(e core.ContextMenuEvent) { nsdesktop.PopupMenu(cx, filmMenu(cx, a, item()), e.PagePosition, nil) },
		Children:      []core.Node{poster, caption},
	}
}

// filmMenu is what can be done with a done film, from ⋯ or a right click on its poster or the hero, in the system's menu. Its reveal item names the file manager of the system the app runs on, as the buttons beside it do. It is built as the menu opens, so it says what is true then.
func filmMenu(cx *core.Scope, a App, item Film) []nsdesktop.MenuItem {
	do := func(fn func(Film)) func() { return live.Do(a, func() { fn(item) }) }
	items := []nsdesktop.MenuItem{
		{Label: locales.FilmsPlay(cx), OnSelect: do(a.Open)},
		{Label: locales.FilmsShowInFinder(cx, desktop.Platform(cx)), OnSelect: live.Do(a, func() { a.Reveal(item.VideoPath) })},
	}
	if item.VideoPath != "" {
		items = append(items, nsdesktop.MenuItem{Label: locales.FilmsRename(cx), OnSelect: do(a.AskRename)})
	}
	items = append(items, nsdesktop.MenuItem{Label: locales.FilmsReprocess(cx), OnSelect: do(a.Reprocess)})
	if item.JobID != "" {
		items = append(items, nsdesktop.MenuItem{Label: locales.FilmsTranslateAgain(cx), OnSelect: do(a.TranslateAgain)})
		if item.Untranslated > 0 {
			items = append(items, nsdesktop.MenuItem{Label: locales.FilmsTranslateMissing(cx), OnSelect: do(a.TranslateMissing)})
		}
	}
	return append(items, nsdesktop.MenuItem{Separator: true}, nsdesktop.MenuItem{Label: locales.FilmsRemoveFromLibrary(cx), OnSelect: do(a.AskRemove)})
}

// chipShade is the frosted dark glass of the chips over a poster.
var chipShade = core.RGBA(0x000000, 0.42)

// posterChip is a small label over a poster, such as the subtitle language or the length, on frosted dark glass, placed in the poster by at.
func posterChip(text func() string, at core.Style) core.Node {
	look := core.Style{FontSize: 10, Color: onPicture, LetterSpacing: at.LetterSpacing, FontWeight: at.FontWeight, FontVariant: at.FontVariant}
	place := core.Style{Position: core.Absolute, Top: at.Top, Right: at.Right, Bottom: at.Bottom, Left: at.Left, CornerRadius: 6, Padding: core.Symmetric(2, 7)}
	return core.View{PointerEvents: core.PointerEventsNone, Style: place.Merge(frosted(10, chipShade)), Children: []core.Node{
		core.Text{ContentFunc: text, MaxLines: 1, Style: look},
	}}
}

// The ⋯ button's fill while the pointer is over it and while it is pressed.
var (
	moreHover   = core.RGBA(0x000000, 0.06)
	morePressed = core.RGBA(0x000000, 0.08)
	moreInk     = core.RGB(0x3A3A3F)
)

// moreButton is ⋯ beside a poster's title: a 26-point round button with the prototype's three dots, gray while pressed. It opens the film's menu under it, at its leading edge.
type moreButton struct {
	a      App
	item   func() Film
	testID string
}

// Render implements core.Node.
func (m moreButton) Render(cx *core.Scope) core.Node {
	a := m.a
	open := core.NewSignal(cx, false)
	// place is the view around the button, which base.Button takes no Ref for, so its frame gives the button's.
	place := &core.Ref{}
	press := func(core.PressEvent) {
		at := core.Point{}
		if b, ok := place.Bounds(); ok {
			at = core.Point{X: b.Origin.X, Y: b.Origin.Y + b.Size.Height + 4}
		}
		open.Set(true)
		nsdesktop.PopupMenu(cx, filmMenu(cx, a, m.item()), at, func() { open.Set(false) })
	}
	button := base.Button{
		TestID:    m.testID,
		LabelFunc: func() string { return locales.FilmsMoreActions(cx) },
		Expanded:  open.Get,
		OnPress:   press,
		Style: core.Style{Width: core.Pt(26), Height: core.Pt(26), CornerRadius: 13, FlexShrink: 0, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Cursor: core.CursorDefault,
			Hover: &core.Style{Background: moreHover}, Active: &core.Style{Background: morePressed}},
		Children: []core.Node{core.Icon{SVG: moreIcon, Style: core.Style{Width: core.Pt(14), Height: core.Pt(4), Color: moreInk}}},
	}
	return core.View{Ref: place, Style: core.Style{FlexShrink: 0}, Children: []core.Node{button}}
}

// doneMeta is the line under a poster's title, in the language of m: where the film comes from, how long it is, that it has no subtitles yet, and how many lines its translation left untranslated.
func doneMeta(m locales.Messages, a App, item Film) string {
	parts := []string{SourceLabel(m, item)}
	if d := a.Duration(item.VideoPath); d > 0 {
		parts = append(parts, Clock(d))
	}
	if item.SubtitlePath == "" {
		parts = append(parts, m.FilmsNoSubtitles())
	}
	if note := untranslatedNote(m, item); note != "" {
		parts = append(parts, note)
	}
	return strings.Join(parts, " · ")
}

// untranslatedNote says how many lines of a done film the translation left untranslated, as the prototype words a translation that failed for some lines; "" when it left none.
func untranslatedNote(m locales.Messages, item Film) string {
	if item.Untranslated <= 0 {
		return ""
	}
	return m.FilmsUntranslated(item.Untranslated)
}
