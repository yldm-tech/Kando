package annotate

import (
	"slices"
	"strings"
	"unicode/utf16"
	"unicode/utf8"

	"kando/apps/kando/plugins"
)

// The most marks an annotator of a script plugin may give a line, and the most characters of a reading it gives.
const (
	pluginMarksMax = 64
	pluginRubyMax  = 32
)

// FromPlugin is the marks an annotator of a script plugin gave line as v, what its annotate returned for the line read as JSON: a list of objects of start and end, offsets in the line as JavaScript counts them (UTF-16 code units, as String.prototype.slice takes them), with ruby, a reading drawn over the run, or tone (accent, warn, success or muted) and underline, a line drawn under it. A mark that is not one, out of the line, that splits a character, empty, or with a reading of a line break or of more than pluginRubyMax characters is left out, as are those after the first pluginMarksMax; the offsets come back as byte offsets in line, as Mark has them.
func FromPlugin(line string, v any) []Mark {
	list, _ := v.([]any)
	// at maps each UTF-16 offset of line to its byte offset, -1 inside a character.
	at := []int{}
	for i, r := range line {
		at = append(at, i)
		if utf16.RuneLen(r) == 2 {
			at = append(at, -1)
		}
	}
	at = append(at, len(line))
	offset := func(x any) (int, bool) {
		n, ok := plugins.Number(x)
		if !ok || n != float64(int(n)) || n < 0 || int(n) >= len(at) || at[int(n)] < 0 {
			return 0, false
		}
		return at[int(n)], true
	}
	var out []Mark
	for _, item := range list {
		if len(out) == pluginMarksMax {
			break
		}
		m, _ := item.(map[string]any)
		start, ok1 := offset(m["start"])
		end, ok2 := offset(m["end"])
		if !ok1 || !ok2 || start >= end {
			continue
		}
		mark := Mark{Start: start, End: end}
		ruby, _ := m["ruby"].(string)
		tone, _ := m["tone"].(string)
		underline, _ := m["underline"].(bool)
		switch {
		case ruby != "":
			ruby = strings.TrimSpace(ruby)
			if ruby == "" || utf8.RuneCountInString(ruby) > pluginRubyMax || strings.ContainsAny(ruby, "\r\n") || !utf8.ValidString(ruby) {
				continue
			}
			mark.Ruby = ruby
		case tone != "" || underline:
			if tone != "" && !slices.Contains([]string{plugins.ToneAccent, plugins.ToneWarn, plugins.ToneSuccess, plugins.ToneMuted}, tone) {
				continue
			}
			mark.Tone, mark.Underline = tone, underline
		default:
			continue
		}
		out = append(out, mark)
	}
	return out
}

// Merge is have with each of extra added that overlaps none of the same kind already there, a reading none of the readings and a line under the words none of the lines, so two sources never draw two readings over one character nor two lines under it; have's come first, and the result is in the order the marks start.
func Merge(have, extra []Mark) []Mark {
	out := slices.Clone(have)
	for _, m := range extra {
		ruby := m.Ruby != ""
		if slices.ContainsFunc(out, func(o Mark) bool { return (o.Ruby != "") == ruby && o.Start < m.End && m.Start < o.End }) {
			continue
		}
		out = append(out, m)
	}
	slices.SortStableFunc(out, func(a, b Mark) int { return a.Start - b.Start })
	return out
}
