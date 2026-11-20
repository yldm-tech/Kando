// Package settings is the Settings tab: the app's card; the language subtitles are translated into, which the computers translate into too, and how the player draws them (style.go); the interface's language and the app's appearance; the computer that processes the links the phone sends, with its AI service and context switch, and its page (computer.go), where the paired computers are picked among, unpaired and another paired; the phone's own review reminder and daily goal, the films saved on it (offline.go), its widgets on iOS (widgets.go) and a tablet's keyboard shortcuts (shortcuts.go); the sync conflicts to settle (conflicts.go); and the app's version, where Kando's source is, with the introduction to see again. It has the page the phone shows once a computer no longer takes it too (revoked.go).
package settings

import (
	"strings"

	"kando/apps/mobile/brand"
	"kando/apps/mobile/langs"
	"kando/apps/mobile/learning"
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/onboarding"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"
	"kando/apps/mobile/theme"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// Actions are where the tab leads; Computer opens the processing computer's page, computer id's, and Unpaired runs once the user unpaired computer id, for the pages of it to go. Shortcuts opens the keyboard shortcuts, which a tablet lists, Widgets the widgets, which iOS has, Language the interface's language and Offline the films saved on the device.
type Actions struct {
	Pair, Target, Style, Goal, Conflicts, Introduction func()
	Shortcuts, Widgets, Language, Offline              func()
	Computer, Unpaired                                 func(id string)
}

// column is the page's column, its groups closer together than a library's sections, as the design's grouped list sets them.
func column(cx *core.Scope) func() core.Style {
	col := look.Column(cx)
	return func() core.Style {
		st := col()
		st.Gap = 23
		return st
	}
}

// fixed is a tile's color that is the same in both appearances.
func fixed(c core.Color) func() core.Color { return func() core.Color { return c } }

// glyph is a tile's mark that is the same in every language: a symbol, or the letters of a name, as the design's K for Kando and AI.
func glyph(g string) func() string { return func() string { return g } }

// Page is the tab's root page: the app's card, then the subtitles, the app's look, the computer that processes the links, this device's own settings, the sync conflicts and what the app is, over its version and license.
func Page(cx *core.Scope, s *session.Session, act Actions) base.NavPage {
	colors := ui.ColorsOf(cx)
	title := func() string { return locales.SettingsTitle(cx) }
	ref := &core.Ref{}
	version := core.App().Version
	about := []core.Node{}
	if version != "" {
		about = append(about, look.Row{TestID: "settings-version",
			Leading: look.GlyphTile(cx, glyph("K"), theme.Of(cx).TileInk),
			Title:   func() string { return locales.SettingsVersion(cx) },
			Value:   func() string { return version },
			Chevron: true,
			OnPress: func() { cx.OpenURL(onboarding.DownloadURL+"/releases", nil) }})
	}
	source := len(about)
	about = append(about,
		look.Row{TestID: "settings-source", Index: func() int { return source }, Chevron: true,
			Leading: look.GlyphTile(cx, glyph("{ }"), theme.Of(cx).TileInk),
			Title:   func() string { return locales.SettingsSource(cx) },
			Value:   func() string { return "GitHub" },
			OnPress: func() { cx.OpenURL(onboarding.DownloadURL, nil) }})
	intro := len(about)
	about = append(about, look.Row{TestID: "settings-introduction", Index: func() int { return intro }, Chevron: true,
		Leading: look.GlyphTile(cx, glyph("?"), fixed(theme.TileBlue)),
		Title:   func() string { return locales.SettingsIntroduction(cx) },
		OnPress: act.Introduction})
	look2 := []core.Node{}
	// A tablet leaves the language to the system's settings, as the design's iPad has no row of it.
	if !look.Tablet(cx) && act.Language != nil {
		look2 = append(look2, look.Row{TestID: "settings-language", Chevron: true,
			Leading: look.GlyphTile(cx, func() string { return locales.SettingsTileLanguage(cx) }, fixed(theme.TilePurple)),
			Title:   func() string { return locales.SettingsLanguage(cx) },
			Value:   func() string { return languageName(cx, s.Prefs().Language) },
			OnPress: act.Language})
	}
	appearance := len(look2)
	look2 = append(look2, look.Row{TestID: "settings-appearance", Index: func() int { return appearance }, Chevron: true,
		Leading: look.GlyphTile(cx, glyph("◐"), theme.Of(cx).TileInk),
		Title:   func() string { return locales.SettingsAppearance(cx) },
		Value:   func() string { return appearanceName(cx, s.Prefs().Appearance) },
		OnPress: func() { s.SetAppearance(session.NextAppearance(s.Prefs().Appearance)) }})
	return look.TabRoot(cx, base.NavPage{TestID: "settings-page", Children: []core.Node{
		core.Scroll{TestID: "settings-scroll", Ref: ref, Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
			core.View{StyleFunc: column(cx), Children: []core.Node{
				profile(cx, s),
				section(cx, func() string { return locales.SettingsSubtitles(cx) },
					look.Row{TestID: "settings-target", Chevron: true,
						Leading: look.GlyphTile(cx, func() string { return locales.SettingsTileTarget(cx) }, fixed(theme.TileBlue)),
						Title:   func() string { return locales.SettingsTarget(cx) },
						Value:   func() string { return langs.TargetName(s.Target()) },
						OnPress: act.Target},
					look.Row{TestID: "settings-style", Index: func() int { return 1 }, Chevron: true,
						Leading: look.GlyphTile(cx, glyph("Aa"), fixed(theme.TileIndigo)),
						Title:   func() string { return locales.SettingsSubtitleStyle(cx) },
						Value: func() string {
							st := s.Prefs().Subtitles
							return sizeShortName(cx, st.Size) + " · " + colorName(cx, st.Color)
						},
						OnPress: act.Style}),
				section(cx, func() string { return locales.SettingsAppearance(cx) }, look2...),
				section(cx, func() string { return locales.SettingsComputers(cx) }, computerRows(cx, s, act)),
				section(cx, func() string { return thisDevice(cx) }, deviceRows(cx, s, act)...),
				section(cx, func() string { return locales.SettingsSync(cx) },
					look.Row{TestID: "settings-conflicts", Chevron: true,
						Leading: look.GlyphTile(cx, glyph("⇄"), fixed(theme.TileOrange)),
						Title:   func() string { return locales.SettingsConflicts(cx) },
						Detail: func() string {
							if len(s.Conflicts()) == 0 {
								return ""
							}
							return locales.SettingsConflictsDetail(cx)
						},
						Value: func() string {
							if n := len(s.Conflicts()); n > 0 {
								return locales.SettingsConflictsCount(cx, n)
							}
							return locales.SettingsConflictsNone(cx)
						},
						OnPress: act.Conflicts}),
				section(cx, func() string { return locales.SettingsAbout(cx) }, about...),
				core.Show{When: func() bool { return version != "" }, Then: func(cx *core.Scope) core.Node {
					// The footer sits clear of the column's end, about 35 points over the tab bar at the end of the scroll, as the design leaves it.
					return core.View{Style: core.Style{Padding: core.Edges{Right: 16, Bottom: 11, Left: 16}}, Children: []core.Node{
						core.Text{TestID: "settings-footer", ContentFunc: func() string { return locales.SettingsFooter(cx, locales.AppName(cx), version) },
							Style: look.Footnote.Merge(core.Style{LineHeight: 17, TextAlign: core.TextAlignCenter}), StyleFunc: ui.Colored(colors.Muted)},
					}}
				}},
			}},
		}},
	}}, title, nil, nil)
}

