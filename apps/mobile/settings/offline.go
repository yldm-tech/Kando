package settings

import (
	"strings"

	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// OfflinePage is Settings › Offline Films: how much space the films saved on this device take, and the films, each with its poster, its size and where its download stands, and Delete, which asks first.
// TODO(nsgo): no API tells the space left on the device, so the design's bar of the films' share of it and its "about 38 GB free" line are left out.
func OfflinePage(cx *core.Scope, s *session.Session, back func()) base.NavPage {
	colors := ui.ColorsOf(cx)
	ref := &core.Ref{}
	return ui.NavPage(cx, base.NavPage{TitleFunc: func() string { return locales.SettingsOffline(cx) }, TestID: "offline-page", Children: []core.Node{
		core.Scroll{Ref: ref, Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
			core.View{StyleFunc: column(cx), Children: []core.Node{
				core.View{TestID: "offline-used", Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Padding: core.Symmetric(14, 16)},
					StyleFunc: func() core.Style { return look.Card(cx) },
					Children: []core.Node{
						core.Text{ContentFunc: func() string { return locales.SettingsOfflineUse(cx) }, MaxLines: 2, Style: look.Subhead.Merge(core.Style{FlexGrow: 1, FlexShrink: 1}), StyleFunc: ui.Colored(colors.Text)},
						core.Text{TestID: "offline-used-size", ContentFunc: func() string { return look.Size(s.Used()) }, MaxLines: 1,
							Style: look.Subhead.Merge(core.Style{FontWeight: core.FontWeightBold, FlexShrink: 0}), StyleFunc: ui.Colored(colors.Text)},
					}},
				core.Show{When: func() bool { return len(s.Films()) > 0 },
					Then: func(cx *core.Scope) core.Node {
						return section(cx, func() string { return locales.SettingsOfflineSaved(cx) },
							core.Each(s.Films, func(f store.Film) string { return f.Key() }, func(cx *core.Scope, f func() store.Film, i func() int) core.Node {
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
	detail := func() string {
		var parts []string
		if n := f().Film.VideoSize; n > 0 {
			parts = append(parts, look.Size(n))
		}
		if st := look.Status(cx, s, key); st != "" {
			parts = append(parts, st)
		}
		return strings.Join(parts, " · ")
	}
	remove := func() {
		cx.ShowAlert(core.AlertConfig{Title: locales.FilmDeleteTitle(cx, f().Film.Title), Actions: []core.AlertAction{
			{Label: locales.CommonCancel(cx), Role: core.AlertCancel},
			{Label: locales.FilmDeleteConfirm(cx), Role: core.AlertDestructive},
		}}, func(i int) {
			if i == 1 {
				s.Delete(key)
			}
		})
	}
	return look.Row{TestID: "offline-film-" + key, Index: i, TitleLines: 1,
		Leading: look.Poster(cx, func() core.ImageSource { return look.LocalPoster(s, f()) }, 72, ""),
		Title:   func() string { return f().Film.Title },
		Detail:  detail,
		Trailing: core.View{TestID: "offline-delete-" + key, Role: core.RoleButton, LabelFunc: func() string { return locales.FilmDeleteTitle(cx, f().Film.Title) },
			OnPress: func(core.PressEvent) { remove() },
			Style:   core.Style{Height: core.Pt(30), Padding: core.Symmetric(0, 14), CornerRadius: 15, JustifyContent: core.JustifyCenter, FlexShrink: 0},
			StyleFunc: func() core.Style {
				return core.Style{Background: tints.Danger(), Active: &core.Style{Opacity: core.Alpha(0.6)}}
			},
			Children: []core.Node{core.Text{ContentFunc: func() string { return locales.FilmDeleteConfirm(cx) }, MaxLines: 1,
				Style: core.Style{FontSize: 13, LineHeight: 18, FontWeight: core.FontWeightBold}, StyleFunc: ui.Colored(colors.Danger)}}}}
}
