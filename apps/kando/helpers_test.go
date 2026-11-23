package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"image"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
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

// The tests start the app as a user meets it, with nsgotest: newTestApp mounts the main view headless (nsgotest.Start), and the steps press, type and read what shows by selectors, as nsgotest names them: a bare string is a test ID, else an accessibility label, else a whole text; "text:" a substring and `text:"…"` a whole text; "role:button 保存" a role and its name; "id:" a test ID alone. A test that changes the app's state itself, as MyGo's tests set a field and drew a frame, calls refresh, which tells the revision and settles, in place of tt.Frame(). Headless, the graph runs on the test's goroutine, so a test reads and writes the app's state directly between steps; under `nsgo test -target` the engine runs on a goroutine of its own, so refresh, apply and the helpers that apply background updates run the change on it (onEngine), and a test that calls into the graph itself between steps does so through onEngine too.
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
//   - tt.Image() and the PNG screenshots: none headless; on a platform (nsgo test -target) tt.Capture() and tt.CapturePNG(), which skip a headless test, so call them only when nsgotest.Target() is set (shootPNG)

// TestMain reads the interface in the language Kando shows by default, whatever the languages of the machine running the tests; the screenshots read it in KANDO_SCREENSHOTS_LANG when that names one of locales.Interface (screenshot_helpers_test.go). desktop.Headless keeps every reveal and trash off the system's tools, whichever area calls them and whether or not its test replaced its hook: a reveal opens the item's folder through the scope, which nsgotest records rather than opens, and a trash fails as unsupported, so a test never opens the file manager of the machine running it nor moves its files to the Trash. A reveal no test watches shows nothing at all.
// testVideos is the library the test opens (newTestApp): a bundle named videos.kando of the test's own (testVideosFor), as every library Kando makes is, whose loose videos Kando gathers into folders of their own. Films are written beside their videos (media.Make), so it is a folder on disk.
var testVideos string

// testVideosOf is the test testVideos was made for.
var testVideosOf *testing.T

// testVideosFor makes testVideos t's own folder, once: newTestApp calls it, and so does a test that names the library before it starts its app.
func testVideosFor(t *testing.T) {
	t.Helper()
	if testVideosOf == t {
		return
	}
	testVideosOf, testVideos = t, filepath.Join(t.TempDir(), "videos"+media.LibraryExt)
	if err := os.Mkdir(testVideos, 0o755); err != nil {
		t.Fatal(err)
	}
}

func TestMain(m *testing.M) {
	testlocale.Pin(locales.Default)
	desktop.Headless()
	showItemInFolder = func(_ *ns.Scope, _ string, done func(error)) { done(nil) }
	// The tests never move the user's own log files from where earlier versions kept them (logsOldDir).
	logsOldDir = func() (string, error) { return "", nil }
	// The fake tools' folder goes once the tests have run, headless and on a platform alike.
	nsgotest.MainFunc(func() int {
		defer fakeToolsCleanup()
		return m.Run()
	})
}

// testOS is the system the tests' app runs on, a Mac unless usePlatform says another for a test.
var testOS = ns.OSMacOS

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

// fakePipeline finishes every stage at once unless block is set, in which case Download waits for cancellation. A link downloads into the test's folder as talk.mp4 (testFolder), a film whose one line, "hello", is said in its first second (hearsHello).
type fakePipeline struct {
	testFolder
	hearsHello
	block    bool
	language string
}

func (f *fakePipeline) Check() error { return nil }

func (f *fakePipeline) Download(ctx context.Context, _ string) (string, error) {
	if f.block {
		<-ctx.Done()
		return "", ctx.Err()
	}
	return f.video("talk.mp4")
}

func (f *fakePipeline) Translate(_ context.Context, segments []core.Segment, language string) ([]core.Segment, error) {
	f.language = language
	return segments, nil
}

// testFolder is where a test's fake pipeline downloads its videos: a folder of the test's own, which newTestApp gives it (useFolder), or else one of the fake's own.
type testFolder struct{ dir string }

func (f *testFolder) useFolder(dir string) {
	if f.dir == "" {
		f.dir = dir
	}
}

// video writes a video named name into the folder and returns its path, as a download does.
func (f *testFolder) video(name string) (string, error) {
	if f.dir == "" {
		dir, err := os.MkdirTemp("", "kando-test-*")
		if err != nil {
			return "", err
		}
		f.dir = dir
	}
	path := filepath.Join(f.dir, name)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return "", err
	}
	return path, os.WriteFile(path, []byte("v"), 0o644)
}

