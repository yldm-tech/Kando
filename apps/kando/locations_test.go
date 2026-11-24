package main

import (
	"crypto/ed25519"
	"crypto/rand"
	"encoding/json"
	"errors"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"runtime"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"kando/apps/kando/account"
	"kando/apps/kando/feedback"
	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/settings"
	"kando/apps/kando/storage"

	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/pkg/sftp"
	"golang.org/x/crypto/ssh"
	"golang.org/x/net/webdav"
)

// fakeLocationDAV is a WebDAV server over a new folder, under /dav and behind basic authentication as kando with the password secret.
func fakeLocationDAV(t *testing.T) (url, dir string) {
	t.Helper()
	dir = t.TempDir()
	dav := &webdav.Handler{Prefix: "/dav", FileSystem: webdav.Dir(dir), LockSystem: webdav.NewMemLS()}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if user, password, ok := r.BasicAuth(); !ok || user != "kando" || password != "secret" {
			w.Header().Set("WWW-Authenticate", `Basic realm="nas"`)
			http.Error(w, "who are you", http.StatusUnauthorized)
			return
		}
		dav.ServeHTTP(w, r)
	}))
	t.Cleanup(srv.Close)
	return srv.URL + "/dav", dir
}

// fakeLocationSSH is an SFTP server on loopback over a new folder, with a new ed25519 host key, that lets in kando with the password secret. setKey gives it another host key, as a reinstalled server has.
type fakeLocationSSH struct {
	host string
	port int
	home string

	mu     sync.Mutex
	config *ssh.ServerConfig
	key    string
}

func newFakeLocationSSH(t *testing.T) *fakeLocationSSH {
	t.Helper()
	f := &fakeLocationSSH{home: t.TempDir()}
	f.setKey(t)
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatal(err)
	}
	host, port, _ := net.SplitHostPort(ln.Addr().String())
	f.host = host
	f.port, _ = strconv.Atoi(port)
	var wg sync.WaitGroup
	t.Cleanup(func() {
		ln.Close()
		wg.Wait()
	})
	wg.Go(func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			f.mu.Lock()
			config := f.config
			f.mu.Unlock()
			wg.Go(func() { serveLocationSSH(conn, config, f.home) })
		}
	})
	return f
}

// setKey gives the server a new host key.
func (f *fakeLocationSSH) setKey(t *testing.T) {
	t.Helper()
	_, key, err := ed25519.GenerateKey(rand.Reader)
	if err != nil {
		t.Fatal(err)
	}
	signer, err := ssh.NewSignerFromKey(key)
	if err != nil {
		t.Fatal(err)
	}
	config := &ssh.ServerConfig{PasswordCallback: func(c ssh.ConnMetadata, password []byte) (*ssh.Permissions, error) {
		if c.User() == "kando" && string(password) == "secret" {
			return nil, nil
		}
		return nil, errors.New("wrong password")
	}}
	config.AddHostKey(signer)
	f.mu.Lock()
	f.config, f.key = config, strings.TrimSpace(string(ssh.MarshalAuthorizedKey(signer.PublicKey())))
	f.mu.Unlock()
}

func (f *fakeLocationSSH) hostKey() string {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.key
}

// serveLocationSSH answers one connection: a session that asks for the sftp subsystem gets an SFTP server over home.
func serveLocationSSH(conn net.Conn, config *ssh.ServerConfig, home string) {
	defer conn.Close()
	_, chans, reqs, err := ssh.NewServerConn(conn, config)
	if err != nil {
		return
	}
	go ssh.DiscardRequests(reqs)
	for nc := range chans {
		if nc.ChannelType() != "session" {
			nc.Reject(ssh.UnknownChannelType, "only sessions")
			continue
		}
		ch, requests, err := nc.Accept()
		if err != nil {
			return
		}
		go func() {
			for req := range requests {
				ok := req.Type == "subsystem" && len(req.Payload) > 4 && string(req.Payload[4:]) == "sftp"
				req.Reply(ok, nil)
				if ok {
					go func() {
						defer ch.Close()
						server, err := sftp.NewServer(ch, sftp.WithServerWorkingDirectory(home))
						if err != nil {
							return
						}
						if err := server.Serve(); err != nil && !errors.Is(err, io.EOF) {
							return
						}
					}()
				}
			}
		}()
	}
}

