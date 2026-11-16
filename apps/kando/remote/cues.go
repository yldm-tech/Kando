package remote

import (
	"fmt"
	"strconv"
	"strings"
)

// The languages of a film's subtitles a client can pick (PickCues).
const (
	// WhichBoth is the translation above the original, as the bilingual .srt has them, or the original alone for a line left untranslated.
	WhichBoth = "both"
	// WhichOriginal is the original speech alone.
	WhichOriginal = "original"
	// WhichTranslation is the translation alone, leaving out the lines the translation left out.
	WhichTranslation = "translation"
)

// PickCues is the cues for which, as `kando subtitles` and kando_get_subtitles pick them: every cue for both; for the original, every cue without its translation; for the translation, only the translated cues, their text left empty and their times kept. It is never nil, so no cues encode as [].
func PickCues(cues []Cue, which string) []Cue {
	picked := make([]Cue, 0, len(cues))
	for _, c := range cues {
		switch which {
		case WhichOriginal:
			c.Translation = ""
		case WhichTranslation:
			if c.Translation == "" {
				continue
			}
			c.Text = ""
		}
		picked = append(picked, c)
	}
	return picked
}

// cueText is the lines a cue has, the translation above the original, joined by sep, "" when it has none.
func cueText(c Cue, sep string) string {
	switch {
	case c.Translation == "":
		return strings.TrimSpace(c.Text)
	case strings.TrimSpace(c.Text) == "":
		return strings.TrimSpace(c.Translation)
	}
	return strings.TrimSpace(c.Translation) + sep + strings.TrimSpace(c.Text)
}

// CueLine is the cue on one line, "translation | original", with only the part it has when it has one, its lines joined by spaces.
func CueLine(c Cue) string { return strings.Join(strings.Fields(cueText(c, " | ")), " ") }

// CuesText is one line per cue, "[hh:mm:ss] translation | original" (Clock, CueLine); a cue with no text is left out. It is the text format of `kando subtitles` and kando_get_subtitles, and each line ends with a line break.
func CuesText(cues []Cue) string {
	var b strings.Builder
	for _, c := range cues {
		text := CueLine(c)
		if text == "" {
			continue
		}
		b.WriteString("[" + Clock(c.StartMS) + "] " + text + "\n")
	}
	return b.String()
}

// CuesSRT is the cues as a SubRip file, the translation above the original as Kando's bilingual .srt has them, numbering the cues that have text from 1; a cue with no text is left out.
func CuesSRT(cues []Cue) string {
	var b strings.Builder
	n := 0
	for _, c := range cues {
		text := cueText(c, "\n")
		if text == "" {
			continue
		}
		n++
		fmt.Fprintf(&b, "%d\n%s --> %s\n%s\n\n", n, SRTTime(c.StartMS), SRTTime(c.EndMS), text)
	}
	return b.String()
}

// Clock is ms as hh:mm:ss.
func Clock(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	s := ms / 1000
	return fmt.Sprintf("%02d:%02d:%02d", s/3600, s/60%60, s%60)
}

// SRTTime is ms as SubRip's hh:mm:ss,mmm.
func SRTTime(ms int64) string {
	if ms < 0 {
		ms = 0
	}
	return fmt.Sprintf("%s,%03d", Clock(ms), ms%1000)
}

// ParseClock reads a moment of a video as a person or Clock writes it, in milliseconds: seconds ("75", "75.5"), minutes and seconds ("1:15") or hours, minutes and seconds ("1:02:03", "01:02:03.250"), with a comma as SRTTime writes it taken for the decimal point.
func ParseClock(s string) (int64, error) {
	given := s
	s = strings.ReplaceAll(strings.TrimSpace(s), ",", ".")
	parts := strings.Split(s, ":")
	bad := fmt.Errorf("%q is not a time: write it as seconds, mm:ss or hh:mm:ss", given)
	if len(parts) > 3 {
		return 0, bad
	}
	var seconds, ms int64
	for i, part := range parts {
		whole, fraction, dotted := strings.Cut(part, ".")
		// Each part is digits; only the seconds may have a fraction, and only the first part may pass 59, as in 75 seconds or 90:00.
		if !digits(whole) || dotted && (i < len(parts)-1 || !digits(fraction)) {
			return 0, bad
		}
		n, err := strconv.ParseInt(whole, 10, 64)
		if err != nil || i > 0 && n >= 60 {
			return 0, bad
		}
		seconds = seconds*60 + n
		if dotted {
			f, _ := strconv.ParseFloat("0."+fraction, 64)
			ms = int64(f*1000 + 0.5)
		}
	}
	return seconds*1000 + ms, nil
}

// digits reports whether s is one or more ASCII digits.
func digits(s string) bool {
	if s == "" {
		return false
	}
	for _, r := range s {
		if r < '0' || r > '9' {
			return false
		}
	}
	return true
}
