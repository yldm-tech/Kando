package settings

import (
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"
	"kando/apps/mobile/theme"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// reach is how computer id stands as the phone follows its queue: Online while it answers, here or through the user's storage, Offline while it cannot be reached, else not known yet.
func reach(s *session.Session, id string) session.Reach { return s.Queue(id).Reach }

// processing is how many of computer id's jobs are running or waiting to.
func processing(s *session.Session, id string) int {
	n := 0
	for _, j := range s.Queue(id).Jobs {
		switch j.State {
		case sync.JobRunning, sync.JobWaiting, sync.JobPaused:
			n++
		}
	}
	return n
}

// serviceName is the AI service computer id translates with as its last sync said, with its model; "" before one said it, or from a computer too old to share it.
func serviceName(cx *core.Scope, s *session.Session, id string) string {
	st, ok := s.DesktopSettings(id)
	switch {
	case !ok || st.Context == nil:
		return ""
	case st.Service == "":
		return locales.ComputerServiceNone(cx)
	case st.Model == "":
		return st.Service
	}
	return st.Service + " · " + st.Model
}

// contextRow is the switch of whether computer id's translation sends the lines before a batch with it, which the phone turns on and off there, at index in its group. While the computer takes a change the switch stays where the user put it and takes no presses; a change it could not take says so.
func contextRow(cx *core.Scope, s *session.Session, id func() string, index int) core.Node {
	// pending is the switch's position while the computer takes it.
	pending := core.NewSignalFunc[*bool](cx, nil, nil)
	got := func() (session.DesktopSettings, bool) { return s.DesktopSettings(id()) }
	contextOn := func() bool {
		if p := pending.Get(); p != nil {
			return *p
		}
		st, _ := got()
		return st.Context != nil && *st.Context
	}
	setContext := func(on bool) {
		if pending.GetUntracked() != nil {
			return
		}
		to := core.Untracked(cx, id)
		pending.Set(&on)
		s.SetDesktopContext(to, on, func(err error) {
			pending.Set(nil)
			if err != nil {
				d, _ := s.Desktop(to)
				cx.ShowAlert(core.AlertConfig{Title: locales.ComputerContextFailed(cx, d.Name), Message: locales.ComputerContextFailedMessage(cx),
					Actions: []core.AlertAction{{Label: locales.CommonOK(cx), Role: core.AlertCancel}}}, nil)
			}
		})
	}
	return look.Row{TestID: "computer-context", Index: func() int { return index }, TitleLines: 2,
		Leading: look.ColorTile(cx, lucide.RotateCcw, fixed(theme.TileOrange)),
		Title:   func() string { return locales.ComputerContext(cx) },
		Detail:  func() string { return locales.ComputerContextDetail(cx) },
		Trailing: ui.Switch{TestID: "computer-context-switch", Label: locales.ComputerContext(cx), Variant: ui.SwitchSystem,
			Checked: contextOn, OnChange: setContext,
			Disabled: func() bool {
				st, ok := got()
				return !ok || st.Context == nil || st.ReadOnly || pending.Get() != nil
			}}}
}

// ComputerPage is the page of the computer that processes the links the phone sends (session.SendTo): its card, with whether it answers and how many videos it is processing; the paired computers to send new links to, each of which can be unpaired; how the phone reaches it; the AI service it translates with, which only the computer changes, as it keeps the service's key; the films it lists and how the user's storage serves the phone away from it; and the ways to pair another computer and to unpair this one. What the computer answered at the last sync this run is what it shows; before one reached it, or when its Kando is too old to share them, it says so. unpaired runs once the user unpaired a computer, which takes the page away when it was the one it opened on.
func ComputerPage(cx *core.Scope, s *session.Session, pair func(), unpaired func(id string), back func()) base.NavPage {
	colors := ui.ColorsOf(cx)
	id := s.SendTo
	d := func() store.Desktop { d, _ := s.Desktop(id()); return d }
	note := func(testID string, text func() string) core.Node {
		return core.Text{TestID: testID, ContentFunc: text, Style: core.Style{FontSize: 13, LineHeight: 19, Padding: core.Edges{Right: 16, Left: 16}}, StyleFunc: ui.Colored(colors.Muted)}
	}
	group := func(title func() string, rows ...core.Node) core.Node {
		return core.View{Style: core.Style{Gap: 7}, Children: []core.Node{look.GroupHeader(cx, title), look.Group{Children: rows}}}
	}
	ref := &core.Ref{}
	return ui.NavPage(cx, base.NavPage{TitleFunc: func() string { return locales.SettingsComputer(cx) }, TestID: "computer-page", Children: []core.Node{
		core.Scroll{Ref: ref, Style: core.Style{FlexGrow: 1, FlexShrink: 1}, Children: []core.Node{
			core.View{StyleFunc: column(cx), Children: []core.Node{
				core.Show{When: func() bool { return id() != "" }, Then: func(cx *core.Scope) core.Node {
					return core.View{Style: core.Style{Gap: 23}, Children: []core.Node{
						core.View{Style: core.Style{Gap: 12}, Children: []core.Node{
							computerCard(cx, s, id, d),
							note("computer-note", func() string { return locales.ComputerNote(cx) }),
						}},
						group(func() string { return locales.ComputerSendTo(cx) },
							core.Each(s.Desktops, func(d store.Desktop) string { return d.ID }, func(cx *core.Scope, d func() store.Desktop, i func() int) core.Node {
								return computerChoice(cx, s, d, i, unpaired)
							})),
						// The connection's card: its value beside its name, and the note across the card under them.
						core.View{TestID: "computer-connection", Style: core.Style{Gap: 8, Padding: core.Symmetric(14, 16)}, StyleFunc: func() core.Style { return look.Card(cx) }, Children: []core.Node{
							core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12}, Children: []core.Node{
								core.Text{ContentFunc: func() string { return locales.ComputerConnection(cx) }, MaxLines: 1, Style: look.Body.Merge(core.Style{FlexShrink: 0}), StyleFunc: ui.Colored(colors.Text)},
								core.Text{TestID: "computer-connection-value", ContentFunc: func() string { return connection(cx, s.Queue(id())) }, MaxLines: 2,
									Style: look.Subhead.Merge(core.Style{FlexGrow: 1, FlexShrink: 1, TextAlign: core.TextAlignEnd}), StyleFunc: ui.Colored(colors.Muted)},
							}},
							core.Text{ContentFunc: func() string { return locales.ComputerConnectionNote(cx) }, Style: core.Style{FontSize: 12, LineHeight: 17}, StyleFunc: ui.Colored(colors.Muted)},
						}},
						core.View{Style: core.Style{Gap: 7}, Children: []core.Node{
							look.GroupHeader(cx, func() string { return locales.ComputerOnComputer(cx) }),
							look.Group{Children: []core.Node{
								look.Row{TestID: "computer-service",
									Title: func() string { return locales.ComputerService(cx) },
									Value: func() string { return serviceName(cx, s, id()) }},
							}},
							note("computer-service-note", func() string {
								st, ok := s.DesktopSettings(id())
								switch {
								case !ok:
									return locales.ComputerUnknown(cx)
								case st.Context == nil:
									return locales.ComputerTooOld(cx)
								case st.ReadOnly:
									return locales.ComputerReadOnly(cx)
								}
								return locales.ComputerServiceNote(cx)
							}),
						}},
						computerSync(cx, s, id),
					}}
				}},
				look.Group{Children: []core.Node{
					core.View{TestID: "computer-pair", Role: core.RoleButton, LabelFunc: func() string { return locales.SettingsPairAnother(cx) }, OnPress: func(core.PressEvent) { pair() },
						Style:     core.Style{Height: core.Pt(48), AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Padding: core.Symmetric(0, 16)},
						StyleFunc: func() core.Style { return core.Style{Active: &core.Style{Background: colors.Pressed()}} },
						Children: []core.Node{core.Text{ContentFunc: func() string { return locales.SettingsPairAnother(cx) }, MaxLines: 1,
							Style: core.Style{FontSize: 16, LineHeight: 21, FontWeight: core.FontWeightSemibold}, StyleFunc: ui.Colored(colors.Accent)}}},
				}},
				core.Show{When: func() bool { return id() != "" }, Then: func(cx *core.Scope) core.Node {
					return look.Group{Children: []core.Node{
						look.Row{TestID: "computer-unpair", TitleTone: look.ToneDanger,
							Title:   func() string { return locales.ComputersUnpair(cx) },
							OnPress: func() { confirmUnpair(cx, s, id(), d().Name, unpaired) }},
					}}
				}},
			}},
		}},
	}}, ui.NavHeader{OnBack: back, Scroll: ref})
}

