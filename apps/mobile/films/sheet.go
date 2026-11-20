package films

import (
	"slices"
	"time"

	"kando/apps/mobile/catalog"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// option is an action of a film's options: what it is, as the sheet's row keys it, what it says and the short value at its end, whether it takes something away, and what it does.
type option struct {
	id, label, value string
	danger           bool
	do               func()
}

// optionsOf is what can be done with e's film: play it, from where it was left or from the start, once it is on the phone, and delete its files from the phone; save it to the phone for offline, or pause, resume or cancel its download; for a film its computer keeps no video of, keep its lines on the phone and have the computer download the video, and once it has, download the video to the phone; through its computer, export its subtitles to share, translate it again and delete it from the library. Read in a binding, it follows the film's state.
func (v *view) optionsOf(cx *core.Scope, e catalog.Entry) []option {
	s, key := v.s, e.Key()
	k, kept := s.Film(key)
	t, moving := s.Transfer(key)
	q := s.Queue(e.Desktop)
	computer := e.Computer
	if d, ok := s.Desktop(e.Desktop); ok {
		computer = d.Name
	}
	about := ""
	if e.Film.VideoSize > 0 {
		about = locales.FilmsAboutSize(cx, look.Size(e.Film.VideoSize))
	}
	download := func() option {
		o := option{id: "download", label: locales.FilmDownload(cx), value: about, do: func() { s.Download(e.Desktop, e.Library.Name, e.Film) }}
		if e.Film.SubtitlesOnly {
			o.label, o.value = locales.FilmSaveLines(cx), ""
		}
		return o
	}
	var opts []option
	switch {
	case kept && k.Done():
		play := locales.FilmPlay(cx)
		if !k.HasVideo() {
			play = locales.FilmStudyLines(cx)
		}
		opts = append(opts, option{id: "play", label: play, do: func() { v.resume(e) }})
		if k.HasVideo() && catalog.Resumable(e.Position, time.Duration(e.Film.DurationMS)*time.Millisecond) {
			opts = append(opts, option{id: "start", label: locales.FilmsPlayFromStart(cx), do: func() { v.playAt(key, 0) }})
		}
		// A film kept by its lines whose computer has its video now downloads that video.
		if !k.HasVideo() && !e.Film.SubtitlesOnly && s.Client(e.Desktop) != nil {
			opts = append(opts, download())
		}
		del := option{id: "delete", label: locales.FilmDelete(cx), do: func() { v.confirmDelete(cx, e) }}
		if k.HasVideo() {
			del.value = about
		}
		opts = append(opts, del)
	case moving && t.State == session.Running:
		opts = append(opts,
			option{id: "pause", label: locales.FilmPause(cx), do: func() { s.Pause(key) }},
			option{id: "cancel", label: locales.FilmCancel(cx), danger: true, do: func() { s.Delete(key) }})
	case kept:
		opts = append(opts,
			option{id: "resume", label: locales.FilmResume(cx), do: func() { s.Resume(key) }},
			option{id: "cancel", label: locales.FilmCancel(cx), danger: true, do: func() { s.Delete(key) }})
	case s.Client(e.Desktop) != nil:
		opts = append(opts, download())
	}
	_, listed := s.Listed(key)
	if listed && e.Film.SubtitlesOnly && q.Can(sync.OpFilmVideo) && !s.VideoComing(e.Desktop, e.Film.ID) {
		opts = append(opts, option{id: "video", label: locales.FilmVideoOnComputer(cx, computer), do: func() { s.DownloadVideoOnComputer(e.Desktop, e.Film) }})
	}
	if listed && q.Can(sync.OpFilmExport) {
		// The subtitles are exported as SubRip, which its extension says; it is not a word to translate.
		opts = append(opts, option{id: "export", label: locales.FilmsExportSubtitles(cx), value: ".srt", do: func() { v.export(cx, e) }})
	}
	if listed && q.Can(sync.OpFilmRetranslate) {
		st, _ := s.DesktopSettings(e.Desktop)
		opts = append(opts, option{id: "retranslate", label: locales.FilmsRetranslate(cx), value: st.Service, do: func() { s.RetranslateFilm(e.Desktop, e.Film) }})
	}
	if listed && q.Can(sync.OpFilmDelete) {
		opts = append(opts, option{id: "remove", label: locales.FilmsDeleteFromLibrary(cx), danger: true, do: func() { v.confirmDeleteFromLibrary(cx, e, computer) }})
	}
	return opts
}

// options shows e's film's options.
func (v *view) options(e catalog.Entry) { v.sheet.Set(e.Key()) }

// optionsSheet is the sheet of the film whose options show: a sheet from the bottom on a phone and a card in the middle of a tablet's screen, headed by the film's poster, title, source and length, over a card of its options, each with its value at its end. An option pressed closes it, then acts. It closes once the film is gone from the list.
func (v *view) optionsSheet(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	entry := func() (catalog.Entry, bool) {
		key := v.sheet.Get()
		if key == "" {
			return catalog.Entry{}, false
		}
		i := slices.IndexFunc(v.entries.Get(), func(e catalog.Entry) bool { return e.Key() == key })
		if i < 0 {
			return catalog.Entry{}, false
		}
		return v.entries.Get()[i], true
	}
	e := func() catalog.Entry { got, _ := entry(); return got }
	cx.Effect(func() {
		if _, ok := entry(); !ok && v.sheet.Get() != "" {
			cx.Post(func() { v.sheet.Set("") })
		}
	})
	// The rows follow which options there are, not each change of the film: its labels and values are read in their own bindings.
	ids := core.NewMemoFunc(cx, func() []string {
		got, ok := entry()
		if !ok {
			return nil
		}
		var ids []string
		for _, o := range v.optionsOf(cx, got) {
			ids = append(ids, o.id)
		}
		return ids
	}, slices.Equal)
	find := func(id string) option {
		for _, o := range v.optionsOf(cx, e()) {
			if o.id == id {
				return o
			}
		}
		return option{}
	}
	length := func() string {
		if e().Film.DurationMS <= 0 {
			return sourceName(cx, catalog.SourceOf(e().Film))
		}
		return sourceName(cx, catalog.SourceOf(e().Film)) + " · " + look.Duration(time.Duration(e().Film.DurationMS)*time.Millisecond)
	}
	header := core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Padding: core.Edges{Top: 8, Bottom: 4}}, Children: []core.Node{
		look.Poster(cx, poster(cx, v.s, e), 96, 8, "films-options-poster"),
		core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: []core.Node{
			core.Text{TestID: "films-options-title", ContentFunc: func() string { return e().Film.Title }, Role: core.RoleHeader, MaxLines: 2,
				Style: core.Style{FontSize: 16, LineHeight: 21, FontWeight: core.FontWeightBold}, StyleFunc: ui.Colored(colors.Text)},
			core.Text{ContentFunc: length, MaxLines: 1, Style: look.Footnote, StyleFunc: ui.Colored(colors.Muted)},
		}},
	}}
	return look.Dialog{TestID: "films-options", Open: func() bool { _, ok := entry(); return ok }, OnClose: func() { v.sheet.Set("") },
		Title: func() string { return locales.FilmsActions(cx) }, Header: header,
		Children: []core.Node{
			look.Group{Children: []core.Node{
				core.Each(ids.Get, func(id string) string { return id }, func(cx *core.Scope, id func() string, i func() int) core.Node {
					return look.Row{TestID: "films-option-" + id(), Index: i, TitleLines: 2,
						TitleTone: func() look.Tone {
							if find(id()).danger {
								return look.ToneDanger
							}
							return look.ToneDefault
						}(),
						Title: func() string { return find(id()).label },
						Value: func() string { return find(id()).value },
						OnPress: func() {
							o := find(id())
							v.sheet.Set("")
							if o.do != nil {
								o.do()
							}
						}}
				}),
			}},
		}}
}

