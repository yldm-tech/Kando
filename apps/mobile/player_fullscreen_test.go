package main

import (
	"fmt"
	"math"
	"strings"
	"testing"
	"time"

	"kando/apps/mobile/store"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// The videos the layout tests open, by their ratio: a talk's 16:9, an old film's 4:3, a cinema 2.39:1 and a phone's upright 9:16.
var videoSizes = map[string]core.Size{
	"16:9":   {Width: 1920, Height: 1080},
	"4:3":    {Width: 1440, Height: 1080},
	"2.39:1": {Width: 1920, Height: 804},
	"9:16":   {Width: 1080, Height: 1920},
}

// openPlayerOf opens the talk on a screen of width by height whose video loads at size.
func openPlayerOf(t *testing.T, width, height core.Points, size core.Size) *phone {
	t.Helper()
	p, _ := openPlayer(t, nsgotest.Size(width, height))
	p.app.Play("id:player-video", core.VideoInfo{Duration: 10 * sec, Size: size})
	p.at(1500 * time.Millisecond)
	return p
}

// within fails t unless the view selector matches lies inside the window of width by height.
func within(t *testing.T, p *phone, selector string, width, height core.Points) core.Bounds {
	t.Helper()
	b := p.app.Find(selector).Bounds()
	if b.Origin.X < -0.5 || b.Origin.Y < -0.5 || b.Origin.X+b.Size.Width > width+0.5 || b.Origin.Y+b.Size.Height > height+0.5 {
		t.Fatalf("%s is at %+v, outside the %vx%v window", selector, b, width, height)
	}
	return b
}

func near(a, b core.Points) bool { return math.Abs(float64(a-b)) < 1 }

// stacked leaves the immersive layout an upright video plays in on a phone for the stacked one: the picture over the card, the transport and the panel.
func (p *phone) stacked() {
	p.t.Helper()
	p.app.WaitFor("id:player-fullscreen-exit", wait)
	p.app.Press("id:player-fullscreen-exit")
	p.app.WaitFor("id:player-card-scroll", wait)
}

// Upright, on a small phone, a large one and a tablet, whatever the video's ratio, the picture, the card, the transport and the list of lines all fit the window: the picture is as wide as the window, or set in on a tablet, at most 30% of its height, and as narrow as its ratio where that caps it, centered; the list keeps room for its lines and scrolls them.
func TestTheUprightPlayerFitsTheWindow(t *testing.T) {
	for _, screen := range []core.Size{{Width: 375, Height: 667}, {Width: 430, Height: 932}, {Width: 820, Height: 1180}} {
		for name, size := range videoSizes {
			t.Run(fmt.Sprintf("%vx%v %s", screen.Width, screen.Height, name), func(t *testing.T) {
				p := openPlayerOf(t, screen.Width, screen.Height, size)
				if size.Width < size.Height && screen.Width < 700 {
					p.stacked()
				}
				stage := within(t, p, "id:player-stage", screen.Width, screen.Height)
				room := screen.Width
				if screen.Width >= 700 {
					room -= 48
				}
				aspect := core.Points(size.Width / size.Height)
				wantH := min(room/aspect, screen.Height*0.3)
				if !near(stage.Size.Height, wantH) || !near(stage.Size.Width, wantH*aspect) {
					t.Fatalf("the picture is %vx%v, want %vx%v", stage.Size.Width, stage.Size.Height, wantH*aspect, wantH)
				}
				if mid := stage.Origin.X + stage.Size.Width/2; !near(mid, screen.Width/2) {
					t.Fatalf("the picture's middle is at %v of %v", mid, screen.Width)
				}
				card := p.app.Find("id:player-card-scroll")
				within(t, p, "id:player-card-scroll", screen.Width, screen.Height)
				// Past a small phone the card shows whole; on one the card's own scroll gives way to the list.
				if screen.Height >= 900 && card.Content.Height > card.Frame.Size.Height+0.5 {
					t.Fatalf("the card's %v are cut to %v", card.Content.Height, card.Frame.Size.Height)
				}
				within(t, p, "id:player-transport", screen.Width, screen.Height)
				within(t, p, "id:player-play", screen.Width, screen.Height)
				lines := within(t, p, "id:player-lines", screen.Width, screen.Height)
				if lines.Size.Height < 56 {
					t.Fatalf("the list of lines is %v tall", lines.Size.Height)
				}
			})
		}
	}
}

// On a small phone the list of lines, shorter than its four lines, scrolls them.
func TestOnASmallPhoneTheListOfLinesScrolls(t *testing.T) {
	p := openPlayerOf(t, 375, 667, videoSizes["16:9"])
	list := p.app.Find("id:player-lines")
	if list.Content.Height <= list.Frame.Size.Height {
		t.Fatalf("the list's lines are %v tall in a list %v tall", list.Content.Height, list.Frame.Size.Height)
	}
	p.app.Scroll("id:player-lines", 60)
	p.app.WaitUntil(wait, func() bool { return p.app.Find("id:player-lines").Offset.Y > 0 })
}

// A video near square, as a dance filmed upright and cropped to 828×720, keeps its full screen upright: on its side it would hardly grow, and it reads as upright. From 4:3 on, full screen turns the phone sideways.
func TestFullScreenKeepsANearSquareVideoUpright(t *testing.T) {
	p := openPlayerOf(t, 390, 844, core.Size{Width: 828, Height: 720})
	p.app.Press("id:player-fullscreen")
	if p.app.Orientation != core.OrientationPortrait {
		t.Fatalf("full screen of a near-square video asks for %v, want upright", p.app.Orientation)
	}
	p.app.WaitFor("id:player-mode-pill", wait)
	if b := p.app.Find("id:player-stage").Bounds(); !near(b.Size.Width, 390) {
		t.Fatalf("the picture is %+v in an upright full screen", b.Size)
	}

	q := openPlayerOf(t, 390, 844, videoSizes["4:3"])
	q.app.Press("id:player-fullscreen")
	if q.app.Orientation != core.OrientationLandscape {
		t.Fatalf("full screen of a 4:3 video asks for %v, want sideways", q.app.Orientation)
	}
}

// Full screen from the picture's button turns a wide video's screen sideways and lays the player out as on its side, over the same video, which plays on where it was; its button in the row over the picture leaves it, and the screen is the system's to turn again.
func TestFullScreenTurnsAWideVideoSideways(t *testing.T) {
	p := openPlayerOf(t, 390, 844, videoSizes["16:9"])
	p.at(4 * time.Second)
	video := p.app.Find("id:player-video").ID
	if p.app.Orientation != 0 {
		t.Fatalf("the player asks for %v before full screen", p.app.Orientation)
	}
	p.app.Press("id:player-fullscreen")
	if p.app.Orientation != core.OrientationLandscape {
		t.Fatalf("full screen asks for %v", p.app.Orientation)
	}
	// The system turns the screen.
	p.app.Resize(844, 390)
	p.app.WaitFor("id:player-mode-pill", wait)
	if p.app.Exists("id:player-card") || p.app.Exists("id:player-fullscreen") {
		t.Fatal("the card or the full-screen button shows in full screen")
	}
	if b := p.app.Find("id:player-stage").Bounds(); !near(b.Size.Width, 844) || !near(b.Size.Height, 390) {
		t.Fatalf("the picture is %+v in full screen", b.Size)
	}
	v := p.app.Find("id:player-video")
	if v.ID != video || v.Video.Paused {
		t.Fatalf("full screen changed the video: %v (was %v), paused %v", v.ID, video, v.Video.Paused)
	}
	if got := p.texts("id:player-at"); got != "0:04" {
		t.Fatalf("full screen is at %q", got)
	}
	p.app.Press("id:player-fullscreen-exit")
	if p.app.Orientation != 0 {
		t.Fatalf("leaving full screen still asks for %v", p.app.Orientation)
	}
	p.app.Resize(390, 844)
	p.app.WaitFor("id:player-card", wait)
	p.app.WaitFor("id:player-fullscreen", wait)
	if v := p.app.Find("id:player-video"); v.ID != video || v.Video.Paused {
		t.Fatal("leaving full screen changed the video")
	}
	if got := p.texts("id:player-at"); got != "0:04" {
		t.Fatalf("after full screen the player is at %q", got)
	}
}

// On a phone held upright an upright video plays immersive by default, as full screen: the picture fills the screen, a tap shows and hides the controls, whose capsule takes two rows, and the list of lines slides up from the foot with the card and its actions over the panel's tabs. Its button stacks the picture over the card and the transport, which the player remembers, and the picture's button goes back; the video is the same throughout, and the screen is not asked to turn.
func TestAnUprightVideoPlaysImmersiveOnAPhone(t *testing.T) {
	p := openPlayerOf(t, 402, 874, videoSizes["9:16"])
	video := p.app.Find("id:player-video").ID
	p.app.WaitFor("id:player-mode-pill", wait)
	if p.app.Orientation != 0 {
		t.Fatalf("the immersive layout asks for %v", p.app.Orientation)
	}
	if b := p.app.Find("id:player-stage").Bounds(); !near(b.Size.Width, 402) || !near(b.Size.Height, 874) {
		t.Fatalf("the picture is %+v immersive", b.Size)
	}
	if p.app.Exists("id:player-card") {
		t.Fatal("the card shows under an immersive picture")
	}
	capsule := within(t, p, "id:player-transport", 402, 874)
	if !near(capsule.Size.Height, 88) {
		t.Fatalf("the capsule is %v tall upright", capsule.Size.Height)
	}
	for _, id := range []string{"player-play", "player-prev", "player-next", "player-loop", "player-speed", "player-position", "player-fullscreen-exit", "player-panel-toggle", "player-more"} {
		within(t, p, "id:"+id, 402, 874)
	}
	// A tap on the picture hides the controls, another shows them.
	p.app.Press("id:player-video")
	p.app.WaitForGone("id:player-transport", wait)
	p.app.Press("id:player-video")
	p.app.WaitFor("id:player-transport", wait)

	// The list slides up with the card, whose actions are in reach, and its lines scroll.
	p.app.Press("id:player-panel-toggle")
	p.app.WaitFor("id:player-card", wait)
	pane := within(t, p, "id:player-lines-pane", 402, 874)
	if pane.Origin.Y < 874*0.3 {
		t.Fatalf("the list slid up to %v", pane.Origin.Y)
	}
	for _, id := range []string{"player-card", "player-replay", "player-card-loop", "player-save", "player-explain-line", "player-tabs", "player-lines"} {
		within(t, p, "id:"+id, 402, 874)
	}
	if p.app.Exists("id:player-transport") {
		t.Fatal("the capsule shows over the list slid up")
	}
	list := p.app.Find("id:player-lines")
	if list.Frame.Size.Height < 56 {
		t.Fatalf("the slid-up list is %v tall", list.Frame.Size.Height)
	}
	p.app.Press("id:player-card-loop")
	if !p.app.Exists("id:player-pill-loop") {
		t.Fatal("the card's loop in the slid-up list did not loop the line")
	}
	// Back puts the list away before it minimizes.
	p.app.Back()
	p.app.WaitForGone("id:player-lines-pane", wait)
	if p.app.Exists("id:player-mini") {
		t.Fatal("Back minimized the player with the list up")
	}

	p.stacked()
	if p.app.Exists("id:player-mode-pill") {
		t.Fatal("the stacked layout keeps the row over the picture")
	}
	if b := p.app.Find("id:player-stage").Bounds(); !near(b.Size.Height, 874*0.3) {
		t.Fatalf("the stacked picture is %+v", b.Size)
	}
	// Minimized and opened again, the player stays stacked.
	p.app.Press("id:player-minimize")
	p.app.WaitFor("id:player-mini", wait)
	p.app.Press("id:player-mini-open")
	p.app.WaitFor("id:player-card", wait)
	if p.app.Exists("id:player-mode-pill") {
		t.Fatal("opened again the player left the stacked layout")
	}
	p.app.Press("id:player-fullscreen")
	p.app.WaitFor("id:player-mode-pill", wait)
	if p.app.Orientation != 0 {
		t.Fatalf("going immersive asks for %v", p.app.Orientation)
	}
	if v := p.app.Find("id:player-video"); v.ID != video {
		t.Fatal("switching layouts made the video anew")
	}
	// Turned sideways the video stays full; a wide video upright stays stacked.
	p.app.Resize(874, 402)
	p.app.WaitFor("id:player-mode-pill", wait)
	p.app.Resize(402, 874)
	p.app.Play("id:player-video", core.VideoInfo{Duration: 10 * sec, Size: videoSizes["16:9"]})
	p.app.WaitFor("id:player-card", wait)
}

// Back leaves full screen before it minimizes the player, and minimizing ends full screen and its request.
func TestBackAndMinimizingLeaveFullScreen(t *testing.T) {
	p := openPlayerOf(t, 390, 844, videoSizes["16:9"])
	p.app.Press("id:player-fullscreen")
	p.app.Back()
	p.app.WaitFor("id:player-card", wait)
	if p.app.Orientation != 0 || p.app.Exists("id:player-mini") {
		t.Fatalf("Back in full screen: orientation %v, minimized %v", p.app.Orientation, p.app.Exists("id:player-mini"))
	}
	p.app.Press("id:player-fullscreen")
	p.app.Press("id:player-minimize")
	p.app.WaitFor("id:player-mini", wait)
	if p.app.Orientation != 0 {
		t.Fatalf("minimized, the player still asks for %v", p.app.Orientation)
	}
	p.app.Press("id:player-mini-open")
	p.app.WaitFor("id:player-card", wait)
	if !p.app.Exists("id:player-fullscreen") {
		t.Fatal("the player opened again in full screen")
	}
}

// On a tablet on its side an upright video's picture keeps its ratio within the room beside the panel, over the card and the transport.
func TestOnATabletOnItsSideAnUprightVideoFits(t *testing.T) {
	p := openPlayerOf(t, 1180, 820, videoSizes["9:16"])
	stage := within(t, p, "id:player-stage", 1180, 820)
	if !near(stage.Size.Width/stage.Size.Height, 9.0/16) {
		t.Fatalf("the picture is %+v", stage.Size)
	}
	within(t, p, "id:player-play", 1180, 820)
	within(t, p, "id:player-lines", 1180, 820)
}

// On an iPhone 17 upright (402 by 874, its safe area 62 over and 34 under), stacked, an upright 9:16 video's picture is 30% of the height inside the safe area at its ratio, and the list of lines under the transport keeps room for two lines.
func TestOnAnIPhone17AnUprightVideoLeavesTheListRoom(t *testing.T) {
	p, _ := openPlayer(t, nsgotest.Size(402, 874), nsgotest.Environment(core.Environment{Locale: lang, SafeArea: core.Edges{Top: 62, Bottom: 34}}))
	p.app.Play("id:player-video", core.VideoInfo{Duration: 10 * sec, Size: videoSizes["9:16"]})
	p.stacked()
	stage := within(t, p, "id:player-stage", 402, 874)
	if h := (874 - 62 - 34) * core.Points(0.3); !near(stage.Size.Height, h) || !near(stage.Size.Width, h*9/16) {
		t.Fatalf("the picture is %+v", stage.Size)
	}
	card := p.app.Find("id:player-card-scroll")
	if card.Content.Height > card.Frame.Size.Height+0.5 {
		t.Fatalf("the card's %v are cut to %v", card.Content.Height, card.Frame.Size.Height)
	}
	within(t, p, "id:player-play", 402, 874-34)
	lines := within(t, p, "id:player-lines", 402, 874-34)
	if lines.Size.Height < 112 {
		t.Fatalf("the list of lines is %v tall", lines.Size.Height)
	}
	t.Logf("stage %+v, card %v, lines %+v", stage, card.Frame.Size.Height, lines)
}

// The card's place keeps to one line next to its hint at an iPhone 17's width, however many lines the film has; the hint is cut short.
func TestTheCardsPlaceKeepsToOneLine(t *testing.T) {
	dir := t.TempDir()
	f := keepTalk(t, dir)
	many := make([]sync.Cue, 1236)
	for i := range many {
		many[i] = sync.Cue{StartMS: int64(i) * 3000, EndMS: int64(i)*3000 + 2500, Original: "Good morning.", Translation: "早上好。"}
	}
	if err := store.Open(dir).SaveCues(f.Desktop, f.Film.ID, many); err != nil {
		t.Fatal(err)
	}
	p := startPhone(t, dir, nsgotest.Size(402, 874))
	p.app.WaitFor(talkRow(f.Desktop), wait)
	p.app.Press(talkRow(f.Desktop))
	p.app.WaitFor("id:player-card-place", wait)
	p.app.Play("id:player-video", core.VideoInfo{Duration: time.Hour, Size: videoSizes["16:9"]})
	p.app.Progress("id:player-video", 54*time.Second, time.Hour)
	p.app.WaitUntil(wait, func() bool { return strings.Contains(p.texts("id:player-card-place"), "1,236") })
	for _, id := range []string{"player-card-place", "player-card-hint"} {
		if h := p.app.Find("id:" + id).Frame.Size.Height; h > 17 {
			t.Fatalf("%s wraps to %v at 402 wide: %q", id, h, p.texts("id:"+id))
		}
	}
}

// A drag down on the playback settings' sheet swipes it away.
func TestThePlaybackSettingsSwipeAway(t *testing.T) {
	p, _ := openPlayer(t)
	p.app.Press("id:player-more")
	p.app.WaitFor("id:player-settings", wait)
	p.app.Advance(600 * time.Millisecond)
	p.app.Pan("id:player-settings-sheet", core.Point{Y: 120}, core.Point{Y: 420})
	p.app.Advance(600 * time.Millisecond)
	p.app.WaitForGone("id:player-settings", wait)
}

// A film its computer did not translate has no translation for the card to blur, so the card's hint is the one about the words, not the one to tap a blurred line; with a translation hidden it is.
func TestTheCardsHintFollowsWhatIsBlurred(t *testing.T) {
	dir := t.TempDir()
	f := keepTalk(t, dir)
	f.Film.Translation = ""
	st := store.Open(dir)
	if err := st.SaveFilm(f); err != nil {
		t.Fatal(err)
	}
	own := make([]sync.Cue, len(talkCues))
	for i, c := range talkCues {
		own[i] = sync.Cue{StartMS: c.StartMS, EndMS: c.EndMS, Original: c.Original}
	}
	if err := st.SaveCues(f.Desktop, f.Film.ID, own); err != nil {
		t.Fatal(err)
	}
	p := startPhone(t, dir)
	p.app.WaitFor(talkRow(f.Desktop), wait)
	p.app.Press(talkRow(f.Desktop))
	p.app.WaitFor("id:player-card-hint", wait)
	p.app.Play("id:player-video", core.VideoInfo{Duration: 10 * sec, Size: videoSizes["16:9"]})
	p.at(1500 * time.Millisecond)
	if got := p.texts("id:player-card-hint"); got != msgs.PlayerHintWords() {
		t.Fatalf("an untranslated film's hint reads %q", got)
	}

	q, _ := openPlayer(t)
	q.at(1500 * time.Millisecond)
	q.app.Press("id:player-mode")
	q.app.Press("id:player-mode-original")
	if got := q.texts("id:player-card-hint"); got != msgs.PlayerHintReveal() {
		t.Fatalf("with the translation hidden the hint reads %q", got)
	}
}
