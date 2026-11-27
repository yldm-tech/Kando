package main

import (
	"testing"
	"time"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// The player upright on a small phone (375 by 667, an iPhone SE's screen) with the store's talk (testdata/store/talk.mp4, 16:9) and the same talk cut to an upright 9:16 picture (talk-portrait.mp4), held where it was left: the picture, the card, the transport and the list of lines all fit the screen. On a target the system's player reads the video's size from the file; headless the scene reports it.

// playerSmallPhone opens the talk kept with the video of testdata/store named asset, of size, on a phone of 375 by 667, held where it was left after the recap.
func playerSmallPhone(t *testing.T, asset string, size core.Size) *phone {
	t.Helper()
	d := storeDesktop(t)
	dir := t.TempDir()
	f := storeKeepVideo(t, d, dir, 3500*time.Millisecond, asset)
	studyNoted(t, dir)
	p := startPhone(t, dir, pairedWith(t, d, dir), nsgotest.Size(375, 667))
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
	return p
}

func TestScreenshotsPlayerUprightWide(t *testing.T) {
	shot := screenshots(t)
	p := playerSmallPhone(t, "talk.mp4", core.Size{Width: 1280, Height: 720})
	shot(p, "player-upright-small-16x9")
}

func TestScreenshotsPlayerUprightTall(t *testing.T) {
	shot := screenshots(t)
	p := playerSmallPhone(t, "talk-portrait.mp4", core.Size{Width: 360, Height: 640})
	shot(p, "player-upright-small-9x16")
}
