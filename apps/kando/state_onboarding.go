package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"kando/apps/kando/films"
	"kando/apps/kando/locales"
	"kando/apps/kando/onboarding"
	"kando/apps/kando/settings"
	"kando/packages/core"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
)

// onboardUI is the state of the first-run guide (onboarding/): the step it shows, whether it was seen, the languages chosen on its first step and the link typed on its last.
type onboardUI struct {
	// step is the step shown, 1 to onboarding.Steps, or 0 while the guide is closed.
	step int
	// done is saved once the guide was seen, skipped or finished, so it opens by itself only on the first launch. 帮助 › 重新查看入门引导 opens it again.
	done bool
	// learning is the languages of the videos the user watches, as codes. The second step recommends speech models for them.
	learning []string
	// link is the third step's field, and linkErr why the link in it could not be added.
	link, linkErr string
	// errLink is the link linkErr is about; once the field holds another, the error no longer shows.
	errLink string
	// lastDownload is the model the guide last started downloading, so a failed download's error shows on its row.
	lastDownload string
	// tip is set from the guide's closing until the note it leaves in the library's message bar is closed with its ×.
	tip bool
	// release gives back the window's modal flag (holdModal) the guide holds while it is open, so the menu bar's commands wait behind it: openOnboarding takes it and closeOnboarding gives it back. It is nil while the guide is closed.
	release func()
}

func newOnboardUI() onboardUI {
	return onboardUI{learning: slices.Clone(onboarding.DefaultLearning)}
}

// onboardingTip is the note the library shows once the guide has closed, as the prototype's does, in the interface's language, as the message bar keeps it. It names the way back that this platform has: the menu bar's 帮助 › 重新查看入门引导 on macOS, F1 (onboardingKeys) where the app has no menu bar of its own.
func (a *app) onboardingTip() string { return onboardingTipIn(a.messages()) }

// onboardingTipIn is onboardingTip in the language of m.
func onboardingTipIn(m locales.Messages) string {
	if appMenuBar {
		return m.OnboardingTipMenu(m.MenuGroupHelp(), m.OnboardingReopen())
	}
	return m.OnboardingTipKey()
}

// isOnboardingTip reports whether text is the guide's note in any of the interface's languages, so a note shown before the language changed is still known as the guide's once its × closes it.
func isOnboardingTip(text string) bool {
	for _, tag := range locales.Interface {
		if text == onboardingTipIn(locales.In(tag)) {
			return true
		}
	}
	return false
}

// onboardingLibraryName is the library the guide makes when there is none (D8): 我的影片.kando in the folder where new libraries go, ~/Movies at first. It is a folder's name on disk, so it stays the same in every language.
const onboardingLibraryName = "我的影片"

// savedOnboarding is the guide's part of settings.json.
type savedOnboarding struct {
	// Onboarded is written on every save, false while the guide has not been seen through, so a settings.json without it is one from before the guide existed.
	Onboarded      *bool    `json:"onboarded,omitempty"`
	LearnLanguages []string `json:"learnLanguages,omitempty"`
	OnboardingTip  bool     `json:"onboardingTip,omitempty"`
}

// clean keeps the languages the guide knows, each once.
func (s *savedOnboarding) clean() {
	var kept []string
	for _, code := range s.LearnLanguages {
		code = strings.TrimSpace(code)
		if !onboarding.Known(code) || slices.Contains(kept, code) {
			continue
		}
		kept = append(kept, code)
	}
	s.LearnLanguages = kept
}

