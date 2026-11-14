package nowplaying

import (
	"testing"
	"time"
)

// TestSystemTakesWhatThePlayerShows checks that the player can show, update and clear a film and listen for commands on System, which sends none, and stop listening twice.
func TestSystemTakesWhatThePlayerShows(t *testing.T) {
	center := System()
	off := center.OnCommand(func(Command, time.Duration) { t.Fatal("System sent a command") })
	center.Set(Info{Title: "Spring Clip", Duration: 10 * time.Second, State: Playing, Commands: Play | Pause | Seek})
	center.SetPlayback(Paused, 3*time.Second)
	center.Clear()
	off()
	off()
}
