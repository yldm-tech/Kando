package main

import (
	"context"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"slices"
	"strings"
	"sync"
	"sync/atomic"
	"testing"

	"kando/apps/kando/account"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/mediasource"
	"kando/apps/kando/plugins"
	"kando/apps/kando/settings"

	"github.com/nsgo-dev/nsgo/nsgotest"
	"golang.org/x/net/webdav"
)

// pluginNASLiveCode is acme.nas' code in the tests of its source: a WebDAV client that lists a folder with PROPFIND and reads a file from its path, through kando.source.fetch alone, with probe, which sends a request as the test says and answers the reply or the code it was refused with.
const pluginNASLiveCode = `import { host } from "nsgo"
function fail(code, message) { const e = new Error(message); e.code = code; throw e }
async function dav(source, method, path, depth) {
  let r
  try {
    r = await host.call("kando.source.fetch", { source: source.id, method, path, header: { Depth: depth } }, { timeout: 150000 })
  } catch (e) { fail((e && e.code) || "unavailable", "fetch") }
  if (r.status === 401 || r.status === 403) fail("auth", "refused")
  if (r.status === 404) return null
  if (r.status !== 207) fail("unavailable", "status " + r.status)
  return r.body
}
function between(s, a, b) {
  const i = s.indexOf(a)
  if (i < 0) return ""
  const j = s.indexOf(b, i + a.length)
  return j < 0 ? "" : s.slice(i + a.length, j)
}
function entry(chunk) {
  const href = decodeURIComponent(between(chunk, "<D:href>", "</D:href>")).replace(/\/$/, "")
  const name = href.slice(href.lastIndexOf("/") + 1)
  const size = between(chunk, "<D:getcontentlength>", "</D:getcontentlength>")
  return { name, dir: chunk.indexOf("<D:collection") >= 0, size: size ? Number(size) : 0, version: between(chunk, "<D:getetag>", "</D:getetag>") }
}
export async function list(source, dir) {
  const xml = await dav(source, "PROPFIND", dir, "1")
  if (xml === null) fail("not-found", dir)
  return xml.split("<D:response>").slice(2).map(entry)
}
export async function stat(source, path) {
  const xml = await dav(source, "PROPFIND", path, "0")
  return xml === null ? null : entry(xml.split("<D:response>")[1])
}
export async function open(source, path) { return { path } }
let seen = []
export async function probe(source, request) {
  try {
    const r = await host.call("kando.source.fetch", Object.assign({ source }, request), { timeout: 150000 })
    seen.push(JSON.stringify(r))
    return r.status
  } catch (e) { return "refused " + (e && e.code) }
}
export function said() { return seen.join("\n") }
`

// sourceServer is a WebDAV server of films/talk.mp4 under /dav/ that takes the user kando with the password secret alone, and the passwords it heard.
type sourceServer struct {
	*httptest.Server
	mu    sync.Mutex
	heard []string
}

func newSourceServer(t *testing.T, tls bool) *sourceServer {
	t.Helper()
	fs := webdav.NewMemFS()
	ctx := context.Background()
	if err := fs.Mkdir(ctx, "/films", 0o755); err != nil {
		t.Fatal(err)
	}
	f, err := fs.OpenFile(ctx, "/films/talk.mp4", os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatal(err)
	}
	io.WriteString(f, "0123456789")
	f.Close()
	s := &sourceServer{}
	dav := &webdav.Handler{Prefix: "/dav", FileSystem: fs, LockSystem: webdav.NewMemLS()}
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		user, pass, _ := r.BasicAuth()
		s.mu.Lock()
		s.heard = append(s.heard, user+":"+pass)
		s.mu.Unlock()
		if user != "kando" || pass != "secret" {
			w.WriteHeader(http.StatusUnauthorized)
			return
		}
		dav.ServeHTTP(w, r)
	})
	if tls {
		s.Server = httptest.NewTLSServer(handler)
	} else {
		s.Server = httptest.NewServer(handler)
	}
	t.Cleanup(s.Close)
	return s
}

