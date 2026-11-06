package main

// areaHooks is what an area does at the moments every area shares. Each area returns its hooks from a function in its own file (sidebarHooks in state_sidebar.go, libraryHooks in state_library.go, queueHooks in queue.go, settingsHooks in state_settings.go, cloudHooks in state_cloud.go, playerHooks in state_player.go, overlayHooks in state_overlay.go, menuHooks in state_menu.go), as each area returns its rows of the shortcut table. A nil field means the area does nothing then. An area that needs another moment sets the field in its own hooks, not a call where the moment happens.
type areaHooks struct {
	// save writes the area's part of settings.json, its own struct embedded in savedSettings. restore takes it back at startup, after the shelf and before the window exists.
	save    func(saved *savedSettings)
	restore func(saved savedSettings)
	// ready runs once the main window exists, so background work that reports through update can start.
	ready func()
	// afterUpdate runs after each report of background work has run on the UI thread (app.update). Only main's update runs it; a test's update does not.
	afterUpdate func()
	// afterFrame runs at the end of each frame of the main window, after the overlays are drawn.
	afterFrame func()
	// leavingLibrary runs before applyLibrary opens another library; libraryChanged runs when applyLibrary returns, on every path.
	leavingLibrary func()
	libraryChanged func()
	// shelfChanged runs after the shelf's films may have changed: refreshLibrary read the library folder again, or show recorded a job's progress.
	shelfChanged func()
	// toolsChanged runs after the pipeline's tools changed: another speech model was chosen, or the tools were detected again (redetected).
	toolsChanged func(redetected bool)
	// llmChanged runs after the AI service's settings were applied to the pipeline.
	llmChanged func()
}

// areas is each area's hooks, in the order they run.
func (a *app) areas() []areaHooks {
	return []areaHooks{
		a.sidebarHooks(),
		a.libraryHooks(),
		a.queueHooks(),
		a.settingsHooks(),
		a.cloudHooks(),
		a.playerHooks(),
		a.overlayHooks(),
		a.menuHooks(),
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
