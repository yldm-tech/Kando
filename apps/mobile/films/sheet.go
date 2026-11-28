package films

import (
	"strconv"
	"strings"
	"time"

	"kando/apps/mobile/catalog"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// option is an action of a film's options: its label, a short value at its end (a size, a file type, the AI service), whether it takes something away, and what it does.
type option struct {
	label  string
	value  string
	danger bool
	do     func()
}

// optionsSheet is the film whose options show, and the options; none while closed.
type optionsSheet struct {
	entry catalog.Entry
	opts  []option
	open  bool
}

// options shows what can be done with e's film, in a sheet on a phone and a dialog in the middle of a tablet's screen (optionsNode), headed by its poster, title, source and length: play it, from where it was left or from the start, once it is on the phone, and delete it from the phone; download it, or pause, resume or cancel its download; for a film its computer keeps no video of, keep its lines on the phone and have the computer download the video, and once it has, download the video to the phone; through its computer, export its subtitles to share, translate it again and delete it from the library.
func (v *view) options(cx *core.Scope, e catalog.Entry) {
	// What is on the phone of a computer's film matched to a film of a NAS is the copy that stands for both (session.Copy); the options that go through the computer are of the computer's film.
	s := v.s
	key := s.Copy(e.Key())
	k, kept := s.Film(key)
	t, moving := s.Transfer(key)
	q := s.Queue(e.Desktop)
	computer := e.Computer
	if d, ok := s.Desktop(e.Desktop); ok {
		computer = d.Name
	}
	canStream := !e.Film.SubtitlesOnly && s.OnNetwork(e.Desktop)
	if !canStream {
		if rf, ok := s.RelayFilm(e.Desktop, e.Film.ID); ok && rf.Media() != "" {
			canStream = true
		}
	}
	streamPlay := func() {
		s.Stream(e.Desktop, e.Library.Name, e.Film, func(key string) {
			if !e.Position.Mine && e.Position.At > 0 {
				v.playAt(key, e.Position.At)
				return
			}
			v.act.Play(key)
		}, func(error) {})
	}
	_, listed := s.Listed(e.Key())
	var opts []option
	switch {
	case kept && k.Done():
		play := locales.FilmPlay(cx)
		if !k.HasVideo() {
			play = locales.FilmStudyLines(cx)
		}
		opts = append(opts, option{label: play, do: func() { v.resume(e) }})
		if k.HasVideo() && catalog.Resumable(e.Position, time.Duration(e.Film.DurationMS)*time.Millisecond) {
			opts = append(opts, option{label: locales.FilmsPlayFromStart(cx), do: func() { v.playAt(key, 0) }})
		}
		// A film kept by its lines whose computer has its video now downloads that video; while it does, Delete cancels that download alone (session.Delete).
		if !k.HasVideo() && moving && t.State == session.Running {
			opts = append(opts, option{label: locales.FilmCancel(cx), danger: true, do: func() { s.Delete(key) }})
			break
		}
		if !k.HasVideo() && !e.Film.SubtitlesOnly && s.Client(e.Desktop) != nil {
			opts = append(opts, option{label: downloadLabel(cx, e), value: about(cx, downloadSize(e)), do: func() { s.Download(e.Desktop, e.Library.Name, e.Film) }})
		}
		opts = append(opts, v.removal(cx, e, k, listed)...)
	case moving && t.State == session.Running:
		opts = append(opts,
			option{label: locales.FilmPause(cx), do: func() { s.Pause(key) }},
			option{label: locales.FilmCancel(cx), danger: true, do: func() { s.Delete(key) }})
	case s.Matched(e.Key()) && !moving:
		// Streamed from its NAS, or from its computer when the NAS cannot be read, and downloaded from either (session.DownloadCopy).
		opts = append(opts, option{label: locales.FilmPlay(cx), do: func() { v.resume(e) }})
		if catalog.Resumable(e.Position, time.Duration(e.Film.DurationMS)*time.Millisecond) {
			opts = append(opts, option{label: locales.FilmsPlayFromStart(cx), do: func() { v.playAt(key, 0) }})
		}
		opts = append(opts, option{label: downloadLabel(cx, e), value: about(cx, downloadSize(e)), do: func() { s.DownloadCopy(e.Key()) }})
		opts = append(opts, v.removal(cx, e, k, listed)...)
	case kept && k.Streamed && !moving:
		// Kept to stream from its computer or storage: plays while its video can stream, and downloads offline on demand.
		if s.StreamLink(key) != "" {
			opts = append(opts, option{label: locales.FilmPlay(cx), do: func() { v.resume(e) }})
			if catalog.Resumable(e.Position, time.Duration(e.Film.DurationMS)*time.Millisecond) {
				opts = append(opts, option{label: locales.FilmsPlayFromStart(cx), do: func() { v.playAt(key, 0) }})
			}
		}
		if !e.Film.Making && s.Client(e.Desktop) != nil && !s.FilmGone(key) {
			opts = append(opts, option{label: downloadLabel(cx, e), value: about(cx, downloadSize(e)), do: func() { s.Download(e.Desktop, e.Library.Name, e.Film) }})
		}
		opts = append(opts, v.removal(cx, e, k, listed)...)
	case kept:
		// A download that stopped as its computer no longer serves the film has nothing to go on with.
		if !moving || t.State != session.Failed || t.Problem != session.Gone {
			opts = append(opts, option{label: locales.FilmResume(cx), do: func() { s.Resume(key) }})
		}
		opts = append(opts, option{label: locales.FilmCancel(cx), danger: true, do: func() { s.Delete(key) }})
	case e.Film.Making:
		// Being made, it plays from its computer on its network as it is made, and downloads once done.
		if s.OnNetwork(e.Desktop) {
			opts = append(opts, option{label: locales.FilmPlay(cx), do: streamPlay})
		}
	case s.FilmGone(key):
		// Its computer no longer serves it, as a download of it found: nothing of it can be played or downloaded from there.
	case s.Client(e.Desktop) != nil:
		if canStream {
			opts = append(opts, option{label: locales.FilmPlay(cx), do: streamPlay})
		}
		opts = append(opts, option{label: downloadLabel(cx, e), value: about(cx, downloadSize(e)), do: func() { s.Download(e.Desktop, e.Library.Name, e.Film) }})
	}
	if listed && e.Film.SubtitlesOnly && q.Can(sync.OpFilmVideo) && !s.VideoComing(e.Desktop, e.Film.ID) {
		opts = append(opts, option{label: locales.FilmVideoOnComputer(cx, computer), do: func() { s.DownloadVideoOnComputer(e.Desktop, e.Film) }})
	}
	if listed && q.Can(sync.OpFilmExport) {
		opts = append(opts, option{label: locales.FilmsExportSubtitles(cx), value: ".srt", do: func() { v.export(cx, e) }})
	}
	if listed && q.Can(sync.OpFilmRetranslate) {
		opts = append(opts, option{label: locales.FilmsRetranslate(cx), value: service(s, e.Desktop), do: func() { s.RetranslateFilm(e.Desktop, e.Film) }})
	}
	if listed && q.Can(sync.OpFilmDelete) {
		opts = append(opts, option{label: locales.FilmsDeleteFromLibrary(cx), danger: true, do: func() { v.confirmDeleteFromLibrary(cx, e, computer) }})
	}
	v.sheet.Set(optionsSheet{entry: e, opts: opts, open: true})
}

// optionsNode is the sheet of a film's options (options): its poster, title, source and length over a card of the options, those that take something away in danger's red. An option pressed closes the sheet, then does what it says.
func (v *view) optionsNode(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	entry := func() catalog.Entry { return v.sheet.Get().entry }
	close := func() { v.sheet.Update(func(o optionsSheet) optionsSheet { o.open = false; return o }) }
	meta := func() string {
		e := entry()
		parts := []string{look.FilmSource(cx, v.s, e.Desktop, sourceName(cx, catalog.SourceOf(e.Film)))}
		if e.Film.DurationMS > 0 {
			parts = append(parts, look.Duration(time.Duration(e.Film.DurationMS)*time.Millisecond))
		}
		return strings.Join(parts, " · ")
	}
	header := core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12}, Children: []core.Node{
		core.View{Style: core.Style{Width: core.Pt(96), AspectRatio: 16.0 / 9, CornerRadius: 8, BorderCurve: core.BorderCurveContinuous, Overflow: core.Hidden, FlexShrink: 0},
			StyleFunc: ui.Filled(colors.Inset),
			Children: []core.Node{core.Image{SourceFunc: poster(cx, v.s, entry), ResizeMode: core.Cover, ElementsHidden: true,
				Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0)}}}},
		core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: []core.Node{
			core.Text{TestID: "films-options-title", ContentFunc: func() string { return entry().Film.Title }, Role: core.RoleHeader, MaxLines: 2,
				Style: core.Style{FontSize: 16, LineHeight: 21, FontWeight: core.FontWeightBold}, StyleFunc: ui.Colored(colors.Text)},
			core.Text{ContentFunc: meta, MaxLines: 1, Style: look.Footnote, StyleFunc: ui.Colored(colors.Muted)},
		}},
	}}
	return look.Dialog{TestID: "films-options-sheet", Open: func() bool { return v.sheet.Get().open }, OnClose: close, Title: func() string { return entry().Film.Title }, Header: header,
		Children: []core.Node{look.Group{Children: []core.Node{
			core.Each(func() []option { return v.sheet.Get().opts }, func(o option) string { return o.label }, func(cx *core.Scope, o func() option, i func() int) core.Node {
				tone := look.ToneDefault
				if o().danger {
					tone = look.ToneDanger
				}
				return look.Row{TestID: "films-option-" + strconv.Itoa(i()), Index: i, Title: func() string { return o().label }, TitleTone: tone,
					Value: func() string { return o().value }, ValueStyle: look.Caption,
					OnPress: func() {
						do := o().do
						close()
						// A film it plays goes on, by a swipe, through the page's list.
						v.feed(v.keys)
						// The option runs once the sheet has closed, so an alert it asks with shows over the page.
						cx.Post(do)
					}}
			}),
		}}}}
}

