package account

import (
	"errors"
	"fmt"
	"strings"
	"testing"

	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"
	"kando/packages/remotefs"
)

// fakeLocations is the locations as the app keeps them, plain fields, with the calls the page made.
type fakeLocations struct {
	list    []Location
	status  map[string]Status
	editing bool
	isNew   bool
	draft   Draft
	problem string
	alert   Alert
	inFile  bool
	lcalls  []string
}

func (f *fakeLocations) Locations() []Location           { return f.list }
func (f *fakeLocations) LocationStatus(id string) Status { return f.status[id] }
func (f *fakeLocations) TestLocation(id string)          { f.lcalls = append(f.lcalls, "test "+id) }
func (f *fakeLocations) AddLocation() {
	f.lcalls = append(f.lcalls, "add")
	f.editing, f.isNew, f.draft = true, true, Draft{Kind: string(remotefs.KindSMB)}
}
func (f *fakeLocations) EditLocation(id string)      { f.lcalls = append(f.lcalls, "edit "+id) }
func (f *fakeLocations) AskRemoveLocation(id string) { f.lcalls = append(f.lcalls, "remove "+id) }
func (f *fakeLocations) LocationEditing() bool       { return f.editing }
func (f *fakeLocations) LocationIsNew() bool         { return f.isNew }
func (f *fakeLocations) LocationDraft() *Draft       { return &f.draft }
func (f *fakeLocations) LocationProblem() string     { return f.problem }
func (f *fakeLocations) SaveLocation()               { f.lcalls = append(f.lcalls, "save") }
func (f *fakeLocations) CloseLocation()              { f.lcalls = append(f.lcalls, "close"); f.editing = false }
func (f *fakeLocations) LocationAlert() Alert        { return f.alert }
func (f *fakeLocations) LocationSecretsInFile() bool { return f.inFile }
func (f *fakeLocations) AnswerLocationAlert(yes bool) {
	f.lcalls = append(f.lcalls, fmt.Sprint("answer ", yes))
	f.alert = Alert{}
}

// A draft is tried only with what its kind needs, and becomes a Remote with only the fields of its kind.
func TestDraftProblemsAndRemote(t *testing.T) {
	m := testlocale.Messages()
	for _, c := range []struct {
		name  string
		draft Draft
		want  func(locales.Messages) string
	}{
		{"smb without a host", Draft{Kind: "smb", Share: "films", User: "u"}, locales.Messages.AccountLocationNeedsHost},
		{"smb share with a path", Draft{Kind: "smb", Host: "nas", Share: "films/x", User: "u"}, locales.Messages.AccountLocationNeedsShare},
		{"smb without a user", Draft{Kind: "smb", Host: "nas", Share: "films"}, locales.Messages.AccountLocationNeedsUser},
		{"smb bad port", Draft{Kind: "smb", Host: "nas", Share: "films", User: "u", Port: "70000"}, locales.Messages.AccountLocationNeedsPort},
		{"smb", Draft{Kind: "smb", Host: "nas", Share: "/films/", User: "u", Port: "4450"}, nil},
		{"webdav ftp", Draft{Kind: "webdav", URL: "ftp://nas/dav"}, locales.Messages.AccountLocationNeedsURL},
		{"webdav with a user", Draft{Kind: "webdav", URL: "https://me:pw@nas/dav"}, locales.Messages.AccountLocationNeedsURL},
		{"webdav", Draft{Kind: "webdav", URL: "https://nas/dav"}, nil},
		{"sftp without a secret", Draft{Kind: "sftp", Host: "nas", User: "u"}, locales.Messages.AccountLocationNeedsSecret},
		{"sftp with a key", Draft{Kind: "sftp", Host: "nas", User: "u", PrivateKey: "-----BEGIN"}, nil},
	} {
		got := c.draft.Problem()
		switch {
		case c.want == nil && got != nil:
			t.Errorf("%s: %q", c.name, got(m))
		case c.want != nil && (got == nil || got(m) != c.want(m)):
			t.Errorf("%s: want %q", c.name, c.want(m))
		}
	}
	r := Draft{Kind: "smb", Host: " nas ", Share: "/films/", User: "u", Password: " pw ", Port: "4450", PrivateKey: "stray", URL: "stray"}.Remote()
	if r != (remotefs.Remote{Kind: remotefs.KindSMB, Host: "nas", Port: 4450, Share: "films", User: "u", Password: " pw "}) {
		t.Fatalf("smb remote %+v", r)
	}
	r = Draft{Kind: "sftp", Host: "nas", User: "u", PrivateKey: " key \n", HostKey: "ssh-ed25519 AAAA ", Share: "stray"}.Remote()
	if r != (remotefs.Remote{Kind: remotefs.KindSFTP, Host: "nas", User: "u", PrivateKey: "key", HostKey: "ssh-ed25519 AAAA"}) {
		t.Fatalf("sftp remote %+v", r)
	}
	if d := DraftOf(Location{Name: "n", Remote: r}); d.Kind != "sftp" || d.Port != "" || d.HostKey != r.HostKey || d.Remote() != r {
		t.Fatalf("draft of %+v = %+v", r, d)
	}
	if (Draft{Name: "  "}).NameProblem() == nil || (Draft{Name: "NAS"}).NameProblem() != nil {
		t.Fatal("name problem")
	}
}

