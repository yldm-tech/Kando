// Package browse is Browse: the sites learners study from and the videos last looked at, the sites' own pages in the system's web view under a bar to go back, type an address and come back to the start, and over it, on a page of one video of a site Kando knows (kcore.VideoLink, as the computer's browser knows them), a bar that sends the video to the computer, which downloads, transcribes and translates it while the Jobs page follows it. Signing in to a site happens on its own pages and is the phone's alone, for browsing; the computer signs in to download. Store builds leave the tab out (Available).
package browse

import (
	"errors"
	"net/url"
	"strings"

	"kando/apps/mobile/add"
	"kando/apps/mobile/langs"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"
	kcore "kando/packages/core"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// Actions are where Browse leads: Pair opens pairing, for a page's video to have a computer to go to. Shown reports whether the tab shows, as the shell keeps it while another does, and nothing covers it, as pairing or the player.
type Actions struct {
	Pair  func()
	Shown func() bool
}

// pauseScript pauses what a page plays, as it goes out of sight: behind the start, or under another tab.
const pauseScript = `for (const m of document.querySelectorAll("video, audio")) m.pause();`

// browser is what Browse shows: the address the web view was first given, "" until a page is first opened, which makes the web view; whether the page shows rather than the start; the page's address, title and history as the web view and the page report them; whether it failed to load; the address field's text while it has the keyboard; and the videos sent from here, by their links, with how many were sent, which the bindings that read them follow.
type browser struct {
	s       *session.Session
	act     Actions
	web     *core.Ref
	field   *core.Ref
	source  *core.Signal[string]
	shown   *core.Signal[bool]
	nav     *core.Signal[core.WebNavigation]
	failed  *core.Signal[bool]
	editing *core.Signal[bool]
	typed   *core.Signal[string]
	sent    map[string]bool
	sends   *core.Signal[int]
}

// Page is the tab's root page: the start, or the page open, over the bars.
func Page(cx *core.Scope, s *session.Session, act Actions) base.NavPage {
	colors := ui.ColorsOf(cx)
	b := &browser{s: s, act: act, web: &core.Ref{}, field: &core.Ref{},
		source: core.NewSignal(cx, ""), shown: core.NewSignal(cx, false), nav: core.NewSignal(cx, core.WebNavigation{}),
		failed: core.NewSignal(cx, false), editing: core.NewSignal(cx, false), typed: core.NewSignal(cx, ""),
		sent: map[string]bool{}, sends: core.NewSignal(cx, 0)}
	// A page out of sight does not go on playing.
	cx.Effect(func() {
		if !b.shown.Get() || (act.Shown != nil && !act.Shown()) {
			b.web.InjectJavaScript(pauseScript)
		}
	})
	hidden := func(hide func() bool) func() core.Style {
		return func() core.Style {
			if hide() {
				return core.Style{Display: core.DisplayNone}
			}
			return core.Style{}
		}
	}
	return base.NavPage{TestID: "browse-page", HeaderHidden: true, StyleFunc: ui.Filled(colors.Background), Children: []core.Node{
		core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)}, Children: []core.Node{
			// The start stays built while a page shows, keeping where it was scrolled to.
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)}, StyleFunc: hidden(b.shown.Get), Children: []core.Node{b.start(cx)}},
			// The web view is made with the first page opened and kept after, so the start shows in its place while it keeps its pages.
			core.Show{When: func() bool { return b.source.Get() != "" }, Then: func(cx *core.Scope) core.Node {
				return core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0), PaddingEnv: core.EnvEdges{Top: core.EnvSafeArea}},
					StyleFunc: hidden(func() bool { return !b.shown.Get() }), Children: []core.Node{webPage{b}}}
			}},
		}},
		look.ActionBar(cx, b.videoBar(cx), b.failure(cx), b.toolbar(cx)),
	}}
}

// open shows the page at address: the web view's first, which makes it, or another the web view loads.
func (b *browser) open(address string) {
	b.failed.Set(false)
	b.shown.Set(true)
	if b.source.Get() == "" {
		b.source.Set(address)
		return
	}
	b.web.Load(core.WebSrc(address))
}

// home shows the start, the page kept behind it.
func (b *browser) home() {
	b.field.Blur()
	b.shown.Set(false)
}

// back goes back a page: in the page's history, else to the start; from the start, back to the page it was left on.
func (b *browser) back() {
	switch {
	case !b.shown.Get():
		if b.source.Get() != "" {
			b.shown.Set(true)
		}
	case b.nav.Get().CanGoBack:
		b.web.GoBack()
	default:
		b.home()
	}
}

