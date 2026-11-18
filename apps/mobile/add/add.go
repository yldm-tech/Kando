// Package add is the Add sheet: a link to a video, pasted or typed, the site it is on, and the computer it goes to, which downloads, transcribes and translates it while the Jobs page follows it.
package add

import (
	"strings"

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

// Sheet is the Add sheet while open reports true; close closes it, and sent follows a link sent.
// TODO(nsgo#655): take the links other apps share to Kando (an iOS share extension, Android's SEND intent) into this sheet; NSGo apps cannot receive a share yet, so a link comes by paste alone https://github.com/nsgo-dev/nsgo/issues/655
func Sheet(cx *core.Scope, s *session.Session, open func() bool, close func(), sent func()) core.Node {
	colors := ui.ColorsOf(cx)
	text := core.NewSignal(cx, "")
	// chosen is the computer the user picked, "" for the first one that answers.
	chosen := core.NewSignal(cx, "")
	link := func() (kcore.Link, bool) {
		l, ok := kcore.ParseLink(text.Get())
		return l, ok && l.Platform != kcore.PlatformLocal
	}
	valid := func() bool { _, ok := link(); return ok }
	desktop := func() string {
		list := s.Desktops()
		for _, d := range list {
			if d.ID == chosen.Get() {
				return d.ID
			}
		}
		for _, d := range list {
			if q := s.Queue(d.ID); q.Reach == session.Online && takes(q) {
				return d.ID
			}
		}
		for _, d := range list {
			if takes(s.Queue(d.ID)) {
				return d.ID
			}
		}
		if len(list) > 0 {
			return list[0].ID
		}
		return ""
	}
	desktopName := func() string { d, _ := s.Desktop(desktop()); return d.Name }
	send := func() {
		l, ok := link()
		to := desktop()
		if !ok || to == "" || !takes(s.Queue(to)) {
			return
		}
		s.Send(to, l.URL)
		text.Set("")
		close()
		sent()
	}
	paste := func() { cx.ReadClipboard(func(clip string) { text.Set(firstLink(clip)) }) }
	return ui.Dialog{TestID: "add-sheet", Open: open, TitleFunc: func() string { return locales.AddTitle(cx) },
		OnOpenChange: func(open bool, _ base.Reason) {
			if !open {
				close()
			}
		},
		OnConfirm: send,
		Children: []core.Node{
			core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
				core.Text{ContentFunc: func() string { return locales.AddLinkLabel(cx) }, Style: look.Section.Merge(core.Style{Padding: core.Edges{Left: 4}}), StyleFunc: ui.Colored(colors.Muted)},
				core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8}, Children: []core.Node{
					ui.Input{TestID: "add-link", LabelFunc: func() string { return locales.AddLinkLabel(cx) }, Value: text.Get, OnChange: text.Set,
						PlaceholderFunc: func() string { return locales.AddLinkPlaceholder(cx) }, KeyboardType: core.KeyboardURL, AutoCapitalize: core.AutoCapitalizeNone, NoAutoCorrect: true,
						Style: core.Style{Height: core.Pt(50), FontSize: 17, CornerRadius: 12, Padding: core.Symmetric(0, 14), FlexGrow: 1, FlexShrink: 1}},
				}},
				core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 8, MinHeight: core.Pt(36)}, Children: []core.Node{
					pasteButton(cx, paste),
					core.Show{When: valid, Then: func(cx *core.Scope) core.Node {
						return core.Text{TestID: "add-site", ContentFunc: func() string { l, _ := link(); return site(l) }, MaxLines: 1,
							Style:     look.Caption.Merge(core.Style{FontWeight: core.FontWeightSemibold, Padding: core.Symmetric(2, 10), CornerRadius: 10}),
							StyleFunc: func() core.Style { return core.Style{Background: ui.TintsOf(cx).Accent(), Color: colors.Accent()} }}
					}},
					core.Show{When: func() bool { return strings.TrimSpace(text.Get()) != "" && !valid() }, Then: func(cx *core.Scope) core.Node {
						return core.Text{TestID: "add-invalid", ContentFunc: func() string { return locales.AddNotALink(cx) }, LiveRegion: core.LiveRegionPolite, MaxLines: 2,
							Style: look.Caption.Merge(core.Style{FlexShrink: 1}), StyleFunc: ui.Colored(colors.Danger)}
					}},
				}},
			}},
			core.Show{When: func() bool { return len(s.Desktops()) > 1 }, Then: func(cx *core.Scope) core.Node {
				return core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
					core.Text{ContentFunc: func() string { return locales.AddComputer(cx) }, Style: look.Section.Merge(core.Style{Padding: core.Edges{Left: 4}}), StyleFunc: ui.Colored(colors.Muted)},
					core.View{Role: core.RoleRadioGroup, LabelFunc: func() string { return locales.AddComputer(cx) }, Children: []core.Node{look.Chips(
						core.Each(s.Desktops, func(d store.Desktop) string { return d.ID }, func(cx *core.Scope, d func() store.Desktop, _ func() int) core.Node {
							return look.Chip{TestID: "add-computer-" + d().ID, Radio: true, Text: func() string { return d().Name },
								On: func() bool { return desktop() == d().ID }, OnPress: func() { chosen.Set(d().ID) }}
						}),
					)}},
				}}
			}},
			core.Text{TestID: "add-note", ContentFunc: func() string {
				q := s.Queue(desktop())
				switch {
				case q.Reach == session.Older:
					return locales.JobsOlder(cx)
				case q.Reach == session.Revoked:
					return locales.ComputersUnpaired(cx, desktopName())
				case !takes(q):
					return locales.AddNoLinks(cx, desktopName())
				case q.Reach == session.Online && q.Relayed:
					return locales.AddNoteRelay(cx, desktopName())
				case q.Reach == session.Online:
					return locales.AddNote(cx, desktopName())
				case q.Relayed:
					return locales.AddNoteOfflineRelay(cx, desktopName())
				}
				return locales.AddNoteOffline(cx, desktopName())
			}, Style: look.Caption, StyleFunc: ui.Colored(colors.Muted)},
		},
		Footer: []core.Node{
			look.Button{Kind: look.Plain, Text: func() string { return locales.CommonCancel(cx) }, TestID: "add-cancel", OnPress: close},
			look.Button{Kind: look.Primary, Text: func() string { return locales.AddSend(cx) }, Icon: lucide.Send, TestID: "add-send",
				Disabled: func() bool { to := desktop(); return !valid() || to == "" || !takes(s.Queue(to)) }, OnPress: send},
		}}
}