// computerCard is the card of computer id at the top of its page: the drawing of a computer, its name, and whether it answers, with how many videos it is processing.
func computerCard(cx *core.Scope, s *session.Session, id func() string, d func() store.Desktop) core.Node {
	colors := ui.ColorsOf(cx)
	status := func() string {
		switch reach(s, id()) {
		case session.Online:
			return locales.ComputerStatusOnline(cx, processing(s, id()))
		case session.Offline:
			return locales.ComputerOffline(cx)
		}
		return ""
	}
	return core.View{TestID: "computer-card", Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 14, Padding: core.Symmetric(18, 16)},
		StyleFunc: func() core.Style { return look.Card(cx) },
		Children: []core.Node{
			look.Laptop(60),
			core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 3}, Children: []core.Node{
				core.Text{TestID: "computer-name", ContentFunc: func() string { return d().Name }, Role: core.RoleHeader, MaxLines: 2,
					Style: core.Style{FontSize: 17, LineHeight: 22, FontWeight: look.Heavy}, StyleFunc: ui.Colored(colors.Text)},
				core.Show{When: func() bool { return status() != "" }, Then: func(cx *core.Scope) core.Node {
					online := func() bool { return reach(s, id()) == session.Online }
					return core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 6}, Children: []core.Node{
						statusDot(online),
						core.Text{TestID: "computer-status", ContentFunc: status, MaxLines: 2, Style: look.Caption.Merge(core.Style{FlexShrink: 1}),
							StyleFunc: func() core.Style {
								if online() {
									return core.Style{Color: theme.StatGreen}
								}
								return core.Style{Color: colors.Muted()}
							}},
					}}
				}},
			}},
		}}
}

