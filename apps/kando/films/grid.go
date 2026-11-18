package films

import (
	"fmt"
	"strings"

	"kando/apps/kando/desktop"
	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/sidebar"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// gridTracks are the columns of a grid of cards at least min wide that fill the row, as the prototype's CSS grids (auto-fill, minmax) do, so as many fit across as Columns counts.
func gridTracks(min core.Points) core.GridTracks {
	return core.GridTracks(fmt.Sprintf("repeat(auto-fill, minmax(%gpx, 1fr))", min))
}

// grid is the films that are done, as 16:9 posters across the page under their heading (doneHeading), in the order chosen (done is in that order already).
type grid struct {
	a    App
	done func() []Film
	// placed hears where the posters are under the grid's heading and how wide they are, as they are laid out.
	placed func(core.Bounds)
}

// gridTop is the room over the grid's heading.
const gridTop core.Points = 26

// Render implements core.Node.
func (g grid) Render(cx *core.Scope) core.Node {
	a := g.a
	cards := core.View{TestID: "films-grid", OnLayout: g.placed,
		Style: core.Style{Display: core.DisplayGrid, GridTemplateColumns: gridTracks(CardMinWidth), ColumnGap: CardGapX, RowGap: CardGapY, AlignItems: core.AlignStart},
		Children: []core.Node{core.Each(g.done, func(item Film) string { return item.Key }, func(cx *core.Scope, item func() Film, index func() int) core.Node {
			return doneCard{a: a, item: item, done: g.done, index: index}
		})}}
	// A request for the first poster's focus that finds no poster is dropped, so a poster does not take the focus when it shows later.
	cx.Effect(func() {
		a.Track()
		if focus := a.PosterFocus(); *focus && len(g.done()) == 0 {
			*focus = false
		}
	})
	return core.View{Style: core.Style{Margin: core.Edges{Top: gridTop}, Gap: 14}, Children: []core.Node{doneHeading{a: a, done: g.done}, cards}}
}

// doneHeading is the line over the done films, as posters or as a list: how many they are, then Translate N Films while more than one has no translation, the sort button while there is more than one to order, and the switch between posters and the list.
type doneHeading struct {
	a    App
	done func() []Film
}

// Render implements core.Node.
func (h doneHeading) Render(cx *core.Scope) core.Node {
	a := h.a
	colors := ui.ColorsOf(cx)
	// The top bar names the source the page shows, so the heading is only how many films it holds.
	count := func() string { return locales.FilmsGridCount(cx, len(h.done())) }
	// Past one film shown without a translation, as those made before an AI service was set up, the heading offers to translate them all at once while a service is set up to do it, as 处理中 offers Retry All; a single one has its own 翻译.
	untranslated := live.Memo(cx, a, func() int {
		if !a.CanTranslate() {
			return 0
		}
		return len(NotTranslated(h.done()))
	})
	translateAll := core.Show{When: func() bool { return untranslated() > 1 }, Then: func(*core.Scope) core.Node {
		return textButton(func(cx *core.Scope) string { return locales.FilmsTranslateAll(cx, untranslated()) },
			ui.Button{Variant: ui.ButtonSecondary, Size: ui.SizeSmall, TestID: "films-translate-all", OnPress: live.Do(a, func() { a.TranslateFilms(h.done()) })})
	}}
	return core.View{Style: core.Style{Direction: core.Row, Gap: 12, AlignItems: core.AlignCenter}, Children: []core.Node{
		core.View{Style: core.Style{FlexGrow: 1, MinWidth: core.Pt(0)}, Children: []core.Node{
			heading(2, core.Text{ContentFunc: count, Style: core.Style{FontSize: 13, FontWeight: ui.Semibold}, StyleFunc: ui.Colored(colors.Muted)}),
		}},
		translateAll,
		// With a single film there is nothing to order, and the sort button stays away.
		core.Show{When: func() bool { return len(h.done()) > 1 }, Then: func(*core.Scope) core.Node { return sortButton{a} }},
		viewSwitch{a},
	}}
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

// The poster's shadows at rest and while the pointer is over it, when it lifts toward the pointer. Its face before the picture is there is the theme's thumb, and its hairline the shade (theme.Of).
var (
	posterShadow = []core.BoxShadow{{Offset: core.Point{Y: 4}, Blur: 12, Color: core.RGBA(0x000000, 0.12)}}
	posterLifted = []core.BoxShadow{{Offset: core.Point{Y: 14}, Blur: 30, Color: core.RGBA(0x000000, 0.22)}}
)

// doneCard is a film in the grid: the poster opens it where it stopped last time, a ⌘-click selects it or leaves it out and a ⇧-click selects the films from the one selected last to it (pressPoster), the selected ones ringed in the accent color; and ⋯ and a right click offer the selection's menu on one of several selected (menuOf), else 播放, 从头播放 for a film stopped partway, 标记为已看完 or 标记为未看 (watchMenu), 在访达中显示, 重命名…, 重新处理, 重新翻译 and 从资料库移除, and 补译未译的句子 for a film whose translation left lines untranslated, which also has that button under its line. A video found in the folder without subtitles has 生成字幕 under its line and in place of 重新处理 (needsSubtitles).
type doneCard struct {
	a    App
	item func() Film
	// done is the films the grid shows, in its order, which a ⇧-click selects among; index is where the card is among them.
	done  func() []Film
	index func() int
}

// Render implements core.Node.
func (d doneCard) Render(cx *core.Scope) core.Node {
	a, item := d.a, d.item
	colors, k := ui.ColorsOf(cx), theme.Of(cx)
	name := func() string { return filmName(item()) }
	duration := live.Func(a, func() string {
		if length := a.Duration(item().VideoPath); length > 0 {
			return Clock(length)
		}
		return ""
	})
	seen := live.Memo(cx, a, func() watched { return watchedOf(a, item()) })
	selected := live.Func(a, func() bool { return a.Selection().Has(item().Key) })
	menu := func() []nsdesktop.MenuItem { return menuOf(cx, a, item(), d.done(), false) }
	ref := &core.Ref{}
	// The first poster takes the keyboard focus when it is asked for (PosterFocus).
	cx.Effect(func() {
		a.Track()
		if focus := a.PosterFocus(); *focus && d.index() == 0 {
			*focus = false
			ref.Focus()
		}
	})
	poster := base.Button{
		Ref:       ref,
		TestID:    "film-" + item().Key,
		LabelFunc: name,
		Selected:  selected,
		OnPress:   live.On(a, func(e core.PressEvent) { pressPoster(cx, a, item(), d.done(), e) }),
		Style: core.Style{Width: core.Pct(100), AspectRatio: 16.0 / 9, CornerRadius: 13, BorderWidth: 0.5, OutlineOffset: 3,
			Shadows: posterShadow, Cursor: core.CursorDefault, Hover: &core.Style{Shadows: posterLifted, Transform: core.Transform{Scale: 1.015}}, FocusVisible: ui.FocusRing(cx, 2)},
		StyleFunc: func() core.Style {
			style := core.Style{Background: k.Thumb(), BorderColor: k.Shade(0.1)}
			if selected() {
				style.OutlineWidth, style.OutlineColor = 3, colors.Accent()
			}
			return style
		},
		Children: []core.Node{clip(13,
			cover(live.Func(a, func() core.ImageSource { return a.Poster(item().VideoPath) })),
			core.Show{When: func() bool { return item().Language != "" && item().TranslationErr == "" }, Then: func(*core.Scope) core.Node {
				return posterChip(live.Func(a, func() string { return PosterLanguages(locales.For(cx), item(), *a.Language()) }),
					core.Style{Top: core.Pt(9), Left: core.Pt(9), LetterSpacing: 0.4, FontWeight: ui.Bold})
			}},
			core.Show{When: func() bool { return duration() != "" }, Then: func(*core.Scope) core.Node {
				return posterChip(duration, core.Style{Right: core.Pt(9), Bottom: core.Pt(9), FontWeight: ui.Semibold, FontVariant: core.FontTabularNums})
			}},
			core.Show{When: func() bool { return seen().end }, Then: func(*core.Scope) core.Node { return watchedMark(cx) }},
			core.Show{When: func() bool { s := seen(); return s.part > 0 && !s.end }, Then: func(*core.Scope) core.Node {
				return watchedBar(cx, func() float64 { return seen().part })
			}},
		)},
	}
	// The title under the poster may be cut short; the tooltip names the film in full.
	tip := ui.Tooltip{TextFunc: name, Trigger: poster, Style: core.Style{Width: core.Pct(100)}}
	meta := core.Text{ContentFunc: live.Func(a, func() string { return doneMeta(locales.For(cx), item()) }), MaxLines: 1, EllipsizeMode: core.EllipsizeTail,
		Style: core.Style{FontSize: 12}, StyleFunc: ui.Colored(colors.Muted)}
	missing := core.Show{When: func() bool { return item().JobID != "" && item().Untranslated > 0 }, Then: func(*core.Scope) core.Node {
		return untranslatedActions{a: a, item: item}
	}}
	makeSubs := core.Show{When: func() bool { return needsSubtitles(item()) }, Then: func(*core.Scope) core.Node {
		return core.View{Style: core.Style{Margin: core.Edges{Top: 3}, AlignSelf: core.AlignStart}, Children: []core.Node{
			textButton(locales.FilmsMakeSubtitles, ui.Button{Size: ui.SizeSmall, TestID: "film-make-subtitles", OnPress: live.Do(a, func() { a.Reprocess(item()) })}),
		}}
	}}
	caption := core.View{Style: core.Style{Direction: core.Row, Gap: 6, AlignItems: core.AlignStart}, Children: []core.Node{
		core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: []core.Node{
			core.Text{ContentFunc: name, MaxLines: 2, EllipsizeMode: core.EllipsizeTail, Style: core.Style{FontSize: 13, FontWeight: ui.Semibold, LineHeight: 17}, StyleFunc: ui.Colored(colors.Text)},
			meta,
			missing,
			makeSubs,
		}},
		moreButton{a: a, item: item, testID: "film-more", menu: menu},
	}}
	return core.View{
		Style:         core.Style{Width: core.Pct(100), MinWidth: core.Pt(0), Gap: 9},
		OnContextMenu: func(e core.ContextMenuEvent) { nsdesktop.PopupMenu(cx, menu(), e.PagePosition, nil) },
		Children:      []core.Node{tip, caption},
	}
}

// watched is how much of a film has been seen: part is the furthest point played over its length, from 0 to 1, and end is set once it played to within 3 seconds of its end, as ResumeFrom counts the end. A film played on for less than 3 seconds, which WatchState does not count as started, has seen none of it; its length must be known for either.
type watched struct {
	part float64
	end  bool
}

func watchedOf(p Progress, item Film) watched {
	length, furthest := p.Duration(item.VideoPath), p.Furthest(item.VideoPath)
	if length <= 0 || furthest <= 0 {
		return watched{}
	}
	part := min(float64(furthest)/float64(length), 1)
	if furthest >= length-watchSlack {
		return watched{part: part, end: true}
	}
	if furthest < watchSlack {
		return watched{}
	}
	return watched{part: part}
}

// watchedBar is the accent line along a poster's bottom edge as far as the film has been played, over a faint track the width of the poster.
func watchedBar(cx *core.Scope, part func() float64) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{TestID: "film-progress", PointerEvents: core.PointerEventsNone,
		Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0), Height: core.Pt(3), Background: onPicture.Faded(0.3)},
		Children: []core.Node{core.View{Style: core.Style{Height: core.Pct(100)}, StyleFunc: func() core.Style {
			return core.Style{Width: core.Pct(float32(part() * 100)), Background: colors.Accent()}
		}}}}
}