// profile is the card at the top of Settings: the app's icon and name, the computer that processes its links and the language its subtitles are translated into.
func profile(cx *core.Scope, s *session.Session) core.Node {
	colors := ui.ColorsOf(cx)
	detail := func() string {
		target := langs.TargetName(s.Target())
		if d, ok := s.Desktop(s.SendTo()); ok {
			return locales.SettingsProfileDetail(cx, d.Name, target)
		}
		return locales.SettingsProfileUnpaired(cx, target)
	}
	return core.View{TestID: "settings-profile", Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 14, Padding: core.Symmetric(14, 16)},
		StyleFunc: func() core.Style { return look.Card(cx) },
		Children: []core.Node{
			core.Image{Source: brand.Icon(), ResizeMode: core.Cover, ElementsHidden: true,
				Style: core.Style{Width: core.Pt(54), Height: core.Pt(54), CornerRadius: 12, BorderCurve: core.BorderCurveContinuous, FlexShrink: 0}},
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: []core.Node{
				core.Text{ContentFunc: func() string { return locales.AppName(cx) }, Role: core.RoleHeader, MaxLines: 1,
					Style: core.Style{FontSize: 17, LineHeight: 22, FontWeight: look.Heavy}, StyleFunc: ui.Colored(colors.Text)},
				core.Text{TestID: "settings-profile-detail", ContentFunc: detail, MaxLines: 3, Style: look.Caption, StyleFunc: ui.Colored(colors.Muted)},
			}},
		}}
}

