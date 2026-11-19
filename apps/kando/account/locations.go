package account

import (
	"slices"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"
	"kando/apps/kando/storage"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Locations is what the 存储位置 card reads and does: the folders on a NAS or a server films can be kept in, their dialog and the questions asked over the page. App embeds it.
type Locations interface {
	// Locations are the locations saved, in the order they were added.
	Locations() []Location
	// LocationStatus is how the last Test of the location id went; id "" is the dialog's draft.
	LocationStatus(id string) Status
	// TestLocation connects to the location id, or with id "" to the dialog's draft, off the engine's goroutine, and reports in LocationStatus; an SFTP server whose key is not trusted asks first (LocationAlert).
	TestLocation(id string)
	AddLocation()
	EditLocation(id string)
	// AskRemoveLocation asks whether to remove the location id (AlertRemove).
	AskRemoveLocation(id string)
	// LocationEditing is whether the dialog is open, LocationIsNew whether it adds a location rather than edits one.
	LocationEditing() bool
	LocationIsNew() bool
	// LocationDraft is the dialog's location as typed, which its fields change in place.
	LocationDraft() *Draft
	// LocationProblem is what keeps the draft from being saved, in the interface's language, "" when nothing does or Save was not pressed yet.
	LocationProblem() string
	SaveLocation()
	CloseLocation()
	// LocationAlert is the question open over the page, Kind AlertNone when none is.
	LocationAlert() Alert
	// AnswerLocationAlert answers it: yes removes the location, trusts the key, or replaces the key; no leaves everything as it was.
	AnswerLocationAlert(yes bool)
}

// locationsSection is the 存储位置 card under its label, its footnote, and the location dialog and the alerts it opens over the page.
func locationsSection(cx *core.Scope, a App) core.Node {
	return core.Group{
		label(func() string { return locales.AccountSectionLocations(cx) }, 12),
		card("account-locations", locationRows(cx, a), addLocationRow{a}),
		note(func() string { return locales.AccountLocationsNote(cx) }, 0),
		locationDialog{a},
		locationAlert{a},
	}
}

// locationRows are the card's rows, one per location: its name over where it is, Test, Edit and Remove, and under them how the last Test went.
func locationRows(cx *core.Scope, a App) core.Node {
	list := core.NewMemoFunc(cx, live.Func(a, a.Locations), slices.Equal[[]Location])
	return core.Each(list.Get, func(l Location) string { return l.ID }, func(cx *core.Scope, l func() Location, index func() int) core.Node {
		id := l().ID
		name := func() string { return l().Name }
		summary := func() string { return locales.AccountLocationSummary(cx, KindName(l().Remote.Kind), Place(l().Remote)) }
		status := core.NewMemo(cx, live.Func(a, func() Status { return a.LocationStatus(id) }))
		busy := func() bool { return status.Get().Busy }
		buttons := core.Group{
			form.Button{TestID: "location-test-" + id, Label: locales.AccountLocationTestLabel(cx, l().Name), Disabled: busy,
				TextFunc: func() string { return locales.AccountLocationTest(cx) }, OnPress: live.Do(a, func() { a.TestLocation(id) })},
			form.Button{TestID: "location-edit-" + id, Label: locales.AccountLocationEditLabel(cx, l().Name),
				TextFunc: func() string { return locales.AccountLocationEdit(cx) }, OnPress: live.Do(a, func() { a.EditLocation(id) })},
			form.Button{TestID: "location-remove-" + id, Label: locales.AccountLocationRemoveLabel(cx, l().Name),
				TextFunc: func() string { return locales.AccountLocationRemove(cx) }, OnPress: live.Do(a, func() { a.AskRemoveLocation(id) })},
		}
		return core.View{TestID: "location-" + id, Style: core.Style{FlexShrink: 0, AlignItems: core.AlignStretch}, Children: []core.Node{
			core.Show{When: func() bool { return index() > 0 }, Then: func(*core.Scope) core.Node { return separator() }},
			row(name, summary, buttons),
			statusLine(cx, "location-status-"+id, status.Get, core.Edges{Right: 14, Bottom: 8, Left: 14}),
		}}
	})
}

// statusLine is how a Test went, under its row or in the dialog: muted while it runs, the accent once connected, the danger color when it failed; nothing before the first Test.
func statusLine(cx *core.Scope, test string, status func() Status, pad core.Edges) core.Node {
	colors := ui.ColorsOf(cx)
	text := func() string {
		switch s := status(); {
		case s.Busy:
			return locales.AccountLocationTesting(cx)
		case s.OK:
			return locales.AccountLocationConnected(cx)
		default:
			return s.Err
		}
	}
	color := func() core.Color {
		switch s := status(); {
		case s.Busy:
			return colors.Muted()
		case s.OK:
			return colors.Accent()
		default:
			return colors.Danger()
		}
	}
	return core.Show{When: func() bool { return text() != "" }, Then: func(*core.Scope) core.Node {
		return core.Text{TestID: test, ContentFunc: text, Selectable: core.SelectableOn, Style: core.Style{FontSize: 11, LineHeight: 15, Padding: pad},
			StyleFunc: func() core.Style { return core.Style{Color: color()} }}
	}}
}

// addLocationRow is the card's last row: that there is no location yet while there is none, and 添加… at its end.
type addLocationRow struct{ a App }

// Render implements core.Node.
func (r addLocationRow) Render(cx *core.Scope) core.Node {
	a := r.a
	empty := live.Memo(cx, a, func() bool { return len(a.Locations()) == 0 })
	title := func() string {
		if empty() {
			return locales.AccountLocationsEmpty(cx)
		}
		return ""
	}
	add := form.Button{TestID: "location-add", TextFunc: func() string { return locales.AccountLocationAdd(cx) }, OnPress: live.Do(a, a.AddLocation)}
	return core.Group{
		core.Show{When: func() bool { return !empty() }, Then: func(*core.Scope) core.Node { return separator() }},
		row(title, none, add),
	}
}

// locationDialog edits the draft of a location: its name and kind, then the fields its kind uses, built again when another kind is chosen; Test tries the draft as it is, Save keeps it.
type locationDialog struct{ a App }

// Render implements core.Node.
func (d locationDialog) Render(cx *core.Scope) core.Node {
	a := d.a
	return core.Show{When: live.Func(a, a.LocationEditing), Then: func(cx *core.Scope) core.Node {
		colors := ui.ColorsOf(cx)
		kind := live.Memo(cx, a, func() string { return a.LocationDraft().Kind })
		status := core.NewMemo(cx, live.Func(a, func() Status { return a.LocationStatus("") }))
		problem := live.Func(a, a.LocationProblem)
		closeDialog := live.Do(a, a.CloseLocation)
		fields := core.Dynamic(func(cx *core.Scope) core.Node { return draftFields(cx, a, storage.Kind(kind())) })
		test := form.Button{TestID: "location-dialog-test", Disabled: func() bool { return status.Get().Busy },
			TextFunc: func() string { return locales.AccountLocationTest(cx) }, OnPress: live.Do(a, func() { a.TestLocation("") })}
		// The buttons' words are set as they are built, so they are built again for another language.
		footer := core.Dynamic(func(cx *core.Scope) core.Node {
			// TODO(nsgo#145): give ui.Button a TextFunc twin so its text follows the interface language without building the button again, then drop this Dynamic https://github.com/nsgo-dev/nsgo/issues/145
			return core.View{Style: core.Style{Direction: core.Row, Gap: 8}, Children: []core.Node{
				ui.Button{Text: locales.AccountLocationCancel(cx), Variant: ui.ButtonSecondary, TestID: "location-dialog-cancel", OnPress: closeDialog},
				ui.Button{Text: locales.AccountLocationSave(cx), TestID: "location-dialog-save", OnPress: live.Do(a, a.SaveLocation)},
			}}
		})
		return ui.Dialog{
			TestID: "location-dialog",
			Open:   func() bool { return true },
			TitleFunc: live.Func(a, func() string {
				if a.LocationIsNew() {
					return locales.AccountLocationAddTitle(cx)
				}
				return locales.AccountLocationEditTitle(cx)
			}),
			NoCloseOnBackdropPress: true,
			OnOpenChange: func(open bool, _ base.Reason) {
				if !open {
					closeDialog()
				}
			},
			Style: core.Style{Width: core.Pt(560)},
			Children: []core.Node{
				card("location-fields", fields),
				core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10}, Children: []core.Node{
					test,
					core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, Children: []core.Node{
						core.Show{When: func() bool { return problem() != "" }, Then: func(*core.Scope) core.Node {
							return core.Text{TestID: "location-dialog-problem", ContentFunc: problem, Style: core.Style{FontSize: 11, LineHeight: 15}, StyleFunc: ui.Colored(colors.Danger)}
						}},
						core.Show{When: func() bool { return problem() == "" }, Then: func(cx *core.Scope) core.Node {
							return statusLine(cx, "location-dialog-status", status.Get, core.Edges{})
						}},
					}},
				}},
			},
			Footer: []core.Node{footer},
		}
	}}
}

