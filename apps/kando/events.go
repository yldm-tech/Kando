package main

import (
	"context"

	"kando/apps/kando/live"

	"github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
)

// areaHooks is what an area does at the moments every area shares. Each area returns its hooks from a function in its own file (sidebarHooks in state_sidebar.go, libraryHooks in state_library.go, queueHooks in queue.go, settingsHooks in state_settings.go, cloudHooks in state_cloud.go, playerHooks in state_player.go, overlayHooks in state_overlay.go, menuHooks in state_menu.go, feedbackHooks in state_feedback.go, onboardingHooks in state_onboarding.go, kitsHooks in state_kits.go, toolsHooks in state_tools.go, remoteHooks in state_remote.go, windowHooks in state_window.go, logsHooks in state_logs.go, browseHooks in state_browse.go, lockHooks in state_lock.go), as each area returns its rows of the shortcut table. A nil field means the area does nothing then. An area that needs another moment sets the field in its own hooks, not a call where the moment happens.
//
// Startup runs the moments in this order: restore, shelfLoaded, shelfChanged (main's first refreshLibrary), then ready. Everything before ready runs before nsgo.Run starts the engine: a report through update waits for it (live.Rev.Post), but nothing may use the app's scope (app.cx) or a window, which do not exist yet. An area whose shelfChanged starts work that needs them holds it back until its ready, with a flag of its own that its restore sets. ready runs in run, the app's function, on the engine's goroutine, once the main window is open.
type areaHooks struct {
	// save writes the area's part of settings.json, its own struct embedded in savedSettings. restore takes it back at startup (restoreSettings), after the shelf and before the window exists.
	save    func(saved *savedSettings)
	restore func(saved savedSettings)
	// ready runs once the main window exists, on the engine's goroutine, with the app's scope in app.cx. It runs once, from run.
	ready func()
	// openLinks runs, from ready on, with the links of Kando's own scheme (nsgo.yaml's url_schemes) the system opened Kando with; each area takes those it knows and leaves the others. One handler takes them for every area (areasReady), as NSGo gives the links that came before any handler, as the one Kando was launched to open, to the first handler registered alone (core.Scope.OnOpenLinks).
	openLinks func(links []string)
	// afterUpdate runs after each report of background work has run on the engine's goroutine (app.update). Only the app's own update (appUpdate) runs it; a test's update does not.
	afterUpdate func()
	// afterChange runs once the main view is built and again after every change of the app's state (live.State.Changed), once the bindings have followed it, to bring the area up to date with that change, as closing a dialog whose reason went away. It is no clock: it runs only after a change, never on its own while a film plays, so work that follows time uses core.Video's OnProgress or the app scope's AfterFunc instead. It may change what a binding or the menu bar reads, and then returns true, and only then: areasFollow lets everything follow and runs every afterChange again, so one that always returned true would never let the app settle. It runs under the main view's scope (areasFollow), so what it creates stays until the window goes; it creates no Effect, which would pile up with each run. A modal flag (holdModal) is given back by the handler that closes its dialog, not here, so the menu bar follows at once.
	afterChange func() (changed bool)
	// playing runs at each of the player's progress reports (core.Video's OnProgress, about four times a second while a film plays, and once as it pauses, ends or a seek lands), after the playhead moved there, for an area that follows the film's time, as the palette noting how far it played. It reports whether that changed what a binding reads, and only then: the player calls Changed once for all of them, never on an ordinary tick.
	playing func() (changed bool)
	// leavingLibrary runs before applyLibrary opens another library; libraryChanged runs when applyLibrary returns, on every path.
	leavingLibrary func()
	libraryChanged func()
	// shelfLoaded runs after openLibrary read the shelf's library.json into app.library: once at startup, before ready, and in tests that call openLibrary.
	shelfLoaded func()
	// shelfChanged runs after the shelf's films may have changed: refreshLibrary read the library folder again, or show recorded a job's progress. At startup main's first refreshLibrary runs it before ready.
	shelfChanged func()
	// toolsChanged runs after the pipeline's tools changed: another speech model was chosen, or the tools were detected again (redetected).
	toolsChanged func(redetected bool)
	// llmChanged runs after the AI service's settings were applied to the pipeline.
	llmChanged func()
	// llmTested runs when the AI service's 测试 has answered: err is nil when it passed, and credential reports that the test asked the EveryAPI CLI for its key, so a pass also means the CLI is signed in.
	llmTested func(err error, credential bool)
	// positionsChanged runs after the player remembered or forgot where a video stopped (rememberPosition), now and then while it plays (playerKeep), or after a film's subtitle delay changed, so an area that keeps the positions and delays on disk can save them.
	positionsChanged func()
	// videosMoved runs after the shelf pointed its jobs at videos that moved on disk: the library folder was renamed in Finder, or the library refresh gathered a video into a folder of its own, paired a renamed one or found a missing one elsewhere. move gives a video's new path for its old one, and reports false for one that did not move; an area that keeps something by video path keys it by the new path. The shelf moves what library.json keeps itself (shelf.Library.MoveVideos) and saves it.
	videosMoved func(move func(path string) (string, bool))
	// beforeQuit runs when a quit starts, before the windows close: ⌘Q, the menu's and the status icon's Quit and the main window's close button where it quits (requestQuit), a signal, and a quit NSGo asks about, as the Dock's Quit or the updater's relaunch (quitRequested). An area that must not let the app quit yet, such as the queue while a job runs, prevents e and asks first; it quits again itself once the user agrees. A quit a signal asked for goes ahead without asking (queueWatchSignals). A quit the app cannot hold back, a logout, restart or shutdown, or a quit that asked no one (quitHeard), comes with e.Forced set, as the app is ending already; a dialog then would never be answered.
	beforeQuit func(e *quitEvent)
	// quitStarting runs once no area's beforeQuit prevented the quit, right before the windows close, so an area that keeps a window from closing (state_window.go) lets it close.
	quitStarting func()
	// windowShown runs when the main window was put away by its close button (shown false) or came back (state_window.go), as the player lets the display sleep while only its sound plays on.
	windowShown func(shown bool)
	// becameActive runs when the app becomes the active one, the one in front (watchLifecycle), as when the user clicks its window or one of its notifications.
	becameActive func()
	// quitting runs once when the app quits, after its windows have closed and the engine has stopped (main, once nsgo.Run returns), before the process ends, so an area can stop what would outlive it, such as a player's sound in a process of its own, or the queue's running job. The app's scope is gone by then: it uses none of it.
	quitting func()
}