// computerRows are the Computer group's rows: the computer that processes the links, with whether it answers, which opens its page, the AI service it translates with and its context switch; or, with none paired, the row that pairs one.
func computerRows(cx *core.Scope, s *session.Session, act Actions) core.Node {
	id := s.SendTo
	return core.Show{When: func() bool { return id() != "" },
		Then: func(cx *core.Scope) core.Node {
			name := func() string { d, _ := s.Desktop(id()); return d.Name }
			return core.View{Children: []core.Node{
				look.Row{TestID: "settings-computer", Chevron: true, TitleLines: 2,
					Leading: look.GlyphTile(cx, glyph("⌘"), fixed(theme.TileTeal)),
					Title:   func() string { return locales.SettingsComputer(cx) },
					Detail: func() string {
						switch reach(s, id()) {
						case session.Online:
							return locales.SettingsComputerOnline(cx)
						case session.Offline:
							return locales.SettingsComputerOffline(cx)
						}
						return ""
					},
					Value:   name,
					OnPress: func() { act.Computer(id()) }},
				look.Row{TestID: "settings-service", Index: func() int { return 1 }, Chevron: true,
					Leading: look.GlyphTile(cx, glyph("AI"), fixed(theme.TileViolet)),
					Title:   func() string { return locales.ComputerService(cx) },
					Value:   func() string { return serviceName(cx, s, id()) },
					OnPress: func() { act.Computer(id()) }},
				contextRow(cx, s, id, 2),
			}}
		},
		Else: func(cx *core.Scope) core.Node {
			return look.Row{TestID: "settings-pair", TitleTone: look.ToneAccent,
				Leading: look.GlyphTile(cx, glyph("+"), fixed(theme.TileTeal)),
				Title:   func() string { return locales.SettingsPair(cx) },
				OnPress: act.Pair}
		}}
}

// languageName is the interface language lang as the Language row says it: its own name, or Automatic for the system's.
func languageName(cx *core.Scope, lang string) string {
	if n := langs.TargetName(lang); lang != "" && n != "" {
		return n
	}
	return locales.SettingsLanguageSystem(cx)
}

// LanguagePage is the page that picks the interface's language among the catalogs', or the system's; the app shows it at once.
func LanguagePage(cx *core.Scope, s *session.Session, back func()) base.NavPage {
	colors := ui.ColorsOf(cx)
	ref := &core.Ref{}
	choices := append([]string{""}, locales.Interface...)
	var rows []core.Node
	for i, tag := range choices {
		id := tag
		if id == "" {
			id = "system"
		}
		rows = append(rows, look.Row{TestID: "settings-language-" + id, Index: func() int { return i },
			Title:   func() string { return languageName(cx, tag) },
			Checked: func() bool { return s.Prefs().Language == tag },
			Trailing: core.Show{When: func() bool { return s.Prefs().Language == tag }, Then: func(*core.Scope) core.Node {
				return ui.Icon(lucide.Check, 20, colors.Accent)
			}},
			OnPress: func() { s.SetLanguage(tag) }})
	}
	return look.NavPage(cx, base.NavPage{TitleFunc: func() string { return locales.SettingsLanguage(cx) }, TestID: "settings-language-page", Children: []core.Node{
		core.Scroll{Ref: ref, Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
			core.View{StyleFunc: look.Column(cx), Children: []core.Node{
				core.View{Role: core.RoleRadioGroup, LabelFunc: func() string { return locales.SettingsLanguage(cx) }, Children: []core.Node{look.Group{Children: rows}}},
			}},
		}},
	}}, ui.NavHeader{OnBack: back, Scroll: ref})
}

// appearanceName is the app's appearance as the row of it says it: light, dark, or the system's.
func appearanceName(cx *core.Scope, a string) string {
	switch a {
	case store.AppearanceLight:
		return locales.SettingsAppearanceLight(cx)
	case store.AppearanceDark:
		return locales.SettingsAppearanceDark(cx)
	}
	return locales.SettingsAppearanceSystem(cx)
}

