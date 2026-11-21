package main

import (
	"context"
	"log"
	"net/url"
	"runtime"
	"slices"
	"strings"

	"kando/apps/kando/settings"
	"kando/apps/kando/sites/catalog"
	"kando/packages/media"
)

// Settings › 通用 › 浏览器登录: the browser whose sign-in every download borrows (media.Tools.Cookies, yt-dlp's --cookies-from-browser), which the queue gives each job it starts without a browser of its own (queueRun), the playlist dialog reads lists with (playlistAsk), the built-in browser's pages are signed in with (browseSignIn), and the built-in browser's start names (browse.State.SignIn). Choosing one checks at once that yt-dlp can read its sign-in, and puts it into the pages once it could, so the Keychain's question on macOS comes from the user's choice rather than in the middle of a download; the browser asks again as it first opens in a launch and on 刷新登录, never as the app starts.

// signinUI is the state of 浏览器登录: the browser chosen, and the last check of its sign-in.
type signinUI struct {
	// browser is the chosen browser's --cookies-from-browser name, one of media.Browsers, or "" for 关闭, as it is until the user chooses one.
	browser string
	// checking is a check under way (media.Tools.BrowserCookies); checked is the browser the last finished check was about, and trouble what it found, "" when yt-dlp read the sign-in. gen counts the checks and choices, so only the newest check lands.
	checking bool
	checked  string
	trouble  media.LoginTrouble
	gen      int
}

// savedSignIn is 浏览器登录's part of settings.json: the browser chosen, empty for none.
type savedSignIn struct {
	SignInBrowser string `json:"signInBrowser,omitempty"`
}

// clean keeps only a browser Kando knows, as another version may have saved one it does not; it goes to yt-dlp as an argument.
func (s *savedSignIn) clean() {
	s.SignInBrowser = strings.TrimSpace(s.SignInBrowser)
	if !media.KnownBrowser(s.SignInBrowser) {
		s.SignInBrowser = ""
	}
}

// signinHooks: the browser chosen is saved and taken back at startup, without checking it then, so a launch never asks for the Keychain.
func (a *app) signinHooks() areaHooks {
	return areaHooks{
		save:    func(saved *savedSettings) { saved.savedSignIn = savedSignIn{SignInBrowser: a.signin.browser} },
		restore: func(saved savedSettings) { a.signin.browser = saved.SignInBrowser },
	}
}

// signinFor is the browser whose sign-in a job for source borrows when it names none of its own: the one chosen, for a link that downloads, and none for a video on disk.
func (a *app) signinFor(source string) string {
	if u, err := url.Parse(source); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return ""
	}
	return a.signin.browser
}

// signinChoose makes id, one of media.Browsers or "" for none, the browser the pages and downloads borrow the sign-in of, and checks it at once, which puts it into the pages; a check under way of another browser is dropped. 关闭 leaves the sign-in the pages took: 清除登录信息…, which would clear it (a.browse.clear), is left out of the start and the File menu until it is wired (nsgo#114, state_browse.go).
func (a *app) signinChoose(id string) {
	if id != "" && !media.KnownBrowser(id) {
		return
	}
	a.signin.browser = id
	a.signin.gen++
	a.signin.checking, a.signin.checked, a.signin.trouble = false, "", ""
	a.signinCheck()
}

// signinAsk is who asked to read the chosen browser's sign-in, which decides what is said of it where: Settings › 通用 (signinFromSettings), whose row says what the check found, the browser as it first opens in a launch (signinFromBrowse), whose message bar says what went wrong, or 刷新登录 (signinFromRefresh), whose bar also says that it worked.
type signinAsk uint8

const (
	signinFromSettings signinAsk = iota
	signinFromBrowse
	signinFromRefresh
)

func (a *app) signinCheck() { a.signinRead(signinFromSettings) }

// signinRead has yt-dlp read the chosen browser's sign-in off the UI thread, which on macOS asks the user, the first time, to allow its key in the Keychain, and once it could, puts the cookies of the video sites Browse is for (catalog.SignInCookies) into the built-in browser's pages (browseSignIn). Without a browser or a yt-dlp, which 组件 says is missing, there is nothing to read.
func (a *app) signinRead(ask signinAsk) {
	tools, ok := a.pipeline.(media.Tools)
	browser := a.signin.browser
	if browser == "" || !ok || tools.YTDLP == "" || a.update == nil {
		return
	}
	a.signin.gen++
	a.signin.checking = true
	gen, update := a.signin.gen, a.update
	go func() {
		cookies, trouble, err := tools.BrowserCookies(context.Background(), browser, catalog.SignInCookies)
		if err != nil {
			log.Printf("checking %s's sign-in: %v", browser, err)
		}
		update(func() {
			if gen != a.signin.gen {
				return
			}
			a.signin.checking, a.signin.checked, a.signin.trouble = false, browser, trouble
			if trouble != "" {
				a.browseSignInTrouble(browser, trouble, ask)
				return
			}
			a.browseSignIn(browser, cookies, ask)
		})
	}()
}

// signinPlatform is the system as the catalogs' platform choices name it (desktop.Platform), for the words of a failed download's card, which no scope reads.
func signinPlatform() string {
	switch runtime.GOOS {
	case "darwin":
		return "macOS"
	case "windows":
		return "Windows"
	case "linux":
		return "Linux"
	}
	return "other"
}

// SignIn is 浏览器登录 as the 通用 page shows it: the browser chosen, the browsers the computer has with the chosen one among them, and the last check of it.
func (a *app) SignIn() settings.SignIn {
	st := settings.SignIn{Browser: a.signin.browser, Browsers: slices.Clone(a.queue.ytdlp.browsers), Checking: a.signin.checking}
	if st.Browser != "" && !slices.ContainsFunc(st.Browsers, func(b media.Browser) bool { return b.ID == st.Browser }) {
		st.Browsers = append(st.Browsers, media.Browser{ID: st.Browser, Name: media.BrowserName(st.Browser)})
	}
	if !st.Checking && st.Browser != "" && a.signin.checked == st.Browser {
		st.Checked, st.Trouble = true, a.signin.trouble
	}
	return st
}

// SetSignInBrowser chooses the browser downloads borrow the sign-in of, saves it and checks it.
func (a *app) SetSignInBrowser(id string) {
	a.signinChoose(id)
	a.Save()
}

func (a *app) CheckSignIn() { a.signinCheck() }

func (a *app) OpenFullDiskAccess() { a.queueOpenDiskAccess() }
