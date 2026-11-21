package settings

import (
	"maps"
	"slices"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// MediaSourcesApp is what 设置 › 媒体来源 reads and does: the NAS shares and servers Kando browses and imports films from (docs/media-sources.md), the dialog that adds or edits one and the confirmation that removes one. Like PluginsApp it is an App of its own, which the window is given (Window.MediaSources), so App does not grow. What it says is in the interface's language.
type MediaSourcesApp interface {
	live.State

	// MediaSources are the sources added, in the order they were.
	MediaSources() []MediaSource
	// MediaSourceStatus is how the last Test of the source id went; id "" is the dialog's draft.
	MediaSourceStatus(id string) MediaSourceStatus
	// TestMediaSource connects to the source id, or with id "" to the dialog's draft, off the engine's goroutine, lists its root and reports in MediaSourceStatus.
	TestMediaSource(id string)
	AddMediaSource()
	EditMediaSource(id string)
	// AskRemoveMediaSource asks whether to remove the source id (MediaSourceRemoving).
	AskRemoveMediaSource(id string)

	// MediaSourceEditing is whether the dialog is open, MediaSourceIsNew whether it adds a source rather than edits one.
	MediaSourceEditing() bool
	MediaSourceIsNew() bool
	// MediaSourceKinds are the kinds of source the dialog's drop-down offers: the built-in ones, then the plugins'.
	MediaSourceKinds() []MediaSourceKind
	// MediaSourceDraft is the dialog's source as typed, which its fields change in place; SetMediaSourceKind chooses another kind for it.
	MediaSourceDraft() *MediaSourceDraft
	SetMediaSourceKind(kind string)
	// MediaSourceFields are the fields the draft's kind asks for, in its order.
	MediaSourceFields() []MediaSourceField
	// MediaSourceWarnings are what the dialog says under the fields about the draft's kind and settings, as SMB's: it cannot prove which server it reached.
	MediaSourceWarnings() []string
	// MediaSourceProblem is what kept Save from saving the draft, "" when nothing did or Save was not pressed yet; MediaSourceSaving is whether Save is keeping its secrets in the system's secure store.
	MediaSourceProblem() string
	MediaSourceSaving() bool
	SaveMediaSource()
	CloseMediaSource()

	// MediaSourceRemoving is the confirmation open of removing a source, Open false when none is.
	MediaSourceRemoving() MediaSourceRemoval
	// AnswerRemoveMediaSource answers it: yes removes the source and forgets its secrets; no leaves it.
	AnswerRemoveMediaSource(yes bool)
}

// MediaSource is a source as the page lists it: its name, and its kind and where it is (Summary), as "SMB · nas.local/media".
type MediaSource struct {
	ID, Name, Summary string
}

// MediaSourceStatus is how the last Test of a source went: running, connected, or not, with what to say, as the shares a server without a share offers or why the connection failed. The zero status is no test yet.
type MediaSourceStatus struct {
	Busy, OK bool
	Text     string
}

// MediaSourceKind is a kind of source the dialog offers: its ID and its name.
type MediaSourceKind struct {
	ID, Title string
}

// MediaSourceDraft is a source as its dialog edits it: its name, its kind's ID and the values of its fields by key, each as typed, a switch's as "true" or "".
type MediaSourceDraft struct {
	Name   string
	Kind   string
	Values map[string]string
}

// Equal reports whether d and o hold the same, as a memo of the draft tells a change.
func (d MediaSourceDraft) Equal(o MediaSourceDraft) bool {
	return d.Name == o.Name && d.Kind == o.Kind && maps.Equal(d.Values, o.Values)
}

// MediaSourceField is a field of the draft's kind as the dialog draws it: its key, its type (mediasource.FieldType's values: text, number, secret, bool), its label, the line under it and what an empty field shows.
type MediaSourceField struct {
	Key, Type, Title, Hint, Placeholder string
}

// MediaSourceRemoval is the confirmation of removing the source Name: Linked is how many films were indexed from it rather than copied, which keep only their subtitles once it goes.
type MediaSourceRemoval struct {
	Open   bool
	Name   string
	Linked int
}

// MediaSources is the 媒体来源 page: the sources, the dialog and the confirmation it opens over the settings window.
type MediaSources struct{ App MediaSourcesApp }

// Render implements core.Node.
func (p MediaSources) Render(cx *core.Scope) core.Node {
	a := p.App
	return form.Page(
		form.Label(locales.SettingsSourcesLabel(cx), 6),
		form.Card(mediaSourceRows(cx, a), mediaSourceAddRow{a}),
		form.Note(func() string { return locales.SettingsSourcesNote(cx) }, 0),
		mediaSourceDialog{a},
		mediaSourceRemove{a},
	)
}

// mediaSourceRows are the card's rows, one per source: its name over its kind and place, Test, Edit and Remove, and under them how the last Test went.
func mediaSourceRows(cx *core.Scope, a MediaSourcesApp) core.Node {
	list := core.NewMemoFunc(cx, live.Func(a, a.MediaSources), slices.Equal[[]MediaSource])
	return core.Each(list.Get, func(s MediaSource) string { return s.ID }, func(cx *core.Scope, s func() MediaSource, index func() int) core.Node {
		id := s().ID
		status := core.NewMemo(cx, live.Func(a, func() MediaSourceStatus { return a.MediaSourceStatus(id) }))
		buttons := []core.Node{
			form.Button{TestID: "media-source-test-" + id, Label: locales.AccountLocationTestLabel(cx, s().Name), Text: locales.AccountLocationTest(cx),
				Disabled: func() bool { return status.Get().Busy }, OnPress: live.Do(a, func() { a.TestMediaSource(id) })},
			form.Button{TestID: "media-source-edit-" + id, Label: locales.AccountLocationEditLabel(cx, s().Name), Text: locales.AccountLocationEdit(cx),
				OnPress: live.Do(a, func() { a.EditMediaSource(id) })},
			form.Button{TestID: "media-source-remove-" + id, Label: locales.AccountLocationRemoveLabel(cx, s().Name), Text: locales.AccountLocationRemove(cx),
				OnPress: live.Do(a, func() { a.AskRemoveMediaSource(id) })},
		}
		return core.View{TestID: "media-source-" + id, Style: core.Style{FlexShrink: 0, AlignItems: core.AlignStretch}, Children: []core.Node{
			core.Show{When: func() bool { return index() > 0 }, Then: func(*core.Scope) core.Node { return form.Separator(true) }},
			core.View{Style: core.Style{Direction: core.Row, FlexShrink: 0, MinHeight: core.Pt(46), Padding: core.Symmetric(6, 14), Gap: 10, AlignItems: core.AlignCenter},
				Role: core.RoleGroup, LabelFunc: func() string { return s().Name },
				Children: []core.Node{
					mediaSourceTitles(cx, func() string { return s().Name }, func() string { return s().Summary }),
					core.View{Style: core.Style{Direction: core.Row, FlexShrink: 0, AlignItems: core.AlignCenter, Gap: 8}, Children: buttons},
				}},
			mediaSourceStatusLine(cx, "media-source-status-"+id, status.Get, core.Edges{Right: 14, Bottom: 8, Left: 14}),
		}}
	})
}

// mediaSourceTitles is a row's title over its subtitle, both of which change with the app's state, taking the room the row leaves.
func mediaSourceTitles(cx *core.Scope, title, subtitle func() string) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, Children: []core.Node{
		core.Text{ContentFunc: title, MaxLines: 1, Style: core.Style{FontSize: 13, LineHeight: 18}, StyleFunc: ui.Colored(colors.Text)},
		core.Text{ContentFunc: subtitle, MaxLines: 1, EllipsizeMode: core.EllipsizeMiddle, Style: core.Style{FontSize: 11, LineHeight: 16}, StyleFunc: ui.Colored(colors.Muted)},
	}}
}