// navigated takes the page's address and history as the web view reports them, and keeps a video looked at once it has loaded.
func (b *browser) navigated(n core.WebNavigation) {
	if n.Loading {
		b.failed.Set(false)
	}
	b.nav.Set(n)
	if !n.Loading {
		b.visited(n.URL, n.Title)
	}
}

// heard takes the address and title a page posted as it moved within itself or named itself (pageScript).
func (b *browser) heard(msg core.WebMessage) {
	p, ok := parsePage(msg)
	if !ok {
		return
	}
	n := b.nav.Get()
	if p.URL != n.URL {
		// A page that pushed an address has the one it left to go back to.
		n.CanGoBack, n.CanGoForward = true, false
	}
	n.URL, n.Title = p.URL, p.Title
	b.nav.Set(n)
	b.visited(n.URL, n.Title)
}

// visited keeps the video at address among the last ones looked at, when the page is one video's.
func (b *browser) visited(address, title string) {
	if kcore.VideoLink(address) {
		b.s.Visit(address, kcore.VideoTitle(address, title))
	}
}

// fail tells a page that did not load; a navigation the web view refused, as to another app's link, leaves the page as it was.
func (b *browser) fail(err error) {
	var failed *core.WebError
	if errors.As(err, &failed) && failed.URL != "" && !kcore.WebAddress(failed.URL) {
		return
	}
	b.failed.Set(true)
}

// video is the link of the video the page shows, "" while it shows none or the start shows.
func (b *browser) video() string {
	if !b.shown.Get() {
		return ""
	}
	if u := b.nav.Get().URL; kcore.VideoLink(u) {
		return u
	}
	return ""
}

// webPage is the web view showing the sites' pages, which go to http and https addresses at the top level and nowhere else (webOnly), and reach the app only through what they post (pageScript).
type webPage struct{ b *browser }

// Render implements core.Node.
func (w webPage) Render(cx *core.Scope) core.Node {
	b := w.b
	cx.ProvideAccess(webOnly{})
	return core.WebView{TestID: "browse-web", Ref: b.web,
		SourceFunc:                      func() core.WebSource { return core.WebSrc(b.source.Get()) },
		InjectedJavaScriptBeforeContent: pageScript,
		BackForwardGestures:             true,
		PinchZoom:                       true,
		OnNavigationStateChange:         b.navigated,
		OnError:                         b.fail,
		MessageOrigins:                  []string{"*"},
		OnMessageFrom:                   b.heard,
		Style:                           core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0)},
	}
}

// start is the page before a site is open, and after the start button: what Browse is for, the sites learners study from most, the videos last looked at, and what signing in here is for.
func (b *browser) start(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	visits := func() []store.Visit { return b.s.Prefs().Visited }
	heading := func() string { return locales.BrowseTitle(cx) }
	return core.Scroll{TestID: "browse-start", Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
		core.View{StyleFunc: func() core.Style {
			st := look.ContentStyle(cx)
			st.Padding.Top, st.Padding.Bottom, st.PaddingEnv.Top = 12, 24, core.EnvSafeArea
			st.Gap = 12
			return st
		}, Children: []core.Node{
			core.View{Style: core.Style{Gap: 6, Padding: core.Edges{Top: 12, Bottom: 8}}, Children: []core.Node{
				core.Text{ContentFunc: heading, Role: core.RoleHeader, MaxLines: 1, Style: look.Display, StyleFunc: ui.Colored(colors.Text)},
				core.Text{TestID: "browse-intro", ContentFunc: func() string { return locales.BrowseIntro(cx) }, Style: look.Subhead, StyleFunc: ui.Colored(colors.Muted)},
			}},
			look.SectionTitle(cx, func() string { return locales.BrowseSites(cx) }),
			b.tiles(cx),
			core.Show{When: func() bool { return len(visits()) > 0 }, Then: func(cx *core.Scope) core.Node {
				return core.View{Style: core.Style{Gap: 12, Padding: core.Edges{Top: 16}}, Children: []core.Node{
					look.SectionTitle(cx, func() string { return locales.BrowseRecent(cx) }),
					look.Group{TestID: "browse-recent", Children: []core.Node{
						core.Each(visits, func(v store.Visit) string { return v.URL }, func(cx *core.Scope, v func() store.Visit, i func() int) core.Node {
							return b.recent(cx, v, i)
						}),
					}},
				}}
			}},
			core.Text{TestID: "browse-note", ContentFunc: func() string { return locales.BrowseNote(cx) }, Style: look.Caption.Merge(core.Style{Padding: core.Edges{Top: 8, Left: 4, Right: 4}}),
				StyleFunc: ui.Colored(colors.Muted)},
		}},
	}}
}

