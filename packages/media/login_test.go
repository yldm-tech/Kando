package media

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"kando/packages/core"
)

// readTestdata reads a file of testdata/ytdlp.
func readTestdata(t *testing.T, name string) string {
	t.Helper()
	data, err := os.ReadFile(filepath.Join("testdata", "ytdlp", filepath.FromSlash(name)))
	if err != nil {
		t.Fatal(err)
	}
	return string(data)
}

// What yt-dlp prints when it cannot borrow a browser's sign-in tells why, as the card of a failed download and Settings › General say it.
func TestReadLoginTroubleTellsWhyTheSignInCouldNotBeRead(t *testing.T) {
	for file, want := range map[string]LoginTrouble{
		"errors/cookies-safari-denied.stderr":   LoginNoAccess,
		"errors/cookies-firefox-missing.stderr": LoginNotFound,
		"errors/cookies-chrome-in-use.stderr":   LoginInUse,
		"errors/cookies-chrome-dpapi.stderr":    LoginEncrypted,
		"login/chrome-keychain-denied.stderr":   LoginNoKey,
		"login/chrome-read.stderr":              "",
		"errors/private-youtube.stderr":         LoginUnreadable,
	} {
		if got := ReadLoginTrouble(readTestdata(t, file)); got != want {
			t.Errorf("%s: %q, want %q", file, got, want)
		}
	}
	if got := ReadLoginTrouble("ERROR: could not find safari cookies database"); got != LoginNotFound {
		t.Errorf("Safari without its database: %q", got)
	}
	// A job's error carries yt-dlp's ERROR lines after the stage's own words.
	if got := ReadLoginTrouble("download: yt-dlp: exit status 1: " + readTestdata(t, "errors/cookies-chrome-dpapi.stderr")); got != LoginEncrypted {
		t.Errorf("a job's error: %q", got)
	}
}

// The errors of a browser on Windows whose cookies yt-dlp cannot copy or decrypt fail a download as one whose sign-in could not be read, so its card offers the remedies of that.
func TestClassifyDownloadReadsWindowsCookieErrorsAsCookies(t *testing.T) {
	for _, file := range []string{"cookies-chrome-in-use.stderr", "cookies-chrome-dpapi.stderr"} {
		if got := ClassifyDownload(readTestdata(t, "errors/"+file)); got != core.FailCookies {
			t.Errorf("%s: %q, want %q", file, got, core.FailCookies)
		}
	}
}

// CheckLogin runs yt-dlp with the browser and a cookies file of its own and no URL, so nothing downloads, and reads what it printed: exiting with its usage error after reading the cookies is success; an error, or warnings that it had no key, say why not. The cookies file goes once yt-dlp exits.
func TestCheckLoginReadsTheSignInWithoutDownloading(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stderr string
		exit   int
		want   LoginTrouble
	}{
		{"read", "login/chrome-read.stderr", 2, ""},
		{"keychain denied", "login/chrome-keychain-denied.stderr", 2, LoginNoKey},
		{"Safari without Full Disk Access", "errors/cookies-safari-denied.stderr", 1, LoginNoAccess},
		{"Chrome on Windows", "errors/cookies-chrome-dpapi.stderr", 1, LoginEncrypted},
		{"another failure", "", 3, LoginUnreadable},
	} {
		t.Run(tc.name, func(t *testing.T) {
			dir := t.TempDir()
			cookies := filepath.Join(dir, "cookies")
			args := filepath.Join(dir, "args")
			fake := fakeTool{Args: args, Exit: tc.exit}
			if tc.stderr != "" {
				fake.Stderr = readTestdata(t, tc.stderr)
			}
			tools := Tools{YTDLP: writeFakeTool(t, filepath.Join(dir, "bin"), "yt-dlp", fake), CookieDir: cookies}
			trouble, err := tools.CheckLogin(context.Background(), "chrome")
			if trouble != tc.want || (err == nil) != (tc.want == "") {
				t.Fatalf("CheckLogin = %q, %v; want %q", trouble, err, tc.want)
			}
			data, err := os.ReadFile(args)
			if err != nil {
				t.Fatal(err)
			}
			got := strings.Split(strings.TrimSpace(string(data)), "\n")
			if len(got) != 4 || got[0] != "--cookies-from-browser" || got[1] != "chrome" || got[2] != "--cookies" || !strings.HasPrefix(got[3], filepath.Join(cookies, cookieDirPrefix)) {
				t.Fatalf("yt-dlp ran with %q", got)
			}
			if entries, err := os.ReadDir(cookies); err != nil || len(entries) != 0 {
				t.Fatalf("the cookies folder holds %v (%v)", entries, err)
			}
		})
	}
	if trouble, err := (Tools{}).CheckLogin(context.Background(), "chrome"); trouble != LoginUnreadable || err == nil {
		t.Fatalf("without yt-dlp: %q, %v", trouble, err)
	}
}

