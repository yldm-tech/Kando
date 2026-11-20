package main

import (
	"archive/zip"
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/index"
	"kando/apps/kando/settings"

	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

// pluginsIndexServer is a community index on an https server of the test's own: the signed index at /index.json, with an ETag it answers 304 to, and the pack files under /packs/. hits counts the requests for the index and full those answered with it.
type pluginsIndexServer struct {
	*httptest.Server
	key        ed25519.PrivateKey
	served     atomic.Pointer[[]byte]
	files      sync.Map
	hits, full atomic.Int32
	made       time.Time
	// hold, while set, keeps every answer back until it is closed or the request is cancelled (holdAnswers).
	hold atomic.Pointer[chan struct{}]
}

// holdAnswers keeps the server's answers back until the returned func, which the test's cleanup calls too, lets them go.
func (s *pluginsIndexServer) holdAnswers(t *testing.T) func() {
	t.Helper()
	c := make(chan struct{})
	s.hold.Store(&c)
	var once sync.Once
	release := func() { once.Do(func() { s.hold.Store(nil); close(c) }) }
	t.Cleanup(release)
	return release
}

func newPluginsIndexServer(t *testing.T) *pluginsIndexServer {
	t.Helper()
	key, err := pkg.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	s := &pluginsIndexServer{key: key, made: time.Date(2026, 11, 19, 10, 0, 0, 0, time.UTC)}
	s.Server = httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if hold := s.hold.Load(); hold != nil {
			select {
			case <-*hold:
			case <-r.Context().Done():
				return
			}
		}
		if r.URL.Path == "/index.json" {
			s.hits.Add(1)
			data := s.served.Load()
			if data == nil {
				http.NotFound(w, r)
				return
			}
			sum := sha256.Sum256(*data)
			etag := `"` + hex.EncodeToString(sum[:8]) + `"`
			if r.Header.Get("If-None-Match") == etag {
				w.WriteHeader(http.StatusNotModified)
				return
			}
			s.full.Add(1)
			w.Header().Set("ETag", etag)
			w.Write(*data)
			return
		}
		if data, ok := s.files.Load(r.URL.Path); ok {
			w.Write(data.([]byte))
			return
		}
		http.NotFound(w, r)
	}))
	t.Cleanup(s.Close)
	return s
}

// point has the app read this index.
func (s *pluginsIndexServer) point(a *app) {
	a.plugins.index.url = s.URL + "/index.json"
	a.plugins.index.key = pkg.EncodePublicKey(s.key.Public().(ed25519.PublicKey))
	a.plugins.index.client = s.Client()
}

// entry serves the pack file at path as /packs/<name> and returns its entry, with source for a script plugin.
func (s *pluginsIndexServer) entry(t *testing.T, name, path string, source plugins.Source) index.Entry {
	t.Helper()
	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	_, p, err := plugins.Stage(t.TempDir(), path)
	if err != nil {
		t.Fatal(err)
	}
	s.files.Store("/packs/"+name, data)
	return index.FromPack(p, s.URL+"/packs/"+name, int64(len(data)), sha256.Sum256(data), source)
}

// list serves an index of entries, made a minute after the one before.
func (s *pluginsIndexServer) list(t *testing.T, entries ...index.Entry) {
	t.Helper()
	s.made = s.made.Add(time.Minute)
	data, err := index.Sign(index.Index{Format: index.Format, Version: index.Version, Generated: s.made, Entries: entries}, s.key)
	if err != nil {
		t.Fatal(err)
	}
	s.served.Store(&data)
}

