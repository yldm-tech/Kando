// Package add is the Add sheet: a link to a video, pasted or typed, the site it is on, the computer it goes to, which downloads, transcribes and translates it while the Jobs page follows it, and, where that computer takes them, the picture it downloads and where the original lines come from. A link to a channel, an uploader's space, a collection or a playlist becomes a subscription of the computer's instead, whose new videos it adds as they come (Session.Subscribe), where the computer shares its subscriptions.
package add

import (
	"slices"
	"strings"

	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"
	"kando/apps/mobile/theme"
	kcore "kando/packages/core"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// Sheet is the Add sheet while open reports true, headed by the design's bar of Cancel and Send; close closes it, and sent follows a link sent.
// TODO(nsgo#655): take the links other apps share to Kando (an iOS share extension, Android's SEND intent) into this sheet; NSGo apps cannot receive a share yet, so a link comes by paste alone https://github.com/nsgo-dev/nsgo/issues/655
func Sheet(cx *core.Scope, s *session.Session, open func() bool, close func(), sent func()) core.Node {
	colors := ui.ColorsOf(cx)
	text := core.NewSignal(cx, "")
	// chosen is the computer the user picked in the sheet, "" for the one that processes new links (session.Session.SendTo).
	chosen := core.NewSignal(cx, "")
	// quality and subtitles are the job options the user picked (sync.JobOptions), "" for the computer's own setting.
	quality := core.NewSignal(cx, "")
	subtitles := core.NewSignal(cx, "")
	link := func() (kcore.Link, bool) {
		l, ok := kcore.ParseLink(text.Get())
		return l, ok && l.Platform != kcore.PlatformLocal
	}
	valid := func() bool { _, ok := link(); return ok }
	// list is a link the computer subscribes to rather than downloads: a list, sent to a computer that shares its subscriptions.
	list := func(to string) bool {
		l, ok := link()
		return ok && l.IsList() && slices.Contains(s.Queue(to).Ops, sync.OpSubscribe)
	}
	// ready reports whether the link can go to computer to now: a subscription needs the computer to answer, and to let this phone subscribe; a video waits on the phone for a computer not reached yet.
	ready := func(to string) bool {
		if to == "" || !valid() {
			return false
		}
		if list(to) {
			q := s.Queue(to)
			return q.Reach == session.Online && q.Can(sync.OpSubscribe) && !s.Subscribing(to)
		}
		return Takes(s.Queue(to))
	}
	desktop := func() string { return Destination(s, chosen.Get()) }
	desktopName := func() string { d, _ := s.Desktop(desktop()); return d.Name }
	send := func() {
		l, _ := link()
		to := desktop()
		if !ready(to) {
			return
		}
		if list(to) {
			text.Set("")
			close()
			s.Subscribe(to, l.URL)
			return
		}
		// Only the options the sheet shows for this computer go with the link: one picked for another computer, or before this one went offline, is not on screen and not sent.
		q := s.Queue(to)
		var opts sync.JobOptions
		if q.Offers(sync.OptionQuality) {
			opts.Quality = quality.Get()
		}
		if q.Offers(sync.OptionSubtitles) {
			opts.Subtitles = subtitles.Get()
		}
		s.Send(to, l.URL, opts)
		text.Set("")
		close()
		sent()
	}
	paste := func() { cx.ReadClipboard(func(clip string) { text.Set(firstLink(clip)) }) }
	focused := core.NewSignal(cx, false)
	return look.Dialog{TestID: "add-sheet", Open: open, Title: func() string { return locales.AddTitle(cx) }, OnClose: close,
		OnConfirm: send,
		// A list link is sent too, as the note under the form says it becomes a subscription.
		Nav: &look.SheetNav{Cancel: func() string { return locales.CommonCancel(cx) }, Confirm: func() string { return locales.AddSend(cx) }, CancelID: "add-cancel", ConfirmID: "add-send",
			Busy: func() bool { return s.Subscribing(desktop()) }, Disabled: func() bool { return !ready(desktop()) }, OnCancel: close, OnConfirm: send},
		Children: []core.Node{
			core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
				label(cx, func() string { return locales.AddLinkLabel(cx) }),
				core.View{Children: []core.Node{
					ui.Input{TestID: "add-link", LabelFunc: func() string { return locales.AddLinkLabel(cx) }, Value: text.Get, OnChange: text.Set,
						OnFocus: func() { focused.Set(true) }, OnBlur: func() { focused.Set(false) },
						PlaceholderFunc: func() string { return locales.AddLinkPlaceholder(cx) }, KeyboardType: core.KeyboardURL, AutoCapitalize: core.AutoCapitalizeNone, NoAutoCorrect: true,
						Style: core.Style{Height: core.Pt(50), FontSize: 16, CornerRadius: 13, Padding: core.Edges{Right: 100, Left: 14}},
						// The field is a card with no rim, as the design draws it, until it takes focus, when it shows the accent's rim as ui.Input's own does: setting the rim's color turns ui.Input's focus rim off (ui/input.go:8), so the sheet draws it.
						StyleFunc: func() core.Style {
							rim := colors.Surface()
							if focused.Get() {
								rim = colors.Accent()
							}
							return core.Style{Background: colors.Surface(), BorderColor: rim, Hover: &core.Style{BorderColor: rim}}
						}},
					core.Show{When: valid, Then: func(cx *core.Scope) core.Node {
						return core.View{Style: core.Style{Position: core.Absolute, Top: core.Pt(0), Bottom: core.Pt(0), Right: core.Pt(10), JustifyContent: core.JustifyCenter}, PointerEvents: core.PointerEventsNone, Children: []core.Node{
							core.Text{TestID: "add-site", ContentFunc: func() string { l, _ := link(); return site(cx, l) }, MaxLines: 1,
								Style:     core.Style{FontSize: 11, LineHeight: 14, FontWeight: core.FontWeightBold, Padding: core.Symmetric(3, 8), CornerRadius: 10, MaxWidth: core.Pt(90), Color: theme.OnMark},
								StyleFunc: func() core.Style { l, _ := link(); return core.Style{Background: mark(l)} }},
						}}
					}},
				}},
				core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, MinHeight: core.Pt(36)}, Children: []core.Node{
					pasteButton(cx, paste),
					core.Show{When: func() bool { return strings.TrimSpace(text.Get()) != "" && !valid() }, Then: func(cx *core.Scope) core.Node {
						return core.Text{TestID: "add-invalid", ContentFunc: func() string { return locales.AddNotALink(cx) }, LiveRegion: core.LiveRegionPolite, MaxLines: 2,
							Style: look.Caption.Merge(core.Style{FlexShrink: 1}), StyleFunc: ui.Colored(colors.Danger)}
					}},
				}},
			}},
			core.Show{When: func() bool { return len(s.Desktops()) > 1 }, Then: func(cx *core.Scope) core.Node {
				return core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
					label(cx, func() string { return locales.AddComputer(cx) }),
					core.View{Role: core.RoleRadioGroup, LabelFunc: func() string { return locales.AddComputer(cx) }, Children: []core.Node{look.Chips(
						core.Each(s.Desktops, func(d store.Desktop) string { return d.ID }, func(cx *core.Scope, d func() store.Desktop, _ func() int) core.Node {
							return look.Chip{TestID: "add-computer-" + d().ID, Radio: true, Text: func() string { return d().Name },
								On: func() bool { return desktop() == d().ID }, OnPress: func() { chosen.Set(d().ID) }}
						}),
					)}},
				}}
			}},
			core.Show{When: func() bool { return !list(desktop()) && s.Queue(desktop()).Offers(sync.OptionSubtitles) }, Then: func(cx *core.Scope) core.Node {
				return subtitlesChoice(cx, desktopName, subtitles)
			}},
			core.Show{When: func() bool { return !list(desktop()) && s.Queue(desktop()).Offers(sync.OptionQuality) }, Then: func(cx *core.Scope) core.Node {
				return qualityChoice(cx, desktopName, quality)
			}},
			core.Text{TestID: "add-note", ContentFunc: func() string {
				q := s.Queue(desktop())
				switch {
				case list(desktop()) && q.Reach == session.Online && !q.Can(sync.OpSubscribe):
					return locales.AddSubscribeReadOnly(cx, desktopName())
				case list(desktop()) && q.Reach == session.Online:
					return locales.AddSubscribeNote(cx, desktopName())
				case list(desktop()):
					return locales.AddSubscribeOffline(cx, desktopName())
				case q.Reach == session.Older:
					return locales.JobsOlder(cx)
				case q.Reach == session.Revoked:
					return locales.ComputersUnpaired(cx, desktopName())
				case q.ReadOnly:
					return locales.JobsReadOnly(cx, desktopName())
				case !Takes(q):
					return locales.AddNoLinks(cx, desktopName())
				case q.Relayed && q.Storage != sync.RelayNoFailure:
					// The storage fails the phone: the link waits on it.
					return locales.AddNoteOffline(cx, desktopName())
				case q.Reach == session.Online && q.Relayed:
					return locales.AddNoteRelay(cx, desktopName())
				case q.Reach == session.Online:
					return locales.AddNote(cx, desktopName())
				case q.Relayed:
					return locales.AddNoteOfflineRelay(cx, desktopName())
				}
				return locales.AddNoteOffline(cx, desktopName())
			}, Style: core.Style{FontSize: 12, LineHeight: 17, Padding: core.Edges{Right: 4, Left: 4}}, StyleFunc: ui.Colored(colors.Muted)},
		}}
}