// pasteButton puts the clipboard's link in the field.
func pasteButton(cx *core.Scope, paste func()) core.Node {
	colors, tints := ui.ColorsOf(cx), ui.TintsOf(cx)
	text := func() string { return locales.AddPaste(cx) }
	return base.Button{TestID: "add-paste", LabelFunc: text, OnPress: func(core.PressEvent) { paste() },
		Style: core.Style{Direction: core.Row, Gap: 6, MinHeight: core.Pt(36), Padding: core.Symmetric(0, 14), CornerRadius: 18, BorderCurve: core.BorderCurveContinuous,
			AlignItems: core.AlignCenter, Active: &core.Style{Opacity: core.Alpha(0.7)}},
		StyleFunc: func() core.Style { return core.Style{Background: tints.Accent(), Color: colors.Accent()} },
		Children: []core.Node{
			ui.Icon(lucide.ClipboardPaste, 16, ui.Inherited),
			core.Text{ContentFunc: text, MaxLines: 1, Style: look.Subhead.Merge(core.Style{FontWeight: core.FontWeightSemibold})},
		}}
}

// takes reports whether a computer's queue may take a link: not one too old for links or that revoked the phone, nor one that answers without listing sync.OpJobCreate. One not reached yet may, and the link waits on the phone until it answers.
func takes(q session.Queue) bool {
	switch q.Reach {
	case session.Older, session.Revoked:
		return false
	case session.Online:
		return q.Can(sync.OpJobCreate)
	}
	return true
}

// site is the site a link is on, as its chip names it: the site Kando knows it as, else its host.
func site(l kcore.Link) string {
	if l.Site != "" {
		return l.Site
	}
	return l.Host
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
