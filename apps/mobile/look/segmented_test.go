package look

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// A segmented control is ui.Tabs: a tab list Label names, a tab for each segment named by its text, the chosen one selected; a press on another chooses it. The names are bound, so they follow the interface language with the control left as it is.
func TestASegmentedControlIsTabsNamedByItsTexts(t *testing.T) {
	var group, best *core.Signal[string]
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		group, best = core.NewSignal(cx, "Quality"), core.NewSignal(cx, "Best")
		value := core.NewSignal(cx, "best")
		return core.View{TestID: "segmented", Children: []core.Node{Segmented{Label: group.Get, Value: value.Get, OnChange: value.Set, Segments: []Segment{
			{Value: "best", TestID: "seg-best", Text: best.Get},
			{Value: "720", TestID: "seg-720", Text: func() string { return "720p" }},
		}}}}
	}, nsgotest.Environment(core.Environment{OS: core.OSIOS, Idiom: core.IdiomPhone}))
	list := func() *nsgotest.View { return app.Find("id:segmented").Children[0] }
	if got := list().Props.Role; got != core.RoleTabList {
		t.Fatalf("the control's role is %v, want a tab list", got)
	}
	if got := list().Props.Label; got != "Quality" {
		t.Fatalf("the control is read as %q, want Quality", got)
	}
	if v := app.Find("id:seg-best"); v.Props.Role != core.RoleTab || v.Props.Label != "Best" || !v.Props.Selected {
		t.Fatalf("the chosen segment is %v %q, selected %v; want a selected tab read as Best", v.Props.Role, v.Props.Label, v.Props.Selected)
	}
	if got := app.Find("id:seg-best").Frame.Size.Height; got != 32 {
		t.Fatalf("a segment is %v points tall, want 32 on the 36-point track", got)
	}
	app.Press("id:seg-720")
	app.WaitUntil(time.Second, func() bool { return app.Find("id:seg-720").Props.Selected && !app.Find("id:seg-best").Props.Selected })

	group.Set("画质")
	best.Set("最佳")
	app.WaitUntil(time.Second, func() bool { return list().Props.Label == "画质" && app.Find("id:seg-best").Props.Label == "最佳" })
	if !app.Exists("id:seg-best >> text:最佳") {
		t.Fatal("the segment does not show 最佳")
	}
}

// A segment's text longer than the segment stays inside it, ending in an ellipsis, rather than overflowing on both sides past the control and the screen.
func TestALongSegmentTextStaysInItsSegment(t *testing.T) {
	const long = "As on a computer with a very long name indeed"
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		value := core.NewSignal(cx, "")
		return core.View{Style: core.Style{Width: core.Pt(300)}, Children: []core.Node{Segmented{Label: func() string { return "Save" }, Value: value.Get, OnChange: value.Set, Segments: []Segment{
			{Value: "", TestID: "seg-long", Text: func() string { return long }},
			{Value: "best", TestID: "seg-best", Text: func() string { return "Best" }},
			{Value: "720", TestID: "seg-720", Text: func() string { return "720P" }},
		}}}}
	}, nsgotest.Environment(core.Environment{OS: core.OSIOS, Idiom: core.IdiomPhone}))
	seg := app.Find("id:seg-long").Bounds()
	text := app.Find("id:seg-long >> text:" + long).Bounds()
	if text.Origin.X < seg.Origin.X-0.5 || text.Origin.X+text.Size.Width > seg.Origin.X+seg.Size.Width+0.5 {
		t.Fatalf("the text is at x %v, %v wide, outside its segment at x %v, %v wide", text.Origin.X, text.Size.Width, seg.Origin.X, seg.Size.Width)
	}
}
