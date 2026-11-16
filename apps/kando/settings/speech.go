package settings

import (
	"path/filepath"
	"strings"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/ui"
)

// speech is the 语音识别 page: the whisper models, one of which recognizes speech, all but the recommended one, Small and Base under 更多模型 until they are on disk, their downloads and what can be done with the files on disk, and the language spoken in the videos.
type speech struct{ s screen }

// Render implements core.Node.
func (p speech) Render(cx *core.Scope) core.Node {
	a := p.s.a
	colors := ui.ColorsOf(cx)
	models := media.SpeechModels()
	// shown is whether a model is in the card rather than under 更多模型.
	shown := func(model media.SpeechModel) func() bool {
		return live.Func(a, func() bool { return featuredSpeechModel(a, model) })
	}
	hidden := func(model media.SpeechModel) func() bool {
		return live.Func(a, func() bool { return !featuredSpeechModel(a, model) })
	}
	rows := modelRows(a, models, shown)
	rows = append(rows, core.Show{When: live.Func(a, func() bool { return otherSpeechModel(a) != "" }), Then: func(*core.Scope) core.Node {
		return core.Group([]core.Node{form.Separator(false), fileRow{a: a}})
	}})
	open := core.NewSignal(cx, false)
	more := core.Show{When: live.Func(a, func() bool {
		for _, model := range models {
			if !featuredSpeechModel(a, model) {
				return true
			}
		}
		return false
	}), Then: func(cx *core.Scope) core.Node {
		return ui.Collapsible{Text: locales.SettingsSpeechMoreModels(cx), TestID: "settings-models-more", Open: open.Get, OnOpenChange: open.Set,
			Variant: ui.CollapsibleArrowLeft, Style: core.Style{Margin: core.Edges{Left: 6}},
			Content: form.Card(modelRows(a, models, hidden)...)}
	}}
	// One radio group, so that Tab stops once in it and the arrows choose among the models on disk.
	group := ui.RadioGroup{Label: locales.SettingsSpeechModels(cx), Children: []core.Node{form.Card(rows...), more}, Style: core.Style{Gap: 6, AlignItems: core.AlignStretch}}
	failed := core.Show{When: live.Func(a, func() bool { return a.DownloadErr() != "" }), Then: func(cx *core.Scope) core.Node {
		return core.Text{ContentFunc: live.Func(a, a.DownloadErr), Style: core.Style{FontSize: 11, Padding: core.Symmetric(0, 12)}, StyleFunc: ui.Colored(colors.Danger)}
	}}
	choose := form.Button{Text: locales.SettingsSpeechChooseFile(cx), TestID: "settings-model-choose-file", OnPress: live.Do(a, a.PickSpeechModel)}
	return form.Page(
		form.Label(locales.SettingsSpeechModels(cx), 6, choose),
		group,
		form.Note(static(locales.SettingsSpeechNote(cx)), 0),
		failed,
		spokenLanguage(cx, a),
		form.Note(static(locales.SettingsSpeechSpokenNote(cx)), 0),
		trashDialog{a: a},
	)
}

// chosenSpeechModel is the whisper catalog model in use, "" when the one in use is none of them or not on disk.
func chosenSpeechModel(a App) string {
	tools, _ := a.Tools()
	for _, model := range media.SpeechModels() {
		if installed := a.InstalledSpeechModel(model.ID); installed != "" && filepath.Clean(installed) == filepath.Clean(tools.WhisperModel) {
			return model.ID
		}
	}
	return ""
}

// featuredSpeechModel reports whether the catalog model is listed in the card rather than under 更多模型: the recommended one, Small and Base, which cover the choice between accuracy and size, and any on disk, which can be chosen.
func featuredSpeechModel(a App, model media.SpeechModel) bool {
	return model.Recommended || model.ID == "small" || model.ID == "base" || a.InstalledSpeechModel(model.ID) != ""
}