// pluginsZipFolder zips the files of the pack folder dir and returns the zip's path.
func pluginsZipFolder(t *testing.T, dir string) string {
	t.Helper()
	path := filepath.Join(t.TempDir(), "pack.zip")
	f, err := os.Create(path)
	if err != nil {
		t.Fatal(err)
	}
	w := zip.NewWriter(f)
	entries, _ := os.ReadDir(dir)
	for _, e := range entries {
		data, err := os.ReadFile(filepath.Join(dir, e.Name()))
		var out io.Writer
		if err == nil {
			out, err = w.Create(e.Name())
		}
		if err == nil {
			_, err = out.Write(data)
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := w.Close(); err != nil {
		t.Fatal(err)
	}
	if err := f.Close(); err != nil {
		t.Fatal(err)
	}
	return path
}

// pluginsIndexReady waits until the index has been read and no ask for it runs.
func pluginsIndexReady(a *app) func() bool {
	return func() bool { return a.plugins.index.read && !a.plugins.index.reading && !a.plugins.index.busy }
}

// TestPluginIndexBrowseAndInstall checks 浏览插件 from the 插件 page: nothing is asked for before it opens; opened, it lists the packs of the signed index, which the search and the kind filter narrow; 安装 downloads a pack and opens the same install confirmation as a file, which says where it comes from and installs nothing until it is confirmed; a task pack is installed off, and an installed pack reads as installed.
func TestPluginIndexBrowseAndInstall(t *testing.T) {
	a, tt, updates, root := pluginsApp(t)
	srv := newPluginsIndexServer(t)
	srv.point(a)
	m := testlocale.Messages()
	srv.list(t, srv.entry(t, "anime.zip", pluginZip(t, pluginGlossary), ""), srv.entry(t, "quiz.zip", pluginZip(t, pluginTaskLine), ""))
	tt = settingsOpenAt(a, tt, settings.SectionPlugins)
	if tt.Exists("id:settings-plugins-index") || srv.hits.Load() != 0 {
		t.Fatal("the index was asked for before 浏览插件 opened")
	}
	tt.Press("id:settings-plugins-browse")
	waitUntil(t, tt, a, updates, pluginsIndexReady(a))
	if srv.full.Load() != 1 || a.plugins.index.err != nil {
		t.Fatalf("the index read: %d, %v", srv.full.Load(), a.plugins.index.err)
	}
	if !tt.Exists("id:settings-plugins-index-row-example.anime-ja-zh") || !tt.Exists("id:settings-plugins-index-row-example.quiz") || !hasText(tt, m.SettingsPluginsIndexUpdated(pluginsIndexDate(a.plugins.index.meta))) {
		t.Fatalf("the list; texts %q", tt.Texts())
	}
	tt.Type("id:settings-plugins-index-search", "quiz")
	refresh(tt, a)
	if tt.Exists("id:settings-plugins-index-row-example.anime-ja-zh") || !tt.Exists("id:settings-plugins-index-row-example.quiz") {
		t.Errorf("the search for quiz; texts %q", tt.Texts())
	}
	apply(tt, a, func() { a.SetPluginIndexQuery(""); a.SetPluginIndexKind(plugins.KindGlossary) })
	if !tt.Exists("id:settings-plugins-index-row-example.anime-ja-zh") || tt.Exists("id:settings-plugins-index-row-example.quiz") {
		t.Errorf("the glossaries; texts %q", tt.Texts())
	}
	apply(tt, a, func() { a.SetPluginIndexKind("") })

	tt.Press("id:settings-plugins-index-install-example.anime-ja-zh")
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.pending != nil && !a.plugins.busy })
	if a.plugins.pending.err != nil || !tt.Exists("id:plugins-install-confirm") || !hasText(tt, m.SettingsPluginsInstallSourceIndex()) || hasText(tt, m.SettingsPluginsInstallSource()) {
		t.Fatalf("the confirmation: %v; texts %q", a.plugins.pending.err, tt.Texts())
	}
	if packs, _ := plugins.Scan(root); len(packs) != 0 {
		t.Fatal("the pack was installed before it was confirmed")
	}
	tt.Press("id:plugins-install-ok")
	waitUntil(t, tt, a, updates, func() bool {
		return a.plugins.pending == nil && !a.plugins.busy && !a.plugins.saving && len(a.plugins.packs) == 1
	})
	if !a.plugins.state.Enabled("example.anime-ja-zh") || tt.Exists("id:settings-plugins-index-install-example.anime-ja-zh") {
		t.Errorf("the glossary after its install; texts %q", tt.Texts())
	}
	if e, _ := a.plugins.index.idx.Lookup("example.anime-ja-zh"); func() bool { s, _ := a.pluginsIndexState(e); return s != settings.PluginIndexInstalled }() {
		t.Error("the glossary does not read as installed")
	}

	// A task pack spends tokens, so it is installed off, from the index as from a file.
	tt.Press("id:settings-plugins-index-install-example.quiz")
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.pending != nil && !a.plugins.busy })
	tt.Press("id:plugins-install-ok")
	waitUntil(t, tt, a, updates, func() bool {
		return a.plugins.pending == nil && !a.plugins.busy && !a.plugins.saving && len(a.plugins.packs) == 2
	})
	for _, p := range a.plugins.packs {
		if p.Kind == plugins.KindTask && a.plugins.state.EnabledPack(p) {
			t.Error("the task pack was installed on")
		}
	}
}

// TestPluginIndexUpdate checks that an installed pack whose version the index lists newer offers 更新 with the version installed, which goes through the install confirmation naming the version it replaces.
func TestPluginIndexUpdate(t *testing.T) {
	a, tt, updates, _ := pluginsApp(t, pluginGlossary)
	srv := newPluginsIndexServer(t)
	srv.point(a)
	m := testlocale.Messages()
	srv.list(t, srv.entry(t, "anime.zip", pluginZip(t, strings.Replace(pluginGlossary, `"version": "1.2.0"`, `"version": "1.3.0"`, 1)), ""))
	tt = settingsOpenAt(a, tt, settings.SectionPlugins)
	tt.Press("id:settings-plugins-browse")
	waitUntil(t, tt, a, updates, pluginsIndexReady(a))
	if !tt.Exists("id:settings-plugins-index-update-example.anime-ja-zh") || !hasText(tt, m.SettingsPluginsIndexInstalledVersion("1.2.0")) {
		t.Fatalf("no update; texts %q", tt.Texts())
	}
	tt.Press("id:settings-plugins-index-update-example.anime-ja-zh")
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.pending != nil && !a.plugins.busy })
	if !hasText(tt, m.SettingsPluginsInstallReplaces("1.2.0")) {
		t.Fatalf("the confirmation does not name the version replaced; texts %q", tt.Texts())
	}
	tt.Press("id:plugins-install-ok")
	waitUntil(t, tt, a, updates, func() bool {
		return a.plugins.pending == nil && !a.plugins.busy && !a.plugins.saving && len(a.plugins.packs) == 1 && a.plugins.packs[0].Version == "1.3.0"
	})
	if tt.Exists("id:settings-plugins-index-update-example.anime-ja-zh") {
		t.Error("更新 still shows after the update")
	}
}

