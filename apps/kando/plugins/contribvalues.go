package plugins

import (
	kcore "kando/packages/core"
)

// SubtitlesMax is the most bytes of a file a subtitle format of a script plugin may return.
const SubtitlesMax = 8 << 20

// CueValues are cues as a subtitle format of a script plugin is given them (exportSubtitles(formatID, cues)): each with startMs, endMs and its lines from the top, each line its text, whether it is the original, and dim for the placeholder of a missing translation. They are the cues Kando's own formats write (kcore.Export.Cues), the user's choices of lines, order and delay already applied.
func CueValues(cues []kcore.ExportCue) []any {
	out := make([]any, len(cues))
	for i, c := range cues {
		lines := make([]any, len(c.Lines))
		for j, l := range c.Lines {
			lines[j] = map[string]any{"text": l.Text, "original": l.Original, "dim": l.Dim}
		}
		out[i] = map[string]any{"startMs": c.Start.Milliseconds(), "endMs": c.End.Milliseconds(), "lines": lines}
	}
	return out
}

// SubtitlesText is the file a subtitle format of a script plugin returned as v, and whether it is one: a string of at most SubtitlesMax bytes.
func SubtitlesText(v any) (string, bool) {
	s, ok := v.(string)
	return s, ok && len(s) <= SubtitlesMax
}

// Exported is what an exporter of a script plugin answered as v ({sent, skipped}), each a count of at least 0, and whether it answered that.
func Exported(v any) (sent, skipped int, ok bool) {
	m, isMap := v.(map[string]any)
	if !isMap {
		return 0, 0, false
	}
	count := func(key string) (int, bool) {
		n, ok := Number(m[key])
		if !ok {
			return 0, m[key] == nil
		}
		return int(min(max(n, 0), 1e9)), n == float64(int64(n))
	}
	sent, ok1 := count("sent")
	skipped, ok2 := count("skipped")
	return sent, skipped, ok1 && ok2
}

// Number is v as a number, as a plugin's answer holds one: tsx gives a whole number as an int64 and another as a float64.
func Number(v any) (float64, bool) {
	switch n := v.(type) {
	case float64:
		return n, true
	case int64:
		return float64(n), true
	case int:
		return float64(n), true
	case int32:
		return float64(n), true
	}
	return 0, false
}