// label names a part of the form, over it: the design's 13-point muted caption, in line with the text in the cards under it.
func label(cx *core.Scope, text func() string) core.Node {
	return core.Text{ContentFunc: text, MaxLines: 1, Style: look.Caption.Merge(core.Style{Padding: core.Edges{Left: 4}}), StyleFunc: ui.Colored(ui.ColorsOf(cx).Muted)}
}

// qualityChoice is the picture the computer downloads, as the design's segmented control under Save: its own setting, the best there is, or 720p.
// The design's third choice, the subtitles alone with no video, is not a job option the protocol has (sync.JobOptions.Quality).
func qualityChoice(cx *core.Scope, computer func() string, quality *core.Signal[string]) core.Node {
	text := func() string { return locales.AddQuality(cx) }
	return core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
		label(cx, text),
		look.Segmented{Label: text, Value: quality.Get, OnChange: quality.Set, Segments: []look.Segment{
			{Value: "", TestID: "add-quality-computer", Text: func() string { return locales.AddAsComputer(cx, computer()) }},
			{Value: sync.QualityBest, TestID: "add-quality-best", Text: func() string { return locales.AddQualityBest(cx) }},
			{Value: sync.Quality720, TestID: "add-quality-720", Text: func() string { return locales.AddQuality720(cx) }},
		}},
	}}
}

