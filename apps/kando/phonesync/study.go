package phonesync

import (
	"strings"
	"time"

	"kando/apps/kando/lines"
	psync "kando/packages/sync"
)

// Line is a line a phone sends, as Explain Line and the recap read lines.
func Line(c psync.Cue) lines.Line {
	return lines.New(time.Duration(c.StartMS)*time.Millisecond, strings.TrimSpace(c.Translation), strings.TrimSpace(c.Original))
}

// ExplainLines are the line req explains and the lines before it that go with it, at most before of them; the lines after it are left out, so what the AI service is given gives nothing away of what comes after. req has been checked (psync.LineExplain.Check).
func ExplainLines(req psync.LineExplain, before int) ([]lines.Line, lines.Line) {
	var out []lines.Line
	for _, c := range req.Lines[max(req.Index-before, 0):req.Index] {
		out = append(out, Line(c))
	}
	return out, Line(req.Lines[req.Index])
}
