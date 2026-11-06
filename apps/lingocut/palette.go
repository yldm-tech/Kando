package main

import (
	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// accent is LingoCut's blue, used in place of the desktop's accent color.
var accent = ui.Hex("#0A66D8")

// palette gives the window the theme of the platform whose controls breeze draws, light or dark as the window is, with LingoCut's accent. Every color the app sets on the theme is set here.
func palette(c *ui.Context) *ui.Theme {
	t := breeze.Theme(c, breeze.PlatformOf(c))
	t.Accent = accent
	t.AccentHover = accent
	t.AccentPressed = accent.Mix(ui.RGB(0, 0, 0), 0.18)
	t.AccentText = ui.RGB(255, 255, 255)
	t.Focus = accent.Alpha(0.5)
	c.SetTheme(t)
	return t
}
