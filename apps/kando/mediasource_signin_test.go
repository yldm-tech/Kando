package main

import (
	"context"
	"net/url"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/mediasource"
	"kando/apps/kando/mediasource/plexlink/plexlinktest"
	"kando/apps/kando/settings"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// plexTestKindID is the kind of source signInApp registers in place of the official Plex plugin's, whose token field signs in to Plex.
var plexTestKindID = mediasource.PluginKindID("kando.plex", "plex")

// signInApp starts the app with a kind of source whose token Kando fills by signing in to Plex, connected to through src, and the dialog of 设置 › 媒体来源 open on a new source of it; the fake plex.tv tv is asked every 10 ms.
func signInApp(t *testing.T, tv *plexlinktest.TV) (*app, *nsgotest.App, chan func(), *fakeSource) {
	t.Helper()
	a, tt, updates := newTestApp(t, &fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	src := newFakeSource(map[string]fakeSourceFile{"Films/Heat.mkv": {data: "heat"}})
	a.sources.open = src.open
	title := func(s string) func(string) string { return func(string) string { return s } }
	k := mediasource.Kind{ID: plexTestKindID, Title: title("Plex"), Icon: "film", Origin: mediasource.Origin{Plugin: "kando.plex"},
		Open: func(context.Context, mediasource.Source, map[string]string) (mediasource.Driver, error) {
			return nil, context.Canceled
		},
		Fields: []mediasource.Field{
			{Key: mediasource.EndpointKey, Type: mediasource.FieldURL, Title: title("Server address")},
			{Key: "token", Type: mediasource.FieldSecret, Title: title("Token"), Auth: &mediasource.Auth{Header: "X-Plex-Token", Format: "{secret}", SignIn: mediasource.SignInPlex}},
		}}
	if err := a.sources.kinds.Register(k); err != nil {
		t.Fatal(err)
	}
	a.sources.signIn.api, a.sources.signIn.every = tv.URL, 10*time.Millisecond
	tt = settingsOpenAt(a, tt, settings.SectionMediaSources)
	tt.Press("id:media-source-add")
	onEngine(a, func() { a.SetMediaSourceKind(plexTestKindID) })
	refresh(tt, a)
	return a, tt, updates, src
}

// signInPage is the code of the PIN of the sign-in page Kando opened last, after checking that it is plex.tv's, for a client that names itself and Kando.
func signInPage(t *testing.T, tt *nsgotest.App) string {
	t.Helper()
	if len(tt.OpenedURLs) == 0 {
		t.Fatal("no page opened")
	}
	u, err := url.Parse(tt.OpenedURLs[len(tt.OpenedURLs)-1])
	if err != nil || u.Scheme != "https" || u.Host != "app.plex.tv" || u.Path != "/auth" {
		t.Fatalf("opened %v (%v)", u, err)
	}
	q, err := url.ParseQuery(strings.TrimPrefix(u.Fragment, "?"))
	if err != nil || q.Get("clientID") == "" || q.Get("code") == "" || q.Get("context[device][product]") != testlocale.Messages().AppName() {
		t.Fatalf("the page asks %v (%v)", q, err)
	}
	return q.Get("code")
}

// TestMediaSourceSignsInToPlex checks docs/media-sources.md 11.5: a Plex source's dialog offers Sign In with Plex, which opens plex.tv's page in the browser and waits there; once the person signed in, the draft is filled with the address of the account's own server that answers, not one that does not, and the token that server takes, named after it, and tested; the account's other servers are offered, choosing one fills the draft with its address and token; and Save keeps the token in the secure store, not in settings.json.
func TestMediaSourceSignsInToPlex(t *testing.T) {
	m := testlocale.Messages()
	nas := plexlinktest.Identity(t, "nas")
	friend := plexlinktest.Identity(t, "friend")
	tv := plexlinktest.New(t, "account-token")
	tv.Resources = []plexlinktest.Resource{
		{Name: "Friend's", ClientIdentifier: "friend", Provides: "server", AccessToken: "friend-token", Connections: []plexlinktest.Connection{
			{Protocol: "http", URI: friend.URL},
		}},
		{Name: "nas", ClientIdentifier: "nas", Provides: "server", Owned: true, AccessToken: "nas-token", Connections: []plexlinktest.Connection{
			{Protocol: "http", URI: "http://127.0.0.1:1", Local: true},
			{Protocol: "http", URI: nas.URL, Local: true},
		}},
	}
	a, tt, updates, src := signInApp(t, tv)
	if !tt.Exists("id:media-source-sign-in") || tt.Exists("id:media-source-sign-in-server") {
		t.Fatal("the dialog does not offer to sign in, or offers servers before it")
	}
	tt.Press("id:media-source-sign-in")
	waitUpdate(t, tt, a, updates)
	code := signInPage(t, tt)
	wantText(t, tt, m.SettingsSourcesPlexWaiting())
	if !tv.SignIn(code) {
		t.Fatalf("plex.tv gave out no PIN %s", code)
	}
	waitUntil(t, tt, a, updates, func() bool {
		return !a.sources.signIn.busy && a.MediaSourceStatus("").OK
	})
	d := a.MediaSourceDraft()
	if d.Name != "nas" || d.Values[mediasource.EndpointKey] != nas.URL || d.Values["token"] != "nas-token" {
		t.Fatalf("the draft %+v", *d)
	}
	if name, address := tt.Find("id:media-source-name").Text, tt.Find("id:media-source-field-endpoint").Text; name != "nas" || address != nas.URL {
		t.Errorf("the fields show %q %q", name, address)
	}
	if got := src.lastSecrets()["token"]; got != "nas-token" {
		t.Errorf("tested with the token %q", got)
	}
	wantText(t, tt, m.SettingsSourcesPlexSignedIn("nas"))
	if !tt.Exists("id:media-source-sign-in-server") {
		t.Fatal("the account's servers are not offered")
	}

	onEngine(a, func() { a.ChooseMediaSourceSignInServer("friend") })
	waitUntil(t, tt, a, updates, func() bool { return a.MediaSourceStatus("").OK })
	if d.Name != "Friend's" || d.Values[mediasource.EndpointKey] != friend.URL || d.Values["token"] != "friend-token" {
		t.Fatalf("another server: the draft %+v", *d)
	}
	// A name the person typed is theirs.
	onEngine(a, func() { d.Name = "Shared films"; a.ChooseMediaSourceSignInServer("nas") })
	waitUntil(t, tt, a, updates, func() bool { return a.MediaSourceStatus("").OK })
	if d.Name != "Shared films" || d.Values["token"] != "nas-token" {
		t.Fatalf("a name typed: the draft %+v", *d)
	}

	tt.Press("id:media-source-dialog-save")
	waitUntil(t, tt, a, updates, func() bool { return !a.sources.editing })
	if len(a.sources.list) != 1 || a.sources.list[0].Config[mediasource.EndpointKey] != nas.URL || a.sources.list[0].Config["token"] != "" {
		t.Fatalf("saved %+v", a.sources.list)
	}
	if v, ok, err := a.mediaSourceSecretStore().Get(mediaSourceSecretAccount(a.sources.list[0].ID, "token")); err != nil || !ok || v != "nas-token" {
		t.Errorf("the secure store keeps %q %v %v", v, ok, err)
	}
}

// TestMediaSourcePlexSignInFails checks what the dialog says when signing in to Plex does not fill the draft: offline mode, which asks plex.tv nothing; a PIN that expired; an account without a server; and that a sign-in stops with the dialog, which a late answer then does not fill.
func TestMediaSourcePlexSignInFails(t *testing.T) {
	m := testlocale.Messages()
	tv := plexlinktest.New(t, "account-token")
	a, tt, updates, _ := signInApp(t, tv)

	a.privacy.offline = true
	refresh(tt, a)
	tt.Press("id:media-source-sign-in")
	wantText(t, tt, m.SettingsSourcesPlexOffline())
	if len(tt.OpenedURLs) != 0 || tv.Count("POST", "/api/v2/pins") != 0 {
		t.Fatal("asked plex.tv in offline mode")
	}
	a.privacy.offline = false
	refresh(tt, a)

	tt.Press("id:media-source-sign-in")
	waitUpdate(t, tt, a, updates)
	tv.Expire()
	waitUntil(t, tt, a, updates, func() bool { return !a.sources.signIn.busy })
	wantText(t, tt, m.SettingsSourcesPlexExpired())

	tv2 := plexlinktest.New(t, "account-token")
	a.sources.signIn.api = tv2.URL
	tt.Press("id:media-source-sign-in")
	waitUpdate(t, tt, a, updates)
	tv2.SignIn(signInPage(t, tt))
	waitUntil(t, tt, a, updates, func() bool { return !a.sources.signIn.busy })
	wantText(t, tt, m.SettingsSourcesPlexNoServer())
	if a.MediaSourceDraft().Values[mediasource.EndpointKey] != "" {
		t.Errorf("an account without a server filled %+v", a.MediaSourceDraft().Values)
	}

	tt.Press("id:media-source-sign-in")
	waitUpdate(t, tt, a, updates)
	tv2.Resources = []plexlinktest.Resource{{Name: "nas", ClientIdentifier: "nas", Provides: "server", Owned: true, Connections: []plexlinktest.Connection{{Protocol: "http", URI: "http://127.0.0.1:1", Local: true}}}}
	tt.Press("id:media-source-dialog-cancel")
	if a.sources.signIn.busy || a.sources.signIn.cancel != nil {
		t.Fatal("the sign-in goes on with the dialog closed")
	}
	tv2.SignIn(signInPage(t, tt))
	settle(t, a, tt, updates)
	if a.sources.editing || len(a.sources.signIn.servers) != 0 {
		t.Errorf("a sign-in of a closed dialog came back: %+v", a.sources.signIn)
	}
}
