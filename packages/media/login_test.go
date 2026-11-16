package media

import (
	"context"
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