// TestPluginIndexRefusesDownloads checks that a downloaded file whose SHA-256 is not the index's, or whose pack is not the one the entry names, is refused in the confirmation's failure view, and leaves nothing installed or staged.
func TestPluginIndexRefusesDownloads(t *testing.T) {
	a, tt, updates, root := pluginsApp(t)
	srv := newPluginsIndexServer(t)
	srv.point(a)
	m := testlocale.Messages()
	tampered := srv.entry(t, "anime.zip", pluginZip(t, pluginGlossary), "")
	other := sha256.Sum256([]byte("another file"))
	tampered.SHA256 = hex.EncodeToString(other[:])
	renamed := srv.entry(t, "gateway.zip", pluginZip(t, pluginService), "")
	renamed.ID, renamed.Publisher = "example.gateway", "example"
	srv.list(t, tampered, renamed)
	tt = settingsOpenAt(a, tt, settings.SectionPlugins)
	tt.Press("id:settings-plugins-browse")
	waitUntil(t, tt, a, updates, pluginsIndexReady(a))
	for id, want := range map[string]struct {
		err  error
		text string
	}{"example.anime-ja-zh": {index.ErrChecksum, m.PluginsErrIndexChecksum()}, "example.gateway": {index.ErrNotListed, m.PluginsErrIndexNotListed()}} {
		tt.Press("id:settings-plugins-index-install-" + id)
		waitUntil(t, tt, a, updates, func() bool { return a.plugins.pending != nil && !a.plugins.busy })
		if !errors.Is(a.plugins.pending.err, want.err) || !tt.Exists("id:plugins-install-failed") || !hasText(tt, want.text) {
			t.Errorf("%s: %v; texts %q", id, a.plugins.pending.err, tt.Texts())
		}
		tt.Press("id:plugins-install-done")
		refresh(tt, a)
	}
	if packs, _ := plugins.Scan(root); len(packs) != 0 {
		t.Errorf("installed %d packs", len(packs))
	}
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".staging-") {
			t.Errorf("left %s", e.Name())
		}
	}
}