// onboardingHooks: the guide opens by itself at startup when there is no settings.json yet, or one that says it was not seen through (quitting while it is open), and once the window exists it opens a library under it. A settings.json from before the guide existed counts as seen when it names a library, and one that could not be read in full always does, so the guide never makes a library and saves over it. It saves whether it was seen, the languages chosen and whether its note is still to be read, puts that note back in the message bar at startup, holds the window's modal flag while it is open, forgets the note once its × has closed it, and fetches the AI service's model list for its second step when no model is chosen (onboardingLoadModels).
func (a *app) onboardingHooks() areaHooks {
	return areaHooks{
		save: func(saved *savedSettings) {
			saved.savedOnboarding = savedOnboarding{Onboarded: new(a.onb.done), LearnLanguages: slices.Clone(a.onb.learning), OnboardingTip: a.onb.tip}
		},
		restore: func(saved savedSettings) {
			if saved.LearnLanguages != nil {
				a.onb.learning = slices.Clone(saved.LearnLanguages)
			}
			switch {
			case saved.Onboarded != nil:
				a.onb.done = *saved.Onboarded
			case a.settingsFound == settingsDamaged:
				a.onb.done = true
			case a.settingsFound == settingsNone:
				a.onb.done = false
			default:
				a.onb.done = saved.LibraryDir != "" || len(saved.Libraries) > 0
			}
			a.onb.tip = saved.OnboardingTip
			if a.onb.tip {
				a.libraryShow(a.noticeTip())
			}
			if !a.onb.done {
				a.openOnboarding()
			}
		},
		ready: a.onboardingReady,
		afterChange: func() bool {
			tip := a.onboardingAfterChange()
			models := a.onboardingLoadModels()
			return tip || models
		},
	}
}

// onboardingLoadModels fetches the AI service's model list while the guide's second step shows and no model is chosen, as for a service the guide picked that names no model of its own, so one is chosen from the list (adoptLLMModel) as the AI 服务 page does. It reports whether it changed the settings' state so.
func (a *app) onboardingLoadModels() bool {
	tools, ok := a.pipeline.(media.Tools)
	if !ok || a.onb.step != 2 || strings.TrimSpace(tools.Translator.Model) != "" {
		return false
	}
	return a.fetchLLMModels(false)
}

// onboardingReady opens a library under the guide (onboardingLibrary, D8) once the window exists, when the guide opened by itself and no library is open, so the page under the guide is that library's rather than one asking for a library. It waits for the window because opening a library starts following its folder, which reports through the window.
func (a *app) onboardingReady() {
	if a.onb.step > 0 {
		a.onboardingLibrary()
	}
}

// onboardingKeys are the guide's shortcuts. On macOS 帮助 › 重新查看入门引导 is an item of the menu bar alone (menu.go), without a key, as the prototype has it. Linux and Windows have no app menu bar, so there it is F1, the key for help there, which the cheat sheet and 设置 › 快捷键 list.
func (a *app) onboardingKeys(cx *ns.Scope) []shortcut {
	if appMenuBar {
		return nil
	}
	return []shortcut{
		{Group: groupHelp, Label: locales.OnboardingReopen(cx), Key: "f1", When: func() bool { return a.onb.step == 0 }, Run: a.reopenOnboarding},
	}
}

// noticeTip is the message bar holding the guide's note.
func (a *app) noticeTip() films.Notice {
	return films.Notice{Kind: "warning", Text: a.onboardingTip(), Open: true}
}

// onboardingOverlay is the guide over the window while it is open (state_overlay.go): its sheet opens on a.onb.step and closes back into it.
func (a *app) onboardingOverlay(*ns.Scope) ns.Node { return onboarding.View{App: a} }

// onboardingAfterChange puts the guide's note in the message bar whenever nothing else is shown there while the note is still to be read, as after 已加入，开始处理。 is closed, and forgets it once its own × has closed it. It reports whether it changed either, which the message bar reads.
func (a *app) onboardingAfterChange() bool {
	switch {
	case !a.onb.tip || a.onb.step > 0 || a.lib.notice.Open:
		return false
	case isOnboardingTip(a.lib.notice.Text):
		a.onb.tip = false
		a.onboardingSave()
	default:
		a.libraryShow(a.noticeTip())
	}
	return true
}

// openOnboarding shows the guide from its first step with an empty link field, holding the window's modal flag until it closes, and looks up which speech models are on disk for its second step.
func (a *app) openOnboarding() {
	a.onb.step = 1
	a.onb.link, a.onb.linkErr = "", ""
	if a.onb.release == nil {
		a.onb.release = a.holdModal()
	}
	a.findSpeechModels()
}

// closeOnboarding closes the guide: it opens a library when there is none, as D8 has it, marks the guide as seen, leaves its note in the message bar and gives back the window's modal flag, so the menu bar's commands run again.
func (a *app) closeOnboarding() {
	a.onboardingLibrary()
	a.onb.step = 0
	a.onb.done = true
	a.onb.tip = true
	a.Select(pageFilms)
	a.libraryShow(a.noticeTip())
	a.onboardingSave()
	if a.onb.release != nil {
		a.onb.release()
		a.onb.release = nil
	}
}

