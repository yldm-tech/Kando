package main

import (
	"context"
	"errors"
	"fmt"
	"io"
	"maps"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path"
	"path/filepath"
	"slices"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"

	"kando/apps/kando/account"
	"kando/apps/kando/feedback"
	"kando/apps/kando/films"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/mediasource"
	"kando/apps/kando/saved"
	"kando/apps/kando/settings"
	"kando/apps/kando/shelf"
	"kando/packages/core"
	"kando/packages/media"
	"kando/packages/remotefs"
	"kando/packages/remotefs/davtest"
	"kando/packages/remotefs/sftptest"

	"github.com/cloudsoda/go-smb2"
	ns "github.com/nsgo-dev/nsgo/core"
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
	if !tt.Exists("id:source-" + pageMediaSources) {
		t.Fatal("the row does not show before a source was added")
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

// With no source added, the 媒体来源 page says what one is and adds one: 添加来源… opens 设置 › 媒体来源 with the dialog adding a source, and once it is saved the settings close and the page shows the source at its root. A source added in the settings themselves keeps them open; its row's 浏览 closes them and shows it on the page.
func TestMediaSourceAddedFromItsPageShowsThere(t *testing.T) {
	a, tt, updates := newTestApp(t, &fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	src := newFakeSource(map[string]fakeSourceFile{"media/Heat.mkv": {data: "heat"}, "backup/old.mp4": {data: "old"}})
	a.sources.open = src.open
	msgs := testlocale.Messages()
	tt.Press("id:source-" + pageMediaSources)
	if a.page != pageMediaSources || !tt.Exists("id:media-sources-none") {
		t.Fatalf("page %q; texts %q", a.page, tt.Texts())
	}
	tt.Press("id:media-sources-add")
	refresh(tt, a)
	if !a.prefs.open || a.SettingsSection() != settings.SectionMediaSources || !a.sources.editing || !a.MediaSourceIsNew() || !tt.Exists("id:media-source-dialog") {
		t.Fatalf("settings open %v at %q, dialog %v", a.prefs.open, a.SettingsSection(), a.sources.editing)
	}
	tt.Type("id:media-source-field-host", "nas.local")
	tt.Type("id:media-source-field-share", "media")
	tt.Type("id:media-source-field-user", "kando")
	tt.Type("id:media-source-field-password", "pa55-w0rd!")
	tt.Press("id:media-source-dialog-save")
	waitUntil(t, tt, a, updates, func() bool { return !a.sources.editing })
	if len(a.sources.list) != 1 || a.prefs.open || a.modalOpen() {
		t.Fatalf("list %+v, settings open %v, modal %v", a.sources.list, a.prefs.open, a.modalOpen())
	}
	first := a.sources.list[0].ID
	waitUntil(t, tt, a, updates, func() bool { return !a.sources.browse.loading && a.sources.browse.source == first })
	if a.page != pageMediaSources || a.sources.browse.dir != "" || !tt.Exists("id:media-sources-entry-media") || !tt.Exists("id:media-sources-add-more") {
		t.Fatalf("page %q dir %q; texts %q", a.page, a.sources.browse.dir, tt.Texts())
	}

	// A second source, added in the settings, leaves them open on its row.
	onEngine(a, func() { a.Select(pageFilms) })
	tt = settingsOpenAt(a, tt, settings.SectionMediaSources)
	tt.Press("id:media-source-add")
	tt.Type("id:media-source-field-host", "studio.lan")
	tt.Type("id:media-source-field-share", "backup")
	tt.Type("id:media-source-field-user", "kando")
	tt.Type("id:media-source-field-password", "pa55-w0rd!")
	tt.Press("id:media-source-dialog-save")
	waitUntil(t, tt, a, updates, func() bool { return !a.sources.editing })
	if len(a.sources.list) != 2 || !a.prefs.open || a.page != pageFilms {
		t.Fatalf("list %d, settings open %v, page %q", len(a.sources.list), a.prefs.open, a.page)
	}
	second := a.sources.list[1].ID
	wantText(t, tt, msgs.SettingsSourcesBrowse())
	tt.Press("id:media-source-browse-" + second)
	waitUntil(t, tt, a, updates, func() bool { return !a.sources.browse.loading && a.sources.browse.source == second })
	if a.prefs.open || a.page != pageMediaSources || !tt.Exists("id:media-sources-entry-backup") {
		t.Fatalf("settings open %v, page %q; texts %q", a.prefs.open, a.page, tt.Texts())
	}
	// On the page already, 浏览 shows the other source.
	tt = settingsOpenAt(a, tt, settings.SectionMediaSources)
	tt.Press("id:media-source-browse-" + first)
	waitUntil(t, tt, a, updates, func() bool { return !a.sources.browse.loading && a.sources.browse.source == first })
	if a.prefs.open || !tt.Exists("id:media-sources-entry-media") {
		t.Fatalf("settings open %v; texts %q", a.prefs.open, tt.Texts())
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

// An SFTP source connects only to a server whose key it trusts: the first Test asks whether to trust the key the server offers, which Cancel leaves untrusted and Trust keeps in the draft, saved with it in settings.json while its password goes to the secure store; a key that changed afterwards is replaced only by the destructive Replace Key, which saves the source at once.
func TestMediaSourceSFTPTrustsTheServersKey(t *testing.T) {
	a, tt, updates := newTestApp(t, &fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	msgs := testlocale.Messages()
	server := sftptest.New(t)
	if err := os.Mkdir(filepath.Join(server.Home, "films"), 0o755); err != nil {
		t.Fatal(err)
	}
	host, port := server.HostPort()
	tt = settingsOpenAt(a, tt, settings.SectionMediaSources)
	tt.Press("id:media-source-add")
	onEngine(a, func() { a.SetMediaSourceKind(mediasource.KindSFTP) })
	refresh(tt, a)
	wantText(t, tt, msgs.AccountLocationHostKeyNone())
	wantNoText(t, tt, msgs.SettingsSourcesSMBWarning())
	tt.Type("id:media-source-field-host", host)
	tt.Type("id:media-source-field-port", strconv.Itoa(port))
	tt.Type("id:media-source-field-root", "films")
	tt.Type("id:media-source-field-user", sftptest.User)
	tt.Press("id:media-source-dialog-save")
	wantText(t, tt, msgs.AccountLocationNeedsSecret())
	tt.Type("id:media-source-field-password", sftptest.Password)

	ask := func(target, test string) {
		t.Helper()
		tt.Press(test)
		waitUntil(t, tt, a, updates, func() bool { return a.sources.alert.kind != account.AlertNone || !a.sources.tests[target].busy })
		refresh(tt, a)
	}
	ask("", "id:media-source-dialog-test")
	if !tt.Exists("id:media-source-trust") || a.sources.alert.target != "" || a.MediaSourceAlert().Fingerprint != remotefs.Fingerprint(server.HostKey) {
		t.Fatalf("no question about the key: %+v", a.MediaSourceAlert())
	}
	tt.Press("id:media-source-alert-cancel")
	wantText(t, tt, msgs.AccountLocationNotTrusted())
	if a.MediaSourceDraft().Values[mediasource.SFTPHostKey] != "" || !a.modalOpen() {
		t.Fatal("Cancel trusted the key, or closed the dialog")
	}
	ask("", "id:media-source-dialog-test")
	tt.Press("id:media-source-alert-trust")
	waitUntil(t, tt, a, updates, func() bool { return !a.sources.tests[""].busy })
	refresh(tt, a)
	wantText(t, tt, msgs.AccountLocationConnected())
	wantText(t, tt, remotefs.Fingerprint(server.HostKey))
	if a.MediaSourceDraft().Values[mediasource.SFTPHostKey] != server.HostKey {
		t.Fatalf("the draft trusts %q", a.MediaSourceDraft().Values[mediasource.SFTPHostKey])
	}

	tt.Press("id:media-source-dialog-save")
	waitUntil(t, tt, a, updates, func() bool { return !a.sources.editing })
	if len(a.sources.list) != 1 || a.modalOpen() {
		t.Fatalf("list %+v, modal %v", a.sources.list, a.modalOpen())
	}
	s := a.sources.list[0]
	if s.Kind != mediasource.KindSFTP || s.Name != host || s.Config[mediasource.SFTPHostKey] != server.HostKey || s.Config[mediasource.SFTPPassword] != "" {
		t.Fatalf("saved %+v", s)
	}
	wantText(t, tt, msgs.AccountLocationSummary("SFTP", host+"/films"))
	raw, err := os.ReadFile(a.settingsPath)
	if err != nil || strings.Contains(string(raw), sftptest.Password) || !strings.Contains(string(raw), `"hostKey"`) {
		t.Fatalf("settings.json: %s, %v", raw, err)
	}
	if v := mediaSourceStored(t, a, s.ID); v != sftptest.Password {
		t.Fatalf("secure store %q", v)
	}

	// The server's key changed: Test warns, and Replace Key trusts the new one and saves it at once.
	_, other := sftptest.NewKey(t)
	onEngine(a, func() { a.sources.list[0].Config[mediasource.SFTPHostKey] = sftptest.AuthorizedKey(other) })
	ask(s.ID, "id:media-source-test-"+s.ID)
	if !tt.Exists("id:media-source-key-changed") || a.MediaSourceAlert().Trusted != remotefs.Fingerprint(sftptest.AuthorizedKey(other)) {
		t.Fatalf("no warning that the key changed: %+v", a.MediaSourceAlert())
	}
	tt.Press("id:media-source-alert-replace")
	waitUntil(t, tt, a, updates, func() bool { return !a.sources.tests[s.ID].busy })
	refresh(tt, a)
	wantText(t, tt, msgs.AccountLocationConnected())
	if got := loadSavedSettings(a.settingsPath).MediaSources; len(got) != 1 || got[0].Config[mediasource.SFTPHostKey] != server.HostKey {
		t.Fatalf("saved %+v", got)
	}
	if a.modalOpen() {
		t.Fatal("the question still holds the modal flag")
	}
}

// The question about a server's key goes with the settings, and gives back its modal flag.
func TestMediaSourceKeyQuestionClosesWithTheSettings(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	tt = settingsOpenAt(a, tt, settings.SectionMediaSources)
	tt.Press("id:media-source-add")
	onEngine(a, func() {
		a.mediaSourceAsk(mediaSourceAlert{kind: account.AlertTrust, hk: &remotefs.HostKeyError{Host: "nas", Key: "k", Fingerprint: "SHA256:x"}})
	})
	refresh(tt, a)
	if !tt.Exists("id:media-source-trust") || !a.modalOpen() {
		t.Fatal("no question")
	}
	onEngine(a, func() { a.prefs.open = false })
	refresh(tt, a)
	if a.sources.alert.kind != account.AlertNone || a.modalOpen() {
		t.Fatalf("the question stayed: %+v, modal %v", a.sources.alert, a.modalOpen())
	}
}

// A WebDAV source needs a plain https:// or http:// address, says that an http:// one sends its password unencrypted, connects with Test, and is saved named after its server, with its password in the secure store rather than settings.json.
func TestMediaSourceWebDAV(t *testing.T) {
	a, tt, updates := newTestApp(t, &fakePipeline{})
	a.settingsPath = filepath.Join(t.TempDir(), "settings.json")
	msgs := testlocale.Messages()
	server := davtest.New(t)
	if err := os.Mkdir(filepath.Join(server.Dir, "films"), 0o755); err != nil {
		t.Fatal(err)
	}
	tt = settingsOpenAt(a, tt, settings.SectionMediaSources)
	tt.Press("id:media-source-add")
	onEngine(a, func() { a.SetMediaSourceKind(mediasource.KindWebDAV) })
	refresh(tt, a)
	wantNoText(t, tt, msgs.SettingsSourcesSMBWarning())
	tt.Type("id:media-source-field-url", "ftp://nas.local/dav")
	tt.Press("id:media-source-dialog-save")
	wantText(t, tt, msgs.AccountLocationNeedsURL())
	tt.Clear("id:media-source-field-url")
	tt.Type("id:media-source-field-url", server.URL)
	wantText(t, tt, msgs.SettingsSourcesPlainHTTP())
	tt.Type("id:media-source-field-root", "films")
	tt.Type("id:media-source-field-user", davtest.User)
	tt.Type("id:media-source-field-password", davtest.Password)
	tt.Press("id:media-source-dialog-test")
	waitUntil(t, tt, a, updates, func() bool { return !a.sources.tests[""].busy })
	wantText(t, tt, msgs.AccountLocationConnected())

	tt.Press("id:media-source-dialog-save")
	waitUntil(t, tt, a, updates, func() bool { return !a.sources.editing })
	if len(a.sources.list) != 1 {
		t.Fatalf("list %+v", a.sources.list)
	}
	s := a.sources.list[0]
	if s.Kind != mediasource.KindWebDAV || s.Name != "127.0.0.1" || s.Config[mediasource.WebDAVURL] != server.URL || s.Config[mediasource.WebDAVPassword] != "" {
		t.Fatalf("saved %+v", s)
	}
	raw, err := os.ReadFile(a.settingsPath)
	if err != nil || strings.Contains(string(raw), davtest.Password) {
		t.Fatalf("settings.json: %s, %v", raw, err)
	}
	if v := mediaSourceStored(t, a, s.ID); v != davtest.Password {
		t.Fatalf("secure store %q", v)
	}
	u, _ := url.Parse(server.URL)
	wantText(t, tt, msgs.AccountLocationSummary("WebDAV", u.Host+u.Path+"/films"))

	// A wrong password says so.
	onEngine(a, func() {
		a.EditMediaSource(s.ID)
		a.MediaSourceDraft().Values[mediasource.WebDAVPassword] = "wrong"
	})
	refresh(tt, a)
	tt.Press("id:media-source-dialog-test")
	waitUntil(t, tt, a, updates, func() bool { return !a.sources.tests[""].busy })
	wantText(t, tt, msgs.AccountLocationFailSignIn())
}

// A video imported to be indexed rather than copied gets a record in library.json under a placeholder in the library's folder, where no video is, and shows on the shelf marked as playing from its source; the way chosen is remembered when asked to. It plays from the source through the loopback server; a file changed there plays all the same, the bar saying so until it is taken as it is; a source that cannot be reached, a file no longer at its path, which can be found again on the source, and a source removed say so, its lines still read. Taking it off the shelf forgets its record and nothing on the source.
// streamingPipeline is fakePipeline that reads an indexed film's video where it is, through the app's copier (media.SourceStreamer), as media.Tools does through Tools.Sources; heard is what it was asked to hear from.
type streamingPipeline struct {
	fakePipeline
	copier media.SourceReader
	mu     sync.Mutex
	heard  []string
}

func (p *streamingPipeline) StreamSource(ctx context.Context, source string) (string, error) {
	return p.copier.URL(ctx, source)
}

func (p *streamingPipeline) Hear(ctx context.Context, path string, from, to time.Duration, heard func(core.Segment)) (string, error) {
	p.mu.Lock()
	p.heard = append(p.heard, path)
	p.mu.Unlock()
	return p.fakePipeline.Hear(ctx, path, from, to, heard)
}

// A video imported to be indexed rather than copied gets a record in library.json under a placeholder in the library's folder, where no video is, and shows on the shelf marked as playing from its source, its lines made by a job that hears it from the source through the loopback server and writes them beside the placeholder; the way chosen is remembered when asked to. It plays from the source through the loopback server; a file changed there plays all the same, the bar saying so until it is taken as it is; a source that cannot be reached, a file no longer at its path, which can be found again on the source, and a source removed say so, its lines still read. Taking it off the shelf forgets its record and nothing on the source.
func TestMediaSourceIndexAndPlay(t *testing.T) {
	p := &streamingPipeline{}
	a, tt, updates := newTestApp(t, p)
	msgs := testlocale.Messages()
	heat := "Movies/Heat (1995).mkv"
	s, src := browseSource(a, map[string]fakeSourceFile{
		heat:               {data: "heat video"},
		"Movies/Ronin.mp4": {data: "ronin"},
	})
	p.copier = a.mediaSourceCopier()
	a.mediaSourceOpenPage()
	refresh(tt, a)
	waitUntil(t, tt, a, updates, func() bool { return !a.sources.browse.loading })
	a.mediaSourceOpenDir("Movies")
	waitUntil(t, tt, a, updates, func() bool { return !a.sources.browse.loading })
	tt.Press("id:media-sources-choose-" + heat)
	tt.Press("id:media-sources-import")
	if a.MediaBrowseImport().Link {
		t.Fatal("the dialog indexes before it was asked to")
	}
	tt.Press("id:media-sources-import-link")
	wantText(t, tt, msgs.MediaSourceImportLinkNote("Living Room NAS"))
	tt.Press("id:media-sources-import-remember")
	tt.Press("id:media-sources-import-start")
	waitUntil(t, tt, a, updates, func() bool { return len(a.library.Linked) == 1 })
	if !a.sources.rememberImport || !a.sources.importLink {
		t.Fatalf("the way chosen is not remembered: %v %v", a.sources.rememberImport, a.sources.importLink)
	}
	placeholder := filepath.Join(a.snap().LibraryDir, "Living Room NAS", "Movies", "Heat (1995).mkv")
	l, ok := a.library.Linked[placeholder]
	if !ok || l.Source != s.ID || l.Path != heat || l.Size != int64(len("heat video")) || l.Version != "v1" {
		t.Fatalf("records %+v", a.library.Linked)
	}
	if _, err := os.Stat(placeholder); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a file at the placeholder: %v", err)
	}
	if info, err := os.Stat(filepath.Dir(placeholder)); err != nil || !info.IsDir() {
		t.Fatalf("no folder for the placeholder: %v", err)
	}
	refresh(tt, a)
	wantText(t, tt, msgs.MediaSourceIndexed(1))
	waitUntil(t, tt, a, updates, func() bool {
		return len(a.library.Jobs) == 1 && a.library.Jobs[0].Stage.Done() && !a.running()
	})
	job := a.library.Jobs[0]
	if job.Stage != core.StageComplete || !job.Linked || job.VideoPath != placeholder || job.Source != (mediasource.Ref{Source: s.ID, Path: heat}).String() {
		t.Fatalf("the job %+v", job)
	}
	if filepath.Dir(job.SubtitlePath) != filepath.Dir(placeholder) || !fileExists(job.SubtitlePath) {
		t.Fatalf("subtitles at %q", job.SubtitlePath)
	}
	if _, err := os.Stat(placeholder); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("the job copied the video: %v", err)
	}
	p.mu.Lock()
	heardFrom := slices.Clone(p.heard)
	p.mu.Unlock()
	if len(heardFrom) == 0 || !strings.HasPrefix(heardFrom[0], "http://127.0.0.1:") {
		t.Fatalf("heard from %q", heardFrom)
	}
	if !tt.Exists("id:media-sources-in-library-" + heat) {
		t.Fatalf("the video indexed is not marked: %q", tt.Texts())
	}
	// The next import starts on the way remembered.
	tt.Press("id:media-sources-choose-Movies/Ronin.mp4")
	tt.Press("id:media-sources-import")
	if !a.MediaBrowseImport().Link || !a.MediaBrowseImport().Remember {
		t.Fatalf("the next import starts as %+v", a.MediaBrowseImport())
	}
	tt.Press("id:media-sources-import-cancel")

	item, ok := films.Find(a.films(), job.ID)
	if !ok || item.Linked != s.ID {
		t.Fatalf("the film on the shelf: %+v", item)
	}
	if place, gone := a.LinkedMark(msgs, item); place != "Living Room NAS" || gone {
		t.Fatalf("mark %q %v", place, gone)
	}

	play := func() {
		t.Helper()
		a.closeFilm()
		a.page = pageFilms
		a.Open(item)
		waitUntil(t, tt, a, updates, func() bool {
			return a.player.playErr != "" || (a.player.play != nil && a.player.play.source != "")
		})
	}
	play()
	if a.player.playErr != "" || !strings.HasPrefix(a.player.play.source, "http://127.0.0.1:") || a.player.linkedChanged != nil {
		t.Fatalf("plays %q, error %q", a.player.play.source, a.player.playErr)
	}
	wantRange(t, a.player.play.source, "bytes=5-", "video")
	url := a.player.play.source

	// A file that changed on the source plays all the same, the bar naming the source until 知道了.
	src.mu.Lock()
	src.files[heat] = fakeSourceFile{data: "heat video, cut anew"}
	src.mu.Unlock()
	play()
	if a.player.playErr != "" || a.LinkedChanged() != "Living Room NAS" {
		t.Fatalf("a changed file: error %q, bar %q", a.player.playErr, a.LinkedChanged())
	}
	if resp, err := http.Get(url); err == nil {
		resp.Body.Close()
		if resp.StatusCode != http.StatusNotFound {
			t.Fatalf("the address of the film played before still serves %d", resp.StatusCode)
		}
	}
	a.KeepLinkedChanged()
	if a.LinkedChanged() != "" || a.library.Linked[placeholder].Size != int64(len("heat video, cut anew")) {
		t.Fatalf("知道了 kept %+v", a.library.Linked[placeholder])
	}

	// A source that cannot be reached: 重试 plays it once it can.
	src.mu.Lock()
	src.unreachable = errors.New("dial tcp: connection refused")
	src.mu.Unlock()
	a.sources.pool.Forget(s.ID)
	play()
	if a.player.playErr != playerErrLinkedUnreachable {
		t.Fatalf("an unreachable source: error %q", a.player.playErr)
	}
	refresh(tt, a)
	wantText(t, tt, msgs.MediaSourceUnreachable("Living Room NAS"))
	src.mu.Lock()
	src.unreachable = nil
	src.mu.Unlock()
	tt.Press("id:film-stage-linked-retry")
	waitUntil(t, tt, a, updates, func() bool { return a.player.play != nil && a.player.play.source != "" })

	// A file no longer at its path: 在来源里找 shows its folder, and the video chosen there is where the film plays from.
	src.mu.Lock()
	delete(src.files, heat)
	src.files["Movies/Heat.mkv"] = fakeSourceFile{data: "heat video, cut anew"}
	src.mu.Unlock()
	play()
	if a.player.playErr != playerErrLinkedMissing {
		t.Fatalf("a file gone: error %q", a.player.playErr)
	}
	refresh(tt, a)
	tt.Press("id:film-stage-linked-find")
	waitUntil(t, tt, a, updates, func() bool { return a.page == pageMediaSources && !a.sources.browse.loading })
	if a.sources.browse.dir != "Movies" || a.sources.browse.relink != placeholder {
		t.Fatalf("finding opened %q relinking %q", a.sources.browse.dir, a.sources.browse.relink)
	}
	tt.Press("id:media-sources-choose-Movies/Heat.mkv")
	if a.library.Linked[placeholder].Path != "Movies/Heat.mkv" || a.sources.browse.relink != "" {
		t.Fatalf("after finding it: %+v", a.library.Linked[placeholder])
	}
	refresh(tt, a)
	wantText(t, tt, msgs.MediaSourceRelinked(films.Name(item)))
	play()
	if a.player.playErr != "" {
		t.Fatalf("the film found again: error %q", a.player.playErr)
	}

	// The source removed: its address is kept while the film is indexed from it, and adding it back gives the film its source again.
	a.closeFilm()
	a.askRemoveMediaSource(s.ID)
	a.answerRemoveMediaSource(true)
	if len(a.sources.list) != 0 || len(a.sources.removed) != 1 || a.sources.removed[0].ID != s.ID {
		t.Fatalf("list %+v removed %+v", a.sources.list, a.sources.removed)
	}
	play()
	if a.player.playErr != playerErrLinkedGone {
		t.Fatalf("a source removed: error %q", a.player.playErr)
	}
	if _, gone := a.LinkedMark(msgs, item); !gone {
		t.Fatal("the mark of a film whose source was removed")
	}
	back := mediasource.Source{Kind: s.Kind, Config: map[string]string{mediasource.SMBHost: "NAS.local", mediasource.SMBShare: "media/", mediasource.SMBUser: "other"}}
	if id := a.mediaSourceRelinkID(back); id != s.ID || len(a.sources.removed) != 0 {
		t.Fatalf("added back as %q, removed %+v", id, a.sources.removed)
	}

	// Off the shelf: the record goes, and nothing on the source was written.
	a.closeFilm()
	a.libraryForget(a.snap().LibraryDir, []films.Film{item})
	if len(a.library.Linked) != 0 || len(a.library.Jobs) != 0 {
		t.Fatalf("a removed film's record is kept: %v %+v", a.library.Linked, a.library.Jobs)
	}
	src.mu.Lock()
	defer src.mu.Unlock()
	if _, ok := src.files["Movies/Heat.mkv"]; !ok {
		t.Fatal("the video on the source went")
	}
}

// Settings keeps the sources removed while films are indexed from them, and the way of importing remembered, and drops a way it does not know.
func TestSavedMediaSourcesKeepRemovedAndTheWay(t *testing.T) {
	s := savedMediaSources{
		MediaSources:        []savedMediaSource{{ID: "0123456789abcdef", Kind: mediasource.KindSMB, Name: "NAS"}},
		RemovedMediaSources: []savedMediaSource{{ID: "0123456789abcdef", Kind: mediasource.KindSMB, Name: "Again"}, {ID: "fedcba9876543210", Kind: mediasource.KindSMB, Name: "Old", Config: map[string]string{mediasource.SMBHost: "nas", mediasource.SMBPassword: "secret"}}},
		MediaSourceImport:   "move",
	}
	s.clean()
	if len(s.RemovedMediaSources) != 1 || s.RemovedMediaSources[0].Name != "Old" || s.RemovedMediaSources[0].Config[mediasource.SMBPassword] != "" || s.MediaSourceImport != "" {
		t.Fatalf("cleaned %+v", s)
	}
	s.MediaSourceImport = mediaSourceImportLink
	s.clean()
	if s.MediaSourceImport != mediaSourceImportLink {
		t.Fatalf("the way %q", s.MediaSourceImport)
	}
}

// Phone Sync serves an indexed film's video from its source, with the size it had as it was indexed, and one whose source is gone by its lines alone.
func TestPhoneSyncServesAnIndexedVideo(t *testing.T) {
	a, _, _ := newTestApp(t, &fakePipeline{})
	s, _ := browseSource(a, map[string]fakeSourceFile{"talk.mp4": {data: "indexed video"}})
	a.mediaSourcePoolSync()
	added := time.Date(2026, 11, 24, 9, 0, 0, 0, time.UTC)
	path := filepath.Join(a.snap().LibraryDir, "Living Room NAS", "talk", "video.mp4")
	item := films.Film{Key: path, VideoPath: path, Linked: s.ID}
	ps := phoneShelf{linked: map[string]shelf.Linked{path: {Source: s.ID, Path: "talk.mp4", Size: 13, Added: added}}, sources: a.mediaSourceIDs(), sourcePool: a.sources.pool}
	got := phoneFilmOf(item, ps)
	if got.Remote == nil || got.RemoteSize != 13 || !got.RemoteAt.Equal(added) || got.SubtitlesOnly {
		t.Fatalf("the indexed film is served as %+v", got)
	}
	// A file taken anew on the source (知道了, 在来源里找) is a change a phone sees, by the time the source gave of it.
	modified := added.Add(time.Hour)
	l := ps.linked[path]
	l.Modified = modified
	ps.linked[path] = l
	if got := phoneFilmOf(item, ps); !got.RemoteAt.Equal(modified) {
		t.Fatalf("a file changed on the source is served as of %v", got.RemoteAt)
	}
	req, _ := http.NewRequest(http.MethodGet, "/video", nil)
	req.Header.Set("Range", "bytes=8-")
	rec := httptest.NewRecorder()
	got.Remote.ServeHTTP(rec, req)
	if rec.Code != http.StatusPartialContent || rec.Body.String() != "video" {
		t.Fatalf("the source serves %d %q", rec.Code, rec.Body.String())
	}
	ps.sources = nil
	if got := phoneFilmOf(item, ps); got.Remote != nil || !got.SubtitlesOnly {
		t.Fatalf("a film whose source is gone is served as %+v", got)
	}
}

// Phone Sync names the file of a NAS a film was made of by its source key (psync.Film.SourceKey), the one a phone that reaches the same NAS computes for it: an indexed film by the size the source gave, a copied one by its video's size here, and neither a film of a plugin's source, of a source no longer in the settings, nor one of the web.
func TestPhoneSyncNamesTheSourceFile(t *testing.T) {
	dir := t.TempDir()
	nas := mediasource.Source{ID: "0123456789abcdef", Kind: mediasource.KindSMB, Name: "NAS", Config: map[string]string{mediasource.SMBHost: "nas.local", mediasource.SMBShare: "video", mediasource.SMBRoot: "Films", mediasource.SMBUser: "anna"}}
	plugin := mediasource.Source{ID: "fedcba9876543210", Kind: "kando.webdav", Name: "DAV", Config: map[string]string{"url": "https://nas.local/dav"}}
	indexed := filepath.Join(dir, "NAS", "River", "video.mp4")
	copied := filepath.Join(dir, "NAS", "Lake.mp4")
	if err := os.MkdirAll(filepath.Dir(copied), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(copied, []byte("seven b"), 0o644); err != nil {
		t.Fatal(err)
	}
	ps := phoneShelf{linked: map[string]shelf.Linked{indexed: {Source: nas.ID, Path: "River.mp4", Size: 1 << 20}}, sources: map[string]bool{nas.ID: true, plugin.ID: true}, sourceList: []mediasource.Source{nas, plugin}}
	// smb ‖ nas.local ‖ 445 ‖ video ‖ Films/River.mp4 ‖ 1048576, the vector of remotefs's TestSourceKeyVectors.
	if got := phoneFilmOf(films.Film{Key: indexed, VideoPath: indexed, Linked: nas.ID, Source: mediasource.Ref{Source: nas.ID, Path: "River.mp4"}.String()}, ps); got.SourceKey != "6aTKFGnkWxhnQ8vrl6Wj-G" {
		t.Errorf("the indexed film has the key %q", got.SourceKey)
	}
	lake := films.Film{Key: "job1", JobID: "job1", VideoPath: copied, Source: mediasource.Ref{Source: nas.ID, Path: "Lake.mp4"}.String()}
	if got, want := phoneFilmOf(lake, ps).SourceKey, remotefs.SourceKey(remotefs.Remote{Kind: remotefs.KindSMB, Host: "nas.local", Share: "video", Root: "Films"}, "Lake.mp4", 7); got != want || got == "" {
		t.Errorf("the copied film has the key %q, want %q", got, want)
	}
	for name, item := range map[string]films.Film{
		"of a plugin's source":   {Key: "job2", JobID: "job2", VideoPath: copied, Source: mediasource.Ref{Source: plugin.ID, Path: "Lake.mp4"}.String()},
		"of a source gone":       {Key: "job3", JobID: "job3", VideoPath: copied, Source: mediasource.Ref{Source: "1111111111111111", Path: "Lake.mp4"}.String()},
		"of the web":             {Key: "job4", JobID: "job4", VideoPath: copied, Source: "https://www.youtube.com/watch?v=dQw4w9WgXcQ"},
		"whose video is no more": {Key: "job5", JobID: "job5", VideoPath: filepath.Join(dir, "gone.mp4"), Source: mediasource.Ref{Source: nas.ID, Path: "gone.mp4"}.String()},
	} {
		if got := phoneFilmOf(item, ps).SourceKey; got != "" {
			t.Errorf("a film %s has the key %q", name, got)
		}
	}
}

// Two films indexed from the same folder of a source are said to be in the same folder of the library, where no video of either is: taking one off the shelf sends only its own files to the Trash, never the folder with the other's subtitles, and asks to remove its files rather than its folder.
func TestTrashOneOfTwoIndexedFilmsKeepsTheOther(t *testing.T) {
	a, tt, updates := newTestApp(t, &fakePipeline{})
	_, _ = browseSource(a, map[string]fakeSourceFile{"Movies/Heat.mkv": {data: "heat"}, "Movies/Ronin.mkv": {data: "ronin"}})
	a.mediaSourceOpenPage()
	refresh(tt, a)
	waitUntil(t, tt, a, updates, func() bool { return !a.sources.browse.loading })
	a.queue.stopped = true
	a.mediaSourceLink([]string{"Movies/Heat.mkv", "Movies/Ronin.mkv"})
	waitUntil(t, tt, a, updates, func() bool { return len(a.library.Linked) == 2 })
	dir := filepath.Join(a.snap().LibraryDir, "Living Room NAS", "Movies")
	heat, ronin := filepath.Join(dir, "Heat.mkv"), filepath.Join(dir, "Ronin.mkv")
	heatSub, roninSub := filepath.Join(dir, "Heat.bilingual.srt"), filepath.Join(dir, "Ronin.bilingual.srt")
	writeFiles(t, map[string]string{heatSub: "1\n", roninSub: "1\n"})
	a.refreshLibrary()
	var trashed []string
	prev := trashItem
	trashItem = func(_ *ns.Scope, path string, done func(error)) {
		trashed = append(trashed, path)
		done(os.RemoveAll(path))
	}
	t.Cleanup(func() { trashItem = prev })
	item := filmOfVideo(t, a, heat)
	a.AskRemove(item)
	if _, _, folder := a.Removing(); folder {
		t.Fatal("the confirmation says the folder goes, the other film's subtitles in it")
	}
	a.libraryCloseRemove()
	a.TrashFilms(item)
	if slices.Contains(trashed, dir) {
		t.Fatalf("the shared folder went to the Trash: %q", trashed)
	}
	if !fileExists(roninSub) {
		t.Fatalf("the other film's subtitles went: %q", trashed)
	}
	if fileExists(heatSub) {
		t.Fatalf("the film's own subtitles stayed: %q", trashed)
	}
	if _, ok := a.library.Linked[ronin]; !ok || len(a.library.Linked) != 1 {
		t.Fatalf("records %+v", a.library.Linked)
	}
}

// Offline mode keeps a source on the internet closed to an indexed film: the page says offline mode is why and offers the privacy settings, rather than saying the source cannot be reached; Phone Sync does not read it for a phone either, while a source on the local network plays and serves as before.
func TestIndexedFilmOffline(t *testing.T) {
	a, tt, updates := newTestApp(t, &fakePipeline{})
	msgs := testlocale.Messages()
	s, _ := browseSource(a, map[string]fakeSourceFile{"talk.mp4": {data: "indexed video"}})
	a.mediaSourceOpenPage()
	refresh(tt, a)
	waitUntil(t, tt, a, updates, func() bool { return !a.sources.browse.loading })
	a.queue.stopped = true
	a.mediaSourceLink([]string{"talk.mp4"})
	waitUntil(t, tt, a, updates, func() bool { return len(a.library.Linked) == 1 })
	a.refreshLibrary()
	path := filepath.Join(a.snap().LibraryDir, "Living Room NAS", "talk", "video.mp4")
	item := filmOfVideo(t, a, path)
	apply(tt, a, func() { a.privacySetOffline(true) })
	a.sources.localHost = func(context.Context, string) (bool, error) { return false, nil }
	a.page = pageFilms
	a.Open(item)
	waitUntil(t, tt, a, updates, func() bool { return a.player.playErr != "" || (a.player.play != nil && a.player.play.source != "") })
	if a.player.playErr != playerErrLinkedOffline {
		t.Fatalf("offline: error %q", a.player.playErr)
	}
	refresh(tt, a)
	wantText(t, tt, msgs.MediaSourceOffline())
	tt.Press("id:film-stage-linked-privacy")
	wantSettings(t, a, settings.SectionPrivacy)

	serve := func() int {
		t.Helper()
		ps := phoneShelf{linked: maps.Clone(a.library.Linked), sources: a.mediaSourceIDs(), sourcePool: a.sources.pool, sourceList: slices.Clone(a.sources.list), offline: a.privacy.now, localHost: a.sources.localHost}
		got := phoneFilmOf(item, ps)
		if got.Remote == nil {
			t.Fatalf("served as %+v", got)
		}
		rec := httptest.NewRecorder()
		got.Remote.ServeHTTP(rec, httptest.NewRequest(http.MethodGet, "/video", nil))
		return rec.Code
	}
	if code := serve(); code != http.StatusServiceUnavailable {
		t.Fatalf("offline, a phone reads the source: %d", code)
	}
	a.sources.localHost = func(context.Context, string) (bool, error) { return true, nil }
	if code := serve(); code != http.StatusOK {
		t.Fatalf("offline, a source on the local network serves %d", code)
	}
	_ = s
}

// An indexed film is processed again from its source, as 重新处理 does, and 复制到资料库 copies its video to its placeholder path, after which it is a film of the library like any other, its record gone; a copy that fails says why and leaves it indexed.
func TestMediaSourceIndexedFilmProcessedAgainAndCopied(t *testing.T) {
	p := &streamingPipeline{}
	a, tt, updates := newTestApp(t, p)
	msgs := testlocale.Messages()
	s, src := browseSource(a, map[string]fakeSourceFile{"Talk.mp4": {data: "talk video"}})
	p.copier = a.mediaSourceCopier()
	a.mediaSourcePoolSync()
	a.sources.browse.source = s.ID
	a.sources.browse.chosen = map[string]int64{"Talk.mp4": 10}
	a.mediaSourceLink([]string{"Talk.mp4"})
	done := func() bool { return len(a.library.Jobs) == 1 && a.library.Jobs[0].Stage.Done() && !a.running() }
	waitUntil(t, tt, a, updates, done)
	job := a.library.Jobs[0]
	if job.Stage != core.StageComplete || !job.Linked {
		t.Fatalf("the job %+v", job)
	}
	item, _ := films.Find(a.films(), job.ID)

	p.mu.Lock()
	p.heard = nil
	p.mu.Unlock()
	a.Reprocess(item)
	waitUntil(t, tt, a, updates, func() bool { return a.running() || a.library.Jobs[0].Stage == core.StageQueued })
	waitUntil(t, tt, a, updates, done)
	p.mu.Lock()
	heard := slices.Clone(p.heard)
	p.mu.Unlock()
	if a.library.Jobs[0].Stage != core.StageComplete || len(heard) == 0 || !strings.HasPrefix(heard[0], "http://127.0.0.1:") {
		t.Fatalf("processed again: %+v, heard from %q", a.library.Jobs[0], heard)
	}

	menu := a.StoredMenu(a.cx, item)
	if len(menu) != 1 || menu[0].Label != msgs.MediaSourceCopyToLibrary() || menu[0].Disabled {
		t.Fatalf("the menu %+v", menu)
	}
	src.mu.Lock()
	src.unreachable = errors.New("dial tcp: connection refused")
	src.mu.Unlock()
	a.sources.pool.Forget(s.ID)
	a.CopyLinked([]films.Film{item})
	waitUntil(t, tt, a, updates, func() bool { return len(a.sources.copying) == 0 })
	if !a.linkedHas(item.VideoPath) || fileExists(item.VideoPath) {
		t.Fatal("a copy that failed changed the film")
	}
	refresh(tt, a)
	wantText(t, tt, msgs.MediaSourceCopyFailed(films.Name(item), "Living Room NAS", "dial tcp: connection refused"))

	src.mu.Lock()
	src.unreachable = nil
	src.mu.Unlock()
	a.CopyLinked([]films.Film{item})
	waitUntil(t, tt, a, updates, func() bool { return len(a.sources.copying) == 0 })
	if data, err := os.ReadFile(item.VideoPath); err != nil || string(data) != "talk video" {
		t.Fatalf("the copy: %q, %v", data, err)
	}
	if a.linkedHas(item.VideoPath) || a.library.Jobs[0].Linked {
		t.Fatalf("still indexed: %+v", a.library.Jobs[0])
	}
	if got, _ := films.Find(a.films(), job.ID); got.Linked != "" {
		t.Fatalf("the film is still marked: %+v", got)
	}
}

// A line saved from a film indexed from a media source keeps its sound, cut from where its source is served, as a line of a film on disk does, so the review plays it, and keeps playing it once the source cannot be read.
func TestASavedLineOfAnIndexedFilmKeepsItsSound(t *testing.T) {
	tools := media.Tools{FFmpeg: fakeTool(t, filepath.Join(t.TempDir(), "bin"), "ffmpeg")}
	a, tt, updates := newTestApp(t, tools)
	a.queue.stopped = true
	_, src := browseSource(a, map[string]fakeSourceFile{"talk.mp4": {data: "kando fake clip 12 102030 405060\n"}})
	a.mediaSourceOpenPage()
	refresh(tt, a)
	waitUntil(t, tt, a, updates, func() bool { return !a.sources.browse.loading })
	a.mediaSourceLink([]string{"talk.mp4"})
	waitUntil(t, tt, a, updates, func() bool { return len(a.library.Linked) == 1 })
	placeholder := filepath.Join(a.snap().LibraryDir, "Living Room NAS", "talk", "video.mp4")
	if _, ok := a.library.Linked[placeholder]; !ok {
		t.Fatalf("records %+v", a.library.Linked)
	}
	rel, err := filepath.Rel(a.saved.book.Dir(), placeholder)
	if err != nil {
		t.Fatal(err)
	}
	a.saved.book.Toggle(saved.Line{Video: filepath.ToSlash(rel), Start: 2 * time.Second, End: 3 * time.Second, Original: "おはよう", Film: "talk"})
	l := a.saved.book.Lines[0]
	a.savedKeepSound(l)
	waitUntil(t, tt, a, updates, func() bool { return len(a.saved.keeping) == 0 })
	kept := saved.StoredSound(a.saved.book, l.ID)
	if !saved.HasSound(kept) {
		t.Fatalf("no sound kept at %s", kept)
	}
	src.mu.Lock()
	src.unreachable = errors.New("dial tcp: connection refused")
	src.mu.Unlock()
	a.sources.pool.Forget(a.sources.list[0].ID)
	if from := a.savedSoundSource(placeholder); from == nil {
		t.Fatal("an indexed film has nothing to cut its sound from")
	}
}

// A job making the lines of an indexed film that was copied into the library meanwhile reports itself as the indexed film's still, the pipeline holding the job as it was queued: the library keeps the film as one of its own, so it is made again from the copy, not from the source.
func TestAJobOfAFilmCopiedMeanwhileStaysCopied(t *testing.T) {
	a, _, _ := newTestApp(t, &fakePipeline{})
	video := filepath.Join(a.snap().LibraryDir, "Living Room NAS", "talk", "video.mp4")
	job := core.Job{ID: "talk", Source: mediasource.Ref{Source: "0123456789abcdef", Path: "talk.mp4"}.String(), VideoPath: video, Linked: true, Stage: core.StageTranscribing, LibraryDir: a.snap().LibraryDir}
	a.library.Upsert(job)
	// linkedCopied, as the copy ends: the library's job is the library's own.
	copied := job
	copied.Linked = false
	a.library.Upsert(copied)
	done := job
	done.Stage = core.StageComplete
	a.show(done)
	if got, _ := a.library.Find("talk"); got.Linked || got.Stage != core.StageComplete {
		t.Fatalf("the job is kept as %+v", got)
	}
}

// Taking an indexed film off the shelf while its video is being copied into the library stops the copy, which would otherwise leave a video there that no film has.
func TestForgettingAnIndexedFilmStopsItsCopy(t *testing.T) {
	a, _, _ := newTestApp(t, &fakePipeline{})
	video := filepath.Join(a.snap().LibraryDir, "Living Room NAS", "talk", "video.mp4")
	a.library.Linked = map[string]shelf.Linked{video: {Source: "0123456789abcdef", Path: "talk.mp4"}}
	stopped := false
	a.sources.copying = map[string]context.CancelFunc{video: func() { stopped = true }}
	a.libraryForget(a.snap().LibraryDir, []films.Film{{Key: video, VideoPath: video, Linked: "0123456789abcdef"}})
	if !stopped || len(a.library.Linked) != 0 {
		t.Fatalf("stopped %v, records %+v", stopped, a.library.Linked)
	}
}

// filmOfVideo is the film of the open library whose video is at video, as its key is the job's ID once a job holds it.
func filmOfVideo(t *testing.T, a *app, video string) films.Film {
	t.Helper()
	for _, item := range a.films() {
		if item.VideoPath == video {
			return item
		}
	}
	t.Fatalf("no film of %s among %q", video, films.Keys(a.films()))
	return films.Film{}
}
