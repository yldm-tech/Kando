package desktop

import (
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"testing"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

func TestMain(m *testing.M) { nsgotest.Main(m) }

// start runs an app on a Mac with a button for each entry of calls, labelled with its key, that calls it with the app's scope.
func start(t *testing.T, calls map[string]func(cx *core.Scope)) *nsgotest.App {
	t.Helper()
	return nsgotest.Start(t, func(cx *core.Scope) core.Node {
		var buttons []core.Node
		for label, do := range calls {
			buttons = append(buttons, base.Button{
				OnPress:  func(core.PressEvent) { do(cx) },
				Style:    core.Style{Width: core.Pt(120), Height: core.Pt(30)},
				Children: []core.Node{core.Text{Content: label}},
			})
		}
		return core.View{Children: buttons}
	}, nsgotest.Size(400, 200), nsgotest.Environment(core.Environment{OS: core.OSMacOS, Idiom: core.IdiomDesktop, Locale: "en-US"}))
}

// TestOpenURL checks that a web page and a System Settings pane reach the system, and that done hears whether the system opened them.
func TestOpenURL(t *testing.T) {
	var answers []bool
	done := func(opened bool) { answers = append(answers, opened) }
	app := start(t, map[string]func(*core.Scope){
		"Page": func(cx *core.Scope) { OpenURL(cx, "https://example.com/help", done) },
		"Settings": func(cx *core.Scope) {
			OpenURL(cx, "x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles", done)
		},
		"Script": func(cx *core.Scope) { OpenURL(cx, "javascript:alert(1)", done) },
	})
	app.OpenURL = func(url string) bool { return url != "https://example.com/help" }
	app.Press("text:Page")
	app.Press("text:Settings")
	app.Press("text:Script")
	app.Settle()
	want := []string{"https://example.com/help", "x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles"}
	if fmt.Sprint(app.OpenedURLs) != fmt.Sprint(want) {
		t.Fatalf("the system was asked to open %v, want %v", app.OpenedURLs, want)
	}
	if fmt.Sprint(answers) != fmt.Sprint([]bool{false, true, false}) {
		t.Fatalf("done heard %v", answers)
	}
}

// TestRevealOpensTheFolder checks that Reveal opens the folder holding the item, and says so when no app opened it or the item's folder is gone.
func TestRevealOpensTheFolder(t *testing.T) {
	dir := t.TempDir()
	film := filepath.Join(dir, "Spring Clip.mp4")
	if err := os.WriteFile(film, []byte("film"), 0o644); err != nil {
		t.Fatal(err)
	}
	var errs []error
	done := func(err error) { errs = append(errs, err) }
	app := start(t, map[string]func(*core.Scope){
		"Film":    func(cx *core.Scope) { Reveal(cx, film, done) },
		"Missing": func(cx *core.Scope) { Reveal(cx, filepath.Join(dir, "gone", "clip.mp4"), done) },
	})
	app.Press("text:Film")
	app.Settle()
	if len(app.OpenedFiles) != 1 || app.OpenedFiles[0].Path != dir {
		t.Fatalf("opened %+v, want the folder %s", app.OpenedFiles, dir)
	}
	app.OpenFile = func(core.SharedFile) bool { return false }
	app.Press("text:Film")
	app.Press("text:Missing")
	app.Settle()
	if len(errs) != 3 || errs[0] != nil || !errors.Is(errs[1], errFolderNotOpened) || errs[2] == nil {
		t.Fatalf("done heard %v", errs)
	}
}

// TestTrashLeavesTheItem checks that Trash, which NSGo cannot do yet, fails as unsupported and leaves the item where it is.
func TestTrashLeavesTheItem(t *testing.T) {
	film := filepath.Join(t.TempDir(), "Spring Clip.mp4")
	if err := os.WriteFile(film, []byte("film"), 0o644); err != nil {
		t.Fatal(err)
	}
	var err error
	app := start(t, map[string]func(*core.Scope){"Trash": func(cx *core.Scope) { err = Trash(cx, film) }})
	app.Press("text:Trash")
	app.Settle()
	if !errors.Is(err, errors.ErrUnsupported) {
		t.Fatalf("Trash answered %v", err)
	}
	if _, statErr := os.Stat(film); statErr != nil {
		t.Fatalf("the item is gone: %v", statErr)
	}
}

// TestKeepAwakeReleases checks that the hold KeepAwake gives can be given back, twice too.
func TestKeepAwakeReleases(t *testing.T) {
	app := start(t, map[string]func(*core.Scope){"Hold": func(cx *core.Scope) {
		release := KeepAwake(cx, "Downloading", true)
		release()
		release()
	}})
	app.Press("text:Hold")
	app.Settle()
}

// TestPlatform checks the names the catalogs' select messages take for each desktop.
func TestPlatform(t *testing.T) {
	for os, want := range map[core.OS]string{core.OSMacOS: "macOS", core.OSWindows: "Windows", core.OSLinux: "Linux", core.OSIOS: "other"} {
		app := nsgotest.Start(t, func(cx *core.Scope) core.Node {
			return core.Text{TestID: "platform", ContentFunc: func() string { return Platform(cx) }}
		}, nsgotest.Environment(core.Environment{OS: os}))
		if got := app.Find("id:platform").Text; got != want {
			t.Errorf("Platform on %v is %q, want %q", os, got, want)
		}
	}
}
