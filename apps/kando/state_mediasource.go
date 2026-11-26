package main

import (
	"context"
	"errors"
	"log"
	"maps"
	"slices"
	"strconv"
	"strings"
	"time"

	"kando/apps/kando/account"
	"kando/apps/kando/feedback"
	"kando/apps/kando/locales"
	"kando/apps/kando/mediasource"
	"kando/apps/kando/settings"
	"kando/packages/remotefs"

	ns "github.com/nsgo-dev/nsgo/core"
)

// mediaSourceUI is the state of the media sources (docs/media-sources.md): the kinds Kando knows (kinds, the built-in SMB, SFTP and WebDAV for now), the sources added (list, saved in settings.json without their secrets, which the system's secure store keeps, mediaSourceSecrets), the dialog of 设置 › 媒体来源 that adds or edits one (editing, editID "" for a new one, draft as typed, problem why Save refused it, saving while Save keeps the secrets), how the last Test of each went (tests, by ID, the draft's under ""), the question a Test asks about an SFTP server's key (alert), and the confirmation of removing one (removing, the source's ID). gen numbers the Tests, so the answer of one started before another, or before the dialog closed, is dropped; saveGen numbers the saves the same way, apart from the Tests, so a Test pressed while Save keeps the secrets does not drop the save's answer and leave the dialog saving, and a save of a dialog that closed does not finish in the next one. editRelease, alertRelease and removeRelease give back the modal flags the dialog, the question and the confirmation hold (holdModal). open connects to a source of a kind with its secrets, the kind's own Open unless a test sets another.
type mediaSourceUI struct {
	kinds         *mediasource.Registry
	list          []mediasource.Source
	editing       bool
	editID        string
	draft         settings.MediaSourceDraft
	problem       func(locales.Messages) string
	saving        bool
	tests         map[string]mediaSourceTest
	removing      string
	alert         mediaSourceAlert
	gen           int
	saveGen       int
	editRelease   func()
	alertRelease  func()
	removeRelease func()
	// thenBrowse is set while the dialog adds a source the 媒体来源 page asked for (AddMediaBrowseSource): once it is saved, the settings close and the page shows it.
	thenBrowse bool
	// offers are the official plugins the dialog offers as kinds of source while they are not on (mediaSourceOffers; a test sets its own), and install the one being installed or turned on from it (state_mediasource_install.go).
	offers  []mediaSourceOffer
	install mediaSourceInstall
	// secretsHeld is held by whoever is changing the sources' secrets in the secure store, from a save's first write until settings.json took the save or its secrets were put back as they were (mediaSourceSecretWrite), and by a removal's forgetting, so a save started after another that was abandoned reads the secrets that one put back, and nothing puts back a secret a removal forgot.
	secretsHeld chan struct{}
	open        func(ctx context.Context, k mediasource.Kind, s mediasource.Source, secrets map[string]string) (mediasource.Driver, error)
	// pool keeps the connections the 媒体来源 page lists through and the jobs copy through (mediaSourcePoolSync); browse is that page's state (state_mediasource_browse.go). localHost, when set, judges whether a host is on the local network for offline mode in place of mediasource.Local, as a test's does.
	pool      *mediasource.Pool
	browse    mediaSourceBrowse
	localHost func(ctx context.Context, host string) (bool, error)
	// removed are the sources removed while films were indexed from them, without their secrets (saved too), so one added back at the same address takes its ID back and those films play again (mediaSourceRelinkID). importLink is the way of importing the user asked to remember (mediaSourceImportWays): index and play from the source when set, else copy; rememberImport is whether they asked.
	removed        []mediasource.Source
	importLink     bool
	rememberImport bool
	// copying is the indexed films whose videos are being copied into the library (CopyLinked), by placeholder path, with what stops each.
	copying map[string]context.CancelFunc
	// link is the dialog's sign-in to the account its kind's token field names (state_mediasource_link.go).
	signIn mediaSourceSignIn
	// follows are the folders followed, whose new videos are imported by themselves (state_mediasource_follow.go); followBusy the ones being listed, by mediaSourceFollowKey; followStop stops the timer of the next look.
	follows    []mediasource.Follow
	followBusy map[string]bool
	followStop func() bool
}

// mediaSourceTest is how a Test of a source went: running (gen is its number), connected, with the shares a server without a share offers (shares, listed is whether it listed them), or why not, as a function of the messages so it reads in the language shown.
type mediaSourceTest struct {
	busy   bool
	ok     bool
	gen    int
	listed bool
	shares []string
	err    func(locales.Messages) string
}

// mediaSourceAlert is the question a Test asks about an SFTP server's key, or about the certificate of a plugin's source's server that the system does not trust, as a NAS's own (docs/media-sources.md 17.5): kind (account.AlertTrust at the first connection, account.AlertKeyChanged once the key or the certificate trusted changed), the source it is about (target, "" for the dialog's draft) and the server's error, which holds the key it offered (hk), or the certificate's (cert, of the server host). One question at a time is open, whichever it is about.
type mediaSourceAlert struct {
	kind   account.AlertKind
	target string
	hk     *remotefs.HostKeyError
	cert   *mediasource.CertError
	host   string
}

