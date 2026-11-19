package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"strings"

	"kando/apps/kando/addsheet"
	"kando/apps/kando/browse"
	"kando/apps/kando/browserext"
	"kando/apps/kando/remote"
	"kando/apps/kando/settings"
	"kando/apps/kando/sites/catalog"
	kcore "kando/packages/core"
)

// The browser extension (browserext/, resources/browser-extension): Settings › 通用 › 浏览器扩展, which registers Kando's native messaging host with the browsers and gives out the extension's folder, and the control socket's methods the host relays the extension's buttons to (remote.MethodBrowserState, MethodBrowserAdd, MethodBrowserShow), which follow the built-in browser's buttons (state_browse_buttons.go).

// extUI is the state of Settings › 通用 › 浏览器扩展: where the host stands (browserext.Look), which the app looks up off the UI thread as the 通用 page opens and on 重新检测, never while the page is drawn, a connection, a disconnection or the folder's copy under way, and why the last one failed.
type extUI struct {
	// env is the computer the card looks at, with Kando's own folder; shipped is the extension's folder the app was built with. Tests replace both.
	env     func(dir string) browserext.Env
	shipped func() string
	// status is what the last look found; known reports whether one has finished, gen counts the looks so only the newest one lands.
	status browserext.Status
	known  bool
	gen    int
	// busy is a connection, a disconnection or the folder's copy under way; err is why the last one failed, until the next one starts.
	busy bool
	err  string
}

func newExtUI() extUI {
	return extUI{env: browserext.EnvHere, shipped: func() string {
		exe, _ := os.Executable()
		return browserext.Shipped(runtime.GOOS, exe)
	}}
}

// browserExtDir is Kando's own folder beside its settings, where the extension's folder is copied and, on Windows, the host's manifests go.
func (a *app) browserExtDir() string { return filepath.Dir(a.settingsPath) }

// browserExtFolder is the copy of the extension's folder the browsers load (browserext.Install).
func (a *app) browserExtFolder() string {
	return filepath.Join(a.browserExtDir(), browserext.FolderName)
}

// browserExtHooks: the links of Kando's own scheme the bookmarklet opens are taken (browserExtOpenLinks), and once the window exists the host is registered again with the browsers it is registered with, for the executable as it is now, as after Kando moved or its AppImage was replaced, and the extension's folder given out before is brought up to the app's, off the UI thread; the browsers load the new one once the extension is reloaded.
func (a *app) browserExtHooks() areaHooks {
	return areaHooks{openLinks: a.browserExtOpenLinks, ready: func() {
		env, src, dst := a.ext.env(a.browserExtDir()), a.ext.shipped(), a.browserExtFolder()
		go func() {
			_ = browserext.Refresh(env)
			if _, err := os.Stat(dst); err == nil && !browserext.Installed(src, dst) {
				_, _ = browserext.Install(src, dst)
			}
		}()
	}}
}

// browserExtOpenLinks takes the links of Kando's own scheme the system opened Kando with, which the bookmarklet makes (browserext.ReadLink): the video of the page it names opens the Add sheet in front, whatever 链接到达后立即开始 says, as any page could make such a link. A page that is not one online video Kando can add now goes into the library's link field, and its message bar says what precheck finds wrong with it. Any other link is left alone.
func (a *app) browserExtOpenLinks(links []string) {
	for _, link := range links {
		page, title, at, ok := browserext.ReadLink(link)
		if !ok {
			continue
		}
		if !a.addSheetAsk(page, addsheet.FromBrowser, title, at) {
			if a.page != pageFilms || a.openKey != "" {
				a.Select(pageFilms)
			}
			a.queue.source, a.queue.linkFocus = page, true
			if msg, section := a.precheck([]string{page}); msg != "" {
				a.warn(msg, section)
			}
		}
		a.remoteRaise()
	}
}

// browserExtCheck looks for the browsers and the host again off the UI thread, keeping what the last look found on the page until it lands.
func (a *app) browserExtCheck() {
	a.ext.gen++
	gen, env, update := a.ext.gen, a.ext.env(a.browserExtDir()), a.update
	go func() {
		status := browserext.Look(env)
		update(func() {
			if gen == a.ext.gen {
				a.ext.status, a.ext.known = status, true
			}
		})
	}()
}

