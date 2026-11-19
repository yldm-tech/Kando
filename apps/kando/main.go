package main

import (
	"cmp"
	"os"
	"path/filepath"

	"kando/apps/kando/brand"
	"kando/apps/kando/cli"
	"kando/apps/kando/desktop"
	"kando/apps/kando/live"
	"kando/apps/kando/shelf"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo"
	"github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
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
	// settingsFound is what loadSettings found at settingsPath at startup (settings_file.go). settingsUnsaved is set while settings.json is one the app could neither read nor copy aside, which only a save the user asks for writes over; settingsKept is the file as the app last read or wrote it, so a quit saves only what changed (saveSettingsChanged).
	settingsFound   settingsFound
	settingsUnsaved bool
	settingsKept    []byte
	library         shelf.Library
	pipeline        media.Pipeline
	// rev is the revision every binding reads the app's state through, which whatever changes the state bumps (package live, state.go).
	rev live.Rev
	// cx is the app's own scope once nsgo.Run started the app (run), nil before and in tests that start none: the services, the windows, the menu bar and the quit go through it. Code in a component uses the component's own scope instead, which knows the window it is in.
	cx *core.Scope
	// update runs fn on the engine's goroutine and lets the bindings follow what it changed; background jobs report through it, from any goroutine.
	update func(fn func())

	side     sidebarUI   // state_sidebar.go
	lib      libraryUI   // state_library.go
	queue    jobQueue    // queue.go
	player   playerUI    // state_player.go
	correct  correctUI   // state_correct.go
	gloss    glossaryUI  // state_glossary.go
	review   reviewUI    // state_review.go
	prefs    settingsUI  // state_settings.go
	cloud    cloudUI     // state_cloud.go
	overlay  overlayUI   // state_overlay.go
	menu     menuUI      // state_menu.go
	feedback feedbackUI  // state_feedback.go
	palette  paletteUI   // state_palette.go
	onb      onboardUI   // state_onboarding.go
	sites    sitesUI     // state_sites.go
	kits     kitsUI      // state_kits.go
	tools    toolsUI     // state_tools.go
	remote   remoteUI    // state_remote.go
	cliTool  cliToolUI   // state_cli.go
	ext      extUI       // state_browserext.go
	updates  updatesUI   // state_updates.go
	window   windowUI    // state_window.go
	logs     logsUI      // state_logs.go
	browse   browseUI    // state_browse.go
	playlist playlistUI  // state_playlist.go
	addsheet addSheetUI  // state_addsheet.go
	online   onlineUI    // state_online.go
	saved    savedUI     // state_saved.go
	signin   signinUI    // state_signin.go
	phone    phoneUI     // state_phonesync.go
	lock     lockUI      // state_lock.go
	tray     trayUI      // state_tray.go
	privacy  privacyUI   // state_privacy.go
	subs     subscribeUI // state_subscribe.go
	vocab    vocabUI     // state_vocab.go
	export   subexportUI // state_subexport.go
	explain  explainUI   // state_explain.go
	recap    recapUI     // state_recap.go
	plugins  pluginsUI   // state_plugins.go
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
		correct:  newCorrectUI(),
		gloss:    newGlossaryUI(),
		review:   newReviewUI(),
		prefs:    newSettingsUI(),
		cloud:    newCloudUI(),
		overlay:  newOverlayUI(),
		menu:     newMenuUI(),
		feedback: newFeedbackUI(),
		palette:  newPaletteUI(),
		onb:      newOnboardUI(),
		sites:    newSitesUI(),
		kits:     newKitsUI(),
		tools:    newToolsUI(),
		remote:   newRemoteUI(),
		cliTool:  newCLIToolUI(),
		ext:      newExtUI(),
		updates:  newUpdatesUI(),
		window:   newWindowUI(),
		logs:     newLogsUI(),
		browse:   newBrowseUI(),
		playlist: newPlaylistUI(),
		addsheet: addSheetUI{},
		saved:    newSavedUI(),
		signin:   signinUI{},
		phone:    newPhoneUI(),
		lock:     newLockUI(),
		tray:     newTrayUI(),
		privacy:  newPrivacyUI(),
		subs:     newSubscribeUI(),
		vocab:    newVocabUI(),
		export:   newSubexportUI(),
		explain:  newExplainUI(),
		recap:    newRecapUI(),
		plugins:  newPluginsUI(),
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
	// One copy of Kando runs per user, as two would both write library.json and settings.json. A second launch reopens the first (run's OnReopen), which brings its window forward, and hands it the files it names (run's OnOpenFiles): on macOS LaunchServices runs one instance of the bundle, on Windows and Linux nsgo.Run hands the launch to the running copy and ends. A copy started with KANDO_SOCKET, as tests and the command line's second copy start it, runs beside the user's own: on macOS through open -n (remote.launchCommand), on Windows and Linux under an instance lock of its socket's own (nsgo.InstanceKey).
	// The programs Kando downloads for itself go beside its settings, and are looked up before those on PATH.
	media.OwnToolsDir = filepath.Join(filepath.Dir(shelf.DefaultPath()), "tools")
	// A new library goes in the videos folder the system names, as Windows' when it was moved, rather than always in ~/Videos.
	media.VideosDir = desktop.VideosDir()
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
	opts := []nsgo.Option{
		nsgo.WindowSize(1100, 720),
		nsgo.WindowConstraints(core.WindowConstraints{MinWidth: 900, MinHeight: 560}),
		nsgo.Appearance(a.settingsAppearance()),
		nsgo.WindowMaterial(sidebarMaterial()),
	}
	if socket := os.Getenv("KANDO_SOCKET"); socket != "" {
		opts = append(opts, nsgo.InstanceKey(socket))
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
	nsdesktop.OnReopen(app, live.Do(a, a.remoteRaise))
	nsdesktop.OnOpenFiles(app, live.On(a, func(paths []string) {
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

// watchLifecycle tells the areas when the app becomes the active one, in front (becameActive).
func (a *app) watchLifecycle(app *core.Scope) {
	active := core.LifecycleActive
	app.Effect(func() {
		now := app.Env().Lifecycle()
		if now == active {
			return
		}
		active = now
		// The areas hear about it under the app's scope (core.Scope.Run), not under the effect, whose next run would dispose of what they made. The player follows the lifecycle itself (playerAwakeLifecycle).
		if now == core.LifecycleActive {
			app.Run(a.areasBecameActive)
		}
		a.Changed()
	})
}
