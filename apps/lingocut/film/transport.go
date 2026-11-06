package film

import (
	"time"

	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// transport is the glass capsule along the bottom of the picture: back and forward 5 seconds, play or hold, the time played, the scrubber, the time left, the subtitle mode, the side panel and full screen. It shows once a video is opening and fades with the title bar.
func (s screen) transport(c *ui.Context, t *ui.Theme, fade float32, shown bool) {
	if s.Play() == nil && s.PlayPath() == "" {
		return
	}
	right := float32(20)
	if s.Panel() {
		right = panelRoom + 16
	}
	bar := ui.Row(c).Absolute().Left(20).Right(right).Bottom(18).Gap(10).Padding(8, 10, 8, 12).Radius(24).AlignItems(ui.Center).TextColor(onGlass).Opacity(fade)
	glassFace(bar, ui.RGBA(30, 30, 34, 0.5), 28, 0.22).Shadow(0, 10, 30, 0, ui.RGBA(0, 0, 0, 0.3))
	if !shown {
		bar.PassThrough()
	}
	bar.Children(func() {
		ui.Row(c).Gap(2).AlignItems(ui.Center).Shrink(0).Children(func() {
			c.Key("film-back5")
			if breeze.FlatIconButton(c, iconBack5, "后退 5 秒").Tooltip("后退 5 秒 " + breeze.ShortcutText(c, 0, ui.KeyLeft)).Clicked() {
				s.Skip(-5 * time.Second)
			}
			s.playButton(c)
			c.Key("film-forward5")
			if breeze.FlatIconButton(c, iconForward5, "前进 5 秒").Tooltip("前进 5 秒 " + breeze.ShortcutText(c, 0, ui.KeyRight)).Clicked() {
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
		ui.Row(c).Gap(2).AlignItems(ui.Center).Shrink(0).Children(func() {
			s.modeMenu(c, t)
			c.Key("film-panel")
			on := s.Panel()
			if breeze.FlatToggle(c, &on, iconPanel, "侧栏").Tooltip("侧栏 " + breeze.ShortcutText(c, ui.Shift|ui.Cmd, ui.KeyT)).Changed() {
				s.TogglePanel()
			}
			c.Key("film-full")
			if breeze.FlatIconButton(c, iconFullScreen, "全屏").Tooltip("全屏 F").Clicked() {
				s.ToggleFullScreen()
			}
		})
	})
}

// playButton starts or holds the film. While the video opens it waits, named for that, so that 播放 shows only once there is a picture to play.
func (s screen) playButton(c *ui.Context) {
	c.Key("film-play")
	switch {
	case s.Play() == nil:
		breeze.FlatIconButton(c, iconPlay, "正在打开").Disabled(true)
	case s.Play().Playing():
		if breeze.FlatIconButton(c, iconPause, "暂停").Tooltip("暂停 空格").Clicked() {
			s.togglePlay(c)
		}
	default:
		if breeze.FlatIconButton(c, iconPlay, "播放").Tooltip("播放 空格").Clicked() {
			s.togglePlay(c)
		}
	}
}
