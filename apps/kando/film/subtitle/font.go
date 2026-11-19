package subtitle

import (
	"kando/apps/kando/locales"

	"github.com/nsgo-dev/nsgo/core"
)

// The font families a face can have, as the settings save them. They are data, not text: FontName names them in the interface's language. 默认 leaves the line in the system's font; 苹方 and 冬青黑体 are macOS's own Chinese fonts, the ones the style controls offer there; 黑体 and 宋体 are a sans-serif and a serif Chinese font, the ones they offer on Windows and Linux.
const (
	FontDefault  = "默认"
	FontPingFang = "苹方"
	FontHiragino = "冬青黑体"
	FontSans     = "黑体"
	FontSerif    = "宋体"
)

// fontFamilies are the fonts that draw each saved family, a CSS list for a style's FontFamily: the family's own font first, then a font of the same kind on each system (macOS, Windows, Linux), so a family saved on one system still draws in its kind on another, and on a Linux without the first a common substitute does.
var fontFamilies = map[string]string{
	FontPingFang: "PingFang SC, Noto Sans CJK SC, Source Han Sans SC, Microsoft YaHei",
	FontHiragino: "Hiragino Sans GB, Noto Sans CJK SC, Source Han Sans SC, Microsoft YaHei",
	FontSans:     "Microsoft YaHei, Noto Sans CJK SC, Source Han Sans SC, WenQuanYi Micro Hei, PingFang SC",
	FontSerif:    "SimSun, Noto Serif CJK SC, Source Han Serif SC, AR PL UMing CN, Songti SC",
}

// systemFont names the system's font in a style's FontFamily. An empty FontFamily would not do: it inherits the family of the views around the line, and the windows set the design's interface font (theme.FontOf), Segoe UI Variable or Cantarell, which is not what 默认 means. NSGo draws system-ui in the system's interface font on every desktop, GTK's own (gtk-font-name) on Linux.
const systemFont = "system-ui"

// FontFamily is the font list that draws the saved family name, or the system's font for 默认 and a family it does not know. The player and the settings' preview draw lines with it.
func FontFamily(name string) string {
	if f, ok := fontFamilies[name]; ok {
		return f
	}
	return systemFont
}

// Fonts are the families the style controls offer on os, as the settings save them: macOS's own fonts on macOS, as the prototype has them, and elsewhere a sans-serif and a serif font, which look apart on every system that has Chinese fonts.
func Fonts(os core.OS) []string {
	if os == core.OSMacOS {
		return []string{FontDefault, FontPingFang, FontHiragino}
	}
	return []string{FontDefault, FontSans, FontSerif}
}

// FontName is what the style controls call a saved font family in the language the interface shows under cx; a family it does not know is shown as saved.
func FontName(cx *core.Scope, family string) string { return FontNameIn(locales.For(cx), family) }

// FontNameIn is FontName in the language of m.
func FontNameIn(m locales.Messages, family string) string {
	switch family {
	case FontDefault:
		return m.FilmFontDefault()
	case FontPingFang:
		return m.FilmFontPingFang()
	case FontHiragino:
		return m.FilmFontHiragino()
	case FontSans:
		return m.FilmFontSans()
	case FontSerif:
		return m.FilmFontSerif()
	}
	return family
}