// modelRows are the rows of the catalog models for which shown reports true, in the catalog's order, a line between two of them.
func modelRows(a App, models []media.SpeechModel, shown func(media.SpeechModel) func() bool) []core.Node {
	rows := make([]core.Node, len(models))
	for i, model := range models {
		visible := shown(model)
		// first is whether no model before this one shows, so no line goes above it.
		first := func() bool {
			for _, before := range models[:i] {
				if shown(before)() {
					return false
				}
			}
			return true
		}
		rows[i] = core.Show{When: visible, Then: func(*core.Scope) core.Node {
			return core.Group([]core.Node{
				core.Show{When: func() bool { return !first() }, Then: func(*core.Scope) core.Node { return form.Separator(false) }},
				modelRow{a: a, model: model},
			})
		}}
	}
	return rows
}

// otherSpeechModel is the model file in use when it is none of the whisper catalog's, as one chosen with 选择模型文件…, or "". A file named as a catalog model's is that model, wherever it is, as the app finds it there (media.ResolveSpeechModel).
func otherSpeechModel(a App) string {
	tools, _ := a.Tools()
	if tools.WhisperModel == "" {
		return ""
	}
	name := filepath.Base(tools.WhisperModel)
	for _, model := range media.SpeechModels() {
		if model.File == name {
			return ""
		}
	}
	return tools.WhisperModel
}

// fileRow is the model file in use when it is none of the catalog's: its radio button, chosen, its file's name over where it is. It is built again for another file, as the radio button takes its name as it is built.
type fileRow struct{ a App }

// Render implements core.Node.
func (r fileRow) Render(cx *core.Scope) core.Node {
	a := r.a
	path := core.NewMemo(cx, live.Func(a, func() string { return otherSpeechModel(a) }))
	return core.Dynamic(func(*core.Scope) core.Node {
		file := path.Get()
		name := filepath.Base(file)
		radio := ui.Radio{Label: name, TestID: "settings-model-file", Checked: func() bool { return true }, OnChange: func(bool) {}}
		texts := core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: []core.Node{
			core.Text{Content: name, MaxLines: 1, EllipsizeMode: core.EllipsizeMiddle, Style: core.Style{FontSize: 13, FontWeight: ui.Semibold, Color: form.Text}},
			form.Value(static(a.DisplayPath(file)), 11),
		}}
		return core.View{TestID: "settings-model-row-file",
			Style:    core.Style{Direction: core.Row, FlexShrink: 0, MinHeight: core.Pt(54), Padding: core.Symmetric(8, 14), Gap: 12, AlignItems: core.AlignCenter},
			Children: []core.Node{radio, texts}}
	})
}

// spokenLanguage is the card naming the language spoken in the videos: 自动识别, which lets whisper detect it from each video's first 30 seconds, or one of the languages whisper is told to hear. The drop-down shows the languages' names; what is saved is whisper's code.
func spokenLanguage(cx *core.Scope, a App) core.Node {
	codes := append([]string{""}, media.SpeechLanguages()...)
	options := make([]ui.Option[string], len(codes))
	for i, code := range codes {
		options[i] = ui.Option[string]{Value: code, Label: spokenLanguageName(locales.For(cx), code)}
	}
	choose := ui.Select[string]{Options: options, Label: locales.SettingsSpeechSpokenLanguage(cx), TestID: "settings-speech-language",
		Searchable: true, SearchPlaceholder: locales.SettingsLanguagesSearch(cx),
		Value: live.Func(a, a.SpeechLanguage), OnChange: live.On(a, a.SetSpeechLanguage), Style: core.Style{Width: core.Pt(160)}}
	card := form.Card(form.Row{Title: locales.SettingsSpeechSpokenLanguage(cx), NoGroup: true, Trailing: []core.Node{choose}})
	card.Style.Margin = core.Edges{Top: 10}
	return card
}

