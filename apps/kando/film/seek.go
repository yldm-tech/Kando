package film

import "time"

// seekNear is how far from where a seek went a report of the system's player may be and still be where it landed, as a player that seeks to a key frame lands near the time asked for rather than on it.
const seekNear = time.Second

// SeekLanded reports whether position, a time the system's player reported after a seek from from to to, is where the seek landed rather than a report the player sent before it landed. ran is how much of the film has played since the seek, 0 while it is held: a report after the seek landed is at to or up to ran past it, and one sent before goes on from from. A report counts as landed when it is no farther than seekNear from what a landed one says and at least as near that as what one sent before would say, so a seek back by less than seekNear, as a short looped line going back to its start, does not believe a report from its end.
func SeekLanded(from, to, ran, position time.Duration) bool {
	// core.Video reports no seek's completion (NSGo declined it in nsgo#27), so where a seek landed is told from the reports' positions.
	landed := time.Duration(0)
	switch {
	case position < to:
		landed = to - position
	case position > to+ran:
		landed = position - to - ran
	}
	if landed > seekNear {
		return false
	}
	return landed <= (position - from - ran).Abs()
}