// mediaSourceStatusLine is how a Test went, under its row or in the dialog: muted while it runs, the accent once connected, the danger color when it failed; nothing before the first Test.
func mediaSourceStatusLine(cx *core.Scope, test string, status func() MediaSourceStatus, pad core.Edges) core.Node {
	colors := ui.ColorsOf(cx)
	text := func() string {
		if s := status(); s.Busy {
			return locales.AccountLocationTesting(cx)
		}
		return status().Text
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

// mediaSourceAddRow is the card's last row: that there is no source yet while there is none, and 添加… at its end.
type mediaSourceAddRow struct{ a MediaSourcesApp }

// Render implements core.Node.
func (r mediaSourceAddRow) Render(cx *core.Scope) core.Node {
	a := r.a
	empty := live.Memo(cx, a, func() bool { return len(a.MediaSources()) == 0 })
	colors := ui.ColorsOf(cx)
	add := form.Button{TestID: "media-source-add", Text: locales.SettingsSourcesAdd(cx), OnPress: live.Do(a, a.AddMediaSource)}
	said := core.Text{TestID: "media-sources-empty", ContentFunc: func() string {
		if empty() {
			return locales.SettingsSourcesEmpty(cx)
		}
		return ""
	}, Style: core.Style{FontSize: 13, LineHeight: 18, FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, StyleFunc: ui.Colored(colors.Text)}
	return core.Group{
		core.Show{When: func() bool { return !empty() }, Then: func(*core.Scope) core.Node { return form.Separator(true) }},
		core.View{Style: core.Style{Direction: core.Row, FlexShrink: 0, MinHeight: core.Pt(40), Padding: core.Symmetric(6, 14), Gap: 10, AlignItems: core.AlignCenter},
			Children: []core.Node{said, add}},
	}
}

// mediaSourceDialogWidth is the dialog's width, room for a label beside a field.
const mediaSourceDialogWidth core.Points = 560

// mediaSourceDialog edits the draft of a source: its name and kind and the fields its kind asks for, built again when another kind is chosen, which scroll, and under them what to know about it, which stays in sight; Test tries the draft as it is, Save keeps it.
type mediaSourceDialog struct{ a MediaSourcesApp }

// Render implements core.Node.
func (d mediaSourceDialog) Render(cx *core.Scope) core.Node {
	a := d.a
	return core.Show{When: live.Func(a, a.MediaSourceEditing), Then: func(cx *core.Scope) core.Node {
		colors := ui.ColorsOf(cx)
		kind := live.Memo(cx, a, func() string { return a.MediaSourceDraft().Kind })
		status := core.NewMemo(cx, live.Func(a, func() MediaSourceStatus { return a.MediaSourceStatus("") }))
		problem := live.Func(a, a.MediaSourceProblem)
		saving := live.Func(a, a.MediaSourceSaving)
		closeDialog := live.Do(a, a.CloseMediaSource)
		fields := core.Dynamic(func(cx *core.Scope) core.Node {
			_ = kind()
			return mediaSourceFields(cx, a)
		})
		warnings := core.NewMemoFunc(cx, live.Func(a, a.MediaSourceWarnings), slices.Equal[[]string])
		notes := core.Each(warnings.Get, func(s string) string { return s }, func(cx *core.Scope, s func() string, _ func() int) core.Node {
			return core.Text{ContentFunc: s, Style: core.Style{FontSize: 11, LineHeight: 15, FlexShrink: 0, Padding: core.Symmetric(0, 12)}, StyleFunc: ui.Colored(colors.Muted)}
		})
		test := form.Button{TestID: "media-source-dialog-test", Text: locales.AccountLocationTest(cx), Disabled: func() bool { return status.Get().Busy },
			OnPress: live.Do(a, func() { a.TestMediaSource("") })}
		said := core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0)}, Children: []core.Node{
			core.Show{When: func() bool { return problem() != "" }, Then: func(*core.Scope) core.Node {
				return core.Text{TestID: "media-source-dialog-problem", ContentFunc: problem, Style: core.Style{FontSize: 11, LineHeight: 15}, StyleFunc: ui.Colored(colors.Danger)}
			}},
			core.Show{When: func() bool { return problem() == "" && saving() }, Then: func(cx *core.Scope) core.Node {
				return core.Text{TestID: "media-source-dialog-saving", Content: locales.SettingsSourcesSaving(cx), Style: core.Style{FontSize: 11, LineHeight: 15}, StyleFunc: ui.Colored(colors.Muted)}
			}},
			core.Show{When: func() bool { return problem() == "" && !saving() }, Then: func(cx *core.Scope) core.Node {
				return mediaSourceStatusLine(cx, "media-source-dialog-status", status.Get, core.Edges{})
			}},
		}}
		title := locales.SettingsSourcesEditTitle(cx)
		if core.Untracked(cx, live.Func(a, a.MediaSourceIsNew)) {
			title = locales.SettingsSourcesAddTitle(cx)
		}
		return ui.Dialog{
			TestID:                 "media-source-dialog",
			Open:                   func() bool { return true },
			Title:                  title,
			NoCloseOnBackdropPress: true,
			OnOpenChange: func(open bool, _ base.Reason) {
				if !open {
					closeDialog()
				}
			},
			Style: core.Style{Width: core.Pt(mediaSourceDialogWidth)},
			// The fields scroll when the window is too short for all of them, so what to know, the test row and the footer stay in the dialog.
			Children: []core.Node{core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinHeight: core.Pt(0), Gap: 10, AlignItems: core.AlignStretch}, Children: []core.Node{
				core.Scroll{TestID: "media-source-dialog-fields", Style: core.Style{FlexShrink: 1, MinHeight: core.Pt(0)}, Children: []core.Node{
					form.Card(fields),
				}},
				core.View{Style: core.Style{Gap: 4, FlexShrink: 0, AlignItems: core.AlignStretch}, Children: []core.Node{notes}},
				core.View{Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 10, FlexShrink: 0}, Children: []core.Node{test, said}},
			}}},
			Footer: []core.Node{
				ui.Button{Text: locales.AccountLocationCancel(cx), Variant: ui.ButtonSecondary, TestID: "media-source-dialog-cancel", OnPress: closeDialog},
				ui.Button{Text: locales.AccountLocationSave(cx), TestID: "media-source-dialog-save", Disabled: saving, OnPress: live.Do(a, a.SaveMediaSource)},
			},
		}
	}}
}

