package film

import (
	"fmt"
	"math"
	"slices"
	"time"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/core"
)

// DefaultLook is the prototype's: outlined, the translation 30 bold white over the original 22 yellow, at the bottom. A face's Family is the value saved in the settings (默认, 苹方, 冬青黑体, 黑体 or 宋体), not what the style view shows: fontName names it.
func DefaultLook() Look {
	return Look{
		Preset: "outline", Shadow: true,
		Translation: Face{Family: fontDefault, Size: 30, Color: "#FFFFFF", Bold: true},
		Original:    Face{Family: fontDefault, Size: 22, Color: "#FFD60A"},
	}
}

type styledLine struct {
	text string
	face Face
	// dim draws the line at 60%, as an original standing in for a missing translation.
	dim bool
}

// The subtitles' places, from the player's bottom: 44 points, or 30% of its height when High. While the controls show they yield to them by 60 points, or 4% of the height when High.
const (
	subsBottom     core.Points = 44
	subsYield      core.Points = 60
	subsHigh                   = 30
	subsHighYield              = 4
	subsDragMargin core.Points = 8
)

// subtitles draws the playing cue over the picture, centered above the transport, its lines as the mode picks them, or both for the cue the app peeks at (Peek), whatever the mode. While the controls show and the look yields to them, the lines move up out of their way; with the side panel open they keep to the room it leaves. The pointer drags the lines up and down, off a film's own burned-in subtitles (Look.Lift), and a double click puts dragged lines back; on lines in their place it enters or leaves full screen, as on the picture, and a click starts or holds the film. The lines are built again only when the playing cue or how it is drawn changes, as the clock moves on (tick).
func (s *screen) subtitles() core.Node {
	a, cx := s.a, s.cx
	cues := filmCues(cx, a)
	lines := core.NewMemoFunc(cx, func() []styledLine {
		a.Track()
		if a.Play() == nil {
			return nil
		}
		list := cues()
		i := cueIndex(list, s.playhead()-a.Delay())
		switch {
		case i < 0:
			return nil
		case i == a.Peek():
			return styledLines(bothLines{a}, list[i].Text)
		case a.SubMode() == SubOff:
			return nil
		}
		return pictureLines(a, list[i].Text)
	}, slices.Equal[[]styledLine]).Get
	return core.Show{When: func() bool { return len(lines()) > 0 }, Then: func(cx *core.Scope) core.Node { return s.subtitleLayer(cx, lines) }}
}

