package core

import (
	"fmt"
	"strings"
	"time"
)

// Exporting a film's subtitles to a file of the user's: its lines with the user's corrections laid over them (Correct), those marked as no speech left out, as SubRip, WebVTT or Advanced SubStation Alpha, holding the originals, the translations or both, in UTF-8.

// The formats a film's subtitles export to, by the extension of their files.
const (
	FormatSRT = "srt"
	FormatVTT = "vtt"
	FormatASS = "ass"
)

// What an export does with a line that has no translation, where the translations are exported: leaves it out (only its original shows in a bilingual export), puts Export.Placeholder in the translation's place, or shows its original in a translation export (a bilingual one shows the original once).
const (
	MissingSkip        = "skip"
	MissingPlaceholder = "placeholder"
	MissingOriginal    = "original"
)

// Export is how a film's subtitles export: Format, one of the Format values, SRT for any other; Content, which lines of each cue, as the player's modes (SubOriginal, SubTranslation, SubBilingual, bilingual for any other); Order, which of a bilingual cue's lines goes on top (SubTranslationFirst or SubOriginalFirst); Missing, what a line without a translation becomes (one of the Missing values, MissingSkip for any other), and Placeholder the text MissingPlaceholder puts in its place; Delay is added to every time, as the player's subtitle delay shows the lines later by it.
type Export struct {
	Format      string
	Content     string
	Order       string
	Missing     string
	Placeholder string
	Delay       time.Duration
}

// ExportCue is a cue as it is written: when it shows and its lines, from the top. A line with Dim set is the placeholder of a missing translation.
type ExportCue struct {
	Start, End time.Duration
	Lines      []CueLine
}

// Ext is the extension of the file e writes, with its dot.
func (e Export) Ext() string { return "." + e.format() }

func (e Export) format() string {
	switch e.Format {
	case FormatVTT, FormatASS:
		return e.Format
	}
	return FormatSRT
}

// Cues are the cues e writes of lines, a film's lines with the user's corrections laid over them in the order they start (Correct): the lines marked as no speech are left out, every time is moved by e.Delay, a line that then ends before the video starts is left out and one that starts before it starts at 0, and a cue left without lines is left out.
func (e Export) Cues(lines []Corrected) []ExportCue {
	var out []ExportCue
	for _, l := range lines {
		if l.NoSpeech {
			continue
		}
		start, end := max(l.Start+e.Delay, 0), l.End+e.Delay
		if end <= start {
			continue
		}
		if text := e.lines(strings.TrimSpace(l.Translation), strings.TrimSpace(l.Original)); len(text) > 0 {
			out = append(out, ExportCue{Start: start, End: end, Lines: text})
		}
	}
	return out
}

// lines are a cue's lines as e picks them from its translation and its original.
func (e Export) lines(translation, original string) []CueLine {
	var out []CueLine
	add := func(text string, isOriginal, dim bool) {
		if text != "" {
			out = append(out, CueLine{Text: text, Original: isOriginal, Dim: dim})
		}
	}
	// translated adds the translation, or what e puts in place of a missing one; fill tells whether the original may stand in for it.
	translated := func(fill bool) {
		switch {
		case translation != "":
			add(translation, false, false)
		case e.Missing == MissingPlaceholder:
			add(e.Placeholder, false, true)
		case e.Missing == MissingOriginal && fill:
			add(original, true, false)
		}
	}
	switch e.Content {
	case SubOriginal:
		add(original, true, false)
	case SubTranslation:
		translated(true)
	default:
		if e.Order == SubOriginalFirst {
			add(original, true, false)
			translated(false)
		} else {
			translated(false)
			add(original, true, false)
		}
	}
	return out
}

// ExportCount is what an export of a film's lines holds, for the dialog to say before it is written: Lines, the lines it holds, those marked as no speech left out; Translated, how many of them have a translation, and Missing, how many do not, where the translations are exported (0 for an export of the originals); Edited, how many of them the user corrected; NoSpeech, how many lines it leaves out as no speech.
type ExportCount struct {
	Lines, Translated, Missing, Edited, NoSpeech int
}

// Count is what e exports of lines (Cues).
func (e Export) Count(lines []Corrected) ExportCount {
	var c ExportCount
	for _, l := range lines {
		if l.NoSpeech {
			c.NoSpeech++
			continue
		}
		c.Lines++
		if strings.TrimSpace(l.Translation) != "" {
			c.Translated++
		} else if e.Content != SubOriginal {
			c.Missing++
		}
		if l.Edit >= 0 {
			c.Edited++
		}
	}
	return c
}

// Write is the file e writes of lines (Cues), in UTF-8.
func (e Export) Write(lines []Corrected) []byte {
	cues := e.Cues(lines)
	var b strings.Builder
	switch e.format() {
	case FormatVTT:
		writeVTT(&b, cues)
	case FormatASS:
		writeASS(&b, cues)
	default:
		writeSRT(&b, cues)
	}
	return []byte(b.String())
}

