package main

import (
	"fmt"
	"math"
	"testing"
	"time"

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

// Upright, on a small phone, a large one and a tablet, whatever the video's ratio, the picture, the card, the transport and the list of lines all fit the window: the picture is as wide as the window, or set in on a tablet, at most 30% of its height, and as narrow as its ratio where that caps it, centered; the list keeps room for its lines and scrolls them.
func TestTheUprightPlayerFitsTheWindow(t *testing.T) {
	for _, screen := range []core.Size{{Width: 375, Height: 667}, {Width: 430, Height: 932}, {Width: 820, Height: 1180}} {
		for name, size := range videoSizes {
			t.Run(fmt.Sprintf("%vx%v %s", screen.Width, screen.Height, name), func(t *testing.T) {
				p := openPlayerOf(t, screen.Width, screen.Height, size)
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

// Full screen of an upright video stays upright: the picture fills the screen, the capsule takes two rows and the list of lines does not slide in.
func TestFullScreenOfAnUprightVideoStaysUpright(t *testing.T) {
	p := openPlayerOf(t, 390, 844, videoSizes["9:16"])
	p.app.Press("id:player-fullscreen")
	if p.app.Orientation != core.OrientationPortrait {
		t.Fatalf("full screen of an upright video asks for %v", p.app.Orientation)
	}
	p.app.WaitFor("id:player-mode-pill", wait)
	if b := p.app.Find("id:player-stage").Bounds(); !near(b.Size.Width, 390) || !near(b.Size.Height, 844) {
		t.Fatalf("the picture is %+v in full screen", b.Size)
	}
	capsule := within(t, p, "id:player-transport", 390, 844)
	if !near(capsule.Size.Height, 88) {
		t.Fatalf("the capsule is %v tall upright", capsule.Size.Height)
	}
	for _, id := range []string{"player-play", "player-prev", "player-next", "player-loop", "player-speed", "player-position", "player-at", "player-left", "player-fullscreen-exit", "player-more"} {
		within(t, p, "id:"+id, 390, 844)
	}
	if p.app.Exists("id:player-panel-toggle") {
		t.Fatal("an upright full screen offers the list of lines at its side")
	}
	// Turned sideways the phone shows it as on its side; back upright, still in full screen.
	p.app.Resize(844, 390)
	p.app.WaitFor("id:player-panel-toggle", wait)
	p.app.Resize(390, 844)
	p.app.WaitForGone("id:player-panel-toggle", wait)
	if p.app.Exists("id:player-card") {
		t.Fatal("turned back upright the player left full screen")
	}
	p.app.Press("id:player-fullscreen-exit")
	p.app.WaitFor("id:player-card", wait)
	if p.app.Orientation != 0 {
		t.Fatalf("leaving full screen still asks for %v", p.app.Orientation)
	}
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

// On an iPhone 17 upright (402 by 874, its safe area 62 over and 34 under) an upright 9:16 video's picture is 30% of the height inside the safe area at its ratio, and the list of lines under the transport keeps room for two lines.
func TestOnAnIPhone17AnUprightVideoLeavesTheListRoom(t *testing.T) {
	p, _ := openPlayer(t, nsgotest.Size(402, 874), nsgotest.Environment(core.Environment{Locale: lang, SafeArea: core.Edges{Top: 62, Bottom: 34}}))
	p.app.Play("id:player-video", core.VideoInfo{Duration: 10 * sec, Size: videoSizes["9:16"]})
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
