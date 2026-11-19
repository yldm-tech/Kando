package films

import (
	"time"

	"kando/apps/mobile/catalog"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/core"
)

// option is an action of a film's options.
type option struct {
	label string
	role  core.AlertRole
	do    func()
}

// options shows what can be done with e's film, as an action sheet on a phone and a dialog in the middle of a tablet's screen: play it, from where it was left or from the start, once it is on the phone, and delete it from the phone; download it, or pause, resume or cancel its download; for a film its computer keeps no video of, keep its lines on the phone and have the computer download the video, and once it has, download the video to the phone; through its computer, export its subtitles to share, translate it again and delete it from the library; or open its page.
func (v *view) options(cx *core.Scope, e catalog.Entry) {
	s, key := v.s, e.Key()
	k, kept := s.Film(key)
	t, moving := s.Transfer(key)
	q := s.Queue(e.Desktop)
	computer := e.Computer
	if d, ok := s.Desktop(e.Desktop); ok {
		computer = d.Name
	}
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
		// A film kept by its lines whose computer has its video now downloads that video.
		if !k.HasVideo() && !e.Film.SubtitlesOnly && s.Client(e.Desktop) != nil {
			opts = append(opts, option{label: downloadLabel(cx, e), do: func() { s.Download(e.Desktop, e.Library.Name, e.Film) }})
		}
		opts = append(opts, option{label: locales.FilmDelete(cx), role: core.AlertDestructive, do: func() { v.confirmDelete(cx, e) }})
	case moving && t.State == session.Running:
		opts = append(opts,
			option{label: locales.FilmPause(cx), do: func() { s.Pause(key) }},
			option{label: locales.FilmCancel(cx), role: core.AlertDestructive, do: func() { s.Delete(key) }})
	case kept:
		opts = append(opts,
			option{label: locales.FilmResume(cx), do: func() { s.Resume(key) }},
			option{label: locales.FilmCancel(cx), role: core.AlertDestructive, do: func() { s.Delete(key) }})
	case s.Client(e.Desktop) != nil:
		opts = append(opts, option{label: downloadLabel(cx, e), do: func() { s.Download(e.Desktop, e.Library.Name, e.Film) }})
	}
	_, listed := s.Listed(key)
	if listed && e.Film.SubtitlesOnly && q.Can(sync.OpFilmVideo) && !s.VideoComing(e.Desktop, e.Film.ID) {
		opts = append(opts, option{label: locales.FilmVideoOnComputer(cx, computer), do: func() { s.DownloadVideoOnComputer(e.Desktop, e.Film) }})
	}
	if listed && q.Can(sync.OpFilmExport) {
		opts = append(opts, option{label: locales.FilmsExportSubtitles(cx), do: func() { v.export(cx, e) }})
	}
	if listed && q.Can(sync.OpFilmRetranslate) {
		opts = append(opts, option{label: locales.FilmsRetranslate(cx), do: func() { s.RetranslateFilm(e.Desktop, e.Film) }})
	}
	if listed && q.Can(sync.OpFilmDelete) {
		opts = append(opts, option{label: locales.FilmsDeleteFromLibrary(cx), role: core.AlertDestructive, do: func() { v.confirmDeleteFromLibrary(cx, e, computer) }})
	}
	opts = append(opts, option{label: locales.FilmsDetails(cx), do: func() { v.act.Film(e) }})
	actions := make([]core.AlertAction, 0, len(opts)+1)
	for _, o := range opts {
		actions = append(actions, core.AlertAction{Label: o.label, Role: o.role})
	}
	actions = append(actions, core.AlertAction{Label: locales.CommonCancel(cx), Role: core.AlertCancel})
	cx.ShowAlert(core.AlertConfig{Title: e.Film.Title, Message: look.Facts(e.Film), Sheet: !look.Tablet(cx), Cancelable: true, Actions: actions}, func(i int) {
		if i >= 0 && i < len(opts) {
			opts[i].do()
		}
	})
}

// downloadLabel is what the option that downloads e's film says: its lines alone for a film its computer keeps no video of, else its video, with its size where its computer said.
func downloadLabel(cx *core.Scope, e catalog.Entry) string {
	switch {
	case e.Film.SubtitlesOnly:
		return locales.FilmSaveLines(cx)
	case e.Film.VideoSize > 0:
		return locales.FilmsDownloadSize(cx, look.Size(e.Film.VideoSize))
	}
	return locales.FilmDownload(cx)
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
