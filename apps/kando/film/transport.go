package film

import (
	"time"

	"kando/apps/kando/locales"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// transport is the glass capsule along the bottom of the picture: back and forward 5 seconds, play or hold, the time played, the scrubber, the time left, the volume, the speed, the subtitle mode, the side panel and full screen. It shows once a video is opening and fades with the title bar.
func (s screen) transport(c *ui.Context, t *ui.Theme, fade float32, shown bool, panelSpace float32) {
	if s.Play() == nil && s.PlayPath() == "" {
		return
	}
	right := float32(20)
	if panelSpace > 0 {
		right = panelSpace + 16
	}
	bar := ui.Row(c).Absolute().Left(20).Right(right).Bottom(18).Gap(10).Padding(8, 10, 8, 12).Radius(24).AlignItems(ui.Center).TextColor(onGlass).Opacity(fade)
	glassFace(bar, ui.RGBA(30, 30, 34, 0.5), 28, 0.22, 24).Shadow(0, 10, 30, 0, ui.RGBA(0, 0, 0, 0.3))
	if !shown {
		bar.PassThrough()
	}
	bar.Children(func() {
		ui.Row(c).Gap(2).AlignItems(ui.Center).Shrink(0).Children(func() {
			c.Key("film-back5")
			if skipButton(c, iconBack5, locales.FilmSkipBack(5)).Tooltip(locales.FilmSkipBack(5) + " " + breeze.ShortcutText(c, 0, ui.KeyLeft)).Clicked() {
				s.Skip(-5 * time.Second)
			}
			s.playButton(c)
			c.Key("film-forward5")
			if skipButton(c, iconForward5, locales.FilmSkipForward(5)).Tooltip(locales.FilmSkipForward(5) + " " + breeze.ShortcutText(c, 0, ui.KeyRight)).Clicked() {
				s.Skip(5 * time.Second)
			}
		})
		head, length := s.PlayHead(), s.PlayLength()
		// While the scrubber is held, the time shows where it would seek.
		if value, held := s.Scrub(); *held {
			head = seconds(*value)
		}
		ui.Text(c, cueClock(head)).FontSize(12).FontFeatures("tnum").MinWidth(32).Shrink(0)
		s.scrubber(c)
		left := length - head
		if left < 0 {
			left = 0
		}
		ui.Text(c, "-"+cueClock(left)).FontSize(12).FontFeatures("tnum").Opacity(0.75).Shrink(0)
		s.volume(c)
		ui.Row(c).Gap(2).AlignItems(ui.Center).Shrink(0).Children(func() {
			s.speedMenu(c, t)
			s.modeMenu(c, t)
			c.Key("film-panel")
			on := s.Panel()
			if glyphToggle(c, &on, iconPanel, locales.FilmSidebar(), 34, 30, 18, 15).Tooltip(locales.FilmSidebar() + " " + breeze.ShortcutText(c, ui.Shift|ui.Cmd, ui.KeyT)).Changed() {
				s.TogglePanel()
			}
			c.Key("film-full")
			if glyphButton(c, iconFullScreen, locales.FilmFullScreen(), 34, 30, 16, 16).Tooltip(locales.FilmFullScreen() + " F").Clicked() {
				s.ToggleFullScreen()
			}
		})
	})
}

// skipButton is a 5-second skip: the prototype's round arrow with a 5 inside it, in a 32-point button.
func skipButton(c *ui.Context, icon *ui.SVG, label string) ui.Element {
	b := glyphButton(c, icon, label, 32, 32, 20, 20)
	b.Children(func() {
		// The prototype's 5 sits on the arrow's middle, its baseline 13.2 down the 20-point glyph.
		ui.Text(c, "5").FontSize(6.5).Bold().LineHeight(1).TextAlign(ui.Center).Absolute().Left(0).Right(0).Top(14.2).Role(ui.RoleNone).PassThrough()
	})
	return b
}

// playButton starts or holds the film. While the video opens it waits, named for that, so that 播放 shows only once there is a picture to play.
func (s screen) playButton(c *ui.Context) {
	c.Key("film-play")
	switch {
	case s.Play() == nil:
		glyphButton(c, iconPlay, locales.FilmOpening(), 36, 36, 20, 20).Disabled(true)
	case s.Play().Playing():
		if glyphButton(c, iconPause, locales.FilmPause(), 36, 36, 20, 20).Tooltip(locales.FilmPause() + " " + locales.FilmKeySpace()).Clicked() {
			s.togglePlay(c)
		}
	default:
		if glyphButton(c, iconPlay, locales.FilmPlay(), 36, 36, 20, 20).Tooltip(locales.FilmPlay() + " " + locales.FilmKeySpace()).Clicked() {
			s.togglePlay(c)
		}
	}
}