// watchedMark is the check at the top end of the poster of a film played to its end.
func watchedMark(cx *core.Scope) core.Node {
	place := core.Style{Position: core.Absolute, Top: core.Pt(9), Right: core.Pt(9), Width: core.Pt(20), Height: core.Pt(20), CornerRadius: 10, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter}
	return core.View{TestID: "film-watched", Role: core.RoleImage, LabelFunc: func() string { return locales.FilmsWatched(cx) }, PointerEvents: core.PointerEventsNone,
		Style:    place.Merge(frosted(10, chipShade)),
		Children: []core.Node{core.Icon{SVG: lucide.Check(), Style: core.Style{Width: core.Pt(12), Height: core.Pt(12), Color: onPicture}}}}
}

// untranslatedActions is what a done film whose translation left lines untranslated offers under its line: 补译未译的句子, or, for one that translated none (Film.TranslationErr), why in a line of its own, 翻译 and 打开设置 where a setting fixes it.
type untranslatedActions struct {
	a    App
	item func() Film
}

// Render implements core.Node.
func (u untranslatedActions) Render(cx *core.Scope) core.Node {
	a, item := u.a, u.item
	colors := ui.ColorsOf(cx)
	none := func() bool { return item().TranslationErr != "" }
	hint := live.Memo(cx, a, func() [2]string {
		if !none() {
			return [2]string{}
		}
		reason, section := a.TranslationHint(item())
		return [2]string{reason, section}
	})
	reason := core.Show{When: func() bool { return hint()[0] != "" }, Then: func(*core.Scope) core.Node {
		return core.Text{TestID: "film-translation-reason", ContentFunc: func() string { return hint()[0] }, MaxLines: 2, EllipsizeMode: core.EllipsizeTail,
			Style: core.Style{FontSize: 11, LineHeight: 15}, StyleFunc: ui.Colored(colors.Muted)}
	}}
	translate := core.Dynamic(func(cx *core.Scope) core.Node {
		label, testID := locales.FilmsTranslateMissing, "film-translate-missing"
		if none() {
			label, testID = locales.FilmsTranslate, "film-translate"
		}
		return textButton(label, ui.Button{Variant: ui.ButtonSecondary, Size: ui.SizeSmall, TestID: testID, OnPress: live.Do(a, func() { a.TranslateMissing(item()) })})
	})
	settings := core.Show{When: func() bool { return hint()[1] != "" }, Then: func(*core.Scope) core.Node {
		return textButton(locales.FilmsOpenSettings, ui.Button{Variant: ui.ButtonSecondary, Size: ui.SizeSmall, TestID: "film-translation-settings",
			OnPress: live.Do(a, func() { a.OpenSettings(hint()[1]) })})
	}}
	return core.View{Style: core.Style{Gap: 5, Margin: core.Edges{Top: 3}}, Children: []core.Node{
		reason,
		core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, Gap: 6, AlignSelf: core.AlignStart}, Children: []core.Node{translate, settings}},
	}}
}

