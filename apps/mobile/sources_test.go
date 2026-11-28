package main

import (
	"encoding/json"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"kando/apps/mobile/nas"
	"kando/apps/mobile/store"
	"kando/packages/remotefs"
	"kando/packages/remotefs/davtest"
	"kando/packages/remotefs/sftptest"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// onboarded is a phone's data folder in dir that has been through the introduction, paired with no computer.
func onboarded(t *testing.T, dir string) {
	t.Helper()
	if err := store.Open(dir).SavePrefs(store.Prefs{Onboarded: true}); err != nil {
		t.Fatal(err)
	}
}

// davFilms is a davtest server whose folder holds Films, with the talk's video as Holiday.mp4, a folder Trip, a text file and a hidden folder, which the phone does not list.
func davFilms(t *testing.T, tls bool) *davtest.Server {
	t.Helper()
	f := davtest.New(t)
	if tls {
		f = davtest.NewTLS(t)
	}
	for _, d := range []string{"Films/Trip", "Films/.hidden"} {
		if err := os.MkdirAll(filepath.Join(f.Dir, d), 0o755); err != nil {
			t.Fatal(err)
		}
	}
	for name, body := range map[string][]byte{"Films/Holiday.mp4": talkVideo, "Films/notes.txt": []byte("notes"), "Films/Trip/Beach.mov": []byte("beach")} {
		if err := os.WriteFile(filepath.Join(f.Dir, name), body, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return f
}

// keepSource puts src in dir's sources.json, with its password, as added on an earlier run, and returns the option that starts the phone with the password in its secure store.
func keepSource(t *testing.T, dir string, src store.Source, password string) nsgotest.Option {
	t.Helper()
	onboarded(t, dir)
	if err := store.Open(dir).SaveSources([]store.Source{src}); err != nil {
		t.Fatal(err)
	}
	return nsgotest.Secrets(map[string]string{store.SourceSecretAccount(src.ID, nas.Password): password})
}

// davSource is a WebDAV source of f's Films folder.
func davSource(f *davtest.Server) store.Source {
	return store.Source{ID: "0123456789abcdef", Kind: nas.KindWebDAV, Name: "Home NAS", Config: map[string]string{nas.URL: f.URL, nas.Root: "Films", nas.User: davtest.User}}
}

// openSettingsSources opens Settings › NAS & Servers.
func (p *phone) openSettingsSources() {
	p.t.Helper()
	p.openTab(settingsTab, msgs.SettingsTitle())
	p.app.WaitFor("id:settings-sources", wait)
	p.app.Press("id:settings-sources")
	p.app.WaitFor("id:sources-page", wait)
}

// secret is what the phone's secure store keeps under account, and whether it keeps anything.
func (p *phone) secret(account string) (string, bool) {
	for item, v := range p.app.Secrets.Items() {
		if item.Account == account {
			return v, true
		}
	}
	return "", false
}

// A WebDAV source is added in Settings: its Test connects, Save keeps it in sources.json without its password, which the secure store keeps, and the Library's NAS group opens its folder, which lists its folders first and its videos, leaving out other files and hidden folders. A video downloads into the source's folder of films, marked downloaded, and plays offline.
func TestAWebDAVSourceIsAddedBrowsedAndDownloaded(t *testing.T) {
	f := davFilms(t, false)
	dir := t.TempDir()
	onboarded(t, dir)
	p := startPhone(t, dir)
	if p.app.Exists("id:films-sources") {
		t.Fatal("the Library shows the NAS group before any source is added")
	}
	p.openSettingsSources()
	p.app.Press("id:sources-add")
	p.app.WaitFor("id:source-page", wait)
	p.app.Press("id:source-kind-webdav")
	p.app.WaitFor("id:source-field-url", wait)
	p.app.Type("id:source-name", "Home NAS")
	p.app.Type("id:source-field-url", f.URL)
	p.app.Type("id:source-field-root", "Films")
	p.app.Type("id:source-field-user", davtest.User)
	p.app.Type("id:source-field-password", davtest.Password)
	p.app.Press("id:source-test")
	p.app.WaitUntil(wait, func() bool { return p.texts("id:source-test-result") == msgs.SourceConnected() })
	p.app.Press("id:source-save")
	p.app.WaitFor("id:sources-page", wait)

	list, err := store.Open(dir).Sources()
	if err != nil || len(list) != 1 {
		t.Fatalf("sources.json holds %v, %v", list, err)
	}
	src := list[0]
	if src.Kind != nas.KindWebDAV || src.Name != "Home NAS" || src.Config[nas.URL] != f.URL || src.Config[nas.Root] != "Films" || src.Config[nas.User] != davtest.User {
		t.Fatalf("sources.json keeps %+v", src)
	}
	data, _ := os.ReadFile(filepath.Join(dir, "sources.json"))
	if strings.Contains(string(data), davtest.Password) {
		t.Fatalf("sources.json holds the password: %s", data)
	}
	if got, ok := p.secret(store.SourceSecretAccount(src.ID, nas.Password)); !ok || got != davtest.Password {
		t.Fatalf("the secure store keeps %q, %v", got, ok)
	}
	if got := p.texts("id:sources-source-" + src.ID); !strings.Contains(got, "Home NAS") || !strings.Contains(got, msgs.SourceSummary("WebDAV", strings.TrimPrefix(f.URL, "http://")+"/Films")) {
		t.Errorf("the source's row says %q", got)
	}

	p.openTab(filmsTab, msgs.FilmsTitle())
	p.app.WaitFor("id:films-source-"+src.ID, wait)
	p.app.Press("id:films-source-" + src.ID)
	p.app.WaitFor("id:browse-video-Holiday.mp4", wait)
	var rows []string
	var walk func(v *nsgotest.View)
	walk = func(v *nsgotest.View) {
		if id := v.Props.TestID; strings.HasPrefix(id, "browse-folder-") || strings.HasPrefix(id, "browse-video-") {
			rows = append(rows, id)
		}
		for _, c := range v.Children {
			walk(c)
		}
	}
	walk(p.app.Find("id:browse-list"))
	if want := []string{"browse-folder-Trip", "browse-video-Holiday.mp4"}; !slices.Equal(rows, want) {
		t.Errorf("the folder lists %q, want %q", rows, want)
	}

	p.app.Press("id:browse-download-Holiday.mp4")
	id := store.SourceFilmID("Holiday.mp4")
	filmDir := filepath.Join("films", store.SourceOwner(src.ID), id)
	p.waitFile(filepath.Join(filmDir, "video.mp4"), int64(len(talkVideo)))
	p.app.WaitFor("id:browse-downloaded-Holiday.mp4", wait)
	var kept store.Film
	data, err = os.ReadFile(filepath.Join(dir, filmDir, "film.json"))
	if err != nil || json.Unmarshal(data, &kept) != nil {
		t.Fatalf("film.json: %s, %v", data, err)
	}
	if kept.Source != src.ID || kept.Path != "Holiday.mp4" || kept.Desktop != store.SourceOwner(src.ID) || kept.Film.ID != id || kept.Film.Title != "Holiday" || kept.Video != "video.mp4" {
		t.Errorf("the film is kept as %+v", kept)
	}
	if got := p.texts("id:browse-video-Holiday.mp4"); !strings.Contains(got, msgs.FilmDownloaded()) {
		t.Errorf("the downloaded video's row says %q", got)
	}

	// Downloaded, the row plays it offline.
	p.app.Press("id:browse-video-Holiday.mp4")
	p.app.WaitFor("id:player-video", wait)
	if src := p.playerSource(); !strings.HasSuffix(src, filepath.Join(filmDir, "video.mp4")) {
		t.Errorf("the player plays %q", src)
	}

	// A folder opens a page of its own.
	p.app.Press("id:player-minimize")
	p.app.WaitFor("id:browse-folder-Trip", wait)
	p.app.Press("id:browse-folder-Trip")
	p.app.WaitFor("id:browse-video-Beach.mov", wait)
}

// A download stopped partway goes on from its part, by a range of the same file, and ends whole.
func TestASourceDownloadResumesFromItsPart(t *testing.T) {
	f := davFilms(t, false)
	dir := t.TempDir()
	src := davSource(f)
	opt := keepSource(t, dir, src, davtest.Password)
	// The part of an earlier run, of the same file as it is now.
	info, err := os.Stat(filepath.Join(f.Dir, "Films/Holiday.mp4"))
	if err != nil {
		t.Fatal(err)
	}
	r, err := nas.Remotefs.Open(t.Context(), nas.Remote(src, map[string]string{nas.Password: davtest.Password}))
	if err != nil {
		t.Fatal(err)
	}
	listed, err := r.Stat(t.Context(), "Holiday.mp4")
	r.Close()
	if err != nil || listed.Size != info.Size() {
		t.Fatalf("stat: %+v, %v", listed, err)
	}
	st := store.Open(dir)
	id := store.SourceFilmID("Holiday.mp4")
	rec := store.Film{Desktop: store.SourceOwner(src.ID), Library: src.Name, Source: src.ID, Path: "Holiday.mp4", ETag: nas.Version(listed), Added: now()}
	rec.Film.ID, rec.Film.Title, rec.Film.VideoSize = id, "Holiday", info.Size()
	if err := st.SaveFilm(rec); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(st.FilmDir(rec.Desktop, id), store.PartFile), talkVideo[:300_000], 0o600); err != nil {
		t.Fatal(err)
	}
	p := startPhone(t, dir, opt)
	p.app.WaitFor("id:films-source-"+src.ID, wait)
	p.app.Press("id:films-source-" + src.ID)
	p.app.WaitFor("id:browse-video-Holiday.mp4", wait)
	if got := p.texts("id:browse-video-Holiday.mp4"); !strings.Contains(got, msgs.FilmPaused(30)) {
		t.Errorf("the row of the part says %q", got)
	}
	f.ForgetRanges()
	p.app.Press("id:browse-download-Holiday.mp4")
	p.waitFile(filepath.Join("films", rec.Desktop, id, "video.mp4"), int64(len(talkVideo)))
	data, _ := os.ReadFile(filepath.Join(dir, "films", rec.Desktop, id, "video.mp4"))
	if string(data) != string(talkVideo) {
		t.Fatal("the video resumed is not the file")
	}
	if got := f.Ranges(); !slices.ContainsFunc(got, func(r string) bool { return strings.HasPrefix(r, "bytes=300000-") }) || slices.Contains(got, "") {
		t.Errorf("the server was asked for %q, want the rest from the part's end", got)
	}
}

// A password the server refuses says so, in the form's Test and on the folder's page, which offers to try again and to edit the source.
func TestASourceWithAWrongPasswordSaysSo(t *testing.T) {
	f := davFilms(t, false)
	dir := t.TempDir()
	src := davSource(f)
	p := startPhone(t, dir, keepSource(t, dir, src, "wrong"))
	p.app.WaitFor("id:films-source-"+src.ID, wait)
	p.app.Press("id:films-source-" + src.ID)
	p.app.WaitFor("id:browse-failed", wait)
	if got := p.texts("id:browse-failed"); !strings.Contains(got, msgs.SourceFailSignIn()) || !strings.Contains(got, msgs.SourceRetry()) || !strings.Contains(got, msgs.SourceEditSource()) {
		t.Errorf("the page says %q", got)
	}
	p.app.Press("id:browse-edit")
	p.app.WaitFor("id:source-page", wait)
	p.app.Press("id:source-test")
	p.app.WaitUntil(wait, func() bool { return p.texts("id:source-test-result") == msgs.SourceFailSignIn() })
	// The right password, typed, is the one the Test and Save take, and the folder reads.
	p.app.Type("id:source-field-password", davtest.Password)
	p.app.Press("id:source-test")
	p.app.WaitUntil(wait, func() bool { return p.texts("id:source-test-result") == msgs.SourceConnected() })
	p.app.Press("id:source-save")
	p.app.WaitForGone("id:source-page", wait)
	p.app.Press("id:browse-retry")
	p.app.WaitFor("id:browse-video-Holiday.mp4", wait)
}

// An SFTP server's key is asked about at the first Test, its fingerprint said; trusted, the form keeps it, Save keeps it in sources.json, and a key changed since is warned of rather than taken.
func TestAnSFTPServersKeyIsTrustedAtTheFirstConnection(t *testing.T) {
	srv := sftptest.New(t)
	if err := os.MkdirAll(filepath.Join(srv.Home, "Films"), 0o755); err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	onboarded(t, dir)
	p := startPhone(t, dir, nsgotest.PendingAlerts())
	p.openSettingsSources()
	p.app.Press("id:sources-add")
	p.app.WaitFor("id:source-page", wait)
	p.app.Press("id:source-kind-sftp")
	p.app.WaitFor("id:source-field-host", wait)
	host, port, _ := strings.Cut(srv.Addr, ":")
	p.app.Type("id:source-field-host", host)
	p.app.Type("id:source-field-port", port)
	p.app.Type("id:source-field-root", "Films")
	p.app.Type("id:source-field-user", sftptest.User)
	p.app.Type("id:source-field-password", sftptest.Password)
	if got := p.texts("id:source-field-hostKey"); got != msgs.SourceTrustedNone() {
		t.Errorf("the server key says %q before any is trusted", got)
	}
	p.app.Press("id:source-test")
	p.app.WaitUntil(wait, func() bool { return len(p.app.Alerts()) == 1 })
	fingerprint := remotefs.Fingerprint(srv.HostKey)
	if a := p.app.Alerts()[0]; a.Title != msgs.SourceTrustTitle(host) || a.Message != msgs.SourceTrustMessage(fingerprint) {
		t.Fatalf("the question is %q: %q", a.Title, a.Message)
	}
	p.app.Answer(1)
	p.app.WaitUntil(wait, func() bool { return p.texts("id:source-test-result") == msgs.SourceConnected() })
	if got := p.texts("id:source-field-hostKey"); got != fingerprint {
		t.Errorf("the server key says %q, want %q", got, fingerprint)
	}
	p.app.Press("id:source-save")
	p.app.WaitFor("id:sources-page", wait)
	list, _ := store.Open(dir).Sources()
	if len(list) != 1 || list[0].Config[nas.HostKey] != srv.HostKey {
		t.Fatalf("sources.json keeps %+v", list)
	}

	// Another key in its place is warned of, naming both, and not taken while cancelled.
	other := sftptest.New(t)
	p.app.Press("id:sources-source-" + list[0].ID)
	p.app.WaitFor("id:source-page", wait)
	p.app.Clear("id:source-field-port")
	_, otherPort, _ := strings.Cut(other.Addr, ":")
	p.app.Type("id:source-field-port", otherPort)
	p.app.Press("id:source-test")
	p.app.WaitUntil(wait, func() bool {
		a := p.app.Alerts()
		return len(a) > 0 && a[len(a)-1].Title == msgs.SourceKeyChangedTitle(host) || p.texts("id:source-test-result") != ""
	})
	if a := p.app.Alerts()[len(p.app.Alerts())-1]; a.Title != msgs.SourceKeyChangedTitle(host) || a.Message != msgs.SourceKeyChangedMessage(fingerprint, remotefs.Fingerprint(other.HostKey)) {
		t.Fatalf("the warning is %q: %q", a.Title, a.Message)
	}
	p.app.Answer(0)
	p.app.WaitUntil(wait, func() bool { return p.texts("id:source-test-result") == msgs.SourceNotTrusted() })
	if got := p.texts("id:source-field-hostKey"); got != fingerprint {
		t.Errorf("cancelled, the server key says %q", got)
	}
}

// A WebDAV server over HTTPS whose certificate the system does not trust, as a NAS's own, is asked about by its fingerprint; trusted, the source reads, by that certificate alone.
func TestAWebDAVCertificateIsTrustedAtTheFirstConnection(t *testing.T) {
	f := davFilms(t, true)
	dir := t.TempDir()
	src := davSource(f)
	p := startPhone(t, dir, keepSource(t, dir, src, davtest.Password), nsgotest.PendingAlerts())
	p.app.WaitFor("id:films-source-"+src.ID, wait)
	p.app.Press("id:films-source-" + src.ID)
	p.app.WaitFor("id:browse-failed", wait)
	if got := p.texts("id:browse-failed"); !strings.Contains(got, msgs.SourceFailCertificate("127.0.0.1")) {
		t.Errorf("the page says %q", got)
	}
	p.app.Press("id:browse-edit")
	p.app.WaitFor("id:source-page", wait)
	p.app.Press("id:source-test")
	p.app.WaitUntil(wait, func() bool { return len(p.app.Alerts()) == 1 })
	a := p.app.Alerts()[0]
	if a.Title != msgs.SourceTrustTitle("127.0.0.1") || !strings.Contains(a.Message, "SHA256 ") {
		t.Fatalf("the question is %q: %q", a.Title, a.Message)
	}
	p.app.Answer(1)
	p.app.WaitUntil(wait, func() bool { return p.texts("id:source-test-result") == msgs.SourceConnected() })
	p.app.Press("id:source-save")
	p.app.WaitForGone("id:source-page", wait)
	list, _ := store.Open(dir).Sources()
	if len(list) != 1 || !strings.HasPrefix(list[0].Config[nas.CertKey], "SHA256 ") {
		t.Fatalf("sources.json keeps %+v", list)
	}
	p.app.Press("id:browse-retry")
	p.app.WaitFor("id:browse-video-Holiday.mp4", wait)
}

// A source on the local network the user does not let the app reach says so, with the way to the system's settings; Kando asks before its first connection, and not for this device's own loopback, which is no network.
func TestASourceWithTheLocalNetworkRefusedOpensSettings(t *testing.T) {
	dir := t.TempDir()
	// A NAS on the local network, which the phone never reaches: it asks first, and is refused.
	src := store.Source{ID: "0123456789abcdef", Kind: nas.KindSMB, Name: "Home NAS", Config: map[string]string{nas.Host: "192.168.1.10", nas.Share: "media", nas.User: "family"}}
	p := startPhone(t, dir, keepSource(t, dir, src, davtest.Password))
	p.app.PermissionAnswers[core.PermissionLocalNetwork] = core.PermissionDenied
	p.app.WaitFor("id:films-source-"+src.ID, wait)
	p.app.Press("id:films-source-" + src.ID)
	p.app.WaitFor("id:browse-failed", wait)
	if got := p.texts("id:browse-failed"); !strings.Contains(got, msgs.SourceFailLocalNetwork()) {
		t.Errorf("the page says %q", got)
	}
	p.app.Press("id:browse-settings")
	if got := p.app.PermissionSettings; len(got) != 1 || got[0] != core.PermissionLocalNetwork {
		t.Errorf("the settings opened are %v", got)
	}
}

// Removing a source asks first, forgets it and its password, and keeps the films downloaded from it, which the Library still lists; the NAS group goes with the last source.
func TestRemovingASourceKeepsItsFilms(t *testing.T) {
	f := davFilms(t, false)
	dir := t.TempDir()
	src := davSource(f)
	opt := keepSource(t, dir, src, davtest.Password)
	p := startPhone(t, dir, opt, nsgotest.PendingAlerts())
	p.app.WaitFor("id:films-source-"+src.ID, wait)
	p.app.Press("id:films-source-" + src.ID)
	p.app.WaitFor("id:browse-download-Holiday.mp4", wait)
	p.app.Press("id:browse-download-Holiday.mp4")
	p.app.WaitFor("id:browse-downloaded-Holiday.mp4", wait)
	p.openSettingsSources()
	p.app.Press("id:sources-source-" + src.ID)
	p.app.WaitFor("id:source-remove", wait)
	p.app.Press("id:source-remove")
	p.app.WaitUntil(wait, func() bool { return len(p.app.Alerts()) == 1 })
	if a := p.app.Alerts()[0]; a.Title != msgs.SourceRemoveTitle("Home NAS") || a.Message != msgs.SourceRemoveMessage() {
		t.Fatalf("the question is %q: %q", a.Title, a.Message)
	}
	p.app.Answer(1)
	p.app.WaitForGone("id:sources-source-"+src.ID, wait)
	p.app.WaitUntil(wait, func() bool { list, _ := store.Open(dir).Sources(); return len(list) == 0 })
	p.app.WaitUntil(wait, func() bool { _, ok := p.secret(store.SourceSecretAccount(src.ID, nas.Password)); return !ok })
	p.openTab(filmsTab, msgs.FilmsTitle())
	p.app.WaitFor(filmRow(store.SourceOwner(src.ID), store.SourceFilmID("Holiday.mp4")), wait)
	if p.app.Exists("id:films-sources") {
		t.Error("the Library shows the NAS group with no source left")
	}
}

// What keeps a source from being tried is said before anything connects.
func TestASourceFormSaysWhatIsMissing(t *testing.T) {
	dir := t.TempDir()
	onboarded(t, dir)
	p := startPhone(t, dir)
	p.openSettingsSources()
	p.app.Press("id:sources-add")
	p.app.WaitFor("id:source-page", wait)
	p.app.Press("id:source-test")
	if got := p.texts("id:source-test-result"); got != msgs.SourceNeedsHost() {
		t.Errorf("an empty SMB form says %q", got)
	}
	// SMB requires signing unless its switch is on, which says what that risks.
	if got := p.texts("id:source-page"); !strings.Contains(got, msgs.SourceSMBWarning()) || strings.Contains(got, msgs.SourceNoSigningRisk()) {
		t.Errorf("the SMB form warns %q", got)
	}
	p.app.Press("id:source-field-noSigning")
	p.app.WaitUntil(wait, func() bool { return strings.Contains(p.texts("id:source-page"), msgs.SourceNoSigningRisk()) })
	p.app.Type("id:source-field-host", "nas.local")
	p.app.Type("id:source-field-root", "films")
	p.app.Press("id:source-save")
	if got := p.texts("id:source-problem"); got != msgs.SourceNeedsShareForRoot() {
		t.Errorf("a folder without a share says %q", got)
	}
	if list, _ := store.Open(dir).Sources(); len(list) != 0 {
		t.Errorf("a source that misses its share was saved: %v", list)
	}
}

// A film downloaded from a NAS says it comes from the NAS, by the name the user gave it, in its row and the player's Info tab, not that it is a file of the computer's.
func TestASourceFilmSaysItsSource(t *testing.T) {
	dir := t.TempDir()
	src := davSource(davFilms(t, false))
	st := store.Open(dir)
	f := store.Film{Desktop: store.SourceOwner(src.ID), Library: src.Name, Source: src.ID, Path: "Holiday.mp4", Video: "video.mp4", Added: now(),
		Film: sync.Film{ID: store.SourceFilmID("Holiday.mp4"), Title: "Holiday", Source: "local", VideoSize: int64(len(talkVideo))}}
	if err := st.SaveFilm(f); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(st.FilmDir(f.Desktop, f.Film.ID), f.Video), talkVideo, 0o600); err != nil {
		t.Fatal(err)
	}
	p := startPhone(t, dir, keepSource(t, dir, src, davtest.Password))
	row := filmRow(f.Desktop, f.Film.ID)
	p.app.WaitFor(row, wait)
	if got := p.texts(row); !strings.Contains(got, src.Name+" · ") || strings.Contains(got, msgs.FilmsSourceLocal()) {
		t.Fatalf("the NAS film's row reads %q, want its source %q", got, src.Name)
	}
	p.app.Press(row)
	p.app.WaitFor("id:player-video", wait)
	p.app.Press("id:player-tab-info")
	p.app.WaitFor("id:player-info-source", wait)
	if got, want := p.texts("id:player-info-source"), msgs.PlayerInfoSource()+"\n"+src.Name; got != want {
		t.Fatalf("the Info tab's source reads %q, want %q", got, want)
	}
}