// tiles are the sites, three to a row on a phone and all in one on a tablet; pressing one opens it.
// TODO(nsgo#112): mark the sites signed in on this phone with a check, and list them in Settings with a way to sign out of each, once NSGo lists and clears the web views' cookies https://github.com/nsgo-dev/nsgo/issues/112
func (b *browser) tiles(cx *core.Scope) core.Node {
	columns := 3
	if look.Tablet(cx) {
		columns = len(sites)
	}
	var rows []core.Node
	for i := 0; i < len(sites); i += columns {
		var row []core.Node
		for _, s := range sites[i:min(i+columns, len(sites))] {
			row = append(row, b.tile(cx, s))
		}
		rows = append(rows, core.View{Style: core.Style{Direction: core.Row, Gap: 10}, Children: row})
	}
	return core.View{TestID: "browse-sites", Style: core.Style{Gap: 10}, Children: rows}
}

// tile is a site's mark over its name.
func (b *browser) tile(cx *core.Scope, s site) core.Node {
	colors := ui.ColorsOf(cx)
	name := func() string { return s.name(cx) }
	return base.Button{TestID: "browse-site-" + s.id, LabelFunc: name, OnPress: func(core.PressEvent) { b.open(s.url) },
		Style: core.Style{FlexGrow: 1, FlexShrink: 1, FlexBasis: core.Pt(0), MinWidth: core.Pt(0), AlignItems: core.AlignCenter, Gap: 8, Padding: core.Edges{Top: 14, Right: 6, Bottom: 12, Left: 6},
			CornerRadius: ui.RadiusCard, BorderCurve: core.BorderCurveContinuous, Active: &core.Style{Opacity: core.Alpha(0.7)}},
		StyleFunc: func() core.Style { return core.Style{Background: ui.Alpha(colors.Text(), 0.045), Color: colors.Text()} },
		Children: []core.Node{
			mark(s, 48),
			core.Text{ContentFunc: name, MaxLines: 1, Style: look.Caption.Merge(core.Style{FontWeight: core.FontWeightSemibold, TextAlign: core.TextAlignCenter})},
		}}
}

// mark is a site's mark, size points square: its letter in white on the brand's color.
func mark(s site, size core.Points) core.Node {
	return core.View{Style: core.Style{Width: core.Pt(size), Height: core.Pt(size), CornerRadius: size * 0.27, BorderCurve: core.BorderCurveContinuous, FlexShrink: 0,
		AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Background: s.mark},
		Children: []core.Node{core.Text{Content: s.letter, MaxLines: 1, Style: core.Style{FontSize: size * 0.4, FontWeight: core.FontWeightBold, Color: core.RGB(0xFFFFFF)}}}}
}

// recent is a video looked at, by its title over its site; pressing it opens it again.
func (b *browser) recent(cx *core.Scope, v func() store.Visit, i func() int) core.Node {
	host := func() string { return displayHost(v().URL) }
	title := func() string {
		if t := v().Title; t != "" {
			return t
		}
		return host()
	}
	var leading core.Node = look.IconTile(cx, lucide.SquarePlay, look.ToneDefault)
	for _, s := range sites {
		if siteOf(v().URL) == siteOf(s.url) {
			leading = mark(s, 28)
			break
		}
	}
	return look.Row{TestID: "browse-recent-" + v().URL, Index: i, Inset: 56, Leading: leading, Title: title, TitleLines: 1, Detail: host,
		OnPress: func() { b.open(v().URL) }}
}

// siteOf is the site an address is on, as kcore.ParseLink names it, else its host.
func siteOf(address string) string {
	l, ok := kcore.ParseLink(address)
	if !ok {
		return ""
	}
	if l.Site != "" {
		return l.Site
	}
	return l.Host
}

// displayHost is the host of address as the address bar shows it while it is not being typed in: without www.
func displayHost(address string) string {
	u, err := url.Parse(address)
	if err != nil {
		return address
	}
	return strings.TrimPrefix(strings.ToLower(u.Host), "www.")
}