// needsSubtitles reports whether item is a video found in the library's folder that no job has processed and that has no subtitles beside it, whose card offers 生成字幕: nothing is replaced, so it starts without asking, as adding a video does.
func needsSubtitles(item Film) bool {
	return item.JobID == "" && item.SubtitlePath == ""
}

// filmMenu is what can be done with a done film, from ⋯ or a right click on its poster or the hero, in the system's menu, 移到资料库 among it while Kando knows another library. Its reveal item names the file manager of the system the app runs on, as the buttons beside it do. It is built as the menu opens, so it says what is true then.
func filmMenu(cx *core.Scope, a App, item Film) []nsdesktop.MenuItem {
	do := func(fn func(Film)) func() { return live.Do(a, func() { fn(item) }) }
	items := []nsdesktop.MenuItem{
		{Label: locales.FilmsPlay(cx), OnSelect: do(a.Open)},
	}
	// 播放 opens the film where it stopped (ResumeFrom), so a film stopped partway can also start over, as the hero's 从头看 does.
	if _, ok := ResumeFrom(a, item); ok {
		items = append(items, nsdesktop.MenuItem{Label: locales.FilmsPlayFromBeginning(cx), OnSelect: live.Do(a, func() { a.OpenAt(item, 0) })})
	}
	items = append(items, watchMenu(cx, a, item)...)
	items = append(items, nsdesktop.MenuItem{Label: locales.FilmsShowInFinder(cx, desktop.Platform(cx)), OnSelect: live.Do(a, func() { a.Reveal(item.VideoPath) })})
	if item.VideoPath != "" {
		items = append(items, nsdesktop.MenuItem{Label: locales.FilmsRename(cx), OnSelect: do(a.AskRename)})
	}
	if needsSubtitles(item) {
		items = append(items, nsdesktop.MenuItem{Label: locales.FilmsMakeSubtitles(cx), OnSelect: do(a.Reprocess)})
	} else {
		items = append(items, nsdesktop.MenuItem{Label: locales.FilmsReprocess(cx), OnSelect: live.Do(a, func() { a.AskReprocess(item) })})
	}
	if item.JobID != "" {
		// A film that was not translated at all is in no language yet, whatever its job was asked to translate it into.
		current := item.Language
		if item.TranslationErr != "" {
			current = ""
		}
		into := subtitle.TranslateIntoMenu(cx, a.TargetLanguage(), current, func(language string) { live.Do(a, func() { a.TranslateAgain(item, language) })() })
		items = append(items, nsdesktop.MenuItem{Label: locales.FilmsTranslateAgain(cx), Items: into})
		switch {
		case item.TranslationErr != "":
			items = append(items, nsdesktop.MenuItem{Label: locales.FilmsTranslate(cx), OnSelect: do(a.TranslateMissing)})
		case item.Untranslated > 0:
			items = append(items, nsdesktop.MenuItem{Label: locales.FilmsTranslateMissing(cx), OnSelect: do(a.TranslateMissing)})
		}
	}
	if move := a.MoveFilmMenu(cx, item); len(move) > 0 {
		items = append(items, nsdesktop.MenuItem{Label: locales.FilmsMoveToLibrary(cx), Items: move})
	}
	return append(items, nsdesktop.MenuItem{Separator: true}, nsdesktop.MenuItem{Label: locales.FilmsRemoveFromLibrary(cx), OnSelect: live.Do(a, func() { a.AskRemove(item) })})
}