// sourcesApp starts the app with acme.nas installed and turned on, granted kando.source.fetch.
func sourcesApp(t *testing.T) (*app, *nsgotest.App, chan func()) {
	t.Helper()
	key := newPluginsKey(t)
	a, tt, updates, _ := scriptsApp(t, key, pluginScriptFolderAbout(t, key, "acme.nas", "1.0.0", pluginNASAbout, pluginNASFields, pluginNASLiveCode))
	scriptsGrant(t, a, tt, updates, "acme.nas", string(plugins.PermSourceFetch))
	refresh(tt, a)
	return a, tt, updates
}

// pluginKindID is acme.nas' kind of source.
const pluginKindID = "plugin:acme.nas:webdav"

// sourcesDraft opens 设置 › 媒体来源's dialog for a new source of acme.nas' kind at the server's /dav, the user kando with password.
func sourcesDraft(t *testing.T, a *app, tt *nsgotest.App, server, password string) *nsgotest.App {
	t.Helper()
	tt = settingsOpenAt(a, tt, settings.SectionMediaSources)
	onEngine(a, func() {
		a.AddMediaSource()
		a.SetMediaSourceKind(pluginKindID)
		d := a.MediaSourceDraft()
		d.Name = "NAS"
		d.Values[mediasource.EndpointKey], d.Values["user"], d.Values["password"] = server+"/dav", "kando", password
	})
	refresh(tt, a)
	return tt
}

// sourcesTest tests the dialog's draft and waits for its answer.
func sourcesTest(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) settings.MediaSourceStatus {
	t.Helper()
	onEngine(a, func() { a.TestMediaSource("") })
	scriptsSettle(t, a, tt, updates, func() bool { return !a.MediaSourceStatus("").Busy })
	return a.MediaSourceStatus("")
}

// TestPluginSourceIsAKindOfSource checks docs/media-sources.md 11: a plugin's source is offered in 设置 › 媒体来源 beside the built-in ones, named with its plugin; a draft of it is tested through the plugin's list, which reaches the server only through kando.source.fetch, with the password Kando put in, which the plugin never sees; a wrong password says so; and the kind is taken off once the plugin is turned off.
func TestPluginSourceIsAKindOfSource(t *testing.T) {
	a, tt, updates := sourcesApp(t)
	m := testlocale.Messages()
	var kinds []settings.MediaSourceKind
	onEngine(a, func() { kinds = a.MediaSourceKinds() })
	if !slices.Contains(kinds, settings.MediaSourceKind{ID: pluginKindID, Title: m.SettingsSourcesKindPlugin("WebDAV", "NAS 浏览器")}) || kinds[0].ID != mediasource.KindSMB {
		t.Fatalf("kinds %+v", kinds)
	}
	srv := newSourceServer(t, false)
	tt = sourcesDraft(t, a, tt, srv.URL, "secret")
	if warnings := a.MediaSourceWarnings(); !slices.Contains(warnings, m.SettingsSourcesPlainHTTP()) {
		t.Errorf("an http address: warnings %q", warnings)
	}
	if status := sourcesTest(t, a, tt, updates); !status.OK {
		t.Fatalf("the test: %+v", status)
	}
	srv.mu.Lock()
	heard := slices.Clone(srv.heard)
	srv.mu.Unlock()
	if len(heard) == 0 || heard[len(heard)-1] != "kando:secret" {
		t.Errorf("the server heard %q", heard)
	}
	onEngine(a, func() { a.MediaSourceDraft().Values["password"] = "wrong" })
	if status := sourcesTest(t, a, tt, updates); status.OK || status.Text != m.SettingsSourcesFailAuth() {
		t.Errorf("a wrong password: %+v", status)
	}
	onEngine(a, func() { a.MediaSourceDraft().Values[mediasource.EndpointKey] = "http://dav.example.com/" })
	if status := sourcesTest(t, a, tt, updates); status.Text != m.SettingsSourcesNeedsEndpoint() {
		t.Errorf("http on the internet: %+v", status)
	}
	onEngine(a, func() { a.SetPluginEnabled("acme.nas", false) })
	scriptsSettle(t, a, tt, updates, func() bool { _, ok := a.sources.kinds.Kind(pluginKindID); return !ok })
}