// areas is each area's hooks, in the order they run.
func (a *app) areas() []areaHooks {
	return []areaHooks{
		// The lock comes first, as its libraryChanged locks the library left before the sidebar counts the films of the libraries that are not open (countLibraries), which leaves the locked ones out.
		a.lockHooks(),
		a.logsHooks(),
		a.sidebarHooks(),
		a.libraryHooks(),
		// The browser comes before the queue, as its ready sweeps the cookie folders of downloads that never finished (media.SweepCookieDirs) before the queue's ready starts the first job left waiting, which would make one of its own, and gives the pipeline the pages' cookies (media.Tools.SiteCookies) there too, which that job then takes.
		a.browseHooks(),
		a.queueHooks(),
		a.settingsHooks(),
		a.pluginsHooks(),
		a.cloudHooks(),
		a.locationsHooks(),
		a.mediaSourceHooks(),
		a.storedHooks(),
		a.playerHooks(),
		a.correctHooks(),
		a.glossaryHooks(),
		a.reviewHooks(),
		a.overlayHooks(),
		a.menuHooks(),
		a.feedbackHooks(),
		a.paletteHooks(),
		a.onboardingHooks(),
		a.sitesHooks(),
		a.kitsHooks(),
		a.toolsHooks(),
		a.remoteHooks(),
		a.browserExtHooks(),
		a.windowHooks(),
		a.playlistHooks(),
		a.addSheetHooks(),
		a.onlineHooks(),
		a.savedHooks(),
		a.signinHooks(),
		a.phoneSyncHooks(),
		a.phoneFilmsHooks(),
		a.trayHooks(),
		a.privacyHooks(),
		a.subscribeHooks(),
		a.categoryHooks(),
		a.dropHooks(),
		a.vocabHooks(),
		a.subexportHooks(),
		a.explainHooks(),
		a.recapHooks(),
		// The encrypted libraries' images come last: the quit detaches them once the queue and the player have stopped using their files.
		a.lockVaultHooks(),
	}
}