// closeBrowserFolder makes home the home folder of a Mac (closedBrowserData looks at macOS's folders whatever system the test runs on) and makes the folder dir under it, in which yt-dlp looks for a browser's login, one that cannot be listed, as macOS keeps another app's data from Kando without Full Disk Access: chmod 000 stands in for macOS's refusal. It skips where chmod does not close a folder: on Windows, and for root.
func closeBrowserFolder(t *testing.T, home, dir string) {
	t.Helper()
	if runtime.GOOS == "windows" || os.Geteuid() == 0 {
		t.Skip("chmod 000 does not keep a folder from this process")
	}
	macHome(t, home)
	folder := filepath.Join(home, filepath.FromSlash(dir))
	mkdirs(t, home, dir)
	if err := os.WriteFile(filepath.Join(folder, "Local State"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.Chmod(folder, 0); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = os.Chmod(folder, 0o755) })
}

// macHome makes home the home folder of a Mac whose browser folders closedBrowserData looks at.
func macHome(t *testing.T, home string) {
	t.Helper()
	saved := browserDataOS
	browserDataOS = "darwin"
	t.Cleanup(func() { browserDataOS = saved })
	t.Setenv("HOME", home)
}

// yt-dlp says it found no cookies database of a Chromium browser or Firefox whose data folder macOS will not let it list, as it drops the error of listing it. CheckLogin then looks at the folder: there but closed is the system keeping it from Kando, which Full Disk Access opens, and missing is a browser with no sign-in on this computer.
func TestCheckLoginTellsAClosedBrowserFolderFromAMissingOne(t *testing.T) {
	for _, tc := range []struct {
		name, browser, stderr, closed, open string
		want                                LoginTrouble
	}{
		{name: "Chrome's folder closed", browser: "chrome", stderr: "errors/cookies-chrome-missing.stderr", closed: "Library/Application Support/Google/Chrome", want: LoginNoAccess},
		{name: "Firefox's folder closed", browser: "firefox", stderr: "errors/cookies-firefox-missing.stderr", closed: "Library/Application Support/Firefox/Profiles", want: LoginNoAccess},
		{name: "Chrome not on the computer", browser: "chrome", stderr: "errors/cookies-chrome-missing.stderr", want: LoginNotFound},
		{name: "Chrome's folder open but without cookies", browser: "chrome", stderr: "errors/cookies-chrome-missing.stderr", open: "Library/Application Support/Google/Chrome", want: LoginNotFound},
		{name: "another browser's folder closed", browser: "chrome", stderr: "errors/cookies-chrome-missing.stderr", closed: "Library/Application Support/Firefox/Profiles", want: LoginNotFound},
	} {
		t.Run(tc.name, func(t *testing.T) {
			home := t.TempDir()
			if tc.closed != "" {
				closeBrowserFolder(t, home, tc.closed)
			} else {
				macHome(t, home)
				mkdirs(t, home, tc.open)
			}
			dir := t.TempDir()
			tools := Tools{YTDLP: writeFakeTool(t, filepath.Join(dir, "bin"), "yt-dlp", fakeTool{Stderr: readTestdata(t, tc.stderr), Exit: 1}), CookieDir: filepath.Join(dir, "cookies")}
			trouble, err := tools.CheckLogin(context.Background(), tc.browser)
			if trouble != tc.want || err == nil || !strings.Contains(err.Error(), "cookies database") {
				t.Fatalf("CheckLogin = %q, %v; want %q", trouble, err, tc.want)
			}
		})
	}
}