// statusDot is the 8-point dot of a computer that answers, green, or not, grey.
func statusDot(online func() bool) core.Node {
	return core.View{Style: core.Style{Width: core.Pt(8), Height: core.Pt(8), CornerRadius: 4, FlexShrink: 0},
		StyleFunc: func() core.Style {
			if online() {
				return core.Style{Background: theme.StatusOK}
			}
			return core.Style{Background: theme.Offline}
		}}
}

// computerChoice is paired computer d's row among those to send new links to: whether it answers, its name, checked while it is the one they go to, which pressing it makes it, with its options, which unpair it; unpaired runs once they did.
func computerChoice(cx *core.Scope, s *session.Session, d func() store.Desktop, i func() int, unpaired func(id string)) core.Node {
	colors := ui.ColorsOf(cx)
	chosen := func() bool { return s.SendTo() == d().ID }
	return look.Row{TestID: "settings-computer-" + d().ID, Index: i,
		Leading: statusDot(func() bool { return reach(s, d().ID) == session.Online }),
		Title:   func() string { return d().Name },
		Detail: func() string {
			switch reach(s, d().ID) {
			case session.Online:
				return locales.ComputerOnline(cx)
			case session.Offline:
				return locales.ComputerOffline(cx)
			}
			return ""
		},
		Checked: chosen,
		OnPress: func() { s.SetSendTo(d().ID) },
		Trailing: core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 2}, Children: []core.Node{
			core.Show{When: chosen, Then: func(*core.Scope) core.Node { return ui.Icon(lucide.Check, 20, colors.Accent) }},
			ui.IconButton{TestID: "settings-computer-options-" + d().ID, Icon: lucide.Ellipsis, Variant: ui.IconButtonGhost, Size: 44, IconSize: 20,
				LabelFunc: func() string { return locales.ComputersOptions(cx, d().Name) }, OnPress: func() { confirmUnpair(cx, s, d().ID, d().Name, unpaired) }},
		}}}
}

