package main

import (
	"log"
	"os"
	"path/filepath"
	"runtime"

	"kando/apps/kando/brand"
	"kando/apps/kando/locales"
	"kando/apps/kando/shelf"
	"kando/packages/media"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/yldm-tech/nodra"
)

// app is the window's state. The fields here are the ones every area shares: the shelf, the pipeline, the window and which page shows. Each area keeps its own state in a struct of its own, declared in that area's file; an area adds a field to its struct there, not to app.
type app struct {
	saveErr      string
	page         string
	openKey      string
	store        *nodra.Store[session]
	libraryWatch chan struct{}
	settingsPath string
	// settingsFound is what loadSettings found at settingsPath at startup (settings_file.go).
	settingsFound settingsFound
	library       shelf.Library
	pipeline      media.Pipeline
	win           *mygo.Window
	// update runs fn on the UI thread and redraws; background jobs report through it.
	update func(fn func())

	side     sidebarUI  // state_sidebar.go
	lib      libraryUI  // state_library.go
	queue    jobQueue   // queue.go
	player   playerUI   // state_player.go
	prefs    settingsUI // state_settings.go
	cloud    cloudUI    // state_cloud.go
	overlay  overlayUI  // state_overlay.go
	menu     menuUI     // state_menu.go
	feedback feedbackUI // state_feedback.go
	palette  paletteUI  // state_palette.go
	onb      onboardUI  // state_onboarding.go
	sites    sitesUI    // state_sites.go
	kits     kitsUI     // state_kits.go
}

func newApp(pipeline media.Pipeline, update func(fn func())) *app {
	return &app{
		store:    newSessionStore(),
		pipeline: pipeline, update: update,
		// One line per area; an area sets up its state in its own constructor.
		side:     newSidebarUI(),
		lib:      newLibraryUI(),
		queue:    newJobQueue(),
		player:   newPlayerUI(),
		prefs:    newSettingsUI(),
		cloud:    newCloudUI(),
		overlay:  newOverlayUI(),
		menu:     newMenuUI(),
		feedback: newFeedbackUI(),
		palette:  newPaletteUI(),
		onb:      newOnboardUI(),
		sites:    newSitesUI(),
		kits:     newKitsUI(),
	}
}

func main() {
	var win *mygo.Window
	var a *app
	// The programs Kando downloads for itself go beside its settings, and are looked up before those on PATH.
	media.OwnToolsDir = filepath.Join(filepath.Dir(shelf.DefaultPath()), "tools")
	a = newApp(media.ToolsFromEnv(), func(fn func()) {
		win.Update(func() {
			fn()
			a.areasAfterUpdate()
		})
	})
	defer func() { _ = a.store.Close() }()
	a.settingsPath = filepath.Join(filepath.Dir(shelf.DefaultPath()), "settings.json")
	a.prefs.modelsDir = filepath.Join(filepath.Dir(a.settingsPath), "models")
	a.prefs.extraModelDirs = defaultExtraModelDirs()
	a.restoreSettings(a.loadSettings())
	if cache, err := os.UserCacheDir(); err == nil {
		a.lib.posterDir = filepath.Join(cache, "Kando", "posters")
	}
	a.openLibrary(shelf.DefaultPath())
	a.migrateLibrary()
	for _, dir := range a.knownLibraries() {
		brand.Ensure(dir, appIconPNG)
	}
	a.refreshLibrary()
	a.restartLibraryWatch()
	mygo.Theme.SetSource(mygo.ThemeLight)
	mygo.App.WhenReady(func() {
		win = mygo.NewWindow(mygo.WindowOptions{
			Title:                locales.LibraryWindowTitle(),
			Width:                1100,
			Height:               720,
			MinWidth:             900,
			MinHeight:            560,
			StateKey:             "main",
			TitleBarStyle:        mygo.TitleBarHidden,
			TrafficLightPosition: trafficLights(),
			Transparent:          runtime.GOOS == "darwin",
			Vibrancy:             sidebarMaterial(),
			Content:              ui.View(a.view),
		})
		a.win = win
		// The window's title is set once; it follows the interface's language when Settings › 通用 changes it.
		mygo.App.OnLocaleChanged(func(string) { win.SetTitle(locales.LibraryWindowTitle()) })
		nodra.Invalidate(a.store, win)
		a.areasReady()
	})
	mygo.App.OnWillQuit(func(*mygo.QuitEvent) { a.areasQuitting() })
	mygo.App.OnDidResignActive(func() { a.playerDidResignActive() })
	mygo.App.OnDidBecomeActive(func() { a.playerDidBecomeActive() })
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