// draftFields are the dialog's rows for a location of kind: the name and the kind, then what the kind is reached with.
func draftFields(cx *core.Scope, a App, kind storage.Kind) core.Node {
	text := func(message func(*core.Scope) string) func() string { return func() string { return message(cx) } }
	str := func(get func(*Draft) *string) func() *string { return func() *string { return get(a.LocationDraft()) } }
	port := func() string { return locales.AccountLocationPortHint(cx, defaultPort(kind)) }
	rows := []core.Node{
		draftRow(a, "location-name", text(locales.AccountLocationName), none, str(func(d *Draft) *string { return &d.Name }), draftPlain, text(locales.AccountLocationNamePlaceholder)),
		separator(),
		row(text(locales.AccountLocationKind), none, kindSelect(cx, a)),
	}
	add := func(test string, title, hint func() string, value func() *string, face draftFace) {
		rows = append(rows, separator(), draftRow(a, test, title, hint, value, face, none))
	}
	switch kind {
	case storage.KindWebDAV:
		add("location-url", text(locales.AccountLocationURL), none, str(func(d *Draft) *string { return &d.URL }), draftMono)
		add("location-root", text(locales.AccountLocationRoot), text(locales.AccountLocationRootHintWebDAV), str(func(d *Draft) *string { return &d.Root }), draftMono)
		add("location-user", text(locales.AccountLocationUser), none, str(func(d *Draft) *string { return &d.User }), draftMono)
		add("location-password", text(locales.AccountLocationPassword), none, str(func(d *Draft) *string { return &d.Password }), draftSecret)
	case storage.KindSFTP:
		add("location-host", text(locales.AccountLocationHost), none, str(func(d *Draft) *string { return &d.Host }), draftMono)
		add("location-port", text(locales.AccountLocationPort), port, str(func(d *Draft) *string { return &d.Port }), draftMono)
		add("location-root", text(locales.AccountLocationRoot), text(locales.AccountLocationRootHintSFTP), str(func(d *Draft) *string { return &d.Root }), draftMono)
		add("location-user", text(locales.AccountLocationUser), none, str(func(d *Draft) *string { return &d.User }), draftMono)
		add("location-password", text(locales.AccountLocationPassword), none, str(func(d *Draft) *string { return &d.Password }), draftSecret)
		add("location-key", text(locales.AccountLocationPrivateKey), text(locales.AccountLocationPrivateKeyHint), str(func(d *Draft) *string { return &d.PrivateKey }), draftKey)
		add("location-passphrase", text(locales.AccountLocationPassphrase), none, str(func(d *Draft) *string { return &d.Passphrase }), draftSecret)
		rows = append(rows, separator(), hostKeyRow(cx, a))
	default:
		add("location-host", text(locales.AccountLocationHost), none, str(func(d *Draft) *string { return &d.Host }), draftMono)
		add("location-share", text(locales.AccountLocationShare), none, str(func(d *Draft) *string { return &d.Share }), draftMono)
		add("location-root", text(locales.AccountLocationRoot), text(locales.AccountLocationRootHintSMB), str(func(d *Draft) *string { return &d.Root }), draftMono)
		add("location-user", text(locales.AccountLocationUser), text(locales.AccountLocationUserHintSMB), str(func(d *Draft) *string { return &d.User }), draftMono)
		add("location-password", text(locales.AccountLocationPassword), none, str(func(d *Draft) *string { return &d.Password }), draftSecret)
		add("location-domain", text(locales.AccountLocationDomain), text(locales.AccountLocationDomainHint), str(func(d *Draft) *string { return &d.Domain }), draftMono)
		add("location-port", text(locales.AccountLocationPort), port, str(func(d *Draft) *string { return &d.Port }), draftMono)
	}
	return core.Group(rows)
}