// A failure reads as a catalog message, never the library's own words with the server's address in them.
func TestFailureNeverShowsTheLibrarysError(t *testing.T) {
	m := testlocale.Messages()
	r := remotefs.Remote{Kind: remotefs.KindWebDAV, URL: "https://nas.example:8443/dav"}
	secret := errors.New("PROPFIND https://user:hunter2@10.0.0.5/dav: something odd")
	if got := Failure(m, r, secret); got != m.AccountLocationFailOther() || strings.Contains(got, "10.0.0.5") {
		t.Fatalf("other = %q", got)
	}
	if got := Failure(m, r, fmt.Errorf("x: %w", &remotefs.HostKeyError{})); got != m.AccountLocationNotTrusted() {
		t.Fatalf("host key = %q", got)
	}
	if got := Place(r); got != "nas.example:8443/dav" {
		t.Fatalf("place = %q", got)
	}
	if got := Place(remotefs.Remote{Kind: remotefs.KindSMB, Host: "nas", Share: "films", Root: "/kando/"}); got != "nas/films/kando" {
		t.Fatalf("smb place = %q", got)
	}
}

// The card lists each location with its Test, Edit and Remove, says how a Test went, and offers 添加… with or without any.
func TestLocationsCard(t *testing.T) {
	a := &fakeApp{}
	app := show(t, a)
	msgs := testlocale.Messages()
	if !app.Exists(whole(msgs.AccountSectionLocations())) || !app.Exists(whole(msgs.AccountLocationsEmpty())) {
		t.Fatalf("no empty card: %q", app.Texts())
	}
	nas := Location{ID: "n1", Name: "Home NAS", Remote: remotefs.Remote{Kind: remotefs.KindSMB, Host: "nas.local", Share: "films"}}
	change(app, a, func() { a.list = []Location{nas} })
	if app.Exists(whole(msgs.AccountLocationsEmpty())) || !app.Exists(whole("Home NAS")) || !app.Exists(whole(msgs.AccountLocationSummary("SMB", "nas.local/films"))) {
		t.Fatalf("texts %q", app.Texts())
	}
	app.Press("id:location-test-n1")
	app.Press("id:location-edit-n1")
	app.Press("id:location-remove-n1")
	app.Press("id:location-add")
	if got := strings.Join(a.lcalls, ","); got != "test n1,edit n1,remove n1,add" {
		t.Fatalf("calls %q", got)
	}
	change(app, a, func() { a.editing, a.status = false, map[string]Status{"n1": {Busy: true}} })
	if !app.Exists(whole(msgs.AccountLocationTesting())) || app.Enabled("id:location-test-n1") {
		t.Fatalf("busy: %q", app.Texts())
	}
	change(app, a, func() { a.status["n1"] = Status{Err: msgs.AccountLocationFailSignIn()} })
	if !app.Exists(whole(msgs.AccountLocationFailSignIn())) || !app.Enabled("id:location-test-n1") {
		t.Fatalf("failed: %q", app.Texts())
	}
}