// mediaSourceFields are the dialog's rows: the name and the kind, then the fields of the draft's kind (MediaSourceFields), each a text field, or a switch for a bool.
func mediaSourceFields(cx *core.Scope, a MediaSourcesApp) core.Node {
	rows := []core.Node{
		form.Row{Title: locales.AccountLocationName(cx), NoGroup: true, Trailing: []core.Node{mediaSourceInput(a, "media-source-name", locales.AccountLocationName(cx), locales.AccountLocationNamePlaceholder(cx), false,
			func() string { return a.MediaSourceDraft().Name }, func(s string) { a.MediaSourceDraft().Name = s })}},
		form.Separator(true),
		form.Row{Title: locales.AccountLocationKind(cx), NoGroup: true, Trailing: []core.Node{mediaSourceKindSelect(cx, a)}},
	}
	for _, f := range core.Untracked(cx, live.Func(a, a.MediaSourceFields)) {
		key := f.Key
		value := func() string { return a.MediaSourceDraft().Values[key] }
		set := func(s string) { a.MediaSourceDraft().Values[key] = s }
		rows = append(rows, form.Separator(true))
		if f.Type == "bool" {
			rows = append(rows, form.SwitchRow{Title: f.Title, Subtitle: f.Hint, TestID: "media-source-field-" + key,
				On: live.Func(a, func() bool { return value() == "true" }),
				OnChange: live.On(a, func(on bool) {
					if on {
						set("true")
					} else {
						set("")
					}
				})})
			continue
		}
		rows = append(rows, form.Row{Title: f.Title, Subtitle: f.Hint, NoGroup: true, Trailing: []core.Node{
			mediaSourceInput(a, "media-source-field-"+key, f.Title, f.Placeholder, f.Type == "secret", live.Func(a, value), set),
		}})
	}
	return core.Group(rows)
}