// subtitleLayer is the layer the lines sit in, at their place over the picture, and the block of them the pointer drags.
func (s *screen) subtitleLayer(cx *core.Scope, lines func() []styledLine) core.Node {
	a := s.a
	look := live.Memo(cx, a, func() Look { return *a.Cue() })
	yield := live.Memo(cx, a, a.YieldSubs)
	dragging := core.NewSignal(cx, false)
	drag := core.NewSignal(cx, core.Points(0))
	layerY := core.NewSignal(cx, core.Points(0))
	block := core.NewSignal(cx, core.Bounds{})
	// offset is how far up from their place the lines show, before a drag moves them: where they were dragged to, or for lines never dragged the yield to the controls while they show.
	offset := func() core.Points {
		h := s.size.Get().Height
		l := look()
		if l.Lift != 0 {
			return core.Points(l.Lift) * h
		}
		if s.shown() && yield() {
			if l.High {
				return h * subsHighYield / 100
			}
			return subsYield
		}
		return 0
	}
	// clamp keeps a drag of the lines inside the player, a margin from its top and its bottom, from their place as their layout put them.
	clamp := func(dy core.Points) core.Points {
		h, b := s.size.Get().Height, block.Get()
		top := layerY.Get() + b.Origin.Y
		up, down := top-subsDragMargin, h-subsDragMargin-top-b.Size.Height
		if up+down < 0 {
			return dy
		}
		return min(max(dy, -up), down)
	}
	pan := func(e core.PanEvent) {
		switch e.Phase {
		case core.GestureBegan:
			dragging.Set(true)
			drag.Set(clamp(-offset()))
		case core.GestureChanged:
			drag.Set(clamp(-offset() + e.Translation.Y))
		case core.GestureEnded, core.GestureCancelled:
			if h := s.size.Get().Height; h > 0 {
				a.Cue().Lift = float32(-clamp(-offset()+e.Translation.Y) / h)
				a.LookChanged()
			}
			dragging.Set(false)
			a.Changed()
		}
	}
	press := live.On(a, func(e core.PressEvent) {
		// The lines take the picture's clicks over them: a click starts or holds the film, and the two of a double click cancel out.
		s.togglePlay()
		if e.Clicks != 2 {
			return
		}
		if a.Cue().Lift != 0 {
			a.Cue().Lift = 0
			a.LookChanged()
			return
		}
		a.ToggleFullScreen()
	})
	body := core.Dynamic(func(cx *core.Scope) core.Node {
		l := look()
		children := []core.Node{}
		for _, line := range lines() {
			text := line.text
			children = append(children, subtitleLine(func() string { return text }, line, l.Box, l.Shadow))
		}
		return core.Group(children)
	})
	return core.View{TestID: "film-subtitles", PointerEvents: core.PointerEventsBoxNone,
		Style:    core.Style{Position: core.Absolute, Left: core.Pt(0), AlignItems: core.AlignCenter},
		OnLayout: func(b core.Bounds) { layerY.Set(b.Origin.Y) },
		StyleFunc: func() core.Style {
			room := s.roomRight()
			st := core.Style{Right: core.Pt(room), Padding: core.Symmetric(0, (s.size.Get().Width-room)*0.05), Transition: roomTransition}
			if look().High {
				st.Bottom = core.Pct(subsHigh)
			} else {
				st.Bottom = core.Pt(subsBottom)
			}
			return st
		},
		Children: []core.Node{core.View{
			TestID: "film-subtitle-lines", Role: core.RoleGroup,
			OnLayout: func(b core.Bounds) { block.Set(b) },
			OnPan:    pan,
			OnPress:  press,
			Style:    core.Style{MaxWidth: core.Pct(100), Gap: 4, AlignItems: core.AlignCenter},
			StyleFunc: func() core.Style {
				st := core.Style{}
				if dragging.Get() {
					st.Transform = core.Transform{TranslateY: drag.Get()}
					st.Cursor = core.CursorGrabbing
				} else {
					// The lines move to their new place as the controls come and go, as MyGo's Draggable sprang them there.
					st.Transform = core.Transform{TranslateY: -offset()}
					st.Transition = core.Transition{Duration: 250 * time.Millisecond, Easing: core.EaseOut, Properties: core.TransitionTransform}
					st.Cursor = core.CursorGrab
				}
				if !s.shown() {
					st.Cursor = core.CursorNone
				}
				return st
			},
			Children: []core.Node{body},
		}},
	}
}

// subtitleLine draws one line of a cue, on its own box with Box, outlined with Shadow. A view's shadow is a box's and would not follow the glyphs, so the outline is the line drawn again in black under it (outlineCopies), after the prototype's text-shadow (0 0 3px, 0 1px 2px and 0 0 1px).
func subtitleLine(text func() string, line styledLine, box, outline bool) core.Node {
	padX, padY := core.Points(0), core.Points(0)
	holder := core.Style{MaxWidth: core.Pct(100), CornerRadius: 6}
	if box {
		padX, padY = 12, 3
		holder.Padding = core.Symmetric(padY, padX)
		holder.Background = core.RGBA(0x000000, 0.62)
	}
	if line.dim {
		holder.Opacity = core.Alpha(0.6)
	}
	color, _ := core.ParseColor(line.face.Color)
	if !outline {
		return core.View{Style: holder, Children: []core.Node{lineText(text, line, core.Style{Color: color})}}
	}
	// Positioned views paint over those in the flow, so the line sizes the box from the flow unseen and is drawn again over its outline.
	children := []core.Node{lineText(text, line, core.Style{Color: core.RGBA(0x000000, 0)})}
	for _, o := range outlineCopies {
		st := core.Style{Position: core.Absolute, Left: core.Pt(padX + o.dx), Right: core.Pt(padX - o.dx), Top: core.Pt(padY + o.dy), Color: core.RGBA(0x000000, o.alpha)}
		if o.blur > 0 {
			st.Filters = []core.Filter{core.Blur(o.blur)}
		}
		children = append(children, lineText(text, line, st))
	}
	children = append(children, lineText(text, line, core.Style{Position: core.Absolute, Left: core.Pt(padX), Right: core.Pt(padX), Top: core.Pt(padY), Color: color}))
	return core.View{Style: holder, Children: children}
}

