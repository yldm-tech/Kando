package main

import (
	"errors"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/settings"
	"kando/packages/core"
	"kando/packages/media"
)

// The browser chosen in Settings › 通用 › 浏览器登录 is kept in settings.json under a key of its own, and taken back at the next launch without being checked, so a launch never asks for the Keychain. A browser Kando does not know, as another version may save, is dropped, since it goes to yt-dlp as an argument.
func TestSignInBrowserIsSavedAndCleaned(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	a := newApp(&fakePipeline{}, func(func()) {})
	a.settingsPath = path
	if data, _ := a.settingsData(); strings.Contains(string(data), "signInBrowser") {
		t.Fatalf("Off is saved: %s", data)
	}
	a.SetSignInBrowser("firefox")
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), `"signInBrowser": "firefox"`) {
		t.Fatalf("settings.json %s (%v)", data, err)
	}
	b := newApp(&fakePipeline{}, func(func()) { t.Fatal("restoring checked the sign-in") })
	b.settingsPath = path
	b.restoreSettings(b.loadSettings())
	if b.signin.browser != "firefox" || b.signin.checking {
		t.Fatalf("restored %+v", b.signin)
	}
	for raw, want := range map[string]string{" chrome ": "chrome", "netscape": "", "--exec=rm": "", "": ""} {
		s := savedSignIn{SignInBrowser: raw}
		s.clean()
		if s.SignInBrowser != want {
			t.Errorf("clean(%q) = %q, want %q", raw, s.SignInBrowser, want)
		}
	}
}

// Every download the queue starts without a browser of its own borrows the chosen browser's sign-in from its first run, as yt-dlp's --cookies-from-browser, whichever way it was added; with 关闭 none does, a video on disk never does, and one a card's remedy gave a browser keeps that one.
func TestDownloadsBorrowTheChosenSignIn(t *testing.T) {
	a, tt, updates := newTestApp(t, &fakePipeline{})
	run := func(source string) core.Job {
		t.Helper()
		if err := a.Enqueue(source); err != nil {
			t.Fatal(err)
		}
		settle(t, a, tt, updates)
		job := a.library.Jobs[len(a.library.Jobs)-1]
		a.remove(job.ID)
		return job
	}
	if job := run("https://www.youtube.com/watch?v=Rg5Lk2Pz8Wd"); job.Cookies != "" {
		t.Fatalf("with Off the download borrows %q", job.Cookies)
	}
	a.signin.browser = "chrome"
	job := run("https://www.youtube.com/watch?v=Rg5Lk2Pz8Wd")
	if job.Cookies != "chrome" || job.Stage != core.StageComplete {
		t.Fatalf("the download borrows %q, %s", job.Cookies, job.Stage)
	}
	if job := run(core.FileURL(filepath.Join(t.TempDir(), "talk.mp4"))); job.Cookies != "" {
		t.Fatalf("a video on disk borrows %q", job.Cookies)
	}
	own, err := a.newJob("https://www.bilibili.com/video/BV1Hx411c7Pq")
	if err != nil {
		t.Fatal(err)
	}
	own.Cookies = "firefox"
	a.queueAdd(own)
	settle(t, a, tt, updates)
	if got, _ := a.library.Find(own.ID); got.Cookies != "firefox" {
		t.Fatalf("a job's own browser became %q", got.Cookies)
	}
}

// The queue's pipeline passes a job's browser to yt-dlp; Tools.Cookies is what Download and ListVideos give --cookies-from-browser.
func TestQueuePipelineBorrowsTheJobsBrowser(t *testing.T) {
	a := newApp(media.Tools{}, func(func()) {})
	a.signin.browser = "chrome"
	job, err := core.NewJob("https://www.youtube.com/watch?v=Rg5Lk2Pz8Wd")
	if err != nil {
		t.Fatal(err)
	}
	job.Cookies = a.signinFor(job.Source)
	if tools := a.queuePipeline(job).(media.Tools); tools.Cookies != "chrome" {
		t.Fatalf("Cookies = %q", tools.Cookies)
	}
}

// A card whose browser's sign-in could not be read says why in the words the setting uses, and 选择浏览器… opens Settings › 通用, where the browser every download borrows from is chosen. Chrome on Windows, whose encryption yt-dlp cannot undo, is told to use another browser; one that is open and holds its cookies is retried once it is closed.
func TestCookieRemediesPointAtTheSetting(t *testing.T) {
	a, tt, _ := newTestApp(t, &fakePipeline{})
	withBrowsers(t, a, "chrome", "firefox")
	msgs := testlocale.Messages()
	yt := "https://www.youtube.com/watch?v=Rg5Lk2Pz8Wd"
	for _, tc := range []struct {
		browser, err string
		why          string
		actions      []string
	}{
		{"chrome", "ERROR: Failed to decrypt with DPAPI. See  https://github.com/yt-dlp/yt-dlp/issues/10927  for more info", msgs.QueueRemedyCookiesEncrypted("Chrome"), []string{msgs.QueueRemedySwitchBrowser("Firefox"), msgs.QueueRemedyChooseBrowser()}},
		{"edge", "ERROR: Could not copy Chrome cookie database. See  https://github.com/yt-dlp/yt-dlp/issues/7271  for more info", msgs.QueueRemedyCookiesInUse("Edge"), []string{msgs.QueueRemedyRetry(), msgs.QueueRemedySwitchBrowser("Firefox"), msgs.QueueRemedyChooseBrowser()}},
		{"firefox", "ERROR: could not find firefox cookies database in '/Users/someone/Library/Application Support/Firefox/Profiles'", msgs.QueueRemedyCookiesWhy("Firefox"), []string{msgs.QueueRemedySwitchBrowser("Chrome"), msgs.QueueRemedyChooseBrowser()}},
	} {
		film := failedJob(t, a, yt, core.FailCookies, tc.browser)
		job, _ := a.library.Find(film.JobID)
		job.Err = errors.New("download: yt-dlp: exit status 1: " + tc.err)
		a.library.Upsert(job)
		r, ok := a.FailureRemedy(film)
		if !ok || r.Why != tc.why || strings.Join(remedyLabels(r), ",") != strings.Join(tc.actions, ",") {
			t.Errorf("%s: %+v", tc.browser, r)
		}
		a.remove(film.JobID)
	}
	film := failedJob(t, a, yt, core.FailCookies, "chrome")
	r, _ := a.FailureRemedy(film)
	choose := r.Actions[len(r.Actions)-1]
	if choose.ID != remedyChooseBrowser {
		t.Fatalf("the card's last action is %+v", choose)
	}
	a.RunRemedy(film, choose.ID)
	refresh(tt, a)
	wantSettings(t, a, settings.SectionGeneral)
	if !tt.Exists("id:settings-signin-browser") {
		t.Fatalf("Settings › 通用 shows no 浏览器登录:\n%s", tt.Dump())
	}
}

