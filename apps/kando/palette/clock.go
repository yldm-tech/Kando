package palette

import (
	"fmt"
	"time"
)

// Clock writes d as the player does, "1:05" or "1:02:03".
func Clock(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	ms := d.Milliseconds()
	h, m, s := ms/3_600_000, ms/60_000%60, ms/1000%60
	if h > 0 {
		return fmt.Sprintf("%d:%02d:%02d", h, m, s)
	}
	return fmt.Sprintf("%d:%02d", m, s)
}

// ClockTenths is Clock with the tenth of a second, as the subtitle list writes where a line starts ("0:30.4").
func ClockTenths(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	return fmt.Sprintf("%s.%d", Clock(d), d.Milliseconds()/100%10)
}
