package subscribe

import (
	"strconv"
	"strings"

	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"
	"kando/packages/core"

	"github.com/nsgo-dev/nsgo/base"
	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Preview is what the subscribe dialog read of the link typed, which it shows before subscribing, as the design's card a pasted link opens: Ready once the list was read, its name, site and kind, and how many videos it holds.
type Preview struct {
	Ready    bool
	Name     string
	Platform core.Platform
	Kind     Kind
	Videos   int
}

// Channel reports whether the list read is a channel or an uploader's space, of whose videos the dialog asks how many to add now.
func (p Preview) Channel() bool { return p.Kind == KindChannel || p.Kind == KindSpace }

// RecentChoices are how many of a channel's most recent videos the dialog offers to add now, as the design's card has them: none, the latest 3 or the latest 10; RecentDefault is the one it starts at.
var RecentChoices = []int{0, 3, 10}

// RecentDefault is how many of a channel's most recent videos the dialog adds unless the user chooses otherwise, as the design's card starts.
const RecentDefault = 3

// Ask is the dialog a link to subscribe to is pasted in, over the window while Open returns true. First the field, then a spinner while the list is read or why it could not be; 订阅 and Return in the field run OnSubmit, which reads the list. Once it was read (Preview), the dialog shows it as the design's card does: its site's mark, kind and name, then, for a collection or a playlist, that its videos are all added in order with whether they are one series (Series, OnSeries), or, for a channel, how many of its most recent videos are added now (Recent, OnRecent); the button says what it does and runs OnSubmit again, which subscribes. The app closes the dialog once it subscribed. 取消, Escape and closing it run OnCancel, which also stops a reading.
type Ask struct {
	Open    func() bool
	Link    func() string
	OnLink  func(text string)
	Reading func() bool
	// Err says why the link could not be subscribed to, in the interface's language; "" while there is nothing to say.
	Err      func() string
	Preview  func() Preview
	Recent   func() int
	OnRecent func(n int)
	Series   func() bool
	OnSeries func(on bool)
	// Adding is how many videos subscribing adds now.
	Adding   func() int
	OnSubmit func()
	OnCancel func()
}

// Render implements ns.Node.
func (d Ask) Render(cx *ns.Scope) ns.Node {
	return ns.Show{When: d.Open, Then: func(cx *ns.Scope) ns.Node {
		preview := ns.NewMemo(cx, d.Preview)
		// The dialog is built again as it turns from the field to the card, which shows what was read.
		return ns.Dynamic(func(cx *ns.Scope) ns.Node {
			if p := preview.Get(); p.Ready {
				return d.card(cx, p)
			}
			return d.field(cx)
		})
	}}
}

// dialog is the dialog around children, with 取消 and the button that runs OnSubmit at its foot.
func (d Ask) dialog(cx *ns.Scope, title, description, label func() string, children []ns.Node, submit ns.Node) ns.Node {
	return ui.Dialog{
		TestID:                 "subscribe-ask",
		NoCloseOnBackdropPress: true,
		Open:                   func() bool { return true },
		TitleFunc:              title,
		DescriptionFunc:        description,
		LabelFunc:              label,
		Children:               children,
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				d.OnCancel()
			}
		},
		Footer: []ns.Node{ui.Button{Text: locales.SubscribeAddCancel(cx), Variant: ui.ButtonSecondary, TestID: "subscribe-cancel", OnPress: d.OnCancel}, submit},
	}
}

// field is the first step: the link field, and a spinner while the list is read or why it could not be.
func (d Ask) field(cx *ns.Scope) ns.Node {
	colors := ui.ColorsOf(cx)
	field := ui.Input{TestID: "subscribe-link", Label: locales.SubscribeAddLink(cx), PlaceholderFunc: func() string { return locales.SubscribeAddPlaceholder(cx) },
		AutoFocus: true, NoAutoCorrect: true, AutoCapitalize: ns.AutoCapitalizeNone, KeyboardType: ns.KeyboardURL,
		Value: d.Link, OnChange: d.OnLink, OnSubmit: func(string) { d.OnSubmit() },
		Disabled: d.Reading, Style: ns.Style{Width: ns.Pct(100)}}
	status := ns.Dynamic(func(cx *ns.Scope) ns.Node {
		switch {
		case d.Reading():
			return ns.View{Style: ns.Style{Direction: ns.Row, Gap: 8, AlignItems: ns.AlignCenter}, Children: []ns.Node{
				ui.Spinner{Size: 16, TestID: "subscribe-spinner"},
				ns.Text{Content: locales.SubscribeAddReading(cx), Style: ui.TypeBody, StyleFunc: ui.Colored(colors.Muted)},
			}}
		case d.Err() != "":
			return d.errLine(cx)
		}
		return nil
	})
	submit := ui.Button{Text: locales.SubscribeAddSubmit(cx), TestID: "subscribe-submit", OnPress: d.OnSubmit,
		Disabled: func() bool { return d.Reading() || strings.TrimSpace(d.Link()) == "" }}
	title := func() string { return locales.SubscribeAddButton(cx) }
	description := func() string { return locales.SubscribeAddDescription(cx) }
	return d.dialog(cx, title, description, title, []ns.Node{ns.View{Style: ns.Style{Gap: 10, AlignItems: ns.AlignStretch}, Children: []ns.Node{field, status}}}, submit)
}