// outlineCopies are the black copies under an outlined line: a soft glow, the shadow below it, and eight hard copies a point around it, which make a stroke along the glyphs that holds over a bright picture.
var outlineCopies = []struct {
	dx, dy, blur core.Points
	alpha        float32
}{
	{0, 0, 3, 0.95},
	{0, 1, 2, 0.95},
	{-1, 0, 0, 0.9}, {1, 0, 0, 0.9}, {0, -1, 0, 0.9}, {0, 1, 0, 0.9},
	{-0.7, -0.7, 0, 0.9}, {0.7, -0.7, 0, 0.9}, {-0.7, 0.7, 0, 0.9}, {0.7, 0.7, 0, 0.9},
}

// lineText is the text of a subtitle line in its face, over extra. Every copy but the first, which sizes the line, is left out of what assistive technology reads.
func lineText(text func() string, line styledLine, extra core.Style) core.Node {
	st := core.Style{FontSize: core.Points(line.face.Size), LineHeightMultiple: 1.3, TextAlign: core.TextAlignCenter, FontFamily: fontFamily(line.face.Family)}
	if line.face.Bold {
		st.FontWeight = core.FontWeightBold
	}
	t := core.Text{ContentFunc: text, Style: st.Merge(extra)}
	if extra.Position == core.Absolute {
		t.Role = core.RoleNone
		t.PointerEvents = core.PointerEventsNone
	}
	return t
}

// lineSettings are the settings that pick a cue's lines and how they look.
type lineSettings interface {
	Cue() *Look
	SubMode() string
	SubOrder() string
	Untranslated() string
}

// pictureLines is the lines of a cue the picture shows. In SubTranslation mode a cue without a translation shows its original dimmed, or nothing when the setting leaves it blank.
func pictureLines(a lineSettings, text string) []styledLine {
	lines := styledLines(a, text)
	if a.SubMode() != SubTranslation {
		return lines
	}
	if translation, _ := splitCue(text); translation == "" && len(lines) > 0 {
		if a.Untranslated() == UntranslatedBlank {
			return nil
		}
		lines[0].dim = true
	}
	return lines
}

// bothLines are the settings with both lines of a cue showing, in their order, as for the cue peeked at.
type bothLines struct{ lineSettings }

func (bothLines) SubMode() string { return SubBilingual }

// listLines is a cue as the subtitle list shows it: shown, its lines as the picture shows them (styledLines), and hidden, the line the mode leaves off it, or "".
func listLines(a lineSettings, text string) (shown []string, hidden string) {
	for _, l := range styledLines(a, text) {
		shown = append(shown, l.text)
	}
	translation, original := splitCue(text)
	for _, line := range []string{translation, original} {
		if line != "" && !slices.Contains(shown, line) {
			hidden = line
		}
	}
	return shown, hidden
}