// textLines are the lines of a cue's text as a file holds them, one per line of its lines, an empty one left out, as an empty line ends a cue in SubRip and WebVTT.
func textLines(lines []CueLine, each func(l CueLine, text string) string) []string {
	var out []string
	for _, l := range lines {
		for text := range strings.SplitSeq(l.Text, "\n") {
			if text = strings.TrimSpace(text); text != "" {
				out = append(out, each(l, text))
			}
		}
	}
	return out
}

// clock is d as hours, minutes, seconds and milliseconds.
func clock(d time.Duration) (h, m, s, ms int64) {
	t := d.Milliseconds()
	return t / 3_600_000, t / 60_000 % 60, t / 1000 % 60, t % 1000
}

func writeSRT(b *strings.Builder, cues []ExportCue) {
	stamp := func(d time.Duration) string {
		h, m, s, ms := clock(d)
		return fmt.Sprintf("%02d:%02d:%02d,%03d", h, m, s, ms)
	}
	for i, c := range cues {
		if i > 0 {
			b.WriteString("\n")
		}
		fmt.Fprintf(b, "%d\n%s --> %s\n", i+1, stamp(c.Start), stamp(c.End))
		for _, text := range textLines(c.Lines, func(_ CueLine, text string) string { return text }) {
			b.WriteString(text + "\n")
		}
	}
}

// vttEscape escapes what WebVTT reads as markup in a cue's text, which also keeps "-->" out of it.
var vttEscape = strings.NewReplacer("&", "&amp;", "<", "&lt;", ">", "&gt;")

func writeVTT(b *strings.Builder, cues []ExportCue) {
	stamp := func(d time.Duration) string {
		h, m, s, ms := clock(d)
		return fmt.Sprintf("%02d:%02d:%02d.%03d", h, m, s, ms)
	}
	b.WriteString("WEBVTT\n")
	for i, c := range cues {
		fmt.Fprintf(b, "\n%d\n%s --> %s\n", i+1, stamp(c.Start), stamp(c.End))
		for _, text := range textLines(c.Lines, func(_ CueLine, text string) string { return vttEscape.Replace(text) }) {
			b.WriteString(text + "\n")
		}
	}
}

// The styles of an ASS export: the translation's, and the original's, smaller and dimmer under it in a bilingual cue.
const (
	assStyleTranslation = "Translation"
	assStyleOriginal    = "Original"
)

// assHeader is the start of an ASS export: a 1080p script, and its two styles, white with a dark outline at the bottom centre.
const assHeader = "\ufeff[Script Info]\n" +
	"ScriptType: v4.00+\n" +
	"WrapStyle: 0\n" +
	"ScaledBorderAndShadow: yes\n" +
	"PlayResX: 1920\n" +
	"PlayResY: 1080\n" +
	"YCbCr Matrix: None\n" +
	"\n[V4+ Styles]\n" +
	"Format: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\n" +
	"Style: " + assStyleTranslation + ",Arial,64,&H00FFFFFF,&H000000FF,&H00000000,&H80000000,0,0,0,0,100,100,0,0,1,3,1,2,60,60,50,1\n" +
	"Style: " + assStyleOriginal + ",Arial,48,&H00D8D8D8,&H000000FF,&H00000000,&H80000000,0,0,0,0,100,100,0,0,1,2.5,1,2,60,60,50,1\n" +
	"\n[Events]\n" +
	"Format: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\n"

// assEscape keeps a line's text from being read as ASS markup: braces, which open and close override tags, become full-width ones, and a backslash is kept from starting \N, \n or \h by a word joiner after it.
var assEscape = strings.NewReplacer("{", "｛", "}", "｝", `\`, "\\\u2060")

func writeASS(b *strings.Builder, cues []ExportCue) {
	stamp := func(d time.Duration) string {
		h, m, s, ms := clock(d)
		return fmt.Sprintf("%d:%02d:%02d.%02d", h, m, s, ms/10)
	}
	style := func(l CueLine) string {
		if l.Original {
			return assStyleOriginal
		}
		return assStyleTranslation
	}
	b.WriteString(assHeader)
	for _, c := range cues {
		first := style(c.Lines[0])
		last := first
		texts := textLines(c.Lines, func(l CueLine, text string) string {
			text = assEscape.Replace(text)
			if s := style(l); s != last {
				last = s
				return `{\r` + s + "}" + text
			}
			return text
		})
		fmt.Fprintf(b, "Dialogue: 0,%s,%s,%s,,0,0,0,,%s\n", stamp(c.Start), stamp(c.End), first, strings.Join(texts, `\N`))
	}
}