func newMediaSourceUI() mediaSourceUI {
	kinds := &mediasource.Registry{}
	for _, k := range []mediasource.Kind{mediasource.SMB(), mediasource.SFTP(), mediasource.WebDAV()} {
		if err := kinds.Register(k); err != nil {
			panic(err)
		}
	}
	return mediaSourceUI{kinds: kinds, tests: map[string]mediaSourceTest{}, open: mediaSourceOpen, pool: &mediasource.Pool{}, secretsHeld: make(chan struct{}, 1), offers: mediaSourceOffers}
}

// mediaSourceOpen connects to s, a source of kind k, with its secrets by key, as the kind does.
func mediaSourceOpen(ctx context.Context, k mediasource.Kind, s mediasource.Source, secrets map[string]string) (mediasource.Driver, error) {
	return k.Open(ctx, s, secrets)
}

// mediaSourceTestTimeout bounds a Test: connecting, signing in and listing the root.
const mediaSourceTestTimeout = 30 * time.Second

// mediaSourceSecretsPartition is the partition of the system's secure store the sources' secrets are kept in (core.Scope.ProvideSecrets), each under the account <source ID>.<field key> (mediaSourceSecretAccount).
const mediaSourceSecretsPartition = "mediasource"

// mediaSourceSecretAccount is the account of the secure store a source's secret field key is kept under.
func mediaSourceSecretAccount(id, key string) string { return id + "." + key }

// savedMediaSources is the media sources' part of settings.json: each source's ID, kind, name and the values of its fields that are not secret, and the same of those removed while films were indexed from them (RemovedMediaSources); the way of importing the user asked to remember (MediaSourceImport, mediaSourceImportWays), "" while they did not; and the folders followed of the sources in the settings, with the videos seen in each (MediaSourceFollows, state_mediasource_follow.go). A secret is never here: the system's secure store keeps it (mediaSourceSecrets).
type savedMediaSources struct {
	MediaSources        []savedMediaSource   `json:"mediaSources,omitempty"`
	RemovedMediaSources []savedMediaSource   `json:"removedMediaSources,omitempty"`
	MediaSourceImport   string               `json:"mediaSourceImport,omitempty"`
	MediaSourceFollows  []mediasource.Follow `json:"mediaSourceFollows,omitempty"`
}

// The ways of importing a media source's videos the user can ask Kando to remember (savedMediaSources.MediaSourceImport).
const (
	mediaSourceImportCopy = "copy"
	mediaSourceImportLink = "link"
)

// savedMediaSource is one media source in settings.json.
type savedMediaSource struct {
	ID     string            `json:"id"`
	Kind   string            `json:"kind"`
	Name   string            `json:"name"`
	Config map[string]string `json:"config,omitempty"`
}

// clean drops a source without an ID of NewID's shape, a kind or a name, and the second of two with the same ID, and trims the rest. A built-in kind's source keeps only the values of its fields that are not secret, so a secret written here by hand is not kept; a source of a kind this version does not know, as a plugin's that is not loaded, keeps its values as they are, so the films imported from it keep their source. A removed source is kept alike, but none with the ID of one in the settings; a way of importing Kando does not know is dropped, and so is a folder followed of a source not in the settings, at a path that is not plain (mediasource.CleanPath), or followed twice.
func (s *savedMediaSources) clean() {
	seen := map[string]bool{}
	s.MediaSources = cleanMediaSources(s.MediaSources, seen)
	s.RemovedMediaSources = cleanMediaSources(s.RemovedMediaSources, seen)
	if s.MediaSourceImport != mediaSourceImportCopy && s.MediaSourceImport != mediaSourceImportLink {
		s.MediaSourceImport = ""
	}
	in := map[string]bool{}
	for _, m := range s.MediaSources {
		in[m.ID] = true
	}
	followed := map[string]bool{}
	kept := s.MediaSourceFollows[:0]
	for _, f := range s.MediaSourceFollows {
		dir, err := mediasource.CleanPath(f.Dir)
		key := mediaSourceFollowKey(f.Source, dir)
		if !in[f.Source] || err != nil || dir == "" || followed[key] {
			continue
		}
		followed[key] = true
		f.Dir = dir
		kept = append(kept, f)
	}
	s.MediaSourceFollows = nil
	if len(kept) > 0 {
		s.MediaSourceFollows = kept
	}
}

// cleanMediaSources is list cleaned as clean says, without the IDs seen already, which it adds to seen.
func cleanMediaSources(list []savedMediaSource, seen map[string]bool) []savedMediaSource {
	kept := list[:0]
	for _, m := range list {
		m.ID, m.Kind, m.Name = strings.TrimSpace(m.ID), strings.TrimSpace(m.Kind), strings.TrimSpace(m.Name)
		if !mediasource.ValidID(m.ID) || m.Kind == "" || m.Name == "" || seen[m.ID] {
			continue
		}
		seen[m.ID] = true
		if k, ok := mediasource.Builtin(m.Kind); ok {
			m.Config = mediaSourceConfig(k, m.Config)
		}
		kept = append(kept, m)
	}
	if len(kept) == 0 {
		return nil
	}
	return kept
}

// mediaSourceConfig is values kept to the fields of k that are not secret, each trimmed, without the empty ones, and for a plugin's kind the fingerprint of the certificate trusted for its endpoint (mediasource.PinKey); nil for none.
func mediaSourceConfig(k mediasource.Kind, values map[string]string) map[string]string {
	var out map[string]string
	if pin := values[mediasource.PinKey]; pin != "" && !k.Origin.Builtin() {
		out = map[string]string{mediasource.PinKey: pin}
	}
	for _, f := range k.Fields {
		v := strings.TrimSpace(values[f.Key])
		if f.Type == mediasource.FieldSecret || v == "" {
			continue
		}
		if f.Type == mediasource.FieldBool && v != "true" {
			continue
		}
		if out == nil {
			out = map[string]string{}
		}
		out[f.Key] = v
	}
	return out
}

