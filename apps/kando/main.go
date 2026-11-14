package main

import (
	"cmp"
	"os"
	"path/filepath"

	"kando/apps/kando/brand"
	"kando/apps/kando/cli"
	"kando/apps/kando/live"
	"kando/apps/kando/shelf"
	"kando/apps/kando/theme"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/desktop"
	"github.com/yldm-tech/nodra"
)

// app is the window's state. The fields here are the ones every area shares: the shelf, the pipeline, the app's scope and revision and which page shows. Each area keeps its own state in a struct of its own, declared in that area's file; an area adds a field to its struct there, not to app.
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
	// rev is the revision every binding reads the app's state through, which whatever changes the state bumps (package live, state.go).
	rev live.Rev
	// cx is the app's own scope once nsgo.Run started the app (run), nil before and in tests that start none: the services, the windows, the menu bar and the quit go through it. Code in a component uses the component's own scope instead, which knows the window it is in.
	cx *core.Scope
	// update runs fn on the engine's goroutine and lets the bindings follow what it changed; background jobs report through it, from any goroutine.
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
	remote   remoteUI   // state_remote.go
	cliTool  cliToolUI  // state_cli.go
	updates  updatesUI  // state_updates.go
	window   windowUI   // state_window.go
	logs     logsUI     // state_logs.go
	browse   browseUI   // state_browse.go
}

// newApp makes the app's state. update is how background jobs report; nil reports through the revision (appUpdate), as main and the tests that start the app with nsgotest do, while a test that runs no app passes its own to apply the reports itself.
func newApp(pipeline media.Pipeline, update func(fn func())) *app {
	a := &app{
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
		remote:   newRemoteUI(),
		cliTool:  newCLIToolUI(),
		updates:  newUpdatesUI(),
		window:   newWindowUI(),
		logs:     newLogsUI(),
		browse:   newBrowseUI(),
	}
	if a.update == nil {
		a.update = a.appUpdate
	}
	return a
}

func main() {
	appVersion = cmp.Or(core.App().Version, appVersion)
	// Started with one of the kando command line's commands (cli), it runs that command against the running app, or serves MCP, and opens no window.
	if code, ok := cli.Main(os.Args[1:], appVersion); ok {
		os.Exit(code)
	}
	// One copy of Kando runs per user, as two would both write library.json and settings.json. On macOS LaunchServices runs one instance of the bundle and a second launch reopens the first (run's OnReopen), which brings its window forward.
	// TODO(nsgo#31): hand a second launch on Windows and Linux over to the running copy and end it, as MyGo's RequestSingleInstanceLock and OnSecondInstance did; a copy started with KANDO_SOCKET stays a second copy on purpose https://github.com/nsgo-dev/nsgo/issues/31
	// The programs Kando downloads for itself go beside its settings, and are looked up before those on PATH.
	media.OwnToolsDir = filepath.Join(filepath.Dir(shelf.DefaultPath()), "tools")
	a := newApp(media.ToolsFromEnv(), nil)
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
	useUpdater()
	opts := []nsgo.Option{
		nsgo.WindowSize(1100, 720),
		nsgo.WindowConstraints(core.WindowConstraints{MinWidth: 900, MinHeight: 560}),
		nsgo.Appearance(theme.Appearance),
		nsgo.WindowMaterial(sidebarMaterial()),
	}
	if f, ok := a.windowFrame(); ok {
		opts = append(opts, nsgo.WindowFrame(f))
	}
	nsgo.Run(a.run, opts...)
	// The windows have closed and the engine has stopped: what would outlive the app stops here, where waiting for it holds up no thread the system watches, as the running job's programs (queueQuit) do for up to two seconds.
	a.areasQuitting()
}

// run is the app's function: it runs once, in the app's own scope, once the engine has started. It gives the revision its graph, which runs the reports held until now, follows the nodra store, listens to the app's lifecycle, opens the main window and installs the menu bar, then tells the areas the window exists.
func (a *app) run(app *core.Scope) {
	a.attach(app)
	// A click on the Dock icon and a second launch reopen the app: the main window comes back, from wherever it was put away.
	desktop.OnReopen(app, live.Do(a, a.remoteRaise))
	desktop.OnOpenFiles(app, live.On(a, func(paths []string) {
		for _, path := range paths {
			a.libraryOpenFile(path)
		}
	}))
	app.OnQuit(a.quitHeard)
	a.watchLifecycle(app)
	nsgo.SetMainView(app, a.view)
	a.installMenu(app)
	a.areasReady()
	a.Changed()
}

// attach makes app the app's scope: the revision becomes a signal of its graph, which runs the reports held until now, and follows the nodra store from then on. run calls it first, and the tests' testView.
func (a *app) attach(app *core.Scope) {
	a.cx = app
	a.rev.Attach(app)
	app.Cleanup(nodra.InvalidateTarget(a.store, &a.rev))
}

// watchLifecycle tells the player and the areas when the app becomes the active one, in front, and when it stops being it.
func (a *app) watchLifecycle(app *core.Scope) {
	active := core.LifecycleActive
	app.Effect(func() {
		now := app.Env().Lifecycle()
		if now == active {
			return
		}
		was := active
		active = now
		core.Untracked(app, func() bool {
			switch {
			case now == core.LifecycleActive:
				a.playerDidBecomeActive()
				a.areasBecameActive()
			case was == core.LifecycleActive:
				a.playerDidResignActive()
			}
			a.Changed()
			return true
		})
	})
}
