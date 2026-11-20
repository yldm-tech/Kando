// Package films is the Library tab: the films of every paired computer and those on the phone as one library (catalog.Merge), which pull to refresh reads again. At its top are the day's study at a glance (stats.go), and under the films to go on with, the computers' subscriptions, each with its sheet, which a tablet's sidebar lists too (subscriptions.go, sidebar.go). A search finds titles and the lines kept on the phone, the films left partway on any device come first to go on with, and chips and the library switcher narrow the list to a source or a library. A computer that cannot be reached says so with the way to try again or pair again, and before any computer is paired the tab says what pairing brings and how.
package films

import (
	"context"
	"time"

	"kando/apps/mobile/catalog"
	"kando/apps/mobile/computers"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/onboarding"
	"kando/apps/mobile/session"
	"kando/apps/mobile/shell"
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
	// PlayHeld opens the player of the film kept on the phone under key held where it was left, with the recap of the part watched, as the film's page's Play does; a film's row opens a film on the phone with it (openFilm).
	PlayHeld func(key string)
	// Play plays the film kept on the phone under key, from where it was left on the phone, and PlayAt from at.
	Play   func(key string)
	PlayAt func(key string, at time.Duration)
	// Add opens the Add sheet.
	Add func()
	// OpenJobs opens the Jobs tab, as the card of the jobs under way does.
	OpenJobs func()
	// PickLibrary opens the sheet that picks the library the list shows (LibrarySheet); nil for no switcher.
	PickLibrary func()
	// Jobs, when set, puts a Jobs button with the unfinished jobs' count on the page's header, which opens the Jobs tab: the app sets it where the tab bar is the system's, which shows no count (shell.SystemBar).
	Jobs func()
	// Library is the library the list shows, none for all, which a tablet's sidebar names and picks too; nil for the page's own.
	Library *core.Signal[catalog.Scope]
	// Searches counts the times the user asked for the search from the keyboard (shortcuts.Search): the tab focuses its search field each time it grows. Nil for none.
	Searches func() int
	// Subscription opens the sheet of the subscription with that key (SubscriptionSheet), which the app shows over every section.
	Subscription func(key string)
}

// view is the tab's state: what the user searched for, and the source and the library the list is narrowed to, none for all.
type view struct {
	cx      *core.Scope
	s       *session.Session
	act     Actions
	query   *core.Signal[string]
	source  *core.Signal[string]
	library *core.Signal[catalog.Scope]
	entries *core.Memo[[]catalog.Entry]
	lines   *core.Resource[map[string][]sync.Cue]
	// field is the search field's.
	field *core.Ref
	// sheet is the film whose options show (options).
	sheet *core.Signal[optionsSheet]
}