// styledLines is the lines of a cue as SubMode and SubOrder pick them, for the picture and the subtitle list. SubOff counts as bilingual here: it hides only the lines on the picture (subtitles), and the list keeps both.
func styledLines(a lineSettings, text string) []styledLine {
	translation, original := splitCue(text)
	add := func(lines []styledLine, s string, face Face) []styledLine {
		if s == "" {
			return lines
		}
		return append(lines, styledLine{text: s, face: face})
	}
	look := a.Cue()
	switch a.SubMode() {
	case SubTranslation:
		lines := add(nil, translation, look.Translation)
		if len(lines) == 0 {
			lines = add(lines, original, look.Original)
		}
		return lines
	case SubOriginal:
		return add(nil, original, look.Original)
	}
	if a.SubOrder() == SubOriginalFirst {
		return add(add(nil, original, look.Original), translation, look.Translation)
	}
	return add(add(nil, translation, look.Translation), original, look.Original)
}

// The font families a face can have, as the settings save them. They are data, not text: fontName names them in the interface's language. 默认 leaves the line in the system's font; 苹方 and 冬青黑体 are macOS's own Chinese fonts, the ones the style view offers there; 黑体 and 宋体 are a sans-serif and a serif Chinese font, the ones it offers on Windows and Linux.
const (
	fontDefault  = "默认"
	fontPingFang = "苹方"
	fontHiragino = "冬青黑体"
	fontSans     = "黑体"
	fontSerif    = "宋体"
)

// fontFamilies are the fonts that draw each saved family, a CSS list for a style's FontFamily: the family's own font first, then a font of the same kind on each system (macOS, Windows, Linux), so a family saved on one system still draws in its kind on another, and on a Linux without the first a common substitute does.
var fontFamilies = map[string]string{
	fontPingFang: "PingFang SC, Noto Sans CJK SC, Source Han Sans SC, Microsoft YaHei",
	fontHiragino: "Hiragino Sans GB, Noto Sans CJK SC, Source Han Sans SC, Microsoft YaHei",
	fontSans:     "Microsoft YaHei, Noto Sans CJK SC, Source Han Sans SC, WenQuanYi Micro Hei, PingFang SC",
	fontSerif:    "SimSun, Noto Serif CJK SC, Source Han Serif SC, AR PL UMing CN, Songti SC",
}

// fontFamily is the font list that draws the saved family name, or "" for 默认 and a family it does not know, which keep the system's font.
func fontFamily(name string) string {
	return fontFamilies[name]
}

// styleFonts are the families the style view offers on os, as the settings save them: macOS's own fonts on macOS, as the prototype has them, and elsewhere a sans-serif and a serif font, which look apart on every system that has Chinese fonts.
func styleFonts(os core.OS) []string {
	if os == core.OSMacOS {
		return []string{fontDefault, fontPingFang, fontHiragino}
	}
	return []string{fontDefault, fontSans, fontSerif}
}

// fontName is what the style view calls a saved font family in the language the interface shows under cx; a family it does not know is shown as saved.
func fontName(cx *core.Scope, family string) string { return fontNameIn(locales.For(cx), family) }

// fontNameIn is fontName in the language of m.
func fontNameIn(m locales.Messages, family string) string {
	switch family {
	case fontDefault:
		return m.FilmFontDefault()
	case fontPingFang:
		return m.FilmFontPingFang()
	case fontHiragino:
		return m.FilmFontHiragino()
	case fontSans:
		return m.FilmFontSans()
	case fontSerif:
		return m.FilmFontSerif()
	}
	return family
}

// DelayText writes a subtitle delay as the mode menu and the OSD show it, to a tenth of a second, in the language the interface shows under cx: "+0.3 秒", "-0.1 秒", "0.0 秒".
func DelayText(cx *core.Scope, d time.Duration) string { return DelayTextIn(locales.For(cx), d) }

// DelayTextIn is DelayText in the language of m, for a message on the picture.
func DelayTextIn(m locales.Messages, d time.Duration) string {
	v := math.Round(d.Seconds()*10) / 10
	switch {
	case v > 0:
		return m.FilmSeconds(fmt.Sprintf("+%.1f", v))
	case v == 0:
		return m.FilmSeconds("0.0")
	}
	return m.FilmSeconds(fmt.Sprintf("%.1f", v))
}