// errLine says why the link could not be subscribed to.
func (d Ask) errLine(cx *ns.Scope) ns.Node {
	return ns.Text{TestID: "subscribe-error", Role: ns.RoleAlert, ContentFunc: d.Err, Selectable: ns.SelectableOn, Style: ui.TypeBody, StyleFunc: ui.Colored(ui.ColorsOf(cx).Danger)}
}

// card is the second step, the design's card a pasted list opens: the list's mark, site, kind and name over how many videos it holds, then what subscribing adds now, and the button that subscribes.
func (d Ask) card(cx *ns.Scope, p Preview) ns.Node {
	colors := ui.ColorsOf(cx)
	titles := ns.View{Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: ns.Pt(0), Gap: 3}, Children: []ns.Node{
		ns.View{Style: ns.Style{Direction: ns.Row, Gap: 6, AlignItems: ns.AlignCenter}, Children: []ns.Node{
			SiteBadge(siteName(cx, p.Platform), p.Platform),
			ns.Text{Content: kindName(cx, p.Kind), MaxLines: 1, Style: ns.Style{FontSize: 11, LineHeight: 16}, StyleFunc: ui.Colored(colors.Muted)},
		}},
		ns.Text{TestID: "subscribe-name", Content: p.Name, Role: ns.RoleHeader, MaxLines: 1, Style: ns.Style{FontSize: 15, LineHeight: 20, FontWeight: ui.Bold}, StyleFunc: ui.Colored(colors.Text)},
		ns.Text{TestID: "subscribe-count", Content: locales.SubscribeAddVideos(cx, p.Videos), MaxLines: 1, Style: ns.Style{FontSize: 12, LineHeight: 17}, StyleFunc: ui.Colored(colors.Muted)},
	}}
	head := ns.View{Style: ns.Style{Direction: ns.Row, Gap: 14, AlignItems: ns.AlignCenter}, Children: []ns.Node{Thumb(p.Platform, 96, 54), titles}}
	var rows []ns.Node
	if p.Channel() {
		options := make([]ui.Option[int], 0, len(RecentChoices))
		for _, n := range RecentChoices {
			label := locales.PlaylistRecentNone(cx)
			if n > 0 {
				label = locales.PlaylistRecentLatest(cx, n)
			}
			options = append(options, ui.Option[int]{Value: n, Label: label, TestID: "subscribe-recent-" + strconv.Itoa(n)})
		}
		rows = append(rows, form.Row{Title: locales.SubscribeAddRecent(cx), Subtitle: locales.SubscribeAddRecentHint(cx), NoGroup: true, TestID: "subscribe-recent-row",
			Trailing: []ns.Node{ui.Tabs[int]{TestID: "subscribe-recent", Label: locales.SubscribeAddRecent(cx), Size: ui.SizeSmall, Options: options, Value: d.Recent, OnChange: d.OnRecent}}})
	} else {
		rows = append(rows,
			form.Row{Title: wholeTitle(cx, p), Subtitle: locales.SubscribeAddWholeHint(cx), TestID: "subscribe-whole-row"},
			form.Separator(true),
			form.SwitchRow{TestID: "subscribe-series", Title: locales.SubscribeSeries(cx), Subtitle: locales.SubscribeSeriesNote(cx), On: d.Series, OnChange: d.OnSeries})
	}
	children := []ns.Node{head, form.Card(rows...),
		ns.Show{When: func() bool { return d.Err() != "" }, Then: func(cx *ns.Scope) ns.Node { return d.errLine(cx) }}}
	adding := ns.NewMemo(cx, d.Adding)
	// TODO(nsgo#145): give ui.Button a TextFunc twin, so its label follows the number without building the button again https://github.com/nsgo-dev/nsgo/issues/145
	submit := ns.Dynamic(func(cx *ns.Scope) ns.Node {
		text := locales.SubscribeAddConfirmList(cx, adding.Get())
		if p.Channel() {
			text = locales.SubscribeAddConfirmChannel(cx, adding.Get())
		}
		return ui.Button{Text: text, TestID: "subscribe-submit", OnPress: d.OnSubmit, Disabled: d.Reading}
	})
	return d.dialog(cx, nil, nil, func() string { return p.Name }, []ns.Node{ns.View{Style: ns.Style{Gap: 12, AlignItems: ns.AlignStretch}, Children: children}}, submit)
}

// wholeTitle says that the collection or playlist p holds its count of videos, all of which subscribing adds.
func wholeTitle(cx *ns.Scope, p Preview) string {
	if p.Kind == KindCollection {
		return locales.SubscribeAddWholeCollection(cx, p.Videos)
	}
	return locales.SubscribeAddWholePlaylist(cx, p.Videos)
}
