package media

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"time"
)

// LoginTrouble is why yt-dlp could not borrow a browser's sign-in (--cookies-from-browser), as ReadLoginTrouble reads it from what yt-dlp printed; "" when nothing was wrong.
type LoginTrouble string

// What ReadLoginTrouble tells apart, in yt-dlp's words of 2026.08.19 (yt_dlp/cookies.py).
const (
	// LoginNoAccess is a browser whose cookies the system keeps from yt-dlp, as macOS keeps Safari's from an app without Full Disk Access ("Operation not permitted"), and the data folder of the other browsers, which yt-dlp then reports as no cookies database: CheckLogin and Download add to that error that the folder is there but closed (browserDataClosed).
	LoginNoAccess LoginTrouble = "no-access"
	// LoginNotFound is a browser that has no cookies on this computer ("could not find chrome cookies database"). macOS reports Safari's this way too when it keeps them from an app without Full Disk Access.
	LoginNotFound LoginTrouble = "not-found"
	// LoginInUse is a Chromium browser on Windows that is open and holds its cookies so yt-dlp cannot copy them ("Could not copy Chrome cookie database", yt-dlp#7271).
	LoginInUse LoginTrouble = "in-use"
	// LoginEncrypted is a Chromium browser on Windows whose cookies only the browser itself can decrypt, as Chrome's since version 127 (app-bound encryption), which yt-dlp cannot ("Failed to decrypt with DPAPI", yt-dlp#10927).
	LoginEncrypted LoginTrouble = "encrypted"
	// LoginNoKey is a Chromium browser whose cookies yt-dlp read without the key that decrypts them: the user did not allow "<Browser> Safe Storage" in the macOS Keychain (KeychainItem), or the Linux keyring did not give it. yt-dlp then goes on without them, so a download that needs the sign-in fails as signed out.
	LoginNoKey LoginTrouble = "no-key"
	// LoginUnreadable is any other reason yt-dlp gave.
	LoginUnreadable LoginTrouble = "unreadable"
)

// loginSigns are the phrases of yt-dlp's output that tell why it could not read a browser's cookies, in the order they are tried: its own errors first, then its warnings that it had no key, which it prints also when it goes on.
var loginSigns = []struct {
	trouble LoginTrouble
	phrases []string
}{
	{LoginNoAccess, []string{"operation not permitted", browserDataClosed}},
	{LoginInUse, []string{"could not copy chrome cookie database"}},
	{LoginEncrypted, []string{"failed to decrypt with dpapi"}},
	{LoginNotFound, []string{"cookies database", "could not find safari cookies"}},
	{LoginNoKey, []string{"no key found", "find-generic-password failed", "failed to read from keyring"}},
}

// browserDataClosed is what withClosedBrowserData adds to yt-dlp's error, in lower case as ReadLoginTrouble compares it.
const browserDataClosed = "the system keeps the browser's data folder from kando"

// withClosedBrowserData adds to err, the error of yt-dlp borrowing the login of browser (its --cookies-from-browser name), that the system keeps the browser's data folder from Kando, when err says yt-dlp found no cookies database and the folder is there but closed (closedBrowserData), so ReadLoginTrouble reads it as LoginNoAccess rather than LoginNotFound. Any other err is returned as it is.
func withClosedBrowserData(err error, browser string) error {
	if err == nil || browser == "" || ReadLoginTrouble(err.Error()) != LoginNotFound {
		return err
	}
	home, _ := os.UserHomeDir()
	if closed := closedBrowserData(browserDataOS, home, os.Getenv, browser); closed != nil {
		return fmt.Errorf("%w (%s: %w)", err, browserDataClosed, closed)
	}
	return err
}

// ReadLoginTrouble reads from what yt-dlp printed, or from an error that carries its ERROR lines as a job's does, why it could not borrow a browser's sign-in; "" when it says nothing of it.
func ReadLoginTrouble(output string) LoginTrouble {
	text := strings.ToLower(output)
	for _, sign := range loginSigns {
		for _, phrase := range sign.phrases {
			if strings.Contains(text, phrase) {
				return sign.trouble
			}
		}
	}
	if strings.Contains(output, "ERROR:") {
		return LoginUnreadable
	}
	return ""
}

// LoginCheckTimeout is how long CheckLogin waits for yt-dlp, which waits in turn while macOS asks the user whether it may use the browser's key in the Keychain.
const LoginCheckTimeout = 3 * time.Minute

// ytdlpUsageExit is how yt-dlp exits when it is given no URL, after it has done what it does without one, as reading the cookies it saves.
const ytdlpUsageExit = 2

