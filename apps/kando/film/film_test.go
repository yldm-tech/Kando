package film

import (
	"fmt"
	"image"
	"math"
	"net/url"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/theme"
	"kando/packages/core"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
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
	fakeCorrector
	fakeGlossary
	fakeExplainer
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
	jobStatus  string
	yield      bool
	delay      time.Duration
	lift       float32
	room       SubRoom
	panel      bool
	styleView  bool
	modeMenu   bool
	speedMenu  bool
	rate       float64
	search     string
	focus      bool
	focused    bool
	osd        string
	lookup     Lookup
	info       Info
	upload     Upload
	uploadNote bool
	volume     float64
	muted      bool
	fills      bool
	jumps      int
	peek       int
	pauseLines bool
	loop       [2]time.Duration
	looping    bool
	saved      map[int]bool
	savedOnly  bool
	calls      []string

	// evenChapters is 导入平台章节 turned off, and chapterUses the cuts SetFilmChapterUse was asked for.
	evenChapters bool
	chapterUses  []core.ChapterUse
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
		panel: true, rate: 1, volume: 1, fills: true, peek: -1,
		video: Video{Source: "/films/talk/talk.mp4", Paused: true, Volume: 1, Rate: 1},
	}
}

func (f *fakeApp) call(s string)                                   { f.calls = append(f.calls, s) }
func (f *fakeApp) OpenKey() string                                 { return f.item.Key }
func (f *fakeApp) SetOpenKey(key string)                           { f.call("open " + key); f.item.Key = key }
func (f *fakeApp) StopPlayback()                                   { f.call("stop") }
func (f *fakeApp) Reveal(path string)                              { f.call("reveal " + path) }
func (f *fakeApp) Cancel(id string)                                { f.call("cancel " + id) }
func (f *fakeApp) RunningJob() string                              { return "" }
func (f *fakeApp) ProcessFilm()                                    { f.call("process") }
func (f *fakeApp) Remove(id string)                                { f.call("remove " + id) }
func (f *fakeApp) RetryOpen(id string)                             { f.call("retry " + id) }
func (f *fakeApp) ReprocessFilm()                                  { f.call("reprocess") }
func (f *fakeApp) TranslateFilmAgain(language string)              { f.call("translate again " + language) }
func (f *fakeApp) TargetLanguage() string                          { return "简体中文" }
func (f *fakeApp) TranslateFilmMissing()                           { f.call("translate missing") }
func (f *fakeApp) MoveOpenFilmMenu(*ns.Scope) []nsdesktop.MenuItem { return nil }
func (f *fakeApp) StageLabel(_ *ns.Scope, s core.Stage) string     { return string(s) }
func (f *fakeApp) JobStatus(*ns.Scope) string                      { return f.jobStatus }
func (f *fakeApp) RedoFailure(*ns.Scope) (string, string)          { return "", "" }
func (f *fakeApp) FillsWindow() bool                               { return f.fills }
func (f *fakeApp) Play() Player {
	if f.play == nil {
		return nil
	}
	return f.play
}
func (f *fakeApp) SetFilmSpeechLanguage(code string) {
	f.call("speech " + code)
	f.item.SpeechLanguage = code
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
func (f *fakeApp) VideoEnded()             { f.ended++ }
func (f *fakeApp) VideoFailed(err error)   { f.failed = err }
func (f *fakeApp) Subs() Subs              { return f.subs }
func (f *fakeApp) FilmSide() string        { return f.side }
func (f *fakeApp) SetFilmSide(side string) { f.side = side }
func (f *fakeApp) Seek(at time.Duration)   { f.call("seek " + at.String()); f.jumps++ }
func (f *fakeApp) Jumps() int              { return f.jumps }
func (f *fakeApp) Peek() int               { return f.peek }
func (f *fakeApp) HeldLine() int           { return -1 }
func (f *fakeApp) Skip(d time.Duration)    { f.call("skip " + d.String()) }
func (f *fakeApp) PrevLine()               { f.call("prev line") }
func (f *fakeApp) NextLine()               { f.call("next line") }
func (f *fakeApp) ReplayLine()             { f.call("replay line") }
func (f *fakeApp) ToggleLoopLine()         { f.call("loop") }
func (f *fakeApp) LoopLine() bool          { return false }
func (f *fakeApp) PauseLines() bool        { return f.pauseLines }
func (f *fakeApp) LoopSpan() (time.Duration, time.Duration, bool) {
	return f.loop[0], f.loop[1], f.looping
}
func (f *fakeApp) LoopThrough(i int)                       { f.call(fmt.Sprintf("loop through %d", i)) }
func (f *fakeApp) TogglePauseLines()                       { f.pauseLines = !f.pauseLines }
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
func (f *fakeApp) SubLift() float32                        { return f.lift }
func (f *fakeApp) SetSubLift(lift float32)                 { f.lift = lift }
func (f *fakeApp) SetSubRoom(room SubRoom)                 { f.room = room }
func (f *fakeApp) SubTip() bool                            { return false }
func (f *fakeApp) Panel() bool                             { return f.panel }
func (f *fakeApp) TogglePanel()                            { f.panel = !f.panel }
func (f *fakeApp) StyleView() bool                         { return f.styleView }
func (f *fakeApp) SetStyleView(on bool)                    { f.styleView = on }
func (f *fakeApp) ModeMenu() *bool                         { return &f.modeMenu }
func (f *fakeApp) Rate() float64                           { return f.rate }
func (f *fakeApp) SetRate(rate float64)                    { f.rate = rate }
func (f *fakeApp) SpeedMenu() *bool                        { return &f.speedMenu }
func (f *fakeApp) SpeedKeys(ns.OS) (string, string)        { return "shift+,", "shift+." }
func (f *fakeApp) Search() *string                         { return &f.search }
func (f *fakeApp) SearchFocus() *bool                      { return &f.focus }
func (f *fakeApp) SearchFocused() *bool                    { return &f.focused }
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
func (f *fakeApp) Flash(text string)                       { f.osd = text }
func (f *fakeApp) ExplainLine(i int)                       { f.call(fmt.Sprintf("explain %d", i)) }
func (f *fakeApp) LineSaved(i int) bool                    { return f.saved[i] }
func (f *fakeApp) SavedOnly() *bool                        { return &f.savedOnly }
func (f *fakeApp) Lookup() Lookup                          { return f.lookup }
func (f *fakeApp) LookUpWord(i int, word string)           { f.call(fmt.Sprintf("look up %d %s", i, word)) }
func (f *fakeApp) CloseLookup()                            { f.call("close lookup") }
func (f *fakeApp) ToggleLookupWord()                       { f.call("toggle word") }
func (f *fakeApp) ExplainLookupLine()                      { f.call("explain lookup") }
func (f *fakeApp) LookupSettings()                         { f.call("lookup settings") }
func (f *fakeApp) SavedLines() []int {
	var out []int
	for i, on := range f.saved {
		if on {
			out = append(out, i)
		}
	}
	slices.Sort(out)
	return out
}
func (f *fakeApp) ToggleSaveLine(i int) {
	f.call(fmt.Sprintf("save %d", i))
	if f.saved == nil {
		f.saved = map[int]bool{}
	}
	f.saved[i] = !f.saved[i]
}

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

// TestPictureShowsWhole checks that the system's player shows the picture whole (Contain), with the panel open or hidden, so no text at a picture's edges is cut off and its burned-in subtitles stay above the transport.
func TestPictureShowsWhole(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	app.Play("id:film-video", ns.VideoInfo{Duration: 10 * time.Second, Size: ns.Size{Width: 1920, Height: 1080}})
	for _, panel := range []bool{true, false} {
		change(app, a, func() { a.panel = panel })
		if got := app.Find("id:film-video").Video.ResizeMode; got != ns.Contain {
			t.Fatalf("panel %v: the picture fits by %v, want it whole (%v)", panel, got, ns.Contain)
		}
	}
}

// TestInfoTabCorrectsTheSpokenLanguage checks that a done film's info tab names the language heard in it in a drop-down of the languages the speech recognition can be told, one it detected that the settings do not offer among them; that naming another keeps it for the film, without building the drop-down again, and offers to transcribe the film again in it, which processes it again; and that the offer goes once the film was heard in that language. A film still being processed, or one no job has processed, has no drop-down.
func TestInfoTabCorrectsTheSpokenLanguage(t *testing.T) {
	a := newFakeApp()
	a.side, a.item.Spoken = "info", "ja"
	app := start(t, a)
	msgs := testlocale.Messages()
	if !app.Exists(whole(msgs.FilmSpokenLanguage())) || app.Find("id:film-info-speech-language").Props.Label != msgs.FilmSpokenLanguage() {
		t.Fatalf("no spoken language in the info tab: %q", app.Texts())
	}
	if !app.Exists(whole(msgs.SettingsSpeechLanguageJa())) {
		t.Fatalf("the drop-down does not show the language heard: %q", app.Texts())
	}
	if app.Exists("id:film-info-transcribe-again") {
		t.Fatal("transcribing again is offered for the language heard")
	}
	field := app.Find("id:film-info-speech-language").ID
	app.Press("id:film-info-speech-language")
	app.Press(fmt.Sprintf("id:film-info-speech-language-%d", slices.Index(media.SpeechLanguages(), "en")))
	if !slices.Equal(a.calls, []string{"speech en"}) {
		t.Fatalf("naming English called %q", a.calls)
	}
	if v := app.Find("id:film-info-speech-language"); v.ID != field {
		t.Fatal("the drop-down was built again as a language was named")
	}
	if want := msgs.FilmTranscribeAgain(msgs.SettingsSpeechLanguageEn()); !app.Exists(whole(want)) {
		t.Fatalf("no %q in %q", want, app.Texts())
	}
	app.Press("id:film-info-transcribe-again")
	if !slices.Contains(a.calls, "reprocess") {
		t.Fatalf("transcribing again called %q", a.calls)
	}
	change(app, a, func() { a.item.Spoken = "en" })
	if app.Exists("id:film-info-transcribe-again") {
		t.Fatal("transcribing again is still offered once the film was heard in the language named")
	}
	// A language whisper detected that the settings do not offer is still one to choose.
	change(app, a, func() { a.item.Spoken, a.item.SpeechLanguage = "sv", "" })
	app.Press("id:film-info-speech-language")
	if !app.Exists(fmt.Sprintf("id:film-info-speech-language-%d", len(media.SpeechLanguages()))) || !app.Exists(whole("SV")) {
		t.Fatalf("the detected language is not among the choices: %q", app.Texts())
	}
	app.ClosePopup()
	for _, item := range []Item{
		{Key: "running", JobID: "running", Stage: core.StageTranscribing, VideoPath: "/films/running/video.mp4"},
		{Key: "loose", VideoPath: "/films/loose.mp4"},
	} {
		change(app, a, func() { a.item = item })
		if app.Exists("id:film-info-speech-language") {
			t.Fatalf("film %s offers a spoken language", item.Key)
		}
	}
}

// TestPanelTabsAndTheStyleView presses the side panel's tabs and checks each shows its own: the subtitle list, the 片段, the film's files and the style view, whose preset cards set the look.
func TestPanelTabsAndTheStyleView(t *testing.T) {
	a := newFakeApp()
	a.info = Info{Folder: "/films/talk", Files: []File{{Name: "talk.bilingual.srt", Size: 2048}, {Name: "talk.mp4", Size: 5 << 20}}}
	app := start(t, a)
	msgs := testlocale.Messages()
	if !app.Exists(whole("你好。")) {
		t.Fatalf("the subtitle tab lists %q", app.Texts())
	}
	app.Press("id:film-tab-clips")
	if a.side != "clips" || !app.Exists(`label:"`+clipName(a, 1)+`"`) {
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
	// The film's languages: the one it was translated into, and the one heard in it once that is known.
	if !app.Exists(whole(msgs.SettingsLanguageSimplifiedChinese())) {
		t.Fatalf("the info tab does not name the translation's language: %q", app.Texts())
	}
	a.item.Spoken = "ja"
	a.Changed()
	app.Settle()
	if want := msgs.FilmInfoLanguages(msgs.SettingsSpeechLanguageJa(), msgs.SettingsLanguageSimplifiedChinese()); !app.Exists(whole(want)) {
		t.Fatalf("the info tab does not say %q: %q", want, app.Texts())
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

// TestCueMenuExplainsTheLine checks that a right click on a line of the subtitle list offers Explain Line first, which asks the app to explain that line by its index, and still offers to copy it.
func TestCueMenuExplainsTheLine(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	msgs := testlocale.Messages()
	app.RightClick(inList("Bye."))
	var labels []string
	for _, item := range app.PopupMenu {
		labels = append(labels, item.Label)
	}
	if len(labels) == 0 || labels[0] != msgs.FilmExplainLine() || !slices.Contains(labels, msgs.FilmCopyLine()) {
		t.Fatalf("the line's menu is %q", labels)
	}
	app.ChoosePopup(msgs.FilmExplainLine())
	if !slices.Contains(a.calls, "explain 1") {
		t.Fatalf("calls %q", a.calls)
	}
}

// TestCueListLetsTheUserReadAway scrolls the subtitle list away from the playing line by hand: as the film moves on to the next line the list stays where it was scrolled to, and a pill offers to go back to the line playing, which brings the list back to it and goes. A line clicked plays and the list follows from it again, and so does a line reached by a move of the user's, as A, D or R, while one scrolled out of view by playing on does not.
func TestCueListLetsTheUserReadAway(t *testing.T) {
	a := newFakeApp()
	a.subs.Cues = nil
	for i := range 200 {
		at := time.Duration(i) * 2 * time.Second
		a.subs.Cues = append(a.subs.Cues, core.Segment{Start: at, End: at + time.Second, Text: fmt.Sprintf("第 %d 句", i+1)})
	}
	a.play = &fakePlay{head: 500 * time.Millisecond, length: 400 * time.Second}
	app := start(t, a)
	msgs := testlocale.Messages()
	offset := func() ns.Points { app.Advance(time.Second); return app.Find("id:film-cues").Offset.Y }
	pill := `label:"` + msgs.FilmBackToCurrentLine() + `"`
	playAt := func(head time.Duration) {
		t.Helper()
		change(app, a, func() { a.play.head = head })
	}
	first := offset()
	playAt(2500 * time.Millisecond)
	if got := offset(); got <= first || app.Exists(pill) {
		t.Fatalf("the list did not follow the second line: offset %v from %v, pill %v", got, first, app.Exists(pill))
	}
	app.Scroll("id:film-cues", 2000)
	away := offset()
	playAt(4500 * time.Millisecond)
	if got := offset(); got != away || !app.Exists(pill) {
		t.Fatalf("the list read away at %v moved to %v as the film went on; pill %v", away, got, app.Exists(pill))
	}
	app.Press(pill)
	back := offset()
	if back >= away || app.Exists(pill) {
		t.Fatalf("the pill took the list to %v from %v, and it stays %v", back, away, app.Exists(pill))
	}
	app.Scroll("id:film-cues", 2000)
	away = offset()
	app.Press(whole("第 60 句"))
	playAt(118500 * time.Millisecond)
	if got := offset(); got == away || app.Exists(pill) {
		t.Fatalf("a clicked line left the list at %v; pill %v", got, app.Exists(pill))
	}
	app.Scroll("id:film-cues", -1000)
	away = offset()
	change(app, a, func() { a.jumps++; a.play.head = 120500 * time.Millisecond })
	if got := offset(); got == away || app.Exists(pill) {
		t.Fatalf("a move of the user's left the list at %v; pill %v", got, app.Exists(pill))
	}
}

// TestCueRowsFollowTheMode lists a bilingual line in each subtitle mode: the list shows the lines the picture shows, in the order the settings put them, and the line the mode hides stays under them blurred until the pointer rests on the row or the keyboard focus comes to it, so the list can be read as a transcript without giving the answer away. The row is named by both lines in their order, and the playing one is selected.
func TestCueRowsFollowTheMode(t *testing.T) {
	a := newFakeApp()
	a.mode = SubOriginal
	a.play = &fakePlay{head: 1500 * time.Millisecond, length: 10 * time.Second}
	app := start(t, a)
	// The panel slides in from the right on its first frames; the pointer reaches its rows once it has.
	app.Advance(panelMotion + 100*time.Millisecond)
	blurred := func(text string) bool {
		e := app.Find(inList(text)).Style.Effects
		return e != nil && len(e.Filters) > 0
	}
	row := app.Find(`label:"0:01 你好。 Hello."`)
	if !row.Props.Selected || app.Find(`label:"0:04 Bye."`).Props.Selected {
		t.Fatalf("the playing row is selected %v", row.Props.Selected)
	}
	if blurred("Hello.") || !blurred("你好。") {
		t.Fatalf("in Original Only the original is blurred %v and the translation %v", blurred("Hello."), blurred("你好。"))
	}
	if hello, nihao := app.Find(inList("Hello.")), app.Find(inList("你好。")); pageY(hello) >= pageY(nihao) {
		t.Fatal("the original is not over the translation it hides")
	}
	app.Hover(inList("Hello."))
	if blurred("你好。") {
		t.Fatal("the translation stays blurred under the pointer")
	}
	app.HoverOut()
	if !blurred("你好。") {
		t.Fatal("the translation stays clear once the pointer left")
	}
	change(app, a, func() { a.mode = SubTranslation })
	if blurred("你好。") || !blurred("Hello.") {
		t.Fatalf("in Translation Only the translation is blurred %v and the original %v", blurred("你好。"), blurred("Hello."))
	}
	change(app, a, func() { a.mode, a.order = SubBilingual, SubOriginalFirst })
	if blurred("你好。") || blurred("Hello.") || pageY(app.Find(inList("Hello."))) >= pageY(app.Find(inList("你好。"))) {
		t.Fatal("bilingual, the original first, is not both lines clear with the original on top")
	}
	if !app.Exists(`label:"0:01 Hello. 你好。"`) {
		t.Fatal("the row is not named by its lines in their order")
	}
}

// inList is the selector of a text of the subtitle list that is s as a whole.
func inList(s string) string { return "id:film-cues >> " + whole(s) }

// pageY is how far down the window v is.
func pageY(v *nsgotest.View) ns.Points {
	y := ns.Points(0)
	for ; v != nil; v = v.Parent {
		y += v.Frame.Origin.Y - v.Offset.Y
	}
	return y
}

// TestPeekShowsTheWholeLine checks the line the app peeks at (T): the picture shows both its lines, whatever the mode, the subtitles off as well, and only it.
func TestPeekShowsTheWholeLine(t *testing.T) {
	a := newFakeApp()
	a.mode = SubOriginal
	app := start(t, a)
	app.Play("id:film-video", ns.VideoInfo{Duration: 10 * time.Second})
	app.Progress("id:film-video", 1500*time.Millisecond, 10*time.Second)
	onPicture := func(text string) bool {
		if !app.Exists("id:film-subtitle-lines") {
			return false
		}
		var has func(v *nsgotest.View) bool
		has = func(v *nsgotest.View) bool {
			return v.Text == text || slices.ContainsFunc(v.Children, has)
		}
		return has(app.Find("id:film-subtitle-lines"))
	}
	if !onPicture("Hello.") || onPicture("你好。") {
		t.Fatal("Original Only does not show the original alone")
	}
	change(app, a, func() { a.peek = 0 })
	if !onPicture("Hello.") || !onPicture("你好。") {
		t.Fatal("the line peeked at does not show both its lines")
	}
	change(app, a, func() { a.mode = SubOff })
	if !onPicture("你好。") {
		t.Fatal("the line peeked at hides with the subtitles off")
	}
	change(app, a, func() { a.peek = 1 })
	if app.Exists("id:film-subtitle-lines") {
		t.Fatal("a peek at another line shows this one with the subtitles off")
	}
}

// TestLoopSpanShows loops the first two lines: the scrubber draws the span played over and over, the subtitle delay counted, and the looped lines of the list are marked by a bar at their start, the others not.
func TestLoopSpanShows(t *testing.T) {
	a := newFakeApp()
	a.subs.Cues = append(a.subs.Cues, core.Segment{Start: 8 * time.Second, End: 9 * time.Second, Text: "再来。\nAgain."})
	a.play = &fakePlay{head: 1500 * time.Millisecond, length: 10 * time.Second}
	app := start(t, a)
	band := func() bool {
		for _, op := range app.Find("id:film-scrub-marks").Ops {
			if op.Color == scrubLoop {
				return true
			}
		}
		return false
	}
	if band() || app.Exists("id:film-cue-looped") {
		t.Fatal("the loop shows without a loop")
	}
	change(app, a, func() { a.loop, a.looping = [2]time.Duration{time.Second, 6 * time.Second}, true })
	if !band() || len(app.Query("id:film-cue-looped")) != 2 {
		t.Fatalf("looping two lines: band %v, %d rows marked", band(), len(app.Query("id:film-cue-looped")))
	}
}

// TestScrubberMarksFollowTheDelay checks that the scrubber's strip of cues sits where the picture plays them, the subtitle delay counted, as the loop band and the hover card do, and keeps on the track for a line the delay pushes past the video's start.
func TestScrubberMarksFollowTheDelay(t *testing.T) {
	a := newFakeApp()
	a.play = &fakePlay{head: 0, length: 10 * time.Second}
	app := start(t, a)
	// strip is where each mark of the cues' strip starts and ends, by its color.
	strip := func() [][2]float32 {
		var out [][2]float32
		var fill ns.Color
		for _, op := range app.Find("id:film-scrub-marks").Ops {
			switch {
			case op.Color != 0:
				fill = op.Color
			case fill == scrubTranslated && op.Y == 15 && op.W > 0:
				out = append(out, [2]float32{op.X, op.X + op.W})
			}
		}
		return out
	}
	w := float32(app.Find("id:film-scrub-marks").Frame.Size.Width)
	near := func(got [][2]float32, want [2]float32) bool {
		return len(got) == 1 && math.Abs(float64(got[0][0]-want[0])) < 0.5 && math.Abs(float64(got[0][1]-want[1])) < 0.5
	}
	if got := strip(); !near(got, [2]float32{w * 0.1, w * 0.3}) {
		t.Fatalf("without a delay the first line is marked at %v of %v", got, w)
	}
	change(app, a, func() { a.delay = 2 * time.Second })
	if got := strip(); !near(got, [2]float32{w * 0.3, w * 0.5}) {
		t.Fatalf("2 seconds late the first line is marked at %v of %v", got, w)
	}
	change(app, a, func() { a.delay = -2 * time.Second })
	if got := strip(); !near(got, [2]float32{0, w * 0.1}) {
		t.Fatalf("2 seconds early the first line is marked at %v of %v", got, w)
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
	if want := []string{msgs.FilmExplainLine(), msgs.SavedKeySaveLine(), "", msgs.FilmCopyLine(), msgs.FilmCopyOriginal(), msgs.FilmCopyTranslation()}; !slices.Equal(labels, want) {
		t.Fatalf("the bilingual line's menu is %q, want %q", labels, want)
	}
	app.ChoosePopup(msgs.FilmCopyOriginal())
	if app.Clipboard != "Hello." {
		t.Fatalf("拷贝原文 copied %q", app.Clipboard)
	}
	app.RightClick(whole("Bye."))
	if len(app.PopupMenu) != 4 {
		t.Fatalf("the line without a translation offers %d items, want Explain Line, Save Line, a separator and Copy Line", len(app.PopupMenu))
	}
	app.ChoosePopup(msgs.FilmCopyLine())
	if app.Clipboard != "Bye." {
		t.Fatalf("拷贝台词 copied %q", app.Clipboard)
	}
}

// searchCues are lines where "hello" is said three times, at 1, 10 and 30 seconds.
var searchCues = []core.Segment{
	{Start: time.Second, End: 2 * time.Second, Text: "你好。\nHello."},
	{Start: 5 * time.Second, End: 6 * time.Second, Text: "再见。\nBye."},
	{Start: 10 * time.Second, End: 11 * time.Second, Text: "你好吗？\nHello again?"},
	{Start: 30 * time.Second, End: 31 * time.Second, Text: "又是你好。\nHello once more."},
}

// TestSearchReturnJumpsToTheNextMatch types a word in the subtitle search and presses Return: the film plays from the next line that holds it after where the picture is, the delay counted, and the picture says which match of how many it is; Return again goes on to the one after, around to the first after the last, and Shift+Return goes back. The field keeps the focus, so the keys go on working. The chip over the list counts the matches of all the lines.
func TestSearchReturnJumpsToTheNextMatch(t *testing.T) {
	a := newFakeApp()
	a.subs.Cues = searchCues
	a.delay = 500 * time.Millisecond
	a.play = &fakePlay{head: 4 * time.Second, length: time.Minute}
	app := start(t, a)
	msgs := testlocale.Messages()
	if !app.Exists(whole(msgs.FilmAll())) || !app.Exists(whole("4")) {
		t.Fatalf("without a query the chip reads %q", app.Texts())
	}
	app.Type("id:film-search", "hello")
	if app.Exists(whole(msgs.FilmAll())) || !app.Exists(whole("3 / 4")) {
		t.Fatalf("with a query the chip reads %q", app.Texts())
	}
	step := func(back bool, want time.Duration, match int) {
		t.Helper()
		a.calls = nil
		if back {
			app.Key("shift+enter")
		} else {
			app.Submit("id:film-search")
		}
		if !slices.Equal(a.calls, []string{"seek " + want.String()}) || a.osd != msgs.FilmMatchOf(match, 3) {
			t.Fatalf("back %v: called %q, osd %q; want seek %v, match %d", back, a.calls, a.osd, want, match)
		}
		a.play.head = want
		a.Changed()
		app.Settle()
	}
	step(false, 10500*time.Millisecond, 2)
	step(false, 30500*time.Millisecond, 3)
	step(false, 1500*time.Millisecond, 1)
	step(true, 30500*time.Millisecond, 3)
	step(true, 10500*time.Millisecond, 2)
	if !a.focused {
		t.Fatal("the search field let go of the focus")
	}
}

// TestSubtitleTabHasNoFooterHint checks the subtitle tab ends with its lines: no hint under them that a click jumps to a line, which every row's hover already shows.
func TestSubtitleTabHasNoFooterHint(t *testing.T) {
	app := start(t, newFakeApp())
	if app.Exists(whole("单击跳到该句")) {
		t.Fatalf("the subtitle tab still has the footer hint: %q", app.Texts())
	}
}

// TestPanelRowsNameWhatTheyShow checks what assistive technology reads of the panel's rows: a line its time, its translation and its original, and that it is the one playing; a clip its number, its span and the line it is named after; a file's 显示 button the file it shows.
func TestPanelRowsNameWhatTheyShow(t *testing.T) {
	a := newFakeApp()
	a.play = &fakePlay{head: 2 * time.Second, length: 10 * time.Second}
	a.info = Info{Folder: "/films/talk", Files: []File{{Name: "talk.mp4", Size: 5 << 20}}}
	app := start(t, a)
	msgs := testlocale.Messages()
	first, second := app.Find("0:01 你好。 Hello."), app.Find("0:04 Bye.")
	if !first.Props.Selected || second.Props.Selected {
		t.Fatalf("the line playing is not the one selected: %v %v", first.Props.Selected, second.Props.Selected)
	}
	app.Press("id:film-tab-clips")
	ch := Chapters(a.subs.Cues, a.play.length)[0]
	if !app.Exists(msgs.FilmClip(1, cueClock(ch.Start), cueClock(ch.End), ch.Title)) {
		t.Fatalf("no clip named %q\n%s", msgs.FilmClip(1, cueClock(ch.Start), cueClock(ch.End), ch.Title), app.Dump())
	}
	app.Press("id:film-tab-info")
	if got := app.Find("id:film-info-file-talk.mp4").Props.Label; got != msgs.FilmRevealFile("talk.mp4") {
		t.Fatalf("the file's button is named %q", got)
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

// TestModeMenuNoteFollowsUntranslated opens the subtitle menu: the line under Translation Only says what the picture shows for a line not yet translated, its original dimmed or nothing, as the setting picks, and follows the setting while the menu is open.
func TestModeMenuNoteFollowsUntranslated(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	msgs := testlocale.Messages()
	app.Press("id:film-mode")
	if !app.Exists(whole(msgs.FilmModeTranslationNote())) || app.Exists(whole(msgs.FilmModeTranslationBlankNote())) {
		t.Fatalf("with untranslated lines dimmed the menu reads %q", app.Texts())
	}
	change(app, a, func() { a.untrans = UntranslatedBlank })
	if app.Exists(whole(msgs.FilmModeTranslationNote())) || !app.Exists(whole(msgs.FilmModeTranslationBlankNote())) {
		t.Fatalf("with untranslated lines left blank the menu reads %q", app.Texts())
	}
}

// TestCueClockFitsAnHourLongFilm lists the lines of a film over an hour long: a line's start, as 1:02:03.0, stays on one line in a column wide enough for it, wider than the column of a short film.
func TestCueClockFitsAnHourLongFilm(t *testing.T) {
	short := start(t, newFakeApp())
	narrow := short.Find(whole("0:01.0")).Frame.Size.Width
	a := newFakeApp()
	a.play = &fakePlay{length: 65 * time.Minute}
	a.subs.Cues = append(a.subs.Cues, core.Segment{Start: time.Hour + 2*time.Minute + 3*time.Second, End: time.Hour + 2*time.Minute + 5*time.Second, Text: "再见。\nSee you."})
	app := start(t, a)
	for _, text := range []string{"0:01.0", "1:02:03.0"} {
		clock := app.Find(whole(text))
		if size := clock.Frame.Size; size.Height > 15 || size.Width <= narrow {
			t.Fatalf("%s is %vx%v, the column of a short film %v wide", text, size.Width, size.Height, narrow)
		}
	}
}

// TestClipPlaysFromItsLineWithTheDelay presses a 片段 with the subtitles moved 2 seconds later: it plays from its line where the picture shows it, 2 seconds after the line's start.
func TestClipPlaysFromItsLineWithTheDelay(t *testing.T) {
	a := newFakeApp()
	a.delay = 2 * time.Second
	a.play = &fakePlay{length: 120 * time.Second}
	a.subs.Cues = chapterCues
	app := start(t, a)
	app.Press("id:film-tab-clips")
	app.Press(`label:"` + clipName(a, 2) + `"`)
	if !slices.Contains(a.calls, "seek 37s") {
		t.Fatalf("the second 片段 played with %v, want seek 37s", a.calls)
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

// TestSpeedMenuIsGoneOnLinux shows the transport on Linux, where the system's player plays every film at the speed it was recorded at: it has no speed button, which would name a speed the film does not play at, and keeps the rest.
func TestSpeedMenuIsGoneOnLinux(t *testing.T) {
	a := newFakeApp()
	a.rate = 2
	app := start(t, a)
	app.SetEnvironment(testlocale.Environment(ns.Environment{OS: ns.OSLinux, Idiom: ns.IdiomDesktop}))
	if app.Exists("id:film-speed") || !app.Exists("id:film-mode") {
		t.Fatalf("the transport on Linux: speed %v, mode %v", app.Exists("id:film-speed"), app.Exists("id:film-mode"))
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

// A film watched while its job runs says beside 取消 what the job is doing, as its card does, and stops saying it once the job is done.
func TestTheTitleBarFollowsTheFilmsJob(t *testing.T) {
	a := newFakeApp()
	a.item.Stage, a.jobStatus = core.StageTranslating, "翻译 · 120/480 句"
	app := start(t, a)
	if !app.Exists(whole(a.jobStatus)) || !app.Exists("id:film-cancel") {
		t.Fatalf("no job status: %q", app.Texts())
	}
	change(app, a, func() { a.jobStatus = "翻译 · 240/480 句" })
	if !app.Exists(whole(a.jobStatus)) {
		t.Fatalf("the status did not follow the job: %q", app.Texts())
	}
	change(app, a, func() { a.item.Stage, a.jobStatus = core.StageComplete, "" })
	if app.Exists("id:film-job-status") || app.Exists("id:film-cancel") {
		t.Fatal("a done film still shows its job")
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

// TestPictureLinesOfAFilmInItsOwnLanguage checks that a film heard in the language its subtitles are for, whose cues are its own lines alone, shows them in 仅译文 and 双语 as 原文 does, never dimmed as lines not translated yet, and still nothing in 关闭.
func TestPictureLinesOfAFilmInItsOwnLanguage(t *testing.T) {
	a := newFakeApp()
	for _, mode := range []string{SubTranslation, SubBilingual, SubOriginal} {
		a.mode = mode
		if got := pictureLines(ownLanguage{a}, "今天讲神经网络"); len(got) != 1 || got[0].text != "今天讲神经网络" || got[0].dim {
			t.Fatalf("%s: %+v", mode, got)
		}
	}
	a.mode = SubOff
	if got := (ownLanguage{a}).SubMode(); got != SubOff {
		t.Fatalf("off is %s", got)
	}
}

// TestSubtitleOutlineFollowsTheGlyphs checks that an outlined line is drawn again in black under itself, ten copies after the prototype's text-shadow, which follow the glyphs as a box's shadow would not, and that only the line itself is read by assistive technology.
func TestSubtitleOutlineFollowsTheGlyphs(t *testing.T) {
	line := styledLine{text: "每条河都始于高山", face: Face{Size: 30, Color: "#FFFFFF", Bold: true}}
	for _, outline := range []bool{false, true} {
		app := nsgotest.Start(t, func(cx *ns.Scope) ns.Node {
			// On a white page, so the white line shows nothing dark of its own, inset so the ring outside its box is on the page.
			return ns.View{TestID: "line", Style: ns.Style{FlexGrow: 1, Padding: ns.Symmetric(30, 20), Background: ns.RGBA(0xFFFFFF, 1)}, Children: []ns.Node{subtitleLine(func() string { return line.text }, line, false, outline, nil)}}
		}, nsgotest.Size(400, 120))
		copies := app.Query(whole(line.text))
		want := 1
		if outline {
			want = 1 + len(outlineCopies) + 1
		}
		if len(copies) != want {
			t.Fatalf("outline %v draws the line %d times, want %d", outline, len(copies), want)
		}
		// Headless runs have no pixels (nsgotest's Capture skips them), so the picture is checked on a platform only, under nsgo test -target.
		if nsgotest.Target() != "" {
			checkOutlinePixels(t, app.Capture(), absoluteFrame(copies[0]), outline)
		}
	}
}

// checkOutlinePixels checks the captured window (400 points wide) around the line's box: the white line on the white page shows nothing dark without its outline; with it, the box has dark pixels along the glyphs but is not dark as a whole, as a box's shadow would make it, and a ring 8 to 12 points outside the box, past the glow's 3-point blur, has none.
func checkOutlinePixels(t *testing.T, img image.Image, box ns.Bounds, outline bool) {
	t.Helper()
	scale := float64(img.Bounds().Dx()) / 400
	px := func(p ns.Points) int { return int(float64(p) * scale) }
	dark := func(x, y int) bool {
		r, g, b, _ := img.At(img.Bounds().Min.X+x, img.Bounds().Min.Y+y).RGBA()
		return (r+g+b)/3 < 0x4000
	}
	x0, y0 := px(box.Origin.X), px(box.Origin.Y)
	x1, y1 := px(box.Origin.X+box.Size.Width), px(box.Origin.Y+box.Size.Height)
	inside, darkInside := 0, 0
	for y := y0; y < y1; y++ {
		for x := x0; x < x1; x++ {
			inside++
			if dark(x, y) {
				darkInside++
			}
		}
	}
	darkRing := 0
	near, far := px(8), px(12)
	for y := y0 - far; y < y1+far; y++ {
		for x := x0 - far; x < x1+far; x++ {
			if x >= x0-near && x < x1+near && y >= y0-near && y < y1+near {
				continue
			}
			if x >= 0 && y >= 0 && x < img.Bounds().Dx() && y < img.Bounds().Dy() && dark(x, y) {
				darkRing++
			}
		}
	}
	share := float64(darkInside) / float64(max(inside, 1))
	switch {
	case !outline && darkInside > 0:
		t.Fatalf("the line without an outline has %d dark pixels in its box %v", darkInside, box)
	case outline && share < 0.01:
		t.Fatalf("the outlined line's box %v is %.1f%% dark, want its outline along the glyphs", box, share*100)
	case outline && share > 0.6:
		t.Fatalf("the outlined line's box %v is %.1f%% dark, as a box's shadow, not one along the glyphs", box, share*100)
	case darkRing > 0:
		t.Fatalf("outline %v leaves %d dark pixels 8 to 12 points outside the line's box %v", outline, darkRing, box)
	}
}

// absoluteFrame is v's frame in the window's points, as its ancestors and their scroll offsets place it.
func absoluteFrame(v *nsgotest.View) ns.Bounds {
	b := v.Frame
	for p := v.Parent; p != nil; p = p.Parent {
		b.Origin.X += p.Frame.Origin.X - p.Offset.X
		b.Origin.Y += p.Frame.Origin.Y - p.Offset.Y
	}
	return b
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

// TestChaptersCoverTheTranscript cuts 100 seconds of subtitles into the least 4 片段 by time: the cut at 25 seconds falls inside 第二句 and moves to 第三句's start at 40, the cuts at 50 and 75 to 第四句's at 70, so three 片段 cover the subtitles end to end, each named after the line it starts with.
func TestChaptersCoverTheTranscript(t *testing.T) {
	cues := []core.Segment{
		{Start: 0, End: 7 * time.Second, Text: "第一句\nFirst."},
		{Start: 7 * time.Second, End: 40 * time.Second, Text: "第二句\nSecond."},
		{Start: 40 * time.Second, End: 70 * time.Second, Text: "第三句\nThird."},
		{Start: 70 * time.Second, End: 100 * time.Second, Text: "第四句\nFourth."},
	}
	want := []Chapter{
		{Start: 0, End: 40 * time.Second, Title: "第一句", Detail: "First."},
		{Start: 40 * time.Second, End: 70 * time.Second, Title: "第三句", Detail: "Third."},
		{Start: 70 * time.Second, End: 100 * time.Second, Title: "第四句", Detail: "Fourth."},
	}
	if chapters := Chapters(cues, 0); !slices.Equal(chapters, want) {
		t.Fatalf("chapters %#v", chapters)
	}
}

// chapterCues are two minutes of lines whose even cuts at 30, 60 and 90 seconds fall inside a line.
var chapterCues = []core.Segment{
	{Start: 0, End: 10 * time.Second, Text: "一\nOne."},
	{Start: 10 * time.Second, End: 35 * time.Second, Text: "二\nTwo."},
	{Start: 35 * time.Second, End: 62 * time.Second, Text: "三\nThree."},
	{Start: 62 * time.Second, End: 95 * time.Second, Text: "四\nFour."},
	{Start: 95 * time.Second, End: 120 * time.Second, Text: "五\nFive."},
}

// TestChaptersStartOnALine cuts films into 片段 that start and end where a line starts, never inside one, each named after the line it starts with: a cut inside a line moves to the next line's start, cuts that move to the same line make one 片段, and a cut after the last line's start goes.
func TestChaptersStartOnALine(t *testing.T) {
	type span struct {
		from, to time.Duration
		title    string
	}
	s := time.Second
	tests := []struct {
		name   string
		cues   []core.Segment
		length time.Duration
		want   []span
	}{
		{"cuts inside lines", chapterCues, 120 * s, []span{{0, 35 * s, "一"}, {35 * s, 62 * s, "三"}, {62 * s, 95 * s, "四"}, {95 * s, 120 * s, "五"}}},
		{"a long gap", []core.Segment{{Start: 0, End: 5 * s, Text: "一"}, {Start: 100 * s, End: 110 * s, Text: "二"}}, 120 * s, []span{{0, 100 * s, "一"}, {100 * s, 120 * s, "二"}}},
		{"lines only at the start", []core.Segment{{Start: 0, End: 5 * s, Text: "一"}, {Start: 6 * s, End: 10 * s, Text: "二"}}, 120 * s, []span{{0, 120 * s, "一"}}},
	}
	for _, tt := range tests {
		var got []span
		for _, ch := range Chapters(tt.cues, tt.length) {
			got = append(got, span{ch.Start, ch.End, ch.Title})
		}
		if !slices.Equal(got, tt.want) {
			t.Errorf("%s: chapters %v, want %v", tt.name, got, tt.want)
		}
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

// clipName is what the 片段 tab names clip n (from 1) of a's film, as clipsTab does: its span and, when it has one, the line it is named after.
func clipName(a *fakeApp, n int) string {
	ch := Chapters(a.subs.Cues, a.PlayLength())[n-1]
	m := testlocale.Messages()
	if ch.Title == "" {
		return m.FilmClipSpan(n, cueClock(ch.Start), cueClock(ch.End))
	}
	return m.FilmClip(n, cueClock(ch.Start), cueClock(ch.End), ch.Title)
}

// TestDeadEndsOfferTheirRemedy checks the notes the player shows in place of what it cannot show: subtitles that did not load in a done film's job offer Process Entire Film Again, in the subtitle list and in the clips alike, which runs the film's job again; a video no job processed has Start Processing in the title bar instead and gets no second button; and a video that would not play offers to show it in Finder.
func TestDeadEndsOfferTheirRemedy(t *testing.T) {
	msgs := testlocale.Messages()
	a := newFakeApp()
	a.subs = Subs{Key: "talk", Ready: true, Err: "subtitle-missing"}
	app := start(t, a)
	if !app.Exists(whole("error subtitle-missing")) {
		t.Fatalf("no note: %q", app.Texts())
	}
	app.Press("id:film-subs-reprocess")
	if !slices.Contains(a.calls, "retry job") || app.Find("id:film-subs-reprocess").Props.Label != msgs.FilmReprocess() {
		t.Fatalf("calls %q", a.calls)
	}
	app.Press("id:film-tab-clips")
	app.Press("id:film-subs-reprocess")
	if n := len(slices.DeleteFunc(slices.Clone(a.calls), func(c string) bool { return c != "retry job" })); n != 2 {
		t.Fatalf("the clips tab's button ran the job %d times in all, calls %q", n, a.calls)
	}

	b := newFakeApp()
	b.item.JobID, b.item.Stage = "", ""
	b.subs = Subs{Key: "talk", Ready: true, Err: "subtitle-unreadable"}
	app = start(t, b)
	if !app.Exists(whole("error subtitle-unreadable")) || app.Exists("id:film-subs-reprocess") {
		t.Fatalf("a video no job processed: %q", app.Texts())
	}

	c := newFakeApp()
	c.err = "video-unplayable"
	c.video.Source = ""
	app = start(t, c)
	if !app.Exists(whole("error video-unplayable")) {
		t.Fatalf("no note: %q", app.Texts())
	}
	app.Press("id:film-stage-reveal")
	if !slices.Contains(c.calls, "reveal /films/talk/talk.mp4") || app.Find("id:film-stage-reveal").Props.Label != msgs.FilmShowInFinder("macOS") {
		t.Fatalf("calls %q", c.calls)
	}
}

// TestCueMenuSavesTheLine checks that a right click on a line offers Save Line, which asks the app to save that line by its index, and that a saved line's row shows a star and is named saved, and its menu offers to remove it instead.
func TestCueMenuSavesTheLine(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	msgs := testlocale.Messages()
	if app.Exists("id:film-cue-saved") {
		t.Fatal("a star shows before any line was saved")
	}
	app.RightClick(inList("Bye."))
	app.ChoosePopup(msgs.SavedKeySaveLine())
	if !slices.Contains(a.calls, "save 1") {
		t.Fatalf("calls %q", a.calls)
	}
	change(app, a, func() {})
	if !app.Exists("id:film-cues >> id:film-cue-saved") {
		t.Fatalf("the saved line shows no star: %q", app.Texts())
	}
	if !app.Exists("role:button 0:04 Bye., " + msgs.SavedMark()) {
		t.Fatal("the saved row is not named saved")
	}
	app.RightClick(inList("Bye."))
	app.ChoosePopup(msgs.SavedRemoveLine())
	if !slices.Equal(a.calls[len(a.calls)-2:], []string{"save 1", "save 1"}) || a.saved[1] {
		t.Fatalf("calls %q, saved %v", a.calls, a.saved)
	}
}

// TestSavedChipShowsTheSavedLines checks that the Saved chip shows once a line is saved, with how many, and that picking it narrows the list to the saved lines, the search narrowing them further, and All shows every line again; with no saved line left the list says so.
func TestSavedChipShowsTheSavedLines(t *testing.T) {
	a := newFakeApp()
	app := start(t, a)
	msgs := testlocale.Messages()
	if app.Exists("id:film-subs-saved") {
		t.Fatal("the Saved chip shows before a line is saved")
	}
	change(app, a, func() { a.saved = map[int]bool{1: true} })
	if !app.Exists("role:button " + msgs.SavedMark() + " 1") {
		t.Fatalf("no Saved chip counting one line: %q", app.Texts())
	}
	app.Press("id:film-subs-saved")
	if !a.savedOnly || app.Exists(inList("你好。")) || !app.Exists(inList("Bye.")) || !app.Find("id:film-subs-saved").Props.Selected || app.Find("id:film-subs-count").Props.Selected {
		t.Fatalf("Saved shows %q", app.Texts())
	}
	app.Type("id:film-search", "zzz")
	if !app.Exists(whole("0 / 1")) || !app.Exists(whole(msgs.FilmNoMatch())) {
		t.Fatalf("a search among the saved lines: %q", app.Texts())
	}
	change(app, a, func() { a.search, a.saved = "", nil })
	if !app.Exists(whole(msgs.SavedNoneInFilm())) {
		t.Fatalf("no note without saved lines: %q", app.Texts())
	}
	app.Press("id:film-subs-count")
	if a.savedOnly || !app.Exists(inList("你好。")) || app.Exists("id:film-subs-saved") {
		t.Fatalf("All shows %q", app.Texts())
	}
}

// TestHeldLineCanBeSelected checks that while the film is held on a line, the subtitle list shows that line in a card whose text can be selected, a word or the whole line, with the line the mode hides blurred, and an Explain Line that explains that line; it goes as the film plays.
func TestHeldLineCanBeSelected(t *testing.T) {
	a := newFakeApp()
	a.mode = SubOriginal
	a.play = &fakePlay{head: 1500 * time.Millisecond, length: 10 * time.Second}
	app := start(t, a)
	msgs := testlocale.Messages()
	card := "id:film-held-line"
	if !app.Exists(card+" >> "+whole("Hello.")) || !app.Exists(card+" >> "+whole("你好。")) {
		t.Fatalf("the held line: %q", app.Texts())
	}
	if !app.Find("id:film-held-text").Props.SelectionArea {
		t.Fatal("the held line's text cannot be selected")
	}
	if e := app.Find(card + " >> " + whole("你好。")).Style.Effects; e == nil || len(e.Filters) == 0 {
		t.Fatal("the translation Original Only hides shows unblurred")
	}
	app.Press("id:film-held-explain")
	if !slices.Contains(a.calls, "explain 0") {
		t.Fatalf("calls %q", a.calls)
	}
	if save := app.Find("id:film-held-save"); save.Props.Label != msgs.SavedKeySaveLine() || save.Props.Selected {
		t.Fatalf("the held line's save button reads %q, selected %v", save.Props.Label, save.Props.Selected)
	}
	app.Press("id:film-held-save")
	if save := app.Find("id:film-held-save"); !slices.Contains(a.calls, "save 0") || save.Props.Label != msgs.SavedMark() || !save.Props.Selected {
		t.Fatalf("calls %q, the button reads %q", a.calls, save.Props.Label)
	}
	change(app, a, func() { a.play.playing = true })
	if app.Exists(card) {
		t.Fatal("the card stayed while the film plays")
	}
	change(app, a, func() { a.play.playing, a.play.head = false, 3500*time.Millisecond })
	if app.Exists(card) {
		t.Fatal("a card shows between lines")
	}
}

// TestInfoTabSourceLinkReadsOnTheDarkPanel checks that the link to a film's source in the info tab is in the theme's accent ink for the dark player, which reads at its size on the dark panel where the accent's own blue does not.
func TestInfoTabSourceLinkReadsOnTheDarkPanel(t *testing.T) {
	a := newFakeApp()
	a.side, a.item.Source = "info", "https://youtu.be/talk"
	var k theme.Colors
	app := nsgotest.Start(t, func(cx *ns.Scope) ns.Node {
		a.Attach(cx)
		theme.Apply(cx)
		return ns.View{Style: ns.Style{FlexGrow: 1, FlexShrink: 1, AlignItems: ns.AlignStretch}, Children: []ns.Node{ns.Func(func(cx *ns.Scope) ns.Node {
			theme.Dark(cx)
			k = theme.Of(cx)
			return View{App: a, Item: live.Func(a, func() Item { return a.item })}
		})}}
	}, nsgotest.Size(1100, 720), nsgotest.Environment(testlocale.Environment(ns.Environment{OS: ns.OSMacOS, Idiom: ns.IdiomDesktop, AppleKeyboard: true})))
	link := app.Find(whole("https://youtu.be/talk"))
	if got := link.TextStyle.Color; got != k.AccentInk() || got == theme.Accent {
		t.Fatalf("the source link is %#x on the dark panel, want the dark accent ink %#x", uint32(got), uint32(k.AccentInk()))
	}
}

// TestPosterURL checks that the poster's file: URL leads back to its file when the folder yt-dlp named after the video's title holds a '#', a '%' or a '?', as YouTube titles often do, which an unescaped URL takes for a fragment, a broken escape or a query.
func TestPosterURL(t *testing.T) {
	path := filepath.Join(t.TempDir(), "#live 100% [abc]?", "poster.jpg")
	u, err := url.Parse(fileURL(path))
	if err != nil {
		t.Fatalf("url.Parse(%q): %v", fileURL(path), err)
	}
	if got, ok := core.FilePath(u); !ok || got != path {
		t.Fatalf("FilePath(%q) = %q, %v, want %q", fileURL(path), got, ok, path)
	}
	if fileURL("") != "" {
		t.Errorf("fileURL(\"\") = %q, want \"\"", fileURL(""))
	}
}

// TestNudgeLift checks where Move Subtitles Up and Down take the lines: a fiftieth of the picture a press, from where they yield to the controls for a film that never moved them, from the film's own place otherwise, no further than a drag's room, freely within lines too tall for the picture, and nowhere before the lines were laid out.
func TestNudgeLift(t *testing.T) {
	room := SubRoom{Rest: 0.08, Lo: -0.05, Hi: 0.3}
	for _, c := range []struct {
		name string
		lift float32
		dir  int
		room SubRoom
		want float32
	}{
		{"up from the yield", 0, 1, room, 0.10},
		{"down from the yield", 0, -1, room, 0.06},
		{"up from the film's place", 0.2, 1, room, 0.22},
		{"down from the film's place", 0.2, -1, room, 0.18},
		{"up at the room's top", 0.29, 1, room, 0.3},
		{"down at the room's bottom", -0.04, -1, room, -0.05},
		{"lines too tall for the picture", 0.5, 1, SubRoom{Lo: 1, Hi: -1}, 0.52},
		{"lines not laid out yet", 0.2, 1, SubRoom{}, 0.2},
	} {
		if got := NudgeLift(c.lift, c.dir, c.room); math.Abs(float64(got-c.want)) > 1e-6 {
			t.Errorf("%s: NudgeLift(%v, %d) = %v, want %v", c.name, c.lift, c.dir, got, c.want)
		}
	}
}

// A done link's info tab says where its original lines came from: its site's subtitles, with how many lines the speech recognition heard in their place, or the speech recognition alone. A local video says nothing of it.
func TestInfoTabSaysWhereTheOriginalLinesCameFrom(t *testing.T) {
	msgs := testlocale.Messages()
	for _, tc := range []struct {
		source   string
		platform bool
		local    int
		want     string
	}{
		{"https://www.youtube.com/watch?v=jNQXAC9IVRw", true, 0, msgs.FilmOriginalPlatform()},
		{"https://www.youtube.com/watch?v=jNQXAC9IVRw", true, 3, msgs.FilmOriginalPlatformMixed(3)},
		{"https://www.youtube.com/watch?v=jNQXAC9IVRw", false, 0, msgs.FilmOriginalHeard()},
		{"file:///films/talk/talk.mp4", false, 0, ""},
	} {
		a := newFakeApp()
		a.side = "info"
		a.item.Source, a.item.PlatformSubtitles, a.item.LocalLines = tc.source, tc.platform, tc.local
		app := start(t, a)
		if tc.want == "" {
			if app.Exists("id:film-info-original") {
				t.Fatalf("%s: a local video says where its lines came from", tc.source)
			}
			continue
		}
		if got := app.Find("id:film-info-original"); got == nil || got.Text != tc.want {
			t.Fatalf("%+v: info tab %q", tc, app.Texts())
		}
	}
}

// TestWordsOfTheOriginalLookUp checks that a click on a word of the original line on the picture looks the word up in its cue, and a click on the translation does not; and that the card of the word looked up shows what the app says of it, its buttons reporting their clicks, Open Settings in place of the others when no AI service can answer.
func TestWordsOfTheOriginalLookUp(t *testing.T) {
	a := newFakeApp()
	a.item.Language = "简体中文"
	app := start(t, a)
	msgs := testlocale.Messages()
	app.Play("id:film-video", ns.VideoInfo{Duration: 10 * time.Second})
	app.Progress("id:film-video", 1500*time.Millisecond, 10*time.Second)
	app.Press(`id:film-subtitle-lines >> text:"Hello"`)
	if !slices.Contains(a.calls, "look up 0 Hello") {
		t.Fatalf("a click on Hello made %q", a.calls)
	}
	a.calls = nil
	app.Press(`id:film-subtitle-lines >> text:"你好。"`)
	if slices.ContainsFunc(a.calls, func(c string) bool { return strings.HasPrefix(c, "look up") }) {
		t.Fatalf("a click on the translation looked up: %q", a.calls)
	}
	if app.Exists("id:film-lookup") {
		t.Fatal("a card shows with no word looked up")
	}
	change(app, a, func() {
		a.lookup = Lookup{Open: true, Word: "hello", Reading: "/həˈləʊ/", Meaning: "int. 你好", Line: "Hello."}
	})
	for _, want := range []string{"hello", "/həˈləʊ/", "int. 你好", "Hello.", msgs.VocabAddToVocabulary(), msgs.FilmExplainLine()} {
		if !app.Exists(whole(want)) {
			t.Fatalf("the card has no %q", want)
		}
	}
	app.Press("id:film-lookup-save")
	app.Press("id:film-lookup-explain")
	app.Press("id:film-lookup-close")
	if got := a.calls[len(a.calls)-3:]; !slices.Equal(got, []string{"toggle word", "explain lookup", "close lookup"}) {
		t.Fatalf("the card's buttons made %q", got)
	}
	change(app, a, func() { a.lookup.Saved = true })
	if !app.Exists(whole(msgs.VocabInVocabulary())) {
		t.Fatal("a word in the vocabulary is not said to be")
	}
	change(app, a, func() {
		a.lookup = Lookup{Open: true, Word: "Hello", Line: "Hello.", Failed: true, NeedsAI: true, Say: func(cx *ns.Scope) string { return "no AI" }}
	})
	if !app.Exists(whole("no AI")) || app.Exists("id:film-lookup-save") || app.Exists("id:film-lookup-explain") {
		t.Fatal("the card without an AI service")
	}
	app.Press("id:film-lookup-settings")
	if a.calls[len(a.calls)-1] != "lookup settings" {
		t.Fatalf("Open Settings made %q", a.calls)
	}
}
