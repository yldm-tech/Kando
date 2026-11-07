package film

import (
	"fmt"
	"math"
	"time"

	"github.com/egoist/mygo/ui"
)

// DefaultLook is the prototype's: outlined, the translation 30 bold white over the original 22 yellow, at the bottom.
func DefaultLook() Look {
	return Look{
		Preset: "outline", Shadow: true,
		Translation: Face{Family: "默认", Size: 30, Color: "#FFFFFF", Bold: true},
		Original:    Face{Family: "默认", Size: 22, Color: "#FFD60A"},
	}
}

type styledLine struct {
	text string
	face Face
	// dim draws the line at 60%, as an original standing in for a missing translation.
	dim bool
}

// subtitles draws the playing cue over the picture, centered above the transport. While the controls show and the look yields to them, the lines move up out of their way; with the side panel open they keep to the room it leaves.
func (s screen) subtitles(c *ui.Context, chrome bool, panelSpace float32) {
	if s.Play() == nil || s.Subs().Key != s.OpenKey() || !s.Subs().Ready || s.SubMode() == SubOff {
		return
	}
	cue, ok := CueAt(s.Subs().Cues, s.Play().Head()-s.Delay())
	if !ok {
		return
	}
	lines := s.pictureLines(cue.Text)
	if len(lines) == 0 {
		return
	}
	look := s.Cue()
	right := panelSpace
	width, _ := c.Size()
	layer := ui.Column(c).Absolute().Left(0).Right(right).Padding(0, (width-right)*0.05).Gap(4).AlignItems(ui.Center).PassThrough()
	yield := chrome && s.YieldSubs()
	switch {
	case look.High && yield:
		layer.BottomPercent(34)
	case look.High:
		layer.BottomPercent(30)
	case yield:
		layer.Bottom(104)
	default:
		layer.Bottom(44)
	}
	layer.Children(func() {
		for _, line := range lines {
			subtitleLine(c, line, look.Box, look.Shadow)
		}
	})
}

// subtitleLine draws one line of a cue, on its own box with Box, outlined with Shadow. Element.Shadow is a box's shadow and would not follow the glyphs, so the outline is the line drawn again in black under it (outlineCopies), after the prototype's text-shadow (0 0 3px, 0 1px 2px and 0 0 1px).
func subtitleLine(c *ui.Context, line styledLine, box, outline bool) {
	padX, padY := float32(0), float32(0)
	holder := ui.Box(c).MaxWidthPercent(100).Radius(6)
	if box {
		padX, padY = 12, 3
		holder.Padding(padY, padX).Background(ui.RGBA(0, 0, 0, 0.62))
	}
	if line.dim {
		holder.Opacity(0.6)
	}
	holder.Children(func() {
		if outline {
			for _, o := range outlineCopies {
				lineText(c, line).Absolute().Left(padX + o.dx).Right(padX - o.dx).Top(padY + o.dy).TextColor(ui.RGBA(0, 0, 0, o.alpha)).Blur(o.blur).Role(ui.RoleNone)
			}
		}
		main := lineText(c, line).TextColor(ui.Hex(line.face.Color))
		if outline {
			// Positioned elements paint over those in the flow, so the line sizes the box from the flow unseen and is drawn again over its outline.
			main.TextColor(ui.Transparent).Role(ui.RoleNone)
			lineText(c, line).Absolute().Left(padX).Right(padX).Top(padY).TextColor(ui.Hex(line.face.Color))
		}
	})
}

// outlineCopies are the black copies under an outlined line: a soft glow, the shadow below it, and eight hard copies a point around it, which make a stroke along the glyphs that holds over a bright picture.
var outlineCopies = []struct{ dx, dy, blur, alpha float32 }{
	{0, 0, 3, 0.95},
	{0, 1, 2, 0.95},
	{-1, 0, 0, 0.9}, {1, 0, 0, 0.9}, {0, -1, 0, 0.9}, {0, 1, 0, 0.9},
	{-0.7, -0.7, 0, 0.9}, {0.7, -0.7, 0, 0.9}, {-0.7, 0.7, 0, 0.9}, {0.7, 0.7, 0, 0.9},
}

func lineText(c *ui.Context, line styledLine) ui.Element {
	text := ui.Text(c, line.text).FontSize(line.face.Size).LineHeight(1.3).TextAlign(ui.Center).Wrap()
	if line.face.Bold {
		text.Bold()
	}
	if family := fontFamily(line.face.Family); family != "" {
		text.Font(family)
	}
	return text
}

// pictureLines is the lines of a cue the picture shows. In SubTranslation mode a cue without a translation shows its original dimmed, or nothing when the setting leaves it blank.
func (s screen) pictureLines(text string) []styledLine {
	lines := s.styledLines(text)
	if s.SubMode() != SubTranslation {
		return lines
	}
	if translation, _ := splitCue(text); translation == "" && len(lines) > 0 {
		if s.Untranslated() == UntranslatedBlank {
			return nil
		}
		lines[0].dim = true
	}
	return lines
}

// styledLines is the lines of a cue as SubMode and SubOrder pick them, for the picture and the subtitle list. SubOff counts as bilingual here: it hides only the lines on the picture (subtitles), and the list keeps both.
func (s screen) styledLines(text string) []styledLine {
	translation, original := splitCue(text)
	add := func(lines []styledLine, s string, face Face) []styledLine {
		if s == "" {
			return lines
		}
		return append(lines, styledLine{text: s, face: face})
	}
	switch s.SubMode() {
	case SubTranslation:
		lines := add(nil, translation, s.Cue().Translation)
		if len(lines) == 0 {
			lines = add(lines, original, s.Cue().Original)
		}
		return lines
	case SubOriginal:
		return add(nil, original, s.Cue().Original)
	}
	if s.SubOrder() == SubOriginalFirst {
		return add(add(nil, original, s.Cue().Original), translation, s.Cue().Translation)
	}
	return add(add(nil, translation, s.Cue().Translation), original, s.Cue().Original)
}

func fontFamily(name string) string {
	switch name {
	case "苹方":
		return "PingFang SC"
	case "冬青黑体":
		return "Hiragino Sans GB"
	default:
		return ""
	}
}

// DelayText writes a subtitle delay as the mode menu and the OSD show it, to a tenth of a second: "+0.3 秒", "-0.1 秒", "0.0 秒".
func DelayText(d time.Duration) string {
	v := math.Round(d.Seconds()*10) / 10
	switch {
	case v > 0:
		return fmt.Sprintf("+%.1f 秒", v)
	case v == 0:
		return "0.0 秒"
	}
	return fmt.Sprintf("%.1f 秒", v)
}
