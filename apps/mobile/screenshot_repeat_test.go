package main

import (
	"testing"
	"time"
)

// The playback settings with 循环播放 (Repeat Film) on, and its pill over the picture.
func TestScreenshotsPlayerRepeat(t *testing.T) {
	shot := screenshots(t)
	p := playerScene(t, "talk.mp4", videoSizes["16:9"])
	p.app.Press("id:player-more")
	p.app.WaitFor("id:player-settings-repeat", wait)
	// The sheet slides up over its transition, which a press waits for.
	p.app.Advance(600 * time.Millisecond)
	p.app.Press("id:player-settings-repeat")
	// The pill over the picture, under the sheet, says repeat is on.
	p.app.WaitUntil(wait, func() bool { return len(p.app.QueryMounted("id:player-pill-repeat")) == 1 })
	p.app.Advance(600 * time.Millisecond)
	shot(p, "player-settings-repeat")
}