// saveMediaSources writes the sources, those removed that films are still indexed from (mediaSourceLinkedCount), and the way of importing remembered, to settings.json.
func (a *app) saveMediaSources(saved *savedSettings) {
	out := savedMediaSources{MediaSources: savedMediaSourcesOf(a.sources.list)}
	var removed []mediasource.Source
	for _, s := range a.sources.removed {
		if a.mediaSourceLinkedCount(s.ID) > 0 {
			removed = append(removed, s)
		}
	}
	out.RemovedMediaSources = savedMediaSourcesOf(removed)
	if a.sources.rememberImport {
		out.MediaSourceImport = mediaSourceImportCopy
		if a.sources.importLink {
			out.MediaSourceImport = mediaSourceImportLink
		}
	}
	for _, f := range a.sources.follows {
		if a.mediaSourceIndex(f.Source) >= 0 {
			f.Seen = slices.Clone(f.Seen)
			out.MediaSourceFollows = append(out.MediaSourceFollows, f)
		}
	}
	saved.savedMediaSources = out
}

// savedMediaSourcesOf is list as settings.json keeps it.
func savedMediaSourcesOf(list []mediasource.Source) []savedMediaSource {
	var out []savedMediaSource
	for _, s := range list {
		out = append(out, savedMediaSource{ID: s.ID, Kind: s.Kind, Name: s.Name, Config: maps.Clone(s.Config)})
	}
	return out
}

// mediaSourcesOfSaved is list as the app holds it.
func mediaSourcesOfSaved(list []savedMediaSource) []mediasource.Source {
	var out []mediasource.Source
	for _, s := range list {
		out = append(out, mediasource.Source{ID: s.ID, Kind: s.Kind, Name: s.Name, Config: maps.Clone(s.Config)})
	}
	return out
}

func (a *app) restoreMediaSources(saved savedSettings) {
	a.sources.list = mediaSourcesOfSaved(saved.MediaSources)
	a.sources.removed = mediaSourcesOfSaved(saved.RemovedMediaSources)
	a.sources.rememberImport = saved.MediaSourceImport != ""
	a.sources.importLink = saved.MediaSourceImport == mediaSourceImportLink
	a.sources.follows = slices.Clone(saved.MediaSourceFollows)
}

// mediaSourceSecrets are what of the media sources must not leave the machine, as in the log or a feedback issue: the values of each one's text fields, as its server, share, folder, user and domain, and what the dialog holds in its text and secret fields, its secrets among them. Ports, switches and a server's public key say nothing of where the source is or who signs in. The secrets of the sources saved are in the system's secure store, never in the app, so they cannot leak from here.
func (m mediaSourceUI) mediaSourceSecrets() []string {
	var out []string
	for _, s := range m.list {
		k, ok := m.kinds.Kind(s.Kind)
		if !ok {
			// A kind not registered, as a plugin's that is not loaded, may hold anything in its values.
			for _, key := range slices.Sorted(maps.Keys(s.Config)) {
				out = append(out, s.Config[key])
			}
			continue
		}
		for _, f := range k.Fields {
			if f.Type == mediasource.FieldText || f.Type == mediasource.FieldURL {
				out = append(out, s.Config[f.Key])
			}
			// A plugin's source's address is said by its host alone too, as an error of a connection does.
			if f.Type == mediasource.FieldURL {
				out = append(out, mediasource.Host(s))
			}
		}
	}
	types := map[string]mediasource.FieldType{}
	if k, ok := m.kinds.Kind(m.draft.Kind); ok {
		for _, f := range k.Fields {
			types[f.Key] = f.Type
		}
	}
	for _, key := range slices.Sorted(maps.Keys(m.draft.Values)) {
		switch types[key] {
		case mediasource.FieldNumber, mediasource.FieldBool, mediasource.FieldHostKey:
		default:
			out = append(out, m.draft.Values[key])
		}
	}
	return out
}

// mediaSourceHooks: the sources are saved in settings.json and taken back at startup, and the length of each indexed video, which no file here tells, is known again from library.json as the shelf is read (linkedLengths). Their connections are made with the secure store, which the app's scope gives once the window exists, so the jobs left waiting from the last run copy their videos then, and the folders followed are looked at then and every mediaSourceFollowEvery (mediaSourceFollowLook); they all close as the app quits. After each change, the settings' dialog and confirmation close when the page that draws them went away (mediaSourceModals), and the 媒体来源 page follows a source removed and closes its import dialog once it went away (mediaSourceBrowseFollow).
func (a *app) mediaSourceHooks() areaHooks {
	return areaHooks{
		save:        a.saveMediaSources,
		restore:     a.restoreMediaSources,
		shelfLoaded: a.linkedLengths,
		ready: func() {
			a.mediaSourcePoolSync()
			a.loadExtras()
			a.mediaSourceFollowLook()
		},
		afterChange: func() bool {
			modals := a.mediaSourceModals()
			install := a.mediaSourceInstallFollow()
			return a.mediaSourceBrowseFollow() || modals || install
		},
		quitStarting: a.linkedStopCopies,
		quitting:     a.sources.pool.Close,
	}
}

