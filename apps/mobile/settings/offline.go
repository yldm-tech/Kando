package settings

import (
	"strconv"
	"strings"

	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// OfflinePage is Settings › Offline Films: how much space the films saved on this device take, their share of the device on a bar and about how much of it is free, once the system said (session.ReadSpace, which the app asks for as it opens the page), and the films, each with its poster, its size and where its download stands, and Delete, which asks first.
func OfflinePage(cx *core.Scope, s *session.Session, back func()) base.NavPage {
	colors := ui.ColorsOf(cx)
	ref := &core.Ref{}
	return look.NavPage(cx, base.NavPage{TitleFunc: func() string { return locales.SettingsOffline(cx) }, TestID: "offline-page", Children: []core.Node{
		core.Scroll{Ref: ref, Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
			core.View{StyleFunc: column(cx), Children: []core.Node{
				core.View{TestID: "offline-used", Style: core.Style{Gap: 10, Padding: core.Symmetric(14, 16)},
					StyleFunc: func() core.Style { return look.Card(cx) },
					Children: []core.Node{
						core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12}, Children: []core.Node{
							core.Text{ContentFunc: func() string { return locales.SettingsOfflineUse(cx) }, MaxLines: 2, Style: look.Subhead.Merge(core.Style{FlexGrow: 1, FlexShrink: 1}), StyleFunc: ui.Colored(colors.Text)},
							core.Text{TestID: "offline-used-size", ContentFunc: func() string { return look.Size(s.Used()) }, MaxLines: 1,
								Style: look.Subhead.Merge(core.Style{FontWeight: core.FontWeightBold, FlexShrink: 0}), StyleFunc: ui.Colored(colors.Text)},
						}},
						core.Show{When: func() bool { _, ok := s.Space(); return ok }, Then: func(cx *core.Scope) core.Node { return offlineSpace(cx, s) }},
					}},
				core.Show{When: func() bool { return len(s.Offline()) > 0 },
					Then: func(cx *core.Scope) core.Node {
						return section(cx, func() string { return locales.SettingsOfflineSaved(cx) },
							core.Each(s.Offline, func(f store.Film) string { return f.Key() }, func(cx *core.Scope, f func() store.Film, i func() int) core.Node {
								return offlineFilm(cx, s, f, i)
							}))
					},
					Else: func(cx *core.Scope) core.Node {
						return core.Text{TestID: "offline-empty", ContentFunc: func() string { return locales.SettingsOfflineEmpty(cx) },
							Style: core.Style{FontSize: 14, LineHeight: 20, TextAlign: core.TextAlignCenter, Padding: core.Symmetric(24, 16)}, StyleFunc: ui.Colored(colors.Muted)}
					}},
			}},
		}},
	}}, ui.NavHeader{OnBack: back, Scroll: ref})
}