// subtitlesChoice is where the original lines come from, as a card of rows each with a radio circle at its end: the computer's own setting, the subtitles the uploader put on the site, or the computer's speech recognition.
func subtitlesChoice(cx *core.Scope, computer func() string, subtitles *core.Signal[string]) core.Node {
	text := func() string { return locales.AddSubtitles(cx) }
	row := func(i int, value, id string, title, detail func() string) core.Node {
		on := func() bool { return subtitles.Get() == value }
		return look.Row{TestID: "add-subtitles-" + id, Index: func() int { return i }, Title: title, Detail: detail, Checked: on, Compact: true,
			Trailing: look.RadioMark(cx, on), OnPress: func() { subtitles.Set(value) }}
	}
	return core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
		label(cx, text),
		core.View{Role: core.RoleRadioGroup, LabelFunc: text, Children: []core.Node{look.Group{Children: []core.Node{
			row(0, "", "computer", func() string { return locales.AddAsComputer(cx, computer()) }, nil),
			row(1, sync.SubtitlesPlatform, "platform", func() string { return locales.AddSubtitlesPlatform(cx) }, func() string { return locales.AddSubtitlesPlatformNote(cx) }),
			row(2, sync.SubtitlesRecognize, "recognize", func() string { return locales.AddSubtitlesRecognize(cx) }, func() string { return locales.AddSubtitlesRecognizeNote(cx, computer()) }),
		}}}},
	}}
}

// pasteButton puts the clipboard's link in the field: the design's white pill with the accent's text.
func pasteButton(cx *core.Scope, paste func()) core.Node {
	colors := ui.ColorsOf(cx)
	text := func() string { return locales.AddPaste(cx) }
	return base.Button{TestID: "add-paste", LabelFunc: text, OnPress: func(core.PressEvent) { paste() },
		Style: core.Style{Direction: core.Row, Gap: 6, MinHeight: core.Pt(36), Padding: core.Symmetric(0, 14), CornerRadius: 18, BorderCurve: core.BorderCurveContinuous,
			AlignItems: core.AlignCenter, Active: &core.Style{Opacity: core.Alpha(0.7)}},
		StyleFunc: func() core.Style { return core.Style{Background: colors.Surface(), Color: colors.Accent()} },
		Children: []core.Node{
			ui.Icon(lucide.Clipboard, 14, ui.Inherited),
			core.Text{ContentFunc: text, MaxLines: 1, Style: core.Style{FontSize: 14, LineHeight: 18, FontWeight: core.FontWeightSemibold}},
		}}
}

// Takes reports whether a computer's queue may take a link (session.Queue.Takes).
func Takes(q session.Queue) bool { return q.Takes() }

// Destination is the paired computer a link goes to: chosen, the one the user picked in the sheet, while it is paired; else the one Settings shows as processing new links (session.Session.SendTo), so the sheet and Settings name the same computer.
func Destination(s *session.Session, chosen string) string {
	for _, d := range s.Desktops() {
		if d.ID == chosen {
			return d.ID
		}
	}
	return s.SendTo()
}

// site is the site a link is on, as its chip names it: YouTube and Bilibili by the names the Library gives them, a channel or a playlist as one whatever its site, else the site Kando knows it as, else its host.
func site(cx *core.Scope, l kcore.Link) string {
	switch {
	case l.IsList():
		return locales.AddChannel(cx)
	case l.Platform == kcore.PlatformYouTube:
		return locales.FilmsSourceYoutube(cx)
	case l.Platform == kcore.PlatformBilibili:
		return locales.FilmsSourceBilibili(cx)
	case l.Site != "":
		return l.Site
	}
	return l.Host
}

// mark is the color of a link's chip: the site's own for YouTube and Bilibili, purple for a channel or a playlist, indigo for another site.
func mark(l kcore.Link) core.Color {
	switch {
	case l.IsList():
		return theme.SourceChannel
	case l.Platform == kcore.PlatformYouTube:
		return theme.SourceYouTube
	case l.Platform == kcore.PlatformBilibili:
		return theme.SourceBilibili
	}
	return theme.SourceOther
}

// firstLink is the first link to a video on the web in text, as a share sheet's text that holds a title before its link; text trimmed when it holds none.
func firstLink(text string) string {
	for _, field := range strings.Fields(text) {
		if l, ok := kcore.ParseLink(field); ok && l.Platform != kcore.PlatformLocal && strings.Contains(field, ".") {
			return field
		}
	}
	return strings.TrimSpace(text)
}