// mediaSourceModals closes the dialog and the confirmation once the settings closed or show another section, as 媒体来源 draws them, and gives back the modal flags they held. It reports whether it changed anything.
func (a *app) mediaSourceModals() bool {
	m := &a.sources
	changed := false
	if (m.editing || m.removing != "" || m.alert.kind != account.AlertNone) && (!a.prefs.open || a.SettingsSection() != settings.SectionMediaSources) {
		m.editing, m.removing, m.saving, m.alert = false, "", false, mediaSourceAlert{}
		m.saveGen++
		changed = true
	}
	// A question about the draft's server goes with the dialog.
	if m.alert.kind != account.AlertNone && m.alert.target == "" && !m.editing {
		m.alert = mediaSourceAlert{}
		changed = true
	}
	if m.alert.kind == account.AlertNone && m.alertRelease != nil {
		m.alertRelease()
		m.alertRelease = nil
		changed = true
	}
	if !m.editing && m.editRelease != nil {
		m.editRelease()
		m.editRelease = nil
		changed = true
	}
	if m.removing == "" && m.removeRelease != nil {
		m.removeRelease()
		m.removeRelease = nil
		changed = true
	}
	return changed
}

// mediaSourceIndex is where the source id is in the list, -1 when it is not.
func (a *app) mediaSourceIndex(id string) int {
	return slices.IndexFunc(a.sources.list, func(s mediasource.Source) bool { return s.ID == id })
}

// mediaSourceSecretStore is the partition of the system's secure store the sources' secrets are kept in; without the app's scope, as in a test that starts no app, one that fails with core.ErrSecretsUnavailable.
func (a *app) mediaSourceSecretStore() ns.Secrets {
	if a.cx == nil {
		return ns.Secrets{}
	}
	inner := a.cx.Child()
	var secrets ns.Secrets
	inner.Run(func() {
		inner.ProvideSecrets(mediaSourceSecretsPartition)
		secrets = inner.Secrets()
	})
	inner.Dispose()
	return secrets
}

// openMediaSourceDialog opens the dialog on draft, for the source id or, with id "", a new one.
func (a *app) openMediaSourceDialog(id string, draft settings.MediaSourceDraft) {
	m := &a.sources
	m.editing, m.editID, m.draft, m.problem, m.saving, m.thenBrowse = true, id, draft, nil, false, false
	m.saveGen++
	delete(m.tests, "")
	a.mediaSourceSignInStop()
	if m.editRelease == nil {
		m.editRelease = a.holdModal()
	}
}

// closeMediaSourceDialog closes the dialog without saving, forgetting the draft and what it held, and gives back its modal flag.
func (a *app) closeMediaSourceDialog() {
	m := &a.sources
	m.editing, m.draft, m.problem, m.saving, m.thenBrowse = false, settings.MediaSourceDraft{}, nil, false, false
	m.gen++
	m.saveGen++
	delete(m.tests, "")
	a.mediaSourceSignInStop()
	if m.editRelease != nil {
		m.editRelease()
		m.editRelease = nil
	}
}

// mediaSourceDraftOf is the dialog's draft of s: its name, kind and values, a secret field empty, as its secret stays in the secure store until a new one is typed.
func mediaSourceDraftOf(s mediasource.Source) settings.MediaSourceDraft {
	values := maps.Clone(s.Config)
	if values == nil {
		values = map[string]string{}
	}
	return settings.MediaSourceDraft{Name: s.Name, Kind: s.Kind, Values: values}
}

// mediaSourceDraftSource is the source the draft describes: the one edited, or a new one without an ID, and the secrets typed in it, by key.
func (a *app) mediaSourceDraftSource() (mediasource.Source, mediasource.Kind, map[string]string, bool) {
	m := &a.sources
	k, ok := m.kinds.Kind(m.draft.Kind)
	if !ok {
		return mediasource.Source{}, mediasource.Kind{}, nil, false
	}
	s := mediasource.Source{ID: m.editID, Kind: k.ID, Name: strings.TrimSpace(m.draft.Name), Config: mediaSourceConfig(k, m.draft.Values)}
	// The certificate trusted for the source's server is not one to trust for another.
	if i := a.mediaSourceIndex(s.ID); i >= 0 && s.Config[mediasource.PinKey] != "" && mediaSourceOrigin(k, s) != mediaSourceOrigin(k, m.list[i]) {
		delete(s.Config, mediasource.PinKey)
	}
	secrets := map[string]string{}
	for _, key := range k.SecretKeys() {
		if v := m.draft.Values[key]; v != "" {
			secrets[key] = v
		}
	}
	return s, k, secrets, true
}

