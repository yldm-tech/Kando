package main

import (
	"context"
	"crypto/rand"
	"errors"
	"io"
	"log"
	"slices"
	"strings"
	"time"

	"kando/apps/kando/account"
	"kando/apps/kando/feedback"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings"
	"kando/apps/kando/storage"

	ns "github.com/nsgo-dev/nsgo/core"
)

// locationsUI is the state of 设置 › 账号与上传 › 存储位置: the folders on a NAS or a server films can be kept in (list, saved in settings.json); those removed while videos were moved to them (removed, without their secrets, also saved), so one added back at the same address gets its ID back and those videos play again (locationRelink); the dialog that adds or edits one (editing, editID "" for a new one, draft as typed, problem why Save refused it), how the last Test of each went (tests, by ID, the draft's under ""), and the question open over the page (alert). gen numbers the Tests, so the answer of one started before another, or before the dialog closed, is dropped. editRelease and alertRelease give back the modal flag the dialog and the alert hold (holdModal). open connects, storage.OpenRemote unless a test sets another. The locations' secrets are kept in the system's secure store (state_locations_secrets.go): plain are the IDs of those whose secrets settings.json keeps instead, as an earlier version wrote them or the store refused them; secretGen numbers each location's changes of its secrets, so a store's answer to an older one is dropped; secretsTail is where the store's work queued last ends, and secretsBusy how many of it are not done; secrets is the store a test sets, the app's own (locationSecretStore) when nil.
type locationsUI struct {
	list         []account.Location
	removed      []account.Location
	editing      bool
	editID       string
	draft        account.Draft
	problem      func(locales.Messages) string
	tests        map[string]locationTest
	alert        locationAlert
	gen          int
	editRelease  func()
	alertRelease func()
	open         func(context.Context, storage.Remote) (storage.Provider, io.Closer, error)
	plain        map[string]bool
	secretGen    map[string]int
	secretsTail  chan struct{}
	secretsBusy  int
	secrets      func() ns.Secrets
}

// locationTest is how a Test of a location went: running (gen is its number), connected, or why not, as a function of the messages so it reads in the language shown.
type locationTest struct {
	busy bool
	ok   bool
	gen  int
	err  func(locales.Messages) string
}

// locationAlert is the question open over the page: kind, the location it is about (target, "" for the dialog's draft), and for a host key the server's error, which holds the key it offered.
type locationAlert struct {
	kind   account.AlertKind
	target string
	hk     *storage.HostKeyError
}

func newLocationsUI() locationsUI {
	return locationsUI{tests: map[string]locationTest{}, open: storage.OpenRemote, plain: map[string]bool{}, secretGen: map[string]int{}}
}

// locationTestTimeout bounds a Test: connecting, signing in and reading the folder.
const locationTestTimeout = 30 * time.Second

// savedLocations is the storage locations' part of settings.json. Passwords, private keys and passphrases are kept in the system's secure store (state_locations_secrets.go), and here only for a location the store did not take them for (locationsUI.plain), in settings.json, which saveSettings writes with mode 0600; they are left out of the log and of feedback (locationSecrets).
type savedLocations struct {
	StorageLocations []savedLocation `json:"storageLocations,omitempty"`
	// RemovedStorageLocations are the locations removed while videos were on them, without their passwords and keys (locationsUI.removed).
	RemovedStorageLocations []savedLocation `json:"removedStorageLocations,omitempty"`
}

// savedLocation is one storage location in settings.json.
type savedLocation struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Kind       string `json:"kind"`
	Host       string `json:"host,omitempty"`
	Port       int    `json:"port,omitempty"`
	Share      string `json:"share,omitempty"`
	URL        string `json:"url,omitempty"`
	Root       string `json:"root,omitempty"`
	User       string `json:"user,omitempty"`
	Password   string `json:"password,omitempty"`
	Domain     string `json:"domain,omitempty"`
	PrivateKey string `json:"privateKey,omitempty"`
	Passphrase string `json:"passphrase,omitempty"`
	HostKey    string `json:"hostKey,omitempty"`
}