// offlineFilm is a film saved on the device, f, at i in the list: its poster, title, size and where its download stands, and Delete, which asks before it deletes it from the device.
func offlineFilm(cx *core.Scope, s *session.Session, f func() store.Film, i func() int) core.Node {
	colors, tints := ui.ColorsOf(cx), ui.TintsOf(cx)
	key := f().Key()
	// A film of a NAS matched to a computer's film is named as the computer names it, and stands for both copies the phone may keep: where the one with its video, or on its way, stands (session.Copy), with the computer's poster.
	named := func() store.Film { lf, _ := s.LinesFilm(key); return lf }
	copied := func() store.Film { c, _ := s.Film(s.Copy(key)); return c }
	detail := func() string {
		var parts []string
		if n := f().Film.VideoSize; n > 0 {
			parts = append(parts, look.Size(n))
		}
		// A film kept whole says it is saved, as the design's list does; one on its way or stopped says where it stands.
		if fl := copied(); fl.Done() && fl.HasVideo() {
			parts = append(parts, locales.SettingsOfflineFilmSaved(cx))
		} else if st := look.Status(cx, s, key); st != "" {
			parts = append(parts, st)
		}
		return strings.Join(parts, " · ")
	}
	remove := func() {
		cx.ShowAlert(core.AlertConfig{Title: locales.FilmDeleteTitle(cx, named().Film.Title), Actions: []core.AlertAction{
			{Label: locales.CommonCancel(cx), Role: core.AlertCancel},
			{Label: locales.FilmDeleteConfirm(cx), Role: core.AlertDestructive},
		}}, func(i int) {
			if i == 1 {
				s.DeleteCopies(key)
			}
		})
	}
	// The rows are the design's 64 points with a 15/600 title, as the films lists' rows with a poster are.
	return look.Row{TestID: "offline-film-" + key, Index: i, TitleLines: 1, MinHeight: 64,
		TitleStyle: core.Style{FontSize: 15, LineHeight: 20, FontWeight: core.FontWeightSemibold},
		Leading: look.Poster(cx, func() core.ImageSource {
			if c, ok := s.Film(named().Key()); ok && c.Poster != "" {
				return look.LocalPoster(s, c)
			}
			return look.LocalPoster(s, f())
		}, 72, ""),
		Title:  func() string { return named().Film.Title },
		Detail: detail,
		// Delete is a capsule in danger's color on its tint; ui.Button takes its text as a string, which would not follow the language, and has no danger look, so it is base.Button in the design's style.
		Trailing: base.Button{TestID: "offline-delete-" + key, LabelFunc: func() string { return locales.SettingsOfflineDelete(cx, named().Film.Title) },
			OnPress: func(core.PressEvent) { remove() },
			Style:   core.Style{Height: core.Pt(30), Padding: core.Symmetric(0, 12), CornerRadius: 15, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, FlexShrink: 0},
			StyleFunc: func() core.Style {
				return core.Style{Background: tints.Danger(), Active: &core.Style{Opacity: core.Alpha(0.6)}, FocusVisible: ui.FocusRing(cx, 2)}
			},
			Children: []core.Node{core.Text{ContentFunc: func() string { return locales.FilmDeleteConfirm(cx) }, MaxLines: 1,
				Style: core.Style{FontSize: 13, LineHeight: 18, FontWeight: core.FontWeightBold}, StyleFunc: ui.Colored(colors.Danger)}}}}
}

// offlineSpace is the device's room under what the films take: a bar of the device, the films' share of it in the accent and what the rest of the device holds in grey, and about how much is free. The texts over and under it say what it shows, so assistive technology skips the bar.
func offlineSpace(cx *core.Scope, s *session.Session) core.Node {
	colors := ui.ColorsOf(cx)
	// share is part's share of the device, in percent; a part that takes any room shows at least a sliver.
	share := func(part func(room core.DiskSpace) int64) func() core.Style {
		return func() core.Style {
			room, _ := s.Space()
			n := max(part(room), 0)
			if n == 0 || room.Total <= 0 {
				return core.Style{Display: core.DisplayNone}
			}
			return core.Style{Width: core.Pct(float32(min(float64(n)*100/float64(room.Total), 100))), MinWidth: core.Pt(3)}
		}
	}
	films := func(core.DiskSpace) int64 { return s.Used() }
	others := func(room core.DiskSpace) int64 { return room.Total - room.Available - s.Used() }
	return core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
		core.View{TestID: "offline-space-bar", AccessibilityElementsHidden: func() bool { return true },
			Style:     core.Style{Direction: core.Row, Height: core.Pt(8), CornerRadius: 4, Overflow: core.Hidden},
			StyleFunc: func() core.Style { return core.Style{Background: colors.Inset()} },
			Children: []core.Node{
				core.View{TestID: "offline-space-films", StyleFunc: func() core.Style { return share(films)().Merge(core.Style{Background: colors.Accent()}) }},
				core.View{TestID: "offline-space-others", StyleFunc: func() core.Style { return share(others)().Merge(core.Style{Background: colors.Faint()}) }},
			}},
		core.Text{TestID: "offline-space-free", ContentFunc: func() string {
			room, _ := s.Space()
			return locales.SettingsOfflineFree(cx, roughSize(room.Available))
		}, MaxLines: 1, Style: look.Caption, StyleFunc: ui.Colored(colors.Muted)},
	}}
}

// roughSize is a size in whole units, as 38 GB or 512 MB, for room that is only about so much.
func roughSize(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return strconv.FormatInt((bytes+1<<29)>>30, 10) + " GB"
	case bytes >= 1<<20:
		return strconv.FormatInt((bytes+1<<19)>>20, 10) + " MB"
	}
	return look.Size(bytes)
}
