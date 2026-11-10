package media

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"testing"
	"time"
)

func TestNetscapeCookiesFormat(t *testing.T) {
	expires := time.Date(2030, 1, 2, 3, 4, 5, 0, time.UTC)
	data, n := netscapeCookies([]Cookie{
		{Domain: ".Example.com", Path: "/", Secure: true, HTTPOnly: true, Expires: expires, Name: "SID", Value: "abc=1"},
		{Domain: "www.example.com", Name: "pref", Value: "dark"},
		{Domain: "example.com", Name: "bad", Value: "a\tb"},
		{Domain: "example.com", Name: "", Value: "nameless"},
		{Domain: "", Name: "nodomain", Value: "x"},
	})
	want := "# Netscape HTTP Cookie File\n" +
		"#HttpOnly_.example.com\tTRUE\t/\tTRUE\t1893553445\tSID\tabc=1\n" +
		"www.example.com\tFALSE\t/\tFALSE\t0\tpref\tdark\n"
	if string(data) != want || n != 2 {
		t.Fatalf("got %d cookies:\n%s\nwant:\n%s", n, data, want)
	}
}

// cookiesYTDLP writes a yt-dlp that saves its arguments one a line to args and, when it is given --cookies, a copy of that file to jar and its ls -ln line to mode. Then it prints stdout and exits with code, or with wait set, waits to be stopped.
func cookiesYTDLP(t *testing.T, stdout string, code int, wait bool) (tool, args, jar, mode string) {
	t.Helper()
	dir := t.TempDir()
	tool, args, jar, mode = filepath.Join(dir, "yt-dlp"), filepath.Join(dir, "args"), filepath.Join(dir, "jar"), filepath.Join(dir, "mode")
	script := "#!/bin/sh\n" +
		"for a in \"$@\"; do echo \"$a\"; done > " + shellQuote(args) + "\n" +
		"prev=\nfor a in \"$@\"; do if [ \"$prev\" = --cookies ]; then ls -ln \"$a\" > " + shellQuote(mode) + "; cp \"$a\" " + shellQuote(jar+".tmp") + " && mv " + shellQuote(jar+".tmp") + " " + shellQuote(jar) + "; fi; prev=$a; done\n"
	if wait {
		script += "exec sleep 30\n"
	} else {
		script += "cat " + shellQuote(stdout) + "\nexit " + strconv.Itoa(code) + "\n"
	}
	if err := os.WriteFile(tool, []byte(script), 0o755); err != nil {
		t.Fatal(err)
	}
	return tool, args, jar, mode
}

func siteCookies(cookies []Cookie, userAgent string, err error) func(context.Context, string) ([]Cookie, string, error) {
	return func(context.Context, string) ([]Cookie, string, error) { return cookies, userAgent, err }
}

var signedIn = []Cookie{{Domain: ".bilibili.com", Path: "/", Secure: true, HTTPOnly: true, Name: "SESSDATA", Value: "secret-session"}}

// assertNoCookieFolders fails t when a download left its cookies folder in dir.
func assertNoCookieFolders(t *testing.T, dir string) {
	t.Helper()
	entries, err := os.ReadDir(dir)
	if err != nil && !errors.Is(err, os.ErrNotExist) {
		t.Fatal(err)
	}
	for _, e := range entries {
		t.Errorf("left %s in the cookie folder", e.Name())
	}
}

// A download passes the cookies the app's pages hold for the site in a 0600 file of its own, with the pages' user agent, and removes the file's folder once yt-dlp has finished, whether it succeeded or failed.
func TestDownloadPassesTheSitesCookies(t *testing.T) {
	for _, tc := range []struct {
		name   string
		stdout string
		code   int
	}{
		{"success", "testdata/ytdlp/download-youtube.stdout", 0},
		{"failure", "", 1},
	} {
		t.Run(tc.name, func(t *testing.T) {
			tool, args, jar, mode := cookiesYTDLP(t, tc.stdout, tc.code, false)
			cookieDir := filepath.Join(t.TempDir(), "Kando")
			var asked string
			tools := Tools{YTDLP: tool, OutputDir: t.TempDir(), DownloadTemp: t.TempDir(), CookieDir: cookieDir, Cookies: "chrome",
				SiteCookies: func(_ context.Context, source string) ([]Cookie, string, error) {
					asked = source
					return signedIn, "Mozilla/5.0 Kando", nil
				}}
			source := "https://www.bilibili.com/video/BV1xx411c7mD"
			_, err := tools.Download(context.Background(), source)
			if (err == nil) != (tc.code == 0) {
				t.Fatalf("err = %v", err)
			}
			if err != nil && strings.Contains(err.Error(), "secret-session") {
				t.Fatalf("the error shows a cookie's value: %v", err)
			}
			if asked != source {
				t.Fatalf("asked for the cookies of %q", asked)
			}
			data, _ := os.ReadFile(args)
			got := string(data)
			for _, want := range []string{"--cookies-from-browser\nchrome\n", "--user-agent\nMozilla/5.0 Kando\n"} {
				if !strings.Contains(got, want) {
					t.Fatalf("args lack %q:\n%s", want, got)
				}
			}
			file, ok := argAfter(got, "--cookies")
			if !ok || !strings.HasPrefix(file, cookieDir+string(filepath.Separator)) || filepath.Base(file) != "cookies.txt" {
				t.Fatalf("cookies file %q in %q", file, cookieDir)
			}
			contents, err := os.ReadFile(jar)
			if err != nil {
				t.Fatal(err)
			}
			if want := "# Netscape HTTP Cookie File\n#HttpOnly_.bilibili.com\tTRUE\t/\tTRUE\t0\tSESSDATA\tsecret-session\n"; string(contents) != want {
				t.Fatalf("cookies file:\n%s", contents)
			}
			perm, _ := os.ReadFile(mode)
			if !strings.HasPrefix(string(perm), "-rw-------") {
				t.Fatalf("cookies file mode %q", perm)
			}
			assertNoCookieFolders(t, cookieDir)
		})
	}
}

