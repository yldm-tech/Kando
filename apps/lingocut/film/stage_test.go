package film

import (
	"testing"

	"github.com/egoist/mygo/ui"
)

// TestPictureFillsTheStage checks that a picture close to the stage's shape covers it, as the prototype's does, with no black bars under the title bar and the transport, and that one far from it shows whole.
func TestPictureFillsTheStage(t *testing.T) {
	for _, tc := range []struct {
		name           string
		stageW, stageH float32
		videoW, videoH int
		want           ui.Fit
	}{
		{"16:9 in the prototype's window", 1304, 782, 1920, 1080, ui.Cover},
		{"16:9 in a 16:10 window", 1440, 900, 1920, 1080, ui.Cover},
		{"1.85:1 in a 16:10 window", 1440, 900, 1998, 1080, ui.Cover},
		{"the same shape", 1280, 720, 1920, 1080, ui.Cover},
		{"2.39:1 in a 16:10 window", 1440, 900, 2048, 858, ui.Contain},
		{"4:3 in the prototype's window", 1304, 782, 640, 480, ui.Contain},
		{"portrait in a wide window", 1304, 782, 1080, 1920, ui.Contain},
		{"no frame yet", 1304, 782, 0, 0, ui.Cover},
	} {
		if got := pictureFit(tc.stageW, tc.stageH, tc.videoW, tc.videoH); got != tc.want {
			t.Errorf("%s: fit %v, want %v", tc.name, got, tc.want)
		}
	}
}