// A download that borrowed the login of a browser whose data folder macOS keeps from Kando fails as one whose login could not be read, and says why as the check does, so its card offers Full Disk Access too.
func TestDownloadSaysTheSystemKeepsTheBrowserFolder(t *testing.T) {
	home := t.TempDir()
	closeBrowserFolder(t, home, "Library/Application Support/Google/Chrome")
	dir := t.TempDir()
	tools := Tools{
		YTDLP:        writeFakeTool(t, filepath.Join(dir, "bin"), "yt-dlp", fakeTool{Stderr: readTestdata(t, "errors/cookies-chrome-missing.stderr"), Exit: 1}),
		OutputDir:    filepath.Join(dir, "library"),
		DownloadTemp: filepath.Join(dir, "temp"),
		Cookies:      "chrome",
	}
	_, err := tools.Download(context.Background(), "https://www.youtube.com/watch?v=jNQXAC9IVRw")
	if err == nil {
		t.Fatal("the download did not fail")
	}
	if got := ClassifyDownload(err.Error()); got != core.FailCookies {
		t.Errorf("ClassifyDownload = %q, want %q", got, core.FailCookies)
	}
	if got := ReadLoginTrouble(err.Error()); got != LoginNoAccess {
		t.Errorf("ReadLoginTrouble = %q, want %q (%v)", got, LoginNoAccess, err)
	}
	// Without the browser borrowed, the same folder says nothing.
	tools.Cookies = ""
	if _, err := tools.Download(context.Background(), "https://www.youtube.com/watch?v=jNQXAC9IVRw"); err == nil || ReadLoginTrouble(err.Error()) != LoginNotFound {
		t.Errorf("without a browser: %v", err)
	}
}

// BrowserCookies runs yt-dlp as CheckLogin does and gives the cookies of the file it saved that keep takes, HttpOnly where keep says so, and none of the others; the file's folder is gone once it returns. A sign-in it cannot read gives no cookies but the reason.
func TestBrowserCookiesReadsOnlyTheCookiesKept(t *testing.T) {
	dir := t.TempDir()
	cookieDir := filepath.Join(dir, "cookies")
	args := filepath.Join(dir, "args")
	saved := "# Netscape HTTP Cookie File\n" +
		".youtube.com\tTRUE\t/\tTRUE\t0\tSID\tsid\n" +
		".youtube.com\tTRUE\t/\tTRUE\t0\tYSC\tysc\n" +
		".bank.example\tTRUE\t/\tTRUE\t0\tsession\tbank-secret\n"
	fake := fakeTool{Args: args, Cookies: saved, Stderr: readTestdata(t, "login/chrome-read.stderr"), Exit: 2}
	tools := Tools{YTDLP: writeFakeTool(t, filepath.Join(dir, "bin"), "yt-dlp", fake), CookieDir: cookieDir}
	var asked []string
	cookies, trouble, err := tools.BrowserCookies(context.Background(), "chrome", func(domain, name string) (bool, bool) {
		asked = append(asked, domain+" "+name)
		return domain == ".youtube.com", name == "YSC"
	})
	if err != nil || trouble != "" {
		t.Fatalf("BrowserCookies = %q, %v", trouble, err)
	}
	if len(cookies) != 2 || cookies[0].Name != "SID" || cookies[0].Value != "sid" || !cookies[0].Secure || cookies[0].HTTPOnly || cookies[1].Name != "YSC" || !cookies[1].HTTPOnly {
		t.Fatalf("cookies %+v", cookies)
	}
	if strings.Join(asked, ",") != ".youtube.com SID,.youtube.com YSC,.bank.example session" {
		t.Fatalf("keep was asked about %q", asked)
	}
	data, err := os.ReadFile(args)
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Split(strings.TrimSpace(string(data)), "\n"); len(got) != 4 || got[0] != "--cookies-from-browser" || got[1] != "chrome" || got[2] != "--cookies" || !strings.HasPrefix(got[3], filepath.Join(cookieDir, cookieDirPrefix)) {
		t.Fatalf("yt-dlp ran with %q", got)
	}
	if entries, err := os.ReadDir(cookieDir); err != nil || len(entries) != 0 {
		t.Fatalf("the cookies folder holds %v (%v)", entries, err)
	}

	fake = fakeTool{Cookies: saved, Stderr: readTestdata(t, "errors/cookies-safari-denied.stderr"), Exit: 1}
	tools.YTDLP = writeFakeTool(t, filepath.Join(dir, "denied"), "yt-dlp", fake)
	cookies, trouble, err = tools.BrowserCookies(context.Background(), "safari", func(string, string) (bool, bool) { return true, false })
	if len(cookies) != 0 || trouble != LoginNoAccess || err == nil {
		t.Fatalf("Safari without Full Disk Access gave %d cookies, %q, %v", len(cookies), trouble, err)
	}
	if entries, err := os.ReadDir(cookieDir); err != nil || len(entries) != 0 {
		t.Fatalf("the cookies folder holds %v (%v)", entries, err)
	}
}