// The dialog shows the fields of the kind chosen, writes what is typed into the draft, and saves or tests through the app.
func TestLocationDialog(t *testing.T) {
	a := &fakeApp{}
	app := show(t, a)
	msgs := testlocale.Messages()
	app.Press("id:location-add")
	if !app.Exists("id:location-dialog") || !app.Exists(whole(msgs.AccountLocationAddTitle())) || !app.Exists("id:location-share") || app.Exists("id:location-url") {
		t.Fatalf("smb dialog: %q", app.Texts())
	}
	app.Type("id:location-host", "nas.local")
	if a.draft.Host != "nas.local" {
		t.Fatalf("draft %+v", a.draft)
	}
	change(app, a, func() { a.draft.Kind = string(remotefs.KindSFTP) })
	if app.Exists("id:location-share") || !app.Exists("id:location-key") || !app.Exists(whole(msgs.AccountLocationHostKeyNone())) {
		t.Fatalf("sftp dialog: %q", app.Texts())
	}
	app.Press("id:location-dialog-test")
	app.Press("id:location-dialog-save")
	change(app, a, func() { a.problem = msgs.AccountLocationNeedsName() })
	if !app.Exists(whole(msgs.AccountLocationNeedsName())) {
		t.Fatalf("no problem shown: %q", app.Texts())
	}
	app.Press("id:location-dialog-cancel")
	if got := strings.Join(a.lcalls, ","); got != "add,test ,save,close" || app.Exists("id:location-dialog") {
		t.Fatalf("calls %q", got)
	}
}

// A changed key is warned about with both fingerprints; Return answers Cancel, and only the destructive Replace Key trusts the new key. The first key is asked about with its fingerprint.
func TestLocationKeyAlerts(t *testing.T) {
	a := &fakeApp{}
	app := show(t, a)
	msgs := testlocale.Messages()
	change(app, a, func() {
		a.alert = Alert{Kind: AlertKeyChanged, Host: "nas.local", Fingerprint: "SHA256:new", Trusted: "SHA256:old"}
	})
	if !app.Exists("id:location-key-changed") || !app.Exists(whole(msgs.AccountLocationKeyChangedMessage("SHA256:old", "SHA256:new"))) || app.Exists("id:location-alert-trust") {
		t.Fatalf("warning: %q", app.Texts())
	}
	app.Key("enter")
	if got := strings.Join(a.lcalls, ","); got != "answer false" || app.Exists("id:location-key-changed") {
		t.Fatalf("Return answered %q", got)
	}
	change(app, a, func() {
		a.alert = Alert{Kind: AlertKeyChanged, Host: "nas.local", Fingerprint: "SHA256:new", Trusted: "SHA256:old"}
	})
	app.Press("id:location-alert-replace")
	change(app, a, func() { a.alert = Alert{Kind: AlertTrust, Host: "nas.local", Fingerprint: "SHA256:first"} })
	if !app.Exists(whole(msgs.AccountLocationTrustTitle("nas.local"))) || !app.Exists(whole(msgs.AccountLocationTrustMessage("SHA256:first"))) {
		t.Fatalf("trust: %q", app.Texts())
	}
	app.Press("id:location-alert-trust")
	if got := strings.Join(a.lcalls, ","); got != "answer false,answer true,answer true" {
		t.Fatalf("calls %q", got)
	}
}
