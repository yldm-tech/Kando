package main

import (
	"os"
	"path/filepath"
	"testing"

	"kando/apps/kando/browse"
	"kando/apps/kando/browserext"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/remote"
	"kando/apps/kando/settings"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
)

// The extension's buttons ask what the library has of a page's videos as the built-in browser's do, and hear the same: the buttons' words in the interface's language, and for each video one of the buttons know, "" or what the library has; any other link is left out.
func TestBrowserExtAsksWhatTheLibraryHas(t *testing.T) {
	a, tt, updates := newTestApp(t, &fakePipeline{})
	remoteServe(t, a)
	msgs := testlocale.Messages()
	if err := a.Enqueue("https://youtu.be/jNQXAC9IVRw"); err != nil {
		t.Fatal(err)
	}
	settle(t, a, tt, updates)
	done, other := "https://www.youtube.com/watch?v=jNQXAC9IVRw", "https://www.youtube.com/shorts/TRWgBxVAb1M"
	got, err := remoteCall[remote.BrowserState](t, updates, remote.MethodBrowserState, remote.BrowserStateParams{URLs: []string{done, other, "https://www.youtube.com/@veritasium", "https://example.com/watch?v=jNQXAC9IVRw", done}})
	if err != nil {
		t.Fatal(err)
	}
	text := remote.BrowserText{Add: msgs.BrowseDownload(), Queued: msgs.BrowseQueued(), Done: msgs.BrowseInLibrary()}
	if got.Text != text || len(got.Have) != 2 || got.Have[done] != browse.HaveDone || got.Have[other] != "" {
		t.Fatalf("state %+v", got)
	}
	// It is the very answer the built-in browser's buttons read.
	page := browse.StateData(browse.PageState{Text: browse.PageText(got.Text), Have: got.Have})
	if want := browse.StateData(browse.PageState{Text: a.browsePageText(), Have: map[string]string{done: browse.HaveDone, other: ""}}); page != want {
		t.Fatalf("as the page reads it: %s, want %s", page, want)
	}
}

// A video the extension adds is added as a click on the built-in browser's button adds it: bilibili's localized link by its canonical one, named by the page's title in what the extension shows, without moving the window; adding a video the library has shows it in front instead.
func TestBrowserExtAddsAsTheBrowsersButton(t *testing.T) {
	a, _, updates := newTestApp(t, &fakePipeline{block: true})
	remoteServe(t, a)
	msgs := testlocale.Messages()
	a.page = pageYouTube
	localized, canonical := "https://www.bilibili.com/zh-Hans/video/BV1nx4y1x7t1/?p=2", "https://www.bilibili.com/video/BV1nx4y1x7t1/?p=2"
	added, err := remoteCall[remote.BrowserAdded](t, updates, remote.MethodBrowserAdd, remote.BrowserAddParams{URL: localized, Title: "爆旋陀螺\n"})
	if err != nil {
		t.Fatal(err)
	}
	if len(a.library.Jobs) != 1 || a.library.Jobs[0].Source != canonical {
		t.Fatalf("jobs %+v", a.library.Jobs)
	}
	if added != (remote.BrowserAdded{Have: browse.HaveQueued, Message: msgs.BrowseAddedTitled("爆旋陀螺")}) || a.page != pageYouTube {
		t.Fatalf("added %+v, page %q", added, a.page)
	}
	state, err := remoteCall[remote.BrowserState](t, updates, remote.MethodBrowserState, remote.BrowserStateParams{URLs: []string{localized, canonical}})
	if err != nil || state.Have[localized] != browse.HaveQueued || state.Have[canonical] != browse.HaveQueued {
		t.Fatalf("state %+v, %v", state, err)
	}

	again, err := remoteCall[remote.BrowserAdded](t, updates, remote.MethodBrowserAdd, remote.BrowserAddParams{URL: canonical})
	if err != nil {
		t.Fatal(err)
	}
	if again != (remote.BrowserAdded{Have: browse.HaveQueued, Shown: true}) || len(a.library.Jobs) != 1 || a.page != pageFilms || !a.lib.queueOpen {
		t.Fatalf("adding it again: %+v, jobs %d, page %q, queue open %v", again, len(a.library.Jobs), a.page, a.lib.queueOpen)
	}
}