// TestPluginSourceReadsAFolderAndAFile checks a saved source of a plugin's kind as the 媒体来源 page and the jobs read it, through the pool: a folder listed, a file's entry, and its bytes from an offset read by Kando from where the plugin's open said, with the password Kando keeps; and what kando.source.fetch refuses a plugin, whatever it asks: a source it does not have open, a method that writes, a path out of the endpoint, a POST its manifest did not name.
func TestPluginSourceReadsAFolderAndAFile(t *testing.T) {
	a, tt, updates := sourcesApp(t)
	srv := newSourceServer(t, false)
	s := mediasource.Source{ID: "0123456789abcdef", Kind: pluginKindID, Name: "NAS", Config: map[string]string{mediasource.EndpointKey: srv.URL + "/dav", "user": "kando"}}
	onEngine(a, func() {
		a.sources.list = []mediasource.Source{s}
		if err := a.mediaSourceSecretStore().Set(mediaSourceSecretAccount(s.ID, "password"), "secret"); err != nil {
			t.Fatal(err)
		}
		a.mediaSourcePoolSync()
	})
	var (
		entries []mediasource.Entry
		stat    mediasource.Entry
		data    []byte
		err     error
		done    atomic.Bool
		d       mediasource.Driver
	)
	go func() {
		defer done.Store(true)
		ctx := context.Background()
		if d, err = a.sources.pool.Driver(ctx, s.ID); err != nil {
			return
		}
		if entries, err = d.List(ctx, "films"); err != nil {
			err = fmt.Errorf("list: %w", err)
			return
		}
		if stat, err = d.Stat(ctx, "films/talk.mp4"); err != nil {
			err = fmt.Errorf("stat: %w", err)
			return
		}
		var r io.ReadCloser
		if r, err = d.Open(ctx, "films/talk.mp4", 4, -1); err != nil {
			return
		}
		data, err = io.ReadAll(r)
		r.Close()
	}()
	scriptsSettle(t, a, tt, updates, func() bool { return done.Load() })
	if err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 || entries[0].Name != "talk.mp4" || entries[0].Size != 10 || stat.Size != 10 || string(data) != "456789" {
		t.Fatalf("entries %+v, stat %+v, data %q", entries, stat, data)
	}
	var id string
	for key := range a.plugins.sources.fetchers {
		id = strings.TrimPrefix(key, "acme.nas\x00")
	}
	if !strings.HasPrefix(id, "s_") || strings.Contains(id, s.ID) {
		t.Fatalf("the plugin knows the source as %q", id)
	}
	for _, tt2 := range []struct {
		name    string
		source  string
		request map[string]any
		want    any
	}{
		{"a source not open", "s_0000000000000000", map[string]any{"method": "GET", "path": "films/talk.mp4"}, "refused denied"},
		{"DELETE", id, map[string]any{"method": "DELETE", "path": "films/talk.mp4"}, "refused denied"},
		{"PUT", id, map[string]any{"method": "PUT", "path": "films/new.mp4", "body": "x"}, "refused denied"},
		{"a path out of the endpoint", id, map[string]any{"method": "GET", "path": "../other"}, "refused denied"},
		{"a POST not named", id, map[string]any{"method": "POST", "path": "films"}, "refused denied"},
		{"a cookie", id, map[string]any{"method": "GET", "path": "films/talk.mp4", "header": map[string]any{"Cookie": "a=1"}}, "refused denied"},
		{"a GET", id, map[string]any{"method": "GET", "path": "films/talk.mp4"}, "200"},
	} {
		got, err := scriptsCall(t, a, tt, updates, "acme.nas", "probe", tt2.source, tt2.request)
		if err != nil || fmt.Sprint(got) != fmt.Sprint(tt2.want) {
			t.Errorf("%s: %v %v, want %v", tt2.name, got, err, tt2.want)
		}
	}
	said, _ := scriptsCall(t, a, tt, updates, "acme.nas", "said")
	if text, _ := said.(string); strings.Contains(text, "secret") || !strings.Contains(text, "0123456789") {
		t.Errorf("the plugin was given %q", said)
	}
	onEngine(a, func() {
		a.sources.list = nil
		a.mediaSourcePoolSync()
	})
	scriptsSettle(t, a, tt, updates, func() bool { return len(a.plugins.sources.fetchers) == 0 })
}