// clean drops a location without an ID, a name or a kind Kando knows, and the second of two with the same ID, and trims what is not a secret. Passwords and passphrases are kept as typed. A removed location keeps none of its secrets, and none with the ID of one in the settings.
func (s *savedLocations) clean() {
	seen := map[string]bool{}
	s.StorageLocations = cleanLocations(s.StorageLocations, seen)
	s.RemovedStorageLocations = cleanLocations(s.RemovedStorageLocations, seen)
	for i := range s.RemovedStorageLocations {
		l := &s.RemovedStorageLocations[i]
		l.Password, l.Domain, l.PrivateKey, l.Passphrase, l.HostKey = "", "", "", "", ""
	}
}

// cleanLocations is cleaned's list of locations, those with an ID in seen left out and the IDs kept added to it, nil for none.
func cleanLocations(locations []savedLocation, seen map[string]bool) []savedLocation {
	kept := locations[:0]
	for _, l := range locations {
		l.ID, l.Name, l.Kind = strings.TrimSpace(l.ID), strings.TrimSpace(l.Name), strings.TrimSpace(l.Kind)
		l.Host, l.Share, l.URL, l.Root = strings.TrimSpace(l.Host), strings.TrimSpace(l.Share), strings.TrimSpace(l.URL), strings.TrimSpace(l.Root)
		l.User, l.Domain, l.PrivateKey, l.HostKey = strings.TrimSpace(l.User), strings.TrimSpace(l.Domain), strings.TrimSpace(l.PrivateKey), strings.TrimSpace(l.HostKey)
		if l.ID == "" || l.Name == "" || seen[l.ID] || !slices.Contains(account.Kinds, storage.Kind(l.Kind)) {
			continue
		}
		if l.Port < 0 || l.Port > 65535 {
			l.Port = 0
		}
		seen[l.ID] = true
		kept = append(kept, l)
	}
	if len(kept) == 0 {
		return nil
	}
	return kept
}

// saveLocations writes the locations, and those removed that videos are still on (storedCount), to settings.json.
func (a *app) saveLocations(saved *savedSettings) {
	out := savedLocations{StorageLocations: savedLocationsOf(a.locations.list)}
	for i := range out.StorageLocations {
		if l := &out.StorageLocations[i]; !a.locations.plain[l.ID] {
			l.Password, l.PrivateKey, l.Passphrase = "", "", ""
		}
	}
	var removed []account.Location
	for _, l := range a.locations.removed {
		if a.storedCount(l.ID) > 0 {
			removed = append(removed, l)
		}
	}
	out.RemovedStorageLocations = savedLocationsOf(removed)
	saved.savedLocations = out
}

// savedLocationsOf is locations as settings.json keeps them.
func savedLocationsOf(locations []account.Location) []savedLocation {
	var out []savedLocation
	for _, l := range locations {
		r := l.Remote
		out = append(out, savedLocation{ID: l.ID, Name: l.Name, Kind: string(r.Kind), Host: r.Host, Port: r.Port, Share: r.Share, URL: r.URL, Root: r.Root,
			User: r.User, Password: r.Password, Domain: r.Domain, PrivateKey: r.PrivateKey, Passphrase: r.Passphrase, HostKey: r.HostKey})
	}
	return out
}

// restoreLocations takes the locations back from settings.json. Those it keeps with their secrets, as an earlier version wrote them, are plain until the secure store took them (locationSecretsLoad); the others' secrets are read from the store once the app's scope exists.
func (a *app) restoreLocations(saved savedSettings) {
	a.locations.list = locationsOfSaved(saved.StorageLocations)
	a.locations.removed = locationsOfSaved(saved.RemovedStorageLocations)
	clear(a.locations.plain)
	for _, l := range a.locations.list {
		if locationHasSecrets(l.Remote) {
			a.locations.plain[l.ID] = true
		}
	}
}

