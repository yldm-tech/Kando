package main

import (
	"time"

	"lingocut/apps/lingocut/film"
	"lingocut/apps/lingocut/films"
	"lingocut/apps/lingocut/settings"
	"lingocut/apps/lingocut/sidebar"
	"lingocut/packages/media"

	"github.com/egoist/mygo/ui"
	"github.com/yldm-tech/nodra"
)

func (a *app) LibraryDir() string { return a.snap().LibraryDir }

func (a *app) KnownLibraries() []string { return a.knownLibraries() }

func (a *app) SaveErr() string { return a.saveErr }

func (a *app) Page() string { return a.page }

func (a *app) Select(page string) {
	a.page = page
	a.openKey = ""
	a.stopPlayback()
}

func (a *app) Choose(dir string) {
	a.edit(func(s *session) { s.MakingLibrary = false })
	a.page = pageFilms
	a.switchLibrary(dir)
}

func (a *app) BeginNew() { a.beginNewLibrary() }

func (a *app) PickLibrary() { a.pickLibrary() }

func (a *app) Reveal(path string) { a.reveal(path) }

func (a *app) Forget(dir string) { a.forgetLibrary(dir) }

func (a *app) MakingLibrary() bool { return a.snap().MakingLibrary }

func (a *app) OpenKey() string { return a.openKey }

func (a *app) SetOpenKey(key string) { a.openKey = key }

func (a *app) Source() *string { return &a.source }

func (a *app) Status() string { return a.status }

func (a *app) Running() bool { return a.running() }

func (a *app) HasJob() bool { return a.job != nil }

func (a *app) PlaybackOpen() bool {
	return a.play != nil || a.playPath != "" || a.playErr != ""
}

func (a *app) StopPlayback() { a.stopPlayback() }

func (a *app) CancelMaking() {
	a.edit(func(s *session) { s.MakingLibrary = false })
}

func (a *app) LibraryPlace() string { return a.libraryPlace() }

func (a *app) PickLibraryParent() { a.pickLibraryParent() }

func (a *app) CreateLibrary() { a.createLibrary() }

func (a *app) BindName(edit func(*string) bool) error {
	name, err := nodra.BindField(a.store, func(s session) string { return s.LibraryName }, func(s *session, value string) error {
		s.LibraryName = value
		return nil
	})
	if err != nil {
		return err
	}
	_ = name.Edit(edit)
	return nil
}

func (a *app) Start() { a.start() }

func (a *app) Stop() { a.stop() }

func (a *app) PickVideos() { a.pickVideos() }

func (a *app) Poster(path string) ui.ImageSource { return a.posterOf(path) }

func (a *app) EnsurePoster(path string) { a.ensurePoster(path) }

func (a *app) Open(item films.Film) {
	a.openKey = item.Key
	a.loadSubtitles(item.Key, item.SubtitlePath)
	a.startPlayback(item.VideoPath)
}

func (a *app) Remove(id string) { a.remove(id) }

func (a *app) Retry(id string) {
	if job, ok := a.library.Find(id); ok {
		a.retry(job)
	}
}

func (a *app) JobFilm(id string) (films.Film, bool) {
	job, ok := a.library.Find(id)
	if !ok {
		return films.Film{}, false
	}
	return films.FromJob(job, jobName(job), statusText(job)), true
}

func (a *app) StartFilm(item films.Film) { a.startFilm(item) }

func (a *app) RetryOpen(id string) {
	if job, ok := a.library.Find(id); ok {
		a.retry(job)
		if a.job != nil {
			a.openKey = a.job.ID
			a.loadSubtitles(a.job.ID, a.job.SubtitlePath)
		}
	}
}

func (a *app) Play() film.Player {
	if a.play == nil {
		return nil
	}
	return a.play
}

func (a *app) PlayPath() string { return a.playPath }

func (a *app) PlayErr() string { return a.playErr }

func (a *app) Subs() film.Subs {
	return film.Subs{Key: a.subs.key, Ready: a.subs.ready, Err: a.subs.err, Cues: a.subs.cues}
}

func (a *app) SubList() *ui.ListState { return &a.subs.list }

func (a *app) ClipList() *ui.ListState { return &a.subs.clips }

func (a *app) FilmSide() string { return a.filmSide }

func (a *app) SetFilmSide(id string) { a.filmSide = id }

func (a *app) Seek(at time.Duration) { a.seekPlayback(at) }

func (a *app) Cue() *film.Look { return &a.cue }

func (a *app) Tools() (media.Tools, bool) {
	tools, ok := a.pipeline.(media.Tools)
	return tools, ok
}

func (a *app) Language() *string { return &a.language }

func (a *app) TranslateContext() *bool { return &a.translateContext }

func (a *app) TranslatePrompt() *string { return &a.translatePrompt }

func (a *app) Save() error { return a.saveSettings() }

func (a *app) LibraryMenu(m *ui.Menu) { sidebar.Menu(m, a) }

func (a *app) Redetect() { a.redetectTools() }

func (a *app) ToolVersion(path string) string {
	if a.toolVersions == nil {
		return ""
	}
	return a.toolVersions[path]
}

func (a *app) SpeechDirs() []string { return a.speechSearchDirs() }

func (a *app) UseSpeechModel(path string) { a.useSpeechModel(path) }

func (a *app) Downloading() string { return a.downloading }

func (a *app) DownloadWritten() int64 { return a.downloadWritten }

func (a *app) DownloadTotal() int64 { return a.downloadTotal }

func (a *app) DownloadErr() string { return a.downloadErr }

func (a *app) CancelDownload() { a.cancelDownload() }

func (a *app) DownloadModel(id string) { a.downloadSpeechModel(id) }

func (a *app) CurrentPreset() (settings.Preset, bool) {
	preset, ok := llmPresetByID(a.llmProvider)
	if !ok {
		return settings.Preset{}, false
	}
	return settings.Preset{ID: preset.ID, Name: preset.Name, Blurb: preset.Blurb, Group: preset.Group}, true
}

func (a *app) Presets() []settings.Preset {
	src := llmPresets()
	out := make([]settings.Preset, len(src))
	for i, preset := range src {
		out[i] = settings.Preset{ID: preset.ID, Name: preset.Name, Blurb: preset.Blurb, Group: preset.Group}
	}
	return out
}

func (a *app) LLMProvider() string { return a.llmProvider }

func (a *app) SetLLMProvider(id string) { a.llmProvider = id }

func (a *app) ProviderID(base string) string { return llmProviderID(base) }

func (a *app) LLMBase() *string { return &a.llmBase }

func (a *app) LLMKey() *string { return &a.llmKey }

func (a *app) LLMModel() *string { return &a.llmModel }

func (a *app) ShowLLMKey() bool { return a.showLLMKey }

func (a *app) ToggleLLMKey() { a.showLLMKey = !a.showLLMKey }

func (a *app) ApplyLLM() { a.applyLLM() }

func (a *app) TestingLLM() bool { return a.testingLLM }

func (a *app) TestLLM() { a.testLLM() }

func (a *app) LLMResult() string { return a.llmTestResult }

func (a *app) SelectLLM(id string) { a.selectLLM(id) }

func (a *app) films() []films.Film {
	shot := a.snap()
	return films.List(shot.LibraryDir, a.library.Jobs, shot.Videos, jobName, statusText)
}
