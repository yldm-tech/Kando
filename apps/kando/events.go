package main

import "github.com/egoist/mygo"

// areaHooks is what an area does at the moments every area shares. Each area returns its hooks from a function in its own file (sidebarHooks in state_sidebar.go, libraryHooks in state_library.go, queueHooks in queue.go, settingsHooks in state_settings.go, cloudHooks in state_cloud.go, playerHooks in state_player.go, overlayHooks in state_overlay.go, menuHooks in state_menu.go, feedbackHooks in state_feedback.go, onboardingHooks in state_onboarding.go, kitsHooks in state_kits.go, remoteHooks in state_remote.go, updatesHooks in state_updates.go, windowHooks in state_window.go, logsHooks in state_logs.go, browseHooks in state_browse.go), as each area returns its rows of the shortcut table. A nil field means the area does nothing then. An area that needs another moment sets the field in its own hooks, not a call where the moment happens.
//
// Startup runs the moments in this order: restore, shelfLoaded, shelfChanged (main's first refreshLibrary), then ready. Everything before ready runs while there is no window, and main's update posts to the window, so a hook must not call update, or start background work that reports through it, before ready; main's update would panic on the missing window. An area whose shelfChanged starts such work holds it back until its ready, with a flag of its own that its restore sets (main runs restore before App.Run; tests that call restoreSettings run it too, so what restore registers on the process-wide mygo.App, as the queue's listener for notification clicks, is registered again by each such test).
type areaHooks struct {
	// save writes the area's part of settings.json, its own struct embedded in savedSettings. restore takes it back at startup (restoreSettings), after the shelf and before the window exists.
	save    func(saved *savedSettings)
	restore func(saved savedSettings)
	// ready runs once the main window exists, so background work that reports through update can start. It runs once, in main only.
	ready func()
	// afterUpdate runs after each report of background work has run on the UI thread (app.update). Only main's update runs it; a test's update does not.
	afterUpdate func()
	// afterFrame runs at the end of each frame of the main window, after the overlays are drawn.
	afterFrame func()
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
	// beforeQuit runs when a quit starts (main's mygo.App.OnBeforeQuit), before the windows close: ⌘Q, the menu's Quit, the updater's relaunch, a signal, or on Linux and Windows the main window's close button. An area that must not let the app quit yet, such as the queue while a job runs, prevents e and asks first; it quits again itself once the user agrees. A quit a signal asked for goes ahead without asking (queueWatchSignals).
	beforeQuit func(e *mygo.QuitEvent)
	// quitStarting runs once no area's beforeQuit prevented the quit, right before the windows close, so an area that keeps a window from closing (state_window.go) lets it close.
	quitStarting func()
	// windowShown runs when the main window was put away by its close button (shown false) or came back (state_window.go). A window out of sight draws no frame, so an area that brings something up to date after each frame, as the menu bar does, does it here too.
	windowShown func(shown bool)
	// becameActive runs when the app becomes the active one, the one in front (main's mygo.App.OnDidBecomeActive), as when the user clicks its window or one of its notifications.
	becameActive func()
	// quitting runs once when the app quits (main's mygo.App.OnWillQuit), after its windows have closed and before the process ends, so an area can stop what would outlive it, such as a player's sound in a process of its own, or the queue's running job.
	quitting func()
}

// areas is each area's hooks, in the order they run.
func (a *app) areas() []areaHooks {
	return []areaHooks{
		a.logsHooks(),
		a.sidebarHooks(),
		a.libraryHooks(),
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
		a.updatesHooks(),
		a.windowHooks(),
		a.browseHooks(),
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

func (a *app) areasAfterFrame() {
	for _, h := range a.areas() {
		if h.afterFrame != nil {
			h.afterFrame()
		}
	}
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
func (a *app) areasBeforeQuit(e *mygo.QuitEvent) {
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