// CheckLogin has yt-dlp read the sign-in of browser, as --cookies-from-browser names it, the way a download borrows it, without downloading anything: given no URL, yt-dlp still loads the browser's cookies to save them to the --cookies file, and exits with ytdlpUsageExit once it has, or with its error when it could not. That file, which holds every cookie of the browser, is written in a folder of its own in CookieDir and removed as soon as yt-dlp exits; SweepCookieDirs removes one a check that never ended left. On macOS the first read of a Chromium browser's sign-in asks the user to allow its key in the Keychain (KeychainItem), so the app checks only when the user asks for it. It returns "" when yt-dlp read the sign-in, and otherwise why not with an error that says it in yt-dlp's words, and that the system keeps the browser's data folder from Kando when yt-dlp took that for no cookies database (withClosedBrowserData).
func (t Tools) CheckLogin(ctx context.Context, browser string) (LoginTrouble, error) {
	_, trouble, err := t.readLogin(ctx, browser, nil)
	return trouble, err
}

// BrowserCookies reads the sign-in of browser as CheckLogin does and returns the cookies of it whose domain keep takes, read from the file yt-dlp saved them to (ReadCookies) before its folder is removed; the browser's other cookies never leave this call. trouble and err are CheckLogin's, and no cookies come with them.
func (t Tools) BrowserCookies(ctx context.Context, browser string, keep func(domain string) bool) (cookies []Cookie, trouble LoginTrouble, err error) {
	if keep == nil {
		keep = func(string) bool { return false }
	}
	return t.readLogin(ctx, browser, keep)
}

// readLogin is CheckLogin, which also reads the cookies keep takes from the file yt-dlp saved when keep is set.
func (t Tools) readLogin(ctx context.Context, browser string, keep func(domain string) bool) ([]Cookie, LoginTrouble, error) {
	if t.YTDLP == "" {
		return nil, LoginUnreadable, errors.New("yt-dlp is not installed")
	}
	base := t.CookieDir
	if base == "" {
		base = os.TempDir()
	}
	if err := os.MkdirAll(base, 0o700); err != nil {
		return nil, LoginUnreadable, err
	}
	dir, err := os.MkdirTemp(base, cookieDirPrefix+"*")
	if err != nil {
		return nil, LoginUnreadable, err
	}
	defer os.RemoveAll(dir)
	ctx, cancel := context.WithTimeout(ctx, LoginCheckTimeout)
	defer cancel()
	file := filepath.Join(dir, "cookies.txt")
	var stdout, stderr bytes.Buffer
	cmd := Command(ctx, t.YTDLP, "--cookies-from-browser", browser, "--cookies", file)
	cmd.Stdout, cmd.Stderr = &stdout, &stderr
	err = cmd.Run()
	if ctxErr := ctx.Err(); ctxErr != nil {
		return nil, LoginUnreadable, ctxErr
	}
	var exit *exec.ExitError
	if err != nil && !errors.As(err, &exit) {
		return nil, LoginUnreadable, err
	}
	output := stderr.String() + "\n" + stdout.String()
	if trouble := ReadLoginTrouble(output); trouble != "" {
		err := withClosedBrowserData(fmt.Errorf("yt-dlp: %s", toolError(output)), browser)
		return nil, ReadLoginTrouble(err.Error()), err
	}
	if err != nil && exit.ExitCode() != ytdlpUsageExit {
		return nil, LoginUnreadable, fmt.Errorf("yt-dlp: %w: %s", err, toolError(stderr.String()))
	}
	if keep == nil {
		return nil, "", nil
	}
	data, err := os.ReadFile(file)
	if errors.Is(err, fs.ErrNotExist) {
		// yt-dlp read the browser's sign-in without saving the file: there are no cookies to take.
		return nil, "", nil
	}
	if err != nil {
		return nil, LoginUnreadable, fmt.Errorf("yt-dlp saved no cookies of %s: %w", browser, err)
	}
	var cookies []Cookie
	for _, c := range ReadCookies(data) {
		if keep(c.Domain) {
			cookies = append(cookies, c)
		}
	}
	return cookies, "", nil
}

// KeychainItem is the item of the macOS Keychain that holds the key to the cookies of the Chromium browser whose --cookies-from-browser name is id, which macOS asks the user to allow yt-dlp to use, as yt_dlp/cookies.py of 2026.08.19 names it (keyring_name followed by Safe Storage); "" for a browser that has none, as Firefox and Safari.
func KeychainItem(id string) string {
	switch id {
	case "chrome":
		return "Chrome Safe Storage"
	case "edge":
		return "Microsoft Edge Safe Storage"
	case "brave":
		return "Brave Safe Storage"
	case "chromium":
		return "Chromium Safe Storage"
	}
	return ""
}

// KnownBrowser reports whether id is the --cookies-from-browser name of one of Browsers.
func KnownBrowser(id string) bool {
	for _, b := range Browsers {
		if b.ID == id {
			return true
		}
	}
	return false
}
