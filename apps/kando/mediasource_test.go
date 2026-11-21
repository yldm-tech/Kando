package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"kando/apps/kando/feedback"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/mediasource"
	"kando/apps/kando/settings"

	"github.com/cloudsoda/go-smb2"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// fakeSource is a media source in memory for the app's tests (docs/media-sources.md 19): a fixed tree of folders and files, each file's bytes read from a file on disk or given, which can be set unreachable. It has no method that writes, as no Driver has. It counts the connections made to it and records the secrets each was made with.
type fakeSource struct {
	mu          sync.Mutex
	files       map[string]fakeSourceFile
	unreachable error
	opened      []map[string]string
	closed      int
}

// fakeSourceFile is a file of a fakeSource: its bytes, or the file on disk they are read from, and its time. size, when set, is the size it lists with in place of its bytes', as a scene's films have.
type fakeSourceFile struct {
	data     string
	disk     string
	modified time.Time
	size     int64
}

func newFakeSource(files map[string]fakeSourceFile) *fakeSource {
	return &fakeSource{files: files}
}

// open is mediaSourceUI.open for the tests: a driver of f, made with secrets, or f's unreachable error.
func (f *fakeSource) open(_ context.Context, _ mediasource.Kind, _ mediasource.Source, secrets map[string]string) (mediasource.Driver, error) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.opened = append(f.opened, secrets)
	if f.unreachable != nil {
		return nil, f.unreachable
	}
	return fakeSourceDriver{f}, nil
}

// lastSecrets are the secrets the last connection was made with.
func (f *fakeSource) lastSecrets() map[string]string {
	f.mu.Lock()
	defer f.mu.Unlock()
	if len(f.opened) == 0 {
		return nil
	}
	return f.opened[len(f.opened)-1]
}

type fakeSourceDriver struct{ f *fakeSource }

func (d fakeSourceDriver) size(file fakeSourceFile) int64 {
	if file.size > 0 {
		return file.size
	}
	if file.disk != "" {
		if fi, err := os.Stat(file.disk); err == nil {
			return fi.Size()
		}
	}
	return int64(len(file.data))
}

func (d fakeSourceDriver) List(_ context.Context, dir string) ([]mediasource.Entry, error) {
	dir, err := mediasource.CleanPath(dir)
	if err != nil {
		return nil, err
	}
	d.f.mu.Lock()
	defer d.f.mu.Unlock()
	seen := map[string]bool{}
	var out []mediasource.Entry
	found := dir == ""
	for p, file := range d.f.files {
		rest, ok := strings.CutPrefix(p, dir+"/")
		if dir == "" {
			rest, ok = p, true
		}
		if !ok {
			continue
		}
		found = true
		name, _, sub := strings.Cut(rest, "/")
		if seen[name] {
			continue
		}
		seen[name] = true
		if sub {
			out = append(out, mediasource.Entry{Name: name, Dir: true})
		} else {
			out = append(out, mediasource.Entry{Name: name, Size: d.size(file), Modified: file.modified, Version: "v1"})
		}
	}
	if !found {
		return nil, mediasource.ErrNotExist
	}
	slices.SortFunc(out, func(a, b mediasource.Entry) int { return strings.Compare(a.Name, b.Name) })
	return out, nil
}

func (d fakeSourceDriver) Stat(ctx context.Context, p string) (mediasource.Entry, error) {
	p, err := mediasource.CleanPath(p)
	if err != nil {
		return mediasource.Entry{}, err
	}
	d.f.mu.Lock()
	file, ok := d.f.files[p]
	d.f.mu.Unlock()
	if ok {
		return mediasource.Entry{Name: path.Base(p), Size: d.size(file), Modified: file.modified, Version: "v1"}, nil
	}
	if _, err := d.List(ctx, p); err != nil {
		return mediasource.Entry{}, err
	}
	return mediasource.Entry{Name: path.Base(p), Dir: true}, nil
}

func (d fakeSourceDriver) Open(_ context.Context, p string, offset, length int64) (io.ReadCloser, error) {
	p, err := mediasource.CleanPath(p)
	if err != nil {
		return nil, err
	}
	d.f.mu.Lock()
	file, ok := d.f.files[p]
	d.f.mu.Unlock()
	if !ok {
		return nil, mediasource.ErrNotExist
	}
	data := file.data
	if file.disk != "" {
		b, err := os.ReadFile(file.disk)
		if err != nil {
			return nil, err
		}
		data = string(b)
	}
	data = data[min(int(offset), len(data)):]
	if length >= 0 {
		data = data[:min(int(length), len(data))]
	}
	return io.NopCloser(strings.NewReader(data)), nil
}

