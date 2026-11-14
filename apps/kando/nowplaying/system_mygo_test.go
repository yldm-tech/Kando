package nowplaying

import (
	"testing"
	"time"

	"github.com/egoist/mygo"
)

// TestSystemMyGoSpeaksMyGo checks that what the player shows reaches mygo.NowPlaying as it was meant, held or playing and with the commands offered, and that the commands the system sends come back as Kando's, those Kando does not offer as 0.
func TestSystemMyGoSpeaksMyGo(t *testing.T) {
	poster := []byte("poster")
	got := mygoInfo(Info{Title: "Spring Clip", Artwork: poster, Duration: 10 * time.Second, Position: 3 * time.Second, State: Paused, Commands: Play | Pause | Seek})
	if got.Title != "Spring Clip" || string(got.Artwork) != "poster" || got.Duration != 10*time.Second || got.Position != 3*time.Second || got.State != mygo.PlaybackPaused || got.Commands != mygo.MediaPlay|mygo.MediaPause|mygo.MediaSeek {
		t.Fatalf("info %+v", got)
	}
	if got.Artist != "" || got.Album != "" || got.Rate != 0 {
		t.Fatalf("info %+v sets what Kando never shows", got)
	}
	if s := mygoState(Playing); s != mygo.PlaybackPlaying {
		t.Fatalf("playing is %v", s)
	}
	for _, tc := range []struct {
		in   mygo.MediaCommand
		want Command
	}{
		{mygo.MediaPlay, Play},
		{mygo.MediaPause, Pause},
		{mygo.MediaSeek, Seek},
		{mygo.MediaStop, 0},
		{mygo.MediaNext, 0},
		{mygo.MediaPrevious, 0},
	} {
		if got := command(tc.in); got != tc.want {
			t.Errorf("command(%v) = %v, want %v", tc.in, got, tc.want)
		}
	}
}
