// Package addsheet is the Add sheet: the card a link to one online video opens before its job is made, as the prototype's 添加在线视频 does. It names the video and its site, says where the film continues from when the link came from a browser that was playing it, warns of a site whose videos are DRM-protected or one Kando does not know, says what it read of the video's page since it opened (Meta: who put it on the site, its length, how many videos its playlist holds, whether it has the uploader's subtitles), and lets the user choose, for this link alone, whether to add the whole playlist it is in and treat it as a series, where the original lines come from (the uploader's subtitles, the speech recognition, or the speech recognition with the site's own captions kept as a reference layer), the picture the download asks for, the browser whose sign-in it borrows and whether the film is cut at its site's chapters. The library's link field opens it, and so do the browser extension and the bookmarklet unless Settings › 通用 › 浏览器扩展 says to start at once. The app keeps its state and makes the job (state_addsheet.go).
package addsheet

import (
	"fmt"
	"reflect"
	"strings"
	"time"

	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"
	"kando/apps/kando/theme"
	kcore "kando/packages/core"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// From is where the link the sheet opened for came from, which decides what it says of where the film continues from.
type From uint8

const (
	// FromField is a link pasted in the library's link field.
	FromField From = iota
	// FromBrowser is a link the browser extension or the bookmarklet sent from the page that plays it.
	FromBrowser
)

// Browser is a browser the download can borrow the sign-in of: its --cookies-from-browser ID and the name people know it by.
type Browser struct {
	ID, Name string
}

// Look is what the sheet shows of the link, which stays as it is while the sheet is open.
type Look struct {
	// Site is the name of the link's site in the interface's language, "" for a site Kando does not name; Host is the link's host, which names it then.
	Site, Host string
	// Title is the video's title as the page that sent it names it, "" for a pasted link.
	Title string
	// Link is the link, shown under the title.
	Link string
	From From
	// ResumeAt is where the film continues from once it is done: where the browser was, or where the link starts (core.StartAt); 0 for neither.
	ResumeAt time.Duration
	// Unknown reports a site that is neither one Kando knows nor in yt-dlp's list, whose generic extractor tries it; DRM a site whose videos are DRM-protected, which nothing is added for.
	Unknown bool
	DRM     bool
	// InList reports a video of a playlist whose whole list can be added instead (core.Link.WholeList).
	InList bool
	// SiteSubtitles reports a site whose uploaders' subtitles a film can take as its original lines, or whose own captions it can keep as a reference layer, and Chapters one whose videos mark chapters: YouTube and bilibili.
	SiteSubtitles bool
	Chapters      bool
	// Bilibili reports a bilibili video, whose site calls its own captions AI subtitles and serves 480p at best to a viewer not signed in.
	Bilibili bool
	// Browsers are those whose sign-in the download can borrow, besides none.
	Browsers []Browser
	// Streams reports whether this computer streams a film that keeps no video (media.StreamsHere), which the line under 仅字幕 says.
	Streams bool
}

// Name is how the sheet names the video: its title, or what it is on its site.
func (l Look) Name(m locales.Messages) string {
	switch {
	case l.Title != "":
		return l.Title
	case l.Site != "":
		return m.AddSheetUntitled(l.Site)
	}
	return m.AddSheetUntitled(l.Host)
}

// Meta is what the sheet learns of the video by reading its page once it is open (media.Previewer): Reading while it reads it, then, once Known, its title, who put it on the site, how long it plays and which subtitles its site has. Episodes is how many videos the playlist it is in holds (Look.InList), 0 until they are counted.
type Meta struct {
	Reading         bool
	Known           bool
	Title, Uploader string
	Length          time.Duration
	Episodes        int
	// Subtitles reports that the uploader put subtitles on it, AutoCaptions that the site made captions of its own.
	Subtitles, AutoCaptions bool
}

// Choice is what the user chose on the sheet for this link.
type Choice struct {
	// Whole adds the playlist the video is in rather than the video (Look.InList), and Series has its films share a glossary as one series.
	Whole   bool
	Series  bool
	Quality kcore.Quality
	Source  kcore.SubtitleSource
	// Browser is the ID of the browser whose sign-in the download borrows, "" for none.
	Browser string
	// Chapters cuts the film at its site's chapters (Look.Chapters).
	Chapters bool
}

// Sheet is the dialog over the window while Open returns true. Look is what it shows of the link, and Meta what it learned of the video since it opened, nil for nothing; Choice and OnChoice are the user's choices, kept by the app. The primary button and Return run OnAdd, but for a DRM-protected site, whose only button closes the sheet; Cancel, Escape and closing it run OnCancel.
type Sheet struct {
	Open     func() bool
	Look     func() Look
	Meta     func() Meta
	Choice   func() Choice
	OnChoice func(Choice)
	OnAdd    func()
	OnCancel func()
}

// Render implements core.Node.
func (s Sheet) Render(cx *core.Scope) core.Node {
	// Look holds the browsers, a slice, so it is compared as a whole.
	look := core.NewMemoFunc(cx, s.Look, func(a, b Look) bool { return reflect.DeepEqual(a, b) })
	cancel := func() {
		if s.OnCancel != nil {
			s.OnCancel()
		}
	}
	if s.Meta == nil {
		s.Meta = func() Meta { return Meta{} }
	}
	return core.Show{When: s.Open, Then: func(cx *core.Scope) core.Node {
		// The look stays while the sheet is open; it is built again for the next link, and as the interface's language changes.
		return core.Dynamic(func(cx *core.Scope) core.Node {
			l := look.Get()
			m := locales.For(cx)
			footer := []core.Node{ui.Button{Text: m.AddSheetCancel(), Variant: ui.ButtonSecondary, TestID: "addsheet-cancel", OnPress: cancel}}
			confirm := s.OnAdd
			if l.DRM {
				confirm = cancel
				footer = []core.Node{ui.Button{Text: m.AddSheetOK(), TestID: "addsheet-ok", OnPress: cancel}}
			} else {
				footer = append(footer, addButton(cx, s))
			}
			return ui.Dialog{
				TestID:                 "addsheet",
				NoCloseOnBackdropPress: true,
				Open:                   func() bool { return true },
				Label:                  m.AddSheetLabel(),
				OnConfirm:              confirm,
				Children:               []core.Node{body(cx, s, l)},
				OnOpenChange: func(open bool, _ base.Reason) {
					if !open {
						cancel()
					}
				},
				Footer: footer,
				Style:  core.Style{Width: core.Pt(460)},
			}
		})
	}}
}

// addButton is the sheet's primary button: 添加并开始, or 列出视频 while the whole playlist is chosen, whose videos the playlist dialog lists before they are added.
func addButton(cx *core.Scope, s Sheet) core.Node {
	whole := core.NewMemo(cx, func() bool { return s.Choice().Whole })
	return ui.Button{TestID: "addsheet-add", OnPress: s.OnAdd, TextFunc: func() string {
		if whole.Get() {
			return locales.AddSheetListVideos(cx)
		}
		return locales.AddSheetAdd(cx)
	}}
}

// body is the sheet's content for l: the video, a note on its site, then the choices, those that do not apply to its site left out.
func body(cx *core.Scope, s Sheet, l Look) core.Node {
	m := locales.For(cx)
	children := []core.Node{header(cx, l, s.Meta)}
	if l.ResumeAt > 0 && !l.DRM {
		text := m.AddSheetResumeLink(clock(l.ResumeAt))
		if l.From == FromBrowser {
			text = m.AddSheetResumeBrowser(clock(l.ResumeAt))
		}
		children = append(children, note(cx, "addsheet-resume", "", text, false))
	}
	switch {
	case l.DRM:
		children = append(children, note(cx, "addsheet-drm", m.AddSheetDrm(l.Site), m.AddSheetDrmWhy(), true))
		return column(children...)
	case l.Unknown:
		children = append(children, note(cx, "addsheet-unknown", m.AddSheetUnknown(l.Host), m.AddSheetUnknownWhy(), false))
	}
	pick := func(change func(*Choice)) {
		c := s.Choice()
		change(&c)
		s.OnChoice(c)
	}
	if l.InList {
		episodes := ui.RadioGroup{Label: m.AddSheetEpisodes(), TestID: "addsheet-episodes", Style: core.Style{Padding: core.Symmetric(10, 14)}, Children: []core.Node{
			ui.Radio{Text: m.AddSheetOnlyThis(), TestID: "addsheet-only-this", Checked: func() bool { return !s.Choice().Whole }, OnChange: func(bool) { pick(func(c *Choice) { c.Whole = false }) }},
			ui.Radio{Text: m.AddSheetWholeList(), TestID: "addsheet-whole-list", Checked: func() bool { return s.Choice().Whole }, OnChange: func(bool) { pick(func(c *Choice) { c.Whole = true }) }},
		}}
		// 作为剧集处理 goes with the whole playlist, whose films then share a glossary, as the playlist dialog that lists them says again.
		series := core.Show{When: func() bool { return s.Choice().Whole }, Then: func(cx *core.Scope) core.Node {
			return core.View{Children: []core.Node{form.Separator(true), form.SwitchRow{Title: m.AddSheetSeries(), Subtitle: m.AddSheetSeriesHint(), TestID: "addsheet-series",
				On: func() bool { return s.Choice().Series }, OnChange: func(on bool) { pick(func(c *Choice) { c.Series = on }) }}}}
		}}
		children = append(children, form.Card(episodes, series))
	}
	var rows []core.Node
	add := func(row core.Node) {
		if len(rows) > 0 {
			rows = append(rows, form.Separator(true))
		}
		rows = append(rows, row)
	}
	if l.SiteSubtitles {
		// What the sheet read of the video says which of the site's subtitles it has; until then, and when it could not be read, each may be chosen, as a site without them leaves the film heard alone.
		lacks := func(has func(Meta) bool) func() bool {
			return func() bool { meta := s.Meta(); return meta.Known && !has(meta) }
		}
		auto, autoHint := m.AddSheetSourceAuto(), m.AddSheetSourceAutoHint()
		if l.Bilibili {
			auto = m.AddSheetSourceAutoBilibili()
		}
		source := func() kcore.SubtitleSource { return s.Choice().Source }
		add(ui.RadioGroup{Label: m.AddSheetSource(), TestID: "addsheet-source", Style: core.Style{Padding: core.Symmetric(10, 14)}, Children: []core.Node{
			ui.Radio{Text: m.AddSheetSourcePlatform(), Hint: m.AddSheetSourcePlatformHint(), TestID: "addsheet-source-platform",
				Checked:  func() bool { return source() != kcore.SubtitlesLocal && source() != kcore.SubtitlesAuto },
				Disabled: lacks(func(meta Meta) bool { return meta.Subtitles }),
				OnChange: func(bool) { pick(func(c *Choice) { c.Source = kcore.SubtitlesPlatform }) }},
			ui.Radio{Text: auto, Hint: autoHint, TestID: "addsheet-source-auto",
				Checked:  func() bool { return source() == kcore.SubtitlesAuto },
				Disabled: lacks(func(meta Meta) bool { return meta.AutoCaptions }),
				OnChange: func(bool) { pick(func(c *Choice) { c.Source = kcore.SubtitlesAuto }) }},
			ui.Radio{Text: m.AddSheetSourceLocal(), TestID: "addsheet-source-local",
				Checked: func() bool { return source() == kcore.SubtitlesLocal }, OnChange: func(bool) { pick(func(c *Choice) { c.Source = kcore.SubtitlesLocal }) }},
		}})
	}
	add(form.Row{Title: m.AddSheetQuality(), NoGroup: true, TestID: "addsheet-quality-row",
		SubtitleFunc: func() string {
			// 仅字幕 keeps no video, which the user is told before the download goes without it, as Settings › 通用 › 在线视频 says it.
			switch {
			case s.Choice().Quality != kcore.QualitySubtitles:
				return ""
			case l.Streams:
				return locales.SettingsGeneralQualitySubtitlesHint(cx)
			}
			return locales.SettingsGeneralQualitySubtitlesHintNoStream(cx)
		},
		Trailing: []core.Node{qualitySelect(cx, s, l, pick)}})
	browsers := []ui.Option[string]{{Value: "", Label: m.AddSheetSignInNone()}}
	for _, b := range l.Browsers {
		browsers = append(browsers, ui.Option[string]{Value: b.ID, Label: m.AddSheetSignInBrowser(b.Name)})
	}
	add(form.Row{Title: m.AddSheetSignIn(), Subtitle: m.AddSheetSignInHint(), NoGroup: true,
		Trailing: []core.Node{ui.Select[string]{Options: browsers, Label: m.AddSheetSignIn(), TestID: "addsheet-signin",
			Value: func() string { return s.Choice().Browser }, OnChange: func(id string) { pick(func(c *Choice) { c.Browser = id }) },
			Style: core.Style{Width: core.Pt(150)}}}})
	if l.Chapters {
		add(form.SwitchRow{Title: m.SettingsGeneralChapters(), Subtitle: m.SettingsGeneralChaptersHint(), TestID: "addsheet-chapters",
			On: func() bool { return s.Choice().Chapters }, OnChange: func(on bool) { pick(func(c *Choice) { c.Chapters = on }) }})
	}
	children = append(children, core.Show{When: func() bool { return !s.Choice().Whole }, Then: func(*core.Scope) core.Node { return form.Card(rows...) }})
	return column(children...)
}

// qualitySelect is the picture the download asks for. On bilibili without a sign-in the best is 480p, which the site serves at best to a viewer not signed in, so the choice says so; it is built again as the sign-in changes.
func qualitySelect(cx *core.Scope, s Sheet, l Look, pick func(func(*Choice))) core.Node {
	signedOut := core.NewMemo(cx, func() bool { return l.Bilibili && s.Choice().Browser == "" })
	return core.Dynamic(func(cx *core.Scope) core.Node {
		best := locales.SettingsGeneralQualityBest(cx)
		if signedOut.Get() {
			best = locales.AddSheetQualityBest480(cx)
		}
		options := []ui.Option[kcore.Quality]{
			{Value: kcore.QualityBest, Label: best},
			{Value: kcore.Quality720, Label: locales.SettingsGeneralQuality720(cx)},
			{Value: kcore.QualitySubtitles, Label: locales.SettingsGeneralQualitySubtitles(cx)},
		}
		return ui.Select[kcore.Quality]{Options: options, Label: locales.AddSheetQuality(cx), TestID: "addsheet-quality",
			Value: func() kcore.Quality { return s.Choice().Quality }, OnChange: func(q kcore.Quality) { pick(func(c *Choice) { c.Quality = q }) },
			Style: core.Style{Width: core.Pt(150)}}
	})
}

// column stacks the sheet's blocks.
func column(children ...core.Node) core.Node {
	return core.View{Style: core.Style{Gap: 10, AlignItems: core.AlignStretch}, Children: children}
}

// header is the video: its site and what the link is, then its name, what the sheet read of it and the link.
func header(cx *core.Scope, l Look, meta func() Meta) core.Node {
	m := locales.For(cx)
	colors := ui.ColorsOf(cx)
	site := l.Site
	if site == "" {
		site = l.Host
	}
	kind := m.AddSheetKindVideo()
	if l.InList {
		kind = m.AddSheetKindInList()
	}
	tag := core.Text{Content: site, MaxLines: 1, Style: core.Style{FontSize: 10, FontWeight: ui.Bold, LineHeight: 17, CornerRadius: 7, Padding: core.Symmetric(0, 7), FlexShrink: 0},
		StyleFunc: func() core.Style {
			return core.Style{Background: theme.Accent.Faded(0.12), Color: theme.Of(cx).AccentInk()}
		}}
	return core.View{Style: core.Style{Gap: 4}, Children: []core.Node{
		core.View{Style: core.Style{Direction: core.Row, Gap: 6, AlignItems: core.AlignCenter}, Children: []core.Node{
			tag,
			core.Text{Content: kind, MaxLines: 1, Style: ui.TypeCaption, StyleFunc: ui.Colored(colors.Muted)},
		}},
		core.Text{ContentFunc: func() string {
			// A pasted link has no title until the sheet reads the video's page.
			if t := meta().Title; l.Title == "" && t != "" {
				return t
			}
			return l.Name(m)
		}, TestID: "addsheet-title", Role: core.RoleHeader, MaxLines: 2, Style: core.Style{FontSize: 15, LineHeight: 20, FontWeight: ui.Bold}, StyleFunc: ui.Colored(colors.Text)},
		core.Show{When: func() bool { return metaLine(cx, l, meta()) != "" }, Then: func(cx *core.Scope) core.Node {
			return core.Text{ContentFunc: func() string { return metaLine(cx, l, meta()) }, TestID: "addsheet-meta", MaxLines: 2, Style: ui.TypeCaption, StyleFunc: ui.Colored(colors.Muted)}
		}},
		core.Text{Content: l.Link, MaxLines: 1, Selectable: core.SelectableOn, Style: ui.TypeCaption, StyleFunc: ui.Colored(colors.Muted)},
	}}
}

// note is a line in a tinted box: the accent's for what the user should know, the danger color's for what keeps the video from being added. head, when not "", is said first in bold.
func note(cx *core.Scope, id, head, text string, danger bool) core.Node {
	colors := ui.ColorsOf(cx)
	tint := colors.Accent
	if danger {
		tint = colors.Danger
	}
	lines := []core.Node{}
	if head != "" {
		lines = append(lines, core.Text{Content: head, Style: core.Style{FontSize: 12, LineHeight: 17, FontWeight: ui.Bold}, StyleFunc: ui.Colored(colors.Text)})
	}
	lines = append(lines, core.Text{Content: text, Style: core.Style{FontSize: 12, LineHeight: 17}, StyleFunc: ui.Colored(colors.Text)})
	return core.View{TestID: id, Style: core.Style{Gap: 2, CornerRadius: 10, Padding: core.Symmetric(8, 12)},
		StyleFunc: func() core.Style { return core.Style{Background: tint().Faded(0.12)} }, Children: lines}
}

// metaLine is what the sheet read of the video, a fact after another: who put it on the site, how long it plays, how many videos its playlist holds and whether the uploader put subtitles on it, those of a site whose subtitles a film can take; that it is being read while it is; "" when nothing is known of it.
func metaLine(cx *core.Scope, l Look, meta Meta) string {
	if !meta.Known {
		if meta.Reading {
			return locales.AddSheetReading(cx)
		}
		return ""
	}
	var facts []string
	if meta.Uploader != "" {
		facts = append(facts, meta.Uploader)
	}
	if meta.Length > 0 {
		facts = append(facts, clock(meta.Length))
	}
	if meta.Episodes > 0 {
		facts = append(facts, locales.AddSheetEpisodesCount(cx, meta.Episodes))
	}
	if l.SiteSubtitles {
		if meta.Subtitles {
			facts = append(facts, locales.AddSheetHasSubtitles(cx))
		} else {
			facts = append(facts, locales.AddSheetNoSubtitles(cx))
		}
	}
	return strings.Join(facts, " · ")
}

// clock is a time in a video as the player says it: 3:05, or 1:02:03 past an hour.
func clock(d time.Duration) string {
	sec := int(d.Round(time.Second) / time.Second)
	if sec >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", sec/3600, sec/60%60, sec%60)
	}
	return fmt.Sprintf("%d:%02d", sec/60, sec%60)
}