// browserExtRun runs work, which registers or unregisters the host, off the UI thread, then looks again, so the card says which browsers are connected, and why work failed.
func (a *app) browserExtRun(work func(browserext.Env) error) {
	if a.ext.busy {
		return
	}
	a.ext.busy, a.ext.err = true, ""
	a.ext.gen++
	gen, env, update := a.ext.gen, a.ext.env(a.browserExtDir()), a.update
	go func() {
		err := work(env)
		status := browserext.Look(env)
		update(func() {
			a.ext.busy = false
			if err != nil {
				a.ext.err = a.browserExtErrorText(err)
			}
			if gen == a.ext.gen {
				a.ext.status, a.ext.known = status, true
			}
		})
	}()
}

// browserExtConnect registers the host with every browser found (browserext.Connect).
func (a *app) browserExtConnect() {
	a.browserExtRun(func(env browserext.Env) error {
		_, err := browserext.Connect(env)
		return err
	})
}

// browserExtDisconnect unregisters the host from every browser (browserext.Disconnect).
func (a *app) browserExtDisconnect() { a.browserExtRun(browserext.Disconnect) }

// browserExtShowFolder copies the extension's folder beside Kando's settings off the UI thread (browserext.Install), and shows the copy in the file manager, selected, for the browser's Load unpacked or Load Temporary Add-on to pick.
func (a *app) browserExtShowFolder() {
	if a.ext.busy {
		return
	}
	a.ext.busy, a.ext.err = true, ""
	src, dst, update := a.ext.shipped(), a.browserExtFolder(), a.update
	go func() {
		folder, err := browserext.Install(src, dst)
		update(func() {
			a.ext.busy = false
			if err != nil {
				a.ext.err = a.browserExtErrorText(err)
				return
			}
			revealInFolder(a.cx, folder, func(err error) {
				if err != nil {
					a.ext.err = a.messages().LibraryRevealNotOpened()
				}
				a.Changed()
			})
		})
	}()
}

// browserExtErrorText is why connecting, disconnecting or copying the folder failed, in the interface's language for the failures browserext names itself; any other, as the system's permission denied, is shown as it is.
func (a *app) browserExtErrorText(err error) string {
	m := a.messages()
	switch {
	case errors.Is(err, browserext.ErrTemporary):
		return m.SettingsGeneralCLIMoveFirst()
	case errors.Is(err, browserext.ErrNoBrowser):
		return m.SettingsGeneralBrowserNone()
	case errors.Is(err, browserext.ErrNotShipped):
		return m.SettingsGeneralBrowserNotShipped()
	}
	return err.Error()
}

// BrowserExtension is where the host stands, from the last look, as the 浏览器扩展 card shows it.
func (a *app) BrowserExtension() settings.BrowserExtension {
	st := a.ext.status
	return settings.BrowserExtension{
		Known:     a.ext.known,
		Found:     st.Found,
		Connected: st.Connected,
		Temporary: st.Temporary,
		Busy:      a.ext.busy,
		Err:       a.ext.err,
	}
}

func (a *app) ConnectBrowsers() { a.browserExtConnect() }

func (a *app) DisconnectBrowsers() { a.browserExtDisconnect() }

func (a *app) ShowExtensionFolder() { a.browserExtShowFolder() }

// browserExtVideo is link when it is one video of a site the page's buttons know and the catalog lists (browse.VideoLink, kcore.Link.OneVideo), as the built-in browser's buttons take it (browsePageAdd); false otherwise. Any program of the user can call the socket, so the app checks again what the host passed on.
func browserExtVideo(link string) bool {
	if !browse.VideoLink(link) || !catalog.Load().On(link) {
		return false
	}
	parsed, ok := kcore.ParseLink(link)
	return ok && parsed.OneVideo()
}

