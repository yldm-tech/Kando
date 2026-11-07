package main

import (
	"os"
	"path/filepath"
	"slices"
	"strings"

	"kando/apps/kando/films"
	"kando/apps/kando/onboarding"
	"kando/packages/core"
	"kando/packages/media"
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
	// release gives back the window's modal flag (holdModal) the guide holds while it is open, so the menu bar's commands wait behind it; nil while it is closed.
	release func()
}

func newOnboardUI() onboardUI {
	return onboardUI{learning: slices.Clone(onboarding.DefaultLearning)}
}

// onboardingTip is the note the library shows once the guide has closed, as the prototype's does.
const onboardingTip = "随时可以在「帮助 › 重新查看入门引导」回到这里。"

// onboardingLibraryName is the library the guide makes when there is none (D8): 我的影片.kando in the folder where new libraries go, ~/Movies at first.
const onboardingLibraryName = "我的影片"

// savedOnboarding is the guide's part of settings.json.
type savedOnboarding struct {
	Onboarded      bool     `json:"onboarded,omitempty"`
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

// onboardingHooks: the guide opens by itself at startup on a Mac where Kando has no settings and no library yet, and once the window exists it opens the library it makes; someone who already has a library from before the guide existed counts as having seen it. It saves whether it was seen, the languages chosen and whether its note is still to be read, puts that note back in the message bar at startup, holds the window's modal flag while it is open, and forgets the note once its × has closed it.
func (a *app) onboardingHooks() areaHooks {
	return areaHooks{
		save: func(saved *savedSettings) {
			saved.savedOnboarding = savedOnboarding{Onboarded: a.onb.done, LearnLanguages: slices.Clone(a.onb.learning), OnboardingTip: a.onb.tip}
		},
		restore: func(saved savedSettings) {
			if saved.LearnLanguages != nil {
				a.onb.learning = slices.Clone(saved.LearnLanguages)
			}
			a.onb.done = saved.Onboarded || saved.LibraryDir != "" || len(saved.Libraries) > 0
			a.onb.tip = saved.OnboardingTip
			if a.onb.tip {
				a.lib.notice = noticeTip()
			}
			if !a.onb.done {
				a.openOnboarding()
			}
		},
		ready:      a.onboardingReady,
		afterFrame: a.onboardingAfterFrame,
	}
}

// onboardingReady opens the library the guide makes (D8) once the window exists, when the guide opened by itself and no library is open, so the page under the guide is that library's, empty, rather than one asking for a library. It waits for the window because opening a library starts following its folder, which reports through the window.
func (a *app) onboardingReady() {
	if a.onb.step > 0 {
		a.onboardingLibrary()
	}
}

// onboardingKeys are the guide's shortcuts. 帮助 › 重新查看入门引导 has none; it is an item of the menu bar alone (menu.go).
func (a *app) onboardingKeys() []shortcut { return nil }

func noticeTip() films.Notice { return films.Notice{Kind: "warning", Text: onboardingTip, Open: true} }

// onboardingAfterFrame holds the window's modal flag while the guide is open and gives it back once it has closed, and forgets the guide's note once the message bar's × has closed it.
func (a *app) onboardingAfterFrame() {
	switch {
	case a.onb.step > 0 && a.onb.release == nil:
		a.onb.release = a.holdModal()
	case a.onb.step == 0 && a.onb.release != nil:
		a.onb.release()
		a.onb.release = nil
	}
	if a.onb.tip && a.lib.notice.Text == onboardingTip && !a.lib.notice.Open {
		a.onb.tip = false
		a.onboardingSave()
	}
}

// openOnboarding shows the guide from its first step with an empty link field, and looks up which speech models are on disk for its second step.
func (a *app) openOnboarding() {
	a.onb.step = 1
	a.onb.link, a.onb.linkErr = "", ""
	a.findSpeechModels()
}

// closeOnboarding closes the guide: it opens a library when there is none, as D8 has it, marks the guide as seen and leaves its note in the message bar.
func (a *app) closeOnboarding() {
	a.onboardingLibrary()
	a.onb.step = 0
	a.onb.done = true
	a.onb.tip = true
	a.Select(pageFilms)
	a.lib.notice = noticeTip()
	a.onboardingSave()
}

// onboardingLibrary opens ~/Movies/我的影片.kando, or 我的影片.kando where new libraries go, when no library is open: it makes it as 新建资料库 does (createLibrary), or opens the one already there. A library that is open stays.
func (a *app) onboardingLibrary() {
	if strings.TrimSpace(a.snap().LibraryDir) != "" {
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

func (a *app) onboardingSave() {
	if err := a.saveSettings(); err != nil {
		a.saveErr = "设置无法保存：" + err.Error()
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
	if _, ok := onboarding.LinkHint(raw); !ok {
		return
	}
	link, _ := core.ParseLink(raw)
	a.onboardingLibrary()
	if msg, _ := a.precheck([]string{link.URL}); msg != "" {
		a.onb.linkErr, a.onb.errLink = msg, a.onb.link
		return
	}
	if err := a.Enqueue(link.URL); err != nil {
		a.onb.linkErr, a.onb.errLink = sourceError(err), a.onb.link
		return
	}
	a.onb.link, a.onb.linkErr = "", ""
	a.closeOnboarding()
	a.tell("已加入，开始处理。")
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

// onboardingModels is the second step's rows, from the speech settings' own state: the models found on disk, the one in use and the download running.
func (a *app) onboardingModels() []onboarding.Model {
	tools, _ := a.pipeline.(media.Tools)
	var out []onboarding.Model
	for _, id := range a.onboardingModelIDs() {
		model, ok := media.SpeechModelByID(id)
		if !ok {
			continue
		}
		m := onboarding.Model{ID: id, Name: model.Name, Size: model.SizeLabel, Blurb: model.Blurb}
		path := a.prefs.speechModels[id]
		switch {
		case a.prefs.downloading == id:
			m.State = onboarding.ModelDownloading
			m.Progress = -1
			if a.prefs.downloadTotal > 0 {
				m.Progress = float64(a.prefs.downloadWritten) / float64(a.prefs.downloadTotal)
			}
		case path != "" && path == tools.WhisperModel:
			m.State = onboarding.ModelInUse
		case path != "":
			m.State = onboarding.ModelReady
		}
		if a.onb.lastDownload == id && a.prefs.downloading == "" && a.prefs.downloadErr != "" {
			m.Err = a.prefs.downloadErr
		}
		out = append(out, m)
	}
	return out
}

// onboardingDownloadsOnNext reports whether 继续 on the second step downloads the first model it offers: no speech model is in use, none is downloading, and that one is not on disk.
func (a *app) onboardingDownloadsOnNext() bool {
	if a.prefs.downloading != "" {
		return false
	}
	if tools, ok := a.pipeline.(media.Tools); ok && tools.WhisperModel != "" {
		return false
	} else if !ok {
		return false
	}
	models := a.onboardingModels()
	return len(models) > 0 && models[0].State == onboarding.ModelMissing
}

// onboardingNext is the primary button: on to the next step, downloading the recommended model when the second step's button says so, or finishing on the last.
func (a *app) onboardingNext() {
	switch a.onb.step {
	case 1:
		a.onb.step = 2
		a.findSpeechModels()
	case 2:
		if a.onboardingDownloadsOnNext() {
			a.onboardingDownload(a.onboardingModels()[0].ID)
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
