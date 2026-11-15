package main

import (
	"context"
	"image"
	"os"
	"path/filepath"
	"reflect"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/cloud"
	"kando/apps/kando/desktop"
	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/theme"
	"kando/packages/core"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// The tests start the app as a user meets it, with nsgotest: newTestApp mounts the main view headless (nsgotest.Start), and the steps press, type and read what shows by selectors, as nsgotest names them: a bare string is a test ID, else an accessibility label, else a whole text; "text:" a substring and `text:"…"` a whole text; "role:button 保存" a role and its name; "id:" a test ID alone. A test that changes the app's state itself, as MyGo's tests set a field and drew a frame, calls refresh, which tells the revision and settles, in place of tt.Frame(). Headless, the graph runs on the test's goroutine, so a test reads and writes the app's state directly between steps.
//
// MyGo's Tester and nsgotest, step by step:
//   - tt.Frame() after changing the state: refresh(tt, a); after a step: nothing, every step settles
//   - tt.Click(label): tt.Press(label), which fails the test when nothing or more than one view matches, or the view is disabled
//   - tt.HasText(s): hasText(tt, s), a whole text; tt.Exists("text:" + s) for a substring
//   - tt.Find(text) for its frame: pageFrame(tt.Find(selector)), in the window's points
//   - tt.Type(s) into the focused field: typeFocused(t, tt, s); into a named field: tt.Type(selector, s)
//   - tt.Key(mods, key): tt.Key("mod+shift+n"), in NSGo's form, "mod" being ⌘ on the Mac the tests run as
//   - tt.Focused(label): tt.Find(label).Focused
//   - tt.Menu() and tt.ChooseMenuItem: tt.PopupMenu (the items desktop.PopupMenu shows) and tt.ChoosePopup(label...); the menu bar's are tt.Menu and tt.ChooseMenu(menu, item)
//   - tt.SetTitleBar(bar): newTestApp's ns.Environment carries TitleBar; useTitleBar(tt, bar) reports another
//   - tt.Move(x, y) away: tt.HoverOut(); tt.Advance(d): tt.Advance(d) on the test's clock
//   - tt.DropFiles(paths...): tt.DropFiles(selector, fileURLs(paths...)...)
//   - tt.Image() and the PNG screenshots: none headless, and on a platform nsgotest's Screenshot gives no bytes (nsgo#119, screenshotFixture.shoot)

// TestMain reads the interface in the language Kando shows by default, whatever the languages of the machine running the tests; the screenshots read it in KANDO_SCREENSHOTS_LANG when that names one of locales.Interface (screenshot_helpers_test.go). desktop.Headless keeps every reveal and trash off the system's tools, whichever area calls them and whether or not its test replaced its hook: a reveal opens the item's folder through the scope, which nsgotest records rather than opens, and a trash fails as unsupported, so a test never opens the file manager of the machine running it nor moves its files to the Trash. A reveal no test watches shows nothing at all.
func TestMain(m *testing.M) {
	testlocale.Pin(locales.Default)
	desktop.Headless()
	showItemInFolder = func(_ *ns.Scope, _ string, done func(error)) { done(nil) }
	if nsgotest.Target() != "" {
		// TODO(nsgo#143): remove the fake tools' folder once the tests have run on a platform too, as headless runs do here; nsgotest.Main gives the main thread to the platform and exits without returning, so nothing runs after the tests there https://github.com/nsgo-dev/nsgo/issues/143
		nsgotest.Main(m)
		return
	}
	code := m.Run()
	fakeToolsCleanup()
	os.Exit(code)
}

// testOS is the system the tests' app runs on, a Mac unless usePlatform says another for a test.
var testOS = ns.OSMacOS

// windowClosingCanceled reports whether the close c stood for was held back (core.WindowClosing.Cancel), which keeps the window open. NSGo's WindowClosing only records it, for the platform that asked, so the tests read what it recorded.
func windowClosingCanceled(c *ns.WindowClosing) bool {
	// TODO(nsgo#201): read WindowClosing's Canceled() (or close the window through nsgotest) instead of its unexported canceled field, which NSGo can rename under these tests https://github.com/nsgo-dev/nsgo/issues/201
	return reflect.ValueOf(c).Elem().FieldByName("canceled").Bool()
}

// usePlatform runs the apps t starts after it on os, with the words and the look of that system, and goes back to the Mac after t. Call it before newTestApp.
func usePlatform(t *testing.T, os ns.OS) {
	t.Helper()
	prev := testOS
	testOS = os
	t.Cleanup(func() { testOS = prev })
}

// platformName is the name the catalogs' select messages take for the system the tests' app runs on (desktop.Platform), so a test of another area can find 在访达中显示 by the words that system's build shows.
func platformName() string {
	switch testOS {
	case ns.OSWindows:
		return "Windows"
	case ns.OSLinux:
		return "Linux"
	}
	return "macOS"
}

// testEnvironment is the environment the tests' app starts in: a desktop of testOS, in testlocale's language, with the Mac's keyboard on a Mac.
func testEnvironment() ns.Environment {
	return testlocale.Environment(ns.Environment{OS: testOS, Idiom: ns.IdiomDesktop, AppleKeyboard: testOS == ns.OSMacOS, PixelRatio: 2})
}

// fakePipeline finishes every stage at once unless block is set, in which case Download waits for cancellation.
type fakePipeline struct {
	block    bool
	language string
}

func (f *fakePipeline) Check() error { return nil }

func (f *fakePipeline) Download(ctx context.Context, _ string) (string, error) {
	if f.block {
		<-ctx.Done()
		return "", ctx.Err()
	}
	return "/videos/talk.mp4", nil
}

func (f *fakePipeline) Transcribe(context.Context, string) ([]core.Segment, error) {
	return []core.Segment{{End: time.Second, Text: "hello"}}, nil
}

func (f *fakePipeline) Translate(_ context.Context, segments []core.Segment, language string) ([]core.Segment, error) {
	f.language = language
	return segments, nil
}

func (f *fakePipeline) Export(context.Context, string, []core.Segment, []core.Segment) (string, error) {
	return "/videos/talk.bilingual.srt", nil
}

// newTestApp starts the app's main window headless, 1100 by 720 as main opens it, on testOS in testlocale's language, with the library at /videos. Background updates are queued, so the test applies them on its own goroutine with settle, waitUntil or waitUpdate, as the engine would; each applies them and refreshes.
func newTestApp(t *testing.T, p media.Pipeline, opts ...nsgotest.Option) (*app, *nsgotest.App, chan func()) {
	t.Helper()
	updates := make(chan func(), 16)
	a := newApp(p, func(fn func()) { updates <- fn })
	a.prefs.listModels = offlineModels
	a.prefs.interfaceLanguage = testlocale.Current()
	a.edit(func(s *session) { s.LibraryDir = "/videos" })
	opts = append([]nsgotest.Option{nsgotest.Size(1100, 720), nsgotest.Environment(testEnvironment())}, opts...)
	return a, nsgotest.Start(t, a.testView, opts...), updates
}

// testView is the app as run starts it, for nsgotest: the revision on the test's graph, the test's root as the app's scope, and the main view. The areas' ready does not run, as in MyGo's tests, so the menu bar is not installed and wireKeys binds the ⌘ rows too: a test of the menu bar calls a.installMenu(a.cx) and reads tt.Menu, and a test that needs another ready calls what it needs.
func (a *app) testView(cx *ns.Scope) ns.Node {
	a.attach(cx)
	// nsgo.Run starts the app in theme.Appearance (main's nsgo.Appearance), which nsgotest's host has no option for, so the test app forces it as the app starts
	cx.SetAppearance(theme.Appearance)
	return a.view(cx)
}

// refresh tells the revision that the test changed the app's state and settles, as MyGo's Tester.Frame drew the state the test had set.
func refresh(tt *nsgotest.App, a *app) {
	a.Changed()
	tt.Settle()
}

// apply runs a background update on the test's goroutine, as the engine runs one, and refreshes.
func apply(tt *nsgotest.App, a *app, fn func()) {
	fn()
	refresh(tt, a)
}

// hasText reports whether a text shows that is s as a whole.
func hasText(tt *nsgotest.App, s string) bool { return tt.Exists(whole(s)) }

// whole is the selector of a text that is s as a whole.
func whole(s string) string { return `text:"` + s + `"` }

// pageFrame is v's frame in the window's points, as its ancestors and their scroll offsets place it.
func pageFrame(v *nsgotest.View) ns.Bounds {
	b := v.Frame
	for p := v.Parent; p != nil; p = p.Parent {
		b.Origin.X += p.Frame.Origin.X - p.Offset.X
		b.Origin.Y += p.Frame.Origin.Y - p.Offset.Y
	}
	return b
}

// typeFocused types s into the text field that has the keyboard focus, as a user types without moving it, and fails t when none has it.
func typeFocused(t *testing.T, tt *nsgotest.App, s string) {
	t.Helper()
	for _, v := range tt.Query("role:textbox") {
		if v.Focused {
			tt.Type("view:"+strconv.FormatUint(uint64(v.ID), 10), s)
			return
		}
	}
	t.Fatalf("no text field has the focus\n%s", tt.Dump())
}

// fileURLs are paths as a drop from the file manager carries them: file: URLs, escaped, with a Windows drive after the slash (file:///C:/Users/…), as each platform reports them.
func fileURLs(paths ...string) []string {
	out := make([]string, len(paths))
	for i, p := range paths {
		out[i] = core.FileURL(p)
	}
	return out
}

// useTitleBar reports bar as the window's hidden title bar, as the platform does once the traffic lights moved, and settles.
func useTitleBar(tt *nsgotest.App, bar ns.Edges) {
	env := testEnvironment()
	env.TitleBar = bar
	tt.SetEnvironment(env)
}

func settle(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for a.running() {
		select {
		case fn := <-updates:
			fn()
		case <-timeout:
			t.Fatalf("job still running, status %q", a.queue.status)
		}
	}
	refresh(tt, a)
}

func startJob(t *testing.T, tt *nsgotest.App, source string) {
	t.Helper()
	tt.Type(testlocale.Messages().FilmsLinkLabel(), source)
	tt.Press(whole(testlocale.Messages().FilmsStart()))
}

func filmRows(t *testing.T, tt *nsgotest.App) int {
	t.Helper()
	tops := []ns.Points{filmTop(t, tt, "one"), filmTop(t, tt, "two"), filmTop(t, tt, "three")}
	slices.Sort(tops)
	rows := 1
	for i := 1; i < len(tops); i++ {
		if tops[i]-tops[i-1] > 4 {
			rows++
		}
	}
	return rows
}

func filmTop(t *testing.T, tt *nsgotest.App, title string) ns.Points {
	t.Helper()
	return pageFrame(tt.Find(whole(title))).Origin.Y
}

func abs32[T ~float32](v T) T {
	if v < 0 {
		return -v
	}
	return v
}

func containsPath(paths []string, want string) bool {
	want = filepath.Clean(want)
	for _, path := range paths {
		if filepath.Clean(path) == want {
			return true
		}
	}
	return false
}

func wantText(t *testing.T, tt *nsgotest.App, text string) {
	t.Helper()
	if textCount(tt, text) == 0 {
		t.Fatalf("missing %q in %q", text, tt.Texts())
	}
}

func wantNoText(t *testing.T, tt *nsgotest.App, text string) {
	t.Helper()
	if textCount(tt, text) != 0 {
		t.Fatalf("unexpected %q in %q", text, tt.Texts())
	}
}

func textCount(tt *nsgotest.App, s string) int {
	n := 0
	for _, text := range tt.Texts() {
		if strings.Contains(text, s) {
			n++
		}
	}
	return n
}

func sampleStage(img *image.RGBA) [3]uint8 {
	// The film picture fills the page while it is open, so the middle of the window is on it.
	b := img.Bounds()
	x, y := (b.Min.X+b.Max.X)/2, (b.Min.Y+b.Max.Y)/2
	if x >= b.Max.X {
		x = b.Max.X - 1
	}
	if y >= b.Max.Y {
		y = b.Max.Y - 1
	}
	r, g, b0, _ := img.At(x, y).RGBA()
	return [3]uint8{uint8(r >> 8), uint8(g >> 8), uint8(b0 >> 8)}
}

func takeUpload(t *testing.T, uploadedFiles <-chan []cloud.File) []cloud.File {
	t.Helper()
	select {
	case files := <-uploadedFiles:
		return files
	case <-time.After(5 * time.Second):
		t.Fatal("upload did not run")
	}
	return nil
}

func keysOf(files []cloud.File) []string {
	out := make([]string, len(files))
	for i, file := range files {
		out[i] = file.Key
	}
	return out
}

// waitUntil applies the background updates until done holds, then refreshes once.
func waitUntil(t *testing.T, tt *nsgotest.App, a *app, updates <-chan func(), done func() bool) {
	t.Helper()
	timeout := time.After(5 * time.Second)
	for !done() {
		select {
		case fn := <-updates:
			fn()
			a.Changed()
		case <-timeout:
			t.Fatal("timed out waiting for a background update")
		}
	}
	refresh(tt, a)
}

// waitUpdate applies the next background update and refreshes.
func waitUpdate(t *testing.T, tt *nsgotest.App, a *app, updates <-chan func()) {
	t.Helper()
	select {
	case fn := <-updates:
		apply(tt, a, fn)
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a background update")
	}
}

// wantSettings fails t unless the settings are open at section, one of the settings package's Section values. Tests outside the settings area check the settings' own state this way, never a.page, because the settings are a window of their own.
func wantSettings(t *testing.T, a *app, section string) {
	t.Helper()
	if !a.prefs.open || a.prefs.section != section {
		t.Fatalf("settings open %v at %q, want open at %q", a.prefs.open, a.prefs.section, section)
	}
}

// titleBarShifts starts the window twice, without a title bar and under bar, after setup has run on each app, and returns how far down each text sits under bar, and the window shown under it.
func titleBarShifts(t *testing.T, bar ns.Edges, setup func(*app), texts ...string) ([]ns.Points, *nsgotest.App) {
	t.Helper()
	plain, plainTT, _ := newTestApp(t, &fakePipeline{})
	barred, barredTT, _ := newTestApp(t, &fakePipeline{})
	useTitleBar(barredTT, bar)
	if setup != nil {
		setup(plain)
		setup(barred)
	}
	refresh(plainTT, plain)
	refresh(barredTT, barred)
	shifts := make([]ns.Points, len(texts))
	for i, text := range texts {
		before := pageFrame(plainTT.Find(whole(text)))
		after := pageFrame(barredTT.Find(whole(text)))
		shifts[i] = after.Origin.Y - before.Origin.Y
	}
	return shifts, barredTT
}

// offlineModels stands in for an AI service's model list in tests, which reach no network: the service offers the model already chosen, or none.
func offlineModels(_ context.Context, t media.ChatTranslator) ([]string, error) {
	if t.Model == "" {
		return nil, nil
	}
	return []string{t.Model}, nil
}

// clickRow puts the keyboard in the field of the form row labeled label, as a click on the row does, and fails t unless the field took the focus.
func clickRow(t *testing.T, tt *nsgotest.App, label string) {
	t.Helper()
	tt.Focus(label)
	if !tt.Find(label).Focused {
		t.Fatalf("the field of the row %q did not take the focus", label)
	}
}

// fakeAwake stands in for desktop.KeepAwake, which needs a running app: it records each hold taken and how many are still held.
type fakeAwake struct {
	holds []awakeHold
	held  int
}

// awakeHold is one call of fakeAwake.keepAwake: why it was taken, whether it keeps the display on too, and whether it was given back.
type awakeHold struct {
	reason   string
	display  bool
	released bool
}

func (f *fakeAwake) keepAwake(reason string, display bool) func() {
	f.holds = append(f.holds, awakeHold{reason: reason, display: display})
	i := len(f.holds) - 1
	f.held++
	return func() {
		if !f.holds[i].released {
			f.holds[i].released = true
			f.held--
		}
	}
}

// applyNext runs the next background update, as the engine would, for a test that reads the app's state rather than the window.
func applyNext(t *testing.T, updates <-chan func()) {
	t.Helper()
	select {
	case fn := <-updates:
		fn()
	case <-time.After(2 * time.Second):
		t.Fatal("timed out waiting for a background update")
	}
}

// shows reports whether s is a part of a text that shows, or of a view's accessible name.
func shows(tt *nsgotest.App, s string) bool {
	if tt.Exists("text:" + s) {
		return true
	}
	return tt.Exists("label:/" + regexp.QuoteMeta(s) + "/")
}

// finds reports whether a text that shows, or a view's accessible name, is s as a whole.
func finds(tt *nsgotest.App, s string) bool {
	return tt.Exists(whole(s)) || tt.Exists(`label:"`+s+`"`)
}

// popupLabels are the labels of the context menu showing, "-" for a separator.
func popupLabels(tt *nsgotest.App) []string {
	var out []string
	for _, it := range tt.PopupMenu {
		if it.Separator {
			out = append(out, "-")
			continue
		}
		out = append(out, it.Label)
	}
	return out
}

// backgroundUnder is the first background drawn under v: its own, or the nearest of its ancestors'.
func backgroundUnder(v *nsgotest.View) ns.Color {
	for c := v; c != nil; c = c.Parent {
		if c.Style.Background != 0 {
			return c.Style.Background
		}
	}
	return 0
}