// signedOutYTDLP writes a yt-dlp that fails as yt-dlp does when it cannot read the browser's sign-in, with the testdata stderr cookies names, and does what signedOut says when it is run without --cookies-from-browser. Each run appends a line to the returned file.
func signedOutYTDLP(t *testing.T, cookies string, signedOut fakeTool) (tool, runs string) {
	t.Helper()
	dir := t.TempDir()
	runs = filepath.Join(dir, "runs")
	signedOut.Runs = runs
	fake := fakeTool{Runs: runs, Stderr: readTestdata(t, cookies), Exit: 1, SignedOut: &signedOut}
	return writeFakeTool(t, dir, "yt-dlp", fake), runs
}

func countRuns(t *testing.T, runs string) int {
	t.Helper()
	data, err := os.ReadFile(runs)
	if err != nil {
		t.Fatal(err)
	}
	return strings.Count(string(data), "run\n")
}

// A download that needs no sign-in, as a public video's, is not failed by a browser whose sign-in yt-dlp cannot read, as Chrome's data folder macOS keeps from Kando: yt-dlp stops before it opens the link, and Download runs it once more without --cookies-from-browser.
func TestDownloadGoesOnSignedOutWhenTheSignInCannotBeRead(t *testing.T) {
	for _, cookies := range []string{"errors/cookies-chrome-missing.stderr", "errors/cookies-safari-denied.stderr", "errors/cookies-chrome-in-use.stderr", "errors/cookies-chrome-dpapi.stderr"} {
		t.Run(cookies, func(t *testing.T) {
			tool, runs := signedOutYTDLP(t, cookies, fakeTool{Act: "download"})
			cache := t.TempDir()
			tools := Tools{YTDLP: tool, OutputDir: t.TempDir(), DownloadTemp: cache, Cookies: "chrome"}
			path, err := tools.Download(context.Background(), "https://www.youtube.com/watch?v=jNQXAC9IVRw")
			if err != nil {
				t.Fatal(err)
			}
			if data, _ := os.ReadFile(path); string(data) != "track 395track 251" {
				t.Fatalf("%s = %q", path, data)
			}
			if n := countRuns(t, runs); n != 2 {
				t.Fatalf("yt-dlp ran %d times", n)
			}
			if entries, _ := os.ReadDir(cache); len(entries) != 0 {
				t.Fatalf("temp folders left: %v", entries)
			}
		})
	}
}

// A video that needs the sign-in fails without it too; the error is then the sign-in's, which tells the user to let Kando read the browser, rather than the login the site asked for.
func TestDownloadReportsTheSignInWhenTheVideoNeedsIt(t *testing.T) {
	tool, runs := signedOutYTDLP(t, "errors/cookies-chrome-missing.stderr", fakeTool{Stderr: readTestdata(t, "errors/login-vimeo.stderr"), Exit: 1})
	tools := Tools{YTDLP: tool, OutputDir: t.TempDir(), DownloadTemp: t.TempDir(), Cookies: "chrome"}
	_, err := tools.Download(context.Background(), "https://vimeo.com/76979871")
	if err == nil {
		t.Fatal("expected an error")
	}
	if got := ReadLoginTrouble(err.Error()); got != LoginNotFound && got != LoginNoAccess {
		t.Fatalf("trouble %q of %v", got, err)
	}
	if got := ClassifyDownload(err.Error()); got != core.FailCookies {
		t.Fatalf("kind %q of %v", got, err)
	}
	if n := countRuns(t, runs); n != 2 {
		t.Fatalf("yt-dlp ran %d times", n)
	}
}