// TestPluginSourceTrustsACertificate checks docs/media-sources.md 17.5 from 设置 › 媒体来源: a server whose certificate the system does not trust asks whether to trust it, by its fingerprint, in the same question an SFTP server's key is asked about; trusted, the test passes and the source keeps the fingerprint once saved; a certificate other than the one trusted asks whether to replace it.
func TestPluginSourceTrustsACertificate(t *testing.T) {
	a, tt, updates := sourcesApp(t)
	srv := newSourceServer(t, true)
	tt = sourcesDraft(t, a, tt, srv.URL, "secret")
	want := mediasource.Fingerprint(srv.Certificate().Raw)
	if status := sourcesTest(t, a, tt, updates); status.OK {
		t.Fatalf("a certificate not trusted: %+v", status)
	}
	refresh(tt, a)
	if q := a.MediaSourceAlert(); q.Kind != account.AlertTrust || !q.Certificate || q.Fingerprint != want || q.Trusted != "" || q.Host != "127.0.0.1" || !tt.Exists("id:media-source-trust") || !hasText(tt, testlocale.Messages().SettingsSourcesCertTrustMessage(want)) {
		t.Fatalf("the question %+v; texts %q", q, tt.Texts())
	}
	tt.Press("id:media-source-alert-trust")
	scriptsSettle(t, a, tt, updates, func() bool { return !a.MediaSourceStatus("").Busy })
	if status := a.MediaSourceStatus(""); !status.OK || a.MediaSourceAlert().Kind != account.AlertNone {
		t.Fatalf("once trusted: %+v", status)
	}
	onEngine(a, a.SaveMediaSource)
	scriptsSettle(t, a, tt, updates, func() bool { return !a.MediaSourceEditing() })
	if len(a.sources.list) != 1 || a.sources.list[0].Config[mediasource.PinKey] != want {
		t.Fatalf("saved %+v", a.sources.list)
	}
	id := a.sources.list[0].ID
	onEngine(a, func() {
		a.sources.list[0].Config[mediasource.PinKey] = "SHA256 00"
		a.mediaSourcePoolSync()
		a.TestMediaSource(id)
	})
	scriptsSettle(t, a, tt, updates, func() bool { return !a.MediaSourceStatus(id).Busy })
	refresh(tt, a)
	if q := a.MediaSourceAlert(); q.Kind != account.AlertKeyChanged || !q.Certificate || q.Trusted != "SHA256 00" || q.Fingerprint != want || !tt.Exists("id:media-source-key-changed") {
		t.Fatalf("another certificate: %+v", q)
	}
	onEngine(a, func() { a.AnswerMediaSourceAlert(true) })
	scriptsSettle(t, a, tt, updates, func() bool { return !a.MediaSourceStatus(id).Busy })
	if a.sources.list[0].Config[mediasource.PinKey] != want || !a.MediaSourceStatus(id).OK {
		t.Fatalf("replaced: %+v %+v", a.sources.list[0], a.MediaSourceStatus(id))
	}
}