// mediaSourceInput is a field of the dialog, 300 points wide, named label, its value read with value and changed with set; masked hides a secret.
func mediaSourceInput(a MediaSourcesApp, test, label, placeholder string, masked bool, value func() string, set func(string)) core.Node {
	return ui.Input{TestID: test, Label: label, Placeholder: placeholder, Masked: masked, NoAutoCorrect: true, AutoCapitalize: core.AutoCapitalizeNone,
		Value: value, OnChange: live.On(a, set), Style: core.Style{Width: core.Pt(300)}}
}

// mediaSourceKindSelect chooses the draft's kind among those the app knows.
func mediaSourceKindSelect(cx *core.Scope, a MediaSourcesApp) core.Node {
	var options []ui.Option[string]
	for _, k := range core.Untracked(cx, live.Func(a, a.MediaSourceKinds)) {
		options = append(options, ui.Option[string]{Value: k.ID, Label: k.Title})
	}
	return ui.Select[string]{TestID: "media-source-kind", Label: locales.AccountLocationKind(cx), Options: options,
		Value: live.Func(a, func() string { return a.MediaSourceDraft().Kind }), OnChange: live.On(a, a.SetMediaSourceKind), Style: core.Style{Width: core.Pt(300)}}
}

// mediaSourceRemove is the confirmation of removing a source. Cancel is the default, so Return removes nothing.
type mediaSourceRemove struct{ a MediaSourcesApp }

// Render implements core.Node.
func (d mediaSourceRemove) Render(cx *core.Scope) core.Node {
	a := d.a
	removal := core.NewMemo(cx, live.Func(a, a.MediaSourceRemoving))
	yes := live.Do(a, func() { a.AnswerRemoveMediaSource(true) })
	no := live.Do(a, func() { a.AnswerRemoveMediaSource(false) })
	return core.Show{When: func() bool { return removal.Get().Open }, Then: func(cx *core.Scope) core.Node {
		colors := ui.ColorsOf(cx)
		return ui.AlertDialog{
			TestID:          "media-source-remove-confirm",
			Open:            func() bool { return true },
			TitleFunc:       func() string { return locales.AccountLocationRemoveTitle(cx, removal.Get().Name) },
			DescriptionFunc: func() string { return locales.SettingsSourcesRemoveConfirm(cx, removal.Get().Linked) },
			Actions: []ui.AlertAction{
				{Text: locales.AccountLocationCancel(cx), Variant: ui.ButtonSecondary, Role: core.AlertCancel, Default: true, OnPress: no, TestID: "media-source-remove-cancel"},
				{Text: locales.AccountLocationRemoveButton(cx), Color: colors.Danger, Role: core.AlertDestructive, OnPress: yes, TestID: "media-source-remove-ok"},
			},
		}
	}}
}