// spokenLanguageName is how the spoken-language drop-down names whisper's language code, 自动识别 for "", in the language of m.
func spokenLanguageName(m locales.Messages, code string) string {
	switch code {
	case "":
		return m.SettingsSpeechSpokenAuto()
	case "en":
		return m.SettingsSpeechLanguageEn()
	case "zh":
		return m.SettingsSpeechLanguageZh()
	case "ja":
		return m.SettingsSpeechLanguageJa()
	case "ko":
		return m.SettingsSpeechLanguageKo()
	case "fr":
		return m.SettingsSpeechLanguageFr()
	case "es":
		return m.SettingsSpeechLanguageEs()
	case "de":
		return m.SettingsSpeechLanguageDe()
	case "pt":
		return m.SettingsSpeechLanguagePt()
	case "it":
		return m.SettingsSpeechLanguageIt()
	case "ru":
		return m.SettingsSpeechLanguageRu()
	case "nl":
		return m.SettingsSpeechLanguageNl()
	case "pl":
		return m.SettingsSpeechLanguagePl()
	case "uk":
		return m.SettingsSpeechLanguageUk()
	case "tr":
		return m.SettingsSpeechLanguageTr()
	case "ar":
		return m.SettingsSpeechLanguageAr()
	case "hi":
		return m.SettingsSpeechLanguageHi()
	case "id":
		return m.SettingsSpeechLanguageId()
	case "vi":
		return m.SettingsSpeechLanguageVi()
	case "th":
		return m.SettingsSpeechLanguageTh()
	}
	return code
}

// trashDialog confirms moving a downloaded model to the Trash, saying how much room it frees. 取消 is the default, so Enter keeps the model; Escape keeps it too.
type trashDialog struct{ a App }

// Render implements core.Node.
func (d trashDialog) Render(cx *core.Scope) core.Node {
	a := d.a
	colors := ui.ColorsOf(cx)
	model := core.NewMemo(cx, live.Func(a, func() string { _, id := a.TrashingSpeechModel(); return id }))
	cancel := live.Do(a, a.CloseTrashSpeechModel)
	return core.Show{When: live.Func(a, func() bool { open, _ := a.TrashingSpeechModel(); return open }), Then: func(cx *core.Scope) core.Node {
		cx.OnKeyWith("enter", core.KeyOptions{When: core.KeyAlways}, cancel)
		// The dialog's words are set as it is built, so it is built again for another model.
		return core.Dynamic(func(cx *core.Scope) core.Node {
			id := model.Get()
			info, _ := media.SpeechModelByID(id)
			// TODO(nsgo#145): give ui.Dialog TitleFunc and DescriptionFunc twins so the words follow the model asked about without building the dialog again, then drop this Dynamic https://github.com/nsgo-dev/nsgo/issues/145
			return ui.Dialog{
				TestID:                 "settings-trash-confirm",
				Role:                   core.RoleAlertDialog,
				NoCloseOnBackdropPress: true,
				Open:                   func() bool { return true },
				Title:                  locales.SettingsSpeechTrashConfirm(cx, SpeechModelName(cx, id)),
				Description:            locales.SettingsSpeechTrashMessage(cx, info.SizeLabel),
				OnOpenChange: func(open bool, _ base.Reason) {
					if !open {
						cancel()
					}
				},
				Footer: []core.Node{
					ui.Button{Text: locales.SettingsSpeechCancel(cx), Variant: ui.ButtonSecondary, TestID: "settings-trash-cancel", OnPress: cancel},
					ui.Button{Text: locales.SettingsSpeechTrashButton(cx), Color: colors.Danger, TestID: "settings-trash-ok", OnPress: live.Do(a, func() {
						a.CloseTrashSpeechModel()
						a.TrashSpeechModel(id)
					})},
				},
			}
		})
	}}
}

// modelRow is a whisper model as the prototype's engine rows: its radio button, its name with 推荐 after it over what it is good for and its speed, accuracy and size, and what it offers at the end. A model not on disk cannot be chosen, so its radio button is disabled.
type modelRow struct {
	a     App
	model media.SpeechModel
}