// mediaSourceProblem is what keeps s, a source of kind k, from being tried or saved with the secrets typed, nil when nothing does. A WebDAV source needs a plain https:// or http:// address, as a storage location does. SMB and SFTP need a server, a port that is a number and a user. An SMB source needs a share of one name, and a share for a folder; its user is guest for a share open to anyone. A new SFTP source needs a password, a private key or both, which one saved keeps in the secure store while its fields are left empty.
func mediaSourceProblem(k mediasource.Kind, s mediasource.Source, typed map[string]string) func(locales.Messages) string {
	if !k.Origin.Builtin() {
		return mediaSourcePluginProblem(s)
	}
	c := s.Config
	switch k.ID {
	case mediasource.KindWebDAV:
		return account.Draft{Kind: string(remotefs.KindWebDAV), URL: c[mediasource.WebDAVURL]}.Problem()
	case mediasource.KindSMB, mediasource.KindSFTP:
	default:
		return nil
	}
	if c[mediasource.SMBHost] == "" {
		return locales.Messages.AccountLocationNeedsHost
	}
	if port := c[mediasource.SMBPort]; port != "" {
		if n, err := strconv.Atoi(port); err != nil || n < 1 || n > 65535 {
			return locales.Messages.AccountLocationNeedsPort
		}
	}
	if k.ID == mediasource.KindSFTP {
		if c[mediasource.SFTPUser] == "" {
			return locales.Messages.AccountLocationNeedsUser
		}
		if s.ID == "" && typed[mediasource.SFTPPassword] == "" && strings.TrimSpace(typed[mediasource.SFTPPrivateKey]) == "" {
			return locales.Messages.AccountLocationNeedsSecret
		}
		return nil
	}
	share := strings.Trim(c[mediasource.SMBShare], `/\`)
	if strings.ContainsAny(share, `/\`) {
		return locales.Messages.AccountLocationNeedsShare
	}
	if share == "" && strings.Trim(c[mediasource.SMBRoot], "/") != "" {
		return locales.Messages.SettingsSourcesNeedsShare
	}
	if c[mediasource.SMBUser] == "" {
		return locales.Messages.AccountLocationNeedsUser
	}
	return nil
}

// saveMediaSourceDraft keeps the dialog's draft as a new source or in place of the one edited, saves settings.json and closes the dialog. A source without a name is named after its server. The secrets typed are kept in the secure store first, off the engine's goroutine, as the system may ask the user, remembering what the store held so they are put back as they were when the save does not finish (mediaSourceSecretWrite); a secret left empty keeps the one stored. A draft that misses something, a secure store that is not there or refused, or a settings.json that cannot be written keeps the dialog open and says why.
func (a *app) saveMediaSourceDraft() {
	m := &a.sources
	if m.saving {
		return
	}
	s, k, secrets, ok := a.mediaSourceDraftSource()
	if !ok {
		return
	}
	if p := mediaSourceProblem(k, s, secrets); p != nil {
		m.problem = p
		return
	}
	if s.Name == "" {
		s.Name = mediasource.Host(s)
	}
	if s.ID == "" {
		s.ID = a.mediaSourceRelinkID(s)
	}
	m.problem = nil
	if origin := mediaSourceOrigin(k, s); origin != "" {
		// A plugin's source's secrets are bound to its endpoint's origin: those typed are kept for it, and when the address moved to another server, those not typed again are emptied rather than sent there (docs/media-sources.md 3.1).
		if i := a.mediaSourceIndex(s.ID); i >= 0 && mediaSourceOrigin(k, m.list[i]) != origin {
			for _, key := range k.SecretKeys() {
				if _, typed := secrets[key]; !typed {
					secrets[key] = ""
				}
			}
		}
		if len(secrets) > 0 {
			secrets[mediaSourceOriginKey] = origin
		}
	}
	if len(secrets) == 0 {
		a.mediaSourceCommit(s)
		return
	}
	m.saveGen++
	gen := m.saveGen
	m.saving = true
	store, update, held := a.mediaSourceSecretStore(), a.update, m.secretsHeld
	go func() {
		held <- struct{}{}
		w := mediaSourceSecretWrite{store: store, id: s.ID, held: held, prev: map[string]string{}}
		var err error
		for _, key := range slices.Sorted(maps.Keys(secrets)) {
			err = w.set(key, secrets[key])
			if err != nil {
				break
			}
		}
		update(func() { a.mediaSourceSecretsKept(gen, s, w, err) })
	}()
}

// mediaSourceSecretWrite is a save's write of the secrets of the source id to store, which holds held until it is done: the keys it wrote, and what each held before, in prev, a key absent from prev having held nothing. Until settings.json took the save, the store holds secrets settings.json does not describe, so a save that is abandoned or fails puts them back (undo); one that settings.json took keeps them (keep).
type mediaSourceSecretWrite struct {
	store ns.Secrets
	id    string
	held  chan struct{}
	keys  []string
	prev  map[string]string
}

// set writes v as the secret key, or forgets it when v is empty, first reading what the store held there to put back.
func (w *mediaSourceSecretWrite) set(key, v string) error {
	account := mediaSourceSecretAccount(w.id, key)
	old, had, err := w.store.Get(account)
	if err != nil {
		return err
	}
	if v == "" {
		err = w.store.Delete(account)
	} else {
		err = w.store.Set(account, v)
	}
	if err != nil {
		return err
	}
	w.keys = append(w.keys, key)
	if had {
		w.prev[key] = old
	}
	return nil
}

// keep ends a write settings.json took.
func (w mediaSourceSecretWrite) keep() { <-w.held }

// undo puts back off the engine's goroutine what the write replaced, forgetting what it added, then ends it. A connection made meanwhile with the secrets written is closed.
func (w mediaSourceSecretWrite) undo(pool *mediasource.Pool) {
	go func() {
		defer w.keep()
		for _, key := range w.keys {
			account := mediaSourceSecretAccount(w.id, key)
			var err error
			if old, ok := w.prev[key]; ok {
				err = w.store.Set(account, old)
			} else {
				err = w.store.Delete(account)
			}
			if err != nil {
				log.Printf("media source: putting a secret back failed: %v", err)
			}
		}
		pool.Forget(w.id)
	}()
}

// mediaSourceSecretsKept finishes the save gen of s once its secrets were written (w), or says why they were not, unless the dialog closed or another save took its place. Unless settings.json takes s, what w wrote is put back.
func (a *app) mediaSourceSecretsKept(gen int, s mediasource.Source, w mediaSourceSecretWrite, err error) {
	m := &a.sources
	if !m.editing || m.saveGen != gen {
		w.undo(m.pool)
		return
	}
	m.saving = false
	switch {
	case errors.Is(err, ns.ErrSecretsUnavailable):
		m.problem = locales.Messages.SettingsSourcesNoSecretStore
	case err != nil:
		log.Printf("media source: keeping a secret failed: %s", feedback.Scrubber{Secrets: a.feedbackSecrets()}.NoSecrets(err.Error()))
		m.problem = locales.Messages.SettingsSourcesSecretFailed
	default:
		if a.mediaSourceCommit(s) {
			w.keep()
			return
		}
	}
	w.undo(m.pool)
}

// mediaSourceCommit puts s in the list, in place of the source of its ID or after the others, saves settings.json and closes the dialog, or keeps it open and says the settings could not be written. A source added from the 媒体来源 page (thenBrowse) is shown there, the settings closed. It reports whether settings.json took s.
func (a *app) mediaSourceCommit(s mediasource.Source) bool {
	m := &a.sources
	before, removed := slices.Clone(m.list), slices.Clone(m.removed)
	browse := m.thenBrowse && a.mediaSourceIndex(s.ID) < 0
	if i := a.mediaSourceIndex(s.ID); i >= 0 {
		m.list[i] = s
	} else {
		m.list = append(m.list, s)
	}
	if err := a.saveSettings(); err != nil {
		m.list, m.removed = before, removed
		m.problem = locales.Messages.SettingsSourcesSaveFailed
		return false
	}
	// How a Test of the old settings went says nothing of the new ones, and a connection made with the old password is not one to keep.
	delete(m.tests, s.ID)
	a.mediaSourcePoolSync()
	m.pool.Forget(s.ID)
	a.forgetLinkedPosterMiss(s.ID)
	a.loadExtras()
	a.closeMediaSourceDialog()
	if browse {
		a.mediaSourceBrowseFrom(s.ID)
	}
	return true
}

// askRemoveMediaSource opens the confirmation of removing the source id, holding the window's modal flag until it is answered.
func (a *app) askRemoveMediaSource(id string) {
	if a.mediaSourceIndex(id) < 0 {
		return
	}
	a.sources.removing = id
	if a.sources.removeRelease == nil {
		a.sources.removeRelease = a.holdModal()
	}
}

// answerRemoveMediaSource answers the confirmation and gives back its modal flag: yes removes the source, saves settings.json and, once it did, forgets its secrets off the engine's goroutine; a settings.json that cannot be written keeps the source, its secrets with it, and says so. Nothing on the source is touched, the films copied from it stay as they are, and those indexed from it keep their lines, its address kept without its secrets so adding it back plays them again.
func (a *app) answerRemoveMediaSource(yes bool) {
	m := &a.sources
	id := m.removing
	m.removing = ""
	if m.removeRelease != nil {
		m.removeRelease()
		m.removeRelease = nil
	}
	i := a.mediaSourceIndex(id)
	if !yes || i < 0 {
		return
	}
	gone, before, removed := m.list[i], slices.Clone(m.list), slices.Clone(m.removed)
	m.list = slices.Delete(m.list, i, i+1)
	// Where it was is kept without its secrets while films are indexed from it, so adding it back at the same address finds them again (mediaSourceRelinkID).
	if a.mediaSourceLinkedCount(id) > 0 {
		m.removed = append(m.removed, mediasource.Source{ID: gone.ID, Kind: gone.Kind, Name: gone.Name, Config: maps.Clone(gone.Config)})
	}
	if err := a.saveSettings(); err != nil {
		m.list, m.removed = before, removed
		a.saveErr = a.messages().SettingsSourcesSaveFailed()
		return
	}
	delete(m.tests, id)
	a.mediaSourcePoolSync()
	var keys []string
	if k, ok := mediasource.Builtin(gone.Kind); ok {
		keys = k.SecretKeys()
	} else if k, ok := m.kinds.Kind(gone.Kind); ok {
		keys = append(k.SecretKeys(), mediaSourceOriginKey)
	}
	if len(keys) == 0 {
		return
	}
	store, held := a.mediaSourceSecretStore(), m.secretsHeld
	go func() {
		held <- struct{}{}
		defer func() { <-held }()
		for _, key := range keys {
			_ = store.Delete(mediaSourceSecretAccount(id, key))
		}
	}()
}

// mediaSourceTestable is how to reach target: the source of that ID, or with "" the dialog's draft, the secrets typed in the draft, and why it cannot be tried, nil when it can. ok is false for a source that is gone, a draft whose dialog closed, or a kind not registered.
func (a *app) mediaSourceTestable(target string) (s mediasource.Source, k mediasource.Kind, typed map[string]string, problem func(locales.Messages) string, ok bool) {
	m := &a.sources
	if target == "" {
		if !m.editing {
			return s, k, nil, nil, false
		}
		s, k, typed, ok = a.mediaSourceDraftSource()
		if !ok {
			return s, k, nil, nil, false
		}
		return s, k, typed, mediaSourceProblem(k, s, typed), true
	}
	i := a.mediaSourceIndex(target)
	if i < 0 {
		return s, k, nil, nil, false
	}
	s = m.list[i]
	if k, ok = m.kinds.Kind(s.Kind); !ok {
		return s, k, nil, nil, false
	}
	return s, k, nil, nil, true
}

// testMediaSource connects to target (mediaSourceTestable) off the engine's goroutine, as the person asked with Test, lists its root, closes the connection again and records how it went. The secrets come from the draft as typed, else from the secure store, read there too.
func (a *app) testMediaSource(target string) {
	m := &a.sources
	s, k, typed, problem, ok := a.mediaSourceTestable(target)
	if !ok || m.tests[target].busy {
		return
	}
	// What kept Save from saving is said again by the Test of the draft, or is gone.
	if target == "" {
		m.problem = nil
	}
	if problem != nil {
		m.tests[target] = mediaSourceTest{err: problem}
		return
	}
	m.gen++
	gen := m.gen
	m.tests[target] = mediaSourceTest{busy: true, gen: gen}
	open, store, update := m.open, a.mediaSourceSecretStore(), a.update
	go func() {
		secrets := mediaSourceReadSecrets(store, k, s, typed)
		ctx, cancel := context.WithTimeout(context.Background(), mediaSourceTestTimeout)
		defer cancel()
		var entries []mediasource.Entry
		d, err := open(ctx, k, s, secrets)
		if err == nil {
			entries, err = d.List(ctx, "")
			_ = d.Close()
		}
		update(func() { a.mediaSourceTested(target, gen, s, secrets, entries, err) })
	}()
}

// mediaSourceReadSecrets are the secrets of the source s of kind k: those typed, else those the secure store keeps. A plugin's source's secrets kept for another origin than its endpoint's (mediaSourceOriginKey) are read as none. It reads the store, which may make it wait, so it runs off the engine's goroutine; a secret it cannot read is left out, so the connection fails at signing in and says so.
func mediaSourceReadSecrets(store ns.Secrets, k mediasource.Kind, s mediasource.Source, typed map[string]string) map[string]string {
	id := s.ID
	secrets := maps.Clone(typed)
	if secrets == nil {
		secrets = map[string]string{}
	}
	if origin := mediaSourceOrigin(k, s); origin != "" && id != "" {
		if kept, ok, err := store.Get(mediaSourceSecretAccount(id, mediaSourceOriginKey)); err != nil || !ok || kept != origin {
			return secrets
		}
	}
	for _, key := range k.SecretKeys() {
		if secrets[key] != "" || id == "" {
			continue
		}
		if v, ok, err := store.Get(mediaSourceSecretAccount(id, key)); err == nil && ok {
			secrets[key] = v
		}
	}
	return secrets
}

// mediaSourceTested records how the Test gen of target went, made with secrets, unless another Test took its place or its source or dialog went away. A server without a share lists its shares, which the status names. An SFTP server whose key Kando does not trust opens the question about it: trust it at the first connection, or the warning that it changed.
func (a *app) mediaSourceTested(target string, gen int, s mediasource.Source, secrets map[string]string, entries []mediasource.Entry, err error) {
	m := &a.sources
	if t := m.tests[target]; !t.busy || t.gen != gen {
		return
	}
	if _, _, _, _, ok := a.mediaSourceTestable(target); !ok {
		delete(m.tests, target)
		return
	}
	if err == nil {
		t := mediaSourceTest{ok: true}
		if s.Kind == mediasource.KindSMB && mediasource.SMBRemote(s, "").Share == "" {
			t.listed = true
			for _, e := range entries {
				t.shares = append(t.shares, e.Name)
			}
		}
		m.tests[target] = t
		return
	}
	var hk *remotefs.HostKeyError
	if errors.As(err, &hk) {
		delete(m.tests, target)
		kind := account.AlertTrust
		if errors.Is(err, remotefs.ErrHostKeyMismatch) {
			kind = account.AlertKeyChanged
		}
		a.mediaSourceAsk(mediaSourceAlert{kind: kind, target: target, hk: hk})
		return
	}
	r, _ := mediasource.Remote(s, secrets)
	// The row says what to do in the interface's language; the whole error goes to the log file, without the secrets and addresses the app holds.
	log.Printf("media source: %s test failed: %s", s.Kind, feedback.Scrubber{Secrets: append(a.feedbackSecrets(), slices.Collect(maps.Values(secrets))...)}.NoSecrets(err.Error()))
	if k, ok := m.kinds.Kind(s.Kind); ok && !k.Origin.Builtin() {
		// A certificate the system does not trust, as a NAS's own, is asked about at once, as an SFTP server's key is.
		if cert := (*mediasource.CertError)(nil); errors.As(err, &cert) {
			delete(m.tests, target)
			kind := account.AlertTrust
			if cert.Pinned != "" {
				kind = account.AlertKeyChanged
			}
			a.mediaSourceAsk(mediaSourceAlert{kind: kind, target: target, cert: cert, host: mediasource.Host(s)})
			return
		}
		m.tests[target] = mediaSourceTest{err: func(msg locales.Messages) string { return a.mediaSourcePluginFailure(msg, k, err) }}
		return
	}
	m.tests[target] = mediaSourceTest{err: func(msg locales.Messages) string { return mediaSourceFailure(msg, r, err) }}
}

// mediaSourceFailure says why reaching r failed with err, in m's language: as a storage location's failure does (account.Failure), and that the server did not sign, which a source requires unless told otherwise.
func mediaSourceFailure(m locales.Messages, r remotefs.Remote, err error) string {
	if remotefs.Diagnose(err) == remotefs.FailSigning {
		return m.SettingsSourcesFailSigning()
	}
	return account.Failure(m, r, err)
}

// mediaSourceWarnings are what the dialog says under the fields of the draft: for SMB, that it cannot prove which server it reached, and while the draft does without signing, what that risks; for WebDAV at an http:// address, that its password crosses the network unencrypted.
func (a *app) mediaSourceWarnings() []string {
	m := &a.sources
	if k, ok := m.kinds.Kind(m.draft.Kind); m.editing && ok && !k.Origin.Builtin() {
		return mediaSourcePluginWarnings(a.messages(), m.draft.Values)
	}
	if !m.editing {
		return nil
	}
	msg := a.messages()
	if m.draft.Kind == mediasource.KindWebDAV {
		if mediasource.PlainHTTP(mediasource.Source{Kind: m.draft.Kind, Config: m.draft.Values}) {
			return []string{msg.SettingsSourcesPlainHTTP()}
		}
		return nil
	}
	if m.draft.Kind != mediasource.KindSMB {
		return nil
	}
	out := []string{msg.SettingsSourcesSMBWarning()}
	if m.draft.Values[mediasource.SMBNoSigning] == "true" {
		out = append(out, msg.SettingsSourcesNoSigningRisk())
	}
	return out
}

// mediaSourceSummary is a source's row's subtitle: its kind's name and where it is, as "SMB · nas.local/media/films", or the kind's ID while no kind of that ID is registered.
func (a *app) mediaSourceSummary(s mediasource.Source) string {
	msg := a.messages()
	kind := s.Kind
	k, ok := a.sources.kinds.Kind(s.Kind)
	if ok {
		kind = k.Title(a.prefs.interfaceLanguage)
	}
	place := s.Config[mediasource.SMBHost]
	if ok && !k.Origin.Builtin() {
		if u, err := mediasource.ParseEndpoint(s.Config[mediasource.EndpointKey]); err == nil {
			place = u.Host + strings.TrimSuffix(u.Path, "/")
		}
	}
	// A whole SMB server, without a share, is its host alone.
	if r, ok := mediasource.Remote(s, nil); ok && (r.Kind != remotefs.KindSMB || r.Share != "") {
		place = account.Place(r)
	}
	return msg.AccountLocationSummary(kind, place)
}

// mediaSourceAsk opens the question q about a server's key over the page, holding the window's modal flag until it is answered (answerMediaSourceAlert).
func (a *app) mediaSourceAsk(q mediaSourceAlert) {
	a.sources.alert = q
	if a.sources.alertRelease == nil {
		a.sources.alertRelease = a.holdModal()
	}
}

// answerMediaSourceAlert answers the question about a server's key and gives back its modal flag. yes trusts the key the server offered: the draft keeps it as its host key, saved with Save, and a source saved keeps it at once in settings.json; either is tested again. no leaves the source as it was and says where the Test's result shows that the key is not trusted.
func (a *app) answerMediaSourceAlert(yes bool) {
	m := &a.sources
	q := m.alert
	if q.kind == account.AlertNone {
		return
	}
	m.alert = mediaSourceAlert{}
	if m.alertRelease != nil {
		m.alertRelease()
		m.alertRelease = nil
	}
	if _, _, _, _, ok := a.mediaSourceTestable(q.target); !ok {
		return
	}
	if !yes {
		m.tests[q.target] = mediaSourceTest{err: locales.Messages.AccountLocationNotTrusted}
		return
	}
	// A server's key is kept as the source's host key; a certificate by its fingerprint, which takes the place of the system's verification for the source's endpoint alone.
	key, value := mediasource.SFTPHostKey, ""
	if q.cert != nil {
		key, value = mediasource.PinKey, q.cert.Fingerprint
	} else {
		value = q.hk.Key
	}
	if q.target == "" {
		if m.draft.Values == nil {
			m.draft.Values = map[string]string{}
		}
		m.draft.Values[key] = value
		a.testMediaSource("")
		return
	}
	i := a.mediaSourceIndex(q.target)
	before := m.list[i]
	s := before
	s.Config = maps.Clone(s.Config)
	if s.Config == nil {
		s.Config = map[string]string{}
	}
	s.Config[key] = value
	m.list[i] = s
	if err := a.saveSettings(); err != nil {
		m.list[i] = before
		a.saveErr = a.messages().SettingsSourcesSaveFailed()
		return
	}
	// The pool connects again with the key now trusted.
	a.mediaSourcePoolSync()
	a.testMediaSource(q.target)
}

// mediaSourceAlertView is the question about a server's key as the page shows it.
func (a *app) mediaSourceAlertView() account.Alert {
	q := a.sources.alert
	if q.kind == account.AlertNone {
		return account.Alert{}
	}
	if q.cert != nil {
		return account.Alert{Kind: q.kind, Host: q.host, Fingerprint: q.cert.Fingerprint, Trusted: q.cert.Pinned, Certificate: true}
	}
	return account.Alert{Kind: q.kind, Host: q.hk.Host, Fingerprint: q.hk.Fingerprint, Trusted: remotefs.Fingerprint(q.hk.Trusted)}
}