// TestPluginIndexCache checks the copy of the index kept: opening 浏览插件 with a copy asked for less than a day ago asks for nothing; offline it shows the copy, says so, and neither refreshes nor downloads; online, 刷新 asks with the copy's ETag, and the server's 304 keeps it. A copy a day old is asked for again as 浏览插件 opens.
func TestPluginIndexCache(t *testing.T) {
	a, tt, updates, _ := pluginsApp(t)
	srv := newPluginsIndexServer(t)
	srv.point(a)
	m := testlocale.Messages()
	srv.list(t, srv.entry(t, "anime.zip", pluginZip(t, pluginGlossary), ""))
	src, err := a.pluginsIndexSource()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := src.Refresh(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	a.privacy.offline = true
	tt = settingsOpenAt(a, tt, settings.SectionPlugins)
	tt.Press("id:settings-plugins-browse")
	waitUntil(t, tt, a, updates, pluginsIndexReady(a))
	if srv.hits.Load() != 1 || !tt.Exists("id:settings-plugins-index-row-example.anime-ja-zh") || !hasText(tt, m.SettingsPluginsIndexOffline(pluginsIndexDate(a.plugins.index.meta))) {
		t.Fatalf("offline: %d asks; texts %q", srv.hits.Load(), tt.Texts())
	}
	if a.PluginIndexCanRefresh() || a.PluginIndexInstallable() {
		t.Error("offline, 刷新 or 安装 is on")
	}
	onEngine(a, func() {
		a.RefreshPluginIndex()
		a.InstallFromPluginIndex("example.anime-ja-zh")
	})
	if a.plugins.index.busy || a.plugins.index.downloading != "" || srv.hits.Load() != 1 {
		t.Error("offline, Kando asked the server")
	}

	apply(tt, a, func() { a.privacy.offline = false })
	tt.Press("id:settings-plugins-index-refresh")
	waitUntil(t, tt, a, updates, pluginsIndexReady(a))
	if srv.hits.Load() != 2 || srv.full.Load() != 1 || a.plugins.index.err != nil {
		t.Errorf("刷新: %d asks, %d answered in full, %v", srv.hits.Load(), srv.full.Load(), a.plugins.index.err)
	}

	// A copy a day old, as the next time Kando starts, is asked for again as 浏览插件 opens.
	if _, _, err := src.Refresh(context.Background(), time.Now().Add(-index.RefreshEvery-time.Minute)); err != nil {
		t.Fatal(err)
	}
	apply(tt, a, func() { a.SetPluginIndexOpen(false); a.plugins.index.read = false })
	hits := srv.hits.Load()
	tt.Press("id:settings-plugins-browse")
	waitUntil(t, tt, a, updates, pluginsIndexReady(a))
	if srv.hits.Load() != hits+1 {
		t.Errorf("a copy a day old was not asked for again: %d asks", srv.hits.Load()-hits)
	}
}

// TestPluginIndexRefusesAnIndex checks that an index not signed by the index key is not shown, the copy kept still is, and the page says why; and that a Kando without an index key asks for nothing and says it has no directory.
func TestPluginIndexRefusesAnIndex(t *testing.T) {
	a, tt, updates, _ := pluginsApp(t)
	srv := newPluginsIndexServer(t)
	srv.point(a)
	m := testlocale.Messages()
	srv.list(t, srv.entry(t, "anime.zip", pluginZip(t, pluginGlossary), ""))
	tt = settingsOpenAt(a, tt, settings.SectionPlugins)
	tt.Press("id:settings-plugins-browse")
	waitUntil(t, tt, a, updates, pluginsIndexReady(a))
	forged, err := pkg.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	srv.key = forged
	srv.list(t, srv.entry(t, "gateway.zip", pluginZip(t, pluginService), ""))
	tt.Press("id:settings-plugins-index-refresh")
	waitUntil(t, tt, a, updates, pluginsIndexReady(a))
	if !errors.Is(a.plugins.index.err, index.ErrSignature) || !hasText(tt, m.SettingsPluginsIndexFailed(m.PluginsErrIndexSignature())) || tt.Exists("id:settings-plugins-index-row-acme.corp-gateway") || !tt.Exists("id:settings-plugins-index-row-example.anime-ja-zh") {
		t.Errorf("a forged index: %v; texts %q", a.plugins.index.err, tt.Texts())
	}

	// A Kando without the index key, as the next time it starts.
	apply(tt, a, func() {
		a.SetPluginIndexOpen(false)
		a.plugins.index = newPluginsIndex()
		a.plugins.index.key = ""
	})
	hits := srv.hits.Load()
	tt.Press("id:settings-plugins-browse")
	waitUntil(t, tt, a, updates, pluginsIndexReady(a))
	if srv.hits.Load() != hits || !hasText(tt, m.SettingsPluginsIndexNotConfigured()) || a.PluginIndexCanRefresh() {
		t.Errorf("no key: %d asks; texts %q", srv.hits.Load()-hits, tt.Texts())
	}
}

// TestPluginIndexScript checks a script plugin from the index: its signature is verified as a file's is, the confirmation says it was downloaded from the directory, and it is installed off with nothing granted; one whose signature does not come from where the index says is refused.
func TestPluginIndexScript(t *testing.T) {
	key := newPluginsKey(t)
	a, tt, updates, _ := scriptsApp(t, key)
	srv := newPluginsIndexServer(t)
	srv.point(a)
	m := testlocale.Messages()
	anki := srv.entry(t, "anki.zip", pluginsZipFolder(t, pluginScriptFolder(t, key, "acme.anki", "1.0.0", pluginAnkiFields, pluginAnkiCode)), plugins.SourceVerified)
	claims := srv.entry(t, "other.zip", pluginsZipFolder(t, pluginScriptFolder(t, key, "acme.other", "1.0.0", pluginAnkiFields, pluginAnkiCode)), plugins.SourceOfficial)
	srv.list(t, anki, claims)
	tt = settingsOpenAt(a, tt, settings.SectionPlugins)
	tt.Press("id:settings-plugins-browse")
	waitUntil(t, tt, a, updates, pluginsIndexReady(a))

	tt.Press("id:settings-plugins-index-install-acme.other")
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.pending != nil && !a.plugins.busy })
	if !errors.Is(a.plugins.pending.err, index.ErrNotListed) {
		t.Errorf("a plugin listed as Kando's own: %v", a.plugins.pending.err)
	}
	tt.Press("id:plugins-install-done")
	refresh(tt, a)

	tt.Press("id:settings-plugins-index-install-acme.anki")
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.pending != nil && !a.plugins.busy })
	if p := a.plugins.pending; p.err != nil || !tt.Exists("text:"+m.SettingsPluginsInstallSourceVerified("acme")) || !hasText(tt, m.SettingsPluginsInstallFromIndex()) {
		t.Fatalf("the confirmation: %v; texts %q", p.err, tt.Texts())
	}
	tt.Press("id:plugins-install-ok")
	waitUntil(t, tt, a, updates, func() bool {
		return a.plugins.pending == nil && !a.plugins.busy && !a.plugins.saving && len(a.pluginsScriptPacks()) == 1 && a.pluginsScriptPacks()[0].Signer != ""
	})
	p := a.pluginsScriptPacks()[0]
	if a.plugins.state.EnabledPack(p) || len(a.plugins.state.Effective(p, false, false).Perms) > 1 {
		t.Errorf("installed on or granted: %+v", a.plugins.state)
	}
}