// onboardingLibrary opens a library when none is open. Libraries already in the folder where new libraries go (~/Movies at first) that Kando does not list, as after its settings were lost, are listed and the one changed last is opened. Without any, it opens 我的影片.kando there, making it as 新建资料库 does (createLibrary). A library that is open stays.
func (a *app) onboardingLibrary() {
	if strings.TrimSpace(a.snap().LibraryDir) != "" {
		return
	}
	if found := unlistedBundles(a.libraryPlace(), a.knownLibraries()); len(found) > 0 {
		libs := a.snap().Libraries
		for _, dir := range found {
			libs = addLibrary(libs, dir)
		}
		a.edit(func(s *session) { s.Libraries = libs })
		a.switchLibrary(onboardingNewest(found))
		return
	}
	a.edit(func(s *session) { s.LibraryName = onboardingLibraryName })
	dir, err := a.NewLibraryPath()
	if err != nil {
		a.saveErr = err.Error()
		return
	}
	if info, statErr := os.Stat(dir); statErr == nil && info.IsDir() {
		a.switchLibrary(dir)
		return
	}
	a.createLibrary()
}

// onboardingNewest is the library among dirs whose folder changed last, the first of them when none can be read.
func onboardingNewest(dirs []string) string {
	newest, at := dirs[0], time.Time{}
	for _, dir := range dirs {
		if info, err := os.Stat(dir); err == nil && info.ModTime().After(at) {
			newest, at = dir, info.ModTime()
		}
	}
	return newest
}

func (a *app) onboardingSave() {
	if err := a.saveSettings(); err != nil {
		a.saveErr = a.messages().OnboardingSaveFailed(err.Error())
	}
}

// reopenOnboarding is 帮助 › 重新查看入门引导.
func (a *app) reopenOnboarding() {
	a.openOnboarding()
}

// onboardingFinish is the last step's primary button: with a link it opens the library and adds the link to its queue through Enqueue, without one it only closes the guide. A link the queue does not take keeps the guide open, saying why.
func (a *app) onboardingFinish() {
	raw := strings.TrimSpace(a.onb.link)
	if raw == "" {
		a.closeOnboarding()
		return
	}
	if !onboarding.Addable(raw) {
		return
	}
	link, _ := core.ParseLink(raw)
	a.onboardingLibrary()
	if msg, _ := a.precheck([]string{link.URL}); msg != "" {
		a.onb.linkErr, a.onb.errLink = msg, a.onb.link
		return
	}
	if err := a.Enqueue(link.URL); err != nil {
		a.onb.linkErr, a.onb.errLink = sourceError(a.messages(), err), a.onb.link
		return
	}
	a.onb.link, a.onb.linkErr = "", ""
	a.closeOnboarding()
	a.tell(a.messages().OnboardingAdded())
}

// onboardingModelIDs is the speech models the second step offers for the languages chosen: Base, quick and enough for clear English, before Large v3 Turbo (compressed) when only English was chosen; otherwise Large v3 Turbo (compressed), the catalog's recommendation, before the lighter Small. A catalog model in use that is neither is listed after them.
func (a *app) onboardingModelIDs() []string {
	learning := onboarding.Effective(a.onb.learning, a.prefs.language)
	ids := []string{"large-v3-turbo-q5", "small"}
	if len(learning) > 0 && !slices.ContainsFunc(learning, func(code string) bool { return code != "en" }) {
		ids = []string{"base", "large-v3-turbo-q5"}
	}
	if tools, ok := a.pipeline.(media.Tools); ok && tools.WhisperModel != "" {
		for _, model := range media.SpeechModels() {
			if filepath.Base(tools.WhisperModel) == model.File && !slices.Contains(ids, model.ID) {
				ids = append(ids, model.ID)
			}
		}
	}
	return ids
}

