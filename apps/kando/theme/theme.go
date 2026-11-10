// Package theme is the window's light and dark themes in Kando's accent.
package theme

import (
	"github.com/egoist/mygo/ui"
	breeze "github.com/yldm-tech/mygo-breeze-ui/ui/breeze"
)

// accent is Kando's blue, used in place of the desktop's accent color.
var accent = ui.Hex("#0A66D8")

// Palette builds the two themes of the platform whose controls breeze draws, both in Kando's accent: light, the window's, which it sets on the window, and dark, the player's and the cheat sheet's, which they build their subtree in with ui.Themed. Every color the app sets on a theme is set here.
func Palette(c *ui.Context) (light, dark *ui.Theme) {
	p := breeze.PlatformOf(c)
	light = breeze.ThemeOf(c, p, false, accent)
	light.Background = ui.Hex("#FFFFFF")
	light.Text = ui.Hex("#1D1D1F")
	// With higher contrast on, the platform's secondary text, which ThemeOf already darkened, stays.
	if !c.Preferences().HighContrast {
		light.TextMuted = ui.Hex("#6A6A70")
	}
	dark = breeze.ThemeOf(c, p, true, accent)
	c.SetTheme(light)
	return light, dark
}