// deviceRows are the rows of what this phone or tablet does on its own: its review reminder and daily goal, the films saved on it, its widgets on iOS, which alone has them, and the keyboard shortcuts on a tablet, which is the one used with a hardware keyboard.
func deviceRows(cx *core.Scope, s *session.Session, act Actions) []core.Node {
	rows := []core.Node{
		look.Row{TestID: "settings-remind", TitleLines: 1,
			Leading: look.GlyphTile(cx, glyph("✎"), fixed(theme.TileOrange)),
			Title:   func() string { return locales.SettingsRemind(cx) },
			Detail:  func() string { return locales.SettingsRemindDetail(cx, learning.ReminderHour) },
			// TODO(nsgo): ui.Switch takes its accessibility label as a string (ui/switch.go:19), with no LabelFunc, so the switch is built again as the language changes; the Language page is another page, so this switch holds no focus then.
			Trailing: core.Dynamic(func(cx *core.Scope) core.Node {
				return look.Switch{TestID: "settings-remind-switch", Label: locales.SettingsRemind(cx),
					Checked: func() bool { return s.Prefs().Remind }, OnChange: s.SetRemind}
			})},
		look.Row{TestID: "settings-goal", Index: func() int { return 1 }, Chevron: true,
			Leading: look.GlyphTile(cx, glyph("◎"), fixed(theme.TileGreen)),
			Title:   func() string { return locales.SettingsGoal(cx) },
			Value:   func() string { return locales.SettingsGoalValue(cx, s.Goal()) },
			OnPress: act.Goal},
	}
	add := func(r look.Row) {
		i := len(rows)
		r.Index = func() int { return i }
		rows = append(rows, r)
	}
	if act.Offline != nil {
		add(look.Row{TestID: "settings-offline", Chevron: true,
			Leading: look.GlyphTile(cx, glyph("↓"), fixed(theme.TileSky)),
			Title:   func() string { return locales.SettingsOffline(cx) },
			Value:   func() string { return locales.SettingsOfflineCount(cx, len(s.Films())) },
			OnPress: act.Offline})
	}
	if cx.Env().OS() == core.OSIOS && act.Widgets != nil {
		add(look.Row{TestID: "settings-widgets", Chevron: true,
			Leading: look.GlyphTile(cx, glyph("▦"), fixed(theme.TileIndigo)),
			Title:   func() string { return locales.SettingsWidgets(cx) },
			OnPress: act.Widgets})
	}
	if look.Tablet(cx) && act.Shortcuts != nil {
		add(look.Row{TestID: "settings-shortcuts", Chevron: true,
			Leading: look.GlyphTile(cx, glyph("⌨"), fixed(theme.TileGrey)),
			Title:   func() string { return locales.SettingsShortcuts(cx) },
			Value:   func() string { return locales.SettingsShortcutsValue(cx) },
			OnPress: act.Shortcuts})
	}
	return rows
}

// section is a titled group of rows.
func section(cx *core.Scope, title func() string, rows ...core.Node) core.Node {
	return core.View{Style: core.Style{Gap: 7}, Children: []core.Node{look.GroupHeader(cx, title), look.Group{Children: rows}}}
}

// confirmUnpair asks whether to unpair computer id, named name, and unpairs it once the user confirms; unpaired runs then.
func confirmUnpair(cx *core.Scope, s *session.Session, id, name string, unpaired func(id string)) {
	// A tablet asks in a dialog in the middle of the screen, as its sheets show, rather than an action sheet with nothing to point at.
	cx.ShowAlert(core.AlertConfig{Title: locales.ComputersUnpairTitle(cx, name), Message: locales.ComputersUnpairMessage(cx), Sheet: !look.Tablet(cx), Cancelable: true,
		Actions: []core.AlertAction{
			{Label: locales.CommonCancel(cx), Role: core.AlertCancel},
			{Label: locales.ComputersUnpair(cx), Role: core.AlertDestructive},
		}}, func(i int) {
		if i == 1 {
			s.Unpair(id)
			unpaired(id)
		}
	})
}

// TargetPage is the page that picks the language subtitles are translated into.
func TargetPage(cx *core.Scope, s *session.Session, back func()) base.NavPage {
	colors := ui.ColorsOf(cx)
	ref := &core.Ref{}
	var rows []core.Node
	for i, l := range langs.Targets {
		rows = append(rows, look.Row{TestID: "settings-target-" + l.Code, Index: func() int { return i },
			Title:   func() string { return l.Name },
			Checked: func() bool { return s.Target() == l.Code },
			Trailing: core.Show{When: func() bool { return s.Target() == l.Code }, Then: func(*core.Scope) core.Node {
				return ui.Icon(lucide.Check, 20, colors.Accent)
			}},
			OnPress: func() { s.SetTarget(l.Code) }})
	}
	return look.NavPage(cx, base.NavPage{TitleFunc: func() string { return locales.SettingsTarget(cx) }, TestID: "settings-target-page", Children: []core.Node{
		core.Scroll{Ref: ref, Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
			core.View{StyleFunc: look.Column(cx), Children: []core.Node{
				core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
					// The note goes over the list, which runs past the screen's foot.
					core.Text{TestID: "settings-target-note", ContentFunc: func() string { return locales.SettingsTargetNote(cx) }, Style: look.Caption.Merge(core.Style{Padding: core.Edges{Right: 16, Left: 16}}), StyleFunc: ui.Colored(colors.Muted)},
					core.View{Role: core.RoleRadioGroup, LabelFunc: func() string { return locales.SettingsTarget(cx) }, Children: []core.Node{look.Group{Children: rows}}},
				}},
			}},
		}},
	}}, ui.NavHeader{OnBack: back, Scroll: ref})
}