// hearsHello is the speech recognition of a test's fake pipeline (media.Hearer): a film of 30 seconds whose one line, "hello", is said in its first second.
type hearsHello struct{}

func (hearsHello) Length(context.Context, string) time.Duration { return 30 * time.Second }

func (hearsHello) Hear(_ context.Context, _ string, from, _ time.Duration, heard func(core.Segment)) (string, error) {
	if from == 0 {
		heard(core.Segment{End: time.Second, Text: "hello"})
	}
	return "", nil
}

// newTestApp starts the app's main window headless, 1100 by 720 as main opens it, on testOS in testlocale's language, with the library at /videos. Background updates are queued, so the test applies them on its own goroutine with settle, waitUntil or waitUpdate, as the engine would; each applies them and refreshes.
func newTestApp(t *testing.T, p media.Pipeline, opts ...nsgotest.Option) (*app, *nsgotest.App, chan func()) {
	t.Helper()
	// The library is testVideos, a folder of the test's own, where a fake pipeline downloads (useFolder), as a film's document is written beside its video.
	testVideosFor(t)
	if f, ok := p.(interface{ useFolder(string) }); ok {
		f.useFolder(testVideos)
	}
	updates := make(chan func(), 16)
	a := newApp(p, func(fn func()) { updates <- fn })
	a.prefs.listModels = offlineModels
	a.prefs.pingLLM = offlinePing
	a.prefs.interfaceLanguage = testlocale.Current()
	a.prefs.overMain = true
	a.tools.candidates = noCandidates
	a.edit(func(s *session) { s.LibraryDir = testVideos })
	opts = append([]nsgotest.Option{nsgotest.Size(1100, 720), nsgotest.Environment(testEnvironment())}, opts...)
	// The plugins' work on their folder still under way when the test ends is waited for, as quitting waits for it, so it writes nothing more into the test's folders once they go.
	t.Cleanup(func() {
		if !a.pluginsWaitWrites(10 * time.Second) {
			t.Error("the plugins' work on their folder did not end")
		}
	})
	// A job still running when the test ends is stopped as quitting stops it, and the test waits for it to return, so it writes nothing more into the test's folders once they go.
	t.Cleanup(func() {
		if a.queue.cancel == nil || a.queue.done == nil {
			return
		}
		a.queue.cancel(errQuitting)
		select {
		case <-a.queue.done:
		case <-time.After(5 * time.Second):
			t.Error("the job running when the test ended did not stop")
		}
	})
	// The work on the programs still under way when the test ends (toolsUI.work), as reading how yt-dlp was installed, which writes a probe file into its folder, or a download of Kando's own copy, which is stopped first, is waited for, so it writes nothing more into the test's folders once they go. What it reports is dropped meanwhile, so a report waiting for room in updates does not keep it from ending.
	t.Cleanup(func() {
		a.kitsCancel()
		idle := make(chan struct{})
		go func() {
			a.tools.work.Wait()
			close(idle)
		}()
		limit := time.After(10 * time.Second)
		for {
			select {
			case <-idle:
				return
			case <-updates:
			case <-limit:
				t.Error("the work on the programs did not end")
				return
			}
		}
	})
	return a, nsgotest.Start(t, a.testView, opts...), updates
}

// noCandidates stands in for media.Candidates in the tests: the copies of the programs on the machine running them, in its PATH and Homebrew's folder, are not the test's, so a check of the tools (toolsCheck) runs only the copies the test's pipeline names.
func noCandidates(string) []string { return nil }

// testView is the app as run starts it, for nsgotest: the revision on the test's graph, the test's root as the app's scope, and the main view. The areas' ready does not run, as in MyGo's tests, so the menu bar is not installed and wireKeys binds the ⌘ rows too: a test of the menu bar calls a.installMenu(a.cx) and reads tt.Menu, and a test that needs another ready calls what it needs.
func (a *app) testView(cx *ns.Scope) ns.Node {
	a.attach(cx)
	// nsgo.Run starts the app in theme.Appearance (main's nsgo.Appearance), the system's, which nsgotest's host has no option for, so the test app sets it as the app starts and the test's Environment (Dark) stands in for the system's
	cx.SetAppearance(theme.Appearance)
	return a.view(cx)
}

// refresh tells the revision that the test changed the app's state and settles, as MyGo's Tester.Frame drew the state the test had set. Under `nsgo test -target` the engine commits what Changed set off only after the post that ran it returned; Settle waits for that commit however long it takes.
func refresh(tt *nsgotest.App, a *app) {
	onEngine(a, a.Changed)
	tt.Settle()
}