func (a *app) areasSave(saved *savedSettings) {
	for _, h := range a.areas() {
		if h.save != nil {
			h.save(saved)
		}
	}
}

func (a *app) areasRestore(saved savedSettings) {
	for _, h := range a.areas() {
		if h.restore != nil {
			h.restore(saved)
		}
	}
}

func (a *app) areasReady() {
	a.areasListenLinks()
	for _, h := range a.areas() {
		if h.ready != nil {
			h.ready()
		}
	}
}

// areasListenLinks registers the one handler of the links the system opens Kando with (areaHooks.openLinks): those a launch names, which NSGo keeps for the first handler, and those that reach the running app, as a second launch on Windows and Linux hands them over.
func (a *app) areasListenLinks() {
	nsdesktop.OnOpenLinks(a.cx, live.On(a, a.areasOpenLinks))
}

// areasOpenLinks gives the links the system opened Kando with to every area's openLinks.
func (a *app) areasOpenLinks(links []string) {
	for _, h := range a.areas() {
		if h.openLinks != nil {
			h.openLinks(links)
		}
	}
}

func (a *app) areasAfterUpdate() {
	for _, h := range a.areas() {
		if h.afterUpdate != nil {
			h.afterUpdate()
		}
	}
}

// areasAfterChange runs every area's afterChange and reports whether any changed the state.
func (a *app) areasAfterChange() bool {
	changed := false
	for _, h := range a.areas() {
		if h.afterChange != nil && h.afterChange() {
			changed = true
		}
	}
	return changed
}

// areasFollow runs the areas' afterChange once now and again after every change of the app's state, from cx, the main view's scope, and then brings the menu bar up to date (menuFollow): one effect, which runs after the bindings, so the menu bar is always built after the hooks, from the state they left. When a hook changed something, the effect says so and runs again before the menu bar is built. The hooks run under cx (core.Scope.Run), not under the effect, whose next run would dispose of what they created.
func (a *app) areasFollow(cx *core.Scope) {
	cx.Effect(func() {
		a.Track()
		changed := false
		cx.Run(func() { changed = a.areasAfterChange() })
		if changed {
			a.Changed()
			return
		}
		a.menuFollow()
	})
}

// areasPlaying runs every area's playing hook and reports whether any changed what a binding reads.
func (a *app) areasPlaying() bool {
	changed := false
	for _, h := range a.areas() {
		if h.playing != nil && h.playing() {
			changed = true
		}
	}
	return changed
}

func (a *app) areasLeavingLibrary() {
	for _, h := range a.areas() {
		if h.leavingLibrary != nil {
			h.leavingLibrary()
		}
	}
}

func (a *app) areasLibraryChanged() {
	for _, h := range a.areas() {
		if h.libraryChanged != nil {
			h.libraryChanged()
		}
	}
}

func (a *app) areasShelfLoaded() {
	for _, h := range a.areas() {
		if h.shelfLoaded != nil {
			h.shelfLoaded()
		}
	}
}

func (a *app) areasShelfChanged() {
	for _, h := range a.areas() {
		if h.shelfChanged != nil {
			h.shelfChanged()
		}
	}
}

func (a *app) areasToolsChanged(redetected bool) {
	for _, h := range a.areas() {
		if h.toolsChanged != nil {
			h.toolsChanged(redetected)
		}
	}
}

func (a *app) areasLLMChanged() {
	for _, h := range a.areas() {
		if h.llmChanged != nil {
			h.llmChanged()
		}
	}
}

func (a *app) areasLLMTested(err error, credential bool) {
	for _, h := range a.areas() {
		if h.llmTested != nil {
			h.llmTested(err, credential)
		}
	}
}

func (a *app) areasPositionsChanged() {
	for _, h := range a.areas() {
		if h.positionsChanged != nil {
			h.positionsChanged()
		}
	}
}