// A location added in the dialog is saved in settings.json without its password, which the system's secure store keeps, read back with it at the next start, edited and removed, which forgets the password; the file stays readable by its owner alone.
func TestLocationsPersistAndReload(t *testing.T) {
	a, tt, updates := newTestApp(t, &fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	msgs := testlocale.Messages()
	tt = settingsOpenAt(a, tt, settings.SectionAccount)
	wantText(t, tt, msgs.AccountLocationsEmpty())
	wantText(t, tt, msgs.AccountLocationsNote())
	tt.Press("id:location-add")
	if !a.modalOpen() {
		t.Fatal("the dialog holds no modal flag")
	}
	tt.Press("id:location-dialog-save")
	wantText(t, tt, msgs.AccountLocationNeedsName())
	tt.Type("id:location-name", "Home NAS")
	tt.Type("id:location-host", "nas.local")
	tt.Type("id:location-share", "films")
	tt.Type("id:location-user", "kando")
	tt.Type("id:location-password", "pa55-w0rd!")
	tt.Press("id:location-dialog-save")
	if a.locations.editing || a.modalOpen() || len(a.locations.list) != 1 {
		t.Fatalf("editing %v modal %v list %+v", a.locations.editing, a.modalOpen(), a.locations.list)
	}
	locationSecretsSettle(t, tt, a, updates)
	id := a.locations.list[0].ID
	wantText(t, tt, "Home NAS")
	wantText(t, tt, msgs.AccountLocationSummary("SMB", "nas.local/films"))
	wantText(t, tt, msgs.AccountLocationsNote())
	raw, err := os.ReadFile(a.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "pa55-w0rd!") || !strings.Contains(string(raw), `"storageLocations"`) {
		t.Fatalf("settings.json: %s", raw)
	}
	if got := locationStored(t, a, id, "password"); got != "pa55-w0rd!" {
		t.Fatalf("the secure store keeps %q", got)
	}

	// The next start reads the password from the secure store, once the window exists.
	b, bUpdates := restartedLocations(t, a)
	want := account.Location{ID: id, Name: "Home NAS", Remote: storage.Remote{Kind: storage.KindSMB, Host: "nas.local", Share: "films", User: "kando"}}
	if len(b.locations.list) != 1 || b.locations.list[0] != want {
		t.Fatalf("reloaded %+v", b.locations.list)
	}
	b.locationsHooks().ready()
	applyLocationSecrets(t, b, bUpdates)
	want.Remote.Password = "pa55-w0rd!"
	if len(b.locations.list) != 1 || b.locations.list[0] != want || b.locations.locationSecretsInFile() {
		t.Fatalf("reloaded with the store %+v, in the file %v", b.locations.list, b.locations.locationSecretsInFile())
	}
	if !slices.Contains(b.feedbackSecrets(), "pa55-w0rd!") {
		t.Fatal("the password read from the store is not left out of the log and feedback")
	}

	tt.Press("id:location-edit-" + id)
	if a.LocationIsNew() || a.LocationDraft().Password != "pa55-w0rd!" {
		t.Fatalf("draft %+v", *a.LocationDraft())
	}
	tt.Type("id:location-root", "kando")
	tt.Press("id:location-dialog-save")
	locationSecretsSettle(t, tt, a, updates)
	if got := loadSavedSettings(a.settingsPath).StorageLocations; len(got) != 1 || got[0].ID != id || got[0].Root != "kando" || got[0].Password != "" {
		t.Fatalf("edited %+v", got)
	}
	if got := locationStored(t, a, id, "password"); got != "pa55-w0rd!" {
		t.Fatalf("after the edit the secure store keeps %q", got)
	}

	tt.Press("id:location-remove-" + id)
	if !tt.Exists("id:location-remove-confirm") || !a.modalOpen() {
		t.Fatal("no confirmation")
	}
	tt.Press("id:location-alert-cancel")
	if len(a.locations.list) != 1 || a.modalOpen() {
		t.Fatal("Cancel removed the location")
	}
	tt.Press("id:location-remove-" + id)
	tt.Press("id:location-alert-remove")
	if len(a.locations.list) != 0 || a.modalOpen() {
		t.Fatalf("list %+v", a.locations.list)
	}
	locationSecretsSettle(t, tt, a, updates)
	raw, err = os.ReadFile(a.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "pa55-w0rd!") || strings.Contains(string(raw), "storageLocations") {
		t.Fatalf("removed location still in settings.json: %s", raw)
	}
	if got := locationStored(t, a, id, "password"); got != "" {
		t.Fatalf("the removed location's password is still in the secure store: %q", got)
	}
}

// A settings.json an earlier version wrote, with a location's password, private key and passphrase in it, keeps them until the secure store took them, then is saved without them; the location keeps them all along.
func TestLocationSecretsMoveIntoTheSecureStore(t *testing.T) {
	a, tt, updates := newTestApp(t, &fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	old := savedLocation{ID: "nas", Name: "NAS", Kind: "sftp", Host: "nas.local", User: "kando", Password: "pa55", PrivateKey: "-----KEY-----", Passphrase: "phrase"}
	writeLocationSettings(t, a.settingsPath, old)
	a.restoreSettings(loadSavedSettings(a.settingsPath))
	if !a.locations.locationSecretsInFile() {
		t.Fatal("the secrets an earlier version saved are not taken as in the settings file")
	}
	// Until the store took them, saving settings.json keeps them there.
	if err := a.saveSettings(); err != nil {
		t.Fatal(err)
	}
	if got := loadSavedSettings(a.settingsPath).StorageLocations; len(got) != 1 || got[0] != old {
		t.Fatalf("saved before the store took them %+v", got)
	}

	a.locationsHooks().ready()
	locationSecretsSettle(t, tt, a, updates)
	if got := loadSavedSettings(a.settingsPath).StorageLocations; len(got) != 1 || got[0].Password != "" || got[0].PrivateKey != "" || got[0].Passphrase != "" || got[0].Host != "nas.local" {
		t.Fatalf("saved once the store took them %+v", got)
	}
	for key, want := range map[string]string{"password": "pa55", "privateKey": "-----KEY-----", "passphrase": "phrase"} {
		if got := locationStored(t, a, "nas", key); got != want {
			t.Errorf("the secure store keeps %s %q", key, got)
		}
	}
	if r := a.locations.list[0].Remote; r.Password != "pa55" || r.PrivateKey != "-----KEY-----" || r.Passphrase != "phrase" {
		t.Fatalf("the location lost its secrets: %+v", r)
	}
	if a.locations.locationSecretsInFile() {
		t.Fatal("still taken as in the settings file")
	}
}

// Without a secure store, as on Linux without a Secret Service, a location's secrets stay in settings.json, those an earlier version saved as those typed in the dialog, and the page says where they are.
func TestLocationSecretsStayInTheFileWithoutASecureStore(t *testing.T) {
	a, tt, updates := newTestApp(t, &fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.locations.secrets = func() ns.Secrets { return ns.Secrets{} }
	msgs := testlocale.Messages()
	old := savedLocation{ID: "old", Name: "Old NAS", Kind: "smb", Host: "old.local", Share: "films", User: "kando", Password: "old-pa55"}
	writeLocationSettings(t, a.settingsPath, old)
	a.restoreSettings(loadSavedSettings(a.settingsPath))
	a.locationsHooks().ready()
	locationSecretsSettle(t, tt, a, updates)
	if got := loadSavedSettings(a.settingsPath).StorageLocations; len(got) != 1 || got[0] != old {
		t.Fatalf("an earlier version's secrets were not kept %+v", got)
	}

	tt = settingsOpenAt(a, tt, settings.SectionAccount)
	wantText(t, tt, msgs.AccountLocationsNoteInFile())
	tt.Press("id:location-add")
	tt.Type("id:location-name", "Home NAS")
	tt.Type("id:location-host", "nas.local")
	tt.Type("id:location-share", "films")
	tt.Type("id:location-user", "kando")
	tt.Type("id:location-password", "pa55-w0rd!")
	tt.Press("id:location-dialog-save")
	locationSecretsSettle(t, tt, a, updates)
	got := loadSavedSettings(a.settingsPath).StorageLocations
	if len(got) != 2 || got[0] != old || got[1].Password != "pa55-w0rd!" {
		t.Fatalf("settings.json %+v", got)
	}
	wantText(t, tt, msgs.AccountLocationsNoteInFile())
	info, err := os.Stat(a.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("settings.json mode %v", info.Mode().Perm())
	}
}

// writeLocationSettings writes a settings.json with the locations given, as an earlier version saved them.
func writeLocationSettings(t *testing.T, path string, locations ...savedLocation) {
	t.Helper()
	raw, err := json.Marshal(map[string]any{"storageLocations": locations})
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, raw, 0o600); err != nil {
		t.Fatal(err)
	}
}

// locationStored is what the app's secure store keeps for the location id's secret key, "" for nothing.
func locationStored(t *testing.T, a *app, id, key string) string {
	t.Helper()
	v, _, err := a.locationSecretStore().Get(locationSecretAccount(id, key))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// restartedLocations is a new app with a's settings.json and secure store, as at the next start, before its window exists, and the channel its background updates arrive on.
func restartedLocations(t *testing.T, a *app) (*app, chan func()) {
	t.Helper()
	updates := make(chan func(), 16)
	b := newApp(&fakePipeline{}, func(fn func()) { updates <- fn })
	store := a.locationSecretStore()
	b.locations.secrets = func() ns.Secrets { return store }
	b.restoreSettings(loadSavedSettings(a.settingsPath))
	return b, updates
}

// applyLocationSecrets applies the updates of b, an app with no window, until its store's work is done.
func applyLocationSecrets(t *testing.T, b *app, updates chan func()) {
	t.Helper()
	deadline := time.After(10 * time.Second)
	for b.locations.secretsBusy > 0 {
		select {
		case fn := <-updates:
			fn()
		case <-deadline:
			t.Fatal("the secure store's work did not end")
		}
	}
}

// locationSecretsSettle applies the background updates until the secure store's work on the locations' secrets is done.
func locationSecretsSettle(t *testing.T, tt *nsgotest.App, a *app, updates chan func()) {
	t.Helper()
	waitUntil(t, tt, a, updates, func() bool { return a.locations.secretsBusy == 0 })
}

// The dialog closes with the page that draws it, and gives back its modal flag.
func TestLocationDialogClosesWithTheSettings(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	tt = settingsOpenAt(a, tt, settings.SectionAccount)
	tt.Press("id:location-add")
	a.OpenSettings(settings.SectionGeneral)
	refresh(tt, a)
	if a.locations.editing || a.modalOpen() {
		t.Fatalf("editing %v modal %v", a.locations.editing, a.modalOpen())
	}
}

// settings.json's locations are tidied as they are read: one without an ID, a name or a known kind, or with an ID seen before, is dropped.
func TestSavedLocationsClean(t *testing.T) {
	s := savedLocations{StorageLocations: []savedLocation{
		{ID: "a", Name: " NAS ", Kind: "smb", Host: " nas ", Password: " keep "},
		{ID: "a", Name: "again", Kind: "smb"},
		{ID: "b", Name: "", Kind: "smb"},
		{ID: "c", Name: "ftp", Kind: "ftp"},
		{ID: "", Name: "no id", Kind: "sftp"},
	}}
	s.clean()
	if len(s.StorageLocations) != 1 || s.StorageLocations[0] != (savedLocation{ID: "a", Name: "NAS", Kind: "smb", Host: "nas", Password: " keep "}) {
		t.Fatalf("clean %+v", s.StorageLocations)
	}
}

// Every value of a saved location but its name, kind, folder, port, ID and the server's public key is a secret the log and the feedback issue leave out, whichever field a later change adds.
func TestLocationSecretsAreScrubbed(t *testing.T) {
	a := newApp(&fakePipeline{}, func(func()) {})
	var saved savedSettings
	loc := savedLocation{ID: "id1", Name: "Home NAS", Kind: "sftp"}
	v := reflect.ValueOf(&loc).Elem()
	var secrets []string
	for i := range v.NumField() {
		switch name := v.Type().Field(i).Name; {
		case v.Field(i).Kind() != reflect.String, name == "ID", name == "Name", name == "Kind", name == "Root", name == "HostKey":
			continue
		default:
			value := "7c6b5a4938271605" + strconv.Itoa(i) + "x" + strings.ToLower(name)
			v.Field(i).SetString(value)
			secrets = append(secrets, value)
		}
	}
	saved.StorageLocations = []savedLocation{loc}
	a.restoreLocations(saved)
	text := "connect failed: " + strings.Join(secrets, " | ")
	for _, s := range []feedback.Scrubber{{Secrets: a.feedbackSecrets()}, a.feedbackScrubber()} {
		out := s.NoSecrets(text)
		for _, secret := range secrets {
			if strings.Contains(out, secret) {
				t.Fatalf("%q leaked into %q", secret, out)
			}
		}
	}
}

// Test connects to a WebDAV server off the engine's goroutine and says it connected; a wrong password reads as the catalog's message, in the language shown, never the library's error.
func TestLocationTestAgainstWebDAV(t *testing.T) {
	url, dir := fakeLocationDAV(t)
	if err := os.Mkdir(filepath.Join(dir, "films"), 0o755); err != nil {
		t.Fatal(err)
	}
	a, tt, updates := newTestApp(t, &fakePipeline{})
	a.locations.list = []account.Location{
		{ID: "good", Name: "DAV", Remote: storage.Remote{Kind: storage.KindWebDAV, URL: url, Root: "films", User: "kando", Password: "secret"}},
		{ID: "bad", Name: "DAV wrong", Remote: storage.Remote{Kind: storage.KindWebDAV, URL: url, User: "kando", Password: "wrong"}},
		{ID: "gone", Name: "DAV missing", Remote: storage.Remote{Kind: storage.KindWebDAV, URL: url, Root: "missing", User: "kando", Password: "secret"}},
	}
	msgs := testlocale.Messages()
	tt = settingsOpenAt(a, tt, settings.SectionAccount)
	tt.Press("id:location-test-good")
	wantText(t, tt, msgs.AccountLocationTesting())
	waitUpdate(t, tt, a, updates)
	wantText(t, tt, msgs.AccountLocationConnected())
	tt.Press("id:location-test-bad")
	waitUpdate(t, tt, a, updates)
	wantText(t, tt, msgs.AccountLocationFailSignIn())
	if got := tt.Find("id:location-status-bad").Text; strings.Contains(got, "401") || strings.Contains(got, "127.0.0.1") {
		t.Fatalf("the library's error shows: %q", got)
	}
	tt.Press("id:location-test-gone")
	waitUpdate(t, tt, a, updates)
	wantText(t, tt, msgs.AccountLocationFailNoRoot())
	a.SetInterfaceLanguage("en")
	refresh(tt, a)
	wantText(t, tt, locales.In("en").AccountLocationFailSignIn())
}

// An SFTP server's key is trusted on first use: the first Test asks with the key's fingerprint, and once trusted the key goes into the draft, which Save keeps, and the Test runs again and connects. A key that changed is warned about with both fingerprints; Return keeps the old key and says the server is not trusted, and only Replace Key takes the new one.
func TestLocationTrustsTheSFTPHostKey(t *testing.T) {
	f := newFakeLocationSSH(t)
	a, tt, updates := newTestApp(t, &fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	msgs := testlocale.Messages()
	tt = settingsOpenAt(a, tt, settings.SectionAccount)
	tt.Press("id:location-add")
	apply(tt, a, func() {
		*a.LocationDraft() = account.Draft{Name: "Server", Kind: string(storage.KindSFTP), Host: f.host, Port: strconv.Itoa(f.port), User: "kando", Password: "secret"}
	})
	tt.Press("id:location-dialog-test")
	waitUpdate(t, tt, a, updates)
	first := storage.Fingerprint(f.hostKey())
	if !tt.Exists("id:location-trust") || !hasText(tt, msgs.AccountLocationTrustMessage(first)) || !hasText(tt, msgs.AccountLocationTrustTitle(f.host)) {
		t.Fatalf("no trust question: %q", tt.Texts())
	}
	if a.LocationDraft().HostKey != "" {
		t.Fatal("the key was trusted before the answer")
	}
	tt.Press("id:location-alert-trust")
	if a.LocationDraft().HostKey != f.hostKey() {
		t.Fatalf("draft key %q", a.LocationDraft().HostKey)
	}
	waitUpdate(t, tt, a, updates)
	if got := tt.Find("id:location-dialog-status").Text; got != msgs.AccountLocationConnected() {
		t.Fatalf("status %q", got)
	}
	wantText(t, tt, first)
	tt.Press("id:location-dialog-save")
	if len(a.locations.list) != 1 || a.locations.list[0].Remote.HostKey != f.hostKey() || a.modalOpen() {
		t.Fatalf("saved %+v, modal %v", a.locations.list, a.modalOpen())
	}
	locationSecretsSettle(t, tt, a, updates)
	id := a.locations.list[0].ID

	old := f.hostKey()
	f.setKey(t)
	tt.Press("id:location-test-" + id)
	waitUpdate(t, tt, a, updates)
	if !tt.Exists("id:location-key-changed") || tt.Exists("id:location-alert-trust") {
		t.Fatalf("no warning: %q", tt.Texts())
	}
	wantText(t, tt, msgs.AccountLocationKeyChangedMessage(storage.Fingerprint(old), storage.Fingerprint(f.hostKey())))
	tt.Key("enter")
	if a.locations.list[0].Remote.HostKey != old || tt.Exists("id:location-key-changed") || a.modalOpen() {
		t.Fatalf("Return replaced the key or left the warning: %q", a.locations.list[0].Remote.HostKey)
	}
	wantText(t, tt, msgs.AccountLocationNotTrusted())
	tt.Press("id:location-test-" + id)
	waitUpdate(t, tt, a, updates)
	tt.Press("id:location-alert-replace")
	if a.locations.list[0].Remote.HostKey != f.hostKey() {
		t.Fatal("Replace Key kept the old key")
	}
	if got := loadSavedSettings(a.settingsPath).StorageLocations; len(got) != 1 || got[0].HostKey != f.hostKey() {
		t.Fatalf("saved %+v", got)
	}
	waitUpdate(t, tt, a, updates)
	wantText(t, tt, msgs.AccountLocationConnected())
}