// onboardingModels is the second step's rows, from the speech settings' own state: the models found on disk, the one in use and the download running, in the language shown under cx.
func (a *app) onboardingModels(cx *ns.Scope) []onboarding.Model {
	var out []onboarding.Model
	for _, id := range a.onboardingModelIDs() {
		model, ok := media.SpeechModelByID(id)
		if !ok {
			continue
		}
		m := onboarding.Model{ID: id, Name: settings.SpeechModelName(cx, id), Size: model.SizeLabel, Blurb: settings.SpeechModelBlurb(cx, id), State: a.onboardingModelState(id)}
		if m.State == onboarding.ModelDownloading {
			m.Progress = -1
			if a.prefs.downloadTotal > 0 {
				m.Progress = float64(a.prefs.downloadWritten) / float64(a.prefs.downloadTotal)
			}
		}
		if a.onb.lastDownload == id && a.prefs.downloading == "" {
			m.Err = settings.SpeechDownloadError(locales.For(cx), a.prefs.downloadErr)
		}
		out = append(out, m)
	}
	return out
}

// onboardingModelState is where the speech model id is: downloading, on disk and in use, on disk, or missing.
func (a *app) onboardingModelState(id string) onboarding.ModelState {
	tools, _ := a.pipeline.(media.Tools)
	path := a.prefs.speechModels[id]
	switch {
	case a.prefs.downloading == id:
		return onboarding.ModelDownloading
	case path != "" && path == tools.WhisperModel:
		return onboarding.ModelInUse
	case path != "":
		return onboarding.ModelReady
	}
	return onboarding.ModelMissing
}

// onboardingFirstModel is the model the second step offers first, the one its 下载并继续 downloads.
func (a *app) onboardingFirstModel() (id string, ok bool) {
	for _, id := range a.onboardingModelIDs() {
		if _, ok := media.SpeechModelByID(id); ok {
			return id, true
		}
	}
	return "", false
}

// onboardingDownloadsOnNext reports whether 继续 on the second step downloads the first model it offers: no speech model is in use, none is downloading, and that one is not on disk.
func (a *app) onboardingDownloadsOnNext() bool {
	if a.prefs.downloading != "" {
		return false
	}
	if tools, ok := a.pipeline.(media.Tools); !ok || tools.WhisperModel != "" {
		return false
	}
	id, ok := a.onboardingFirstModel()
	return ok && a.onboardingModelState(id) == onboarding.ModelMissing
}

// onboardingNext is the primary button: on to the next step, downloading the recommended model when the second step's button says so, or finishing on the last.
func (a *app) onboardingNext() {
	switch a.onb.step {
	case 1:
		a.onb.step = 2
		a.findSpeechModels()
	case 2:
		if a.onboardingDownloadsOnNext() {
			id, _ := a.onboardingFirstModel()
			a.onboardingDownload(id)
		}
		a.onb.step = 3
	case onboarding.Steps:
		a.onboardingFinish()
	}
}

// onboardingDownload starts downloading model id with the speech settings' download (settings_speech.go), which puts the model to use once it has arrived.
func (a *app) onboardingDownload(id string) {
	a.onb.lastDownload = id
	a.downloadSpeechModel(id)
}

// onboardingTools is the programs Kando runs as the second step shows them, in the language shown under cx: there, downloading, missing with Kando's download of it, or missing where Kando cannot download it.
func (a *app) onboardingTools(cx *ns.Scope) []onboarding.Tool {
	tools, own := a.pipeline.(media.Tools)
	rows := []onboarding.Tool{
		{Name: "yt-dlp", Use: locales.SettingsGeneralUseYTDLP(cx)},
		{Name: "ffmpeg", Use: locales.SettingsGeneralUseFFmpeg(cx)},
		{Name: "whisper-cli", Use: locales.SettingsGeneralUseWhisper(cx)},
	}
	paths := []string{tools.YTDLP, tools.FFmpeg, tools.Whisper}
	for i := range rows {
		name := rows[i].Name
		progress, running := a.kitsProgress(name)
		switch {
		// Another pipeline, as a test's, runs no programs of its own.
		case !own, paths[i] != "":
			rows[i].State = onboarding.ToolReady
		case running:
			rows[i].State, rows[i].Progress = onboarding.ToolDownloading, progress
		case a.kitsCanInstall(name):
			rows[i].State, rows[i].Err = onboarding.ToolMissing, settings.KitsError(locales.For(cx), a.kits.errs[name])
		default:
			rows[i].State = onboarding.ToolUnavailable
			if src, ok := media.SourceFor(name); ok {
				rows[i].Page = src.Page
				if src.Scoop != "" {
					rows[i].Install = "scoop install " + src.Scoop
				}
			}
		}
	}
	return rows
}