// What the extension adds is checked again, as any program of the user can call the socket: a channel, a playlist or a site the buttons do not know is refused, and nothing is added.
func TestBrowserExtAddsOnlyOneVideo(t *testing.T) {
	a, _, updates := newTestApp(t, &fakePipeline{})
	remoteServe(t, a)
	for _, u := range []string{"https://www.youtube.com/@veritasium", "https://www.youtube.com/playlist?list=PLrAXtmErZgOeiKm4sgNOknGvNjby9efdf", "https://example.com/watch?v=jNQXAC9IVRw", "file:///etc/passwd", ""} {
		_, err := remoteCall[remote.BrowserAdded](t, updates, remote.MethodBrowserAdd, remote.BrowserAddParams{URL: u})
		remoteWantCode(t, err, remote.CodeInvalid)
		_, err = remoteCall[remote.BrowserShown](t, updates, remote.MethodBrowserShow, remote.BrowserShowParams{URL: u})
		remoteWantCode(t, err, remote.CodeInvalid)
	}
	if len(a.library.Jobs) != 0 {
		t.Fatalf("jobs %+v", a.library.Jobs)
	}
}

// A video the extension cannot add yet brings the window to the front on the library, whose message bar says why, with the setting that fixes it, as the extension's page has nowhere to say it; the extension's popup gets the same words.
func TestBrowserExtSaysWhyItCannotAdd(t *testing.T) {
	a, _, updates := newTestApp(t, media.Tools{})
	remoteServe(t, a)
	msgs := testlocale.Messages()
	a.page = pageBrowse
	added, err := remoteCall[remote.BrowserAdded](t, updates, remote.MethodBrowserAdd, remote.BrowserAddParams{URL: "https://youtu.be/jNQXAC9IVRw"})
	if err != nil {
		t.Fatal(err)
	}
	want := msgs.PrecheckToolsMissing("yt-dlp" + msgs.PrecheckToolSeparator() + "ffmpeg" + msgs.PrecheckToolSeparator() + "whisper-cli")
	if !added.Failed || added.Message != want || a.page != pageFilms || !a.lib.notice.Open || a.lib.notice.Text != want || a.lib.notice.Section != settings.SectionGeneral || len(a.library.Jobs) != 0 {
		t.Fatalf("added %+v, page %q, notice %+v", added, a.page, a.lib.notice)
	}
}

// Show in Kando shows the video's film in front, or the library when it has none.
func TestBrowserExtShowsTheFilm(t *testing.T) {
	a, _, updates := newTestApp(t, &fakePipeline{})
	remoteServe(t, a)
	remoteLibrary(t, a)
	a.page = pageBrowse
	shown, err := remoteCall[remote.BrowserShown](t, updates, remote.MethodBrowserShow, remote.BrowserShowParams{URL: "https://youtu.be/jNQXAC9IVRw"})
	if err != nil || shown.Film || a.page != pageFilms || a.openKey != "" {
		t.Fatalf("a video the library does not have: %+v, %v, page %q, film %q", shown, err, a.page, a.openKey)
	}
	a.page = pageBrowse
	shown, err = remoteCall[remote.BrowserShown](t, updates, remote.MethodBrowserShow, remote.BrowserShowParams{URL: "https://www.youtube.com/watch?v=abcdefg"})
	if err != nil || !shown.Film || a.page != pageFilms || a.openKey != "yt" {
		t.Fatalf("a film of the library: %+v, %v, page %q, film %q", shown, err, a.page, a.openKey)
	}
}

