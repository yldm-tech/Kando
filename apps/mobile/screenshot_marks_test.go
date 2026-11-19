package main

import (
	"testing"

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

// TestScreenshotsPlayerMarks renders the card of a line of Japanese with the word marks its computer computed (player-marks): readings over its words, a level's line in its tone under two of them and a dashed line under a word of the vocabulary. Without reporting the video's progress, which a target cannot, the card shows the first line.
func TestScreenshotsPlayerMarks(t *testing.T) {
	shot := screenshots(t)
	p, _ := openJapaneseMarked(t, &japaneseMarks)
	for _, ruby := range []string{"きょう", "がっこう", "い"} {
		p.app.WaitFor("id:player-card >> id:player-ruby >> text:"+ruby, wait)
	}
	if st := p.app.SpanStyle("id:player-card >> text:に"); st.DecorationThickness != 0 {
		t.Fatalf("に, unmarked, has a mark's line: %+v", st)
	}
	if st := p.app.SpanStyle("id:player-card >> text:った"); st.DecorationStyle != core.DecorationDashed || st.DecorationThickness == 0 {
		t.Fatalf("った, of a word of the vocabulary: %+v", st)
	}
	shot(p, "player-marks")
}
