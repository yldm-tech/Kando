package main

import (
	"log"
	"time"

	"kando/apps/kando/browse"
	"kando/apps/kando/settings"
	"kando/packages/media"
)

// The built-in browser's pages are signed in as the user is in the browser chosen in Settings › 通用 › 浏览器登录: the cookies of the catalog's sites that yt-dlp reads from it (signinRead) go into the web views' store (core.Scope.SetWebCookies), as the browser is chosen, as the browser first opens in a launch, and on 刷新登录. Never as the app starts, as on macOS reading a Chromium browser's sign-in asks for the Keychain.

// browseSignInState is what the browser knows of its pages' sign-in: opened is set once its page showed in this launch, signedIn names the browser whose sign-in the pages took last in it, and writing is set while the store takes it.
type browseSignInState struct {
	opened   bool
	signedIn string
	writing  bool
}

// browseSignInOnOpen reads the chosen browser's sign-in into the pages the first time the browser's page shows in a launch, unless they took that browser's in it already, as by choosing it. It reports whether it started reading.
func (a *app) browseSignInOnOpen() bool {
	if a.page != pageBrowse || a.browse.signin.opened {
		return false
	}
	a.browse.signin.opened = true
	if a.signin.browser == "" || a.signin.checking || a.browse.signin.signedIn == a.signin.browser {
		return false
	}
	a.signinRead(signinFromBrowse)
	return a.signin.checking
}

// browseRefreshSignIn is 刷新登录: it reads the chosen browser's sign-in again, for a site the user signed in to there since.
func (a *app) browseRefreshSignIn() {
	if a.signin.checking || a.browse.signin.writing {
		return
	}
	a.signinRead(signinFromRefresh)
}

// browseSigningIn reports whether the chosen browser's sign-in is being read or put into the pages, when 刷新登录 waits.
func (a *app) browseSigningIn() bool { return a.signin.checking || a.browse.signin.writing }

// browseSignIn puts cookies, those of the catalog's sites that yt-dlp read from browser, into the web views' store, and loads the page shown again once it holds them, so the page is signed in. 刷新登录 says in the message bar that it worked; a store that refused them says why there, whoever asked.
func (a *app) browseSignIn(browser string, cookies []media.Cookie, ask signinAsk) {
	if a.cx == nil {
		return
	}
	a.browse.signin.writing = true
	a.cx.SetWebCookies(browse.WebCookies(cookies, time.Now()), func(err error) {
		a.browse.signin.writing = false
		defer a.Changed()
		if err != nil {
			log.Printf("putting %s's sign-in into the pages: %v", browser, err)
			a.browseWarn(a.messages().BrowseSignInFailed(err.Error()), "")
			return
		}
		a.browse.signin.signedIn = browser
		if ask == signinFromRefresh {
			a.browseTell(a.messages().BrowseSignInRefreshed(media.BrowserName(browser)), "")
		}
		if a.browseShowing() {
			a.browseCutShort()
			a.browse.view.Reload()
			a.browse.loading = true
		}
	})
}

// browseSignInTrouble says in the message bar why yt-dlp could not read browser's sign-in, in the words of the 浏览器登录 row, with the setting, when the browser asked for it; the row says it when the setting did.
func (a *app) browseSignInTrouble(browser string, trouble media.LoginTrouble, ask signinAsk) {
	if ask == signinFromSettings {
		return
	}
	a.browseWarn(settings.SignInWhy(a.messages(), signinPlatform(), browser, trouble), settings.SectionGeneral)
}
