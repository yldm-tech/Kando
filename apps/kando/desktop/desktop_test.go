package desktop

import (
	"errors"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

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

// fakeTools stands in for the system's tools until t ends: runCommand records each command and answers fail, and lookPath finds every tool in /fake/bin, an absolute path where it is. No test starts Finder, File Explorer, a file manager or a Trash.
func fakeTools(t *testing.T, fail error) *[]*exec.Cmd {
	t.Helper()
	var ran []*exec.Cmd
	var mu sync.Mutex
	prevRun, prevLook := runCommand, lookPath
	runCommand = func(cmd *exec.Cmd) error {
		mu.Lock()
		defer mu.Unlock()
		ran = append(ran, cmd)
		return fail
	}
	lookPath = func(name string) (string, error) {
		if filepath.IsAbs(name) {
			return name, nil
		}
		return filepath.Join("/fake/bin", name), nil
	}
	t.Cleanup(func() { runCommand, lookPath = prevRun, prevLook })
	return &ran
}

// TestRevealSelectsTheItem checks that Reveal has the file manager select the item, and opens no folder once it has.
func TestRevealSelectsTheItem(t *testing.T) {
	if runtime.GOOS != "darwin" && runtime.GOOS != "linux" && runtime.GOOS != "windows" {
		t.Skip("no file manager selects an item here")
	}
	ran := fakeTools(t, nil)
	film := filepath.Join(t.TempDir(), "Spring Clip.mp4")
	var errs []error
	app := start(t, map[string]func(*core.Scope){"Film": func(cx *core.Scope) { Reveal(cx, film, func(err error) { errs = append(errs, err) }) }})
	app.Press("text:Film")
	app.WaitUntil(5*time.Second, func() bool { return len(errs) == 1 })
	if errs[0] != nil || len(*ran) != 1 || len(app.OpenedFiles) != 0 {
		t.Fatalf("done heard %v, ran %v, opened %+v", errs, *ran, app.OpenedFiles)
	}
	// Linux's file managers take the item as a file: URI (showItemsCommand), the others its path.
	want := film
	if runtime.GOOS == "linux" {
		want = fileURI(film)
	}
	if cmd := (*ran)[0]; !strings.Contains(strings.Join(cmd.Args, " ")+" "+commandLine(cmd), want) {
		t.Fatalf("the file manager was not given %s: %q", want, cmd.Args)
	}
}

// TestRevealFallsBackToTheFolder checks that Reveal opens the folder holding the item when the file manager cannot select it, and says so when no app opened the folder or the item's folder is gone.
func TestRevealFallsBackToTheFolder(t *testing.T) {
	fakeTools(t, errors.New("no file manager answered"))
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
	app.WaitUntil(5*time.Second, func() bool { return len(errs) == 1 })
	if len(app.OpenedFiles) != 1 || app.OpenedFiles[0].Path != dir {
		t.Fatalf("opened %+v, want the folder %s", app.OpenedFiles, dir)
	}
	app.OpenFile = func(core.SharedFile) bool { return false }
	app.Press("text:Film")
	app.WaitUntil(5*time.Second, func() bool { return len(errs) == 2 })
	app.Press("text:Missing")
	app.WaitUntil(5*time.Second, func() bool { return len(errs) == 3 })
	if errs[0] != nil || !errors.Is(errs[1], errFolderNotOpened) || errs[2] == nil {
		t.Fatalf("done heard %v", errs)
	}
}

// commandLine is the command line cmd runs as written whole, on Windows, where it can be (explorerSelectCommand); "" elsewhere.
func commandLine(cmd *exec.Cmd) string { return sysCommandLine(cmd) }

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

// TestDroppedPaths checks that the file: URLs of a drop become paths, spaces and other escapes undone, and that anything else is left out.
func TestDroppedPaths(t *testing.T) {
	got := DroppedPaths([]string{"file:///Users/me/Movies/My%20Talk.mp4", "https://example.com/v.mp4", "file:///tmp/%E5%BD%B1%E7%89%87.mkv", "::"})
	want := []string{filepath.FromSlash("/Users/me/Movies/My Talk.mp4"), filepath.FromSlash("/tmp/影片.mkv")}
	if fmt.Sprint(got) != fmt.Sprint(want) {
		t.Fatalf("DroppedPaths = %q, want %q", got, want)
	}
}

// TestHeadlessRunsNoTool checks that after Headless, Reveal opens the item's folder through the scope, which nsgotest records, and Trash fails as unsupported, neither running a tool nor touching the item.
func TestHeadlessRunsNoTool(t *testing.T) {
	ran := fakeTools(t, nil)
	Headless()
	t.Cleanup(func() { headless.Store(false) })
	dir := t.TempDir()
	film := filepath.Join(dir, "Spring Clip.mp4")
	if err := os.WriteFile(film, []byte("film"), 0o644); err != nil {
		t.Fatal(err)
	}
	var revealed []error
	var trashed error
	app := start(t, map[string]func(*core.Scope){
		"Reveal": func(cx *core.Scope) { Reveal(cx, film, func(err error) { revealed = append(revealed, err) }) },
		"Trash":  func(cx *core.Scope) { trashed = Trash(cx, film) },
	})
	app.Press("text:Reveal")
	app.WaitUntil(5*time.Second, func() bool { return len(revealed) == 1 })
	app.Press("text:Trash")
	app.Settle()
	if revealed[0] != nil || len(app.OpenedFiles) != 1 || app.OpenedFiles[0].Path != dir {
		t.Fatalf("reveal heard %v, opened %+v, want the folder %s", revealed, app.OpenedFiles, dir)
	}
	if trashed != errors.ErrUnsupported || len(*ran) != 0 {
		t.Fatalf("Trash answered %v, ran %v", trashed, *ran)
	}
	if _, err := os.Stat(film); err != nil {
		t.Fatalf("the item is gone: %v", err)
	}
}
