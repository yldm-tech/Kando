package settings

import (
	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// look is the 字幕外观 page: a preview of the subtitles in the player's look, and how the player shows them. dark is the player's theme, which the preview is drawn in.
func (s screen) look(c *ui.Context, t, dark *ui.Theme) {
	s.lookPreview(c, dark)
	form.Note(c, locales.SettingsLookNote(), 6)
	form.Card(c, func() {
		c.Key("look-mode")
		choice(c, locales.SettingsLookMode(), "", s.SubMode, s.SetSubMode, []option{
			{subtitle.Original, locales.SettingsLookModeOriginal()}, {subtitle.Translation, locales.SettingsLookModeTranslation()}, {subtitle.Bilingual, locales.SettingsLookModeBilingual()},
		}, s.Save)
		form.Separator(c, true)
		c.Key("look-order")
		choice(c, locales.SettingsLookOrder(), "", s.SubOrder, s.SetSubOrder, []option{
			{subtitle.TranslationFirst, locales.SettingsLookOrderTranslationFirst()}, {subtitle.OriginalFirst, locales.SettingsLookOrderOriginalFirst()},
		}, s.Save)
		form.Separator(c, true)
		c.Key("look-untranslated")
		choice(c, locales.SettingsLookUntranslated(), locales.SettingsLookUntranslatedHint(), s.Untranslated, s.SetUntranslated, []option{
			{subtitle.UntranslatedDim, locales.SettingsLookUntranslatedDim()}, {subtitle.UntranslatedBlank, locales.SettingsLookUntranslatedBlank()},
		}, s.Save)
		form.Separator(c, true)
		c.Key("look-yield")
		yield := s.YieldSubs()
		if form.SwitchRow(c, locales.SettingsLookYield(), locales.SettingsLookYieldHint(), &yield) {
			s.SetYieldSubs(yield)
			s.Save()
		}
	})
}

// option is one choice of a segmented row: the value the app keeps and the label it shows.
type option struct{ value, label string }

// choice is a row with a segmented control choosing among options; a change is set and saved. A value none of the options holds, as the player's 关闭 for the display mode, leaves every segment unchosen.
func choice(c *ui.Context, title, subtitle string, get func() string, set func(string), options []option, save func() error) {
	labels := make([]string, len(options))
	chosen := -1
	for i, o := range options {
		labels[i] = o.label
		if o.value == get() {
			chosen = i
		}
	}
	before := chosen
	form.Row(c, title, subtitle, func() {
		breeze.Segmented(c, &chosen, labels...).FontSize(12).Label(title)
	})
	if chosen != before && chosen >= 0 {
		set(options[chosen].value)
		save()
	}
}

// previewScene is the prototype's preview picture: sky, mountains and grass on a 16:7 tile.
var previewScene = icon(`<svg xmlns="http://www.w3.org/2000/svg" width="320" height="140" viewBox="0 0 320 140"><rect width="320" height="140" fill="#7FA8C2"/><path d="M0 90 L60 55 L110 75 L170 40 L230 70 L280 50 L320 66 L320 140 L0 140Z" fill="#5F8496"/><path d="M0 115 Q80 92 160 108 T320 100 L320 140 L0 140Z" fill="#3E6A5B"/></svg>`)

// lookPreview draws the sample cue over the prototype's landscape as the player would: the lines the display mode shows, in its order, with the faces of the look, as high above the bottom as the player puts them.
func (s screen) lookPreview(c *ui.Context, dark *ui.Theme) {
	look := s.Cue()
	lines := previewLines(*look, s.SubMode(), s.SubOrder())
	ui.Themed(c, dark).Shrink(0).AspectRatio(16.0/7.0).Margin(6, 0, 0, 0).Background(ui.Hex("#2B3A3F")).Radius(12).Shadow(0, 0, 0, 0.5, ui.RGBA(0, 0, 0, 0.1)).Clip().Label(locales.SettingsLookPreview()).Children(func() {
		ui.Image(c, previewScene).Absolute().Top(0).Start(0).WidthPercent(100).HeightPercent(100).Fit(ui.Cover).PassThrough()
		ui.Text(c, locales.SettingsLookPreviewTag()).Absolute().Top(8).Start(10).FontSize(10).FontWeight(700).TextColor(ui.Hex("#FFFFFF")).Background(ui.RGBA(0, 0, 0, 0.4)).Radius(6).Padding(2, 7).Role(ui.RoleNone)
		ui.Column(c).Absolute().Start(0).End(0).Bottom(look.PreviewInset()).AlignItems(ui.Center).Children(func() {
			if len(lines) == 0 {
				ui.Text(c, locales.SettingsLookPreviewOff()).FontSize(13).TextColor(ui.RGBA(255, 255, 255, 0.7))
				return
			}
			// The same block the player draws its subtitles in (film's playCue): a dark box behind the lines, or a shadow under each.
			block := ui.Column(c).Gap(2).Radius(8).AlignItems(ui.Center)
			if look.Box {
				block.Padding(8, 14).Background(ui.RGBA(0, 0, 0, 0.55))
			}
			block.Children(func() {
				for _, line := range lines {
					text := ui.Text(c, line.text).FontSize(line.face.Size).LineHeight(1.3).TextColor(ui.Hex(line.face.Color)).TextAlign(ui.Center).Wrap()
					if line.face.Bold {
						text.Bold()
					}
					if look.Shadow {
						text.Shadow(0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.95))
					}
				}
			})
		})
	})
}

// previewLine is a line of the preview and the face it is drawn in.
type previewLine struct {
	text string
	face subtitle.Face
}

// previewLines are the sample cue's lines the display mode shows, the one on top first. The sample is one bilingual line as a translated film shows it: the translation in the interface's language, the original in another (English, or French in the English interface).
func previewLines(look subtitle.Look, mode, order string) []previewLine {
	translation := previewLine{locales.SettingsLookPreviewTranslation(), look.Translation}
	original := previewLine{locales.SettingsLookPreviewOriginal(), look.Original}
	switch mode {
	case subtitle.Off:
		return nil
	case subtitle.Translation:
		return []previewLine{translation}
	case subtitle.Original:
		return []previewLine{original}
	}
	if order == subtitle.OriginalFirst {
		return []previewLine{original, translation}
	}
	return []previewLine{translation, original}
}
