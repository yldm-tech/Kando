// Package settings is the Settings tab: the computers the phone is paired with, each with its page of the AI service it translates with and its context switch, and each of which it can unpair, and the way to pair another; the language subtitles are translated into, which the computers translate into too, and how the player draws them; the phone's own daily goal and review reminder, its widgets on iOS (widgets.go) and a tablet's keyboard shortcuts (shortcuts.go); the sync conflicts to settle (conflicts.go); and where Kando's source is, with the introduction to see again. It has the page the phone shows once a computer no longer takes it too (revoked.go).
package settings

import (
	"strings"

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

// Actions are where the tab leads; Computer opens computer id's page, and Unpaired runs once the user unpaired computer id, for the pages of it to go. Shortcuts opens the keyboard shortcuts, which a tablet lists, and Widgets the widgets, which iOS has.
type Actions struct {
	Pair, Target, Style, Goal, Conflicts, Introduction func()
	Shortcuts, Widgets                                 func()
	Computer, Unpaired                                 func(id string)
}

// Page is the tab's root page.
func Page(cx *core.Scope, s *session.Session, act Actions) base.NavPage {
	title := func() string { return locales.SettingsTitle(cx) }
	ref := &core.Ref{}
	pairRow := func() int { return len(s.Desktops()) }
	return look.TabRoot(cx, base.NavPage{TestID: "settings-page", Children: []core.Node{
		core.Scroll{TestID: "settings-scroll", Ref: ref, Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
			core.View{StyleFunc: look.Column(cx), Children: []core.Node{
				section(cx, func() string { return locales.SettingsComputers(cx) },
					core.Each(s.Desktops, func(d store.Desktop) string { return d.ID }, func(cx *core.Scope, d func() store.Desktop, i func() int) core.Node {
						return computer(cx, s, d, i, act.Computer, act.Unpaired)
					}),
					look.Row{TestID: "settings-pair", Index: pairRow, TitleTone: look.ToneAccent,
						Leading: look.IconTile(cx, lucide.Plus, look.ToneAccent),
						Title: func() string {
							if len(s.Desktops()) > 0 {
								return locales.SettingsPairAnother(cx)
							}
							return locales.SettingsPair(cx)
						},
						OnPress: act.Pair}),
				section(cx, func() string { return locales.SettingsSubtitles(cx) },
					look.Row{TestID: "settings-target", Chevron: true,
						Leading: look.IconTile(cx, lucide.Languages, look.ToneAccent),
						Title:   func() string { return locales.SettingsTarget(cx) },
						Value:   func() string { return langs.TargetName(s.Target()) },
						OnPress: act.Target},
					look.Row{TestID: "settings-style", Index: func() int { return 1 }, Chevron: true,
						Leading: look.IconTile(cx, lucide.Type, look.ToneAccent),
						Title:   func() string { return locales.SettingsSubtitleStyle(cx) },
						Value: func() string {
							st := s.Prefs().Subtitles
							return sizeName(cx, st.Size) + " · " + colorName(cx, st.Color)
						},
						OnPress: act.Style}),
				section(cx, func() string { return locales.SettingsThisDevice(cx) }, deviceRows(cx, s, act)...),
				section(cx, func() string { return locales.SettingsSync(cx) },
					look.Row{TestID: "settings-conflicts", Chevron: true,
						Leading: look.IconTile(cx, lucide.ArrowLeftRight, look.ToneWarn),
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
				section(cx, func() string { return locales.SettingsAbout(cx) },
					look.Row{TestID: "settings-source",
						Leading: look.IconTile(cx, lucide.Code, look.ToneAccent),
						Title:   func() string { return locales.SettingsSource(cx) },
						Value:   func() string { return "GitHub" },
						OnPress: func() { cx.OpenURL(onboarding.DownloadURL, nil) }},
					look.Row{TestID: "settings-introduction", Index: func() int { return 1 },
						Leading: look.IconTile(cx, lucide.Sparkles, look.ToneAccent),
						Title:   func() string { return locales.SettingsIntroduction(cx) },
						OnPress: act.Introduction}),
			}},
		}},
	}}, title, nil, nil)
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

// deviceRows are the rows of what this phone or tablet does on its own: its daily goal, review reminder and appearance, its widgets on iOS, which alone has them, and the keyboard shortcuts on a tablet, which is the one used with a hardware keyboard.
func deviceRows(cx *core.Scope, s *session.Session, act Actions) []core.Node {
	rows := []core.Node{
		look.Row{TestID: "settings-goal", Chevron: true,
			Leading: look.IconTile(cx, lucide.Target, look.ToneAccent),
			Title:   func() string { return locales.SettingsGoal(cx) },
			Value:   func() string { return locales.SettingsGoalValue(cx, s.Goal()) },
			OnPress: act.Goal},
		look.Row{TestID: "settings-remind", Index: func() int { return 1 }, TitleLines: 1,
			Leading: look.IconTile(cx, lucide.Bell, look.ToneAccent),
			Title:   func() string { return locales.SettingsRemind(cx) },
			Detail:  func() string { return locales.SettingsRemindDetail(cx, learning.ReminderHour) },
			Trailing: ui.Switch{TestID: "settings-remind-switch", Label: locales.SettingsRemind(cx), Variant: ui.SwitchSystem,
				Checked: func() bool { return s.Prefs().Remind }, OnChange: s.SetRemind}},
		look.Row{TestID: "settings-appearance", Index: func() int { return 2 }, Chevron: true,
			Leading: look.ColorTile(lucide.Contrast, theme.TileInk),
			Title:   func() string { return locales.SettingsAppearance(cx) },
			Value:   func() string { return appearanceName(cx, s.Prefs().Appearance) },
			OnPress: func() { s.SetAppearance(session.NextAppearance(s.Prefs().Appearance)) }},
	}
	if cx.Env().OS() == core.OSIOS && act.Widgets != nil {
		i := len(rows)
		rows = append(rows, look.Row{TestID: "settings-widgets", Index: func() int { return i }, Chevron: true,
			Leading: look.IconTile(cx, lucide.LayoutGrid, look.ToneAccent),
			Title:   func() string { return locales.SettingsWidgets(cx) },
			OnPress: act.Widgets})
	}
	if look.Tablet(cx) && act.Shortcuts != nil {
		i := len(rows)
		rows = append(rows, look.Row{TestID: "settings-shortcuts", Index: func() int { return i }, Chevron: true,
			Leading: look.IconTile(cx, lucide.Keyboard, look.ToneAccent),
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

// computer is a paired computer's row: its name and address, which opens its page, with its options, which unpair it; unpaired runs once they did.
func computer(cx *core.Scope, s *session.Session, d func() store.Desktop, i func() int, open func(id string), unpaired func(id string)) core.Node {
	return look.Row{TestID: "settings-computer-" + d().ID, Index: i,
		Leading: look.IconTile(cx, lucide.Laptop, look.ToneAccent),
		Title:   func() string { return d().Name },
		Detail:  func() string { return d().Addr.String() },
		OnPress: func() { open(d().ID) },
		Trailing: ui.IconButton{TestID: "settings-computer-options-" + d().ID, Icon: lucide.Ellipsis, Variant: ui.IconButtonGhost, Size: 44, IconSize: 20,
			LabelFunc: func() string { return locales.ComputersOptions(cx, d().Name) }, OnPress: func() { confirmUnpair(cx, s, d().ID, d().Name, unpaired) }}}
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
	return ui.NavPage(cx, base.NavPage{TitleFunc: func() string { return locales.SettingsTarget(cx) }, TestID: "settings-target-page", Children: []core.Node{
		core.Scroll{Ref: ref, Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
			core.View{StyleFunc: look.Column(cx), Children: []core.Node{
				core.View{Style: core.Style{Gap: 8}, Children: []core.Node{
					core.View{Role: core.RoleRadioGroup, LabelFunc: func() string { return locales.SettingsTarget(cx) }, Children: []core.Node{look.Group{Children: rows}}},
					core.Text{ContentFunc: func() string { return locales.SettingsTargetNote(cx) }, Style: look.Caption.Merge(core.Style{Padding: core.Edges{Right: 16, Left: 16}}), StyleFunc: ui.Colored(colors.Muted)},
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
	// online is whether every paired computer answered its last read.
	online := func() bool {
		ds := s.Desktops()
		for _, d := range ds {
			if s.Shelf(d.ID).Err != nil {
				return false
			}
		}
		return len(ds) > 0
	}
	status := func() string {
		active, failed := s.Unfinished()
		switch {
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