// Page is the tab's root page.
func Page(cx *core.Scope, s *session.Session, act Actions) base.NavPage {
	v := &view{cx: cx, s: s, act: act,
		query:   core.NewSignal(cx, ""),
		source:  core.NewSignal(cx, ""),
		library: act.Library,
		entries: core.NewMemoFunc(cx, s.Entries, nil),
		field:   &core.Ref{},
		sheet:   core.NewSignalFunc(cx, optionsSheet{}, nil),
	}
	if v.library == nil {
		v.library = core.NewSignal(cx, catalog.Scope{})
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
	if act.Searches != nil {
		// The search asked for from another tab shows with this one, which the shell selects first: the field is focused once it has.
		asked := act.Searches()
		cx.Effect(func() {
			if n := act.Searches(); n != asked {
				asked = n
				cx.Post(v.field.Focus)
			}
		})
	}
	title := func() string { return locales.FilmsTitle(cx) }
	pulled := core.NewSignal(cx, false)
	cx.Effect(func() {
		if pulled.Get() && !s.Listing() {
			pulled.Set(false)
		}
	})
	paired := func() bool { return len(s.Desktops()) > 0 }
	tablet := look.Tablet(cx)
	// A tablet's drawn header is one row, the search in it beside the title (critic U2); iOS keeps the system's header, so the search stays in the page there.
	inline := tablet && cx.Env().OS() != core.OSIOS
	ref := &core.Ref{}
	blocks := []core.Node{
		core.Show{When: func() bool { return paired() || len(v.entries.Get()) > 0 }, Then: v.stats},
	}
	if !inline {
		blocks = append(blocks, core.Show{When: func() bool { return len(v.entries.Get()) > 0 || !paired() }, Then: v.search})
	}
	if !tablet {
		// A tablet shows the jobs as the stats' fourth tile instead (stats.go).
		blocks = append(blocks, core.Show{When: v.jobsShown, Then: v.jobsCard})
	}
	blocks = append(blocks,
		core.Show{When: func() bool { return len(v.storageFailures()) > 0 }, Then: v.storageBanner},
		core.Each(s.Desktops, func(d store.Desktop) string { return d.ID }, func(cx *core.Scope, d func() store.Desktop, _ func() int) core.Node {
			return computers.Problem(cx, s, d, act.Pair)
		}),
		core.Show{When: func() bool { return !paired() }, Then: func(cx *core.Scope) core.Node { return unpaired(cx, act.Pair) }},
		core.Show{When: func() bool { return v.query.Get() == "" && len(v.resumable()) > 0 }, Then: v.carousel},
		core.Show{When: func() bool { return v.query.Get() == "" && len(v.subscriptions()) > 0 }, Then: v.subscriptionRow},
		v.list(cx),
	)
	body := core.Scroll{TestID: "films-scroll", Ref: ref, Style: core.Style{FlexGrow: 1, FlexShrink: 1},
		OnRefresh:  func() { pulled.Set(true); s.Refresh() },
		Refreshing: pulled.Get,
		Behavior:   core.ScrollBehavior{KeyboardDismissMode: core.KeyboardDismissOnDrag},
		Children:   []core.Node{core.View{StyleFunc: column(cx), Children: blocks}}}
	addLabel := func() string { return locales.AddTitle(cx) }
	// Before a computer is paired the Add button leads to pairing, which a link needs first.
	add := func() {
		if !paired() {
			act.Pair()
			return
		}
		act.Add()
	}
	var trailing []core.Node
	if inline {
		trailing = []core.Node{core.View{Style: core.Style{Width: core.Pt(300), FlexShrink: 1}, Children: []core.Node{v.search(cx)}}, addPill(cx, addLabel, add)}
	} else {
		trailing = []core.Node{look.CircleButton{TestID: "films-add", Icon: look.Plus, Label: addLabel, Primary: true, OnPress: add}}
	}
	unfinished := func() int { active, failed := s.Unfinished(); return active + failed }
	jobsLabel := func() string {
		if n := unfinished(); n > 0 {
			return locales.JobsBadgeLabel(cx, n)
		}
		return locales.JobsTitle(cx)
	}
	if act.Jobs != nil {
		// TODO(nsgo#741): core.HeaderSymbol is a name and a label only (core/platformprops.go:57), so iOS's native header shows no count on the Jobs button; its label says the count, and the drawn header elsewhere shows the badge. Pass the count once a header symbol takes a badge (UIBarButtonItem.badge), and drop the button once the system's tab bar shows the Jobs tab's (shell.Tab.Badge). https://github.com/nsgo-dev/nsgo/issues/741
		trailing = append([]core.Node{look.CircleButton{TestID: "films-jobs", Icon: lucide.ArrowDownToLine, Label: jobsLabel, OnPress: act.Jobs,
			Badge: look.Count{TestID: "films-jobs-badge", Count: unfinished, Warn: func() bool { _, failed := s.Unfinished(); return failed > 0 },
				Ring: true, Style: core.Style{Position: core.Absolute, Top: core.Pt(3), Right: core.Pt(1)}}}}, trailing...)
	}
	head := look.Head{Title: title, Trailing: trailing, Inline: inline, Symbols: func() []core.HeaderSymbol {
		if act.Jobs != nil {
			return []core.HeaderSymbol{{Name: "arrow.down.to.line", Label: jobsLabel()}, {Name: "plus", Label: addLabel()}}
		}
		return []core.HeaderSymbol{{Name: "plus", Label: addLabel()}}
	}}
	// The switcher heads the page over the title where no sidebar names the library (critic U6: a tablet upright has none), once a computer is paired or while films kept on the phone list without one, as their libraries are still the phone's to pick (design 08).
	if act.PickLibrary != nil {
		head.Leading = core.Show{When: func() bool { return (paired() || len(v.entries.Get()) > 0) && !shell.Sidebar(cx) }, Then: v.switcher}
	}
	return look.TabRootWith(cx, base.NavPage{TestID: "films-page", Children: []core.Node{body, v.optionsNode(cx)}}, head)
}

// column is the tab's content: the page's column on a phone, its blocks 12 apart; a tablet's runs the screen's width, 32 in from its sides (critic U2).
func column(cx *core.Scope) func() core.Style {
	base := look.Column(cx)
	return func() core.Style {
		st := base()
		st.Gap = 12
		st.Padding.Top = 10
		if look.Tablet(cx) {
			st.MaxWidth = core.Pct(100)
			st.Padding.Left, st.Padding.Right = 32, 32
		}
		return st
	}
}

// addPill is a tablet header's Add: a capsule in the accent with a plus and its name, lifted by its shadow.
func addPill(cx *core.Scope, label func() string, press func()) core.Node {
	colors := ui.ColorsOf(cx)
	return base.Button{TestID: "films-add", LabelFunc: label, OnPress: func(core.PressEvent) { press() },
		Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6, Height: core.Pt(38), Padding: core.Symmetric(0, 16), CornerRadius: 19, FlexShrink: 0,
			Active: &core.Style{Opacity: core.Alpha(0.7)}},
		StyleFunc: func() core.Style {
			return core.Style{Background: colors.Accent(), Color: colors.OnAccent(), Shadows: []core.BoxShadow{{Offset: core.Point{Y: 4}, Blur: 12, Color: ui.Alpha(colors.Accent(), 0.3)}}}
		},
		Children: []core.Node{ui.Icon(look.Plus, 14, ui.Inherited), core.Text{ContentFunc: label, MaxLines: 1, Style: core.Style{FontSize: 15, LineHeight: 20, FontWeight: core.FontWeightBold}}}}
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

// search is the search field over the films' titles and the lines kept on the phone: 38 tall on the fill, its magnifier and placeholder in the muted ink.
func (v *view) search(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{Children: []core.Node{
		ui.Input{TestID: "films-search", Ref: v.field, Role: core.RoleSearchField, Value: v.query.Get, OnChange: v.query.Set,
			PlaceholderFunc: func() string { return locales.FilmsSearch(cx) }, LabelFunc: func() string { return locales.FilmsSearch(cx) },
			ReturnKeyType: core.ReturnKeySearch, AutoCapitalize: core.AutoCapitalizeNone, NoAutoCorrect: true,
			Style: core.Style{Height: core.Pt(38), FontSize: 15, CornerRadius: 11, BorderWidth: 0, Padding: core.Edges{Left: 33, Right: 12}},
			StyleFunc: func() core.Style {
				return core.Style{Background: colors.Inset(), BorderColor: ui.Alpha(colors.Text(), 0), PlaceholderColor: colors.Muted()}
			}},
		core.View{Style: core.Style{Position: core.Absolute, Left: core.Pt(11), Top: core.Pt(0), Bottom: core.Pt(0), JustifyContent: core.JustifyCenter, PointerEvents: core.PointerEventsNone},
			Children: []core.Node{ui.Icon(lucide.Search, 15, colors.Muted)}},
	}}
}

// unpaired is the card the tab shows under the stats and the search while no computer is paired: the design's laptop, what pairing brings, the way to pair, and where to get the desktop app.
func unpaired(cx *core.Scope, pair func()) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{TestID: "films-unpaired", Style: core.Style{Padding: core.All(18), AlignItems: core.AlignCenter, Gap: 8},
		StyleFunc: func() core.Style { return look.Card(cx) },
		Children: []core.Node{
			look.LaptopFront(74),
			core.Text{ContentFunc: func() string { return locales.FilmsUnpairedTitle(cx) }, Role: core.RoleHeader,
				Style: core.Style{FontSize: 18, LineHeight: 24, FontWeight: look.Heavy, TextAlign: core.TextAlignCenter, Margin: core.Edges{Top: 6}}, StyleFunc: ui.Colored(colors.Text)},
			core.Text{ContentFunc: func() string { return locales.FilmsUnpairedBody(cx) }, Style: core.Style{FontSize: 14, LineHeight: 20, TextAlign: core.TextAlignCenter}, StyleFunc: ui.Colored(colors.Muted)},
			base.Button{TestID: "films-pair", LabelFunc: func() string { return locales.FilmsPair(cx) }, OnPress: func(core.PressEvent) { pair() },
				Style: core.Style{Height: core.Pt(44), Padding: core.Symmetric(0, 22), CornerRadius: 22, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Margin: core.Edges{Top: 6},
					Active: &core.Style{Opacity: core.Alpha(0.7)}},
				StyleFunc: func() core.Style { return core.Style{Background: colors.Accent(), Color: colors.OnAccent()} },
				Children:  []core.Node{core.Text{ContentFunc: func() string { return locales.FilmsPair(cx) }, MaxLines: 1, Style: core.Style{FontSize: 16, LineHeight: 21, FontWeight: core.FontWeightBold}}}},
			base.Button{TestID: "films-desktop-app", LabelFunc: func() string { return locales.FilmsNoDesktop(cx) }, OnPress: func(core.PressEvent) { cx.OpenURL(onboarding.DownloadURL, nil) },
				Style:     core.Style{MinHeight: core.Pt(36), Padding: core.Symmetric(0, 8), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Active: &core.Style{Opacity: core.Alpha(0.6)}},
				StyleFunc: func() core.Style { return core.Style{Color: colors.Accent()} },
				Children:  []core.Node{core.Text{ContentFunc: func() string { return locales.FilmsNoDesktop(cx) }, MaxLines: 1, Style: core.Style{FontSize: 14, LineHeight: 19, FontWeight: core.FontWeightSemibold}}}},
		}}
}
