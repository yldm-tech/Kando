package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"log"
	"net/url"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"time"

	"lingocut/packages/core"
	"lingocut/packages/media"

	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
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
	subs             subtitleView
	libraryDir       string
	libraryWatch     chan struct{}
	libraries        []string
	libraryName      string
	libraryParent    string
	makingLibrary    bool
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
	filmSide         string
	cue              cueLook
	toolVersions     map[string]string
	versionGen       int
	videos           []media.Video
	posterDir        string
	posters          map[string]ui.ImageSource
	posterMiss       map[string]bool
	posterBusy       map[string]bool
	play             *filmPlay
	playPath         string
	playErr          string
	playGen          int
	playFrom         time.Duration
	playJump         bool
	removed          map[string]bool
	job              *core.Job
	library          library
	pipeline         media.Pipeline
	win              *mygo.Window
	// update runs fn on the UI thread and redraws; background jobs report through it.
	update func(fn func())
	cancel context.CancelFunc
}

func (a *app) pickVideos() {
	win := a.win
	update := a.update
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:      win,
			Title:       "选择视频",
			ButtonLabel: "打开",
			Multiple:    true,
			Filters:     []mygo.FileFilter{{Name: "视频", Extensions: []string{"mp4", "m4v", "mov", "mkv", "webm", "avi", "flv"}}},
		})
		update(func() {
			if err != nil {
				a.status = err.Error()
				return
			}
			if len(paths) > 0 {
				a.importLocal(paths[0])
			}
		})
	}()
}

// importLocal points the next job at a video already on disk. The pipeline reads it directly and does not download it.
func (a *app) importLocal(path string) {
	if !media.IsVideoFile(path) {
		a.status = "没有可导入的视频"
		return
	}
	a.source = (&url.URL{Scheme: "file", Path: path}).String()
	a.status = "已选择本地视频"
	a.page = pageFilms
	a.openKey = ""
}

// startFilm processes a video that is already in the library and stays on it.
func (a *app) startFilm(item film) {
	a.importLocal(item.videoPath)
	if a.status != "已选择本地视频" {
		return
	}
	a.start()
	if a.job == nil {
		return
	}
	a.openKey = a.job.ID
	a.loadSubtitles(a.job.ID, a.job.SubtitlePath)
}

func (a *app) pickLibrary() {
	current := a.libraryDir
	if current == "" {
		current = a.libraryPlace()
	}
	a.pickFolder("导入资料库", "导入", current, func(dir string) {
		a.switchLibrary(dir)
		a.makingLibrary = false
		a.page = pageFilms
	})
}

func (a *app) pickLibraryParent() {
	a.pickFolder("资料库放在哪里", "选择", a.libraryPlace(), func(dir string) {
		a.libraryParent = dir
	})
}

func (a *app) pickFolder(title, button, current string, done func(string)) {
	win := a.win
	update := a.update
	go func() {
		paths, err := mygo.Dialog.Open(mygo.OpenDialogOptions{
			Parent:            win,
			Title:             title,
			ButtonLabel:       button,
			Directory:         true,
			CreateDirectories: true,
			DefaultPath:       current,
		})
		update(func() {
			if err != nil {
				a.saveErr = err.Error()
				return
			}
			if len(paths) == 0 {
				return
			}
			done(paths[0])
		})
	}()
}

// createLibrary makes a .lingocut bundle and switches to it. A folder that is already there is opened as it is.
func (a *app) createLibrary() {
	file, err := libraryFileName(a.libraryName)
	if err != nil {
		a.saveErr = err.Error()
		return
	}
	dir := filepath.Join(a.libraryPlace(), file)
	if info, statErr := os.Stat(dir); statErr == nil {
		if info.IsDir() {
			a.saveErr = "已经有这个资料库，换个名字"
		} else {
			a.saveErr = "这个位置已经有同名文件"
		}
		return
	} else if !os.IsNotExist(statErr) {
		a.saveErr = statErr.Error()
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		a.saveErr = err.Error()
		return
	}
	a.switchLibrary(dir)
	a.makingLibrary = false
	a.page = pageFilms
}

