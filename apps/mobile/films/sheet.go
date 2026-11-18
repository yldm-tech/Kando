package films

import (
	"time"

	"kando/apps/mobile/catalog"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"

	"github.com/nsgo-dev/nsgo/core"
)

// option is an action of a film's options.
type option struct {
	label string
	role  core.AlertRole
	do    func()
}

// options shows what can be done with e's film, as an action sheet on a phone and a dialog in the middle of a tablet's screen: play it, from where it was left or from the start, once it is on the phone, and delete it from the phone; download it, or pause, resume or cancel its download; or open its page.
func (v *view) options(cx *core.Scope, e catalog.Entry) {
	s, key := v.s, e.Key()
	k, kept := s.Film(key)
	t, moving := s.Transfer(key)
	var opts []option
	switch {
	case kept && k.Done():
		opts = append(opts, option{label: locales.FilmPlay(cx), do: func() { v.resume(e) }})
		if catalog.Resumable(e.Position, time.Duration(e.Film.DurationMS)*time.Millisecond) {
			opts = append(opts, option{label: locales.FilmsPlayFromStart(cx), do: func() { v.playAt(key, 0) }})
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
		label := locales.FilmDownload(cx)
		if e.Film.VideoSize > 0 {
			label = locales.FilmsDownloadSize(cx, look.Size(e.Film.VideoSize))
		}
		opts = append(opts, option{label: label, do: func() { s.Download(e.Desktop, e.Library.Name, e.Film) }})
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
