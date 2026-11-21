package main

import (
	"strings"

	"kando/apps/kando/mediasource"
	"kando/apps/kando/settings"
)

// The 媒体来源 page's reads and writes (settings.MediaSourcesApp). The page reads them in its bindings and calls the actions from its handlers, which tell the revision (package live), so none of them calls Changed itself; what they say is in the interface's language. The state and what the actions do are in state_mediasource.go.

func (a *app) MediaSources() []settings.MediaSource {
	var out []settings.MediaSource
	for _, s := range a.sources.list {
		out = append(out, settings.MediaSource{ID: s.ID, Name: s.Name, Summary: a.mediaSourceSummary(s)})
	}
	return out
}

func (a *app) MediaSourceStatus(id string) settings.MediaSourceStatus {
	t := a.sources.tests[id]
	s := settings.MediaSourceStatus{Busy: t.busy, OK: t.ok}
	m := a.messages()
	switch {
	case t.err != nil:
		s.Text = t.err(m)
	case t.ok && t.listed && len(t.shares) == 0:
		s.Text = m.SettingsSourcesNoShares()
	case t.ok && t.listed:
		s.Text = m.SettingsSourcesShares(strings.Join(t.shares, ", "))
	case t.ok:
		s.Text = m.AccountLocationConnected()
	}
	return s
}

func (a *app) TestMediaSource(id string) { a.testMediaSource(id) }

func (a *app) AddMediaSource() {
	a.openMediaSourceDialog("", settings.MediaSourceDraft{Kind: mediasource.KindSMB, Values: map[string]string{}})
}

func (a *app) EditMediaSource(id string) {
	if i := a.mediaSourceIndex(id); i >= 0 {
		a.openMediaSourceDialog(id, mediaSourceDraftOf(a.sources.list[i]))
	}
}

func (a *app) AskRemoveMediaSource(id string) { a.askRemoveMediaSource(id) }

func (a *app) MediaSourceEditing() bool { return a.sources.editing }

func (a *app) MediaSourceIsNew() bool { return a.sources.editID == "" }

func (a *app) MediaSourceKinds() []settings.MediaSourceKind {
	var out []settings.MediaSourceKind
	for _, k := range a.sources.kinds.Kinds() {
		out = append(out, settings.MediaSourceKind{ID: k.ID, Title: k.Title(a.prefs.interfaceLanguage)})
	}
	return out
}

func (a *app) MediaSourceDraft() *settings.MediaSourceDraft { return &a.sources.draft }

func (a *app) SetMediaSourceKind(kind string) {
	if _, ok := a.sources.kinds.Kind(kind); ok && kind != a.sources.draft.Kind {
		a.sources.draft.Kind, a.sources.draft.Values = kind, map[string]string{}
		delete(a.sources.tests, "")
	}
}

func (a *app) MediaSourceFields() []settings.MediaSourceField {
	k, ok := a.sources.kinds.Kind(a.sources.draft.Kind)
	if !ok {
		return nil
	}
	lang := a.prefs.interfaceLanguage
	kept := !a.MediaSourceIsNew()
	var out []settings.MediaSourceField
	for _, f := range k.Fields {
		field := settings.MediaSourceField{Key: f.Key, Type: string(f.Type), Title: f.Title(lang)}
		if f.Hint != nil {
			field.Hint = f.Hint(lang)
		}
		// A secret of a source saved stays in the secure store while its field is left empty.
		if f.Type == mediasource.FieldSecret && kept {
			field.Placeholder = a.messages().SettingsSourcesSecretKept()
		}
		out = append(out, field)
	}
	return out
}

func (a *app) MediaSourceWarnings() []string { return a.mediaSourceWarnings() }

func (a *app) MediaSourceProblem() string {
	if a.sources.problem == nil {
		return ""
	}
	return a.sources.problem(a.messages())
}

func (a *app) MediaSourceSaving() bool { return a.sources.saving }

func (a *app) SaveMediaSource() { a.saveMediaSourceDraft() }

func (a *app) CloseMediaSource() { a.closeMediaSourceDialog() }

func (a *app) MediaSourceRemoving() settings.MediaSourceRemoval {
	i := a.mediaSourceIndex(a.sources.removing)
	if i < 0 {
		return settings.MediaSourceRemoval{}
	}
	return settings.MediaSourceRemoval{Open: true, Name: a.sources.list[i].Name}
}

func (a *app) AnswerRemoveMediaSource(yes bool) { a.answerRemoveMediaSource(yes) }