// browserExtFake has Settings › 通用 › 浏览器扩展 look at a Mac of its own, with Chrome and Firefox, whose home is home, Kando's folder beside its settings in it, and the extension's folder as the repository keeps it; it returns home.
func browserExtFake(t *testing.T, a *app) string {
	t.Helper()
	home, err := filepath.EvalSymlinks(t.TempDir())
	if err != nil {
		t.Fatal(err)
	}
	exe := filepath.Join(home, "Applications", "Kando.app", "Contents", "MacOS", "Kando")
	for _, dir := range []string{filepath.Dir(exe), filepath.Join(home, "Library", "Application Support", "Google", "Chrome"), filepath.Join(home, "Library", "Application Support", "Firefox")} {
		if err := os.MkdirAll(dir, 0o755); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(exe, []byte("app"), 0o755); err != nil {
		t.Fatal(err)
	}
	a.settingsPath = filepath.Join(home, "Library", "Application Support", "Kando", "settings.json")
	a.ext.env = func(dir string) browserext.Env {
		return browserext.Env{GOOS: "darwin", Home: home, Getenv: func(string) string { return "" }, Exe: exe, Dir: dir}
	}
	a.ext.shipped = func() string { return filepath.Join("resources", browserext.FolderName) }
	return home
}

// Settings › 通用 › 浏览器扩展 connects Kando to the browsers on the computer, says which, and disconnects them; Show Extension Folder copies the extension's folder beside the settings and shows it in the file manager.
func TestSettingsGeneralConnectsTheBrowsers(t *testing.T) {
	a, tt, updates := newTestApp(t, &fakePipeline{})
	home := browserExtFake(t, a)
	msgs := testlocale.Messages()
	tt = settingsOpenAt(a, tt, settings.SectionGeneral)
	tt.Resize(880, 1800)
	waitUntil(t, tt, a, updates, func() bool { return a.ext.known })
	for _, text := range []string{msgs.SettingsGeneralBrowser(), msgs.SettingsGeneralBrowserHint(), msgs.SettingsGeneralBrowserLoadHint()} {
		wantText(t, tt, text)
	}

	tt.Press("id:settings-browser-connect")
	waitUntil(t, tt, a, updates, func() bool { return !a.ext.busy })
	chrome := filepath.Join(home, "Library", "Application Support", "Google", "Chrome", "NativeMessagingHosts", browserext.HostName+".json")
	firefox := filepath.Join(home, "Library", "Application Support", "Mozilla", "NativeMessagingHosts", browserext.HostName+".json")
	for _, path := range []string{chrome, firefox} {
		if _, err := os.Stat(path); err != nil {
			t.Fatalf("after 连接: %v", err)
		}
	}
	wantText(t, tt, msgs.SettingsGeneralBrowserConnectedTo("Chrome"+msgs.SettingsGeneralBrowserSeparator()+"Firefox"))
	wantText(t, tt, msgs.SettingsGeneralBrowserConnected())
	if tt.Exists("id:settings-browser-connect") {
		t.Fatal("连接 still shows once every browser is connected")
	}

	tt.Press("id:settings-browser-disconnect")
	waitUntil(t, tt, a, updates, func() bool { return !a.ext.busy })
	if _, err := os.Stat(chrome); !os.IsNotExist(err) {
		t.Fatalf("after 断开连接, Chrome's manifest: %v", err)
	}
	wantText(t, tt, msgs.SettingsGeneralBrowserHint())
	if !tt.Exists("id:settings-browser-connect") {
		t.Fatal("连接 does not show once disconnected")
	}

	revealed := ""
	prev := revealInFolder
	revealInFolder = func(_ *ns.Scope, path string, done func(error)) { revealed = path; done(nil) }
	t.Cleanup(func() { revealInFolder = prev })
	tt.Press("id:settings-browser-folder")
	waitUntil(t, tt, a, updates, func() bool { return !a.ext.busy })
	folder := filepath.Join(home, "Library", "Application Support", "Kando", browserext.FolderName)
	if revealed != folder || !browserext.Installed(a.ext.shipped(), folder) {
		t.Fatalf("revealed %q, want the copy %s", revealed, folder)
	}

	// A build without the extension's folder says so.
	a.ext.shipped = func() string { return filepath.Join(home, "nothing") }
	tt.Press("id:settings-browser-folder")
	waitUntil(t, tt, a, updates, func() bool { return !a.ext.busy })
	wantText(t, tt, msgs.SettingsGeneralBrowserNotShipped())
}
