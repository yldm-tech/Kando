package core

import (
	"strings"
	"testing"
	"time"
)

func ms(n int) time.Duration { return time.Duration(n) * time.Millisecond }

func corrected(start, end int, original, translation string) Corrected {
	return Corrected{Line: Line{Start: ms(start), End: ms(end), Original: original, Translation: translation}, Machine: Span{ms(start), ms(end)}, Edit: -1}
}

// exportLines are three lines, the second untranslated and the third marked as no speech.
func exportLines() []Corrected {
	noise := corrected(5000, 6000, "[Music]", "【音乐】")
	noise.NoSpeech, noise.Edit = true, 1
	edited := corrected(1000, 2500, "Hello there.", "你好。")
	edited.Edit = 0
	return []Corrected{edited, corrected(3000, 4000, "How are you?", ""), noise}
}

func TestExportCuesByContent(t *testing.T) {
	cases := []struct {
		name string
		e    Export
		want [][]string
	}{
		{"original", Export{Content: SubOriginal}, [][]string{{"Hello there."}, {"How are you?"}}},
		{"translation skips", Export{Content: SubTranslation, Missing: MissingSkip}, [][]string{{"你好。"}}},
		{"translation placeholder", Export{Content: SubTranslation, Missing: MissingPlaceholder, Placeholder: "[?]"}, [][]string{{"你好。"}, {"[?]"}}},
		{"translation fills with original", Export{Content: SubTranslation, Missing: MissingOriginal}, [][]string{{"你好。"}, {"How are you?"}}},
		{"bilingual translation first skips", Export{Content: SubBilingual, Order: SubTranslationFirst, Missing: MissingSkip}, [][]string{{"你好。", "Hello there."}, {"How are you?"}}},
		{"bilingual original first", Export{Content: SubBilingual, Order: SubOriginalFirst, Missing: MissingSkip}, [][]string{{"Hello there.", "你好。"}, {"How are you?"}}},
		{"bilingual placeholder", Export{Content: SubBilingual, Order: SubTranslationFirst, Missing: MissingPlaceholder, Placeholder: "[?]"}, [][]string{{"你好。", "Hello there."}, {"[?]", "How are you?"}}},
		{"bilingual original first placeholder", Export{Content: SubBilingual, Order: SubOriginalFirst, Missing: MissingPlaceholder, Placeholder: "[?]"}, [][]string{{"Hello there.", "你好。"}, {"How are you?", "[?]"}}},
		{"bilingual fill does not repeat the original", Export{Content: SubBilingual, Missing: MissingOriginal}, [][]string{{"你好。", "Hello there."}, {"How are you?"}}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			cues := tc.e.Cues(exportLines())
			var got [][]string
			for _, c := range cues {
				var texts []string
				for _, l := range c.Lines {
					texts = append(texts, l.Text)
				}
				got = append(got, texts)
			}
			if strings.Join(flat(got), "|") != strings.Join(flat(tc.want), "|") || len(got) != len(tc.want) {
				t.Fatalf("cues = %q, want %q", got, tc.want)
			}
		})
	}
}

func flat(cues [][]string) []string {
	var out []string
	for _, c := range cues {
		out = append(out, strings.Join(c, "/"))
	}
	return out
}

func TestExportCuesMarkOriginals(t *testing.T) {
	cues := Export{Content: SubBilingual, Missing: MissingPlaceholder, Placeholder: "?"}.Cues(exportLines())
	if cues[0].Lines[0].Original || !cues[0].Lines[1].Original {
		t.Fatalf("first cue = %+v, want the translation then the original", cues[0].Lines)
	}
	if cues[1].Lines[0].Original || !cues[1].Lines[0].Dim {
		t.Fatalf("placeholder = %+v, want a dimmed translation line", cues[1].Lines[0])
	}
}

