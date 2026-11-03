package main

import (
	"context"
	"errors"
	"time"
)

// This file is overlaid onto MyGo's cmd/mygo by tools/mygo-dev/build.sh, next to the patched dev.go.

// overlapTimeout bounds how long the running build waits for the next one's window. Past it the running build is stopped anyway, as a build that shows no window would never replace it.
const overlapTimeout = 15 * time.Second

// waitForWindow waits until p shows a window, so that the running build can quit with the new one already on screen. It waits even when the running build's window is out of sight, as on another Space: the new window opens on the active Space, and quitting the running build first would still take the app's icon out of the Dock for a moment. It fails when p exits first, which keeps the running build; p is stopped when ctx ends.
func waitForWindow(ctx context.Context, p *devProcess) error {
	pid := p.cmd.Process.Pid
	deadline := time.NewTimer(overlapTimeout)
	defer deadline.Stop()
	tick := time.NewTicker(30 * time.Millisecond)
	defer tick.Stop()
	for !windowProbe(pid) {
		select {
		case <-ctx.Done():
			p.stop()
			return ctx.Err()
		case <-p.done:
			if p.err != nil {
				return p.err
			}
			return errors.New("the new build exited before it showed a window")
		case <-deadline.C:
			return nil
		case <-tick.C:
		}
	}
	return nil
}

// replaceLive marks the running build as replaced: the run loop then stops it without ending the session.
func (s *devSession) replaceLive() {
	s.liveMu.Lock()
	p := s.live
	s.liveMu.Unlock()
	if p != nil {
		p.replaced.Store(true)
	}
}