// Render implements core.Node.
func (r modelRow) Render(cx *core.Scope) core.Node {
	a, model := r.a, r.model
	name := SpeechModelName(cx, model.ID)
	radio := ui.Radio{Label: name, TestID: "settings-model-" + model.ID,
		Checked:  live.Func(a, func() bool { return chosenSpeechModel(a) == model.ID }),
		Disabled: live.Func(a, func() bool { return a.InstalledSpeechModel(model.ID) == "" }),
		OnChange: live.On(a, func(on bool) {
			if path := a.InstalledSpeechModel(model.ID); on && path != "" {
				a.UseSpeechModel(path)
			}
		})}
	title := []core.Node{core.Text{Content: name, Style: core.Style{FontSize: 13, FontWeight: ui.Semibold, Color: form.Text}}}
	if model.Recommended {
		title = append(title, form.Badge(locales.SettingsSpeechRecommended(cx)))
	}
	about := SpeechModelBlurb(cx, model.ID) + "\n" + locales.SettingsSpeechModelStats(cx, stars(model.Speed), stars(model.Accuracy), model.SizeLabel)
	texts := core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: []core.Node{
		core.View{Style: core.Style{Direction: core.Row, Gap: 6, AlignItems: core.AlignCenter}, Children: title},
		core.Text{Content: about, Style: core.Style{FontSize: 11, LineHeight: 15, Color: form.Secondary}},
	}}
	end := core.View{Style: core.Style{Direction: core.Row, FlexShrink: 0, Gap: 8, AlignItems: core.AlignCenter}, Children: []core.Node{modelAction{a: a, model: model}}}
	return core.View{TestID: "settings-model-row-" + model.ID,
		Style:    core.Style{Direction: core.Row, FlexShrink: 0, MinHeight: core.Pt(54), Padding: core.Symmetric(8, 14), Gap: 12, AlignItems: core.AlignCenter},
		Children: []core.Node{radio, texts, end}}
}

// modelState is what a model offers at the end of its row.
type modelState uint8

const (
	modelDownload modelState = iota
	modelDownloading
	modelInstalled
)

// modelAction is what a model offers at the end of its row: its download's progress with a way to cancel it, 已下载 with ⋯ for its file (modelMenu), or 下载.
type modelAction struct {
	a     App
	model media.SpeechModel
}

// Render implements core.Node.
func (m modelAction) Render(cx *core.Scope) core.Node {
	a, model := m.a, m.model
	state := core.NewMemo(cx, live.Func(a, func() modelState {
		switch {
		case a.Downloading() == model.ID:
			return modelDownloading
		case a.InstalledSpeechModel(model.ID) != "":
			return modelInstalled
		}
		return modelDownload
	}))
	return core.Dynamic(func(cx *core.Scope) core.Node {
		name := SpeechModelName(cx, model.ID)
		switch state.Get() {
		case modelDownloading:
			total := live.Func(a, a.DownloadTotal)
			progress := ui.Progress{Label: locales.SettingsSpeechDownloadProgress(cx, name), TestID: "settings-model-progress-" + model.ID, Style: core.Style{Width: core.Pt(110)},
				Indeterminate: func() bool { return total() <= 0 },
				Value: live.Func(a, func() float32 {
					if a.DownloadTotal() <= 0 {
						return 0
					}
					return float32(a.DownloadWritten()) / float32(a.DownloadTotal()) * 100
				})}
			return core.Group([]core.Node{progress, form.Button{Text: locales.SettingsSpeechCancel(cx), OnPress: live.Do(a, a.CancelDownload), TestID: "settings-model-cancel-" + model.ID}})
		case modelInstalled:
			return core.Group([]core.Node{form.Value(static(locales.SettingsSpeechDownloaded(cx)), 12), modelMore{a: a, model: model}})
		}
		return form.Button{Text: locales.SettingsSpeechDownload(cx), TestID: "settings-model-download-" + model.ID,
			Disabled: live.Func(a, func() bool { return a.Downloading() != "" }),
			OnPress: live.Do(a, func() {
				if a.Downloading() == "" {
					a.DownloadModel(model.ID)
				}
			})}
	})
}

