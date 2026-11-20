package film

import (
	"fmt"
	"math"
	"slices"
	"time"
	"unicode/utf8"

	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/lookup"
	kcore "kando/packages/core"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// DefaultLook is the prototype's: outlined, the translation 30 bold white over the original 22 yellow, at the bottom. A face's Family is the value saved in the settings (默认, 苹方, 冬青黑体, 黑体 or 宋体), not what the style view shows: subtitle.FontName names it.
func DefaultLook() Look {
	return Look{
		Preset: "outline", Shadow: true,
		Translation: Face{Family: subtitle.FontDefault, Size: 30, Color: "#FFFFFF", Bold: true},
		Original:    Face{Family: subtitle.FontDefault, Size: 22, Color: "#FFD60A"},
	}
}

type styledLine struct {
	text string
	face Face
	// dim draws the line at 60%, as an original standing in for a missing translation.
	dim bool
	// original marks the cue's original, whose words a click looks up (LookUpWord); cue is the index of the cue the line is of, set for the lines on the picture.
	original bool
	cue      int
}

// The subtitles' places, from the player's bottom: 44 points, or 30% of its height when High. While the controls show they yield to them by 60 points, or 4% of the height when High. A drag and the keys that move them keep them 8 points inside the picture, and the keys move them by 2% of its height a press.
const (
	subsBottom     core.Points = 44
	subsYield      core.Points = 60
	subsHigh                   = 30
	subsHighYield              = 4
	subsDragMargin core.Points = 8
	subsLiftStep   float32     = 0.02
)

// SubRoom is where the lines on the picture show and may go, in SubLift's measure, a share of the picture's height up from the place their look puts them: Rest is where they show while the film has no place of its own, up out of the way of the controls while those show, else 0; Lo and Hi are as far down and up as a drag takes them, a margin inside the picture, Lo above Hi for lines too tall for the picture, which a drag moves freely. The zero SubRoom is that of lines not laid out yet.
type SubRoom struct{ Rest, Lo, Hi float32 }

// NudgeLift is where one press of Move Subtitles Up (dir 1) or Down (dir -1) takes the lines from lift, the film's place or 0 for none, within room: on from where they show, so the first press on lines yielding to the controls moves them from there, and no further than a drag would. Lines not laid out yet stay where they are.
func NudgeLift(lift float32, dir int, room SubRoom) float32 {
	if room == (SubRoom{}) {
		return lift
	}
	from := lift
	if from == 0 {
		from = room.Rest
	}
	next := from + float32(dir)*subsLiftStep
	if room.Lo <= room.Hi {
		next = min(max(next, room.Lo), room.Hi)
	}
	return next
}

// subtitles draws the playing cue over the picture, centered above the transport, its lines as the mode picks them, or both for the cue the app peeks at (Peek), whatever the mode, and above them the line of the film's reference layer playing, while it shows (Referencer). While the controls show and the look yields to them, the lines move up out of their way; with the side panel open they keep to the room it leaves. The pointer drags the lines up and down, off a film's own burned-in subtitles, to a place the film keeps (SubLift), and a double click puts dragged lines back; on lines in their place it enters or leaves full screen, as on the picture, and a click starts or holds the film. The once-only tip that says so (SubTip) shows over them, and alone where they go between two lines. The lines are built again only when the playing cue or how it is drawn changes, as the clock moves on (tick).
func (s *screen) subtitles() core.Node {
	a, cx := s.a, s.cx
	cues := filmCues(cx, a)
	reference := core.NewMemoFunc(cx, live.Func(a, a.Reference), sameCues).Get
	lines := core.NewMemoFunc(cx, func() []styledLine {
		a.Track()
		if a.Play() == nil {
			return nil
		}
		list := cues()
		i := s.playingCue(list)
		var out []styledLine
		// Subtitles turned off hide the reference layer too, between lines as on them, unless a line is peeked at.
		off := a.SubMode() == SubOff
		switch {
		case i < 0:
			if off {
				return nil
			}
		case i == a.Peek():
			out = ofCue(styledLines(bothLines{a}, list[i].Text), i)
		case off:
			return nil
		default:
			out = ofCue(pictureLines(s.lineSettings(), list[i].Text), i)
		}
		// The reference layer's line, while it shows, goes above the film's own, or alone where they have none.
		if a.ShowReference() {
			if line, ok := referenceLine(a.Cue(), reference(), s.playhead()-a.Delay()); ok {
				out = append([]styledLine{line}, out...)
			}
		}
		return out
	}, slices.Equal[[]styledLine]).Get
	tip := live.Memo(cx, a, a.SubTip)
	return core.Show{When: func() bool { return len(lines()) > 0 || tip() }, Then: func(cx *core.Scope) core.Node { return s.subtitleLayer(cx, lines, tip) }}
}

// subtitleLayer is the layer the lines sit in, at their place over the picture, the block of them the pointer drags, which shows a backdrop while the pointer is over it so it reads as something to take hold of, and the tip above them while it shows.
func (s *screen) subtitleLayer(cx *core.Scope, lines func() []styledLine, tip func() bool) core.Node {
	a := s.a
	colors := ui.ColorsOf(cx)
	look := live.Memo(cx, a, func() Look { return *a.Cue() })
	yield := live.Memo(cx, a, a.YieldSubs)
	lift := live.Memo(cx, a, a.SubLift)
	dragging := core.NewSignal(cx, false)
	hovered := core.NewSignal(cx, false)
	drag := core.NewSignal(cx, core.Points(0))
	layerH := core.NewSignal(cx, core.Points(0))
	block := core.NewSignal(cx, core.Bounds{})
	// place is how far up from the player's bottom the look puts the lines' layer.
	place := func() core.Points {
		if look().High {
			return s.size.Get().Height * subsHigh / 100
		}
		return subsBottom
	}
	// rest is how far up from their place lines the film has not moved show: out of the way of the controls while they show, if the look yields to them.
	rest := func() core.Points {
		if s.shown() && yield() {
			if look().High {
				return s.size.Get().Height * subsHighYield / 100
			}
			return subsYield
		}
		return 0
	}
	// offset is how far up from their place the lines show, before a drag moves them: where the film has them (SubLift), which stays while the controls show, or else rest.
	offset := func() core.Points {
		if l := lift(); l != 0 {
			return core.Points(l) * s.size.Get().Height
		}
		return rest()
	}
	// room is how far a drag can take the lines, up and down from their place, a margin inside the player, and ok false when they are too tall for it.
	room := func() (up, down core.Points, ok bool) {
		h, b := s.size.Get().Height, block.Get()
		top := h - place() - layerH.Get() + b.Origin.Y
		up, down = top-subsDragMargin, h-subsDragMargin-top-b.Size.Height
		return up, down, up+down >= 0
	}
	// clamp keeps a drag of the lines inside the player.
	clamp := func(dy core.Points) core.Points {
		up, down, ok := room()
		if !ok {
			return dy
		}
		return min(max(dy, -up), down)
	}
	// The keys that move the lines move them from where they show, within the room a drag has, which only the layout knows (SetSubRoom). The tip alone between two lines, over no lines, leaves the room the last lines had.
	measured := core.NewMemo(cx, func() SubRoom {
		h := s.size.Get().Height
		if h <= 0 || len(lines()) == 0 {
			return SubRoom{}
		}
		up, down, ok := room()
		r := SubRoom{Rest: float32(rest() / h), Lo: float32(-down / h), Hi: float32(up / h)}
		if !ok {
			r.Lo, r.Hi = 1, -1
		}
		return r
	}).Get
	cx.Effect(func() {
		if r := measured(); r != (SubRoom{}) {
			a.SetSubRoom(r)
		}
	})
	pan := func(e core.PanEvent) {
		switch e.Phase {
		case core.GestureBegan:
			dragging.Set(true)
			drag.Set(clamp(-offset()))
		case core.GestureChanged:
			drag.Set(clamp(-offset() + e.Translation.Y))
		case core.GestureEnded, core.GestureCancelled:
			if h := s.size.Get().Height; h > 0 {
				a.SetSubLift(float32(-clamp(-offset()+e.Translation.Y) / h))
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
		if a.SubLift() != 0 {
			a.SetSubLift(0)
			return
		}
		a.ToggleFullScreen()
	})
	// The words of a Japanese or Chinese line are cut by a dictionary read in the background the first time a line needs it (lookup.Tokens); until then the line has no words to click. cut counts the dictionaries read since, so the lines' words are cut again once one is.
	cut := core.NewSignal(cx, 0)
	loaded := func() { cx.Post(func() { cut.Set(cut.GetUntracked() + 1) }) }
	tokens := func(text string) []lookup.Token {
		cut.Get()
		return lookup.Tokens(text, loaded)
	}
	// The tones of the marks of a word list's tiers are the theme's roles, which color the lines drawn under the words.
	tone := func(t string) (core.Color, bool) { return toneColor(colors, t) }
	// A click on a word of the original looks it up, holding the film (LookUpWord); a click elsewhere on the lines is the picture's, as above. The original's words are drawn with the marks the app computed for them (CueMarks): readings over them, a thick line in the color of their tier under those of a word list, a dashed one under those in the vocabulary.
	body := core.Dynamic(func(cx *core.Scope) core.Node {
		l := look()
		children := []core.Node{}
		for _, line := range lines() {
			text := line.text
			var words *lineWords
			if line.original {
				cue := line.cue
				marks := core.NewMemoFunc(cx, live.Func(a, func() []subtitle.Mark { return a.CueMarks(cue) }), slices.Equal[[]subtitle.Mark]).Get
				words = &lineWords{look: live.On(a, func(word string) { a.LookUpWord(cue, word) }), press: press, tokens: tokens, marks: marks, tone: tone}
			}
			children = append(children, subtitleLine(func() string { return text }, line, l.Box, l.Shadow, words))
		}
		return core.Group(children)
	})
	// The tip is text alone, which takes neither the pointer nor the keyboard's focus.
	tipNode := core.Show{When: tip, Then: func(cx *core.Scope) core.Node {
		return core.View{TestID: "film-subtitle-tip", PointerEvents: core.PointerEventsNone, LiveRegion: core.LiveRegionPolite,
			Style:     core.Style{Margin: core.Edges{Bottom: 10}, Padding: core.Symmetric(6, 12), CornerRadius: 8, BorderWidth: 1},
			StyleFunc: func() core.Style { return core.Style{Background: colors.Raised(), BorderColor: colors.Border()} },
			Children: []core.Node{core.Text{ContentFunc: func() string { return locales.FilmSubtitleTip(cx) },
				Style: core.Style{FontSize: 13, LineHeight: 18, TextAlign: core.TextAlignCenter}, StyleFunc: ui.Colored(colors.Text)}}}
	}}
	// The layer, and the tip in it with the lines, goes up from its place as far as the lines show from it: under the pointer while it drags them, else where they belong, to which they move as the controls and the side panel come and go, as MyGo's Draggable sprang them there. It moves by its frame, laid out again at each step of a drag, rather than by a transform, so a drop springs the lines back from where they were dropped with the one transition on their place.
	return core.View{TestID: "film-subtitles", PointerEvents: core.PointerEventsBoxNone,
		Style:    core.Style{Position: core.Absolute, Left: core.Pt(0), AlignItems: core.AlignCenter},
		OnLayout: func(b core.Bounds) { layerH.Set(b.Size.Height) },
		StyleFunc: func() core.Style {
			room := s.roomRight()
			st := core.Style{Right: core.Pt(room), Padding: core.Symmetric(0, (s.size.Get().Width-room)*0.05)}
			if dragging.Get() {
				st.Bottom = core.Pt(place() - drag.Get())
			} else {
				st.Bottom = core.Pt(place() + offset())
				st.Transition = roomTransition
			}
			return st
		},
		Children: []core.Node{tipNode, core.View{
			TestID: "film-subtitle-lines", Role: core.RoleGroup,
			OnLayout:     func(b core.Bounds) { block.Set(b) },
			OnPan:        pan,
			OnPress:      press,
			OnMouseEnter: func(core.MouseEvent) { hovered.Set(true) },
			OnMouseLeave: func(core.MouseEvent) { hovered.Set(false) },
			// The backdrop's padding is taken back by the margin, so it shows around the lines without moving them.
			Style: core.Style{MaxWidth: core.Pct(100), Gap: 4, AlignItems: core.AlignCenter, Padding: core.Symmetric(6, 14), Margin: core.Edges{Top: -6, Right: -14, Bottom: -6, Left: -14}, CornerRadius: 10},
			StyleFunc: func() core.Style {
				st := core.Style{Cursor: core.CursorGrab}
				switch {
				case !s.shown():
					st.Cursor = core.CursorNone
				case dragging.Get():
					st.Cursor, st.Background = core.CursorGrabbing, colors.Pressed()
				case hovered.Get():
					st.Background = colors.Hover()
				}
				return st
			},
			Children: []core.Node{body},
		}},
	}
}

// toneColor is the color of the tone of a word list's tier (subtitle.ToneAccent and the others), a role of the theme colors read, false for none.
func toneColor(colors ui.Colors, t string) (core.Color, bool) {
	switch t {
	case subtitle.ToneAccent:
		return colors.Accent(), true
	case subtitle.ToneWarn:
		return colors.Warn(), true
	case subtitle.ToneSuccess:
		return colors.Up(), true
	case subtitle.ToneMuted:
		return colors.Muted(), true
	}
	return 0, false
}

// lineWords is what a click on a line's words does: look looks up the word clicked, and press is the click on the rest of the line; tokens cuts a line into its words, in a binding, as lookup.Tokens does. marks are the line's marks (CueMarks), read in a binding, and tone the color of a mark's tone, false for none.
type lineWords struct {
	look   func(word string)
	press  func(core.PressEvent)
	tokens func(text string) []lookup.Token
	marks  func() []subtitle.Mark
	tone   func(tone string) (core.Color, bool)
}

// subtitleLine draws one line of a cue, on its own box with Box, outlined with Shadow. A view's shadow is a box's and would not follow the glyphs, so the outline is the line drawn again in black under it (outlineCopies), after the prototype's text-shadow (0 0 3px, 0 1px 2px and 0 0 1px). With words, the line drawn on top takes the clicks: a click on one of its words looks it up. Readings over its words (lineWords.marks) are drawn in every copy, so the copies under it lay out as it does.
func subtitleLine(text func() string, line styledLine, box, outline bool, words *lineWords) core.Node {
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
		return core.View{Style: holder, Children: []core.Node{withWords(lineText(text, line, core.Style{Color: color}), text, words)}}
	}
	// Positioned views paint over those in the flow, so the line sizes the box from the flow unseen and is drawn again over its outline.
	children := []core.Node{withReadings(lineText(text, line, core.Style{Color: core.RGBA(0x000000, 0)}), text, words)}
	for _, o := range outlineCopies {
		st := core.Style{Position: core.Absolute, Left: core.Pt(padX + o.dx), Right: core.Pt(padX - o.dx), Top: core.Pt(padY + o.dy), Color: core.RGBA(0x000000, o.alpha)}
		if o.blur > 0 {
			st.Filters = []core.Filter{core.Blur(o.blur)}
		}
		children = append(children, withReadings(lineText(text, line, st), text, words))
	}
	children = append(children, withWords(lineText(text, line, core.Style{Position: core.Absolute, Left: core.Pt(padX), Right: core.Pt(padX), Top: core.Pt(padY), Color: color}), text, words))
	return core.View{Style: holder, Children: children}
}

// withWords has the text t of a line take the clicks on it with words: its words are spans a click looks up, which the pointer shows as such, and a click between them is words.press; its marks are drawn on them (lineSpans). Without words t is as it was.
func withWords(t core.Text, text func() string, words *lineWords) core.Text {
	if words == nil {
		return t
	}
	t.PointerEvents = core.PointerEventsAuto
	t.OnPress = words.press
	style, hidden := t.Style, t.Role == core.RoleNone
	t.SpansFunc = func() []core.Span {
		var marks []subtitle.Mark
		if words.marks != nil {
			marks = words.marks()
		}
		return lineSpans(text(), words.tokens(text()), marks, spanInk{style: style, hidden: hidden, look: words.look, tone: words.tone})
	}
	return t
}

// withReadings gives t, a copy of a line under the one on top, the readings of words' marks, so it lays out as the line on top does; it takes no clicks and draws no tone. Without words, or while no mark has a reading, t is drawn from its content, as it was.
func withReadings(t core.Text, text func() string, words *lineWords) core.Text {
	if words == nil || words.marks == nil {
		return t
	}
	style := t.Style
	hidden := t.Role == core.RoleNone
	t.SpansFunc = func() []core.Span {
		marks := words.marks()
		if !slices.ContainsFunc(marks, func(m subtitle.Mark) bool { return m.Ruby != "" }) {
			return nil
		}
		return lineSpans(text(), nil, marks, spanInk{style: style, hidden: hidden})
	}
	return t
}

// spanInk is how lineSpans draws a line's runs: style is the line's text style, which a reading's view repeats, as an attachment does not take its text's, and hidden is set for a copy assistive technology does not read (core.RoleNone); with look, the line on top, a click on a word looks it up and tone is the color of the line under the runs of a mark's tone, which the copies under it do not draw.
type spanInk struct {
	style  core.Style
	hidden bool
	look   func(word string)
	tone   func(tone string) (core.Color, bool)
}

// rubyScale is the size of a reading over a word, a share of the word's.
const rubyScale = 0.5

// lineSpans is text cut into spans: at each of tokens, its words and what lies between them, which ink.look looks up on a click, and at each mark with a reading, which is a view of its reading over its characters (rubySpan). The other marks draw a line under their runs, cut at the marks' own ends, on the line on top (runStyle). A mark outside text, off a character's start, or overlapping a reading before it is left out; tokens of another text are too, as while a line changes.
func lineSpans(text string, tokens []lookup.Token, marks []subtitle.Mark, ink spanInk) []core.Span {
	inText := func(m subtitle.Mark) bool {
		return m.Start >= 0 && m.Start < m.End && m.End <= len(text) && utf8.RuneStart(text[m.Start]) && (m.End == len(text) || utf8.RuneStart(text[m.End]))
	}
	cuts := []int{0, len(text)}
	type run struct {
		start, end int
		word       string
	}
	var words []run
	at := 0
	for _, t := range tokens {
		if t.Word {
			words = append(words, run{at, at + len(t.Text), t.Text})
		}
		at += len(t.Text)
		cuts = append(cuts, at)
	}
	if at != len(text) {
		words, cuts = nil, []int{0, len(text)}
	}
	var rubies, tones []subtitle.Mark
	for _, m := range marks {
		switch {
		case !inText(m):
		case m.Ruby != "":
			if len(rubies) == 0 || rubies[len(rubies)-1].End <= m.Start {
				rubies = append(rubies, m)
				cuts = append(cuts, m.Start, m.End)
			}
		case m.Tone != "" || m.Underline:
			// A tone's run is cut at its own ends, as the tokens need not end where the marks do (segment.Words and segment.Annotated may cut a line differently); a copy under the line draws no tone, so it is not cut.
			tones = append(tones, m)
			if ink.look != nil {
				cuts = append(cuts, m.Start, m.End)
			}
		}
	}
	slices.Sort(cuts)
	cuts = slices.Compact(cuts)
	var spans []core.Span
	next := 0
	for k := 0; k+1 < len(cuts); {
		start, end, ruby := cuts[k], cuts[k+1], ""
		if next < len(rubies) && rubies[next].Start == start {
			end, ruby = rubies[next].End, rubies[next].Ruby
			next++
		}
		for k+1 < len(cuts) && cuts[k] < end {
			k++
		}
		word := ""
		if i := slices.IndexFunc(words, func(w run) bool { return w.start <= start && start < w.end }); i >= 0 && ink.look != nil {
			word = words[i].word
		}
		var rs runStyle
		for _, m := range tones {
			if m.Start <= start && start < m.End {
				if c, ok := ink.toneOf(m.Tone); ok && rs.color == nil {
					rs.color = &c
				}
				rs.underline = rs.underline || m.Underline
			}
		}
		if ink.look == nil {
			rs = runStyle{}
		}
		spans = append(spans, ink.span(text[start:end], ruby, word, rs))
	}
	return spans
}

// runStyle is how a run of a line is drawn besides its line's style: with a line under it in the color of a mark's tone, when one has it, and with a dashed one when it is in the vocabulary (underline), in the tone's color too when it has one and the text's otherwise. The text keeps the color of its line, which the learner chose, so a tone shows on any of them, and the two marks never look alike: solid for a level, dashed for a word in the vocabulary.
type runStyle struct {
	color     *core.Color
	underline bool
}

// markThickness and markOffset are the thickness of the line under a marked run and its room below the baseline, as shares of the font size, thick enough for its color to show over the picture.
var (
	markThickness = core.Pct(9)
	markOffset    = core.Pct(14)
)

// toneOf is the color of tone, false for none or without ink.tone.
func (ink spanInk) toneOf(tone string) (core.Color, bool) {
	if ink.tone == nil || tone == "" {
		return 0, false
	}
	return ink.tone(tone)
}

// span is the run content of a line: under its reading ruby when there is one (rubyView), looking word up on a click when it is one of the words, drawn in rs.
func (ink spanInk) span(content, ruby, word string, rs runStyle) core.Span {
	var st core.Style
	if rs.color != nil || rs.underline {
		st.TextDecoration, st.TextDecorationThickness, st.TextUnderlineOffset = core.TextDecorationUnderline, markThickness, markOffset
	}
	if rs.color != nil {
		st.TextDecorationColor = *rs.color
	}
	if rs.underline {
		st.TextDecorationStyle = core.DecorationDashed
	}
	var press func(core.PressEvent)
	if word != "" {
		press = func(core.PressEvent) { ink.look(word) }
		st.Cursor = core.CursorPointer
	}
	if ruby == "" {
		return core.Span{Content: content, Style: st, OnPress: press}
	}
	// The view sets its own place in the line (rubyView), so the span sets none.
	return core.Span{Content: content, Attachment: core.Func(func(cx *core.Scope) core.Node {
		return ink.rubyView(content, ruby, st, press, core.NewSignal(cx, core.Points(-1)))
	})}
}

// rubyView is the view of a run of a line, content, with its reading ruby over it, in the line's text style with extra over it (the line under it). Without press it takes no pointer, as in the copies under the line; assistive technology reads the run, not its reading, and nothing of a hidden copy.
//
// The run's baseline sits on the line's. drop is how far below the run's baseline its text reaches, as its layout reports it (rubyDrop), -1 until then: the run's text then hangs that far below the view, whose bottom, its baseline, the line puts on its own (VerticalAlignBaseline). Until the run is laid out the view sits on the line's text bottom (VerticalAlignTextBottom), as NSGo puts an attachment by the descent of the line's font, not of the fonts its text falls back to: a run in Han characters, drawn in the CJK font the system font falls back to, whose descent is larger, would sit above the characters around it, and its line under it above theirs.
func (ink spanInk) rubyView(content, ruby string, extra core.Style, press func(core.PressEvent), drop *core.Signal[core.Points]) core.View {
	// TODO(nsgo#1038): an attachment view with VerticalAlignBaseline cannot put its first text's baseline on the line's, only an Em canvas can (core/commit.go baselineDescent); once it can, drop rubyDrop, the drop signal, the negative bottom margin and the VerticalAlignTextBottom fallback https://github.com/nsgo-dev/nsgo/issues/1038
	base := core.Style{FontSize: ink.style.FontSize, FontFamily: ink.style.FontFamily, FontWeight: ink.style.FontWeight, Color: ink.style.Color}.Merge(extra)
	small := core.Style{FontSize: ink.style.FontSize * rubyScale, FontFamily: ink.style.FontFamily, Color: base.Color}
	// The text's height and its line, as its layout reports them, in either order.
	var height core.Points
	var line *core.TextLine
	measured := func() {
		if height > 0 && line != nil {
			drop.Set(rubyDrop(height, *line))
		}
	}
	text := core.Text{Content: content, MaxLines: 1, Style: base, TestID: "film-ruby-run",
		OnLayout: func(b core.Bounds) { height = b.Size.Height; measured() },
		OnTextLayout: func(lines []core.TextLine) {
			if len(lines) > 0 {
				l := lines[0]
				line = &l
				measured()
			}
		},
		StyleFunc: func() core.Style {
			if d := drop.Get(); d > 0 {
				return core.Style{Margin: core.Edges{Bottom: -d}}
			}
			return core.Style{}
		},
	}
	if ink.hidden {
		text.Role = core.RoleNone
	}
	v := core.View{Style: core.Style{AlignItems: core.AlignCenter, Cursor: extra.Cursor}, OnPress: press,
		StyleFunc: func() core.Style {
			if drop.Get() < 0 {
				return core.Style{VerticalAlign: core.VerticalAlignTextBottom}
			}
			return core.Style{VerticalAlign: core.VerticalAlignBaseline}
		},
		Children: []core.Node{core.Text{Content: ruby, MaxLines: 1, Style: small, Role: core.RoleNone}, text},
	}
	if press == nil {
		v.PointerEvents = core.PointerEventsNone
	}
	return v
}

// rubyDrop is how far a text of one line, l, height high, reaches below the line's baseline. A line's box is its ascent, descent and leading high, half the leading over its ascent, as Apple's text lays it out (Linux's and Windows' have none), so its baseline is (the line's height + ascent - descent) / 2 below the line's top.
func rubyDrop(height core.Points, l core.TextLine) core.Points {
	baseline := l.Bounds.Origin.Y + (l.Bounds.Size.Height+l.Ascender-l.Descender)/2
	return max(height-baseline, 0)
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
func lineText(text func() string, line styledLine, extra core.Style) core.Text {
	st := core.Style{FontSize: core.Points(line.face.Size), LineHeightMultiple: 1.3, TextAlign: core.TextAlignCenter, FontFamily: subtitle.FontFamily(line.face.Family)}
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

// pictureLines is the lines of a cue the picture shows (kcore.PictureLines). In SubTranslation mode a cue without a translation shows its original dimmed, or nothing when the setting leaves it blank.
func pictureLines(a lineSettings, text string) []styledLine {
	translation, original := splitCue(text)
	return faced(a.Cue(), kcore.PictureLines(a.SubMode(), a.SubOrder(), a.Untranslated(), translation, original))
}

// ownLanguage are the line settings of a film heard in the language its subtitles are for (Item.SameLanguage), whose cues are its own lines alone: a mode that would show a translation shows them as 原文 does, rather than dimmed as lines not translated yet.
type ownLanguage struct{ lineSettings }

func (o ownLanguage) SubMode() string { return ownMode(o.lineSettings.SubMode()) }

// ownMode is the mode a film heard in the language its subtitles are for shows its lines in when mode is chosen: 原文 for any mode that shows them.
func ownMode(mode string) string {
	if mode == SubTranslation || mode == SubBilingual {
		return SubOriginal
	}
	return mode
}

// lineSettings are the settings the open film's lines show by: the player's, or for a film in its own language, ownLanguage's.
func (s *screen) lineSettings() lineSettings {
	if s.item().SameLanguage {
		return ownLanguage{s.a}
	}
	return s.a
}

// subMode is the mode the open film's lines show in (lineSettings).
func (s *screen) subMode() string { return s.lineSettings().SubMode() }

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

// styledLines is the lines of a cue as SubMode and SubOrder pick them (kcore.CueLines), for the picture and the subtitle list. SubOff counts as bilingual here: it hides only the lines on the picture (subtitles), and the list keeps both.
func styledLines(a lineSettings, text string) []styledLine {
	translation, original := splitCue(text)
	return faced(a.Cue(), kcore.CueLines(a.SubMode(), a.SubOrder(), translation, original))
}

// faced gives each line its face in look: the original's or the translation's.
func faced(look *Look, lines []kcore.CueLine) []styledLine {
	if len(lines) == 0 {
		return nil
	}
	out := make([]styledLine, len(lines))
	for i, l := range lines {
		face := look.Translation
		if l.Original {
			face = look.Original
		}
		out[i] = styledLine{text: l.Text, face: face, dim: l.Dim, original: l.Original}
	}
	return out
}

// ofCue marks lines as those of cue i.
func ofCue(lines []styledLine, i int) []styledLine {
	for k := range lines {
		lines[k].cue = i
	}
	return lines
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
