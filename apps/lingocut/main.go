package main

import (
	"context"
	"log"
	"os"
	"path/filepath"
	"runtime"

	"lingocut/apps/lingocut/brand"
	"lingocut/apps/lingocut/cloud"
	"lingocut/apps/lingocut/shelf"
	"lingocut/packages/core"
	"lingocut/packages/media"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/yldm-tech/nodra"
)

type app struct {
	source           string
	language         string
	translatePrompt  string
	translateContext bool
	status           string
	selectedID       string
	saveErr          string
	page             string
	openKey          string
	store            *nodra.Store[session]
	libraryWatch     chan struct{}
	settingsPath     string
	modelsDir        string
	extraModelDirs   []string
	downloading      string
	downloadWritten  int64
	downloadTotal    int64
	downloadErr      string
	downloadCancel   context.CancelFunc
	llmProvider      string
	llmBase          string
	llmModel         string
	llmKey           string
	showLLMKey       bool
	llmCredential    func(context.Context, bool) (media.Endpoint, error)
	testingLLM       bool
	llmTestResult    string
	toolVersions     map[string]string
	versionGen       int
	posterDir        string
	posters          map[string]ui.ImageSource
	posterMiss       map[string]bool
	posterBusy       map[string]bool
	googleEmail      string
	googleRefresh    string
	googleBusy       bool
	googleErr        string
	googleLogin      func(context.Context, cloud.GoogleConfig) (cloud.Account, error)
	storage          string
	r2Endpoint       string
	r2Bucket         string
	r2AccessKey      string
	r2Secret         string
	s3Region         string
	s3Bucket         string
	s3AccessKey      string
	s3Secret         string
	ossRegion        string
	ossBucket        string
	ossAccessKey     string
	ossSecret        string
	cosRegion        string
	cosBucket        string
	cosAccessKey     string
	cosSecret        string
	compatEndpoint   string
	compatRegion     string
	compatBucket     string
	compatAccessKey  string
	compatSecret     string
	compatPath       bool
	showR2Secret     bool
	uploadPath       string
	uploadWritten    int64
	uploadTotal      int64
	uploadErr        string
	uploadErrPath    string
	uploaded         map[string]string
	uploader         func(context.Context, cloud.Store, []cloud.File, func(written, total int64)) error
	removed          map[string]bool
	job              *core.Job
	library          shelf.Library
	pipeline         media.Pipeline
	win              *mygo.Window
	lib              libraryUI
	player           playerUI
	overlay          overlayUI
	// update runs fn on the UI thread and redraws; background jobs report through it.
	update func(fn func())
	cancel context.CancelFunc
}

func newApp(pipeline media.Pipeline, update func(fn func())) *app {
	return &app{
		language: "简体中文", translatePrompt: media.DefaultSubtitlePrompt, translateContext: true,
		lib:      newLibraryUI(),
		player:   newPlayerUI(),
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
	a.modelsDir = filepath.Join(filepath.Dir(a.settingsPath), "models")
	a.extraModelDirs = defaultExtraModelDirs()
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
	a.llmCredential = credential
	if saved.Language != "" {
		a.language = saved.Language
	}
	if saved.TranslatePrompt != "" {
		a.translatePrompt = saved.TranslatePrompt
	}
	if saved.TranslateContext != nil {
		a.translateContext = *saved.TranslateContext
	}
	applyCloudEnv(a, saved)
	a.restorePlayer(saved)
	a.overlay.onboarded = saved.Onboarded
	if cache, err := os.UserCacheDir(); err == nil {
		a.posterDir = filepath.Join(cache, "LingoCut", "posters")
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
