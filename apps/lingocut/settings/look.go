package settings

import (
	"lingocut/apps/lingocut/film"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// The preview's sample cue, one bilingual line as a translated film shows it.
const (
	previewTranslation = "每条河都始于高山上的一股细流。"
	previewOriginal    = "Every river begins as a trickle high in the mountains."
)

// look is the 字幕外观 page: a preview of the subtitles in the player's look, and how the player shows them. dark is the player's theme, which the preview is drawn in.
func (s screen) look(c *ui.Context, t, dark *ui.Theme) {
	ui.Column(c).Gap(6).AlignItems(ui.Stretch).Children(func() {
		s.lookPreview(c, dark)
		breeze.Text(c, "字号、颜色和字体在播放时从侧栏的「字幕样式」调整，这里的改动和播放器同步生效。", breeze.TextCaption, breeze.TextSecondary).Padding(0, 10).Wrap()
	})
	breeze.PreferencesGroup(c, "", "", nil, func() {
		c.Key("look-mode")
		choice(c, "默认显示方式", "", s.SubMode, s.SetSubMode, []option{
			{film.SubOriginal, "仅原文"}, {film.SubTranslation, "仅译文"}, {film.SubBilingual, "双语"},
		}, s.Save)
		c.Key("look-order")
		choice(c, "双语顺序", "", s.SubOrder, s.SetSubOrder, []option{
			{film.SubTranslationFirst, "译文在上"}, {film.SubOriginalFirst, "原文在上"},
		}, s.Save)
		c.Key("look-untranslated")
		choice(c, "仅译文时，译文还没好", "译文到达后立即换上", s.Untranslated, s.SetUntranslated, []option{
			{film.UntranslatedDim, "弱化原文"}, {film.UntranslatedBlank, "留空"},
		}, s.Save)
		c.Key("look-yield")
		yield := s.YieldSubs()
		if breeze.SwitchRow(c, "控件出现时字幕上移让位", "避免被控制条挡住", &yield).Changed() {
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
	breeze.ActionRow(c, title, subtitle, func() {
		breeze.Segmented(c, &chosen, labels...).Label(title)
	})
	if chosen != before && chosen >= 0 {
		set(options[chosen].value)
		save()
	}
}

// lookPreview draws the sample cue on a dark tile as the player would: the lines the display mode shows, in its order, with the faces of the look.
func (s screen) lookPreview(c *ui.Context, dark *ui.Theme) {
	look := s.Cue()
	lines := previewLines(*look, s.SubMode(), s.SubOrder())
	ui.Themed(c, dark).Height(180).Background(dark.Background).Radius(12).Padding(12, 16, look.Bottom/3, 16).AlignItems(ui.Center).Justify(ui.End).Gap(4).Label("字幕预览").Children(func() {
		ui.Row(c).WidthPercent(100).Children(func() {
			breeze.Badge(c, "预览")
		})
		ui.Column(c).Grow(1)
		if len(lines) == 0 {
			breeze.Text(c, "字幕已关闭", breeze.TextSecondary)
			return
		}
		// The same block the player draws its subtitles in (film's playCue): a dark box behind the lines, or a shadow under each.
		block := ui.Column(c).Gap(2).Padding(8, 14).Radius(8).AlignItems(ui.Center)
		if look.Box {
			block.Background(ui.RGBA(0, 0, 0, 0.55))
		}
		block.Children(func() {
			for _, line := range lines {
				text := ui.Text(c, line.text).FontSize(line.face.Size).TextColor(ui.Hex(line.face.Color)).TextAlign(ui.Center).Wrap()
				if line.face.Bold {
					text.Bold()
				}
				if look.Shadow {
					text.Shadow(0, 1, 3, 0, ui.RGBA(0, 0, 0, 0.95))
				}
			}
		})
	})
}

// previewLine is a line of the preview and the face it is drawn in.
type previewLine struct {
	text string
	face film.Face
}

// previewLines are the sample cue's lines the display mode shows, the one on top first.
func previewLines(look film.Look, mode, order string) []previewLine {
	translation := previewLine{previewTranslation, look.Translation}
	original := previewLine{previewOriginal, look.Original}
	switch mode {
	case film.SubOff:
		return nil
	case film.SubTranslation:
		return []previewLine{translation}
	case film.SubOriginal:
		return []previewLine{original}
	}
	if order == film.SubOriginalFirst {
		return []previewLine{original, translation}
	}
	return []previewLine{translation, original}
}
