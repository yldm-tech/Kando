package main

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// The player upright on a small phone (375 by 667, an iPhone SE's screen) and on an iPhone 17 (402 by 874, 62 over and 34 under its safe area) with the store's talk (testdata/store/talk.mp4, 16:9) and the same talk cut to an upright 9:16 picture (talk-portrait.mp4), held where it was left: the picture, the card, the transport and the list of lines all fit the screen. On a target the system's player reads the video's size from the file; headless the scene reports it.

// playerSmallPhone opens the talk kept with the video of testdata/store named asset, of size, on a phone of 375 by 667, held where it was left after the recap.
func playerSmallPhone(t *testing.T, asset string, size core.Size) *phone {
	t.Helper()
	return playerScene(t, asset, size, nsgotest.Size(375, 667))
}

// playerScene is playerSmallPhone on the phone opts start.
func playerScene(t *testing.T, asset string, size core.Size, opts ...nsgotest.Option) *phone {
	t.Helper()
	d := storeDesktop(t)
	dir := t.TempDir()
	f := storeKeepVideo(t, d, dir, 3500*time.Millisecond, asset)
	studyNoted(t, dir)
	p := startPhone(t, dir, append([]nsgotest.Option{pairedWith(t, d, dir)}, opts...)...)
	p.app.WaitFor(talkRow(f.Desktop), wait)
	p.app.Press(talkRow(f.Desktop))
	p.app.WaitFor("id:player-video", wait)
	if nsgotest.Target() == "" {
		p.app.Play("id:player-video", core.VideoInfo{Duration: 10 * time.Second, Size: size})
	}
	// Mounted, not shown: a transport the screen cuts off still says the video's length once it has loaded.
	p.app.WaitUntil(time.Minute, func() bool {
		left := p.app.QueryMounted("id:player-left")
		return len(left) == 1 && left[0].Text == "-0:06"
	})
	storeCloseRecap(p)
	settle(p)
	return p
}

// settle gives a target's system player a moment to draw the held video's frame at its new size, which it does in its own time.
func settle(p *phone) {
	p.t.Helper()
	if nsgotest.Target() == "" {
		return
	}
	since := time.Now()
	p.app.WaitUntil(time.Minute, func() bool { return time.Since(since) > 2*time.Second })
}

func TestScreenshotsPlayerUprightWide(t *testing.T) {
	shot := screenshots(t)
	p := playerSmallPhone(t, "talk.mp4", core.Size{Width: 1280, Height: 720})
	shot(p, "player-upright-small-16x9")
}

func TestScreenshotsPlayerUprightIPhone17(t *testing.T) {
	shot := screenshots(t)
	p := playerScene(t, "talk-portrait.mp4", core.Size{Width: 360, Height: 640}, nsgotest.Size(402, 874), nsgotest.Environment(core.Environment{Locale: lang, SafeArea: core.Edges{Top: 62, Bottom: 34}}))
	shot(p, "player-upright-iphone17-9x16")
}

func TestScreenshotsPlayerUprightTall(t *testing.T) {
	shot := screenshots(t)
	p := playerSmallPhone(t, "talk-portrait.mp4", core.Size{Width: 360, Height: 640})
	shot(p, "player-upright-small-9x16")
}

// Full screen of the wide talk from the picture's button: the phone turned sideways as the player asks (the scene turns the window as the system would), the row over the picture with the button that leaves full screen.
func TestScreenshotsPlayerFullScreenWide(t *testing.T) {
	shot := screenshots(t)
	p := playerSmallPhone(t, "talk.mp4", core.Size{Width: 1280, Height: 720})
	p.app.Press("id:player-fullscreen")
	if p.app.Orientation != core.OrientationLandscape {
		t.Fatalf("full screen asks for %v", p.app.Orientation)
	}
	p.app.Resize(667, 375)
	p.app.WaitFor("id:player-fullscreen-exit", wait)
	settle(p)
	shot(p, "player-fullscreen-small-16x9")
}

// Full screen of the upright talk stays upright: the picture fills the screen, the capsule's times and slider over its buttons.
func TestScreenshotsPlayerFullScreenTall(t *testing.T) {
	shot := screenshots(t)
	p := playerSmallPhone(t, "talk-portrait.mp4", core.Size{Width: 360, Height: 640})
	p.app.Press("id:player-fullscreen")
	p.app.WaitFor("id:player-fullscreen-exit", wait)
	settle(p)
	shot(p, "player-fullscreen-small-9x16")
}