// videoBar is the bar over the toolbar on a page of one video: what the video is, which computer it goes to, and the button that sends it there, or pairs a computer for it.
func (b *browser) videoBar(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	to := func() string { return add.Destination(b.s, "") }
	name := func() string { d, _ := b.s.Desktop(to()); return d.Name }
	link := func() string {
		if l, ok := kcore.ParseLink(b.video()); ok {
			return l.URL
		}
		return ""
	}
	sent := func() bool { _ = b.sends.Get(); return b.sent[link()] }
	takes := func() bool { return to() != "" && add.Takes(b.s.Queue(to())) }
	kicker := func() string {
		q := b.s.Queue(to())
		switch {
		case to() == "":
			return locales.BrowseVideoUnpaired(cx)
		case q.Reach == session.Older:
			return locales.JobsOlder(cx)
		case q.Reach == session.Revoked:
			return locales.ComputersUnpaired(cx, name())
		case q.ReadOnly:
			return locales.JobsReadOnly(cx, name())
		case !takes():
			return locales.AddNoLinks(cx, name())
		case sent():
			return locales.BrowseVideoSent(cx, name())
		}
		return locales.BrowseVideo(cx, name())
	}
	title := func() string {
		if t := kcore.VideoTitle(b.video(), b.nav.Get().Title); t != "" {
			return t
		}
		return displayHost(b.video())
	}
	send := func() {
		l := link()
		if l == "" || !takes() || sent() {
			return
		}
		// With no options, the computer's own settings pick the picture and where the lines come from.
		b.s.Send(to(), l, sync.JobOptions{})
		b.sent[l] = true
		b.sends.Set(len(b.sent))
	}
	return core.Show{When: func() bool { return b.video() != "" }, Then: func(cx *core.Scope) core.Node {
		return core.View{TestID: "browse-video", Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10, Padding: core.Edges{Top: 10, Right: 10, Bottom: 10, Left: 14},
			CornerRadius: 18, BorderCurve: core.BorderCurveContinuous},
			StyleFunc: func() core.Style {
				return core.Style{Background: colors.Surface(), Shadows: []core.BoxShadow{ui.Shadow(0, 12, 32, 0, ui.Black(0.18))}}
			},
			Children: []core.Node{
				core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: []core.Node{
					core.Text{TestID: "browse-video-kicker", ContentFunc: kicker, MaxLines: 2, Style: look.Caption, StyleFunc: ui.Colored(colors.Muted)},
					core.Text{TestID: "browse-video-title", ContentFunc: title, MaxLines: 1, Style: look.Subhead.Merge(core.Style{FontWeight: core.FontWeightSemibold}), StyleFunc: ui.Colored(colors.Text)},
				}},
				core.Show{When: func() bool { return to() == "" }, Then: func(cx *core.Scope) core.Node {
					return smallButton(cx, "browse-pair", func() string { return locales.BrowsePair(cx) }, lucide.Link, func() bool { return false }, b.act.Pair)
				}, Else: func(cx *core.Scope) core.Node {
					text := func() string {
						if sent() {
							return locales.BrowseSent(cx)
						}
						return locales.BrowseSend(cx)
					}
					icon := func() *core.SVG {
						if sent() {
							return lucide.Check()
						}
						return lucide.Send()
					}
					return smallButton(cx, "browse-send", text, icon, func() bool { return sent() || !takes() }, send)
				}},
			}}
	}}
}

// smallButton is the video bar's capsule button, filled in the accent, with an icon before its text.
func smallButton(cx *core.Scope, testID string, text func() string, icon func() *core.SVG, disabled func() bool, press func()) core.Node {
	colors := ui.ColorsOf(cx)
	return base.Button{TestID: testID, LabelFunc: text, Disabled: disabled, OnPress: func(core.PressEvent) { press() },
		Style: core.Style{Direction: core.Row, Gap: 6, Height: core.Pt(40), Padding: core.Symmetric(0, 14), CornerRadius: 20, FlexShrink: 0,
			AlignItems: core.AlignCenter, Active: &core.Style{Opacity: core.Alpha(0.7)}, Disabled: &core.Style{Opacity: core.Alpha(0.5)}},
		StyleFunc: func() core.Style { return core.Style{Background: colors.Accent(), Color: colors.OnAccent()} },
		Children: []core.Node{
			ui.Icon(icon, 15, ui.Inherited),
			core.Text{ContentFunc: text, MaxLines: 1, Style: look.Subhead.Merge(core.Style{FontWeight: core.FontWeightSemibold})},
		}}
}