// remoteBrowserState answers the extension's buttons as the built-in browser's are answered (browsePageAsk): their words in the interface's language, and what the open library has of each of the videos asked about, at most browse.MaxAsk of them; any other link is left out.
func (a *app) remoteBrowserState(ctx context.Context, p remote.BrowserStateParams) (remote.BrowserState, error) {
	var links []string
	for _, u := range browse.AskLinks(p.URLs) {
		if browserExtVideo(u) {
			links = append(links, u)
		}
	}
	return remote.OnUI(ctx, a.update, func() (remote.BrowserState, error) {
		haves := a.browseHaves()
		have := make(map[string]string, len(links))
		for _, u := range links {
			have[u] = haves[queueSourceKey(u)]
		}
		return remote.BrowserState{Text: remote.BrowserText(a.browsePageText()), Have: have}, nil
	})
}

// remoteBrowserAdd adds the video at p.URL as a click on one of the built-in browser's buttons does (browseAdd): a video the library has already the window shows there, in front (browseShowFilm); any other opens the Add sheet in front, unless Settings › 通用 › 浏览器扩展's 链接到达后立即开始 is on, or is added as the library's link field adds it (libraryEnqueue), whose message bar says what precheck finds wrong, with the setting that fixes it, or why the queue refused it; the window then comes to the front on the library, as the extension's page has nowhere to say it. Where the page's player was (p.At), or else where the link starts, is where the film opens once done.
func (a *app) remoteBrowserAdd(ctx context.Context, p remote.BrowserAddParams) (remote.BrowserAdded, error) {
	link, title, ok := browse.AddVideo(p.URL, p.Title)
	if !ok || !browserExtVideo(link) {
		return remote.BrowserAdded{}, remote.Errorf(remote.CodeInvalid, "not a link to one video of a site Kando's buttons know: %q", p.URL)
	}
	return remote.OnUI(ctx, a.update, func() (remote.BrowserAdded, error) {
		if job, ok := a.browseJob(link); ok {
			a.browseShowFilm(job.ID)
			a.remoteRaise()
			return remote.BrowserAdded{Have: browserExtHave(job), Shown: true}, nil
		}
		at := browse.PlayedTo(p.At)
		if at <= 0 {
			at = kcore.StartAt(link)
		}
		// Unless 链接到达后立即开始 is on, the window asks first, on the Add sheet, which the extension's popup says.
		if !a.addsheet.startAtOnce && a.addSheetAsk(link, addsheet.FromBrowser, title, at) {
			a.remoteRaise()
			return remote.BrowserAdded{Message: a.messages().AddSheetConfirmInKando()}, nil
		}
		if !a.addSheetEnqueue(link, addSheetResumeStamp(at)) {
			a.Select(pageFilms)
			a.remoteRaise()
			return remote.BrowserAdded{Failed: true, Message: a.lib.notice.Text}, nil
		}
		res := remote.BrowserAdded{Message: a.messages().BrowseAdded()}
		if job, ok := a.browseJob(link); ok {
			res.Have = browserExtHave(job)
		}
		switch {
		case a.lib.notice.Kind == "warning":
			res.Message = a.lib.notice.Text
		case strings.TrimSpace(title) != "":
			res.Message = a.messages().BrowseAddedTitled(title)
		}
		return res, nil
	})
}

// browserExtHave is what the library has of job's video, as the buttons say it.
func browserExtHave(job kcore.Job) string {
	if job.Stage == kcore.StageComplete {
		return browse.HaveDone
	}
	return browse.HaveQueued
}

// remoteBrowserShow brings the window to the front showing the film of the video at p.URL, as 下载视频 does on a video the library has (browseShowFilm), or the library when it has none.
func (a *app) remoteBrowserShow(ctx context.Context, p remote.BrowserShowParams) (remote.BrowserShown, error) {
	if !browserExtVideo(p.URL) {
		return remote.BrowserShown{}, remote.Errorf(remote.CodeInvalid, "not a link to one video of a site Kando's buttons know: %q", p.URL)
	}
	return remote.OnUI(ctx, a.update, func() (remote.BrowserShown, error) {
		if w := a.window.main; w != nil && w.Closed() {
			return remote.BrowserShown{}, remote.Errorf(remote.CodeNotReady, "the Kando window was closed; quit Kando and open it again to show it")
		}
		job, ok := a.browseJob(p.URL)
		if ok {
			a.browseShowFilm(job.ID)
		} else {
			a.Select(pageFilms)
		}
		a.remoteRaise()
		return remote.BrowserShown{Film: ok}, nil
	})
}
