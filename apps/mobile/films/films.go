// Package films is the Library tab: the films of every paired computer and those on the phone as one library (catalog.Merge), which pull to refresh reads again. At its top are the day's study at a glance (stats.go), and under the films to go on with, the computers' subscriptions, each with its sheet (subscriptions.go). A search finds titles and the lines kept on the phone, the films left partway on any device come first to go on with, and chips and the library switcher narrow the list to a source or a library. A computer that cannot be reached says so with the way to try again or pair again, and before any computer is paired the tab says what pairing brings and how.
package films

import (
	"context"

	"kando/apps/mobile/catalog"
	"kando/apps/mobile/computers"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/onboarding"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// Actions are where the tab leads.
type Actions struct {
	// Pair opens pairing.
	Pair func()
	// Film opens a film's page.
	Film func(catalog.Entry)
	// Play plays the film kept on the phone under key, from where it was left on the phone.
	Play func(key string)
	// Add opens the Add sheet, and Jobs the Jobs page, which a tablet shows as a section of its own instead.
	Add, Jobs func()
}

// view is the tab's state: what the user searched for, the source and the library the list is narrowed to, none for all, and the subscription whose sheet shows, by its key, "" for none.
type view struct {
	cx       *core.Scope
	s        *session.Session
	act      Actions
	query    *core.Signal[string]
	source   *core.Signal[string]
	library  *core.Signal[catalog.Scope]
	shownSub *core.Signal[string]
	entries  *core.Memo[[]catalog.Entry]
	lines    *core.Resource[map[string][]sync.Cue]
}

// Page is the tab's root page.
func Page(cx *core.Scope, s *session.Session, act Actions) base.NavPage {
	v := &view{cx: cx, s: s, act: act,
		query:    core.NewSignal(cx, ""),
		source:   core.NewSignal(cx, ""),
		library:  core.NewSignal(cx, catalog.Scope{}),
		shownSub: core.NewSignal(cx, ""),
		entries:  core.NewMemoFunc(cx, s.Entries, nil),
	}
	// The lines kept on the phone are read for a search once there is one, and again as the films on the phone change. Both inputs are memos, so typing on or a saved position, which change neither, does not read them again: the resource loads afresh whenever its source reruns.
	searching := core.NewMemo(cx, func() bool { return v.query.Get() != "" })
	linesKey := core.NewMemo(cx, s.LinesKey)
	v.lines = core.NewResource(cx, func() (string, bool) {
		return linesKey.Get(), searching.Get()
	}, func(ctx context.Context, key string, _ core.ResourceInfo) (map[string][]sync.Cue, error) {
		return s.Lines(ctx, key), nil
	})
	// A library gone from the merged list, as its computer unpaired or no longer lists it, takes the switcher back to every library. A computer that did not answer lists nothing until it does, so its library stays chosen meanwhile, with the films of it on the phone.
	cx.Effect(func() {
		if lib := v.library.Get(); lib.Desktop != "" && !v.hasLibrary(lib) && !v.unanswered(lib.Desktop) {
			v.library.Set(catalog.Scope{})
		}
	})
	title := func() string { return locales.FilmsTitle(cx) }
	pulled := core.NewSignal(cx, false)
	cx.Effect(func() {
		if pulled.Get() && !s.Listing() {
			pulled.Set(false)
		}
	})
	ref := &core.Ref{}
	body := core.Scroll{TestID: "films-scroll", Ref: ref, Style: core.Style{FlexGrow: 1, FlexShrink: 1},
		OnRefresh:  func() { pulled.Set(true); s.Refresh() },
		Refreshing: pulled.Get,
		Behavior:   core.ScrollBehavior{KeyboardDismissMode: core.KeyboardDismissOnDrag},
		Children: []core.Node{core.View{StyleFunc: look.Column(cx), Children: []core.Node{
			core.Show{When: func() bool { return len(s.Desktops()) == 0 }, Then: func(cx *core.Scope) core.Node { return unpaired(cx, act.Pair) }},
			core.Show{When: func() bool { return len(s.Desktops()) > 0 || len(v.entries.Get()) > 0 }, Then: v.stats},
			core.Show{When: func() bool { return len(v.entries.Get()) > 0 }, Then: v.search},
			core.Each(s.Desktops, func(d store.Desktop) string { return d.ID }, func(cx *core.Scope, d func() store.Desktop, _ func() int) core.Node {
				return computers.Problem(cx, s, d, act.Pair)
			}),
			core.Show{When: func() bool { return v.query.Get() == "" && len(v.resumable()) > 0 }, Then: v.carousel},
			core.Show{When: func() bool { return v.query.Get() == "" && len(v.subscriptions()) > 0 }, Then: v.subscriptionRow},
			v.list(cx),
		}}}}
	jobsLabel := func() string { return locales.JobsTitle(cx) }
	addLabel := func() string { return locales.AddTitle(cx) }
	trailing := []core.Node{ui.IconButton{TestID: "films-add", Icon: lucide.Plus, LabelFunc: addLabel, Size: 44, OnPress: act.Add}}
	tablet := look.Tablet(cx)
	if !tablet {
		trailing = append([]core.Node{ui.IconButton{TestID: "films-jobs", Icon: lucide.ListChecks, LabelFunc: jobsLabel, Size: 44, OnPress: act.Jobs}}, trailing...)
	}
	// Jobs and Add matter once a computer is paired; before, the card that asks to pair is the page's one action.
	paired := func() bool { return len(s.Desktops()) > 0 }
	shown := []core.Node{core.Show{When: paired, Then: func(*core.Scope) core.Node {
		return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter}, Children: trailing}
	}}}
	return look.TabRoot(cx, base.NavPage{TestID: "films-page", Children: []core.Node{body, v.subscriptionSheet(cx)}}, title, shown, func() []core.HeaderSymbol {
		switch {
		case !paired():
			return nil
		case tablet:
			return []core.HeaderSymbol{{Name: "plus", Label: addLabel()}}
		}
		return []core.HeaderSymbol{{Name: "checklist", Label: jobsLabel()}, {Name: "plus", Label: addLabel()}}
	})
}

