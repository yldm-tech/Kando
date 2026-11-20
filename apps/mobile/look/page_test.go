package look

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/ui"
)

// On Android a titled page keeps its header's buttons beside the title it draws in the toolbar, and the title is bounded to the room the buttons leave, so a long one ends in an ellipsis.
func TestATitledPageOnAndroidKeepsItsHeaderButtons(t *testing.T) {
	if nsgotest.Target() != "" {
		t.Skip("the platform is the target's")
	}
	const window = 390
	app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
		nav := base.NewNav(cx, 0)
		return nav.Stack(core.Style{FlexGrow: 1}, func(cx *core.Scope, _ int, _ func() int) base.NavPage {
			add := ui.Button{TestID: "add", Label: "Add", OnPress: func() {}}
			back := ui.Button{TestID: "lead", Label: "Lead", OnPress: func() {}}
			return NavPage(cx, base.NavPage{TestID: "page"}, ui.NavHeader{TitleFunc: func() string { return "A computer with a very long name indeed" },
				Leading: []core.Node{back}, Trailing: []core.Node{add}})
		})
	}, nsgotest.Size(window, 844), nsgotest.Environment(core.Environment{OS: core.OSAndroid, Idiom: core.IdiomPhone}))
	app.WaitFor("id:page", 5*time.Second)
	for _, id := range []string{"id:add", "id:lead"} {
		if n := len(app.QueryMounted(id)); n != 1 {
			t.Fatalf("%d of %s are mounted in the header, want 1", n, id)
		}
	}
	title := app.QueryMounted("text:A computer with a very long name indeed")
	if len(title) != 1 {
		t.Fatalf("%d titles are mounted, want 1", len(title))
	}
	if got, most := title[0].Frame.Size.Width, SubPageTitleWidth(window, 2); got <= 0 || got > most {
		t.Fatalf("the long title is %v wide, want at most %v", got, most)
	}
}

// The title's bound leaves the toolbar's insets and each button's target, and never falls under a target's width.
func TestSubPageTitleWidth(t *testing.T) {
	for _, c := range []struct {
		window  core.Points
		buttons int
		want    core.Points
	}{{390, 0, 302}, {390, 1, 242}, {390, 2, 194}, {100, 2, 48}} {
		if got := SubPageTitleWidth(c.window, c.buttons); got != c.want {
			t.Errorf("SubPageTitleWidth(%v, %d) = %v, want %v", c.window, c.buttons, got, c.want)
		}
	}
}
