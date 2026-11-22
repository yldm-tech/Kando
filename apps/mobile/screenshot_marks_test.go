package main

import (
	"testing"

	"kando/apps/mobile/theme"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/core"
)

// japaneseMarks are the word marks a computer computes for japaneseLine (今日は学校に行った。): readings over 今日, 学校 and 行, a level's line under 学校 (accent) and 今日 (warn), and a dashed line under 行った, a word of the vocabulary.
var japaneseMarks = sync.FilmMarks{Lines: []sync.LineMarks{{Line: 0, Hash: sync.LineHash(japaneseLine), Marks: []sync.Mark{
	{Start: 0, End: 6, Tone: sync.ToneWarn},
	{Start: 0, End: 6, Ruby: "きょう"},
	{Start: 9, End: 15, Tone: sync.ToneAccent},
	{Start: 9, End: 15, Ruby: "がっこう"},
	{Start: 18, End: 27, Underline: true},
	{Start: 18, End: 21, Ruby: "い"},
}}}}

// TestScreenshotsPlayerMarks renders the card of a line of Japanese with the word marks its computer computed (player-marks): readings over its words, a level's faint band in its tone under two of them and a word of the vocabulary over a yellow band. Without reporting the video's progress, which a target cannot, the card shows the first line.
func TestScreenshotsPlayerMarks(t *testing.T) {
	shot := screenshots(t)
	p, _ := openJapaneseMarked(t, &japaneseMarks)
	for _, ruby := range []string{"きょう", "がっこう", "い"} {
		p.app.WaitFor("id:player-card >> id:player-ruby >> text:"+ruby, wait)
	}
	if sp := p.app.Span("id:player-card >> text:に"); sp.Background.Color != 0 || sp.Style.Decoration != 0 {
		t.Fatalf("に, unmarked, has a mark: %+v", sp)
	}
	// A word of the vocabulary is over a band of the subtitles' yellow, in the line's own ink.
	if sp := p.app.Span("id:player-card >> text:った"); sp.Background.Color != theme.SubtitleYellow || sp.Background.Top != core.Pct(62) || sp.Style.Color == core.RGB(0x000000) {
		t.Fatalf("った, of a word of the vocabulary: %+v", sp)
	}
	shot(p, "player-marks")
}