// TestPluginIndexUnreadableCache checks that a copy kept that cannot be read, its file gone or signed by another key, is asked for again at once, however recently it was asked for, and that why it could not be read shows when the ask fails.
func TestPluginIndexUnreadableCache(t *testing.T) {
	a, tt, updates, root := pluginsApp(t)
	srv := newPluginsIndexServer(t)
	srv.point(a)
	m := testlocale.Messages()
	srv.list(t, srv.entry(t, "anime.zip", pluginZip(t, pluginGlossary), ""))
	src, err := a.pluginsIndexSource()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := src.Refresh(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	// The file is gone, what is known of it says it was asked for a moment ago.
	if err := os.Remove(filepath.Join(index.CacheDir(root), "index.json")); err != nil {
		t.Fatal(err)
	}
	hits := srv.hits.Load()
	tt = settingsOpenAt(a, tt, settings.SectionPlugins)
	tt.Press("id:settings-plugins-browse")
	waitUntil(t, tt, a, updates, pluginsIndexReady(a))
	if srv.hits.Load() != hits+1 || !tt.Exists("id:settings-plugins-index-row-example.anime-ja-zh") {
		t.Fatalf("a copy gone: %d asks; texts %q", srv.hits.Load()-hits, tt.Texts())
	}

	// Kando's key changed: the copy kept is another key's, and the server does not answer.
	apply(tt, a, func() {
		a.SetPluginIndexOpen(false)
		a.plugins.index = newPluginsIndex()
		srv.point(a)
	})
	other, err := pkg.GenerateKey()
	if err != nil {
		t.Fatal(err)
	}
	srv.key = other
	srv.point(a)
	srv.served.Store(nil)
	hits = srv.hits.Load()
	tt.Press("id:settings-plugins-browse")
	waitUntil(t, tt, a, updates, pluginsIndexReady(a))
	if srv.hits.Load() != hits+1 || !errors.Is(a.plugins.index.err, index.ErrHTTP) || !hasText(tt, m.SettingsPluginsIndexFailed(m.PluginsErrIndexHTTP())) {
		t.Errorf("another key's copy: %d asks, %v; texts %q", srv.hits.Load()-hits, a.plugins.index.err, tt.Texts())
	}
	// Offline, the line says so rather than the error of an ask made before.
	apply(tt, a, func() { a.privacy.offline = true })
	if !hasText(tt, m.SettingsPluginsIndexOfflineNone()) || a.PluginIndexFailed() {
		t.Errorf("offline after a failed ask; texts %q", tt.Texts())
	}
}

// TestPluginIndexOfflineStops checks that going offline stops an ask for the list and a download under way, and that what they bring back afterwards is dropped: no list replaced, no install confirmation.
func TestPluginIndexOfflineStops(t *testing.T) {
	a, tt, updates, root := pluginsApp(t)
	srv := newPluginsIndexServer(t)
	srv.point(a)
	srv.list(t, srv.entry(t, "anime.zip", pluginZip(t, pluginGlossary), ""))
	release := srv.holdAnswers(t)
	tt = settingsOpenAt(a, tt, settings.SectionPlugins)
	tt.Press("id:settings-plugins-browse")
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.index.busy })
	apply(tt, a, func() { a.privacy.offline = true })
	if a.plugins.index.busy || a.plugins.index.fetchCancel != nil {
		t.Fatal("going offline did not stop the ask")
	}
	release()
	waitUpdate(t, tt, a, updates)
	if len(a.plugins.index.idx.Entries) != 0 || a.plugins.index.err != nil || a.plugins.index.busy {
		t.Fatalf("the cancelled ask's answer was taken: %+v, %v", a.plugins.index.idx, a.plugins.index.err)
	}

	// Online again, the list comes; a download then stops as Kando goes offline.
	apply(tt, a, func() { a.privacy.offline = false })
	tt.Press("id:settings-plugins-index-refresh")
	waitUntil(t, tt, a, updates, pluginsIndexReady(a))
	release = srv.holdAnswers(t)
	tt.Press("id:settings-plugins-index-install-example.anime-ja-zh")
	if a.plugins.index.downloading != "example.anime-ja-zh" || a.plugins.busy {
		t.Fatalf("the download: %q, busy %v", a.plugins.index.downloading, a.plugins.busy)
	}
	apply(tt, a, func() { a.privacy.offline = true })
	if a.plugins.index.downloading != "" {
		t.Fatal("going offline did not stop the download")
	}
	release()
	waitUpdate(t, tt, a, updates)
	if a.plugins.pending != nil || a.plugins.busy {
		t.Errorf("the cancelled download opened a confirmation: %+v", a.plugins.pending)
	}
	entries, _ := os.ReadDir(root)
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), ".staging-") {
			t.Errorf("left %s", e.Name())
		}
	}
}

// TestPluginIndexDownloadLeavesThePageFree checks that a download from the index holds nothing else up: another pack is turned off meanwhile, and its row's 取消 stops it, after which nothing opens. Closing 浏览插件 stops one too.
func TestPluginIndexDownloadLeavesThePageFree(t *testing.T) {
	a, tt, updates, root := pluginsApp(t, pluginService)
	srv := newPluginsIndexServer(t)
	srv.point(a)
	srv.list(t, srv.entry(t, "anime.zip", pluginZip(t, pluginGlossary), ""))
	tt = settingsOpenAt(a, tt, settings.SectionPlugins)
	tt.Press("id:settings-plugins-browse")
	waitUntil(t, tt, a, updates, pluginsIndexReady(a))
	release := srv.holdAnswers(t)
	tt.Press("id:settings-plugins-index-install-example.anime-ja-zh")
	if !tt.Exists("id:settings-plugins-index-cancel-example.anime-ja-zh") || a.plugins.busy {
		t.Fatalf("downloading: busy %v; texts %q", a.plugins.busy, tt.Texts())
	}
	tt.Press("id:settings-plugins-switch-acme.corp-gateway")
	waitUntil(t, tt, a, updates, func() bool { return !a.plugins.busy && !a.plugins.saving })
	if state, err := plugins.LoadState(root); err != nil || state.Enabled("acme.corp-gateway") {
		t.Fatalf("turning a pack off during a download: %+v, %v", state, err)
	}
	tt.Press("id:settings-plugins-index-cancel-example.anime-ja-zh")
	if a.plugins.index.downloading != "" || tt.Exists("id:settings-plugins-index-cancel-example.anime-ja-zh") {
		t.Fatalf("取消 did not stop the download; texts %q", tt.Texts())
	}
	release()
	waitUpdate(t, tt, a, updates)
	if a.plugins.pending != nil {
		t.Error("a cancelled download opened a confirmation")
	}

	release = srv.holdAnswers(t)
	tt.Press("id:settings-plugins-index-install-example.anime-ja-zh")
	tt.Press("id:settings-plugins-index-hide")
	if a.plugins.index.downloading != "" || tt.Exists("id:settings-plugins-index") {
		t.Fatal("closing 浏览插件 did not stop the download")
	}
	release()
	waitUpdate(t, tt, a, updates)
	if a.plugins.pending != nil {
		t.Error("a download stopped by closing opened a confirmation")
	}
}