// A download that is stopped while yt-dlp runs removes its cookies folder too.
func TestDownloadRemovesTheCookiesWhenStopped(t *testing.T) {
	tool, _, jar, _ := cookiesYTDLP(t, "", 0, true)
	cookieDir := t.TempDir()
	tools := Tools{YTDLP: tool, OutputDir: t.TempDir(), DownloadTemp: t.TempDir(), CookieDir: cookieDir, SiteCookies: siteCookies(signedIn, "", nil)}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	result := make(chan error, 1)
	go func() {
		_, err := tools.Download(ctx, "https://www.bilibili.com/video/BV1xx411c7mD")
		result <- err
	}()
	deadline := time.Now().Add(10 * time.Second)
	for {
		if _, err := os.Stat(jar); err == nil {
			break
		}
		if time.Now().After(deadline) {
			t.Fatal("yt-dlp never got the cookies file")
		}
		time.Sleep(10 * time.Millisecond)
	}
	if entries, _ := os.ReadDir(cookieDir); len(entries) != 1 {
		t.Fatalf("%d folders in the cookie folder while yt-dlp runs", len(entries))
	}
	cancel()
	select {
	case err := <-result:
		if err == nil {
			t.Fatal("a stopped download succeeded")
		}
	case <-time.After(10 * time.Second):
		t.Fatal("the download did not stop")
	}
	assertNoCookieFolders(t, cookieDir)
}

// Without cookies for the site, or when the pages cannot list them, yt-dlp gets neither a cookies file nor the pages' user agent, and no folder is made.
func TestDownloadWithoutSiteCookies(t *testing.T) {
	for name, fn := range map[string]func(context.Context, string) ([]Cookie, string, error){
		"none":   siteCookies(nil, "Mozilla/5.0 Kando", nil),
		"failed": siteCookies(signedIn, "Mozilla/5.0 Kando", errors.New("no web view")),
		"unset":  nil,
	} {
		t.Run(name, func(t *testing.T) {
			tool, args := argsYTDLP(t)
			cookieDir := filepath.Join(t.TempDir(), "Kando")
			tools := Tools{YTDLP: tool, OutputDir: t.TempDir(), DownloadTemp: t.TempDir(), CookieDir: cookieDir, SiteCookies: fn}
			if _, err := tools.Download(context.Background(), "https://www.bilibili.com/video/BV1xx411c7mD"); err == nil {
				t.Fatal("expected the fake yt-dlp to fail")
			}
			data, _ := os.ReadFile(args)
			if strings.Contains(string(data), "--cookies") || strings.Contains(string(data), "--user-agent") {
				t.Fatalf("args %s", data)
			}
			if _, err := os.Stat(cookieDir); !errors.Is(err, os.ErrNotExist) {
				t.Fatalf("made the cookie folder: %v", err)
			}
		})
	}
}

// argAfter is the argument after flag in args, one a line as the fake yt-dlp saves them.
func argAfter(args, flag string) (string, bool) {
	lines := strings.Split(args, "\n")
	for i, line := range lines {
		if line == flag && i+1 < len(lines) {
			return lines[i+1], true
		}
	}
	return "", false
}

// A cookies folder a download left behind goes at the next startup, and nothing else in the folder does.
func TestSweepCookieDirs(t *testing.T) {
	dir := t.TempDir()
	stale := filepath.Join(dir, "cookies-123456")
	if err := os.MkdirAll(stale, 0o700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(stale, "cookies.txt"), []byte(cookiesHeader), 0o600); err != nil {
		t.Fatal(err)
	}
	for _, keep := range []string{"downloads", "posters"} {
		if err := os.MkdirAll(filepath.Join(dir, keep), 0o700); err != nil {
			t.Fatal(err)
		}
	}
	if err := os.WriteFile(filepath.Join(dir, "cookies-note"), []byte("a file, not a folder"), 0o600); err != nil {
		t.Fatal(err)
	}
	SweepCookieDirs(dir)
	if _, err := os.Stat(stale); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("stale cookies folder still there: %v", err)
	}
	for _, keep := range []string{"downloads", "posters", "cookies-note"} {
		if _, err := os.Stat(filepath.Join(dir, keep)); err != nil {
			t.Fatalf("%s went: %v", keep, err)
		}
	}
	SweepCookieDirs(filepath.Join(dir, "missing"))
	SweepCookieDirs("")
}
