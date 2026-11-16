package main

import (
	"context"

	"github.com/nsgo-dev/nsgo/core"
)

// areaHooks is what an area does at the moments every area shares. Each area returns its hooks from a function in its own file (sidebarHooks in state_sidebar.go, libraryHooks in state_library.go, queueHooks in queue.go, settingsHooks in state_settings.go, cloudHooks in state_cloud.go, playerHooks in state_player.go, overlayHooks in state_overlay.go, menuHooks in state_menu.go, feedbackHooks in state_feedback.go, onboardingHooks in state_onboarding.go, kitsHooks in state_kits.go, remoteHooks in state_remote.go, windowHooks in state_window.go, logsHooks in state_logs.go, browseHooks in state_browse.go), as each area returns its rows of the shortcut table. A nil field means the area does nothing then. An area that needs another moment sets the field in its own hooks, not a call where the moment happens.
//
// Startup runs the moments in this order: restore, shelfLoaded, shelfChanged (main's first refreshLibrary), then ready. Everything before ready runs before nsgo.Run starts the engine: a report through update waits for it (live.Rev.Post), but nothing may use the app's scope (app.cx) or a window, which do not exist yet. An area whose shelfChanged starts work that needs them holds it back until its ready, with a flag of its own that its restore sets. ready runs in run, the app's function, on the engine's goroutine, once the main window is open.
type areaHooks struct {
	// save writes the area's part of settings.json, its own struct embedded in savedSettings. restore takes it back at startup (restoreSettings), after the shelf and before the window exists.
	save    func(saved *savedSettings)
	restore func(saved savedSettings)
	// ready runs once the main window exists, on the engine's goroutine, with the app's scope in app.cx. It runs once, from run.
	ready func()
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
	// beforeQuit runs when a quit starts (requestQuit), before the windows close: ⌘Q and the menu's Quit, a signal, and on Linux and Windows the main window's close button once NSGo reports it. An area that must not let the app quit yet, such as the queue while a job runs, prevents e and asks first; it quits again itself once the user agrees. A quit a signal asked for goes ahead without asking (queueWatchSignals). A quit the app cannot hold back, the Dock's Quit, a logout, restart or shutdown or the updater's relaunch, comes with e.SessionEnding set (quitHeard), as the app is ending already; a dialog then would never be answered.
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
		a.logsHooks(),
		a.sidebarHooks(),
		a.libraryHooks(),
		// The browser comes before the queue, as its ready sweeps the cookie folders of downloads that never finished (media.SweepCookieDirs) before the queue's ready starts the first job left waiting, which would make one of its own; giving the pipeline the pages' cookies (media.Tools.SiteCookies) there too, which that job then takes, comes back with nsgo#112.
		a.browseHooks(),
		a.queueHooks(),
		a.settingsHooks(),
		a.cloudHooks(),
		a.playerHooks(),
		a.overlayHooks(),
		a.menuHooks(),
		a.feedbackHooks(),
		a.paletteHooks(),
		a.onboardingHooks(),
		a.sitesHooks(),
		a.kitsHooks(),
		a.remoteHooks(),
		a.browserExtHooks(),
		a.windowHooks(),
		a.playlistHooks(),
		a.savedHooks(),
		a.signinHooks(),
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
	for _, h := range a.areas() {
		if h.ready != nil {
			h.ready()
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
	// SessionEnding is a quit the app cannot hold back and must not ask about (quitHeard).
	SessionEnding bool
	prevented     bool
}

// PreventDefault holds the quit back; the area that did quits again itself once it may.
func (e *quitEvent) PreventDefault() {
	if !e.SessionEnding {
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

// quitHeard runs as the app quits however it does (core.Scope.OnQuit). A quit requestQuit let through has told the areas already; any other, as the Dock's Quit, the system's at a logout or the updater's relaunch, tells them now with SessionEnding set, as the app is ending and cannot ask.
func (a *app) quitHeard(context.Context) {
	// TODO(nsgo#86): hold back the Dock's Quit, a logout, restart or shutdown and the updater's relaunch so a running job can ask first, as MyGo's OnBeforeQuit let the queue do; NSGo's OnQuit cannot keep the app running, so only ⌘Q and the app menu's Quit, which are the app's own items, ask https://github.com/nsgo-dev/nsgo/issues/86
	if a.window.quitting {
		return
	}
	a.areasBeforeQuit(&quitEvent{SessionEnding: true})
}