// removal is the option that takes away what the phone keeps of e's film, k being the copy that stands for it (session.Copy), named for what that takes (session.DeleteCopies): its offline files where its video, or part of it, is on the phone, the whole video's size at the row's end; the lines saved of a film kept by them alone; and of a film kept only as it was streamed, its lines and record, nothing while a computer lists it, as the film stays in the Library and the next play keeps them again, but the film itself where nothing else lists it, which takes it off the Library. None, nil.
func (v *view) removal(cx *core.Scope, e catalog.Entry, k store.Film, listed bool) []option {
	confirm := func() { v.confirmDelete(cx, e) }
	// The computer's film of a film of a source matched to it is kept by its lines alone where the lines were saved from it.
	c, saved := v.s.Film(e.Key())
	switch {
	case k.HasVideo():
		return []option{{label: locales.FilmDelete(cx), value: about(cx, e.Film.VideoSize), do: confirm}}
	case !k.Done() && !k.Streamed:
		return []option{{label: locales.FilmDelete(cx), do: confirm}}
	case k.LinesOnly || saved && c.LinesOnly:
		return []option{{label: locales.FilmDeleteLines(cx), do: confirm}}
	case !listed:
		return []option{{label: locales.FilmRemove(cx), do: confirm}}
	}
	return nil
}

