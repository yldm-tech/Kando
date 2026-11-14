package live

import (
	"strconv"
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

// counter is state kept as Kando keeps its own: a plain field changed in place.
type counter struct {
	Rev
	n     int
	query string
}

func (c *counter) view(cx *core.Scope) core.Node {
	c.Attach(cx)
	return core.View{Children: []core.Node{
		core.Text{TestID: "count", ContentFunc: Func(c, func() string { return strconv.Itoa(c.n) })},
		base.Button{TestID: "add", Label: "add", OnPress: On(c, func(core.PressEvent) { c.n++ }), Children: []core.Node{core.Text{Content: "add"}}},
		base.Input{TestID: "query", Value: Func(c, func() string { return c.query }), OnChange: On(c, func(s string) { c.query = s })},
	}}
}

func TestHandlersAndReportsUpdateTheBindings(t *testing.T) {
	c := &counter{}
	// A report posted before the graph exists runs once it does.
	c.Post(func() { c.n = 10 })
	app := nsgotest.Start(t, c.view)
	if got := app.Find("id:count").Text; got != "10" {
		t.Fatalf("count %q after the held report, want 10", got)
	}
	app.Press("id:add")
	if got := app.Find("id:count").Text; got != "11" {
		t.Fatalf("count %q after a press, want 11", got)
	}
	go c.Post(func() { c.n = 20 })
	app.WaitUntil(5*time.Second, func() bool { return app.Find("id:count").Text == "20" })
}

func TestChangedKeepsTheViews(t *testing.T) {
	c := &counter{}
	app := nsgotest.Start(t, c.view)
	field := app.Find("id:query")
	app.Type("id:query", "talk")
	app.Press("id:add")
	after := app.Find("id:query")
	if after.ID != field.ID || after.Text != "talk" {
		t.Fatalf("the field is view %d holding %q after Changed, want view %d still holding %q", after.ID, after.Text, field.ID, "talk")
	}
	if c.query != "talk" {
		t.Fatalf("state holds %q, want what was typed", c.query)
	}
}

func TestMemoFollowsTheValueOnly(t *testing.T) {
	c := &counter{}
	runs := 0
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		c.Attach(cx)
		even := Memo(cx, c, func() bool { return c.n%2 == 0 })
		return core.Text{TestID: "even", ContentFunc: func() string {
			runs++
			return strconv.FormatBool(even())
		}}
	})
	before := runs
	c.n = 2
	c.Changed()
	app.Settle()
	if runs != before {
		t.Fatalf("the binding ran %d more times though the memo's value stayed", runs-before)
	}
	c.n = 3
	c.Changed()
	app.Settle()
	if got := app.Find("id:even").Text; got != "false" {
		t.Fatalf("even %q, want false", got)
	}
}

func TestReportsKeepTheirOrderAcrossAttach(t *testing.T) {
	const n = 2000
	c := &counter{}
	var got []int
	started := make(chan struct{})
	go func() {
		close(started)
		for i := range n {
			c.Post(func() { got = append(got, i) })
		}
	}()
	<-started
	app := nsgotest.Start(t, c.view)
	app.WaitUntil(10*time.Second, func() bool { return len(got) == n })
	for i, v := range got {
		if v != i {
			t.Fatalf("report %d ran in place %d: the reports held before Attach and the ones posted while it ran came out of order", v, i)
		}
	}
}