// pluginsIndexAge makes the index read an hour old, as one a link does not take without asking for a newer one (pluginsIndexLinkFresh).
func pluginsIndexAge(a *app) { a.plugins.index.meta.Fetched = time.Now().Add(-time.Hour) }

// TestPluginIndexLink checks a link of Kando's site that asks to install a pack of the index (kando://plugin/install?id=…): Settings › 插件 opens with 浏览插件 searching for it, a newer index is asked for unless one was a moment ago, and with it the pack downloads and its install confirmation opens, installing nothing before it is confirmed; a pack that index does not list, and a link naming no pack, say so; any other link is left to the areas that know it.
func TestPluginIndexLink(t *testing.T) {
	a, tt, updates, root := pluginsApp(t)
	srv := newPluginsIndexServer(t)
	srv.point(a)
	m := testlocale.Messages()
	srv.list(t, srv.entry(t, "anime.zip", pluginZip(t, pluginGlossary), ""), srv.entry(t, "quiz.zip", pluginZip(t, pluginTaskLine), ""))

	apply(tt, a, func() { a.areasOpenLinks([]string{"kando://plugin/install?id=example.anime-ja-zh"}) })
	wantSettings(t, a, settings.SectionPlugins)
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.pending != nil && !a.plugins.busy })
	if a.plugins.pending.err != nil || !a.plugins.pending.listed || !tt.Exists("id:plugins-install-confirm") || !hasText(tt, m.SettingsPluginsInstallSourceIndex()) {
		t.Fatalf("the confirmation: %v; texts %q", a.plugins.pending.err, tt.Texts())
	}
	if a.PluginIndexQuery() != "example.anime-ja-zh" || !a.PluginIndexOpen() {
		t.Errorf("浏览插件 is open %v searching for %q", a.PluginIndexOpen(), a.PluginIndexQuery())
	}
	if packs, _ := plugins.Scan(root); len(packs) != 0 {
		t.Fatal("the pack was installed before it was confirmed")
	}
	tt.Press("id:plugins-install-cancel")
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.pending == nil && !a.plugins.busy })

	// A pack the index does not list: a newer index is asked for, and then the page says it is not listed.
	apply(tt, a, func() { pluginsIndexAge(a) })
	hits := srv.hits.Load()
	apply(tt, a, func() { a.areasOpenLinks([]string{index.InstallLink("example.unknown")}) })
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.index.wanted == "" && pluginsIndexReady(a)() })
	if srv.hits.Load() != hits+1 || !hasText(tt, m.SettingsPluginsIndexLinkMissing("example.unknown")) || a.plugins.pending != nil {
		t.Errorf("an unlisted pack: %d asks; texts %q", srv.hits.Load()-hits, tt.Texts())
	}
	// An index asked for a moment ago is taken as it is.
	hits = srv.hits.Load()
	apply(tt, a, func() { a.areasOpenLinks([]string{index.InstallLink("example.other")}) })
	if srv.hits.Load() != hits || a.plugins.index.wanted != "" || !hasText(tt, m.SettingsPluginsIndexLinkMissing("example.other")) {
		t.Errorf("a link after a fresh index: %d asks, waiting for %q", srv.hits.Load()-hits, a.plugins.index.wanted)
	}

	// A pack the index lists once it is asked for again, as one added to the directory since the copy kept.
	srv.list(t, srv.entry(t, "anime.zip", pluginZip(t, pluginGlossary), ""), srv.entry(t, "quiz.zip", pluginZip(t, pluginTaskLine), ""), srv.entry(t, "roots.zip", pluginZip(t, pluginTaskWord), ""))
	apply(tt, a, func() { pluginsIndexAge(a); a.areasOpenLinks([]string{index.InstallLink("example.roots")}) })
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.pending != nil && !a.plugins.busy })
	if a.plugins.pending.err != nil || a.plugins.pending.pack.ID != "example.roots" || hasText(tt, m.SettingsPluginsIndexLinkMissing("example.roots")) {
		t.Fatalf("a pack added since: %v; texts %q", a.plugins.pending.err, tt.Texts())
	}
	tt.Press("id:plugins-install-cancel")
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.pending == nil && !a.plugins.busy })

	apply(tt, a, func() {
		a.areasOpenLinks([]string{"kando://plugin/install?id=../../etc", "kando://plugin/other?id=example.quiz"})
	})
	if !hasText(tt, m.SettingsPluginsIndexLinkBad()) || a.plugins.index.downloading != "" || a.plugins.pending != nil {
		t.Errorf("a link naming no pack; texts %q", tt.Texts())
	}
	apply(tt, a, func() { a.SetPluginIndexOpen(false) })
	if a.PluginIndexLinkNote() != "" {
		t.Error("what was said of the link stays once 浏览插件 closes")
	}
}

