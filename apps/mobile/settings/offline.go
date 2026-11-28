package settings

import (
	"cmp"
	"slices"
	"strconv"
	"strings"

	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// OfflinePage is Settings › Offline Films: how much space the films saved on this device take, their share of the device on a bar and about how much of it is free, once the system said (session.ReadSpace, which the app asks for as it opens the page), and the films in two groups, the downloads not finished, each with how far it got, over the films saved whole, each group largest first (offlineGroups), each film with its poster, its size and Delete, which asks first.
func OfflinePage(cx *core.Scope, s *session.Session, back func()) base.NavPage {
	colors := ui.ColorsOf(cx)
	ref := &core.Ref{}
	unfinished := func() []store.Film { u, _ := offlineGroups(s); return u }
	saved := func() []store.Film { _, w := offlineGroups(s); return w }
	group := func(id string, title func() string, films func() []store.Film) core.Node {
		return core.Show{When: func() bool { return len(films()) > 0 }, Then: func(cx *core.Scope) core.Node {
			return core.View{TestID: id, Children: []core.Node{section(cx, title,
				core.Each(films, func(f store.Film) string { return f.Key() }, func(cx *core.Scope, f func() store.Film, i func() int) core.Node {
					return offlineFilm(cx, s, f, i)
				}))}}
		}}
	}
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
						return core.View{Style: core.Style{Gap: 23}, Children: []core.Node{
							group("offline-unfinished", func() string { return locales.SettingsOfflineUnfinished(cx) }, unfinished),
							group("offline-saved", func() string { return locales.SettingsOfflineSaved(cx) }, saved),
						}}
					},
					Else: func(cx *core.Scope) core.Node {
						return look.Empty{TestID: "offline-empty", Icon: lucide.Download, Title: func() string { return locales.SettingsOfflineEmpty(cx) }}
					}},
			}},
		}},
	}}, ui.NavHeader{OnBack: back, Scroll: ref})
}

// offlineGroups are the films of Offline Films (session.Offline) in the page's two groups: unfinished, those whose download is under way, paused or stopped, and saved, the rest, each largest first by the room it takes (offlineTaken), films of the same size in the order the phone keeps them. Read it in a binding.
func offlineGroups(s *session.Session) (unfinished, saved []store.Film) {
	type sized struct {
		f    store.Film
		size int64
	}
	var u, w []sized
	for _, f := range s.Offline() {
		size, partial := offlineTaken(s, f.Key())
		if partial {
			u = append(u, sized{f, size})
		} else {
			w = append(w, sized{f, size})
		}
	}
	largest := func(list []sized) []store.Film {
		slices.SortStableFunc(list, func(a, b sized) int { return cmp.Compare(b.size, a.size) })
		out := make([]store.Film, len(list))
		for i, it := range list {
			out[i] = it.f
		}
		return out
	}
	return largest(u), largest(w)
}

// offlineTaken is how many bytes of video the film under key takes on the phone, of the copy that stands for it (session.Copy), and whether its download is unfinished: the part downloaded so far of one under way, paused or stopped; the whole video of one saved; none for a film kept by its lines alone or to stream, which session.Offline leaves out. Read it in a binding.
func offlineTaken(s *session.Session, key string) (bytes int64, unfinished bool) {
	key = s.Copy(key)
	if t, ok := s.Transfer(key); ok {
		return t.Done, true
	}
	if f, ok := s.Film(key); ok && f.HasVideo() {
		return f.Film.VideoSize, false
	}
	return 0, false
}

// offlineFilm is a film kept on the device, f, at i in its group: its poster, title, the room it takes and, for a download not finished, where it stands and how far it got on a bar, and Delete, which asks before it deletes it from the device.
func offlineFilm(cx *core.Scope, s *session.Session, f func() store.Film, i func() int) core.Node {
	colors := ui.ColorsOf(cx)
	key := f().Key()
	// A film of a NAS matched to a computer's film is named as the computer names it, and stands for both copies the phone may keep: where the one with its video, or on its way, stands (session.Copy), with the computer's poster.
	named := func() store.Film { lf, _ := s.LinesFilm(key); return lf }
	transfer := func() (session.Transfer, bool) { return s.Transfer(s.Copy(key)) }
	detail := func() string {
		var parts []string
		if n, _ := offlineTaken(s, key); n > 0 {
			parts = append(parts, look.Size(n))
		}
		// The group says a film is saved; a download not finished says where it stands.
		if _, partial := transfer(); partial {
			if st := look.Status(cx, s, key); st != "" {
				parts = append(parts, st)
			}
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
	// The rows are the design's 64 points with a 15/600 title, as the films lists' rows with a poster are, on two lines, as most films' titles are long.
	return look.Row{TestID: "offline-film-" + key, Index: i, TitleLines: 2, MinHeight: 64,
		TitleStyle: core.Style{FontSize: 15, LineHeight: 20, FontWeight: core.FontWeightSemibold},
		Leading: look.Poster(cx, func() core.ImageSource {
			if c, ok := s.Film(named().Key()); ok && c.Poster != "" {
				return look.LocalPoster(s, c)
			}
			return look.LocalPoster(s, f())
		}, 72, "offline-poster-"+key),
		Title:  func() string { return named().Film.Title },
		Detail: detail,
		Below: core.Show{When: func() bool { _, ok := transfer(); return ok }, Then: func(cx *core.Scope) core.Node {
			return ui.Progress{TestID: "offline-progress-" + key, LabelFunc: func() string { return locales.FilmDownload(cx) },
				Value: func() float32 { t, _ := transfer(); return t.Fraction() * 100 }, Style: core.Style{Margin: core.Edges{Top: 4}}}
		}},
		// Delete is a quiet trash can at the row's end, so the list does not read as a column of red buttons; the alert it opens is where the danger shows.
		// TODO(nsgo#2715): NSGo has no swipe actions on a list's row, so Delete is a button on it; make it the row's trailing swipe action, as a system list deletes, once NSGo has them https://github.com/nsgo-dev/nsgo/issues/2715
		Trailing: ui.IconButton{TestID: "offline-delete-" + key, Icon: lucide.Trash, Variant: ui.IconButtonGhost, Size: 44, IconSize: 18, Tint: colors.Muted,
			LabelFunc: func() string { return locales.SettingsOfflineDelete(cx, named().Film.Title) }, OnPress: remove}}
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
