package film

import (
	"time"

	"github.com/egoist/mygo/ui"
)

// osdFor is how long a message shows on the picture (prototype script 1972).
const osdFor = 1400 * time.Millisecond

// osd is the small glass pill at the top left of the picture that says what the last action did, as "−5 秒" or "字幕：双语", for osdFor.
func (s screen) osd(c *ui.Context, now time.Time) {
	text, left := s.OSD(now)
	if text == "" || left <= 0 {
		return
	}
	c.After(left)
	pill := ui.Row(c).Absolute().Left(20).Top(66).Padding(6, 12).Radius(12).TextColor(onGlass).PassThrough().Role(ui.RoleStatus)
	glassFace(pill, ui.RGBA(28, 28, 32, 0.62), 20, 0.2).Children(func() {
		ui.Text(c, text).FontSize(13).FontWeight(600)
	})
}