// locationsOfSaved is the locations settings.json keeps, as the app holds them.
func locationsOfSaved(saved []savedLocation) []account.Location {
	var out []account.Location
	for _, l := range saved {
		out = append(out, account.Location{ID: l.ID, Name: l.Name, Remote: storage.Remote{
			Kind: storage.Kind(l.Kind), Host: l.Host, Port: l.Port, Share: l.Share, URL: l.URL, Root: l.Root,
			User: l.User, Password: l.Password, Domain: l.Domain, PrivateKey: l.PrivateKey, Passphrase: l.Passphrase, HostKey: l.HostKey}})
	}
	return out
}

// locationSecrets are what of the storage locations must not leave the machine, such as in the log or a feedback issue: each one's server, address, share, user and domain, and its password, private key and passphrase. The name, the kind, the folder, the port and the server's public key are left out. Keep it next to savedLocation: a value added there belongs here too (locations_test.go checks).
func (l locationsUI) locationSecrets() []string {
	var out []string
	for _, loc := range slices.Concat(l.list, l.removed) {
		r := loc.Remote
		out = append(out, r.Host, r.URL, r.Share, r.User, r.Domain, r.Password, r.PrivateKey, r.Passphrase)
	}
	// What the dialog holds is not saved yet, but a Test's failure is logged with it.
	d := l.draft
	return append(out, d.Host, d.URL, d.Share, d.User, d.Domain, d.Password, d.PrivateKey, d.Passphrase)
}

// locationsHooks: the storage locations are saved in settings.json and taken back at startup, their secrets read from the system's secure store, or moved into it, once the window exists (locationSecretsLoad); after each change, the dialog and the alert close when the page that draws them went away (locationsModals).
func (a *app) locationsHooks() areaHooks {
	return areaHooks{
		save:        a.saveLocations,
		restore:     a.restoreLocations,
		ready:       a.locationSecretsLoad,
		afterChange: a.locationsModals,
	}
}

// locationsModals closes the location dialog and the alert once the settings closed or show another section, as the 账号与上传 page draws them, and gives back the modal flags they held, so the menu bar's commands do not wait for a dialog nobody sees. It reports whether it changed anything.
func (a *app) locationsModals() bool {
	l := &a.locations
	changed := false
	if (l.editing || l.alert.kind != account.AlertNone) && (!a.prefs.open || a.SettingsSection() != settings.SectionAccount) {
		l.editing, l.alert = false, locationAlert{}
		changed = true
	}
	if !l.editing && l.editRelease != nil {
		l.editRelease()
		l.editRelease = nil
		changed = true
	}
	if l.alert.kind == account.AlertNone && l.alertRelease != nil {
		l.alertRelease()
		l.alertRelease = nil
		changed = true
	}
	return changed
}

// locationIndex is where the location id is in the list, -1 when it is not.
func (a *app) locationIndex(id string) int {
	return slices.IndexFunc(a.locations.list, func(l account.Location) bool { return l.ID == id })
}

// openLocationDialog opens the dialog on draft, for the location id or, with id "", a new one.
func (a *app) openLocationDialog(id string, draft account.Draft) {
	l := &a.locations
	l.editing, l.editID, l.draft, l.problem = true, id, draft, nil
	delete(l.tests, "")
	if l.editRelease == nil {
		l.editRelease = a.holdModal()
	}
}

// closeLocationDialog closes the dialog without saving, forgetting the draft, and gives back its modal flag.
func (a *app) closeLocationDialog() {
	l := &a.locations
	l.editing, l.draft, l.problem = false, account.Draft{}, nil
	delete(l.tests, "")
	if l.editRelease != nil {
		l.editRelease()
		l.editRelease = nil
	}
}