func TestExportDelay(t *testing.T) {
	lines := []Corrected{corrected(0, 400, "gone", ""), corrected(300, 1000, "cut", ""), corrected(2000, 3000, "kept", "")}
	cues := Export{Content: SubOriginal, Delay: -500 * time.Millisecond}.Cues(lines)
	if len(cues) != 2 {
		t.Fatalf("cues = %+v, want the line that ends before the video starts left out", cues)
	}
	if cues[0].Start != 0 || cues[0].End != ms(500) || cues[1].Start != ms(1500) || cues[1].End != ms(2500) {
		t.Fatalf("cues = %+v, want them moved 0.5 s earlier, none before 0", cues)
	}
	later := Export{Content: SubOriginal, Delay: 1200 * time.Millisecond}.Cues(lines)
	if later[0].Start != ms(1200) || later[2].End != ms(4200) {
		t.Fatalf("cues = %+v, want them moved 1.2 s later", later)
	}
}

func TestExportCount(t *testing.T) {
	for _, tc := range []struct {
		e       Export
		missing int
	}{
		{Export{Content: SubOriginal}, 0},
		{Export{Content: SubTranslation}, 1},
		{Export{Content: SubBilingual}, 1},
	} {
		got := tc.e.Count(exportLines())
		want := ExportCount{Lines: 2, Translated: 1, Missing: tc.missing, Edited: 1, NoSpeech: 1}
		if got != want {
			t.Fatalf("%s: count = %+v, want %+v", tc.e.Content, got, want)
		}
	}
}

func TestWriteSRT(t *testing.T) {
	lines := []Corrected{corrected(1000, 2500, "Hello\n\nthere.", "你好。"), corrected(3_723_004, 3_724_000, "Bye.", "再见。")}
	got := string(Export{Format: FormatSRT, Content: SubBilingual}.Write(lines))
	want := "1\n00:00:01,000 --> 00:00:02,500\n你好。\nHello\nthere.\n\n2\n01:02:03,004 --> 01:02:04,000\n再见。\nBye.\n"
	if got != want {
		t.Fatalf("srt =\n%q\nwant\n%q", got, want)
	}
}

func TestWriteVTT(t *testing.T) {
	lines := []Corrected{corrected(1000, 2500, "Tom & <Jerry> --> run", "")}
	got := string(Export{Format: FormatVTT, Content: SubOriginal}.Write(lines))
	want := "WEBVTT\n\n1\n00:00:01.000 --> 00:00:02.500\nTom &amp; &lt;Jerry&gt; --&gt; run\n"
	if got != want {
		t.Fatalf("vtt =\n%q\nwant\n%q", got, want)
	}
}

func TestWriteASS(t *testing.T) {
	lines := []Corrected{corrected(1000, 2505, `a {b} \N c`, "你好"), corrected(36_000_000, 36_001_000, "late", "")}
	got := string(Export{Format: FormatASS, Content: SubBilingual, Order: SubOriginalFirst, Missing: MissingSkip}.Write(lines))
	if !strings.HasPrefix(got, "\ufeff[Script Info]\n") || !strings.Contains(got, "ScriptType: v4.00+\n") {
		t.Fatalf("ass header =\n%s", got)
	}
	for _, style := range []string{"\nStyle: " + assStyleTranslation + ",", "\nStyle: " + assStyleOriginal + ","} {
		if !strings.Contains(got, style) {
			t.Fatalf("ass has no %q:\n%s", style, got)
		}
	}
	events := got[strings.Index(got, "[Events]"):]
	want := "[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n" +
		"Dialogue: 0,0:00:01.00,0:00:02.50," + assStyleOriginal + ",,0,0,0,,a ｛b｝ \\\u2060N c\\N{\\r" + assStyleTranslation + "}你好\n" +
		"Dialogue: 0,10:00:00.00,10:00:01.00," + assStyleOriginal + ",,0,0,0,,late\n"
	if events != want {
		t.Fatalf("ass events =\n%q\nwant\n%q", events, want)
	}
}

func TestExportExt(t *testing.T) {
	for format, ext := range map[string]string{FormatSRT: ".srt", FormatVTT: ".vtt", FormatASS: ".ass", "": ".srt", "x": ".srt"} {
		if got := (Export{Format: format}).Ext(); got != ext {
			t.Fatalf("Ext(%q) = %q, want %q", format, got, ext)
		}
	}
}
