package subtitle

import (
	"slices"
	"strings"
	"testing"

	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"

	"github.com/nsgo-dev/nsgo/core"
)

// TestMain reads the interface in the language Kando shows by default, whatever the languages of the machine running the tests.
func TestMain(m *testing.M) {
	testlocale.Pin(locales.Default)
	m.Run()
}

// TestStyleFontsArePlatformsOwn checks that the font menu offers macOS's own fonts on macOS, as the prototype has them, and a sans-serif and a serif font on Windows and Linux, where PingFang and Hiragino are not installed and would draw in the default font.
func TestStyleFontsArePlatformsOwn(t *testing.T) {
	for os, want := range map[core.OS][]string{
		core.OSMacOS:   {"默认", "苹方", "冬青黑体"},
		core.OSWindows: {"默认", "黑体", "宋体"},
		core.OSLinux:   {"默认", "黑体", "宋体"},
	} {
		if got := Fonts(os); !slices.Equal(got, want) {
			t.Errorf("%v offers %q, want %q", os, got, want)
		}
	}
}

// TestFontFamilyHasAFontOnEverySystem checks that each saved family other than 默认 draws in a font of its kind on each system: its list names a font macOS has, one Windows has and one a Linux with Noto CJK has, so a family saved on one system does not fall back to the default font on another, and the menu's choices look apart everywhere.
func TestFontFamilyHasAFontOnEverySystem(t *testing.T) {
	systems := map[string]map[string][]string{
		"sans": {
			"darwin":  {"PingFang SC", "Hiragino Sans GB"},
			"windows": {"Microsoft YaHei"},
			"linux":   {"Noto Sans CJK SC"},
		},
		"serif": {
			"darwin":  {"Songti SC"},
			"windows": {"SimSun"},
			"linux":   {"Noto Serif CJK SC"},
		},
	}
	kinds := map[string]string{"苹方": "sans", "冬青黑体": "sans", "黑体": "sans", "宋体": "serif"}
	for family, kind := range kinds {
		list := strings.Split(FontFamily(family), ", ")
		for goos, fonts := range systems[kind] {
			if !slices.ContainsFunc(fonts, func(f string) bool { return slices.Contains(list, f) }) {
				t.Errorf("%s draws in %q, none of %s's %s fonts %q", family, list, goos, kind, fonts)
			}
		}
	}
	if got := FontFamily("苹方"); !strings.HasPrefix(got, "PingFang SC,") {
		t.Errorf("苹方 draws in %q, want PingFang SC first", got)
	}
	if got := FontFamily("宋体"); strings.Contains(got, "Sans") {
		t.Errorf("宋体 falls back to a sans-serif font: %q", got)
	}
	for _, family := range []string{"默认", "", "Comic Sans"} {
		if got := FontFamily(family); got != "" {
			t.Errorf("%q draws in %q, want the system's font", family, got)
		}
	}
}

// TestFontNamesFollowTheInterface checks that the style view names every saved family, the new ones too, and shows one it does not know as saved.
func TestFontNamesFollowTheInterface(t *testing.T) {
	m := testlocale.Messages()
	for family, want := range map[string]string{"默认": "默认", "苹方": "苹方", "冬青黑体": "冬青黑体", "黑体": "黑体", "宋体": "宋体", "Comic Sans": "Comic Sans"} {
		if got := FontNameIn(m, family); got != want {
			t.Errorf("FontName(%q) = %q, want %q", family, got, want)
		}
	}
}