// saveLocationDraft keeps the dialog's draft as a new location or in place of the one edited, saves settings.json and closes the dialog; a draft that misses something, or a settings.json that cannot be written, keeps the dialog open and says why.
func (a *app) saveLocationDraft() {
	l := &a.locations
	d := l.draft
	if p := d.NameProblem(); p != nil {
		l.problem = p
		return
	}
	if p := d.Problem(); p != nil {
		l.problem = p
		return
	}
	loc := account.Location{ID: l.editID, Name: strings.TrimSpace(d.Name), Remote: d.Remote()}
	before, removed := slices.Clone(l.list), slices.Clone(l.removed)
	if i := a.locationIndex(loc.ID); loc.ID != "" && i >= 0 {
		l.list[i] = loc
	} else {
		loc.ID = rand.Text()
		a.locationRelink(&loc)
		l.list = append(l.list, loc)
	}
	if err := a.saveSettings(); err != nil {
		l.list, l.removed = before, removed
		l.problem = locales.Messages.AccountLocationSaveFailed
		return
	}
	// How a Test of the old settings went says nothing of the new ones.
	delete(l.tests, loc.ID)
	a.closeLocationDialog()
	a.locationSecretsKeep(loc.ID)
}

// removeLocation forgets the location id with its secrets and saves settings.json. Nothing on the location is removed. When videos were moved to it, where it is is kept without its secrets (locationsUI.removed), so adding it back at the same address finds them again (locationRelink).
func (a *app) removeLocation(id string) {
	i := a.locationIndex(id)
	if i < 0 {
		return
	}
	gone := a.locations.list[i]
	a.locations.list = slices.Delete(a.locations.list, i, i+1)
	delete(a.locations.tests, id)
	if a.storedCount(id) > 0 {
		r := gone.Remote
		gone.Remote = storage.Remote{Kind: r.Kind, Host: r.Host, Port: r.Port, Share: r.Share, URL: r.URL, Root: r.Root, User: r.User}
		a.locations.removed = append(a.locations.removed, gone)
	}
	if err := a.saveSettings(); err != nil {
		a.saveErr = a.messages().AccountLocationSaveFailed()
	}
	a.locationSecretsForget(id)
}

// locationRelink gives loc, a location just added, the ID of a location removed while videos were on it (locationsUI.removed) at the same address: the same kind of server, host and port, share, address and folder. Those videos' records name that ID, so they play from loc again and can be moved back.
func (a *app) locationRelink(loc *account.Location) {
	l := &a.locations
	for i, old := range l.removed {
		if !sameAddress(old.Remote, loc.Remote) || a.locationIndex(old.ID) >= 0 {
			continue
		}
		loc.ID = old.ID
		l.removed = slices.Delete(l.removed, i, i+1)
		return
	}
}

// sameAddress reports whether a and b reach the same folder: the same kind of server, host (ignoring case) and port, share, address and folder, whatever the user and the secrets.
func sameAddress(a, b storage.Remote) bool {
	trim := func(s string) string { return strings.Trim(strings.TrimSpace(s), "/") }
	return a.Kind == b.Kind && strings.EqualFold(strings.TrimSpace(a.Host), strings.TrimSpace(b.Host)) && a.Port == b.Port &&
		trim(a.Share) == trim(b.Share) && trim(a.URL) == trim(b.URL) && trim(a.Root) == trim(b.Root)
}

// locationRemote is how to reach target: the location of that ID, or with "" the dialog's draft, and why it cannot be tried, nil when it can. ok is false for a location that is gone, or a draft whose dialog closed.
func (a *app) locationRemote(target string) (r storage.Remote, problem func(locales.Messages) string, ok bool) {
	if target == "" {
		if !a.locations.editing {
			return storage.Remote{}, nil, false
		}
		return a.locations.draft.Remote(), a.locations.draft.Problem(), true
	}
	i := a.locationIndex(target)
	if i < 0 {
		return storage.Remote{}, nil, false
	}
	return a.locations.list[i].Remote, nil, true
}