// Computers is the card of the paired computers at the foot of a tablet's sidebar: the drawing of a computer, their names with a dot that is green while they answer, and the jobs they have yet to finish, or that none is paired; open shows the Jobs tab.
func Computers(cx *core.Scope, s *session.Session, open func()) core.Node {
	colors := ui.ColorsOf(cx)
	names := func() string {
		var out []string
		for _, d := range s.Desktops() {
			out = append(out, d.Name)
		}
		if len(out) == 0 {
			return locales.SettingsNoComputer(cx)
		}
		return strings.Join(out, " · ")
	}
	// out reports whether the phone lost the computer desktop: its last read failed, or the queue the phone follows found it out of reach since.
	out := func(desktop string) bool {
		return s.Shelf(desktop).Err != nil || s.Queue(desktop).Reach == session.Offline
	}
	// online is whether every paired computer answers.
	online := func() bool {
		ds := s.Desktops()
		for _, d := range ds {
			if out(d.ID) {
				return false
			}
		}
		return len(ds) > 0
	}
	// gone is whether every paired computer is out of reach, so every unfinished job and link waits for one.
	gone := func() bool {
		ds := s.Desktops()
		for _, d := range ds {
			if !out(d.ID) {
				return false
			}
		}
		return len(ds) > 0
	}
	status := func() string {
		active, failed := s.Unfinished()
		switch {
		case gone():
			// Out of reach, the card says so and how many jobs wait for the computers, as the design's sidebar does. While one still answers, its jobs run and the card counts them, and the failed ones, as it does with all in reach.
			if active > 0 {
				return locales.JobsOfflineWaiting(cx, active)
			}
			return locales.JobsOffline(cx)
		case failed > 0:
			return locales.JobsActiveFailed(cx, active, failed)
		case active > 0:
			return locales.JobsActive(cx, active)
		}
		return ""
	}
	label := func() string {
		if st := status(); st != "" {
			return names() + ", " + st
		}
		return names()
	}
	return core.View{TestID: "sidebar-computers", Role: core.RoleButton, LabelFunc: label, OnPress: func(core.PressEvent) { open() },
		Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10, Padding: core.All(12), CornerRadius: 14, BorderCurve: core.BorderCurveContinuous},
		StyleFunc: func() core.Style {
			return core.Style{Background: colors.Background(), Active: &core.Style{Background: colors.Pressed()}}
		},
		Children: []core.Node{
			look.Laptop(40),
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 1}, Children: []core.Node{
				core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6}, Children: []core.Node{
					core.Text{ContentFunc: names, MaxLines: 1, Style: core.Style{FontSize: 14, LineHeight: 18, FontWeight: core.FontWeightBold, FlexShrink: 1}, StyleFunc: ui.Colored(colors.Text)},
					core.Show{When: func() bool { return len(s.Desktops()) > 0 }, Then: func(cx *core.Scope) core.Node {
						return core.View{TestID: "sidebar-computers-dot", Style: core.Style{Width: core.Pt(7), Height: core.Pt(7), CornerRadius: 3.5, FlexShrink: 0},
							StyleFunc: func() core.Style {
								if online() {
									return core.Style{Background: theme.StatusOK}
								}
								return core.Style{Background: theme.Offline}
							}}
					}},
				}},
				core.Show{When: func() bool { return status() != "" }, Then: func(cx *core.Scope) core.Node {
					return core.Text{TestID: "sidebar-computers-status", ContentFunc: status, MaxLines: 1, Style: look.Footnote, StyleFunc: ui.Colored(colors.Muted)}
				}},
			}},
		}}
}

// thisDevice heads the section of what is set on this device alone: on Android, which the design names by its kind, This Phone or This Tablet; elsewhere This Device.
func thisDevice(cx *core.Scope) string {
	switch {
	case cx.Env().OS() != core.OSAndroid:
		return locales.SettingsThisDevice(cx)
	case look.Tablet(cx):
		return locales.SettingsThisTablet(cx)
	}
	return locales.SettingsThisPhone(cx)
}