// hasLibrary reports whether lib is among the libraries the merged list has.
func (v *view) hasLibrary(lib catalog.Scope) bool {
	for _, l := range catalog.Libraries(v.s.Computers(), v.entries.Get()) {
		if l.Desktop == lib.Desktop && l.ID == lib.ID && l.Name == lib.Name {
			return true
		}
	}
	return false
}

// unanswered reports whether the paired computer id has not listed its films since its last read failed, or has not yet.
func (v *view) unanswered(id string) bool {
	if _, paired := v.s.Desktop(id); !paired {
		return false
	}
	sh := v.s.Shelf(id)
	return sh.Err != nil || !sh.Listed
}

// scoped is the merged list narrowed to the library the switcher shows.
func (v *view) scoped() []catalog.Entry {
	lib := v.library.Get()
	if lib.Desktop == "" {
		return v.entries.Get()
	}
	return catalog.Filter(v.entries.Get(), &lib, "")
}

// resumable are the films to go on with, of the library the switcher shows.
func (v *view) resumable() []catalog.Entry { return catalog.Continue(v.scoped(), continueMax) }

// search is the search field over the films' titles and the lines kept on the phone.
func (v *view) search(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{Children: []core.Node{
		ui.Input{TestID: "films-search", Role: core.RoleSearchField, Value: v.query.Get, OnChange: v.query.Set,
			PlaceholderFunc: func() string { return locales.FilmsSearch(cx) }, LabelFunc: func() string { return locales.FilmsSearch(cx) },
			ReturnKeyType: core.ReturnKeySearch, AutoCapitalize: core.AutoCapitalizeNone, NoAutoCorrect: true,
			Style: core.Style{Height: core.Pt(40), FontSize: 16, CornerRadius: 11, BorderWidth: 0, Padding: core.Edges{Left: 36, Right: 12}},
			StyleFunc: func() core.Style {
				return core.Style{Background: ui.Alpha(colors.Text(), 0.06), BorderColor: ui.Alpha(colors.Text(), 0)}
			}},
		core.View{Style: core.Style{Position: core.Absolute, Left: core.Pt(12), Top: core.Pt(0), Bottom: core.Pt(0), JustifyContent: core.JustifyCenter, PointerEvents: core.PointerEventsNone},
			Children: []core.Node{ui.Icon(lucide.Search, 17, colors.Faint)}},
	}}
}

// unpaired is the card the tab starts with while no computer is paired: what pairing brings, the way to pair, and where to get the desktop app.
func unpaired(cx *core.Scope, pair func()) core.Node {
	colors, tints := ui.ColorsOf(cx), ui.TintsOf(cx)
	return core.View{TestID: "films-unpaired", Style: core.Style{CornerRadius: ui.RadiusCard, BorderCurve: core.BorderCurveContinuous, Padding: core.Edges{Top: 28, Right: 20, Bottom: 12, Left: 20}, AlignItems: core.AlignCenter, Gap: 12},
		StyleFunc: func() core.Style { return core.Style{Background: ui.Alpha(colors.Text(), 0.045)} },
		Children: []core.Node{
			core.View{Style: core.Style{Width: core.Pt(64), Height: core.Pt(64), CornerRadius: 16, BorderCurve: core.BorderCurveContinuous, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
				StyleFunc: ui.Filled(tints.Accent), Children: []core.Node{ui.Icon(lucide.Laptop, 32, colors.Accent)}},
			core.Text{ContentFunc: func() string { return locales.FilmsUnpairedTitle(cx) }, Role: core.RoleHeader, Style: look.Heading.Merge(core.Style{TextAlign: core.TextAlignCenter}), StyleFunc: ui.Colored(colors.Text)},
			core.Text{ContentFunc: func() string { return locales.FilmsUnpairedBody(cx) }, Style: look.Subhead.Merge(core.Style{TextAlign: core.TextAlignCenter}), StyleFunc: ui.Colored(colors.Muted)},
			core.View{Style: core.Style{AlignSelf: core.AlignStretch, Gap: 4, Padding: core.Edges{Top: 8}}, Children: []core.Node{
				look.Button{Kind: look.Primary, Text: func() string { return locales.FilmsPair(cx) }, TestID: "films-pair", OnPress: pair},
				look.Button{Kind: look.Plain, Text: func() string { return locales.FilmsNoDesktop(cx) }, TestID: "films-desktop-app", OnPress: func() { cx.OpenURL(onboarding.DownloadURL, nil) }},
			}},
		}}
}