func (d fakeSourceDriver) Close() error {
	d.f.mu.Lock()
	defer d.f.mu.Unlock()
	d.f.closed++
	return nil
}

// A source is added in 设置 › 媒体来源, tested, kept with its password in the system's secure store rather than in settings.json, read back, edited without typing the password again, and removed, which forgets the password.
func TestMediaSourcesAddTestEditRemove(t *testing.T) {
	a, tt, updates := newTestApp(t, &fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	src := newFakeSource(map[string]fakeSourceFile{"media/Heat.mkv": {data: "heat"}, "backup/old.mp4": {data: "old"}})
	a.sources.open = src.open
	msgs := testlocale.Messages()
	tt = settingsOpenAt(a, tt, settings.SectionMediaSources)
	wantText(t, tt, msgs.SettingsSourcesEmpty())
	tt.Press("id:media-source-add")
	if !a.modalOpen() || !tt.Exists("id:media-source-dialog") {
		t.Fatal("the dialog did not open, or holds no modal flag")
	}
	wantText(t, tt, msgs.SettingsSourcesSMBWarning())
	tt.Press("id:media-source-dialog-save")
	wantText(t, tt, msgs.AccountLocationNeedsHost())
	tt.Type("id:media-source-field-host", "nas.local")
	tt.Type("id:media-source-field-user", "kando")
	tt.Type("id:media-source-field-password", "pa55-w0rd!")

	// Without a share, Test lists the server's shares.
	tt.Press("id:media-source-dialog-test")
	waitUntil(t, tt, a, updates, func() bool { return !a.sources.tests[""].busy })
	wantText(t, tt, msgs.SettingsSourcesShares("backup, media"))
	if got := src.lastSecrets()[mediasource.SMBPassword]; got != "pa55-w0rd!" {
		t.Fatalf("tested with password %q", got)
	}

	tt.Press("id:media-source-dialog-save")
	waitUntil(t, tt, a, updates, func() bool { return !a.sources.editing })
	if a.modalOpen() || len(a.sources.list) != 1 {
		t.Fatalf("modal %v list %+v", a.modalOpen(), a.sources.list)
	}
	s := a.sources.list[0]
	if !mediasource.ValidID(s.ID) || s.Name != "nas.local" || s.Kind != mediasource.KindSMB || s.Config[mediasource.SMBPassword] != "" {
		t.Fatalf("saved %+v", s)
	}
	wantText(t, tt, msgs.AccountLocationSummary("SMB", "nas.local"))
	raw, err := os.ReadFile(a.settingsPath)
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(raw), "pa55-w0rd!") || !strings.Contains(string(raw), `"mediaSources"`) {
		t.Fatalf("settings.json: %s", raw)
	}
	store := a.mediaSourceSecretStore()
	if v, ok, err := store.Get(mediaSourceSecretAccount(s.ID, mediasource.SMBPassword)); err != nil || !ok || v != "pa55-w0rd!" {
		t.Fatalf("secure store %q %v %v", v, ok, err)
	}

	b := newApp(&fakePipeline{}, func(func()) {})
	b.restoreSettings(loadSavedSettings(a.settingsPath))
	if len(b.sources.list) != 1 || b.sources.list[0].ID != s.ID || b.sources.list[0].Config[mediasource.SMBHost] != "nas.local" {
		t.Fatalf("reloaded %+v", b.sources.list)
	}

	// Editing leaves the password field empty; Save keeps the one stored, and Test reads it from the store.
	tt.Press("id:media-source-edit-" + s.ID)
	if a.MediaSourceIsNew() || a.MediaSourceDraft().Values[mediasource.SMBPassword] != "" {
		t.Fatalf("draft %+v", *a.MediaSourceDraft())
	}
	tt.Type("id:media-source-field-share", "media")
	tt.Press("id:media-source-dialog-save")
	waitUntil(t, tt, a, updates, func() bool { return !a.sources.editing })
	tt.Press("id:media-source-test-" + s.ID)
	waitUntil(t, tt, a, updates, func() bool { return !a.sources.tests[s.ID].busy })
	wantText(t, tt, msgs.AccountLocationConnected())
	if got := src.lastSecrets()[mediasource.SMBPassword]; got != "pa55-w0rd!" {
		t.Fatalf("tested the saved source with password %q", got)
	}
	if got := loadSavedSettings(a.settingsPath).MediaSources; len(got) != 1 || got[0].Config[mediasource.SMBShare] != "media" {
		t.Fatalf("edited %+v", got)
	}

	tt.Press("id:media-source-remove-" + s.ID)
	if !tt.Exists("id:media-source-remove-confirm") || !a.modalOpen() {
		t.Fatal("no confirmation")
	}
	tt.Press("id:media-source-remove-cancel")
	if len(a.sources.list) != 1 || a.modalOpen() {
		t.Fatal("Cancel removed the source")
	}
	tt.Press("id:media-source-remove-" + s.ID)
	tt.Press("id:media-source-remove-ok")
	if len(a.sources.list) != 0 || a.modalOpen() {
		t.Fatalf("list %+v", a.sources.list)
	}
	deadline := time.Now().Add(5 * time.Second)
	for {
		if _, ok, _ := store.Get(mediaSourceSecretAccount(s.ID, mediasource.SMBPassword)); !ok {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("the password of the removed source is still kept")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if raw, err := os.ReadFile(a.settingsPath); err != nil || strings.Contains(string(raw), "mediaSources") {
		t.Fatalf("the removed source is still in settings.json: %s, %v", raw, err)
	}
}

// A Test that fails says why in the interface's language: a server that did not sign says to turn on the switch for a NAS without signing.
func TestMediaSourceTestSaysWhy(t *testing.T) {
	a, tt, updates := newTestApp(t, &fakePipeline{})
	src := newFakeSource(nil)
	src.unreachable = fmt.Errorf("dial: %w", &smb2.InvalidResponseError{Message: "guest account doesn't support signing"})
	a.sources.open = src.open
	tt = settingsOpenAt(a, tt, settings.SectionMediaSources)
	tt.Press("id:media-source-add")
	tt.Type("id:media-source-field-host", "nas.local")
	tt.Type("id:media-source-field-user", "guest")
	tt.Press("id:media-source-dialog-test")
	waitUntil(t, tt, a, updates, func() bool { return !a.sources.tests[""].busy })
	wantText(t, tt, testlocale.Messages().SettingsSourcesFailSigning())
	src.unreachable = errors.New("nothing known")
	tt.Press("id:media-source-dialog-test")
	waitUntil(t, tt, a, updates, func() bool { return !a.sources.tests[""].busy })
	wantText(t, tt, testlocale.Messages().AccountLocationFailOther())
}

// The switch for a NAS without signing turns the requirement off, and the dialog says what that risks while it is on.
func TestMediaSourceSigningSwitch(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	msgs := testlocale.Messages()
	tt = settingsOpenAt(a, tt, settings.SectionMediaSources)
	tt.Press("id:media-source-add")
	wantNoText(t, tt, msgs.SettingsSourcesNoSigningRisk())
	tt.Press("id:media-source-field-noSigning")
	wantText(t, tt, msgs.SettingsSourcesNoSigningRisk())
	s, _, _, ok := a.mediaSourceDraftSource()
	if !ok || mediasource.SMBRemote(s, "").RequireSigning {
		t.Fatalf("the draft %+v still requires signing", s)
	}
}

// Without a secure store, a source with a password is not saved, and the dialog says why; a guest's, without one, is.
func TestMediaSourceWithoutASecretStore(t *testing.T) {
	updates := make(chan func(), 4)
	a := newApp(&fakePipeline{}, func(fn func()) { updates <- fn })
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.AddMediaSource()
	d := a.MediaSourceDraft()
	d.Values[mediasource.SMBHost], d.Values[mediasource.SMBShare], d.Values[mediasource.SMBUser], d.Values[mediasource.SMBPassword] = "nas", "media", "kando", "secret"
	a.SaveMediaSource()
	if !a.MediaSourceSaving() {
		t.Fatal("not keeping the secret")
	}
	select {
	case fn := <-updates:
		fn()
	case <-time.After(5 * time.Second):
		t.Fatal("the save did not answer")
	}
	if len(a.sources.list) != 0 || a.MediaSourceProblem() != testlocale.Messages().SettingsSourcesNoSecretStore() || !a.MediaSourceEditing() {
		t.Fatalf("list %+v problem %q", a.sources.list, a.MediaSourceProblem())
	}
	d.Values[mediasource.SMBUser], d.Values[mediasource.SMBPassword] = "guest", ""
	d.Values[mediasource.SMBNoSigning] = "true"
	a.SaveMediaSource()
	if len(a.sources.list) != 1 || a.MediaSourceEditing() || a.sources.list[0].Config[mediasource.SMBNoSigning] != "true" {
		t.Fatalf("the guest's source %+v, editing %v", a.sources.list, a.MediaSourceEditing())
	}
}

// A Test pressed while Save is keeping the secrets does not drop the save's answer: the dialog stops saving and says how the save went, rather than staying on 正在保存 with Save greyed out.
func TestMediaSourceTestWhileSaving(t *testing.T) {
	updates := make(chan func(), 4)
	a := newApp(&fakePipeline{}, func(fn func()) { updates <- fn })
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.sources.open = newFakeSource(map[string]fakeSourceFile{"Heat.mkv": {data: "heat"}}).open
	a.AddMediaSource()
	d := a.MediaSourceDraft()
	d.Values[mediasource.SMBHost], d.Values[mediasource.SMBShare], d.Values[mediasource.SMBUser], d.Values[mediasource.SMBPassword] = "nas", "media", "kando", "secret"
	a.SaveMediaSource()
	a.TestMediaSource("")
	for range 2 {
		select {
		case fn := <-updates:
			fn()
		case <-time.After(5 * time.Second):
			t.Fatal("the save or the Test did not answer")
		}
	}
	if a.MediaSourceSaving() || a.MediaSourceProblem() != testlocale.Messages().SettingsSourcesNoSecretStore() {
		t.Fatalf("saving %v problem %q", a.MediaSourceSaving(), a.MediaSourceProblem())
	}
}

// A save whose dialog closed with the settings does not finish in the dialog opened next: its answer neither adds the old draft nor says anything in the new dialog.
func TestMediaSourceSaveOfAClosedDialog(t *testing.T) {
	updates := make(chan func(), 4)
	a := newApp(&fakePipeline{}, func(fn func()) { updates <- fn })
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	a.AddMediaSource()
	d := a.MediaSourceDraft()
	d.Values[mediasource.SMBHost], d.Values[mediasource.SMBShare], d.Values[mediasource.SMBUser], d.Values[mediasource.SMBPassword] = "nas", "media", "kando", "secret"
	a.SaveMediaSource()
	a.prefs.open = false
	a.mediaSourceModals()
	a.AddMediaSource()
	select {
	case fn := <-updates:
		fn()
	case <-time.After(5 * time.Second):
		t.Fatal("the save did not answer")
	}
	if !a.MediaSourceEditing() || a.MediaSourceSaving() || a.MediaSourceProblem() != "" || len(a.sources.list) != 0 {
		t.Fatalf("editing %v saving %v problem %q list %+v", a.MediaSourceEditing(), a.MediaSourceSaving(), a.MediaSourceProblem(), a.sources.list)
	}
}

// settings.json's sources are tidied as they are read: one without a valid ID, a kind or a name, or with an ID seen before, is dropped; an SMB source keeps only its fields that are not secret, and a plugin's, whose kind this version may not know, keeps its own.
func TestSavedMediaSourcesClean(t *testing.T) {
	s := savedMediaSources{MediaSources: []savedMediaSource{
		{ID: "0123456789abcdef", Kind: "smb", Name: " NAS ", Config: map[string]string{"host": " nas ", "password": "leaked", "other": "x", "noSigning": "no", "port": ""}},
		{ID: "0123456789abcdef", Kind: "smb", Name: "again"},
		{ID: "0123456789abcde0", Kind: "smb", Name: ""},
		{ID: "not an id", Kind: "smb", Name: "bad"},
		{ID: "0123456789abcde1", Kind: "plugin:kando.webdav:webdav", Name: "DAV", Config: map[string]string{"endpoint": "https://dav"}},
	}}
	s.clean()
	if len(s.MediaSources) != 2 {
		t.Fatalf("clean %+v", s.MediaSources)
	}
	if m := s.MediaSources[0]; m.Name != "NAS" || len(m.Config) != 1 || m.Config["host"] != "nas" {
		t.Fatalf("smb %+v", m)
	}
	if m := s.MediaSources[1]; m.Config["endpoint"] != "https://dav" {
		t.Fatalf("plugin %+v", m)
	}
}

// Every value of a source but its port and signing switch, and what its dialog holds, its password among them, is left out of the log and the feedback issue.
func TestMediaSourceSecretsAreScrubbed(t *testing.T) {
	a := newApp(&fakePipeline{}, func(func()) {})
	a.sources.list = []mediasource.Source{{ID: "0123456789abcdef", Kind: "smb", Name: "Home", Config: map[string]string{"host": "nas-7c6b5a.local", "share": "share-49382", "root": "root-71605", "user": "user-1a2b3c", "domain": "dom-9z8y7x"}}}
	a.AddMediaSource()
	a.MediaSourceDraft().Values[mediasource.SMBPassword] = "typed-pa55-q1w2e3"
	secrets := []string{"nas-7c6b5a.local", "share-49382", "root-71605", "user-1a2b3c", "dom-9z8y7x", "typed-pa55-q1w2e3"}
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

// The dialog closes with the page that draws it, and gives back its modal flag.
func TestMediaSourceDialogClosesWithTheSettings(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	tt = settingsOpenAt(a, tt, settings.SectionMediaSources)
	tt.Press("id:media-source-add")
	a.OpenSettings(settings.SectionGeneral)
	refresh(tt, a)
	if a.sources.editing || a.modalOpen() {
		t.Fatalf("editing %v modal %v", a.sources.editing, a.modalOpen())
	}
}

// browseSource adds a source of a share to a, read from files, and returns it with the fake behind it.
func browseSource(a *app, files map[string]fakeSourceFile) (mediasource.Source, *fakeSource) {
	src := newFakeSource(files)
	s := mediasource.Source{ID: "0123456789abcdef", Kind: mediasource.KindSMB, Name: "Living Room NAS", Config: map[string]string{mediasource.SMBHost: "nas.local", mediasource.SMBShare: "media", mediasource.SMBUser: "kando"}}
	a.sources.list = []mediasource.Source{s}
	a.sources.open = src.open
	return s, src
}

// The sidebar's 媒体来源 row shows the page, which lists the folders and the media files of the source, its subtitle files as a tag on their video; a video chosen is imported through the dialog, which queues its address, and is then marked as in the library.
func TestMediaSourceBrowseAndImport(t *testing.T) {
	a, tt, updates := newTestApp(t, &fakePipeline{})
	msgs := testlocale.Messages()
	if tt.Exists("id:source-" + pageMediaSources) {
		t.Fatal("the row shows without a source")
	}
	s, _ := browseSource(a, map[string]fakeSourceFile{
		"Movies/Heat (1995).mkv": {data: "heat"},
		"Movies/Heat (1995).srt": {data: "1\n"},
		"Movies/Ronin.mp4":       {data: "ronin"},
		"Movies/notes.txt":       {data: "x"},
		"Docs/a.mkv":             {data: "a"},
		".hidden/b.mkv":          {data: "b"},
	})
	refresh(tt, a)
	tt.Press("id:source-" + pageMediaSources)
	waitUntil(t, tt, a, updates, func() bool { return !a.sources.browse.loading })
	if a.page != pageMediaSources || !tt.Exists("id:media-sources-entry-Docs") || !tt.Exists("id:media-sources-entry-Movies") || tt.Exists("id:media-sources-entry-.hidden") {
		t.Fatalf("page %q; texts %q", a.page, tt.Texts())
	}
	tt.Press("id:media-sources-entry-Movies")
	waitUntil(t, tt, a, updates, func() bool { return !a.sources.browse.loading })
	heat := "Movies/Heat (1995).mkv"
	if !tt.Exists("id:media-sources-entry-"+heat) || !tt.Exists("id:media-sources-subtitles-"+heat) || !tt.Exists("id:media-sources-entry-Movies/Ronin.mp4") {
		t.Fatalf("texts %q", tt.Texts())
	}
	if tt.Exists("id:media-sources-entry-Movies/notes.txt") || tt.Exists("id:media-sources-entry-Movies/Heat (1995).srt") || tt.Exists("id:media-sources-subtitles-Movies/Ronin.mp4") {
		t.Fatalf("a file that is no video is listed: %q", tt.Texts())
	}
	if tt.Exists("id:media-sources-import") {
		t.Fatal("Import shows with nothing chosen")
	}
	tt.Press("id:media-sources-choose-" + heat)
	if a.MediaBrowseChosen() != 1 {
		t.Fatalf("chosen %d", a.MediaBrowseChosen())
	}
	tt.Press("id:media-sources-import")
	if !tt.Exists("id:media-sources-import-dialog") || !a.modalOpen() {
		t.Fatal("no import dialog")
	}
	wantText(t, tt, msgs.MediaSourceImportTitle(1, "Living Room NAS"))
	tt.Press("id:media-sources-import-start")
	if a.modalOpen() || tt.Exists("id:media-sources-import-dialog") {
		t.Fatal("the dialog stayed")
	}
	ref := mediasource.Ref{Source: s.ID, Path: heat}.String()
	if _, ok := a.queueFindSource(ref); !ok {
		t.Fatalf("no job for %s among %+v", ref, a.library.Jobs)
	}
	refresh(tt, a)
	wantText(t, tt, msgs.MediaSourceImported(1))
	if !tt.Exists("id:media-sources-in-library-"+heat) || tt.Exists("id:media-sources-choose-"+heat) || a.MediaBrowseChosen() != 0 {
		t.Fatalf("the video imported is not marked: %q", tt.Texts())
	}
	a.OpenMediaBrowseCrumb(0)
	waitUntil(t, tt, a, updates, func() bool { return !a.sources.browse.loading })
	if a.sources.browse.dir != "" || !tt.Exists("id:media-sources-entry-Docs") {
		t.Fatalf("up to the root: dir %q", a.sources.browse.dir)
	}
}

// A folder that cannot be listed says why, in the interface's language, with Retry and Edit Source…; offline mode lists no source on the internet, and says so.
func TestMediaSourceBrowseFails(t *testing.T) {
	a, tt, updates := newTestApp(t, &fakePipeline{})
	msgs := testlocale.Messages()
	s, src := browseSource(a, map[string]fakeSourceFile{"a.mkv": {data: "a"}})
	src.unreachable = fmt.Errorf("mount: %w", &smb2.ResponseError{Code: 0xC000006D})
	a.mediaSourceOpenPage()
	refresh(tt, a)
	waitUntil(t, tt, a, updates, func() bool { return !a.sources.browse.loading })
	wantText(t, tt, msgs.AccountLocationFailSignIn())
	src.unreachable = nil
	tt.Press("id:media-sources-retry")
	waitUntil(t, tt, a, updates, func() bool { return !a.sources.browse.loading })
	if !tt.Exists("id:media-sources-entry-a.mkv") {
		t.Fatalf("Retry did not list the folder: %q", tt.Texts())
	}

	a.privacy.offline = true
	a.sources.localHost = func(context.Context, string) (bool, error) { return false, nil }
	a.RetryMediaBrowse()
	waitUntil(t, tt, a, updates, func() bool { return !a.sources.browse.loading })
	wantText(t, tt, msgs.MediaSourceOffline())
	a.sources.localHost = func(context.Context, string) (bool, error) { return true, nil }
	a.RetryMediaBrowse()
	waitUntil(t, tt, a, updates, func() bool { return !a.sources.browse.loading })
	if !tt.Exists("id:media-sources-entry-a.mkv") {
		t.Fatalf("a NAS on the local network is not listed offline: %q", tt.Texts())
	}
	// Offline mode adds a media source's video and leaves it to the copy to judge where it is.
	if err := a.privacyRefusesLink(mediasource.Ref{Source: s.ID, Path: "a.mkv"}.String()); err != nil {
		t.Fatalf("offline refused a media source's video: %v", err)
	}

	a.EditMediaBrowseSource()
	refresh(tt, a)
	if !a.prefs.open || a.SettingsSection() != settings.SectionMediaSources || !a.MediaSourceEditing() || a.sources.editID != s.ID {
		t.Fatalf("Edit Source… opened %v %q editing %v %q", a.prefs.open, a.SettingsSection(), a.MediaSourceEditing(), a.sources.editID)
	}
}

// A job copies a media source's video through the pool, connecting with the password the secure store keeps.
func TestMediaSourceCopierReadsTheStoredPassword(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	s, src := browseSource(a, map[string]fakeSourceFile{"Movies/Heat.mkv": {data: "heat"}})
	if err := a.mediaSourceSecretStore().Set(mediaSourceSecretAccount(s.ID, mediasource.SMBPassword), "pa55"); err != nil {
		t.Fatal(err)
	}
	a.mediaSourcePoolSync()
	refresh(tt, a)
	dir := t.TempDir()
	got, err := a.mediaSourceCopier().Copy(context.Background(), mediasource.Ref{Source: s.ID, Path: "Movies/Heat.mkv"}.String(), dir)
	if err != nil {
		t.Fatal(err)
	}
	if b, err := os.ReadFile(got); err != nil || string(b) != "heat" || got != filepath.Join(dir, "Living Room NAS", "Movies", "Heat.mkv") {
		t.Fatalf("copied %q: %q, %v", got, b, err)
	}
	if src.lastSecrets()[mediasource.SMBPassword] != "pa55" {
		t.Fatalf("connected with %v", src.lastSecrets())
	}
}

// storedMediaSource adds to a a source saved in a settings.json of the test's own whose password the secure store keeps as password, and returns it.
func storedMediaSource(t *testing.T, a *app, password string) mediasource.Source {
	t.Helper()
	s := mediasource.Source{ID: "0123456789abcdef", Kind: mediasource.KindSMB, Name: "NAS", Config: map[string]string{mediasource.SMBHost: "nas.local", mediasource.SMBShare: "media", mediasource.SMBUser: "kando"}}
	a.sources.list = []mediasource.Source{s}
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	if err := a.saveSettings(); err != nil {
		t.Fatal(err)
	}
	if err := a.mediaSourceSecretStore().Set(mediaSourceSecretAccount(s.ID, mediasource.SMBPassword), password); err != nil {
		t.Fatal(err)
	}
	return s
}

// mediaSourceStored is the password the secure store keeps for the source id, "" for none.
func mediaSourceStored(t *testing.T, a *app, id string) string {
	t.Helper()
	v, _, err := a.mediaSourceSecretStore().Get(mediaSourceSecretAccount(id, mediasource.SMBPassword))
	if err != nil {
		t.Fatal(err)
	}
	return v
}

// unwritableSettings points a's settings.json under a file, so saving the settings fails.
func unwritableSettings(t *testing.T, a *app) {
	t.Helper()
	file := filepath.Join(t.TempDir(), "file")
	if err := os.WriteFile(file, nil, 0o600); err != nil {
		t.Fatal(err)
	}
	a.settingsPath = filepath.Join(file, "settings.json")
}

// mediaSourceSecretsSettle waits until a save has written its secrets (written), then applies the background updates until none is left and nothing is changing the sources' secrets any more, as putting them back runs off the engine's goroutine without an update of its own.
func mediaSourceSecretsSettle(t *testing.T, tt *nsgotest.App, a *app, updates chan func(), written func() bool) {
	t.Helper()
	deadline := time.Now().Add(5 * time.Second)
	for !written() {
		if time.Now().After(deadline) {
			t.Fatal("the save wrote no secret")
		}
		time.Sleep(5 * time.Millisecond)
	}
	for {
		select {
		case fn := <-updates:
			apply(tt, a, fn)
			continue
		default:
		}
		if len(a.sources.secretsHeld) == 0 && len(updates) == 0 {
			return
		}
		if time.Now().After(deadline) {
			t.Fatal("the sources' secrets are still being changed")
		}
		time.Sleep(5 * time.Millisecond)
	}
}

// An edit whose dialog closes with the settings before its save finished leaves the password stored as it was, as settings.json still describes the source as it was.
func TestMediaSourceAbandonedEditKeepsThePassword(t *testing.T) {
	a, tt, updates := newTestApp(t, &fakePipeline{})
	s := storedMediaSource(t, a, "old")
	tt = settingsOpenAt(a, tt, settings.SectionMediaSources)
	tt.Press("id:media-source-edit-" + s.ID)
	tt.Type("id:media-source-field-password", "new")
	tt.Press("id:media-source-dialog-save")
	if !a.MediaSourceSaving() {
		t.Fatal("not keeping the secret")
	}
	a.OpenSettings(settings.SectionGeneral)
	refresh(tt, a)
	mediaSourceSecretsSettle(t, tt, a, updates, func() bool { return mediaSourceStored(t, a, s.ID) == "new" })
	if got := mediaSourceStored(t, a, s.ID); got != "old" || a.MediaSourceEditing() {
		t.Fatalf("stored %q, editing %v", got, a.MediaSourceEditing())
	}
}

// An edit that settings.json cannot take leaves the password stored as it was and says why; once settings.json can be written, Save keeps the new one.
func TestMediaSourceEditNotSavedKeepsThePassword(t *testing.T) {
	a, tt, updates := newTestApp(t, &fakePipeline{})
	s := storedMediaSource(t, a, "old")
	path := a.settingsPath
	tt = settingsOpenAt(a, tt, settings.SectionMediaSources)
	tt.Press("id:media-source-edit-" + s.ID)
	tt.Type("id:media-source-field-password", "new")
	unwritableSettings(t, a)
	tt.Press("id:media-source-dialog-save")
	mediaSourceSecretsSettle(t, tt, a, updates, func() bool { return mediaSourceStored(t, a, s.ID) == "new" })
	if got := mediaSourceStored(t, a, s.ID); got != "old" || !a.MediaSourceEditing() || a.MediaSourceProblem() != testlocale.Messages().SettingsSourcesSaveFailed() {
		t.Fatalf("stored %q, editing %v, problem %q", got, a.MediaSourceEditing(), a.MediaSourceProblem())
	}
	a.settingsPath = path
	tt.Press("id:media-source-dialog-save")
	mediaSourceSecretsSettle(t, tt, a, updates, func() bool { return mediaSourceStored(t, a, s.ID) == "new" })
	if got := mediaSourceStored(t, a, s.ID); got != "new" || a.MediaSourceEditing() {
		t.Fatalf("saved, but stored %q, editing %v", got, a.MediaSourceEditing())
	}
}

// A new source that settings.json cannot take, or whose dialog closes before its save finished, leaves no password in the secure store.
func TestMediaSourceNewSourceNotSavedLeavesNoSecret(t *testing.T) {
	a, tt, updates := newTestApp(t, &fakePipeline{})
	unwritableSettings(t, a)
	tt = settingsOpenAt(a, tt, settings.SectionMediaSources)
	tt.Press("id:media-source-add")
	tt.Type("id:media-source-field-host", "nas.local")
	tt.Type("id:media-source-field-user", "kando")
	tt.Type("id:media-source-field-password", "pa55")
	tt.Press("id:media-source-dialog-save")
	mediaSourceSecretsSettle(t, tt, a, updates, func() bool { return len(tt.Secrets.Items()) > 0 })
	if n := len(tt.Secrets.Items()); n != 0 || len(a.sources.list) != 0 || a.MediaSourceProblem() != testlocale.Messages().SettingsSourcesSaveFailed() {
		t.Fatalf("%d secrets kept, list %+v, problem %q", n, a.sources.list, a.MediaSourceProblem())
	}

	tt.Press("id:media-source-dialog-save")
	a.OpenSettings(settings.SectionGeneral)
	refresh(tt, a)
	mediaSourceSecretsSettle(t, tt, a, updates, func() bool { return len(tt.Secrets.Items()) > 0 })
	if n := len(tt.Secrets.Items()); n != 0 || len(a.sources.list) != 0 {
		t.Fatalf("%d secrets kept, list %+v", n, a.sources.list)
	}
}

// A removal that settings.json cannot take keeps the source and its password, and says why.
func TestMediaSourceRemoveNotSavedKeepsThePassword(t *testing.T) {
	a, tt, updates := newTestApp(t, &fakePipeline{})
	s := storedMediaSource(t, a, "old")
	tt = settingsOpenAt(a, tt, settings.SectionMediaSources)
	unwritableSettings(t, a)
	tt.Press("id:media-source-remove-" + s.ID)
	tt.Press("id:media-source-remove-ok")
	mediaSourceSecretsSettle(t, tt, a, updates, func() bool { return true })
	if got := mediaSourceStored(t, a, s.ID); got != "old" || len(a.sources.list) != 1 || a.SaveErr() != testlocale.Messages().SettingsSourcesSaveFailed() {
		t.Fatalf("stored %q, list %+v, error %q", got, a.sources.list, a.SaveErr())
	}
}
