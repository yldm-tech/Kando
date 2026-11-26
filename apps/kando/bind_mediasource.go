package main

import (
	"strings"

	"kando/apps/kando/account"
	"kando/apps/kando/mediasource"
	"kando/apps/kando/settings"
	"kando/apps/kando/sourcebrowser"
	"kando/packages/remotefs"
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

func (a *app) BrowseMediaSource(id string) { a.mediaSourceBrowseFrom(id) }

func (a *app) MediaSourceEditing() bool { return a.sources.editing }

func (a *app) MediaSourceIsNew() bool { return a.sources.editID == "" }

func (a *app) MediaSourceKinds() []settings.MediaSourceKind {
	var out []settings.MediaSourceKind
	for _, k := range a.sources.kinds.Kinds() {
		out = append(out, settings.MediaSourceKind{ID: k.ID, Title: a.mediaSourceKindTitle(k)})
	}
	m := a.messages()
	for _, o := range a.mediaSourceOffered() {
		out = append(out, settings.MediaSourceKind{ID: mediaSourceInstallPrefix + o.ID, Title: m.SettingsSourcesKindOffered(o.Name)})
	}
	return out
}

func (a *app) MediaSourceDraft() *settings.MediaSourceDraft { return &a.sources.draft }

func (a *app) SetMediaSourceKind(kind string) {
	_, known := a.sources.kinds.Kind(kind)
	_, offered := a.mediaSourceOfferOf(kind)
	if (known || offered) && kind != a.sources.draft.Kind {
		a.sources.draft.Kind, a.sources.draft.Values = kind, map[string]string{}
		delete(a.sources.tests, "")
		a.mediaSourceSignInStop()
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
		field := settings.MediaSourceField{Key: f.Key, Type: string(f.Type), Title: f.Title(lang), Multiline: f.Multiline}
		if f.Hint != nil {
			field.Hint = f.Hint(lang)
		}
		switch {
		// A secret of a source saved stays in the secure store while its field is left empty.
		case f.Type == mediasource.FieldSecret && kept:
			field.Placeholder = a.messages().SettingsSourcesSecretKept()
		// The first Test asks whether to trust the server's key.
		case f.Type == mediasource.FieldHostKey:
			field.Placeholder = a.messages().AccountLocationHostKeyNone()
		}
		out = append(out, field)
	}
	return out
}

func (a *app) MediaSourceWarnings() []string { return a.mediaSourceWarnings() }

func (a *app) MediaSourceSignIn() settings.MediaSourceSignIn {
	return a.mediaSourceSignInView(a.messages())
}

func (a *app) StartMediaSourceSignIn() { a.mediaSourceSignInStart() }

func (a *app) ChooseMediaSourceSignInServer(id string) { a.mediaSourceSignInChoose(id) }

func (a *app) MediaSourceProblem() string {
	if a.sources.problem == nil {
		return ""
	}
	return a.sources.problem(a.messages())
}

func (a *app) MediaSourceSaving() bool { return a.sources.saving }

func (a *app) SaveMediaSource() { a.saveMediaSourceDraft() }

// MediaSourceOffer is the plugin the draft's kind offers to install or turn on, its Name "" while the kind is one of source.
func (a *app) MediaSourceOffer() settings.MediaSourceOffer {
	o, ok := a.mediaSourceOfferOf(a.sources.draft.Kind)
	if !ok {
		return settings.MediaSourceOffer{}
	}
	_, installed := a.mediaSourceInstalledPack(o.ID)
	return settings.MediaSourceOffer{Name: o.Name, Installed: installed}
}

func (a *app) InstallMediaSourceOffer() { a.mediaSourceInstallOffered() }

func (a *app) CloseMediaSource() { a.closeMediaSourceDialog() }

func (a *app) MediaSourceHostKey() string {
	return remotefs.Fingerprint(a.sources.draft.Values[mediasource.SFTPHostKey])
}

func (a *app) MediaSourceAlert() account.Alert { return a.mediaSourceAlertView() }

func (a *app) AnswerMediaSourceAlert(yes bool) { a.answerMediaSourceAlert(yes) }

func (a *app) MediaSourceRemoving() settings.MediaSourceRemoval {
	i := a.mediaSourceIndex(a.sources.removing)
	if i < 0 {
		return settings.MediaSourceRemoval{}
	}
	return settings.MediaSourceRemoval{Open: true, Name: a.sources.list[i].Name, Linked: a.mediaSourceLinkedCount(a.sources.removing)}
}

func (a *app) AnswerRemoveMediaSource(yes bool) { a.answerRemoveMediaSource(yes) }

// The 媒体来源 page's reads and writes (sourcebrowser.App). The state and what the actions do are in state_mediasource_browse.go.

func (a *app) MediaBrowseSources() []sourcebrowser.Source {
	var out []sourcebrowser.Source
	for _, s := range a.sources.list {
		out = append(out, sourcebrowser.Source{ID: s.ID, Name: s.Name, Summary: a.mediaSourceSummary(s)})
	}
	return out
}

func (a *app) MediaBrowseSource() string { return a.sources.browse.source }

func (a *app) ChooseMediaBrowseSource(id string) { a.mediaSourceChoose(id) }

func (a *app) MediaBrowseSidebarSource() string {
	if a.page != pageMediaSources {
		return ""
	}
	return a.sources.browse.source
}

func (a *app) ShowMediaBrowseSource(id string) { a.mediaSourceShowSource(id) }

func (a *app) MediaBrowseCrumbs() []string {
	b := a.sources.browse
	i := a.mediaSourceIndex(b.source)
	if i < 0 {
		return nil
	}
	out := []string{a.sources.list[i].Name}
	if b.dir != "" {
		out = append(out, strings.Split(b.dir, "/")...)
	}
	return out
}

func (a *app) OpenMediaBrowseCrumb(index int) {
	parts := a.MediaBrowseCrumbs()
	if index < 0 || index >= len(parts) {
		return
	}
	a.mediaSourceOpenDir(strings.Join(parts[1:index+1], "/"))
}

func (a *app) MediaBrowseEntries() []sourcebrowser.Entry {
	b := a.sources.browse
	share := b.dir == "" && a.mediaSourceServer()
	shown := a.mediaSourceShown()
	in := a.mediaSourceInByFolder()
	out := make([]sourcebrowser.Entry, 0, len(shown))
	for _, e := range shown {
		p := b.entryPath(e.Name)
		_, chosen := b.chosen[p]
		x := sourcebrowser.Entry{Path: p, Name: e.Name, Dir: e.Dir, Share: share && e.Dir, Size: e.Size, Modified: e.Modified, Subtitles: b.subtitled[e.Name], Chosen: chosen || b.chosenDirs[p]}
		if e.Dir {
			x.Imported = in[e.Name]
			x.Followed = a.mediaSourceFollowing(b.source, p) >= 0
		} else {
			x.InLibrary = a.mediaSourceIn(a.mediaSourceRef(p))
		}
		x.Choosable = a.mediaSourceChoosable(e, p)
		out = append(out, x)
	}
	return out
}

func (a *app) MediaBrowseLoading() bool { return a.sources.browse.loading }

func (a *app) MediaBrowseErr() string {
	if a.sources.browse.err == nil {
		return ""
	}
	return a.sources.browse.err(a.messages())
}

func (a *app) RetryMediaBrowse() { a.mediaSourceList() }

func (a *app) EditMediaBrowseSource() { a.mediaSourceEditShown() }

func (a *app) OpenMediaBrowseEntry(path string) { a.mediaSourceOpenEntry(path) }

func (a *app) ToggleMediaBrowseEntry(path string) { a.mediaSourceToggle(path) }

func (a *app) FollowMediaBrowseEntry(path string) { a.mediaSourceToggleFollow(path) }

func (a *app) MediaBrowseChosen() int {
	b := a.sources.browse
	return len(b.chosen) + len(b.chosenDirs)
}

func (a *app) MediaBrowseFoldersChosen() bool { return len(a.sources.browse.chosenDirs) > 0 }

func (a *app) MediaBrowseChoice() (chosen, choosable int) { return a.mediaSourceChoice() }

func (a *app) ToggleMediaBrowseAll() { a.mediaSourceChooseAll(false) }

func (a *app) ExtendMediaBrowseChoice(path string) { a.mediaSourceExtend(path) }

func (a *app) MediaBrowseQuery() string { return a.sources.browse.query }

func (a *app) SetMediaBrowseQuery(q string) { a.sources.browse.query = q }

func (a *app) MediaBrowseListed() int { return len(a.sources.browse.entries) }

func (a *app) MediaBrowseExpanding() bool { return a.sources.browse.expanding }

func (a *app) ImportMediaBrowseChosen() { a.mediaSourceAskImport() }

func (a *app) AskMediaBrowseImportWay() { a.mediaSourceAskImportWay() }

// MediaBrowseImportWay is the way remembered of importing, as the import button says it, "" while none is, when the button opens the dialog.
func (a *app) MediaBrowseImportWay() string {
	switch {
	case !a.sources.rememberImport:
		return ""
	case a.sources.importLink:
		return a.messages().MediaSourceWayLink()
	}
	return a.messages().MediaSourceWayCopy()
}

func (a *app) MediaBrowseNote() string {
	if a.sources.browse.note == nil {
		return ""
	}
	return a.sources.browse.note(a.messages())
}

func (a *app) AddMediaBrowseSource() { a.mediaSourceAddThenBrowse() }

func (a *app) MediaBrowseImport() sourcebrowser.Import {
	b := a.sources.browse
	if b.importing == nil {
		return sourcebrowser.Import{}
	}
	in := sourcebrowser.Import{Open: true, Count: len(b.importing), Link: b.importLink, Remember: b.importRemember}
	for _, size := range b.importing {
		in.Size += size
	}
	if i := a.mediaSourceIndex(b.source); i >= 0 {
		in.Source = a.sources.list[i].Name
	}
	return in
}

func (a *app) ChooseMediaBrowseImportLink(link bool) { a.sources.browse.importLink = link }

func (a *app) RememberMediaBrowseImport(on bool) { a.sources.browse.importRemember = on }

func (a *app) StartMediaBrowseImport() { a.mediaSourceStartImport() }

func (a *app) CancelMediaBrowseImport() { a.mediaSourceCloseImport() }

// LinkedChanged names the source the open film's file changed on since it was indexed (film.Linker), "" while it did not.
func (a *app) LinkedChanged() string {
	c := a.player.linkedChanged
	if c == nil || c.path != a.player.playPath {
		return ""
	}
	l, _ := a.linkedOf(c.path)
	return a.mediaSourceName(l.Source)
}

func (a *app) KeepLinkedChanged() { a.linkedKeepChanged() }

func (a *app) RetryLinked() { a.playerRetryLinked() }

func (a *app) FindLinked() { a.mediaSourceFindVideo() }

func (a *app) RedoLinked() { a.linkedRedo() }
