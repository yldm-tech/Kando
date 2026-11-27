package films

import (
	"kando/apps/mobile/catalog"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// WorkPage is the page of the work of computer desktop whose ID is id, which its row in the list opens: its name, how many of its films the phone lists and how many were watched, the button that goes on with it (catalog.GoOn), and every one of its films the phone lists, in the order it plays, each the row the list shows it by, whatever the list's chip or search, as the page is the work's and not the list's. Once the phone lists none of its films, as its computer was unpaired, it goes back (back).
func WorkPage(cx *core.Scope, s *session.Session, act Actions, desktop, id string, back func()) base.NavPage {
	colors := ui.ColorsOf(cx)
	v := newView(cx, s, act)
	work := core.NewMemoFunc(cx, func() catalog.Work { w, _ := catalog.WorkOf(v.entries.Get(), desktop, id); return w }, nil)
	gone := false
	cx.Effect(func() {
		if len(work.Get().Films) == 0 && !gone {
			gone = true
			cx.Post(back)
		}
	})
	goOn := func() (catalog.Entry, int) { return catalog.GoOn(work.Get()) }
	button := func() string {
		e, how := goOn()
		if e.Film.ID == "" {
			return ""
		}
		switch how {
		case catalog.GoOnStart:
			return locales.FilmsWorkStart(cx, e.Film.WorkIndex)
		case catalog.GoOnAgain:
			return locales.FilmsWorkAgain(cx, e.Film.WorkIndex)
		}
		return locales.FilmsWorkContinue(cx, e.Film.WorkIndex)
	}
	v.keys = func() []string {
		var keys []string
		for _, e := range work.Get().Films {
			keys = append(keys, e.Key())
		}
		return keys
	}
	films := func() []catalog.Result {
		var out []catalog.Result
		for _, e := range work.Get().Films {
			out = append(out, catalog.Result{Entry: e})
		}
		return out
	}
	each := core.Each(films, func(r catalog.Result) string { return r.Key() }, func(cx *core.Scope, r func() catalog.Result, i func() int) core.Node {
		if look.Tablet(cx) {
			return v.cell(cx, r)
		}
		return v.row(cx, r, i)
	})
	var list core.Node
	if look.Tablet(cx) {
		list = core.View{TestID: "work-films", Style: core.Style{Direction: core.Row, Wrap: core.Wrap, RowGap: 22, Margin: core.Edges{Right: -8, Left: -8}}, Children: []core.Node{each}}
	} else {
		list = core.View{TestID: "work-films", StyleFunc: func() core.Style {
			st := look.Card(cx)
			st.Overflow = core.Hidden
			return st
		}, Children: []core.Node{each}}
	}
	ref := &core.Ref{}
	return look.NavPage(cx, base.NavPage{TestID: "work-page", Children: []core.Node{
		core.Scroll{Ref: ref, Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
			core.View{StyleFunc: column(cx), Children: []core.Node{
				core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
					core.Text{TestID: "work-title", ContentFunc: func() string { return work.Get().Title }, Role: core.RoleHeader, MaxLines: 3, Style: look.Heading, StyleFunc: ui.Colored(colors.Text)},
					core.Text{TestID: "work-watched", ContentFunc: func() string {
						w := work.Get()
						return locales.FilmsWorkWatched(cx, len(w.Films), len(w.Films)-catalog.Unwatched(w))
					}, MaxLines: 1, Style: look.Subhead, StyleFunc: ui.Colored(colors.Muted)},
				}},
				look.Button{Kind: look.Primary, Text: button, TestID: "work-go-on", OnPress: func() {
					v.feed(v.keys)
					workGoOn(s, act, goOn)
				}},
				list,
			}},
		}},
		v.optionsNode(cx),
	}}, ui.NavHeader{OnBack: back, Scroll: ref})
}

// workGoOn plays the film the work goes on with (catalog.GoOn) where it was left, as Continue Watching does (Resume); a work watched through starts its first film again from the beginning, once its video can play on the phone. A film kept by its lines alone, or not on the phone yet, opens its page.
func workGoOn(s *session.Session, act Actions, goOn func() (catalog.Entry, int)) {
	e, how := goOn()
	if e.Film.ID == "" {
		return
	}
	k, ok := s.Film(e.Key())
	playable := ok && (k.Done() && k.HasVideo() || k.FromSource() || (k.Streamed && s.StreamLink(e.Key()) != "")) || s.Matched(e.Key())
	if !playable {
		act.Film(e)
		return
	}
	if how == catalog.GoOnAgain {
		act.PlayAt(e.Key(), 0)
		return
	}
	if !e.Position.Mine && e.Position.At > 0 {
		act.PlayAt(e.Key(), e.Position.At)
		return
	}
	act.Play(e.Key())
}