// export takes e's subtitles from its computer as a file and shows the share sheet for it.
func (v *view) export(cx *core.Scope, e catalog.Entry) {
	v.s.ExportSubtitles(e.Desktop, e.Film, func(path string, err error) {
		if err == nil {
			cx.Share(core.ShareOptions{Files: []core.SharedFile{{Path: path}}}, nil)
		}
	})
}

// confirmDeleteFromLibrary asks before deleting e's film from its computer's library, whose files go to the computer's Trash.
func (v *view) confirmDeleteFromLibrary(cx *core.Scope, e catalog.Entry, computer string) {
	cx.ShowAlert(core.AlertConfig{Title: locales.FilmsDeleteFromLibraryTitle(cx, e.Film.Title), Message: locales.FilmsDeleteFromLibraryMessage(cx, computer), Actions: []core.AlertAction{
		{Label: locales.CommonCancel(cx), Role: core.AlertCancel},
		{Label: locales.FilmDeleteConfirm(cx), Role: core.AlertDestructive},
	}}, func(i int) {
		if i == 1 {
			v.s.DeleteFromComputer(e.Desktop, e.Film)
		}
	})
}

// confirmDelete asks before deleting e's film from the phone, which its computer keeps.
func (v *view) confirmDelete(cx *core.Scope, e catalog.Entry) {
	cx.ShowAlert(core.AlertConfig{Title: locales.FilmDeleteTitle(cx, e.Film.Title), Actions: []core.AlertAction{
		{Label: locales.CommonCancel(cx), Role: core.AlertCancel},
		{Label: locales.FilmDeleteConfirm(cx), Role: core.AlertDestructive},
	}}, func(i int) {
		if i == 1 {
			v.s.Delete(e.Key())
		}
	})
}