// TestPluginIndexLinkFromTheSystem checks that a link the system opens Kando with reaches the plugins area through areasListenLinks, the handler areasReady registers (registered here directly, as testView runs no area's ready), the one Kando was launched with once the handler is registered and later ones as they come, as the site's 在看懂中打开 opens it on macOS and, through the scheme NSGo's installers register, on Windows and Linux, where a second launch hands it to the running copy: Settings › 插件 opens with 浏览插件 searching for the pack and its install confirmation follows; a link of another scheme in the same launch is left alone.
func TestPluginIndexLinkFromTheSystem(t *testing.T) {
	a, tt, updates, _ := pluginsApp(t)
	srv := newPluginsIndexServer(t)
	srv.point(a)
	srv.list(t, srv.entry(t, "anime.zip", pluginZip(t, pluginGlossary), ""))

	// The link Kando is launched with comes before the handler, which areasReady registers once the window is ready.
	tt.OpenLinks("https://kando.yldm.ai/plugin/", index.InstallLink("example.anime-ja-zh"))
	refresh(tt, a)
	if a.prefs.open {
		t.Fatal("the link was taken before any handler")
	}
	onEngine(a, a.areasListenLinks)
	refresh(tt, a)
	wantSettings(t, a, settings.SectionPlugins)
	if a.PluginIndexQuery() != "example.anime-ja-zh" || !a.PluginIndexOpen() {
		t.Fatalf("浏览插件 is open %v searching for %q", a.PluginIndexOpen(), a.PluginIndexQuery())
	}
	if a.addsheet.open || a.queue.source != "" || len(a.library.Jobs) != 0 {
		t.Fatalf("the https link was taken: sheet open %v, field %q, jobs %d", a.addsheet.open, a.queue.source, len(a.library.Jobs))
	}
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.pending != nil && !a.plugins.busy })
	if a.plugins.pending.err != nil || a.plugins.pending.pack.ID != "example.anime-ja-zh" || !tt.Exists("id:plugins-install-confirm") {
		t.Fatalf("the confirmation: %v; texts %q", a.plugins.pending.err, tt.Texts())
	}
	tt.Press("id:plugins-install-cancel")
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.pending == nil && !a.plugins.busy })

	// Once Kando runs, a link reaches it as it comes, as from a second launch.
	apply(tt, a, func() { a.SetPluginIndexOpen(false) })
	tt.OpenLinks(index.InstallLink("example.unknown"))
	refresh(tt, a)
	if a.PluginIndexQuery() != "example.unknown" || !a.PluginIndexOpen() {
		t.Fatalf("a link while running: 浏览插件 is open %v searching for %q", a.PluginIndexOpen(), a.PluginIndexQuery())
	}
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.index.wanted == "" && pluginsIndexReady(a)() && !a.plugins.busy })
}

// TestPluginIndexLinkWaits checks that a link waits for a newer index rather than taking the copy kept: offline it shows the pack and downloads nothing, nor says it is missing, until Kando is back online; when the ask fails the status says why, until 刷新 brings the index; and the version installed is the one the newer index lists, whose file is the one served, not the copy kept's.
func TestPluginIndexLinkWaits(t *testing.T) {
	a, tt, updates, _ := pluginsApp(t)
	srv := newPluginsIndexServer(t)
	srv.point(a)
	m := testlocale.Messages()
	srv.list(t, srv.entry(t, "anime-1.2.0.zip", pluginZip(t, pluginGlossary), ""), srv.entry(t, "quiz.zip", pluginZip(t, pluginTaskLine), ""))
	tt = settingsOpenAt(a, tt, settings.SectionPlugins)
	tt.Press("id:settings-plugins-browse")
	waitUntil(t, tt, a, updates, pluginsIndexReady(a))
	apply(tt, a, func() { a.SetPluginIndexOpen(false); pluginsIndexAge(a) })

	// The directory moved on to 1.3.0 and serves 1.2.0's file no longer.
	srv.files.Delete("/packs/anime-1.2.0.zip")
	srv.list(t, srv.entry(t, "anime-1.3.0.zip", pluginZip(t, strings.Replace(pluginGlossary, `"version": "1.2.0"`, `"version": "1.3.0"`, 1)), ""), srv.entry(t, "quiz.zip", pluginZip(t, pluginTaskLine), ""))

	hits := srv.hits.Load()
	apply(tt, a, func() {
		a.privacy.offline = true
		a.areasOpenLinks([]string{index.InstallLink("example.anime-ja-zh"), index.InstallLink("example.unknown")})
	})
	if srv.hits.Load() != hits || a.plugins.index.wanted != "example.unknown" || a.plugins.index.downloading != "" || a.PluginIndexLinkNote() != "" {
		t.Fatalf("offline: %d asks, waiting for %q, downloading %q, note %q", srv.hits.Load()-hits, a.plugins.index.wanted, a.plugins.index.downloading, a.PluginIndexLinkNote())
	}

	// Back online, the ask fails: the link still waits, and the status says why.
	served := srv.served.Load()
	srv.served.Store(nil)
	apply(tt, a, func() {
		a.privacy.offline = false
		a.areasOpenLinks([]string{index.InstallLink("example.anime-ja-zh")})
	})
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.index.asked && pluginsIndexReady(a)() })
	if a.plugins.index.wanted != "example.anime-ja-zh" || a.PluginIndexLinkNote() != "" || !a.PluginIndexFailed() || a.plugins.pending != nil {
		t.Fatalf("a failed ask: waiting for %q, note %q, failed %v", a.plugins.index.wanted, a.PluginIndexLinkNote(), a.PluginIndexFailed())
	}

	// 刷新 brings the newer index, and the version it lists is the one offered.
	srv.served.Store(served)
	tt.Press("id:settings-plugins-index-refresh")
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.pending != nil && !a.plugins.busy })
	if a.plugins.pending.err != nil || a.plugins.pending.pack.Version != "1.3.0" || !hasText(tt, m.SettingsPluginsInstallSourceIndex()) {
		t.Fatalf("the confirmation: %v %q; texts %q", a.plugins.pending.err, a.plugins.pending.pack.Version, tt.Texts())
	}
}

