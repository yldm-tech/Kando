package media

import (
	"context"
	"os"
	"path/filepath"
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
