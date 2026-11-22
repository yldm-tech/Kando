package player

import (
	"slices"
	"strings"
	"unicode/utf8"

	"kando/apps/mobile/theme"
	kcore "kando/packages/core"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// The word marks of the lines' originals, as the computer draws them (docs/plugin-system.md, E2): readings over the words written in Han characters, a thick line in a level's tone under the words of the word list chosen there, and the words in the vocabulary black on yellow. The computer computes them from its plugins and the phone draws them (session.FilmMarks), on the line over the picture and on the card; a line whose original is not the one they were computed for, as one fixed on the phone, shows none (sync.FilmMarks.Of). Settings › Subtitle Style turns them off (store.SubStyle.NoMarks).

// lineMarks are the marks of line i's original, nil for none. Read it in a binding.
func (p *play) lineMarks(i int) []sync.Mark {
	if m := p.marks.Get(); i >= 0 && i < len(m) {
		return m[i]
	}
	return nil
}

// sameMarks reports whether two films' marks are alike, line for line.
func sameMarks(a, b [][]sync.Mark) bool { return slices.EqualFunc(a, b, slices.Equal[[]sync.Mark]) }

// toneOf is the color of a mark's tone, the theme's role for each of a word list's tiers, false for none or one the phone does not know.
func toneOf(colors ui.Colors) func(tone string) (core.Color, bool) {
	return func(tone string) (core.Color, bool) {
		switch tone {
		case sync.ToneAccent:
			return colors.Accent(), true
		case sync.ToneWarn:
			return colors.Warn(), true
		case sync.ToneSuccess:
			return colors.Up(), true
		case sync.ToneMuted:
			return colors.Muted(), true
		}
		return 0, false
	}
}

// markInk is how markSpans draws a line: style is the line's text style, which a reading's view repeats, as an attachment does not take its text's; look looks up a word tapped; tone is the color of a mark's tone; word is how a word without a mark is drawn; picked is the word looked up in the line, "" for none, which is marked as a word of the vocabulary is; band draws a run the computer marked as a highlighter would, a faint band in its tone's color (the subtitles' yellow without one), as the card does, rather than a line under it.
type markInk struct {
	style  core.Style
	look   func(word string)
	tone   func(tone string) (core.Color, bool)
	word   core.Style
	picked string
	band   bool
}

// markBand is the highlighter's band of a run marked in band, in color at .38, over the lower 38% of each of its lines, as the design's linear-gradient(transparent 62%, color .38 62%).
func markBand(color core.Color) core.Style {
	return core.Style{Background: color.Faded(0.38), BackgroundInsetTop: bandTop}
}

// pickedWord is how a word of the vocabulary, and the word looked up, is marked: the subtitles' yellow over the lower 38% of each of its lines, under the line's own ink, as a highlighter would (the design's linear-gradient(transparent 62%, yellow 62%)).
var pickedWord = core.Style{Background: theme.SubtitleYellow, BackgroundInsetTop: bandTop}

// bandTop is where a highlighter's band starts down a line's box.
var bandTop = core.Pct(62)

// rubyScale is the size of a reading over a word, a share of the word's.
const rubyScale = 0.5

// markThickness and markOffset are the thickness of the line under a marked run and its room below the baseline, as shares of the font size, as the computer draws them.
var (
	markThickness = core.Pct(9)
	markOffset    = core.Pct(14)
)

// markSpans is text cut into spans at each of tokens, its words and what lies between them, a tap on a word looking it up, and at the ends of each of marks: a mark with a reading is a view of its reading over its characters, the others draw a line under their runs in a tone's color, and a word in the vocabulary, and the word looked up, is over a yellow band (pickedWord). A mark outside text, off a character's start, or overlapping a reading before it is left out; tokens of another text are too, as while a line changes.
func markSpans(text string, tokens []kcore.WordToken, marks []sync.Mark, ink markInk) []core.Span {
	inText := func(m sync.Mark) bool {
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
	var rubies, lines []sync.Mark
	for _, m := range marks {
		switch {
		case !inText(m):
		case m.Ruby != "":
			if len(rubies) == 0 || rubies[len(rubies)-1].End <= m.Start {
				rubies = append(rubies, m)
				cuts = append(cuts, m.Start, m.End)
			}
		case m.Tone != "" || m.Underline:
			lines = append(lines, m)
			cuts = append(cuts, m.Start, m.End)
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
		if i := slices.IndexFunc(words, func(w run) bool { return w.start <= start && start < w.end }); i >= 0 {
			word = words[i].word
		}
		var color *core.Color
		vocab, marked := false, false
		for _, m := range lines {
			if m.Start <= start && start < m.End {
				if c, ok := ink.tone(m.Tone); ok && color == nil {
					color = &c
				}
				vocab = vocab || m.Underline
				marked = marked || color != nil || m.Underline
			}
		}
		var st core.Style
		switch {
		case word != "" && strings.EqualFold(word, ink.picked), vocab:
			st = pickedWord
		case marked && ink.band:
			c := theme.SubtitleYellow
			if color != nil {
				c = *color
			}
			st = markBand(c)
		case marked:
			st.TextDecoration, st.TextDecorationThickness, st.TextUnderlineOffset = core.TextDecorationUnderline, markThickness, markOffset
			if color != nil {
				st.TextDecorationColor = *color
			}
		case word != "":
			st = ink.word
		}
		spans = append(spans, ink.span(text[start:end], ruby, word, st))
	}
	return spans
}

// span is the run content of a line: under its reading ruby when there is one, looking word up on a tap when it is one of the words, drawn in st.
func (ink markInk) span(content, ruby, word string, st core.Style) core.Span {
	var press func(core.PressEvent)
	if word != "" && ink.look != nil {
		press = func(core.PressEvent) { ink.look(word) }
	}
	if ruby == "" {
		return core.Span{Content: content, Style: st, OnPress: press}
	}
	base := core.Style{FontSize: ink.style.FontSize, FontWeight: ink.style.FontWeight, Color: ink.style.Color}
	small := core.Style{FontSize: ink.style.FontSize * rubyScale, Color: ink.style.Color}
	// The line puts a baseline-aligned view's first text on its baseline, so the run comes first, and the column is reversed to draw the reading over it: the run sits level with the characters around it, though it falls back to a font whose descent is larger than the line's.
	return core.Span{Content: content, Attachment: core.View{TestID: "player-ruby", Style: core.Style{Direction: core.ColumnReverse, AlignItems: core.AlignCenter, VerticalAlign: core.VerticalAlignBaseline}, OnPress: press, Children: []core.Node{
		// The run is a span of its text, so a band of st lies under its characters rather than filling the text's box.
		core.Text{Spans: []core.Span{{Content: content, Style: st}}, MaxLines: 1, Style: base},
		// The screen reader reads the run, not its reading.
		core.Text{Content: ruby, MaxLines: 1, Style: small, Role: core.RoleNone},
	}}}
}