// TestPluginIndexUpdates checks the notice of updates: with the index read once, an installed pack it lists newer is counted beside 插件 in the settings' sidebar and in 有可用更新 at the top of the page, whose button shows 浏览插件 listing only it until 显示全部; and as Kando starts it reads the copy kept for the count without asking the network.
func TestPluginIndexUpdates(t *testing.T) {
	a, tt, updates, _ := pluginsApp(t, pluginGlossary)
	srv := newPluginsIndexServer(t)
	srv.point(a)
	m := testlocale.Messages()
	newer := strings.Replace(pluginGlossary, `"version": "1.2.0"`, `"version": "1.3.0"`, 1)
	srv.list(t, srv.entry(t, "anime.zip", pluginZip(t, newer), ""), srv.entry(t, "quiz.zip", pluginZip(t, pluginTaskLine), ""))
	tt = settingsOpenAt(a, tt, settings.SectionPlugins)
	if tt.Exists("id:settings-plugins-updates") || tt.Exists("id:settings-section-plugins-updates") {
		t.Fatal("updates are shown before the index was read")
	}

	// Kando starting with the copy kept counts the update from it, asking the network nothing.
	src, err := a.pluginsIndexSource()
	if err != nil {
		t.Fatal(err)
	}
	if _, _, err := src.Refresh(context.Background(), time.Now()); err != nil {
		t.Fatal(err)
	}
	hits := srv.hits.Load()
	onEngine(a, a.pluginsIndexPeek)
	waitUntil(t, tt, a, updates, func() bool { return len(a.plugins.index.idx.Entries) == 2 })
	if srv.hits.Load() != hits || a.plugins.index.read {
		t.Errorf("reading the copy kept asked the network %d times, or took it as 浏览插件's read", srv.hits.Load()-hits)
	}
	if a.PluginIndexUpdates() != 1 || !hasText(tt, m.SettingsPluginsIndexUpdatesAvailable(1)) || !tt.Exists("id:settings-section-plugins-updates") {
		t.Fatalf("%d updates; texts %q", a.PluginIndexUpdates(), tt.Texts())
	}

	tt.Press("id:settings-plugins-updates-show")
	waitUntil(t, tt, a, updates, pluginsIndexReady(a))
	if !a.PluginIndexOpen() || !a.PluginIndexUpdatesOnly() || !tt.Exists("id:settings-plugins-index-update-example.anime-ja-zh") || tt.Exists("id:settings-plugins-index-row-example.quiz") || tt.Exists("id:settings-plugins-updates") {
		t.Fatalf("the directory filtered to updates; texts %q", tt.Texts())
	}
	tt.Press("id:settings-plugins-index-show-all")
	refresh(tt, a)
	if !tt.Exists("id:settings-plugins-index-row-example.quiz") || tt.Exists("id:settings-plugins-index-updates-only") || !tt.Exists("id:settings-plugins-updates") {
		t.Errorf("every pack again; texts %q", tt.Texts())
	}

	// Updated, the pack is no longer counted.
	tt.Press("id:settings-plugins-index-update-example.anime-ja-zh")
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.pending != nil && !a.plugins.busy })
	tt.Press("id:plugins-install-ok")
	waitUntil(t, tt, a, updates, func() bool {
		return a.plugins.pending == nil && !a.plugins.busy && !a.plugins.saving && len(a.plugins.packs) == 1 && a.plugins.packs[0].Version == "1.3.0"
	})
	if a.PluginIndexUpdates() != 0 || tt.Exists("id:settings-plugins-updates") || tt.Exists("id:settings-section-plugins-updates") {
		t.Errorf("updates after the update; texts %q", tt.Texts())
	}
}