func libraryFileName(name string) (string, error) {
	name = strings.TrimSpace(name)
	name = strings.TrimSuffix(name, media.LibraryExt)
	name = strings.TrimSpace(name)
	if name == "" {
		return "", errors.New("先写资料库的名称")
	}
	if name == "." || name == ".." || strings.ContainsAny(name, `/\`) {
		return "", errors.New("资料库名称不能包含路径")
	}
	return name + media.LibraryExt, nil
}

// beginNewLibrary opens the create form even when a library is already open, and picks a name that is not taken.
func (a *app) beginNewLibrary() {
	a.openKey = ""
	a.stopPlayback()
	a.makingLibrary = true
	a.page = pageFilms
	a.libraryName = a.freeLibraryName()
	a.saveErr = ""
}

func (a *app) freeLibraryName() string {
	parent := a.libraryPlace()
	base := "影视库"
	if _, err := os.Stat(filepath.Join(parent, base+media.LibraryExt)); os.IsNotExist(err) {
		return base
	}
	for n := 2; n < 100; n++ {
		name := fmt.Sprintf("%s %d", base, n)
		if _, err := os.Stat(filepath.Join(parent, name+media.LibraryExt)); os.IsNotExist(err) {
			return name
		}
	}
	return base
}

func (a *app) libraryPlace() string {
	if strings.TrimSpace(a.libraryParent) != "" {
		return a.libraryParent
	}
	return media.MoviesDir()
}

// switchLibrary points new downloads at dir and stops a job that is still writing into the previous one.
func (a *app) switchLibrary(dir string) {
	dir = filepath.Clean(dir)
	if dir == "." || dir == "" {
		return
	}
	a.libraries = addLibrary(a.libraries, dir)
	if filepath.Clean(a.libraryDir) == dir {
		if err := a.saveSettings(); err != nil {
			a.saveErr = "资料库无法保存：" + err.Error()
		}
		return
	}
	a.applyLibrary(dir)
	if err := a.saveSettings(); err != nil {
		a.saveErr = "资料库无法保存：" + err.Error()
		return
	}
	a.saveErr = ""
}

// forgetLibrary drops dir from the list. The folder on disk stays.
func (a *app) forgetLibrary(dir string) {
	dir = filepath.Clean(dir)
	kept := make([]string, 0, len(a.libraries))
	for _, item := range a.libraries {
		if filepath.Clean(item) != dir {
			kept = append(kept, item)
		}
	}
	a.libraries = kept
	if filepath.Clean(a.libraryDir) != dir {
		if err := a.saveSettings(); err != nil {
			a.saveErr = "资料库无法保存：" + err.Error()
		}
		return
	}
	next := ""
	if len(a.libraries) > 0 {
		next = a.libraries[0]
	}
	a.applyLibrary(next)
	if err := a.saveSettings(); err != nil {
		a.saveErr = "资料库无法保存：" + err.Error()
		return
	}
	a.saveErr = ""
}

func (a *app) applyLibrary(dir string) {
	if a.running() {
		a.stop()
	}
	a.openKey = ""
	a.stopPlayback()
	if dir == "" {
		a.libraryDir = ""
		if tools, ok := a.pipeline.(media.Tools); ok {
			tools.OutputDir = ""
			a.pipeline = tools
		}
		a.videos = nil
		a.restartLibraryWatch()
		return
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		a.saveErr = err.Error()
		return
	}
	a.libraryDir = dir
	if tools, ok := a.pipeline.(media.Tools); ok {
		tools.OutputDir = dir
		a.pipeline = tools
	}
	ensureLibraryLogo(dir)
	a.refreshLibrary()
	a.restartLibraryWatch()
}

// restartLibraryWatch follows the current library folder. A new file, a deletion, or a rewrite refreshes the list on the UI thread.
func (a *app) restartLibraryWatch() {
	if a.libraryWatch != nil {
		close(a.libraryWatch)
		a.libraryWatch = nil
	}
	if a.libraryDir == "" || a.update == nil {
		return
	}
	stop := make(chan struct{})
	a.libraryWatch = stop
	dir := a.libraryDir
	update := a.update
	go media.WatchLibrary(dir, time.Second, stop, func() {
		update(func() {
			if a.libraryWatch != stop || filepath.Clean(a.libraryDir) != filepath.Clean(dir) {
				return
			}
			a.refreshLibrary()
			a.reloadOpenSubtitles()
		})
	})
}

func (a *app) knownLibraries() []string {
	out := append([]string{}, a.libraries...)
	if a.libraryDir != "" {
		out = addLibrary(out, a.libraryDir)
	}
	return out
}

func dropLibrary(libraries []string, dir string) []string {
	dir = filepath.Clean(dir)
	kept := make([]string, 0, len(libraries))
	for _, item := range libraries {
		if filepath.Clean(item) != dir {
			kept = append(kept, item)
		}
	}
	return kept
}

func addLibrary(libraries []string, dir string) []string {
	dir = filepath.Clean(strings.TrimSpace(dir))
	if dir == "" || dir == "." {
		return libraries
	}
	for _, item := range libraries {
		if filepath.Clean(item) == dir {
			return libraries
		}
	}
	return append(libraries, dir)
}

func (a *app) saveSettings() error {
	if a.settingsPath == "" {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(a.settingsPath), 0o700); err != nil {
		return err
	}
	contextOn := a.translateContext
	saved := savedSettings{
		LibraryDir:       a.libraryDir,
		Libraries:        append([]string{}, a.knownLibraries()...),
		LLMProvider:      a.llmProvider,
		Language:         a.language,
		TranslatePrompt:  a.translatePrompt,
		TranslateContext: &contextOn,
	}
	if tools, ok := a.pipeline.(media.Tools); ok {
		saved.WhisperModel = tools.WhisperModel
		saved.LLMBaseURL = tools.Translator.BaseURL
		saved.LLMModel = tools.Translator.Model
		saved.LLMAPIKey = tools.Translator.APIKey
	}
	data, err := json.MarshalIndent(saved, "", "  ")
	if err != nil {
		return err
	}
	data = append(data, '\n')
	tmp := a.settingsPath + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, a.settingsPath)
}

func loadSavedSettings(path string) savedSettings {
	data, err := os.ReadFile(path)
	if err != nil {
		return savedSettings{}
	}
	var saved savedSettings
	if json.Unmarshal(data, &saved) != nil {
		return savedSettings{}
	}
	saved.LibraryDir = strings.TrimSpace(saved.LibraryDir)
	saved.Libraries = cleanLibraries(saved.Libraries)
	saved.WhisperModel = strings.TrimSpace(saved.WhisperModel)
	saved.LLMBaseURL = strings.TrimSpace(saved.LLMBaseURL)
	saved.LLMModel = strings.TrimSpace(saved.LLMModel)
	saved.LLMAPIKey = strings.TrimSpace(saved.LLMAPIKey)
	saved.LLMProvider = strings.TrimSpace(saved.LLMProvider)
	saved.Language = strings.TrimSpace(saved.Language)
	saved.TranslatePrompt = strings.TrimSpace(saved.TranslatePrompt)
	return saved
}

type savedSettings struct {
	LibraryDir       string   `json:"libraryDir,omitempty"`
	Libraries        []string `json:"libraries,omitempty"`
	WhisperModel     string   `json:"whisperModel,omitempty"`
	LLMProvider      string   `json:"llmProvider,omitempty"`
	LLMBaseURL       string   `json:"llmBaseURL,omitempty"`
	LLMModel         string   `json:"llmModel,omitempty"`
	LLMAPIKey        string   `json:"llmAPIKey,omitempty"`
	Language         string   `json:"language,omitempty"`
	TranslatePrompt  string   `json:"translatePrompt,omitempty"`
	TranslateContext *bool    `json:"translateContext,omitempty"`
}

type llmPreset struct {
	ID, Name, Blurb, BaseURL, Model, Group string
}

func llmPresets() []llmPreset {
	return []llmPreset{
		{ID: "everyapi", Name: "EveryAPI", Blurb: "本机已登录的 EveryAPI", BaseURL: media.EveryAPIBaseURL, Model: media.EveryAPIModel, Group: "云服务"},
		{ID: "openai", Name: "OpenAI", Blurb: "OpenAI 官方接口", BaseURL: "https://api.openai.com/v1", Group: "云服务"},
		{ID: "openrouter", Name: "OpenRouter", Blurb: "一个接口调用多家模型", BaseURL: "https://openrouter.ai/api/v1", Group: "云服务"},
		{ID: "groq", Name: "Groq", Blurb: "Groq 的 OpenAI 兼容接口", BaseURL: "https://api.groq.com/openai/v1", Group: "云服务"},
		{ID: "deepseek", Name: "DeepSeek", Blurb: "DeepSeek 的 OpenAI 兼容接口", BaseURL: "https://api.deepseek.com/v1", Group: "云服务"},
		{ID: "xai", Name: "xAI", Blurb: "Grok 的 OpenAI 兼容接口", BaseURL: "https://api.x.ai/v1", Group: "云服务"},
		{ID: "mistral", Name: "Mistral", Blurb: "Mistral 的 OpenAI 兼容接口", BaseURL: "https://api.mistral.ai/v1", Group: "云服务"},
		{ID: "dashscope", Name: "通义千问", Blurb: "阿里云 DashScope 兼容接口", BaseURL: "https://dashscope.aliyuncs.com/compatible-mode/v1", Group: "云服务"},
		{ID: "moonshot", Name: "Kimi", Blurb: "Moonshot 的 OpenAI 兼容接口", BaseURL: "https://api.moonshot.cn/v1", Group: "云服务"},
		{ID: "zhipu", Name: "智谱 GLM", Blurb: "智谱的 OpenAI 兼容接口", BaseURL: "https://open.bigmodel.cn/api/paas/v4", Group: "云服务"},
		{ID: "siliconflow", Name: "硅基流动", Blurb: "硅基流动的 OpenAI 兼容接口", BaseURL: "https://api.siliconflow.cn/v1", Group: "云服务"},
		{ID: "ollama", Name: "Ollama", Blurb: "本机 Ollama", BaseURL: "http://127.0.0.1:11434/v1", Group: "本机"},
		{ID: "lmstudio", Name: "LM Studio", Blurb: "本机 LM Studio", BaseURL: "http://127.0.0.1:1234/v1", Group: "本机"},
		{ID: "custom", Name: "OpenAI 兼容", Blurb: "任何兼容 OpenAI Chat Completions 的接口", Group: "其他"},
	}
}

func llmPresetByID(id string) (llmPreset, bool) {
	for _, preset := range llmPresets() {
		if preset.ID == id {
			return preset, true
		}
	}
	return llmPreset{}, false
}

func llmProviderID(base string) string {
	base = strings.TrimRight(strings.TrimSpace(base), "/")
	for _, preset := range llmPresets() {
		if preset.ID == "custom" || preset.BaseURL == "" {
			continue
		}
		if strings.TrimRight(preset.BaseURL, "/") == base {
			return preset.ID
		}
	}
	return "custom"
}

func (a *app) syncLLM(tools media.Tools) {
	a.pipeline = tools
	a.llmBase = tools.Translator.BaseURL
	a.llmModel = tools.Translator.Model
	a.llmKey = tools.Translator.APIKey
	a.llmCredential = tools.Translator.Credential
	a.llmProvider = llmProviderID(tools.Translator.BaseURL)
}

func (a *app) selectLLM(id string) {
	if a.llmProvider == id {
		return
	}
	preset, ok := llmPresetByID(id)
	if !ok {
		return
	}
	a.llmProvider = id
	if preset.BaseURL != "" {
		a.llmBase = preset.BaseURL
	}
	if preset.Model != "" {
		a.llmModel = preset.Model
	}
	a.applyLLM()
}

func (a *app) applyLLM() {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return
	}
	tools.Translator.BaseURL = strings.TrimSpace(a.llmBase)
	tools.Translator.Model = strings.TrimSpace(a.llmModel)
	tools.Translator.APIKey = strings.TrimSpace(a.llmKey)
	tools.Translator.Credential = nil
	if tools.Translator.BaseURL == media.EveryAPIBaseURL && tools.Translator.APIKey == "" {
		tools.Translator.Credential = a.llmCredential
	}
	a.pipeline = tools
	if err := a.saveSettings(); err != nil {
		a.saveErr = "翻译服务无法保存：" + err.Error()
	}
}

func (a *app) testLLM() {
	tools, ok := a.pipeline.(media.Tools)
	if !ok || a.testingLLM {
		return
	}
	a.testingLLM = true
	a.llmTestResult = ""
	translator := tools.Translator
	update := a.update
	go func() {
		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
		defer cancel()
		err := translator.Ping(ctx)
		update(func() {
			a.testingLLM = false
			if err != nil {
				a.llmTestResult = err.Error()
				return
			}
			a.llmTestResult = "连接成功"
		})
	}()
}

func (a *app) speechSearchDirs() []string {
	dirs := []string{a.modelsDir}
	if tools, ok := a.pipeline.(media.Tools); ok && tools.WhisperModel != "" {
		dirs = append(dirs, filepath.Dir(tools.WhisperModel))
	}
	return append(dirs, a.extraModelDirs...)
}

func (a *app) useSpeechModel(path string) {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return
	}
	tools.WhisperModel = path
	a.pipeline = tools
	if err := a.saveSettings(); err != nil {
		a.saveErr = "模型无法保存：" + err.Error()
	}
}

func (a *app) downloadSpeechModel(id string) {
	model, ok := media.SpeechModelByID(id)
	if !ok || a.downloading != "" {
		return
	}
	if a.modelsDir == "" {
		a.downloadErr = "还没有模型目录"
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.downloading = id
	a.downloadWritten = 0
	a.downloadTotal = 0
	a.downloadErr = ""
	a.downloadCancel = cancel
	dir := a.modelsDir
	update := a.update
	go func() {
		var last time.Time
		path, err := media.DownloadSpeechModel(ctx, media.WhisperModelBaseURL, model.File, dir, func(written, total int64) {
			if time.Since(last) < 200*time.Millisecond && written != total {
				return
			}
			last = time.Now()
			update(func() {
				if a.downloading != id {
					return
				}
				a.downloadWritten = written
				a.downloadTotal = total
			})
		})
		update(func() {
			if a.downloading != id {
				return
			}
			a.downloading = ""
			a.downloadCancel = nil
			if err != nil {
				if !errors.Is(err, context.Canceled) {
					a.downloadErr = err.Error()
				}
				return
			}
			a.useSpeechModel(path)
		})
	}()
}

func (a *app) cancelDownload() {
	if a.downloadCancel != nil {
		a.downloadCancel()
	}
}

func (a *app) redetectTools() {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return
	}
	a.pipeline = tools.Relook()
	a.loadToolVersions()
}

// loadToolVersions asks each installed tool for its version off the UI thread.
func (a *app) loadToolVersions() {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return
	}
	paths := []string{tools.YTDLP, tools.FFmpeg, tools.Whisper}
	a.versionGen++
	gen := a.versionGen
	update := a.update
	go func() {
		versions := map[string]string{}
		for _, path := range paths {
			if path == "" {
				continue
			}
			if version := toolVersion(path); version != "" {
				versions[path] = version
			}
		}
		update(func() {
			if a.versionGen != gen {
				return
			}
			a.toolVersions = versions
		})
	}()
}

func toolVersion(path string) string {
	ctx, cancel := context.WithTimeout(context.Background(), 2*time.Second)
	defer cancel()
	args := []string{"--version"}
	if strings.Contains(strings.ToLower(filepath.Base(path)), "ffmpeg") {
		args = []string{"-version"}
	}
	out, err := exec.CommandContext(ctx, path, args...).CombinedOutput()
	if len(out) == 0 || (err != nil && ctx.Err() != nil) {
		return ""
	}
	line := strings.TrimSpace(string(out))
	if i := strings.IndexByte(line, '\n'); i >= 0 {
		line = strings.TrimSpace(line[:i])
	}
	return shortVersion(line)
}

func shortVersion(line string) string {
	fields := strings.Fields(line)
	if len(fields) >= 3 && strings.EqualFold(fields[0], "ffmpeg") && strings.EqualFold(fields[1], "version") {
		return fields[2]
	}
	lower := strings.ToLower(line)
	if i := strings.Index(lower, "version"); i >= 0 {
		rest := strings.Trim(line[i+len("version"):], " :\t")
		if parts := strings.Fields(rest); len(parts) > 0 {
			return strings.Trim(parts[0], ":")
		}
	}
	if len(line) > 48 {
		return line[:48]
	}
	return line
}

func (a *app) refreshLibrary() {
	if a.libraryDir == "" {
		return
	}
	moved, err := media.GatherVideos(a.libraryDir)
	if err != nil {
		a.saveErr = err.Error()
		return
	}
	if a.retargetLibrary(moved) || a.relocateMissing() || a.dropMissing() {
		if err := a.library.save(); err != nil {
			a.saveErr = "视频库无法保存：" + err.Error()
		}
	}
	videos, err := media.ListVideos(a.libraryDir)
	if err != nil {
		a.saveErr = err.Error()
		return
	}
	a.videos = videos
}

// retargetLibrary points jobs at files GatherVideos moved into per-video folders.
func (a *app) retargetLibrary(moved map[string]string) bool {
	if len(moved) == 0 {
		return false
	}
	changed := false
	for i, job := range a.library.jobs {
		if next, ok := moved[job.VideoPath]; ok {
			a.library.jobs[i].VideoPath = next
			changed = true
		}
		if next, ok := moved[job.SubtitlePath]; ok {
			a.library.jobs[i].SubtitlePath = next
			changed = true
		}
	}
	return changed
}

func (a *app) dropMissing() bool {
	kept := make([]core.Job, 0, len(a.library.jobs))
	changed := false
	for _, job := range a.library.jobs {
		if job.VideoPath != "" && job.Stage.Done() {
			if _, err := os.Stat(job.VideoPath); err != nil {
				changed = true
				if a.openKey == job.ID {
					a.openKey = ""
					a.stopPlayback()
				}
				continue
			}
		}
		kept = append(kept, job)
	}
	if changed {
		a.library.jobs = kept
	}
	return changed
}

func (a *app) relocateMissing() bool {
	changed := false
	for i, job := range a.library.jobs {
		if next, ok := media.Relocate(a.libraryDir, job.VideoPath); ok {
			a.library.jobs[i].VideoPath = next
			changed = true
		}
		if next, ok := media.Relocate(a.libraryDir, job.SubtitlePath); ok {
			a.library.jobs[i].SubtitlePath = next
			changed = true
		}
	}
	return changed
}

func cleanLibraries(dirs []string) []string {
	var out []string
	for _, dir := range dirs {
		out = addLibrary(out, dir)
	}
	return out
}

func newApp(pipeline media.Pipeline, update func(fn func())) *app {
	return &app{
		language: "简体中文", translatePrompt: media.DefaultSubtitlePrompt, translateContext: true,
		libraryName: "影视库",
		cue:         defaultCueLook(),
		pipeline:    pipeline, update: update,
	}
}

func (a *app) openLibrary(path string) {
	lib, err := loadLibrary(path)
	a.library = lib
	if err != nil {
		a.saveErr = "视频库无法读取：" + err.Error()
	}
}

func (a *app) running() bool {
	for _, job := range a.library.jobs {
		if job.Stage != "" && !job.Stage.Done() {
			return true
		}
	}
	return false
}

func (a *app) remove(id string) {
	if a.removed == nil {
		a.removed = map[string]bool{}
	}
	a.removed[id] = true
	kept := make([]core.Job, 0, len(a.library.jobs))
	for _, job := range a.library.jobs {
		if job.ID != id {
			kept = append(kept, job)
		}
	}
	a.library.jobs = kept
	if a.selectedID == id {
		a.newDraft()
	}
	if err := a.library.save(); err != nil {
		a.saveErr = "视频库无法保存：" + err.Error()
	}
}

func (a *app) retry(job core.Job) {
	if a.running() {
		return
	}
	job.Err = nil
	job.Stage = core.StageQueued
	if strings.TrimSpace(job.Language) == "" {
		job.Language = strings.TrimSpace(a.language)
	}
	if job.Language == "" {
		a.status = "请填写目标语言"
		return
	}
	a.page = pageFilms
	a.run(job)
}

func (a *app) newDraft() {
	a.selectedID = ""
	a.job = nil
	a.source = ""
	a.status = ""
}

func (a *app) start() {
	if a.running() {
		return
	}
	if strings.TrimSpace(a.libraryDir) == "" {
		a.job = nil
		a.status = "先创建或导入一个资料库"
		return
	}
	job, err := core.NewJob(a.source)
	if err != nil {
		a.job = nil
		a.status = err.Error()
		return
	}
	language := strings.TrimSpace(a.language)
	if language == "" {
		a.job = nil
		a.status = "请填写目标语言"
		return
	}
	job.Language = language
	a.run(job)
}

// run keeps job.ID, so a retry updates the card already on screen.
func (a *app) run(job core.Job) {
	if tools, ok := a.pipeline.(media.Tools); ok {
		tools.Translator.Prompt = a.translatePrompt
		tools.Translator.Context = a.translateContext
		a.pipeline = tools
	}
	if job.LibraryDir == "" {
		job.LibraryDir = a.libraryDir
	}
	a.selectedID = job.ID

	ctx, cancel := context.WithCancel(context.Background())
	a.cancel = cancel
	a.show(job)
	pipeline, update := a.pipeline, a.update
	go func() {
		defer cancel()
		media.Run(ctx, pipeline, job, job.Language, func(j core.Job) {
			update(func() { a.show(j) })
		})
	}()
}

func (a *app) stop() {
	if a.cancel != nil {
		a.cancel()
	}
}

func (a *app) show(job core.Job) {
	if a.removed[job.ID] {
		return
	}
	a.library.upsert(job)
	if err := a.library.save(); err != nil {
		a.saveErr = "视频库无法保存：" + err.Error()
	} else {
		a.saveErr = ""
	}
	if a.selectedID == job.ID {
		a.job = &job
		a.status = statusText(job)
	}
	if a.openKey == job.ID {
		a.loadSubtitles(job.ID, job.SubtitlePath)
	}
}

// subtitleView is the subtitle file open on the film page. list keeps the scroll position across frames.
type subtitleView struct {
	key   string
	path  string
	cues  []core.Segment
	err   string
	ready bool
	list  ui.ListState
	clips ui.ListState
	mod   time.Time
	size  int64
	seen  bool
}

// loadSubtitles reads path off the UI thread. An empty path means this film has no subtitle yet.
func (a *app) loadSubtitles(key, path string) {
	if a.subs.key == key && a.subs.path == path {
		return
	}
	a.subs = subtitleView{key: key, path: path}
	if path == "" {
		a.subs.ready = true
		return
	}
	update := a.update
	if update == nil {
		a.subs.err = "字幕无法读取"
		a.subs.ready = true
		return
	}
	go func() {
		data, err := os.ReadFile(path)
		info, statErr := os.Stat(path)
		var cues []core.Segment
		msg := ""
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				msg = "字幕文件不在了"
			} else {
				msg = "字幕无法读取"
			}
		} else if cues, err = media.ParseSRT(data); err != nil {
			msg = "字幕无法读取"
			cues = nil
		}
		update(func() {
			if a.subs.key != key || a.subs.path != path {
				return
			}
			a.subs.cues = cues
			a.subs.err = msg
			a.subs.ready = true
			if statErr == nil {
				a.subs.mod = info.ModTime()
				a.subs.size = info.Size()
				a.subs.seen = true
			}
		})
	}()
}

// reloadOpenSubtitles reads the open subtitle again when the file on disk was rewritten or removed.
func (a *app) reloadOpenSubtitles() {
	if a.subs.key == "" || a.subs.path == "" {
		return
	}
	info, err := os.Stat(a.subs.path)
	if err != nil {
		if !a.subs.ready || a.subs.err != "" {
			return
		}
	} else if a.subs.seen && a.subs.size == info.Size() && a.subs.mod.Equal(info.ModTime()) {
		return
	}
	key, path := a.subs.key, a.subs.path
	a.subs.path = ""
	a.loadSubtitles(key, path)
}

// revealInFolder shows path in the system file manager. Tests replace it.
var revealInFolder = revealPath

func revealPath(path string) error {
	switch runtime.GOOS {
	case "darwin":
		return exec.Command("open", "-R", path).Start()
	case "windows":
		return exec.Command("explorer", "/select,", path).Start()
	default:
		return exec.Command("xdg-open", filepath.Dir(path)).Start()
	}
}

func (a *app) reveal(path string) {
	if path == "" {
		return
	}
	if _, err := os.Stat(path); err != nil {
		a.saveErr = "文件不在了"
		return
	}
	if err := revealInFolder(path); err != nil {
		a.saveErr = "无法打开所在文件夹：" + err.Error()
		return
	}
	a.saveErr = ""
}

func jobTitle(job core.Job) string {
	return shorten(jobName(job), 22)
}

func jobName(job core.Job) string {
	if job.VideoPath != "" {
		return displayVideoName(job.VideoPath)
	}
	parsed, err := url.Parse(job.Source)
	if err != nil || parsed.Host == "" {
		return job.Source
	}
	host := strings.TrimPrefix(parsed.Hostname(), "www.")
	if host == "youtu.be" {
		if id := strings.Trim(parsed.Path, "/"); id != "" {
			return "YouTube · " + id
		}
	}
	if host == "youtube.com" || host == "m.youtube.com" || host == "music.youtube.com" {
		if id := parsed.Query().Get("v"); id != "" {
			return "YouTube · " + id
		}
	}
	title := host
	if parsed.Path != "" && parsed.Path != "/" {
		title += parsed.Path
	}
	return title
}

func displayVideoName(path string) string {
	base := filepath.Base(path)
	name := strings.TrimSuffix(base, filepath.Ext(base))
	if name != "video" {
		return name
	}
	parent := filepath.Base(filepath.Dir(path))
	if parent == "" || parent == "." || parent == string(filepath.Separator) {
		return name
	}
	return parent
}

func (a *app) posterOf(video string) ui.ImageSource {
	if video == "" {
		return nil
	}
	return a.posters[video]
}

// ensurePoster grabs one frame the first time a card needs it. A miss is remembered so the card does not retry every frame.
func (a *app) ensurePoster(video string) {
	if video == "" || a.update == nil || a.posters[video] != nil || a.posterMiss[video] || a.posterBusy[video] {
		return
	}
	dest := posterDest(a.posterDir, video)
	if dest == "" {
		a.markPosterMiss(video)
		return
	}
	if _, err := os.Stat(video); err != nil {
		a.markPosterMiss(video)
		return
	}
	tools, ok := a.pipeline.(media.Tools)
	replace := false
	if info, err := os.Stat(dest); err == nil && info.Size() > 0 {
		replace = media.PosterTooDark(dest)
		if !replace {
			ok = true
			tools.FFmpeg = ""
		}
	}
	if !ok || (tools.FFmpeg == "" && !fileExists(dest)) {
		a.markPosterMiss(video)
		return
	}
	if a.posterBusy == nil {
		a.posterBusy = map[string]bool{}
	}
	a.posterBusy[video] = true
	update, ffmpeg := a.update, tools.FFmpeg
	go func() {
		var err error
		if _, statErr := os.Stat(dest); statErr != nil || replace {
			err = media.GrabPoster(context.Background(), ffmpeg, video, dest)
		}
		var img ui.ImageSource
		if err == nil {
			data, readErr := os.ReadFile(dest)
			if readErr != nil {
				err = readErr
			} else {
				img, err = ui.DecodeImage(data)
			}
		}
		update(func() {
			delete(a.posterBusy, video)
			if err != nil || img == nil {
				a.markPosterMiss(video)
				return
			}
			if a.posters == nil {
				a.posters = map[string]ui.ImageSource{}
			}
			a.posters[video] = img
		})
	}()
}

func (a *app) markPosterMiss(video string) {
	if a.posterMiss == nil {
		a.posterMiss = map[string]bool{}
	}
	a.posterMiss[video] = true
}

func posterKey(video string) string {
	sum := sha256.Sum256([]byte(video))
	return hex.EncodeToString(sum[:8])
}

func posterDest(posterDir, video string) string {
	if strings.TrimSuffix(filepath.Base(video), filepath.Ext(video)) == "video" {
		return filepath.Join(filepath.Dir(video), "cover.jpg")
	}
	if posterDir == "" {
		return ""
	}
	return filepath.Join(posterDir, posterKey(video)+".jpg")
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() > 0
}

func shorten(s string, n int) string {
	r := []rune(s)
	if len(r) <= n {
		return s
	}
	return string(r[:n-1]) + "…"
}

func stageLabel(stage core.Stage) string {
	switch stage {
	case core.StageQueued:
		return "排队"
	case core.StageDownloading:
		return "下载中"
	case core.StageTranscribing:
		return "转录中"
	case core.StageTranslating:
		return "翻译中"
	case core.StageExporting:
		return "导出中"
	case core.StageComplete:
		return "已完成"
	case core.StageFailed:
		return "失败"
	default:
		return string(stage)
	}
}

func statusText(job core.Job) string {
	switch job.Stage {
	case core.StageQueued:
		return "任务已加入队列"
	case core.StageDownloading:
		return "正在下载视频…"
	case core.StageTranscribing:
		return "正在本地转录…"
	case core.StageTranslating:
		return "正在翻译字幕…"
	case core.StageExporting:
		return "正在导出双语字幕…"
	case core.StageComplete:
		return "完成：" + job.SubtitlePath
	case core.StageFailed:
		if errors.Is(job.Err, context.Canceled) {
			return "已取消"
		}
		return "失败：" + job.Err.Error()
	}
	return string(job.Stage)
}

// defaultExtraModelDirs lists places other local apps keep whisper.cpp models, so one already on disk can be selected without copying it.
func defaultExtraModelDirs() []string {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil
	}
	return []string{
		filepath.Join(home, "Library", "Application Support", "Vimpo", "models"),
		filepath.Join(home, ".cache", "hyperframes", "whisper", "models"),
	}
}

func main() {
	var win *mygo.Window
	tools := media.ToolsFromEnv()
	a := newApp(tools, func(fn func()) { win.Update(fn) })
	a.settingsPath = filepath.Join(filepath.Dir(defaultLibraryPath()), "settings.json")
	a.modelsDir = filepath.Join(filepath.Dir(a.settingsPath), "models")
	a.extraModelDirs = defaultExtraModelDirs()
	saved := loadSavedSettings(a.settingsPath)
	a.libraries = cleanLibraries(saved.Libraries)
	if saved.LibraryDir != "" {
		a.libraries = addLibrary(a.libraries, saved.LibraryDir)
	}
	if env := os.Getenv("LINGOCUT_OUTPUT_DIR"); env != "" {
		a.libraryDir = env
	} else if saved.LibraryDir != "" {
		a.libraryDir = saved.LibraryDir
	} else if len(a.libraries) > 0 {
		a.libraryDir = a.libraries[0]
	}
	tools.OutputDir = a.libraryDir
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
	if cache, err := os.UserCacheDir(); err == nil {
		a.posterDir = filepath.Join(cache, "LingoCut", "posters")
	}
	a.openLibrary(defaultLibraryPath())
	if os.Getenv("LINGOCUT_OUTPUT_DIR") == "" && filepath.Clean(a.libraryDir) == filepath.Clean(media.VimpoLibraryDir()) {
		bundle := media.DefaultLibraryDir()
		moved, err := media.DetachOwnVideos(a.libraryDir, bundle)
		if a.retargetLibrary(moved) {
			if saveErr := a.library.save(); saveErr != nil && err == nil {
				err = saveErr
			}
		}
		if err != nil {
			a.saveErr = "资料库无法搬出：" + err.Error()
		} else {
			a.libraries = dropLibrary(a.libraries, media.VimpoLibraryDir())
			a.libraries = addLibrary(a.libraries, bundle)
			a.libraryDir = bundle
			tools.OutputDir = bundle
			a.pipeline = tools
			if saveErr := a.saveSettings(); saveErr != nil {
				a.saveErr = "资料库无法保存：" + saveErr.Error()
			}
		}
	}
	if os.Getenv("LINGOCUT_OUTPUT_DIR") == "" && filepath.Clean(a.libraryDir) == filepath.Clean(media.LegacyLibraryDir()) {
		bundle := media.DefaultLibraryDir()
		moved, err := media.AdoptLibrary(a.libraryDir, bundle)
		if a.retargetLibrary(moved) {
			if saveErr := a.library.save(); saveErr != nil && err == nil {
				err = saveErr
			}
		}
		if err != nil {
			a.saveErr = "影视库无法整理：" + err.Error()
		} else {
			media.RemoveEmptyLibrary(a.libraryDir)
			a.libraries = addLibrary(a.libraries, bundle)
			a.libraryDir = bundle
			tools.OutputDir = bundle
			a.pipeline = tools
			if saveErr := a.saveSettings(); saveErr != nil {
				a.saveErr = "资料库无法保存：" + saveErr.Error()
			}
		}
	}
	ensureLibraryLogo(a.libraryDir)
	a.refreshLibrary()
	a.restartLibraryWatch()
	mygo.App.WhenReady(func() {
		win = mygo.NewWindow(mygo.WindowOptions{
			Title:           "LingoCut",
			Width:           1100,
			Height:          720,
			MinWidth:        900,
			MinHeight:       560,
			TitleBarStyle:   mygo.TitleBarHidden,
			BackgroundColor: windowBackground,
			// Opens where the user left it last time.
			StateKey: "main",
			// The window shows the interface MyGo draws, not a web page.
			Content: ui.View(a.view),
		})
		a.win = win
		a.loadToolVersions()
	})
	if err := mygo.App.Run(); err != nil {
		log.Fatal(err)
	}
}
