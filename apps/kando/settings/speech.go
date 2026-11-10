package settings

import (
	"path/filepath"
	"slices"
	"strings"

	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"
	"kando/packages/media"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// speech is the 语音识别 page: the whisper models, one of which recognizes speech, their downloads and what can be done with the files on disk, and the language spoken in the videos.
func (s screen) speech(c *ui.Context, t *ui.Theme) {
	tools, _ := s.Tools()
	models := media.SpeechModels()
	installed := make([]string, len(models))
	chosen := ""
	for i, model := range models {
		installed[i] = s.InstalledSpeechModel(model.ID)
		if installed[i] != "" && filepath.Clean(installed[i]) == filepath.Clean(tools.WhisperModel) {
			chosen = model.ID
		}
	}
	sel := chosen
	form.Label(c, locales.SettingsSpeechModels(), 6, nil)
	form.Card(c, func() {
		// One radio group, so that Tab stops once in it and the arrows choose among the models on disk.
		ui.RadioGroup(c, func() {
			for i, model := range models {
				if i > 0 {
					form.Separator(c, false)
				}
				c.Key(model.ID)
				s.modelRow(c, &sel, model, installed[i])
			}
		}).Label(locales.SettingsSpeechModels()).Gap(0).AlignItems(ui.Stretch)
	})
	form.Note(c, locales.SettingsSpeechNote(), 0)
	if sel != chosen {
		for i, model := range models {
			if model.ID == sel {
				s.UseSpeechModel(installed[i])
			}
		}
	}
	if s.DownloadErr() != "" {
		ui.Text(c, s.DownloadErr()).FontSize(11).TextColor(t.Danger).Wrap().Padding(0, 12)
	}
	s.spokenLanguage(c)
	s.trashDialog(c)
}

// spokenLanguage is the card naming the language spoken in the videos: 自动识别, which lets whisper detect it from each video's first 30 seconds, or one of the languages whisper is told to hear. The drop-down shows the languages' names; what is saved is whisper's code.
func (s screen) spokenLanguage(c *ui.Context) {
	form.Card(c, func() {
		form.Row(c, locales.SettingsSpeechSpokenLanguage(), "", func() {
			c.Key("speech-language")
			codes := append([]string{""}, media.SpeechLanguages()...)
			labels := make([]string, len(codes))
			for i, code := range codes {
				labels[i] = spokenLanguageName(code)
			}
			chosen := spokenLanguageName(s.SpeechLanguage())
			if form.Select(c, &chosen, labels).Label(locales.SettingsSpeechSpokenLanguage()).Changed() {
				if i := slices.Index(labels, chosen); i >= 0 {
					s.SetSpeechLanguage(codes[i])
				}
			}
		})
	}).Margin(10, 0, 0, 0)
	form.Note(c, locales.SettingsSpeechSpokenNote(), 0)
}

// spokenLanguageName is how the spoken-language drop-down names whisper's language code, 自动识别 for "", in the interface's language.
func spokenLanguageName(code string) string {
	switch code {
	case "":
		return locales.SettingsSpeechSpokenAuto()
	case "en":
		return locales.SettingsSpeechLanguageEn()
	case "zh":
		return locales.SettingsSpeechLanguageZh()
	case "ja":
		return locales.SettingsSpeechLanguageJa()
	case "ko":
		return locales.SettingsSpeechLanguageKo()
	case "fr":
		return locales.SettingsSpeechLanguageFr()
	case "es":
		return locales.SettingsSpeechLanguageEs()
	case "de":
		return locales.SettingsSpeechLanguageDe()
	}
	return code
}

// trashDialog confirms moving a downloaded model to the Trash, saying how much room it frees. 取消 is the default, so Enter keeps the model.
func (s screen) trashDialog(c *ui.Context) {
	open, id := s.TrashingSpeechModel()
	if !*open {
		return
	}
	model, _ := media.SpeechModelByID(id)
	name := SpeechModelName(id)
	if breeze.DestructiveAlertDialog(c, open, locales.SettingsSpeechTrashConfirm(name), locales.SettingsSpeechTrashMessage(model.SizeLabel), 0, locales.SettingsSpeechTrashButton(), locales.SettingsSpeechCancel()) == 0 {
		s.TrashSpeechModel(id)
	}
}

// modelRow is a whisper model as the prototype's engine rows: its radio button, its name with 推荐 after it over what it is good for and its speed, accuracy and size, and what it offers at the end. A model not on disk cannot be chosen, so its radio button is disabled.
func (s screen) modelRow(c *ui.Context, sel *string, model media.SpeechModel, installed string) {
	name := SpeechModelName(model.ID)
	ui.Row(c).Shrink(0).MinHeight(54).Padding(8, 14).Gap(12).AlignItems(ui.Center).Children(func() {
		breeze.Radio(c, sel, model.ID, "").Label(name).Disabled(installed == "")
		ui.Column(c).Grow(1).Shrink(1).MinWidth(0).Gap(2).Children(func() {
			ui.Row(c).Gap(6).AlignItems(ui.Center).Children(func() {
				ui.Text(c, name).FontSize(13).FontWeight(600).TextColor(form.Text)
				if model.Recommended {
					form.Badge(c, locales.SettingsSpeechRecommended())
				}
			})
			ui.Text(c, SpeechModelBlurb(model.ID)+"\n"+locales.SettingsSpeechModelStats(stars(model.Speed), stars(model.Accuracy), model.SizeLabel)).FontSize(11).FixedLineHeight(15).TextColor(form.Secondary).Wrap()
		})
		ui.Row(c).Shrink(0).Gap(8).AlignItems(ui.Center).Children(func() { s.modelAction(c, model, installed) })
	})
}

// modelAction is what a model offers at the end of its row: its download's progress with a way to cancel it, 已下载 with ⋯ for its file (modelMenu), or 下载.
func (s screen) modelAction(c *ui.Context, model media.SpeechModel, installed string) {
	if s.Downloading() == model.ID {
		progress := float64(-1)
		if s.DownloadTotal() > 0 {
			progress = float64(s.DownloadWritten()) / float64(s.DownloadTotal())
		}
		breeze.Progress(c, progress).Width(110).Label(locales.SettingsSpeechDownloadProgress(SpeechModelName(model.ID)))
		c.Key(model.ID + "-cancel")
		if form.Button(c, locales.SettingsSpeechCancel(), false).Clicked() {
			s.CancelDownload()
		}
		return
	}
	if installed != "" {
		form.Value(c, locales.SettingsSpeechDownloaded(), 12)
		c.Key(model.ID + "-more")
		breeze.FlatIconButton(c, iconMore, locales.SettingsSpeechModelActions(SpeechModelName(model.ID))).Menu(s.modelMenu(model, installed))
		return
	}
	c.Key(model.ID + "-download")
	if form.Button(c, locales.SettingsSpeechDownload(), false).Disabled(s.Downloading() != "").Clicked() && s.Downloading() == "" {
		s.DownloadModel(model.ID)
	}
}

// modelMenu is what can be done with a downloaded model's file, from ⋯ after 已下载: show it in Finder, wherever it is, and move it to the Trash when Kando downloaded it, unless it is a model in use (the one chosen, or the one a running job started with) or a download is running.
func (s screen) modelMenu(model media.SpeechModel, installed string) func(m *ui.Menu) {
	return func(m *ui.Menu) {
		if m.Item(locales.SettingsSpeechReveal()).Chosen() {
			s.Reveal(installed)
		}
		if !s.OwnsSpeechModel(model.ID) {
			return
		}
		m.Separator()
		if m.Item(locales.SettingsSpeechTrash()).Disabled(s.SpeechModelInUse(installed) || s.Downloading() != "").Chosen() {
			s.AskTrashSpeechModel(model.ID)
		}
	}
}

// SpeechModelName is how the interface names the whisper catalog model id (media.SpeechModels), as this page and the first-run guide list it.
func SpeechModelName(id string) string {
	switch id {
	case "tiny":
		return "Tiny"
	case "base":
		return "Base"
	case "small":
		return "Small"
	case "medium":
		return "Medium"
	case "large-v3-turbo-q5":
		return locales.SettingsSpeechNameLargeV3TurboQ5()
	case "large-v3-turbo":
		return "Large v3 Turbo"
	case "large-v3":
		return "Large v3"
	}
	return id
}

// SpeechModelBlurb is what the whisper catalog model id is good for, in the interface's language, or "" for a model it does not know.
func SpeechModelBlurb(id string) string {
	switch id {
	case "tiny":
		return locales.SettingsSpeechBlurbTiny()
	case "base":
		return locales.SettingsSpeechBlurbBase()
	case "small":
		return locales.SettingsSpeechBlurbSmall()
	case "medium":
		return locales.SettingsSpeechBlurbMedium()
	case "large-v3-turbo-q5":
		return locales.SettingsSpeechBlurbLargeV3TurboQ5()
	case "large-v3-turbo":
		return locales.SettingsSpeechBlurbLargeV3Turbo()
	case "large-v3":
		return locales.SettingsSpeechBlurbLargeV3()
	}
	return ""
}

func stars(n int) string {
	if n < 0 {
		n = 0
	}
	if n > 5 {
		n = 5
	}
	return strings.Repeat("★", n) + strings.Repeat("☆", 5-n)
}