// draftFace is how a draft's field shows what is typed.
type draftFace int

const (
	draftPlain draftFace = iota
	// draftMono is an address, a path or a user, in a monospaced face.
	draftMono
	// draftSecret is a password, hidden on screen.
	draftSecret
	// draftKey is a private key's PEM text, several lines in a small monospaced face.
	draftKey
)

// draftRow edits one of the draft's fields in the account page's column of fields. The draft is kept only by Save.
func draftRow(a App, test string, title, hint func() string, value func() *string, face draftFace, placeholder func() string) core.Node {
	in := base.Input{
		TestID:          test,
		Label:           title(),
		PlaceholderFunc: placeholder,
		Value:           live.Func(a, func() string { return *value() }),
		OnChange:        live.On(a, func(text string) { *value() = text }),
		NoAutoCorrect:   true,
		AutoCapitalize:  core.AutoCapitalizeNone,
	}
	style := core.Style{FontSize: 13}
	switch face {
	case draftMono:
		style = core.Style{FontFamily: ui.Mono, FontSize: 12}
	case draftSecret:
		in.Masked = true
	case draftKey:
		in.Multiline = true
		style = core.Style{FontFamily: ui.Mono, FontSize: 10, Height: core.Pt(64), Padding: core.Symmetric(4, 10)}
	}
	return row(title, hint, field{Input: in, Face: style})
}

