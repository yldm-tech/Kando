package theme

import (
	"strings"
	"testing"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/i18n"
	"github.com/nsgo-dev/nsgo/nsgotest"
)

// The design rounds sheets and cards 20 and 14 on macOS, 8 on Windows and 12 on Linux; another system takes macOS's.
func TestRadiiPerPlatform(t *testing.T) {
	for _, c := range []struct {
		os   core.OS
		want Radii
	}{
		{core.OSMacOS, Radii{Dialog: 20, Card: 14}},
		{core.OSWindows, Radii{Dialog: 8, Card: 8}},
		{core.OSLinux, Radii{Dialog: 12, Card: 12}},
		{core.OSIOS, Radii{Dialog: 20, Card: 14}},
	} {
		if got := RadiiFor(c.os); got != c.want {
			t.Errorf("RadiiFor(%v) = %+v, want %+v", c.os, got, c.want)
		}
	}
}

// macOS keeps its system font, which is the design's; Windows and Linux lead with Segoe UI Variable and Cantarell and take the CJK family of the interface's language, never an Apple-only one.
func TestFontPerPlatformAndLanguage(t *testing.T) {
	if got := FontFor(core.OSMacOS, "zh-Hans"); got != "" {
		t.Errorf("macOS font %q, want the system's", got)
	}
	for _, c := range []struct {
		os         core.OS
		lang, lead string
		cjk        string
	}{
		{core.OSWindows, "zh-Hans", "Segoe UI Variable Text", "Microsoft YaHei UI"},
		{core.OSWindows, "zh-Hant", "Segoe UI Variable Text", "Microsoft JhengHei UI"},
		{core.OSWindows, "ja", "Segoe UI Variable Text", "Yu Gothic UI"},
		{core.OSWindows, "ko", "Segoe UI Variable Text", "Malgun Gothic"},
		{core.OSWindows, "en", "Segoe UI Variable Text", "Microsoft YaHei UI"},
		{core.OSLinux, "zh-Hans", "Cantarell", "Noto Sans CJK SC"},
		{core.OSLinux, "zh-Hant", "Cantarell", "Noto Sans CJK TC"},
		{core.OSLinux, "ja", "Cantarell", "Noto Sans CJK JP"},
		{core.OSLinux, "ko", "Cantarell", "Noto Sans CJK KR"},
	} {
		got := FontFor(c.os, c.lang)
		if !strings.HasPrefix(got, c.lead+",") || !strings.Contains(got, c.cjk) {
			t.Errorf("FontFor(%v, %s) = %q, want %s first and %s", c.os, c.lang, got, c.lead, c.cjk)
		}
		for _, apple := range []string{"PingFang", "Hiragino", "SF Pro", "apple-system"} {
			if strings.Contains(got, apple) {
				t.Errorf("FontFor(%v, %s) = %q names Apple's %s", c.os, c.lang, got, apple)
			}
		}
	}
}

// FontOf and RadiiOf read the system the app runs on.
func TestTokensFollowTheSystem(t *testing.T) {
	var font, lang string
	var radii Radii
	nsgotest.Start(t, func(cx *core.Scope) core.Node {
		font, lang, radii = FontOf(cx), i18n.Language(cx), RadiiOf(cx)
		return nil
	}, nsgotest.Environment(core.Environment{OS: core.OSLinux}))
	if font != FontFor(core.OSLinux, lang) || radii != RadiiFor(core.OSLinux) {
		t.Errorf("on Linux in %s: font %q, radii %+v", lang, font, radii)
	}
}
