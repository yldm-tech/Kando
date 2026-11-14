package cheatsheet

import (
	"testing"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) {
	testlocale.Pin(locales.Default)
	nsgotest.Main(m)
}

// sheet is the state the app gives the sheet, kept as plain fields as the app keeps them, with how often it was closed.
type sheet struct {
	live.Rev
	open   bool
	groups []Group
	closed int
}

func newSheet() *sheet {
	return &sheet{open: true, groups: []Group{
		{Title: "播放", Keys: []Key{{Label: "播放 / 暂停", Combos: []string{"space"}}, {Label: "后退 / 前进 5 秒", Combos: []string{"left", "right"}}}},
		{Title: "窗口", Keys: []Key{{Label: "问这部影片 / 命令", Combos: []string{"mod+k"}}, {Label: "返回片库", Combos: []string{"escape"}}}},
	}}
}

// start shows s's sheet over a window of width×height on os, in testlocale's language.
func start(t *testing.T, s *sheet, os core.OS, width, height core.Points) *nsgotest.App {
	t.Helper()
	env := testlocale.Environment(core.Environment{OS: os, Idiom: core.IdiomDesktop, AppleKeyboard: os == core.OSMacOS})
	return nsgotest.Start(t, func(cx *core.Scope) core.Node {
		s.Attach(cx)
		return Sheet{
			Open:    live.Func(s, func() bool { return s.open }),
			Groups:  live.Func(s, func() []Group { return s.groups }),
			OnClose: live.Do(s, func() { s.open = false; s.closed++ }),
		}
	}, nsgotest.Size(width, height), nsgotest.Environment(env))
}

func TestSheetListsTheGroupsAndTheirKeys(t *testing.T) {
	s := newSheet()
	app := start(t, s, core.OSMacOS, 1304, 782)
	msgs := testlocale.Messages()
	for _, want := range []string{msgs.CheatsheetTitle(), msgs.CheatsheetHint(), "播放", "播放 / 暂停", "Space", "← →", "窗口", "⌘K", "Esc"} {
		if !app.Exists(`text:"` + want + `"`) {
			t.Fatalf("missing %q in %q", want, app.Texts())
		}
	}
	// Escape alone reads Esc on its keycap and Escape to assistive technology.
	if !app.Exists("Escape") {
		t.Fatalf("no keycap named Escape\n%s", app.Dump())
	}
}

func TestSheetSplitsKeycapsOffTheMac(t *testing.T) {
	s := newSheet()
	app := start(t, s, core.OSLinux, 1304, 782)
	for _, want := range []string{"←", "→", "Ctrl+K"} {
		if !app.Exists(`text:"` + want + `"`) {
			t.Fatalf("missing keycap %q in %q", want, app.Texts())
		}
	}
	if app.Exists(`text:"← →"`) {
		t.Fatal("the keys of a line share a keycap off the Mac")
	}
}

func TestSheetCloses(t *testing.T) {
	s := newSheet()
	app := start(t, s, core.OSMacOS, 1304, 782)
	app.Press("id:cheatsheet-close")
	if s.open || s.closed != 1 || app.Exists("id:cheatsheet") {
		t.Fatalf("the close button left the sheet open %v (closed %d)", s.open, s.closed)
	}
	s.open = true
	s.Changed()
	app.Settle()
	app.Key("escape")
	if s.open || s.closed != 2 {
		t.Fatalf("Escape left the sheet open %v (closed %d)", s.open, s.closed)
	}
}

// The columns follow the window: as many 240 wide as fit in the sheet, at least one.
func TestSheetColumnsFollowTheWindow(t *testing.T) {
	for _, c := range []struct {
		width core.Points
		want  int
	}{{1304, 3}, {700, 2}, {300, 1}} {
		if got := columns(sheetFits(c.width)); got != c.want {
			t.Errorf("a window %v wide holds %d columns, want %d", c.width, got, c.want)
		}
	}
}

// The app hands the sheet groups that follow its whole revision (live.Func), which changes on every keystroke and progress report; the groups must not be laid out again for those while they stay the same.
func TestSheetStaysWhileTheAppChanges(t *testing.T) {
	s := newSheet()
	app := start(t, s, core.OSMacOS, 1304, 782)
	before := app.Find(`text:"播放 / 暂停"`).ID
	s.Changed()
	app.Settle()
	if after := app.Find(`text:"播放 / 暂停"`).ID; after != before {
		t.Fatalf("the groups were built again (view %d, was %d) though they stayed the same", after, before)
	}
	s.groups = []Group{{Title: "播放", Keys: []Key{{Label: "暂停", Combos: []string{"space"}}}}}
	s.Changed()
	app.Settle()
	if !app.Exists(`text:"暂停"`) {
		t.Fatalf("the sheet does not follow its groups: %q", app.Texts())
	}
}