// kindSelect chooses the draft's protocol.
func kindSelect(cx *core.Scope, a App) core.Node {
	kind := live.Func(a, func() string { return a.LocationDraft().Kind })
	// The drop-down's name is set as it is built, so it is built again for another language.
	return core.Dynamic(func(cx *core.Scope) core.Node {
		// TODO(nsgo#145): give ui.Select a LabelFunc twin for its name so it follows the interface language without building the drop-down again, then drop this Dynamic https://github.com/nsgo-dev/nsgo/issues/145
		options := make([]ui.Option[string], len(Kinds))
		for i, k := range Kinds {
			options[i] = ui.Option[string]{Value: string(k), Label: KindName(k)}
		}
		return ui.Select[string]{TestID: "location-kind", Label: locales.AccountLocationKind(cx), Options: options, Value: kind,
			OnChange: live.On(a, func(k string) { a.LocationDraft().Kind = k })}
	})
}

// hostKeyRow is the fingerprint of the SFTP server's key the draft trusts, or that the first Test asks about it. The key changes only through the questions Test asks (LocationAlert).
func hostKeyRow(cx *core.Scope, a App) core.Node {
	trusted := live.Func(a, func() string { return storage.Fingerprint(a.LocationDraft().HostKey) })
	hint := func() string {
		if trusted() != "" {
			return locales.AccountLocationHostKeyHint(cx)
		}
		return ""
	}
	value := func() string {
		if fp := trusted(); fp != "" {
			return fp
		}
		return locales.AccountLocationHostKeyNone(cx)
	}
	colors := ui.ColorsOf(cx)
	shown := core.Text{TestID: "location-hostkey", ContentFunc: value, Selectable: core.SelectableOn, MaxLines: 1, EllipsizeMode: core.EllipsizeMiddle,
		Style: core.Style{FontFamily: ui.Mono, FontSize: 11, Width: core.Pt(300)}, StyleFunc: ui.Colored(colors.Muted)}
	return row(func() string { return locales.AccountLocationHostKey(cx) }, hint, shown)
}

