package film

import (
	"kando/apps/kando/live"
	"kando/apps/kando/theme"

	"github.com/nsgo-dev/nsgo/core"
)

// osdGlass is the glass of the pill that says what the last action did.
var osdGlass = glassSpec{theme.GlassBar, 12}

// osd is the small glass pill at the top left of the picture that says what the last action did, as "−5 秒" or "字幕：双语", for a moment (the app takes the message away).
func (s *screen) osd() core.Node {
	a := s.a
	text := live.Func(a, a.OSD)
	return core.Show{When: func() bool { return text() != "" }, Then: func(*core.Scope) core.Node {
		return core.View{TestID: "film-osd", PointerEvents: core.PointerEventsNone, Role: core.RoleStatus,
			Style:    osdGlass.style().Merge(core.Style{Position: core.Absolute, Left: core.Pt(20), Top: core.Pt(66), Direction: core.Row, Padding: core.Symmetric(6, 12)}),
			Children: []core.Node{core.Text{ContentFunc: text, Style: core.Style{FontSize: 13, LineHeight: 18, FontWeight: core.FontWeightSemibold, Color: onGlass}}}}
	}}
}