// apply runs a background update on the engine's goroutine (onEngine), as the app's update does, and refreshes.
func apply(tt *nsgotest.App, a *app, fn func()) {
	onEngine(a, fn)
	refresh(tt, a)
}

// onEngine runs fn on the goroutine that owns the app's graph and returns once it ran. Headless that is the test's own goroutine, so fn just runs. Under `nsgo test -target` the app's engine runs on a goroutine of its own beside the test's: a Changed the test calls itself runs the bindings on the wrong goroutine, racing the engine, and tells the engine nothing, so the window never shows the state the test set; posted through the app's scope it runs where the engine commits what it changed. The test's writes before the call reach the engine through the post, so a test still sets the app's state directly between steps, then calls refresh.
func onEngine(a *app, fn func()) {
	if nsgotest.Target() == "" {
		fn()
		return
	}
	ran := make(chan struct{})
	a.cx.Post(func() {
		defer close(ran)
		fn()
	})
	<-ran
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

// typeFocused types s into the text field that has the keyboard focus, as a user types without moving it, and fails t when none has it. The press that gave the field the focus has it there by the time its step settled: headless the press sets it as the step runs, and under `nsgo test -target` a desktop window's step settles the focus the platform reports.
func typeFocused(t *testing.T, tt *nsgotest.App, s string) {
	t.Helper()
	for _, v := range append(tt.Query("role:textbox"), tt.Query("role:searchbox")...) {
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
			onEngine(a, fn)
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
	confirmAddSheet(tt)
}

// confirmAddSheet presses the Add sheet's 添加并开始 when the sheet is open, as it is after a link to one online video was submitted: it adds the link with the settings' choices, as the link field added it before the sheet.
func confirmAddSheet(tt *nsgotest.App) {
	if tt.Exists("id:addsheet-add") {
		tt.Press("id:addsheet-add")
	}
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

// capturedColor is the color of the captured window img, width points wide, at the point (x, y) in the window's points: the pixels of the point's square averaged, 8 bits a channel, in the display's color space (nsgotest's Capture).
func capturedColor(img image.Image, width ns.Points, x, y ns.Points) [3]uint8 {
	scale := float64(img.Bounds().Dx()) / float64(width)
	x0, y0 := int(float64(x)*scale), int(float64(y)*scale)
	n := max(int(scale), 1)
	var sum [3]uint32
	for py := y0; py < y0+n; py++ {
		for px := x0; px < x0+n; px++ {
			r, g, b, _ := img.At(img.Bounds().Min.X+px, img.Bounds().Min.Y+py).RGBA()
			sum[0], sum[1], sum[2] = sum[0]+r>>8, sum[1]+g>>8, sum[2]+b>>8
		}
	}
	d := uint32(n * n)
	return [3]uint8{uint8(sum[0] / d), uint8(sum[1] / d), uint8(sum[2] / d)}
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

// waitUntil applies the background updates until done holds, then refreshes once. It waits as long as nsgotest's own waits do: on Windows CI, a fake tool is a copy of the test binary that runs its script through Git for Windows' sh (writeFakeScript), and a step that starts a few of them, as a film's copy for the system's player, took over five seconds.
func waitUntil(t *testing.T, tt *nsgotest.App, a *app, updates <-chan func(), done func() bool) {
	t.Helper()
	timeout := time.After(10 * time.Second)
	for !done() {
		select {
		case fn := <-updates:
			onEngine(a, func() {
				fn()
				a.Changed()
			})
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

// offlinePing stands in for the connection test in tests, which reach no network: it asks a service on this computer, as a test's httptest server, and no other.
func offlinePing(ctx context.Context, t media.ChatTranslator) error {
	client := http.Client{Transport: loopbackOnly{}}
	if t.Client != nil {
		client = *t.Client
		client.Transport = loopbackOnly{next: t.Client.Transport}
	}
	t.Client = &client
	return t.Ping(ctx)
}

// loopbackOnly sends a request on to next, or the default transport, only when it goes to this computer.
type loopbackOnly struct{ next http.RoundTripper }

func (l loopbackOnly) RoundTrip(r *http.Request) (*http.Response, error) {
	if ip := net.ParseIP(r.URL.Hostname()); r.URL.Hostname() != "localhost" && (ip == nil || !ip.IsLoopback()) {
		return nil, errors.New("tests reach no network: " + r.URL.Host)
	}
	if l.next == nil {
		return http.DefaultTransport.RoundTrip(r)
	}
	return l.next.RoundTrip(r)
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

// serveSpeechModels points the whisper model downloads at url, from whichever download source, and pins every catalog model to data, the file the test's server serves, so the downloads pass the checksum. sources records the download source of every download.
func serveSpeechModels(t *testing.T, url string, data []byte, sources *[]string) {
	t.Helper()
	oldURL, oldModel := whisperModelBaseURL, speechModelFor
	whisperModelBaseURL = func(source string) string {
		if sources != nil {
			*sources = append(*sources, source)
		}
		return url
	}
	sum := sha256.Sum256(data)
	speechModelFor = func(id string) (media.SpeechModel, bool) {
		model, ok := media.SpeechModelByID(id)
		model.SHA256, model.Size = hex.EncodeToString(sum[:]), int64(len(data))
		return model, ok
	}
	t.Cleanup(func() { whisperModelBaseURL, speechModelFor = oldURL, oldModel })
}

// mirrorOnlyServer is Hugging Face where it cannot be reached and hf-mirror.com where it can: a download from Hugging Face is refused with 451, one from the mirror gets data. It points the whisper model downloads at itself, and records the download source of every download in sources.
func mirrorOnlyServer(t *testing.T, data []byte, sources *[]string) {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if !strings.HasPrefix(r.URL.Path, "/mirror/") {
			http.Error(w, "unavailable for legal reasons", http.StatusUnavailableForLegalReasons)
			return
		}
		http.ServeContent(w, r, "ggml-tiny.bin", time.Time{}, bytes.NewReader(data))
	}))
	t.Cleanup(srv.Close)
	serveSpeechModels(t, srv.URL, data, sources)
	whisperModelBaseURL = func(source string) string {
		*sources = append(*sources, source)
		if source == media.SpeechSourceMirror {
			return srv.URL + "/mirror"
		}
		return srv.URL + "/huggingface"
	}
}

// chromeHome makes a folder of its own the home folder of the test, with Chrome's data folder in it, where yt-dlp looks for Chrome's sign-in on a Mac, holding a profile's cookies; when closed, the folder cannot be listed, as macOS keeps another app's data from an app without Full Disk Access: chmod 000 stands in for macOS's refusal. Without Chrome, the home has no such folder. It returns Chrome's folder. Only macOS keeps a browser's folder from Kando (media.Tools.CheckLogin and Download look only there), so the test is skipped on other systems, and for root, whom chmod does not keep out.
func chromeHome(t *testing.T, chrome, closed bool) string {
	t.Helper()
	if runtime.GOOS != "darwin" || os.Geteuid() == 0 {
		t.Skip("only macOS keeps a browser's folder from Kando, and chmod 000 must keep it from this process")
	}
	home := t.TempDir()
	t.Setenv("HOME", home)
	folder := filepath.Join(home, "Library", "Application Support", "Google", "Chrome")
	if !chrome {
		return folder
	}
	if err := os.MkdirAll(filepath.Join(folder, "Default"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(folder, "Default", "Cookies"), nil, 0o600); err != nil {
		t.Fatal(err)
	}
	if closed {
		if err := os.Chmod(folder, 0); err != nil {
			t.Fatal(err)
		}
		t.Cleanup(func() { _ = os.Chmod(folder, 0o755) })
	}
	return folder
}

// chromeYTDLP writes a yt-dlp that borrows Chrome's sign-in as yt-dlp 2026.08.19 does on a Mac: it walks Chrome's data folder under $HOME for the cookies database, and when it cannot list the folder, or the folder is not there, says it could not find one (packages/media/testdata/ytdlp/errors/cookies-chrome-missing.stderr), failing the check of the sign-in and every download. With the cookies found, the check succeeds as yt-dlp's usage error for want of a URL, and a download fails as above all the same, since this yt-dlp downloads nothing.
func chromeYTDLP(t *testing.T) string {
	t.Helper()
	return writeFakeScript(t, t.TempDir(), "yt-dlp", `case "$1" in --version) echo 2026.08.19; exit 0;; esac
chrome="$HOME/Library/Application Support/Google/Chrome"
case " $* " in *" -- "*) ;; *)
  if ls "$chrome/Default" 2>/dev/null | grep -qx Cookies; then
    echo "Extracted 1 cookies from chrome"
    echo "yt-dlp: error: You must provide at least one URL." >&2
    exit 2
  fi;;
esac
echo "ERROR: could not find chrome cookies database in \"$chrome\"" >&2
exit 1`)
}