// connection is how the phone reaches a computer whose queue stands as q: directly on its network, through the user's storage away from it, or not at all.
func connection(cx *core.Scope, q session.Queue) string {
	switch {
	case q.Reach == session.Online && q.Relayed:
		return locales.ComputerConnectionRelay(cx)
	case q.Reach == session.Online:
		return locales.ComputerConnectionDirect(cx)
	case q.Reach == session.Offline:
		return locales.ComputerConnectionOffline(cx)
	}
	return ""
}

// computerSync is the Sync group of computer id's page: how many films it lists, once it did, and, while the phone follows it through the user's storage, whether the storage serves the phone.
func computerSync(cx *core.Scope, s *session.Session, id func() string) core.Node {
	colors := ui.ColorsOf(cx)
	films := func() int {
		n := 0
		for _, l := range s.Shelf(id()).Listing.Libraries {
			n += len(l.Films)
		}
		return n
	}
	storage := func() bool { q := s.Queue(id()); return q.Relayed || q.Storage != sync.RelayNoFailure }
	return core.Show{When: func() bool { return s.Shelf(id()).Listed || storage() }, Then: func(cx *core.Scope) core.Node {
		return core.View{Style: core.Style{Gap: 7}, Children: []core.Node{
			look.GroupHeader(cx, func() string { return locales.SettingsSync(cx) }),
			look.Group{Children: []core.Node{
				core.Show{When: func() bool { return s.Shelf(id()).Listed }, Then: func(cx *core.Scope) core.Node {
					return look.Row{TestID: "computer-synced",
						Title: func() string { return locales.ComputerSynced(cx) },
						Value: func() string { return locales.ComputerSyncedCount(cx, films()) }}
				}},
				core.Show{When: storage, Then: func(cx *core.Scope) core.Node {
					return look.Row{TestID: "computer-storage", Index: func() int {
						if s.Shelf(id()).Listed {
							return 1
						}
						return 0
					},
						Title: func() string { return locales.ComputerStorage(cx) },
						Trailing: core.Text{TestID: "computer-storage-value", ContentFunc: func() string { return storageState(cx, s.Queue(id()).Storage) }, MaxLines: 1,
							Style: core.Style{FontSize: 15, LineHeight: 20, FontWeight: core.FontWeightSemibold},
							StyleFunc: func() core.Style {
								if s.Queue(id()).Storage == sync.RelayNoFailure {
									return core.Style{Color: theme.StatGreen}
								}
								return core.Style{Color: theme.Of(cx).WarnText()}
							}}}
				}},
			}},
			core.Text{ContentFunc: func() string { return locales.ComputerStorageNote(cx) }, Style: core.Style{FontSize: 13, LineHeight: 19, Padding: core.Edges{Right: 16, Left: 16}}, StyleFunc: ui.Colored(colors.Muted)},
		}}
	}}
}

// storageState is how the user's storage serves the phone, as its row says it: OK, a key it no longer takes, full, or out of reach.
func storageState(cx *core.Scope, f sync.RelayFailure) string {
	switch f {
	case sync.RelayNoFailure:
		return locales.ComputerStorageOK(cx)
	case sync.RelayFull:
		return locales.ComputerStorageFull(cx)
	case sync.RelayUnreachable, sync.RelayFailed:
		return locales.ComputerStorageUnreachable(cx)
	}
	return locales.ComputerStorageKeys(cx)
}
