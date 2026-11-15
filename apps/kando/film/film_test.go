package film

import (
	"fmt"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"
	"kando/packages/core"

	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestMain reads the interface in the language Kando shows by default, whatever the languages of the machine running the tests.
func TestMain(m *testing.M) {
	testlocale.Pin(locales.Default)
	nsgotest.Main(m)
}

// fakePlay is the picture of a fakeApp.
type fakePlay struct {
	playing bool
	head    time.Duration
	length  time.Duration
}

func (p *fakePlay) Playing() bool         { return p.playing }
func (p *fakePlay) Toggle()               { p.playing = !p.playing }
func (p *fakePlay) Head() time.Duration   { return p.head }
func (p *fakePlay) Length() time.Duration { return p.length }

// fakeApp is the open film the player reads, kept as plain fields as the app keeps them, with the calls the player made.
type fakeApp struct {
	live.Rev
	item       Item
	play       *fakePlay
	path       string
	err        string
	loaded     *ns.VideoInfo
	progress   []time.Duration
	ended      int
	failed     error
	ref        ns.Ref
	video      Video
	subs       Subs
	side       string
	look       Look
	mode       string
	order      string
	untrans    string
	yield      bool
	delay      time.Duration
	panel      bool
	styleView  bool
	modeMenu   bool
	speedMenu  bool
	rate       float64
	search     string
	focus      bool
	osd        string
	info       Info
	upload     Upload
	uploadNote bool
	volume     float64
	muted      bool
	fills      bool
	calls      []string
}

func newFakeApp() *fakeApp {
	return &fakeApp{
		item: Item{Key: "talk", Title: "Talk", JobID: "job", Stage: core.StageComplete, VideoPath: "/films/talk/talk.mp4", SubtitlePath: "/films/talk/talk.bilingual.srt", Language: "简体中文"},
		path: "/films/talk/talk.mp4",
		subs: Subs{Key: "talk", Ready: true, Cues: []core.Segment{
			{Start: time.Second, End: 3 * time.Second, Text: "你好。\nHello."},
			{Start: 4 * time.Second, End: 6 * time.Second, Text: "Bye."},
		}},
		look: DefaultLook(), mode: SubBilingual, order: SubTranslationFirst, untrans: UntranslatedDim, yield: true,
		panel: true, rate: 1, volume: 1, fills: true,
		video: Video{Source: "/films/talk/talk.mp4", Paused: true, Volume: 1, Rate: 1},
	}
}

func (f *fakeApp) call(s string)                               { f.calls = append(f.calls, s) }
func (f *fakeApp) OpenKey() string                             { return f.item.Key }
func (f *fakeApp) SetOpenKey(key string)                       { f.call("open " + key); f.item.Key = key }
func (f *fakeApp) StopPlayback()                               { f.call("stop") }
func (f *fakeApp) Reveal(path string)                          { f.call("reveal " + path) }
func (f *fakeApp) Cancel(id string)                            { f.call("cancel " + id) }
func (f *fakeApp) RunningJob() string                          { return "" }
func (f *fakeApp) ProcessFilm()                                { f.call("process") }
func (f *fakeApp) Remove(id string)                            { f.call("remove " + id) }
func (f *fakeApp) RetryOpen(id string)                         { f.call("retry " + id) }
func (f *fakeApp) TranslateFilmAgain()                         { f.call("translate again") }
func (f *fakeApp) TranslateFilmMissing()                       { f.call("translate missing") }
func (f *fakeApp) StageLabel(_ *ns.Scope, s core.Stage) string { return string(s) }
func (f *fakeApp) FillsWindow() bool                           { return f.fills }
func (f *fakeApp) Play() Player {
	if f.play == nil {
		return nil
	}
	return f.play
}
func (f *fakeApp) PlayPath() string                      { return f.path }
func (f *fakeApp) PlayErr() string                       { return f.err }
func (f *fakeApp) ErrText(_ *ns.Scope, id string) string { return "error " + id }
func (f *fakeApp) PosterFile(string) string              { return "" }
func (f *fakeApp) PlayHead() time.Duration {
	if f.play == nil {
		return 0
	}
	return f.play.head
}
func (f *fakeApp) PlayLength() time.Duration {
	if f.play == nil {
		return 0
	}
	return f.play.length
}
func (f *fakeApp) Video() Video      { return f.video }
func (f *fakeApp) VideoRef() *ns.Ref { return &f.ref }
func (f *fakeApp) VideoLoaded(info ns.VideoInfo) {
	f.loaded = &info
	f.play = &fakePlay{length: info.Duration}
	f.video.Paused = false
}
func (f *fakeApp) VideoProgress(position, _ time.Duration) bool {
	f.progress = append(f.progress, position)
	if f.play != nil {
		f.play.head = position
	}
	return false
}
func (f *fakeApp) VideoEnded()                             { f.ended++ }
func (f *fakeApp) VideoFailed(err error)                   { f.failed = err }
func (f *fakeApp) Subs() Subs                              { return f.subs }
func (f *fakeApp) FilmSide() string                        { return f.side }
func (f *fakeApp) SetFilmSide(side string)                 { f.side = side }
func (f *fakeApp) Seek(at time.Duration)                   { f.call("seek " + at.String()) }
func (f *fakeApp) Skip(d time.Duration)                    { f.call("skip " + d.String()) }
func (f *fakeApp) PrevLine()                               { f.call("prev line") }
func (f *fakeApp) NextLine()                               { f.call("next line") }
func (f *fakeApp) ReplayLine()                             { f.call("replay line") }
func (f *fakeApp) ToggleLoopLine()                         { f.call("loop") }
func (f *fakeApp) LoopLine() bool                          { return false }
func (f *fakeApp) Cue() *Look                              { return &f.look }
func (f *fakeApp) LookChanged()                            { f.call("look") }
func (f *fakeApp) SubMode() string                         { return f.mode }
func (f *fakeApp) SetSubMode(mode string)                  { f.mode = mode }
func (f *fakeApp) SubOrder() string                        { return f.order }
func (f *fakeApp) SetSubOrder(order string)                { f.order = order }
func (f *fakeApp) Untranslated() string                    { return f.untrans }
func (f *fakeApp) YieldSubs() bool                         { return f.yield }
func (f *fakeApp) Delay() time.Duration                    { return f.delay }
func (f *fakeApp) SetDelay(d time.Duration)                { f.delay = d }
func (f *fakeApp) Panel() bool                             { return f.panel }
func (f *fakeApp) TogglePanel()                            { f.panel = !f.panel }
func (f *fakeApp) StyleView() bool                         { return f.styleView }
func (f *fakeApp) SetStyleView(on bool)                    { f.styleView = on }
func (f *fakeApp) ModeMenu() *bool                         { return &f.modeMenu }
func (f *fakeApp) Rate() float64                           { return f.rate }
func (f *fakeApp) SetRate(rate float64)                    { f.rate = rate }
func (f *fakeApp) SpeedMenu() *bool                        { return &f.speedMenu }
func (f *fakeApp) Search() *string                         { return &f.search }
func (f *fakeApp) SearchFocus() *bool                      { return &f.focus }
func (f *fakeApp) OSD() string                             { return f.osd }
func (f *fakeApp) LibraryDir() string                      { return "/films/我的影片.kando" }
func (f *fakeApp) LibraryPath() string                     { return "/films/library.json" }
func (f *fakeApp) ClipPoster(time.Duration) ([]byte, bool) { return nil, false }
func (f *fakeApp) Info() Info                              { return f.info }
func (f *fakeApp) FilmUpload() Upload                      { return f.upload }
func (f *fakeApp) BeginFilmUpload()                        { f.call("upload") }
func (f *fakeApp) CancelUpload()                           { f.call("cancel upload") }
func (f *fakeApp) UploadNote() *bool                       { return &f.uploadNote }
func (f *fakeApp) OpenSettings(section string)             { f.call("settings " + section) }
func (f *fakeApp) ToggleFullScreen()                       { f.call("full screen") }
func (f *fakeApp) Volume() float64                         { return f.volume }
func (f *fakeApp) Muted() bool                             { return f.muted }
func (f *fakeApp) SetVolume(v float64)                     { f.volume = v }
func (f *fakeApp) ToggleMute()                             { f.muted = !f.muted }
func (f *fakeApp) FeedbackSubtitles()                      { f.call("feedback") }

// start shows the open film of a as the main window does, on a Mac, in testlocale's language.
func start(t *testing.T, a *fakeApp) *nsgotest.App {
	t.Helper()
	env := testlocale.Environment(ns.Environment{OS: ns.OSMacOS, Idiom: ns.IdiomDesktop, AppleKeyboard: true})
	return nsgotest.Start(t, func(cx *ns.Scope) ns.Node {
		a.Attach(cx)
		return ns.View{Style: ns.Style{FlexGrow: 1, FlexShrink: 1, AlignItems: ns.AlignStretch}, Children: []ns.Node{View{App: a, Item: live.Func(a, func() Item { return a.item })}}}
	}, nsgotest.Size(1100, 720), nsgotest.Environment(env))
}

// change runs fn on the app's state as a background report would, and settles.
func change(app *nsgotest.App, a *fakeApp, fn func()) {
	fn()
	a.Changed()
	app.Settle()
}

func whole(s string) string { return `text:"` + s + `"` }

// TestTheSystemsPlayerPlaysTheFilm checks the page's core.Video: it plays the app's source held until it has loaded, at the app's volume and speed, and hands what the player reports to the app; once loaded, the transport offers 播放 in place of 正在打开.
func TestTheSystemsPlayerPlaysTheFilm(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	msgs := testlocale.Messages()
	v := app.Find("id:film-video")
	if v.Video.Source != a.path || !v.Video.Paused || v.Video.Volume != 1 || v.Video.Rate != 1 {
		t.Fatalf("the player plays %+v", v.Video)
	}
	if !app.Exists("id:film-play") || app.Enabled("id:film-play") || app.Find("id:film-play").Props.Label != msgs.FilmOpening() {
		t.Fatalf("the play button before the film loaded: %+v", app.Find("id:film-play").Props)
	}
	app.Play("id:film-video", ns.VideoInfo{Duration: 90 * time.Second, Size: ns.Size{Width: 1920, Height: 1080}})
	a.Changed()
	app.Settle()
	if a.loaded == nil || a.loaded.Duration != 90*time.Second {
		t.Fatalf("the app heard the load as %+v", a.loaded)
	}
	if v := app.Find("id:film-video"); v.Video.Paused {
		t.Fatalf("the player stayed paused once loaded: %+v", v.Video)
	}
	if got := app.Find("id:film-play").Props.Label; got != msgs.FilmPlay() {
		t.Fatalf("the play button is %q once loaded, want %q", got, msgs.FilmPlay())
	}
	app.Progress("id:film-video", 2*time.Second, 90*time.Second)
	app.Progress("id:film-video", 90*time.Second, 90*time.Second)
	if !slices.Equal(a.progress, []time.Duration{2 * time.Second, 90 * time.Second}) || a.ended != 1 {
		t.Fatalf("the app heard progress %v and %d ends", a.progress, a.ended)
	}
	change(app, a, func() { a.video.Muted, a.video.Volume, a.video.Rate = true, 0.4, 1.5 })
	if v := app.Find("id:film-video"); !v.Video.Muted || v.Video.Volume != 0.4 || v.Video.Rate != 1.5 {
		t.Fatalf("the player did not follow the app: %+v", v.Video)
	}
}

// TestSubtitlesFollowThePlayhead checks that the picture shows the line playing as the system's player moves on, the next one once the clock reaches its start between two reports, and none between lines.
func TestSubtitlesFollowThePlayhead(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	app.Play("id:film-video", ns.VideoInfo{Duration: 10 * time.Second})
	a.play.playing = true
	app.Progress("id:film-video", 1500*time.Millisecond, 10*time.Second)
	lines := func() []string {
		if !app.Exists("id:film-subtitle-lines") {
			return nil
		}
		var out []string
		var walk func(v *nsgotest.View)
		walk = func(v *nsgotest.View) {
			if v.Text != "" && !slices.Contains(out, v.Text) {
				out = append(out, v.Text)
			}
			for _, c := range v.Children {
				walk(c)
			}
		}
		walk(app.Find("id:film-subtitle-lines"))
		return out
	}
	if got := lines(); !slices.Equal(got, []string{"你好。", "Hello."}) {
		t.Fatalf("on the first line the picture shows %q", got)
	}
	app.Progress("id:film-video", 3500*time.Millisecond, 10*time.Second)
	if got := lines(); got != nil {
		t.Fatalf("between lines the picture shows %q", got)
	}
	// The second line starts half a second after the last report; the clock moves the player on to it without another.
	a.play.head = 4100 * time.Millisecond
	app.Advance(600 * time.Millisecond)
	if got := lines(); !slices.Equal(got, []string{"Bye."}) {
		t.Fatalf("at the second line's start the picture shows %q", got)
	}
	change(app, a, func() { a.mode = SubOff })
	if got := lines(); got != nil {
		t.Fatalf("with the subtitles off the picture shows %q", got)
	}
}

// TestPlayerStaysWhileTheAppChanges checks that a change of the app's state that leaves the page as it was, as after every key press and report, builds none of it again: the picture, the system's player and the side panel keep their views.
func TestPlayerStaysWhileTheAppChanges(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	app.Play("id:film-video", ns.VideoInfo{Duration: 10 * time.Second})
	a.Changed()
	app.Settle()
	ids := map[string]uint32{}
	for _, id := range []string{"film-stage", "film-video", "film-panel", "film-transport", "film-search"} {
		ids[id] = uint32(app.Find("id:" + id).ID)
	}
	for range 3 {
		a.Changed()
		app.Settle()
	}
	for id, before := range ids {
		if after := uint32(app.Find("id:" + id).ID); after != before {
			t.Fatalf("%s was built again: %d, was %d", id, after, before)
		}
	}
}

// TestPanelTabsAndTheStyleView presses the side panel's tabs and checks each shows its own: the subtitle list, the 片段, the film's files and the style view, whose preset cards set the look.
func TestPanelTabsAndTheStyleView(t *testing.T) {
	a := newFakeApp()
	a.info = Info{Folder: "/films/talk", Files: []File{{Name: "talk.bilingual.srt", Size: 2048}, {Name: "talk.mp4", Size: 5 << 20}}}
	app := start(t, a)
	msgs := testlocale.Messages()
	if !app.Exists(whole("你好。")) || !app.Exists(whole(msgs.FilmClickToJump())) {
		t.Fatalf("the subtitle tab lists %q", app.Texts())
	}
	app.Press("id:film-tab-clips")
	if a.side != "clips" || !app.Exists(whole(msgs.FilmClipsNote())) {
		t.Fatalf("the clips tab: side %q texts %q", a.side, app.Texts())
	}
	app.Press("id:film-tab-info")
	for _, want := range []string{msgs.FilmFolder(), "talk.mp4", "talk.bilingual.srt", msgs.FilmKindVideo() + " · 5.0 MB"} {
		if !app.Exists(whole(want)) {
			t.Fatalf("the info tab misses %q: %q", want, app.Texts())
		}
	}
	// The video comes first, as the prototype lists a film's folder.
	if texts := app.Texts(); slices.Index(texts, "talk.mp4") > slices.Index(texts, "talk.bilingual.srt") {
		t.Fatalf("the info tab lists %q", texts)
	}
	app.Press("id:film-info-file-talk.mp4")
	if !slices.Contains(a.calls, "reveal "+filepath.Join("/films/talk", "talk.mp4")) {
		t.Fatalf("revealing the video called %q", a.calls)
	}
	app.Press("id:film-tab-style")
	if !a.styleView || !app.Exists(whole(msgs.FilmStyleReset())) {
		t.Fatalf("the style tab: style view %v texts %q", a.styleView, app.Texts())
	}
	app.Press("id:film-style-preset-box")
	if a.look.Preset != "box" || !a.look.Box || a.look.Shadow || !slices.Contains(a.calls, "look") {
		t.Fatalf("the box preset left %+v, calls %q", a.look, a.calls)
	}
	app.Press("id:film-style-reset")
	if a.look != DefaultLook() {
		t.Fatalf("reset left %+v", a.look)
	}
}

// TestCueRowsSeekAndCopy checks the subtitle list: a click plays from a line, its delay counted, and a right click offers to copy it, both lines or one, for a bilingual line.
// TestCueListScrollsWithinThePanel checks that a film's many lines scroll in the panel rather than stretch it past the window: NSGo's views keep the height of their content unless they may shrink, so a panel whose boxes between the window and the list only grew would leave the list nothing to scroll.
func TestCueListScrollsWithinThePanel(t *testing.T) {
	a := newFakeApp()
	a.subs.Cues = nil
	for i := range 200 {
		at := time.Duration(i) * 2 * time.Second
		a.subs.Cues = append(a.subs.Cues, core.Segment{Start: at, End: at + time.Second, Text: fmt.Sprintf("第 %d 句", i+1)})
	}
	app := start(t, a)
	list := app.Find("id:film-cues")
	if list.Frame.Size.Height >= 720 || list.Content.Height <= list.Frame.Size.Height {
		t.Fatalf("the list is %v high, its content %v, in a window 720 high", list.Frame.Size.Height, list.Content.Height)
	}
	app.Scroll("id:film-cues", 2000)
	if list := app.Find("id:film-cues"); list.Offset.Y <= 0 {
		t.Fatalf("the list did not scroll: offset %v", list.Offset)
	}
}

func TestCueRowsSeekAndCopy(t *testing.T) {
	a := newFakeApp()
	a.delay = 500 * time.Millisecond
	app := start(t, a)
	msgs := testlocale.Messages()
	app.Press(whole("你好。"))
	if !slices.Contains(a.calls, "seek 1.5s") {
		t.Fatalf("a click on the first line called %q", a.calls)
	}
	app.RightClick(whole("你好。"))
	var labels []string
	for _, it := range app.PopupMenu {
		labels = append(labels, it.Label)
	}
	if want := []string{msgs.FilmCopyLine(), msgs.FilmCopyOriginal(), msgs.FilmCopyTranslation()}; !slices.Equal(labels, want) {
		t.Fatalf("the bilingual line's menu is %q, want %q", labels, want)
	}
	app.ChoosePopup(msgs.FilmCopyOriginal())
	if app.Clipboard != "Hello." {
		t.Fatalf("拷贝原文 copied %q", app.Clipboard)
	}
	app.RightClick(whole("Bye."))
	if len(app.PopupMenu) != 1 {
		t.Fatalf("the line without a translation offers %d items", len(app.PopupMenu))
	}
	app.ChoosePopup(msgs.FilmCopyLine())
	if app.Clipboard != "Bye." {
		t.Fatalf("拷贝台词 copied %q", app.Clipboard)
	}
}

// TestModeMenuPicksTheMode opens the transport's subtitle menu, picks a mode in it, and checks that the menu closes; 样式… opens the style view.
func TestModeMenuPicksTheMode(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	msgs := testlocale.Messages()
	app.Press("id:film-mode")
	if !a.modeMenu || !app.Exists(whole(msgs.FilmDelay())) {
		t.Fatalf("the menu did not open: %q", app.Texts())
	}
	app.Press("id:film-mode-" + SubOriginal)
	if a.mode != SubOriginal || a.modeMenu {
		t.Fatalf("mode %q menu %v", a.mode, a.modeMenu)
	}
	if got := app.Find("id:film-mode").Props.Label; got != msgs.FilmSubtitlesMode(msgs.FilmModeOriginal()) {
		t.Fatalf("the button is named %q", got)
	}
	app.Press("id:film-mode")
	app.Press("id:film-style")
	if !a.styleView || a.modeMenu {
		t.Fatalf("样式… left style view %v menu %v", a.styleView, a.modeMenu)
	}
}

// TestSpeedMenuSetsTheRate opens the transport's speed menu and picks 1.5×; the button keeps its width at every speed, so the scrubber beside it does not move.
func TestSpeedMenuSetsTheRate(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	width := app.Find("id:film-speed").Frame.Size.Width
	app.Press("id:film-speed")
	if !a.speedMenu {
		t.Fatal("the speed menu did not open")
	}
	app.Press("id:film-speed-1.5")
	if a.rate != 1.5 || a.speedMenu {
		t.Fatalf("rate %v menu %v", a.rate, a.speedMenu)
	}
	if got := app.Find("id:film-speed").Frame.Size.Width; got != width {
		t.Fatalf("the speed button is %v wide at 1.5×, %v at 1×", got, width)
	}
}

// TestTopBarActsOnTheFilm checks the title bar: the back button closes the film, and its start keeps clear of the traffic lights only when the film fills the window.
func TestTopBarActsOnTheFilm(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	env := testlocale.Environment(ns.Environment{OS: ns.OSMacOS, Idiom: ns.IdiomDesktop, AppleKeyboard: true, TitleBar: ns.Edges{Top: 28, Left: 80}})
	app.SetEnvironment(env)
	back := app.Find("id:film-back")
	if x := back.Frame.Origin.X; x < 80+backGap {
		t.Fatalf("the back button starts at %v under the traffic lights", x)
	}
	change(app, a, func() { a.fills = false })
	if x := app.Find("id:film-back").Frame.Origin.X; x != 20 {
		t.Fatalf("the back button starts at %v beside the sidebar, want 20", x)
	}
	app.Press("id:film-back")
	if !slices.Contains(a.calls, "stop") || a.item.Key != "" {
		t.Fatalf("back called %q", a.calls)
	}
}

// TestPanelShowsWithoutAPicture checks that a film with no video to play shows its side panel even when the panel is off, as it has no transport to bring the panel back from.
func TestPanelShowsWithoutAPicture(t *testing.T) {
	a := newFakeApp()
	a.panel, a.path, a.video = false, "", Video{}
	app := start(t, a)
	if !app.Exists("id:film-panel") || app.Exists("id:film-transport") || app.Exists("id:film-video") {
		t.Fatalf("a film without a picture: panel %v transport %v", app.Exists("id:film-panel"), app.Exists("id:film-transport"))
	}
	change(app, a, func() { a.path, a.video = "/films/talk/talk.mp4", Video{Source: "/films/talk/talk.mp4"} })
	app.Advance(time.Second)
	if app.Exists("id:film-panel") {
		t.Fatal("the panel stayed off a film with a picture while it is off")
	}
}

// TestStyledLinesFollowModeAndOrder checks which lines of a cue show, and in which order, for each mode.
func TestStyledLinesFollowModeAndOrder(t *testing.T) {
	const cue = "你好\nhello"
	for _, tc := range []struct {
		mode, order string
		want        []string
	}{
		{SubBilingual, SubTranslationFirst, []string{"你好", "hello"}},
		{SubBilingual, SubOriginalFirst, []string{"hello", "你好"}},
		{SubTranslation, SubOriginalFirst, []string{"你好"}},
		{SubOriginal, SubTranslationFirst, []string{"hello"}},
		{SubOff, SubOriginalFirst, []string{"hello", "你好"}},
	} {
		a := newFakeApp()
		a.mode, a.order = tc.mode, tc.order
		var got []string
		for _, l := range styledLines(a, cue) {
			got = append(got, l.text)
		}
		if !slices.Equal(got, tc.want) {
			t.Fatalf("%s %s: %q, want %q", tc.mode, tc.order, got, tc.want)
		}
	}
}

// TestPictureLinesForAMissingTranslation checks what the picture shows in 仅译文 for a cue without a translation: the original dimmed, or nothing when the setting leaves it blank.
func TestPictureLinesForAMissingTranslation(t *testing.T) {
	a := newFakeApp()
	a.mode = SubTranslation
	if got := pictureLines(a, "hello"); len(got) != 1 || got[0].text != "hello" || !got[0].dim {
		t.Fatalf("dim: %+v", got)
	}
	if got := pictureLines(a, "你好\nhello"); len(got) != 1 || got[0].text != "你好" || got[0].dim {
		t.Fatalf("translated: %+v", got)
	}
	a.untrans = UntranslatedBlank
	if got := pictureLines(a, "hello"); len(got) != 0 {
		t.Fatalf("blank: %+v", got)
	}
	a.mode = SubBilingual
	if got := pictureLines(a, "hello"); len(got) != 1 || got[0].dim {
		t.Fatalf("bilingual: %+v", got)
	}
}

// TestSubtitleOutlineFollowsTheGlyphs checks that an outlined line is drawn again in black under itself, ten copies after the prototype's text-shadow, which follow the glyphs as a box's shadow would not, and that only the line itself is read by assistive technology.
func TestSubtitleOutlineFollowsTheGlyphs(t *testing.T) {
	line := styledLine{text: "每条河都始于高山", face: Face{Size: 30, Color: "#FFFFFF", Bold: true}}
	for _, outline := range []bool{false, true} {
		app := nsgotest.Start(t, func(cx *ns.Scope) ns.Node {
			return ns.View{TestID: "line", Children: []ns.Node{subtitleLine(func() string { return line.text }, line, false, outline)}}
		}, nsgotest.Size(400, 120))
		// TODO(nsgo#119): check the pixels, dark among the glyphs and none just outside the line's box, once nsgotest hands a test the bytes of its Screenshot https://github.com/nsgo-dev/nsgo/issues/119
		copies := app.Query(whole(line.text))
		want := 1
		if outline {
			want = 1 + len(outlineCopies) + 1
		}
		if len(copies) != want {
			t.Fatalf("outline %v draws the line %d times, want %d", outline, len(copies), want)
		}
	}
}

func TestDelayText(t *testing.T) {
	m := testlocale.Messages()
	for d, want := range map[time.Duration]string{
		0:                       "0.0 秒",
		300 * time.Millisecond:  "+0.3 秒",
		-100 * time.Millisecond: "-0.1 秒",
		-20 * time.Millisecond:  "0.0 秒",
		1500 * time.Millisecond: "+1.5 秒",
	} {
		if got := DelayTextIn(m, d); got != want {
			t.Errorf("DelayText(%v) = %q, want %q", d, got, want)
		}
	}
	for text, want := range map[string]float64{"+0.3": 0.3, "-1": -1, "0.5 秒": 0.5, "+1.5 s": 1.5} {
		if got, ok := parseDelay(text); !ok || got != want {
			t.Errorf("parseDelay(%q) = %v %v, want %v", text, got, ok, want)
		}
	}
}

// TestLineText checks what Copy Line puts on the clipboard: a bilingual cue's translation over its original, as the subtitle list shows them, and the one line of a cue without a translation, without the spaces around it.
func TestLineText(t *testing.T) {
	for _, tc := range []struct{ cue, want string }{
		{"你好。\nHello.", "你好。\nHello."},
		{"  你好。 \n Hello.  \n", "你好。\nHello."},
		{"Bye.", "Bye."},
		{" Bye.\n", "Bye."},
		{"", ""},
	} {
		if got := LineText(tc.cue); got != tc.want {
			t.Errorf("LineText(%q) = %q, want %q", tc.cue, got, tc.want)
		}
	}
}

// TestNextCueChange checks when the picture's subtitles change next: at the start or end of a line after now, whichever comes first.
func TestNextCueChange(t *testing.T) {
	cues := []core.Segment{{Start: time.Second, End: 3 * time.Second}, {Start: 4 * time.Second, End: 6 * time.Second}, {Start: 8 * time.Second}}
	for at, want := range map[time.Duration]time.Duration{0: time.Second, 2 * time.Second: time.Second, 3 * time.Second: time.Second, 5 * time.Second: time.Second, 7 * time.Second: time.Second} {
		if got, ok := NextCueChange(cues, at); !ok || got != want {
			t.Errorf("at %v the next change is %v %v, want %v", at, got, ok, want)
		}
	}
	if _, ok := NextCueChange(cues, 9*time.Second); ok {
		t.Error("a change after the last line, which has no end")
	}
}

// TestPictureFillsTheStage checks that a picture close to the stage's shape covers it, as the prototype's does, with no black bars under the title bar and the transport, and that one far from it shows whole.
func TestPictureFillsTheStage(t *testing.T) {
	for _, tc := range []struct {
		name           string
		stageW, stageH ns.Points
		videoW, videoH ns.Points
		want           ns.ResizeMode
	}{
		{"16:9 in the prototype's window", 1304, 782, 1920, 1080, ns.Cover},
		{"16:9 in a 16:10 window", 1440, 900, 1920, 1080, ns.Cover},
		{"1.85:1 in a 16:10 window", 1440, 900, 1998, 1080, ns.Cover},
		{"the same shape", 1280, 720, 1920, 1080, ns.Cover},
		{"2.39:1 in a 16:10 window", 1440, 900, 2048, 858, ns.Contain},
		{"4:3 in the prototype's window", 1304, 782, 640, 480, ns.Contain},
		{"portrait in a wide window", 1304, 782, 1080, 1920, ns.Contain},
		{"no frame yet", 1304, 782, 0, 0, ns.Cover},
	} {
		if got := PictureFit(tc.stageW, tc.stageH, tc.videoW, tc.videoH); got != tc.want {
			t.Errorf("%s: fit %v, want %v", tc.name, got, tc.want)
		}
	}
}

// TestStyleFontsArePlatformsOwn checks that the font menu offers macOS's own fonts on macOS, as the prototype has them, and a sans-serif and a serif font on Windows and Linux, where PingFang and Hiragino are not installed and would draw in the default font.
func TestStyleFontsArePlatformsOwn(t *testing.T) {
	for os, want := range map[ns.OS][]string{
		ns.OSMacOS:   {"默认", "苹方", "冬青黑体"},
		ns.OSWindows: {"默认", "黑体", "宋体"},
		ns.OSLinux:   {"默认", "黑体", "宋体"},
	} {
		if got := styleFonts(os); !slices.Equal(got, want) {
			t.Errorf("%v offers %q, want %q", os, got, want)
		}
	}
}

// TestFontFamilyHasAFontOnEverySystem checks that each saved family other than 默认 draws in a font of its kind on each system: its list names a font macOS has, one Windows has and one a Linux with Noto CJK has, so a family saved on one system does not fall back to the default font on another, and the menu's choices look apart everywhere.
func TestFontFamilyHasAFontOnEverySystem(t *testing.T) {
	systems := map[string]map[string][]string{
		"sans": {
			"darwin":  {"PingFang SC", "Hiragino Sans GB"},
			"windows": {"Microsoft YaHei"},
			"linux":   {"Noto Sans CJK SC"},
		},
		"serif": {
			"darwin":  {"Songti SC"},
			"windows": {"SimSun"},
			"linux":   {"Noto Serif CJK SC"},
		},
	}
	kinds := map[string]string{"苹方": "sans", "冬青黑体": "sans", "黑体": "sans", "宋体": "serif"}
	for family, kind := range kinds {
		list := strings.Split(fontFamily(family), ", ")
		for goos, fonts := range systems[kind] {
			if !slices.ContainsFunc(fonts, func(f string) bool { return slices.Contains(list, f) }) {
				t.Errorf("%s draws in %q, none of %s's %s fonts %q", family, list, goos, kind, fonts)
			}
		}
	}
	if got := fontFamily("苹方"); !strings.HasPrefix(got, "PingFang SC,") {
		t.Errorf("苹方 draws in %q, want PingFang SC first", got)
	}
	if got := fontFamily("宋体"); strings.Contains(got, "Sans") {
		t.Errorf("宋体 falls back to a sans-serif font: %q", got)
	}
	for _, family := range []string{"默认", "", "Comic Sans"} {
		if got := fontFamily(family); got != "" {
			t.Errorf("%q draws in %q, want the system's font", family, got)
		}
	}
}

// TestFontNamesFollowTheInterface checks that the style view names every saved family, the new ones too, and shows one it does not know as saved.
func TestFontNamesFollowTheInterface(t *testing.T) {
	m := testlocale.Messages()
	for family, want := range map[string]string{"默认": "默认", "苹方": "苹方", "冬青黑体": "冬青黑体", "黑体": "黑体", "宋体": "宋体", "Comic Sans": "Comic Sans"} {
		if got := fontNameIn(m, family); got != want {
			t.Errorf("fontName(%q) = %q, want %q", family, got, want)
		}
	}
}

// TestChaptersCoverTheTranscript cuts 100 seconds of subtitles into the least 4 片段, each named after the line that started last by its start.
func TestChaptersCoverTheTranscript(t *testing.T) {
	cues := []core.Segment{
		{Start: 0, End: 7 * time.Second, Text: "第一句\nFirst."},
		{Start: 7 * time.Second, End: 40 * time.Second, Text: "第二句\nSecond."},
		{Start: 40 * time.Second, End: 70 * time.Second, Text: "第三句\nThird."},
		{Start: 70 * time.Second, End: 100 * time.Second, Text: "第四句\nFourth."},
	}
	chapters := Chapters(cues, 0)
	if len(chapters) != 4 || chapters[0].Title != "第一句" || chapters[0].Start != 0 || chapters[3].End != 100*time.Second {
		t.Fatalf("chapters %#v", chapters)
	}
}

// TestSeekLanded checks which reports of the system's player count as where a seek landed: one at or a little past where it went, but not one sent before it from where the picture was, even when that is under a second away, as for a short looped line going back to its start; and a seek far off lands only near where it went.
func TestSeekLanded(t *testing.T) {
	ms := func(n int) time.Duration { return time.Duration(n) * time.Millisecond }
	tests := []struct {
		name                    string
		from, to, ran, position time.Duration
		want                    bool
	}{
		{"landed at the start of a short line", ms(10600), ms(10000), ms(20), ms(10010), true},
		{"landed, played on a while", ms(10300), ms(10000), ms(300), ms(10200), true},
		{"sent from the line's end before it landed", ms(10600), ms(10000), ms(20), ms(10620), false},
		{"sent from just before the end", ms(10600), ms(10000), ms(20), ms(10550), false},
		{"forward far, sent before", ms(5000), ms(60000), ms(100), ms(5050), false},
		{"forward far, landed on a key frame before", ms(5000), ms(60000), ms(100), ms(59400), true},
		{"forward far, landed", ms(5000), ms(60000), ms(100), ms(60050), true},
		{"to where it was", ms(3000), ms(3000), ms(250), ms(3200), true},
		{"held, landed", ms(8000), ms(2000), 0, ms(2000), true},
		{"held, sent before", ms(8000), ms(2000), 0, ms(8000), false},
	}
	for _, tt := range tests {
		if got := SeekLanded(tt.from, tt.to, tt.ran, tt.position); got != tt.want {
			t.Errorf("%s: SeekLanded(%v, %v, %v, %v) = %v, want %v", tt.name, tt.from, tt.to, tt.ran, tt.position, got, tt.want)
		}
	}
}

// TestTimerRunsTheLastSet sets a Timer again and again, as each move of the pointer and each report of the player do: only the function set last runs, once, and a Timer stopped runs none.
func TestTimerRunsTheLastSet(t *testing.T) {
	var timer *Timer
	app := nsgotest.Start(t, func(cx *ns.Scope) ns.Node {
		timer = NewTimer(cx)
		return ns.View{}
	})
	var ran []int
	for i := range 100 {
		timer.Set(time.Second, func() { ran = append(ran, i) })
	}
	if timer.At().IsZero() {
		t.Fatal("a timer set has no time to run at")
	}
	app.Advance(2 * time.Second)
	if !slices.Equal(ran, []int{99}) || !timer.At().IsZero() {
		t.Fatalf("ran %v, at %v", ran, timer.At())
	}
	timer.Set(time.Second, func() { ran = append(ran, -1) })
	timer.Stop()
	app.Advance(2 * time.Second)
	if !slices.Equal(ran, []int{99}) {
		t.Fatalf("a stopped timer ran: %v", ran)
	}
}