// watchMenu is the film's 标记为已看完 and 标记为未看, whichever applies (WatchState): a film not started can be marked watched, a watched one unwatched, and one in progress either. A watched film stopped partway again, as while it is watched a second time, can be marked watched too, which forgets where it stopped, as playing it to its end does. 标记为已看完 needs the film's length, which is how far it marks the film watched to, so it is dimmed until the length is known. A film with no video has nothing to mark.
func watchMenu(cx *core.Scope, a App, item Film) []nsdesktop.MenuItem {
	if item.VideoPath == "" {
		return nil
	}
	state := WatchState(a, item)
	_, resumable := ResumeFrom(a, item)
	var items []nsdesktop.MenuItem
	if state != sidebar.WatchWatched || resumable {
		items = append(items, nsdesktop.MenuItem{Label: locales.FilmsMarkWatched(cx), Disabled: a.Duration(item.VideoPath) <= 0, OnSelect: live.Do(a, func() { a.MarkWatched(item) })})
	}
	if state != sidebar.WatchUnwatched {
		items = append(items, nsdesktop.MenuItem{Label: locales.FilmsMarkUnwatched(cx), OnSelect: live.Do(a, func() { a.MarkUnwatched(item) })})
	}
	return items
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

// The ⋯ button's fill while the pointer is over it and while it is pressed is the theme's shade, and its dots its strong ink (theme.Of).
// moreButton is ⋯ beside a poster's title: a 26-point round button with the prototype's three dots, gray while pressed. It opens the film's menu under it, at its leading edge.
type moreButton struct {
	a      App
	item   func() Film
	testID string
	// menu is the menu it opens, the film's own (filmMenu) when nil.
	menu func() []nsdesktop.MenuItem
}

// Render implements core.Node.
func (m moreButton) Render(cx *core.Scope) core.Node {
	a, k := m.a, theme.Of(cx)
	open := core.NewSignal(cx, false)
	// place is the view around the button, which base.Button takes no Ref for, so its frame gives the button's.
	place := &core.Ref{}
	press := func(core.PressEvent) {
		at := core.Point{}
		if b, ok := place.Bounds(); ok {
			at = core.Point{X: b.Origin.X, Y: b.Origin.Y + b.Size.Height + 4}
		}
		open.Set(true)
		var items []nsdesktop.MenuItem
		if m.menu != nil {
			items = m.menu()
		} else {
			items = filmMenu(cx, a, m.item())
		}
		nsdesktop.PopupMenu(cx, items, at, func() { open.Set(false) })
	}
	button := base.Button{
		TestID:    m.testID,
		LabelFunc: func() string { return locales.FilmsMoreActions(cx, filmName(m.item())) },
		Expanded:  open.Get,
		OnPress:   press,
		Style:     core.Style{Width: core.Pt(26), Height: core.Pt(26), CornerRadius: 13, FlexShrink: 0, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Cursor: core.CursorDefault, FocusVisible: ui.FocusRing(cx, -2)},
		StyleFunc: func() core.Style {
			return core.Style{Hover: &core.Style{Background: k.Shade(0.06)}, Active: &core.Style{Background: k.Shade(0.08)}}
		},
		Children: []core.Node{core.Icon{SVG: moreIcon, Style: core.Style{Width: core.Pt(14), Height: core.Pt(4)}, StyleFunc: ui.Colored(k.Strong)}},
	}
	return core.View{Ref: place, Style: core.Style{FlexShrink: 0}, Children: []core.Node{button}}
}

// doneMeta is the line under a poster's title, in the language of m: where the film comes from, that it has no subtitles yet, and how many lines its translation left untranslated. How long it is shows on the poster.
func doneMeta(m locales.Messages, item Film) string {
	parts := []string{SourceLabel(m, item)}
	if item.SubtitlePath == "" {
		parts = append(parts, m.FilmsNoSubtitles())
	}
	if note := untranslatedNote(m, item); note != "" {
		parts = append(parts, note)
	}
	return strings.Join(parts, " · ")
}

// untranslatedNote says how many lines of a done film the translation left untranslated, as the prototype words a translation that failed for some lines, or that it has no translation when it translated none; "" when it left none.
func untranslatedNote(m locales.Messages, item Film) string {
	if item.TranslationErr != "" {
		return m.FilmsNoTranslation()
	}
	if item.Untranslated <= 0 {
		return ""
	}
	return m.FilmsUntranslated(item.Untranslated)
}

// pressPoster is a press of item's poster among done, the films the grid shows: a ⌘-click or a ⇧-click selects (selectPress); a plain click, or Space or Return on it, empties the selection and opens the film where it stopped last time, as the Apple TV app's posters do, so a click means the same whatever is selected.
func pressPoster(cx *core.Scope, a App, item Film, done []Film, e core.PressEvent) {
	if !e.Keyboard && selectPress(cx, a.Selection(), item.Key, Keys(done), e.Modifiers) {
		return
	}
	a.Selection().Clear()
	a.Open(item)
}

// selectPress selects as a click of the film with key among order, the films the page shows, with mods held does in Finder: ⌘ (Ctrl off Apple's systems, as DataTable reads it) selects it or leaves it out, and ⇧ selects the films from the one selected last to it. It reports whether mods held either; a plain click is the caller's.
func selectPress(cx *core.Scope, sel *Selection, key string, order []string, mods core.Modifiers) bool {
	command := mods.Control
	if core.Untracked(cx, cx.Env().AppleKeys) {
		command = mods.Platform
	}
	switch {
	case command:
		sel.Toggle(key)
	case mods.Shift:
		sel.Extend(order, key)
	default:
		return false
	}
	return true
}