// failure is the line over the toolbar when the page shown did not load, with a way to load it again.
func (b *browser) failure(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	return core.Show{When: func() bool { return b.failed.Get() && b.shown.Get() }, Then: func(cx *core.Scope) core.Node {
		return core.View{TestID: "browse-failed", Role: core.RoleAlert, Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10, Padding: core.Edges{Top: 6, Right: 6, Bottom: 6, Left: 14},
			CornerRadius: 14, BorderCurve: core.BorderCurveContinuous},
			StyleFunc: func() core.Style { return core.Style{Background: ui.TintsOf(cx).Danger()} },
			Children: []core.Node{
				core.Text{ContentFunc: func() string { return locales.BrowseFailed(cx) }, MaxLines: 2, Style: look.Subhead.Merge(core.Style{FlexGrow: 1, FlexShrink: 1}), StyleFunc: ui.Colored(colors.Danger)},
				ui.IconButton{TestID: "browse-reload", Icon: lucide.RotateCw, LabelFunc: func() string { return locales.BrowseReload(cx) }, Variant: ui.IconButtonGhost, Size: 36, IconSize: 18,
					OnPress: func() { b.failed.Set(false); b.web.Reload() }},
			}}
	}}
}

// toolbar is back, the address field and the start, at the page's foot where a thumb reaches them.
func (b *browser) toolbar(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	page := func() string {
		if !b.shown.Get() {
			return ""
		}
		return b.nav.Get().URL
	}
	value := func() string {
		if b.editing.Get() {
			return b.typed.Get()
		}
		return displayHost(page())
	}
	submit := func(text string) {
		text = strings.TrimSpace(text)
		if text == "" {
			return
		}
		b.field.Blur()
		b.editing.Set(false)
		b.open(kcore.AddressTarget(text, langs.DefaultTarget(cx.Env().Locale())))
	}
	secure := func() bool { return strings.HasPrefix(strings.ToLower(page()), "https://") }
	return core.View{TestID: "browse-toolbar", Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 4, Padding: core.All(6), CornerRadius: 20, BorderCurve: core.BorderCurveContinuous},
		StyleFunc: func() core.Style {
			return core.Style{Background: colors.Surface(), Shadows: []core.BoxShadow{ui.Shadow(0, 8, 22, 0, ui.Black(0.1))}}
		},
		Children: []core.Node{
			ui.IconButton{TestID: "browse-back", Icon: lucide.ChevronLeft, LabelFunc: func() string { return locales.BrowseBack(cx) }, Variant: ui.IconButtonGhost, Size: 40, IconSize: 22,
				Disabled: func() bool { return !b.shown.Get() && b.source.Get() == "" }, OnPress: b.back},
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6, Height: core.Pt(40), Padding: core.Symmetric(0, 10),
				CornerRadius: 12, BorderCurve: core.BorderCurveContinuous},
				StyleFunc: func() core.Style { return core.Style{Background: ui.Alpha(colors.Text(), 0.06)} },
				Children: []core.Node{
					core.Show{When: func() bool { return secure() && !b.editing.Get() }, Then: func(cx *core.Scope) core.Node {
						return ui.Icon(lucide.Lock, 12, colors.Muted)
					}},
					// The field is drawn into the pill, without ui.Input's box and rim.
					base.Input{TestID: "browse-address", Ref: b.field, Value: value, OnChange: b.typed.Set, OnSubmit: submit,
						LabelFunc: func() string { return locales.BrowseAddress(cx) }, PlaceholderFunc: func() string { return locales.BrowseAddress(cx) },
						KeyboardType: core.KeyboardURL, ReturnKeyType: core.ReturnKeyGo, AutoCapitalize: core.AutoCapitalizeNone, NoAutoCorrect: true,
						OnFocus: func() { b.typed.Set(page()); b.editing.Set(true) },
						OnBlur:  func() { b.editing.Set(false) },
						Style:   core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Height: core.Pt(40), FontSize: 15},
						StyleFunc: func() core.Style {
							return core.Style{Color: colors.Text(), PlaceholderColor: colors.Muted(), CaretColor: colors.Accent(), SelectionColor: cx.Palette().Selection}
						}},
				}},
			ui.IconButton{TestID: "browse-home", Icon: lucide.LayoutGrid, LabelFunc: func() string { return locales.BrowseHome(cx) }, Variant: ui.IconButtonGhost, Size: 40, IconSize: 20,
				OnPress: b.home},
		}}
}