// about is a size as an option's value says it, "" for none.
func about(cx *core.Scope, size int64) string {
	if size <= 0 {
		return ""
	}
	return locales.FilmsAboutSize(cx, look.Size(size))
}

// downloadSize is the size a download of e's film takes, its video's where the computer said, none for its lines alone.
func downloadSize(e catalog.Entry) int64 {
	if e.Film.SubtitlesOnly {
		return 0
	}
	return e.Film.VideoSize
}

// service is the AI service desktop translates with, as its settings said, "" while unknown.
func service(s *session.Session, desktop string) string {
	st, ok := s.DesktopSettings(desktop)
	if !ok {
		return ""
	}
	return st.Service
}

// downloadLabel is what the option that downloads e's film says: its lines alone for a film its computer keeps no video of, else that it saves it for offline, its size at the row's end (downloadSize).
func downloadLabel(cx *core.Scope, e catalog.Entry) string {
	if e.Film.SubtitlesOnly {
		return locales.FilmSaveLines(cx)
	}
	return locales.FilmsSaveOffline(cx)
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

// confirmDelete asks before deleting e's film from the phone, which its computer keeps: every copy of it there, of a computer's film matched to a film of a NAS both (session.DeleteCopies).
func (v *view) confirmDelete(cx *core.Scope, e catalog.Entry) {
	cx.ShowAlert(core.AlertConfig{Title: locales.FilmDeleteTitle(cx, e.Film.Title), Actions: []core.AlertAction{
		{Label: locales.CommonCancel(cx), Role: core.AlertCancel},
		{Label: locales.FilmDeleteConfirm(cx), Role: core.AlertDestructive},
	}}, func(i int) {
		if i == 1 {
			v.s.DeleteCopies(e.Key())
		}
	})
}