func (a *app) areasVideosMoved(move func(path string) (string, bool)) {
	for _, h := range a.areas() {
		if h.videosMoved != nil {
			h.videosMoved(move)
		}
	}
}

// areasBeforeQuit asks every area whether the app may quit now, and when none prevented e, tells them all the quit goes ahead.
func (a *app) areasBeforeQuit(e *quitEvent) {
	for _, h := range a.areas() {
		if h.beforeQuit != nil {
			h.beforeQuit(e)
		}
	}
	if e.DefaultPrevented() {
		return
	}
	for _, h := range a.areas() {
		if h.quitStarting != nil {
			h.quitStarting()
		}
	}
}

func (a *app) areasWindowShown(shown bool) {
	for _, h := range a.areas() {
		if h.windowShown != nil {
			h.windowShown(shown)
		}
	}
}

func (a *app) areasBecameActive() {
	for _, h := range a.areas() {
		if h.becameActive != nil {
			h.becameActive()
		}
	}
}

func (a *app) areasQuitting() {
	for _, h := range a.areas() {
		if h.quitting != nil {
			h.quitting()
		}
	}
}

// quitEvent is a quit starting, which beforeQuit may hold back.
type quitEvent struct {
	// Forced is a quit the app cannot hold back and must not ask about: the system ending the session at a logout, restart or shutdown (quitRequested), or a quit that asked no one (quitHeard).
	Forced    bool
	prevented bool
}

// PreventDefault holds the quit back; the area that did quits again itself once it may.
func (e *quitEvent) PreventDefault() {
	if !e.Forced {
		e.prevented = true
	}
}

// DefaultPrevented reports whether an area held the quit back.
func (e *quitEvent) DefaultPrevented() bool { return e.prevented }

// requestQuit quits as ⌘Q does: the areas may hold the quit back and ask first (beforeQuit), and when none does, the app quits. Without the app's scope, as in a test that starts no app, nothing quits.
func (a *app) requestQuit() {
	e := &quitEvent{}
	a.areasBeforeQuit(e)
	if e.DefaultPrevented() || a.cx == nil {
		return
	}
	a.cx.Quit()
}

// watchQuit listens, under the app's scope app, to the quits that are not the app's own items' (requestQuit): NSGo's quit requests (quitRequested) and the app ending however it does (quitHeard). run calls it; a test that quits through nsgotest's RequestQuit calls it on its app.
func (a *app) watchQuit(app *core.Scope) {
	nsdesktop.OnQuitRequest(app, live.On(a, a.quitRequested))
	app.OnQuit(a.quitHeard)
}

// quitRequested answers a quit NSGo asks the app about (desktop.OnQuitRequest): the Dock's Quit, the updater's relaunch on macOS and Windows, and a logout, restart or shutdown. The main window's close button asks through requestQuit instead where it quits (windowClosing). The areas hear it as they hear ⌘Q (beforeQuit), and when one held it back to ask first, the request is canceled: that area quits again itself once the user agrees (requestQuit). One the system forces as the session ends cannot be held back, and the areas hear it with Forced set, as no dialog would be answered then.
func (a *app) quitRequested(q *nsdesktop.QuitRequest) {
	e := &quitEvent{Forced: q.Forced()}
	a.areasBeforeQuit(e)
	if e.DefaultPrevented() {
		q.Cancel()
	}
}

// quitHeard runs as the app quits however it does (core.Scope.OnQuit). A quit requestQuit or quitRequested let through has told the areas already; any other, which asked no one, tells them now with Forced set, as the app is ending and cannot ask.
func (a *app) quitHeard(context.Context) {
	// TODO(nsgo#1836): have the updater's relaunch ask the quit request handlers (desktop.OnQuitRequest) and wait for their answer before it quits, so a running job can ask first: on Linux it installs and then calls core.Scope.Restart, which quits at once and reaches the areas only here, and on Windows NSGo answers WinSparkle's can-shutdown check yes without asking (canShutdown in internal/platform/windows/updates.go), so WinSparkle goes on with the installer even when the request its shutdown sends is canceled https://github.com/nsgo-dev/nsgo/issues/1836
	if a.window.quitting {
		return
	}
	a.areasBeforeQuit(&quitEvent{Forced: true})
}