// A run without the sign-in that fails for what no sign-in changes, as the network, reports that.
func TestDownloadReportsAFailureTheSignInDoesNotChange(t *testing.T) {
	tool, _ := signedOutYTDLP(t, "errors/cookies-chrome-missing.stderr", fakeTool{Stderr: readTestdata(t, "errors/network-dns.stderr"), Exit: 1})
	tools := Tools{YTDLP: tool, OutputDir: t.TempDir(), DownloadTemp: t.TempDir(), Cookies: "chrome"}
	_, err := tools.Download(context.Background(), "https://example.com/video")
	if got := ClassifyDownload(fmt.Sprint(err)); got != core.FailHost {
		t.Fatalf("kind %q of %v", got, err)
	}
}

// Only a sign-in yt-dlp could not read is tried again without it: a failure of the video, or a Keychain that kept the key, which yt-dlp goes on without, runs yt-dlp once.
func TestDownloadDoesNotRetryOtherFailures(t *testing.T) {
	for _, stderr := range []string{"errors/private-youtube.stderr", "errors/network-dns.stderr", "login/chrome-keychain-denied.stderr"} {
		t.Run(stderr, func(t *testing.T) {
			dir := t.TempDir()
			runs := filepath.Join(dir, "runs")
			tool := writeFakeTool(t, dir, "yt-dlp", fakeTool{Runs: runs, Stderr: readTestdata(t, stderr) + "\nERROR: [youtube] jNQXAC9IVRw: Private video\n", Exit: 1, SignedOut: &fakeTool{Runs: runs, Act: "download"}})
			tools := Tools{YTDLP: tool, OutputDir: t.TempDir(), DownloadTemp: t.TempDir(), Cookies: "chrome"}
			if _, err := tools.Download(context.Background(), "https://www.youtube.com/watch?v=jNQXAC9IVRw"); err == nil {
				t.Fatal("expected an error")
			}
			if n := countRuns(t, runs); n != 1 {
				t.Fatalf("yt-dlp ran %d times", n)
			}
		})
	}
}

// Listing, previewing, streaming and fetching a video's subtitles borrow the sign-in as Download does, and go on without it as it does.
func TestOtherRunsGoOnSignedOutWhenTheSignInCannotBeRead(t *testing.T) {
	ctx := context.Background()
	cases := map[string]struct {
		signedOut fakeTool
		run       func(Tools) error
	}{
		"list": {fakeTool{StdoutFile: "testdata/ytdlp/list/playlist.json"}, func(tools Tools) error {
			_, err := tools.ListVideos(ctx, "https://www.youtube.com/playlist?list=PLFs4vir_WsTwEd-nJgVJCZPNL3HALHHpF", 0)
			return err
		}},
		"preview": {fakeTool{Stdout: `{"_type": "video", "title": "Me at the zoo"}`}, func(tools Tools) error {
			_, err := tools.Preview(ctx, "https://www.youtube.com/watch?v=jNQXAC9IVRw")
			return err
		}},
		"stream": {fakeTool{Stdout: "https://example.com/stream.m3u8\n"}, func(tools Tools) error {
			_, err := tools.StreamURL(ctx, "https://www.youtube.com/watch?v=jNQXAC9IVRw")
			return err
		}},
		"subtitles": {fakeTool{}, func(tools Tools) error {
			_, err := tools.PlatformSubtitles(ctx, "https://www.youtube.com/watch?v=jNQXAC9IVRw", "en")
			return err
		}},
	}
	for name, c := range cases {
		t.Run(name, func(t *testing.T) {
			if c.signedOut.StdoutFile != "" {
				abs, err := filepath.Abs(c.signedOut.StdoutFile)
				if err != nil {
					t.Fatal(err)
				}
				c.signedOut.StdoutFile = abs
			}
			tool, runs := signedOutYTDLP(t, "errors/cookies-chrome-missing.stderr", c.signedOut)
			if err := c.run(Tools{YTDLP: tool, Cookies: "chrome"}); err != nil {
				t.Fatal(err)
			}
			if n := countRuns(t, runs); n != 2 {
				t.Fatalf("yt-dlp ran %d times", n)
			}
		})
	}
}
