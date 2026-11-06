package main

import (
	"log"
	"os"
	"path/filepath"
	"runtime"

	"lingocut/apps/lingocut/brand"
	"lingocut/apps/lingocut/shelf"
	"lingocut/packages/media"

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
	library      shelf.Library
	pipeline     media.Pipeline
	win          *mygo.Window
	// update runs fn on the UI thread and redraws; background jobs report through it.
	update func(fn func())

	side    sidebarUI  // state_sidebar.go
	lib     libraryUI  // state_library.go
	queue   jobQueue   // queue.go
	player  playerUI   // state_player.go
	prefs   settingsUI // state_settings.go
	cloud   cloudUI    // state_cloud.go
	overlay overlayUI  // state_overlay.go
	menu    menuUI     // state_menu.go
}

func newApp(pipeline media.Pipeline, update func(fn func())) *app {
	return &app{
		side:     newSidebarUI(),
		lib:      newLibraryUI(),
		player:   newPlayerUI(),
		prefs:    newSettingsUI(),
		store:    newSessionStore(),
		pipeline: pipeline, update: update,
	}
}

func main() {
	var win *mygo.Window
	tools := media.ToolsFromEnv()
	a := newApp(tools, func(fn func()) { win.Update(fn) })
	defer func() { _ = a.store.Close() }()
	a.settingsPath = filepath.Join(filepath.Dir(shelf.DefaultPath()), "settings.json")
	a.prefs.modelsDir = filepath.Join(filepath.Dir(a.settingsPath), "models")
	a.prefs.extraModelDirs = defaultExtraModelDirs()
	saved := loadSavedSettings(a.settingsPath)
	libs := cleanLibraries(saved.Libraries)
	if saved.LibraryDir != "" {
		libs = addLibrary(libs, saved.LibraryDir)
	}
	libraryDir := ""
	if env := os.Getenv("LINGOCUT_OUTPUT_DIR"); env != "" {
		libraryDir = env
	} else if saved.LibraryDir != "" {
		libraryDir = saved.LibraryDir
	} else if len(libs) > 0 {
		libraryDir = libs[0]
	}
	a.edit(func(s *session) {
		s.Libraries = libs
		s.LibraryDir = libraryDir
	})
	a.restoreLibraryInodes(saved.LibraryInodes)
	if a.reconcileMissingLibraries() {
		libraryDir = a.snap().LibraryDir
		if err := a.saveSettings(); err != nil {
			a.saveErr = "资料库无法保存：" + err.Error()
		}
	}
	tools.OutputDir = libraryDir
	a.pipeline = tools
	if os.Getenv("LINGOCUT_WHISPER_MODEL") == "" && saved.WhisperModel != "" {
		if info, err := os.Stat(saved.WhisperModel); err == nil && !info.IsDir() && info.Size() > 0 {
			tools.WhisperModel = saved.WhisperModel
			a.pipeline = tools
		}
	}
	if os.Getenv("LINGOCUT_LLM_BASE_URL") == "" && saved.LLMBaseURL != "" {
		tools.Translator.BaseURL = saved.LLMBaseURL
	}
	if os.Getenv("LINGOCUT_LLM_MODEL") == "" && saved.LLMModel != "" {
		tools.Translator.Model = saved.LLMModel
	}
	if os.Getenv("LINGOCUT_LLM_API_KEY") == "" && os.Getenv("EVERYAPI_API_KEY") == "" && saved.LLMAPIKey != "" {
		tools.Translator.APIKey = saved.LLMAPIKey
	}
	credential := tools.Translator.Credential
	if tools.Translator.BaseURL != media.EveryAPIBaseURL || tools.Translator.APIKey != "" {
		tools.Translator.Credential = nil
	}
	a.syncLLM(tools)
	a.prefs.llmCredential = credential
	if saved.Language != "" {
		a.prefs.language = saved.Language
	}
	if saved.TranslatePrompt != "" {
		a.prefs.translatePrompt = saved.TranslatePrompt
	}
	if saved.TranslateContext != nil {
		a.prefs.translateContext = *saved.TranslateContext
	}
	applyCloudEnv(a, saved)
	a.restorePlayer(saved)
	a.overlay.onboarded = saved.Onboarded
	if cache, err := os.UserCacheDir(); err == nil {
		a.lib.posterDir = filepath.Join(cache, "LingoCut", "posters")
	}
	a.openLibrary(shelf.DefaultPath())
	if os.Getenv("LINGOCUT_OUTPUT_DIR") == "" && filepath.Clean(a.snap().LibraryDir) == filepath.Clean(media.VimpoLibraryDir()) {
		bundle := media.DefaultLibraryDir()
		moved, err := media.DetachOwnVideos(a.snap().LibraryDir, bundle)
		if a.retargetLibrary(moved) {
			if saveErr := a.library.Save(); saveErr != nil && err == nil {
				err = saveErr
			}
		}
		if err != nil {
			a.saveErr = "资料库无法搬出：" + err.Error()
		} else {
			libs := dropLibrary(a.snap().Libraries, media.VimpoLibraryDir())
			libs = addLibrary(libs, bundle)
			a.edit(func(s *session) {
				s.Libraries = libs
				s.LibraryDir = bundle
			})
			tools.OutputDir = bundle
			a.pipeline = tools
			if saveErr := a.saveSettings(); saveErr != nil {
				a.saveErr = "资料库无法保存：" + saveErr.Error()
			}
		}
	}
	if os.Getenv("LINGOCUT_OUTPUT_DIR") == "" && filepath.Clean(a.snap().LibraryDir) == filepath.Clean(media.LegacyLibraryDir()) {
		from := a.snap().LibraryDir
		bundle := media.DefaultLibraryDir()
		moved, err := media.AdoptLibrary(from, bundle)
		if a.retargetLibrary(moved) {
			if saveErr := a.library.Save(); saveErr != nil && err == nil {
				err = saveErr
			}
		}
		if err != nil {
			a.saveErr = "影视库无法整理：" + err.Error()
		} else {
			media.RemoveEmptyLibrary(from)
			libs := addLibrary(a.snap().Libraries, bundle)
			a.edit(func(s *session) {
				s.Libraries = libs
				s.LibraryDir = bundle
			})
			tools.OutputDir = bundle
			a.pipeline = tools
			if saveErr := a.saveSettings(); saveErr != nil {
				a.saveErr = "资料库无法保存：" + saveErr.Error()
			}
		}
	}
	for _, dir := range a.knownLibraries() {
		brand.Ensure(dir, appIconPNG)
	}
	a.refreshLibrary()
	a.restartLibraryWatch()
	mygo.Theme.SetSource(mygo.ThemeLight)
	mygo.App.WhenReady(func() {
		// The sidebar's material: AppKit's source list on macOS and Mica on Windows, whose NavigationView pane is transparent over it, as Onevra's shell chose.
		vibrancy := mygo.VibrancyNone
		switch runtime.GOOS {
		case "darwin":
			vibrancy = mygo.VibrancySidebar
		case "windows":
			vibrancy = mygo.VibrancyMica
		}
		win = mygo.NewWindow(mygo.WindowOptions{
			Title:                "LingoCut",
			Width:                1100,
			Height:               720,
			MinWidth:             900,
			MinHeight:            560,
			StateKey:             "main",
			TitleBarStyle:        mygo.TitleBarHidden,
			TrafficLightPosition: &mygo.Point{X: 16, Y: 16},
			Transparent:          runtime.GOOS == "darwin",
			Vibrancy:             vibrancy,
			Content:              ui.View(a.view),
		})
		a.win = win
		nodra.Invalidate(a.store, win)
		a.loadToolVersions()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