// Choosing a browser checks at once that yt-dlp can read its sign-in, without downloading anything, so macOS's Keychain question comes from the choice; the row says what the check found: ready, or why not in the card's words, with 重新检查, and for Safari 打开系统设置 too. 关闭 says what a browser is for.
func TestChoosingABrowserChecksItsSignIn(t *testing.T) {
	home := t.TempDir()
	tools := media.Tools{YTDLP: fakeTool(t, filepath.Join(home, "bin"), "yt-dlp"), CookieDir: filepath.Join(home, "cookies")}
	a, tt, updates := newTestApp(t, tools)
	a.settingsPath = filepath.Join(home, "settings.json")
	withBrowsers(t, a, "chrome", "edge", "safari")
	tt = settingsOpenAt(a, tt, settings.SectionGeneral)
	msgs := testlocale.Messages()
	wantText(t, tt, msgs.SettingsGeneralSignInOffHint())
	for _, tc := range []struct {
		browser, why string
		buttons      []string
	}{
		{"chrome", "", nil},
		{"edge", msgs.QueueRemedyCookiesNoKey("macOS", "Edge", "Microsoft Edge Safe Storage"), []string{"settings-signin-check"}},
		{"safari", msgs.QueueRemedySafariWhy(), []string{"settings-signin-disk-access", "settings-signin-check"}},
		{"firefox", msgs.QueueRemedyCookiesWhy("Firefox"), []string{"settings-signin-check"}},
	} {
		a.SetSignInBrowser(tc.browser)
		refresh(tt, a)
		if !a.signin.checking {
			t.Fatalf("%s: choosing it did not check it", tc.browser)
		}
		wantText(t, tt, msgs.SettingsGeneralSignInChecking(media.BrowserName(tc.browser)))
		waitUntil(t, tt, a, updates, func() bool { return !a.signin.checking })
		st := a.SignIn()
		if !st.Checked || (st.Trouble == "") != (tc.why == "") {
			t.Fatalf("%s: %+v", tc.browser, st)
		}
		if tc.why == "" {
			wantText(t, tt, msgs.SettingsGeneralSignInOn(media.BrowserName(tc.browser)))
			wantText(t, tt, msgs.SettingsGeneralSignInReady())
		} else {
			wantText(t, tt, tc.why)
		}
		for _, id := range []string{"settings-signin-check", "settings-signin-disk-access"} {
			if want := strings.Contains(strings.Join(tc.buttons, ","), id); tt.Exists("id:"+id) != want {
				t.Errorf("%s: %s shows %v", tc.browser, id, !want)
			}
		}
		if entries, _ := os.ReadDir(tools.CookieDir); len(entries) != 0 {
			t.Fatalf("%s: the check left %v", tc.browser, entries)
		}
	}
	tt.Press("id:settings-signin-check")
	if !a.signin.checking {
		t.Fatal("重新检查 did not check again")
	}
	waitUntil(t, tt, a, updates, func() bool { return !a.signin.checking })
	if data, err := os.ReadFile(a.settingsPath); err != nil || !strings.Contains(string(data), `"signInBrowser": "firefox"`) {
		t.Fatalf("settings.json %s (%v)", data, err)
	}
	a.SetSignInBrowser("")
	refresh(tt, a)
	if a.signin.checking || a.SignIn().Checked {
		t.Fatalf("关闭 checked: %+v", a.signin)
	}
	wantText(t, tt, msgs.SettingsGeneralSignInOffHint())
}

// The built-in browser's start says whose sign-in downloads use, in the language the app shows, with a button that opens Settings › 通用 to choose or change it.
func TestBrowseStartSaysWhoseSignInDownloadsUse(t *testing.T) {
	a, tt, _, _ := browseApp(t, &fakePipeline{})
	msgs := testlocale.Messages()
	wantText(t, tt, msgs.BrowseSignInOff())
	wantText(t, tt, msgs.BrowseSignInChoose())
	a.signin.browser = "chrome"
	refresh(tt, a)
	wantText(t, tt, msgs.BrowseSignInOn("Chrome"))
	wantText(t, tt, msgs.BrowseSignInChange())
	a.SetInterfaceLanguage("en")
	refresh(tt, a)
	en := locales.In("en")
	wantText(t, tt, en.BrowseSignInOn("Chrome"))
	wantText(t, tt, en.BrowseSignInChange())
	tt.Press("id:browse-signin")
	wantSettings(t, a, settings.SectionGeneral)
}