// modelMore is the ⋯ after 已下载, which opens the system's menu of what can be done with the model's file (modelMenu) under it.
type modelMore struct {
	a     App
	model media.SpeechModel
}

// Render implements core.Node.
func (m modelMore) Render(cx *core.Scope) core.Node {
	// place is the button's frame, which ui.IconButton takes no Ref for: the menu opens under the button, at its leading edge, however it was pressed.
	place := &core.Ref{}
	press := func() {
		at := core.Point{}
		if b, ok := place.Bounds(); ok {
			at = core.Point{X: b.Origin.X, Y: b.Origin.Y + b.Size.Height + 4}
		}
		nsdesktop.PopupMenu(cx, modelMenu(cx, m.a, m.model), at, nil)
	}
	button := ui.IconButton{Icon: svg(iconMore), Label: locales.SettingsSpeechModelActions(cx, SpeechModelName(cx, m.model.ID)), Size: 24, OnPress: press, TestID: "settings-model-more-" + m.model.ID}
	return core.View{Ref: place, Style: core.Style{FlexShrink: 0}, Children: []core.Node{button}}
}

// modelMenu is what can be done with a downloaded model's file, from ⋯ after 已下载: show it in Finder, wherever it is, and move it to the Trash when Kando downloaded it, unless it is a model in use (the one chosen, or the one a running job started with) or a download is running. It is built as the menu opens, so it says what is true then.
func modelMenu(cx *core.Scope, a App, model media.SpeechModel) []nsdesktop.MenuItem {
	installed := a.InstalledSpeechModel(model.ID)
	items := []nsdesktop.MenuItem{{Label: locales.SettingsSpeechReveal(cx), OnSelect: live.Do(a, func() { a.Reveal(installed) })}}
	if !a.OwnsSpeechModel(model.ID) {
		return items
	}
	return append(items, nsdesktop.MenuItem{Separator: true}, nsdesktop.MenuItem{Label: locales.SettingsSpeechTrash(cx),
		Disabled: a.SpeechModelInUse(installed) || a.Downloading() != "",
		OnSelect: live.Do(a, func() { a.AskTrashSpeechModel(model.ID) })})
}

// SpeechModelName is how the interface names the whisper catalog model id (media.SpeechModels) in the language the app shows under cx, as this page and the first-run guide list it.
func SpeechModelName(cx *core.Scope, id string) string { return SpeechModelNameIn(locales.For(cx), id) }

// SpeechModelNameIn is SpeechModelName in the language of m, for words said outside a scope, as an error kept in the app's state.
func SpeechModelNameIn(m locales.Messages, id string) string {
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
		return m.SettingsSpeechNameLargeV3TurboQ5()
	case "large-v3-turbo":
		return "Large v3 Turbo"
	case "large-v3":
		return "Large v3"
	}
	return id
}

// SpeechModelBlurb is what the whisper catalog model id is good for, in the language the app shows under cx, or "" for a model it does not know.
func SpeechModelBlurb(cx *core.Scope, id string) string {
	return SpeechModelBlurbIn(locales.For(cx), id)
}

// SpeechModelBlurbIn is SpeechModelBlurb in the language of m.
func SpeechModelBlurbIn(m locales.Messages, id string) string {
	switch id {
	case "tiny":
		return m.SettingsSpeechBlurbTiny()
	case "base":
		return m.SettingsSpeechBlurbBase()
	case "small":
		return m.SettingsSpeechBlurbSmall()
	case "medium":
		return m.SettingsSpeechBlurbMedium()
	case "large-v3-turbo-q5":
		return m.SettingsSpeechBlurbLargeV3TurboQ5()
	case "large-v3-turbo":
		return m.SettingsSpeechBlurbLargeV3Turbo()
	case "large-v3":
		return m.SettingsSpeechBlurbLargeV3()
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