// testLocation connects to target (locationRemote) off the engine's goroutine, as the person asked with Test, closes the connection again and records how it went. An SFTP server whose key is not trusted asks about its key (locationTested).
func (a *app) testLocation(target string) {
	l := &a.locations
	r, problem, ok := a.locationRemote(target)
	if !ok || l.tests[target].busy {
		return
	}
	if problem != nil {
		l.tests[target] = locationTest{err: problem}
		return
	}
	l.gen++
	gen := l.gen
	l.tests[target] = locationTest{busy: true, gen: gen}
	open := l.open
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), locationTestTimeout)
		defer cancel()
		_, closer, err := open(ctx, r)
		if err == nil {
			_ = closer.Close()
		}
		a.update(func() { a.locationTested(target, gen, r, err) })
	}()
}

// locationTested records how the Test gen of target went, unless another Test took its place or its location or dialog went away. A host key Kando does not trust opens the question about it: trust it at the first connection, or the warning that it changed.
func (a *app) locationTested(target string, gen int, r storage.Remote, err error) {
	l := &a.locations
	if t := l.tests[target]; !t.busy || t.gen != gen {
		return
	}
	if _, _, ok := a.locationRemote(target); !ok {
		delete(l.tests, target)
		return
	}
	if err == nil {
		l.tests[target] = locationTest{ok: true}
		return
	}
	var hk *storage.HostKeyError
	if errors.As(err, &hk) {
		delete(l.tests, target)
		kind := account.AlertTrust
		if errors.Is(err, storage.ErrHostKeyMismatch) {
			kind = account.AlertKeyChanged
		}
		a.askLocation(locationAlert{kind: kind, target: target, hk: hk})
		return
	}
	// The row says what to do in the interface's language; the whole error goes to the log file, without the secrets and addresses the app holds.
	log.Printf("storage location: %s test failed: %s", r.Kind, feedback.Scrubber{Secrets: a.feedbackSecrets()}.NoSecrets(err.Error()))
	l.tests[target] = locationTest{err: func(m locales.Messages) string { return account.Failure(m, r, err) }}
}

// askLocation opens the question q over the page, holding the window's modal flag until it is answered (answerLocation).
func (a *app) askLocation(q locationAlert) {
	a.locations.alert = q
	if a.locations.alertRelease == nil {
		a.locations.alertRelease = a.holdModal()
	}
}

// answerLocation answers the question open over the page and gives back its modal flag. yes removes the location, or trusts the key the server offered (storing it as the location's HostKey, saved at once for a saved location and with Save for the draft) and tests again; no leaves everything as it was, and a key not trusted says so where the Test's result shows.
func (a *app) answerLocation(yes bool) {
	l := &a.locations
	q := l.alert
	if q.kind == account.AlertNone {
		return
	}
	l.alert = locationAlert{}
	if l.alertRelease != nil {
		l.alertRelease()
		l.alertRelease = nil
	}
	switch {
	case q.kind == account.AlertRemove:
		if yes {
			a.removeLocation(q.target)
		}
	case !yes:
		if _, _, ok := a.locationRemote(q.target); ok {
			l.tests[q.target] = locationTest{err: locales.Messages.AccountLocationNotTrusted}
		}
	case q.target == "":
		if l.editing {
			l.draft.HostKey = q.hk.Key
			a.testLocation("")
		}
	default:
		i := a.locationIndex(q.target)
		if i < 0 {
			return
		}
		l.list[i].Remote.HostKey = q.hk.Key
		if err := a.saveSettings(); err != nil {
			a.saveErr = a.messages().AccountLocationSaveFailed()
		}
		a.testLocation(q.target)
	}
}

// locationAlertView is the question open over the page as the page shows it.
func (a *app) locationAlertView() account.Alert {
	q := a.locations.alert
	out := account.Alert{Kind: q.kind}
	switch q.kind {
	case account.AlertRemove:
		if i := a.locationIndex(q.target); i >= 0 {
			out.Name = a.locations.list[i].Name
		}
		out.Films = a.storedCount(q.target)
	case account.AlertTrust, account.AlertKeyChanged:
		out.Host, out.Fingerprint, out.Trusted = q.hk.Host, q.hk.Fingerprint, storage.Fingerprint(q.hk.Trusted)
	}
	return out
}