// locationAlert is the question open over the page about a location: removing it, trusting an SFTP server's key at the first connection, or a key that changed. Cancel is the default everywhere, so Return never trusts a key nor removes anything; a key that changed is replaced only by the destructive Replace Key.
type locationAlert struct{ a App }

// Render implements core.Node.
func (d locationAlert) Render(cx *core.Scope) core.Node {
	a := d.a
	alert := core.NewMemo(cx, live.Func(a, a.LocationAlert))
	yes := live.Do(a, func() { a.AnswerLocationAlert(true) })
	no := live.Do(a, func() { a.AnswerLocationAlert(false) })
	return core.Show{When: func() bool { return alert.Get().Kind != AlertNone }, Then: func(cx *core.Scope) core.Node {
		// The actions' words are set as they are built, so the alert is built again for another language, and for another question.
		return core.Dynamic(func(cx *core.Scope) core.Node {
			// TODO(nsgo#145): give ui.AlertAction a TextFunc twin so the actions follow the interface language without building the alert again, then drop this Dynamic https://github.com/nsgo-dev/nsgo/issues/145
			colors := ui.ColorsOf(cx)
			al := alert.Get()
			cancel := ui.AlertAction{Text: locales.AccountLocationCancel(cx), Variant: ui.ButtonSecondary, Role: core.AlertCancel, Default: true, OnPress: no, TestID: "location-alert-cancel"}
			// Escape runs Cancel, the action of role core.AlertCancel.
			dialog := ui.AlertDialog{Open: func() bool { return true }}
			switch al.Kind {
			case AlertRemove:
				dialog.TestID = "location-remove-confirm"
				dialog.TitleFunc = func() string { return locales.AccountLocationRemoveTitle(cx, alert.Get().Name) }
				dialog.DescriptionFunc = func() string { return locales.AccountLocationRemoveMessage(cx) }
				dialog.Actions = []ui.AlertAction{cancel, {Text: locales.AccountLocationRemoveButton(cx), Color: colors.Danger, Role: core.AlertDestructive, OnPress: yes, TestID: "location-alert-remove"}}
			case AlertTrust:
				dialog.TestID = "location-trust"
				dialog.TitleFunc = func() string { return locales.AccountLocationTrustTitle(cx, alert.Get().Host) }
				dialog.DescriptionFunc = func() string { return locales.AccountLocationTrustMessage(cx, alert.Get().Fingerprint) }
				dialog.Actions = []ui.AlertAction{cancel, {Text: locales.AccountLocationTrust(cx), OnPress: yes, TestID: "location-alert-trust"}}
			default:
				dialog.TestID = "location-key-changed"
				dialog.TitleFunc = func() string { return locales.AccountLocationKeyChangedTitle(cx, alert.Get().Host) }
				dialog.DescriptionFunc = func() string {
					return locales.AccountLocationKeyChangedMessage(cx, alert.Get().Trusted, alert.Get().Fingerprint)
				}
				dialog.Actions = []ui.AlertAction{cancel, {Text: locales.AccountLocationReplaceKey(cx), Color: colors.Danger, Role: core.AlertDestructive, OnPress: yes, TestID: "location-alert-replace"}}
			}
			return dialog
		})
	}}
}
