package subscribe

import (
	"reflect"
	"strconv"
	"strings"

	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"
	"kando/packages/core"

	"github.com/nsgo-dev/nsgo/base"
	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Preview is what the subscribe dialog read of the link typed, which it shows before subscribing, as the design's card a pasted link opens: Ready once the list was read, its name, site and kind, and how many videos it holds. Episode reports that the link typed named one video of the list, which 只要这一集 adds alone. Browsers are those whose sign-in the downloads can borrow, besides none, and Streams whether this computer streams a film that keeps no video (media.StreamsHere), which the line under 仅字幕 says.
type Preview struct {
	Ready    bool
	Name     string
	Platform core.Platform
	Kind     Kind
	Videos   int
	Episode  bool
	Browsers []Browser
	Streams  bool
}

// Browser is a browser the downloads can borrow the sign-in of: its --cookies-from-browser ID and the name people know it by.
type Browser struct {
	ID, Name string
}

// Channel reports whether the list read is a channel or an uploader's space, of whose videos the dialog asks how many to add now.
func (p Preview) Channel() bool { return p.Kind == KindChannel || p.Kind == KindSpace }

// siteSubtitles reports whether the list's site has subtitles a film can take as its original lines or keep as a reference layer, and chapters it can be cut at: YouTube and bilibili.
func (p Preview) siteSubtitles() bool {
	return p.Platform == core.PlatformYouTube || p.Platform == core.PlatformBilibili
}

// Choice is what the user chose on the dialog's card: Subscribe follows the list, or, off, only adds its videos now; Recent is how many of a channel's most recent videos are added now (RecentChoices), and Only adds only the one video the link named rather than all of a collection's or a playlist's (Preview.Episode); Series makes their films one series. Quality is the picture they download, Source where their original lines come from, Browser the ID of the browser whose sign-in the downloads borrow, "" for none, and Chapters cuts them at their site's chapters. The subscription keeps the series, the picture and the source of lines as its rules; the sign-in and the chapters go only on the videos added now.
type Choice struct {
	Subscribe bool
	Recent    int
	Only      bool
	Series    bool
	Quality   core.Quality
	Source    core.SubtitleSource
	Browser   string
	Chapters  bool
}

// RecentChoices are how many of a channel's most recent videos the dialog offers to add now, as the design's card has them: none, the latest 3 or the latest 10; RecentDefault is the one it starts at.
var RecentChoices = []int{0, 3, 10}

// RecentDefault is how many of a channel's most recent videos the dialog adds unless the user chooses otherwise, as the design's card starts.
const RecentDefault = 3

// Ask is the dialog a link to subscribe to is pasted in, over the window while Open returns true. First the field, then a spinner while the list is read or why it could not be; 订阅 and Return in the field run OnSubmit, which reads the list. Once it was read (Preview), the dialog shows it as the design's card does: its site's mark, kind and name; for a collection or a playlist, that its videos are all added in order, or only the one the link named, and whether they are one series, then the switch that subscribes to it; for a channel, the switch, then how many of its most recent videos are added now; where the original lines come from, the picture, the sign-in and the chapters (Choice, OnChoice), and that the downloads and the work are done on this computer. The button says what it does and runs OnSubmit again, which subscribes, or only adds the videos while the switch is off. The app closes the dialog once it did. 取消, Escape and closing it run OnCancel, which also stops a reading.
type Ask struct {
	Open    func() bool
	Link    func() string
	OnLink  func(text string)
	Reading func() bool
	// Err says why the link could not be subscribed to, in the interface's language; "" while there is nothing to say.
	Err      func() string
	Preview  func() Preview
	Choice   func() Choice
	OnChoice func(Choice)
	// Adding is how many videos the button adds now.
	Adding   func() int
	OnSubmit func()
	OnCancel func()
}

// Render implements ns.Node.
func (d Ask) Render(cx *ns.Scope) ns.Node {
	return ns.Show{When: d.Open, Then: func(cx *ns.Scope) ns.Node {
		// Preview holds the browsers, a slice, so it is compared as a whole.
		preview := ns.NewMemoFunc(cx, d.Preview, func(a, b Preview) bool { return reflect.DeepEqual(a, b) })
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
func (d Ask) dialog(cx *ns.Scope, title, description, label func() string, children []ns.Node, submit ns.Node, width ns.Points) ns.Node {
	cancel := pill{Text: func() string { return locales.SubscribeAddCancel(cx) }, Look: LookSecondary, Large: true, TestID: "subscribe-cancel", OnPress: d.OnCancel}
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
		Footer: []ns.Node{cancel, submit},
		Style:  ns.Style{Width: ns.Pt(width)},
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
	submit := pill{Text: func() string { return locales.SubscribeAddSubmit(cx) }, Look: LookPrimary, Large: true, TestID: "subscribe-submit", OnPress: d.OnSubmit,
		Disabled: func() bool { return d.Reading() || strings.TrimSpace(d.Link()) == "" }}
	title := func() string { return locales.SubscribeAddButton(cx) }
	description := func() string { return locales.SubscribeAddDescription(cx) }
	return d.dialog(cx, title, description, title, []ns.Node{ns.View{Style: ns.Style{Gap: 10, AlignItems: ns.AlignStretch}, Children: []ns.Node{field, status}}}, submit, 460)
}

// errLine says why the link could not be subscribed to.
func (d Ask) errLine(cx *ns.Scope) ns.Node {
	return ns.Text{TestID: "subscribe-error", Role: ns.RoleAlert, ContentFunc: d.Err, Selectable: ns.SelectableOn, Style: ui.TypeBody, StyleFunc: ui.Colored(ui.ColorsOf(cx).Danger)}
}

// pick changes the choice with change and hands it to the app.
func (d Ask) pick(change func(*Choice)) {
	c := d.Choice()
	change(&c)
	d.OnChoice(c)
}

// card is the second step, the design's card a pasted list opens: the list's mark, site, kind, name and how many videos it holds, then what is added now and the switch that subscribes, where the original lines come from, the picture, the sign-in and the chapters, a note on where the work is done, and the button that does it.
func (d Ask) card(cx *ns.Scope, p Preview) ns.Node {
	colors := ui.ColorsOf(cx)
	children := []ns.Node{d.head(cx, p)}
	follow := form.SwitchRow{TestID: "subscribe-follow", Title: locales.SubscribeAddFollow(cx, string(p.Kind)), Subtitle: locales.SubscribeAddFollowHint(cx),
		On: func() bool { return d.Choice().Subscribe }, OnChange: func(on bool) { d.pick(func(c *Choice) { c.Subscribe = on }) }}
	if p.Channel() {
		children = append(children, form.Card(follow, form.Separator(true), d.recentRow(cx)))
	} else {
		children = append(children, form.Card(d.wholeRow(cx, p), form.Separator(true), d.seriesRow(cx)), form.Card(follow))
	}
	if p.siteSubtitles() {
		children = append(children, form.Label(locales.SubscribeAddSource(cx), 4), form.Card(d.sourceRadios(cx, p)))
	}
	keep := []ns.Node{d.qualityRow(cx, p)}
	if p.Platform == core.PlatformBilibili {
		keep = append(keep, form.Separator(true), d.signInRow(cx, p))
	}
	if p.siteSubtitles() && !p.Channel() {
		keep = append(keep, form.Separator(true), form.SwitchRow{TestID: "subscribe-chapters", Title: locales.SettingsGeneralChapters(cx), Subtitle: locales.SettingsGeneralChaptersHint(cx),
			On: func() bool { return d.Choice().Chapters }, OnChange: func(on bool) { d.pick(func(c *Choice) { c.Chapters = on }) }})
	}
	children = append(children, form.Card(keep...),
		ns.Text{TestID: "subscribe-footnote", Content: locales.SubscribeAddFootnote(cx), Style: ns.Style{FontSize: 12, LineHeight: 17, Padding: ns.Symmetric(0, 4)}, StyleFunc: ui.Colored(colors.Muted)},
		ns.Show{When: func() bool { return d.Err() != "" }, Then: func(cx *ns.Scope) ns.Node { return d.errLine(cx) }})
	adding := ns.NewMemo(cx, d.Adding)
	following := ns.NewMemo(cx, func() bool { return d.Choice().Subscribe })
	submit := pill{TestID: "subscribe-submit", Look: LookPrimary, Large: true, OnPress: d.OnSubmit,
		Text: func() string {
			n := adding.Get()
			switch {
			case !following.Get():
				return locales.SubscribeAddJustAdd(cx, string(p.Kind), n)
			case p.Channel():
				return locales.SubscribeAddConfirmChannel(cx, n)
			}
			return locales.SubscribeAddConfirmList(cx, n)
		},
		// Without the switch there is nothing to do but add, which needs a video to add.
		Disabled: func() bool { return d.Reading() || (!following.Get() && adding.Get() == 0) }}
	// The card can be taller than a short window; its body scrolls inside the dialog so 取消 and the button stay on screen.
	body := ns.Scroll{TestID: "subscribe-card-scroll", Style: ns.Style{FlexShrink: 1, MinHeight: ns.Pt(0)}, Children: []ns.Node{
		ns.View{Style: ns.Style{Gap: 10, AlignItems: ns.AlignStretch}, Children: children}}}
	return d.dialog(cx, nil, nil, func() string { return p.Name }, []ns.Node{body}, submit, 560)
}

// head is the card's top: the site's mark as a picture of the list, the site's tag and the kind, the list's name, and how many videos it holds, episodes for a collection.
func (d Ask) head(cx *ns.Scope, p Preview) ns.Node {
	colors := ui.ColorsOf(cx)
	count := locales.SubscribeAddVideos(cx, p.Videos)
	if p.Kind == KindCollection {
		count = locales.SubscribeAddEpisodes(cx, p.Videos)
	}
	titles := ns.View{Style: ns.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: ns.Pt(0), Gap: 3}, Children: []ns.Node{
		ns.View{Style: ns.Style{Direction: ns.Row, Gap: 6, AlignItems: ns.AlignCenter}, Children: []ns.Node{
			SiteBadge(siteName(cx, p.Platform), p.Platform),
			ns.Text{Content: kindName(cx, p.Kind), MaxLines: 1, Style: ns.Style{FontSize: 12, LineHeight: 17}, StyleFunc: ui.Colored(colors.Muted)},
		}},
		ns.Text{TestID: "subscribe-name", Content: p.Name, Role: ns.RoleHeader, MaxLines: 2, Style: ns.Style{FontSize: 17, LineHeight: 22, FontWeight: ui.Bold}, StyleFunc: ui.Colored(colors.Text)},
		ns.Text{TestID: "subscribe-count", Content: count, MaxLines: 1, Style: ns.Style{FontSize: 13, LineHeight: 18}, StyleFunc: ui.Colored(colors.Muted)},
	}}
	return ns.View{Style: ns.Style{Direction: ns.Row, Gap: 16, AlignItems: ns.AlignCenter}, Children: []ns.Node{Thumb(p.Platform, 104, 58), titles}}
}

// wholeRow says that the collection or playlist holds its count of videos, which are queued in order, with, when the link named one of them, the choice of all of them or only that one.
func (d Ask) wholeRow(cx *ns.Scope, p Preview) ns.Node {
	title := locales.SubscribeAddWholePlaylist(cx, p.Videos)
	if p.Kind == KindCollection {
		title = locales.SubscribeAddWholeCollection(cx, p.Videos)
	}
	row := form.Row{Title: title, Subtitle: locales.SubscribeAddWholeHint(cx), TestID: "subscribe-whole-row"}
	if p.Episode {
		row.NoGroup = true
		row.Trailing = []ns.Node{ui.Tabs[bool]{TestID: "subscribe-only", Label: title, Size: ui.SizeSmall,
			Options: []ui.Option[bool]{
				{Value: false, Label: locales.SubscribeAddAllEpisodes(cx, string(p.Kind), p.Videos), TestID: "subscribe-only-all"},
				{Value: true, Label: locales.SubscribeAddOnlyThis(cx), TestID: "subscribe-only-this"},
			},
			Value: func() bool { return d.Choice().Only }, OnChange: func(only bool) { d.pick(func(c *Choice) { c.Only = only }) }}}
	}
	return row
}

// seriesRow is 作为一个系列 as the design's card draws it, a checkbox before its title and its line.
func (d Ask) seriesRow(cx *ns.Scope) ns.Node {
	colors := ui.ColorsOf(cx)
	texts := ns.View{Style: ns.Style{Gap: 1, FlexShrink: 1}, Children: []ns.Node{
		ns.Text{Content: locales.SubscribeSeries(cx), Style: ns.Style{FontSize: 13, LineHeight: 18}, StyleFunc: ui.Colored(colors.Text)},
		ns.Text{Content: locales.SubscribeAddSeriesNote(cx), Style: ns.Style{FontSize: 11, LineHeight: 15}, StyleFunc: ui.Colored(colors.Muted)},
	}}
	return ui.Checkbox{TestID: "subscribe-series", Label: locales.SubscribeSeries(cx), Hint: locales.SubscribeAddSeriesNote(cx), Children: []ns.Node{texts},
		Checked: func() bool { return d.Choice().Series }, OnChange: func(on bool) { d.pick(func(c *Choice) { c.Series = on }) },
		Style: ns.Style{Padding: ns.Symmetric(10, 14)}}
}

// recentRow is 现在先加入: how many of the channel's most recent videos are added now, none, the latest 3 or the latest 10 (RecentChoices).
func (d Ask) recentRow(cx *ns.Scope) ns.Node {
	options := make([]ui.Option[int], 0, len(RecentChoices))
	for _, n := range RecentChoices {
		label := locales.SubscribeAddRecentNone(cx)
		if n > 0 {
			label = locales.SubscribeAddRecentLatest(cx, n)
		}
		options = append(options, ui.Option[int]{Value: n, Label: label, TestID: "subscribe-recent-" + strconv.Itoa(n)})
	}
	return form.Row{Title: locales.SubscribeAddRecent(cx), Subtitle: locales.SubscribeAddRecentHint(cx), NoGroup: true, TestID: "subscribe-recent-row",
		Trailing: []ns.Node{ui.Tabs[int]{TestID: "subscribe-recent", Label: locales.SubscribeAddRecent(cx), Size: ui.SizeSmall, Options: options,
			Value: func() int { return d.Choice().Recent }, OnChange: func(n int) { d.pick(func(c *Choice) { c.Recent = n }) }}}}
}

// sourceRadios is 原文来源: the uploader's subtitles, the site's own captions as a reference layer (bilibili's AI subtitles), or the speech recognized on this computer, the one Kando recommends for the site tagged: the uploader's on YouTube, where they are common, and this computer's on bilibili, where they are rare.
func (d Ask) sourceRadios(cx *ns.Scope, p Preview) ns.Node {
	colors := ui.ColorsOf(cx)
	source := func() core.SubtitleSource { return d.Choice().Source }
	recommended := core.SubtitlesPlatform
	auto := locales.AddSheetSourceAuto(cx)
	if p.Platform == core.PlatformBilibili {
		recommended, auto = core.SubtitlesLocal, locales.AddSheetSourceAutoBilibili(cx)
	}
	option := func(value core.SubtitleSource, title, hint, id string) ns.Node {
		line := []ns.Node{ns.Text{Content: title, MaxLines: 1, Style: ns.Style{FontSize: 13, LineHeight: 18, FontWeight: ui.Semibold}, StyleFunc: ui.Colored(colors.Text)}}
		if value == recommended {
			line = append(line, ns.Text{Content: locales.SettingsSpeechRecommended(cx), MaxLines: 1, Role: ns.RoleNone,
				Style:     ns.Style{FontSize: 10, LineHeight: 16, FontWeight: ui.Bold, Padding: ns.Symmetric(0, 6), CornerRadius: 6, FlexShrink: 0},
				StyleFunc: func() ns.Style { return ns.Style{Background: colors.Accent().Faded(0.12), Color: colors.Accent()} }})
		}
		texts := ns.View{Style: ns.Style{Gap: 1, FlexShrink: 1}, Children: []ns.Node{
			ns.View{Style: ns.Style{Direction: ns.Row, Gap: 6, AlignItems: ns.AlignCenter}, Children: line},
			ns.Text{Content: hint, Style: ns.Style{FontSize: 11, LineHeight: 15}, StyleFunc: ui.Colored(colors.Muted)},
		}}
		return ui.Radio{TestID: id, Label: title, Hint: hint, Children: []ns.Node{texts}, Style: ns.Style{Padding: ns.Symmetric(5, 14)},
			Checked: func() bool { return source() == value }, OnChange: func(bool) { d.pick(func(c *Choice) { c.Source = value }) }}
	}
	return ui.RadioGroup{Label: locales.SubscribeAddSource(cx), TestID: "subscribe-source", Style: ns.Style{Padding: ns.Symmetric(2, 0)}, Children: []ns.Node{
		option(core.SubtitlesPlatform, locales.AddSheetSourcePlatform(cx), locales.AddSheetSourcePlatformHint(cx), "subscribe-source-platform"),
		option(core.SubtitlesAuto, auto, locales.AddSheetSourceAutoHint(cx), "subscribe-source-auto"),
		option(core.SubtitlesLocal, locales.AddSheetSourceLocal(cx), locales.SubscribeAddSourceLocalHint(cx), "subscribe-source-local"),
	}}
}

// qualityRow is 保存: the picture the downloads ask for, and what keeping it means.
func (d Ask) qualityRow(cx *ns.Scope, p Preview) ns.Node {
	return form.Row{Title: locales.SubscribeAddSave(cx), NoGroup: true, TestID: "subscribe-quality-row",
		SubtitleFunc: func() string {
			switch {
			case d.Choice().Quality != core.QualitySubtitles:
				return locales.SubscribeAddSaveHint(cx)
			case p.Streams:
				return locales.SettingsGeneralQualitySubtitlesHint(cx)
			}
			return locales.SettingsGeneralQualitySubtitlesHintNoStream(cx)
		},
		Trailing: []ns.Node{ui.Tabs[core.Quality]{TestID: "subscribe-quality", Label: locales.SubscribeAddSave(cx), Size: ui.SizeSmall, Options: qualityOptions(cx),
			Value: func() core.Quality { return d.Choice().Quality }, OnChange: func(q core.Quality) { d.pick(func(c *Choice) { c.Quality = q }) }}}}
}

// signInRow is the bilibili sign-in the downloads borrow: a browser's, or none.
func (d Ask) signInRow(cx *ns.Scope, p Preview) ns.Node {
	browsers := []ui.Option[string]{{Value: "", Label: locales.AddSheetSignInNone(cx)}}
	for _, b := range p.Browsers {
		browsers = append(browsers, ui.Option[string]{Value: b.ID, Label: locales.AddSheetSignInBrowser(cx, b.Name)})
	}
	title := locales.SubscribeAddSignIn(cx, siteName(cx, p.Platform))
	return form.Row{Title: title, Subtitle: locales.SubscribeAddSignInHint(cx), NoGroup: true,
		Trailing: []ns.Node{ui.Select[string]{Options: browsers, Label: title, TestID: "subscribe-signin",
			Value: func() string { return d.Choice().Browser }, OnChange: func(id string) { d.pick(func(c *Choice) { c.Browser = id }) },
			Style: ns.Style{Width: ns.Pt(170)}}}}
}
