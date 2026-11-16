// Package look is what the phone app's screens draw alike: the sizes of their text, a row of a list, a film's poster, and how they write a size, a length and a language.
package look

import (
	"fmt"
	"strings"
	"time"

	"kando/apps/mobile/locales"
	"kando/apps/mobile/session"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Text sizes on a phone, after the system's: a row's title, the body, and the line under a title.
var (
	Title   = core.Style{FontSize: 17, LineHeight: 22, FontWeight: core.FontWeightSemibold}
	Body    = core.Style{FontSize: 16, LineHeight: 21}
	Caption = core.Style{FontSize: 13, LineHeight: 18}
	Section = core.Style{FontSize: 13, LineHeight: 18, FontWeight: core.FontWeightSemibold}
)

// Size writes a count of bytes as a phone shows it: 820 KB, 34.5 MB, 1.2 GB.
func Size(bytes int64) string {
	switch {
	case bytes >= 1<<30:
		return fmt.Sprintf("%.1f GB", float64(bytes)/(1<<30))
	case bytes >= 1<<20:
		return fmt.Sprintf("%.1f MB", float64(bytes)/(1<<20))
	}
	return fmt.Sprintf("%d KB", (bytes+1023)/1024)
}

// Duration writes how long a film plays, as 4:05 or 1:02:03.
func Duration(d time.Duration) string {
	if d < 0 {
		d = 0
	}
	s := int64(d / time.Second)
	if s >= 3600 {
		return fmt.Sprintf("%d:%02d:%02d", s/3600, s/60%60, s%60)
	}
	return fmt.Sprintf("%d:%02d", s/60, s%60)
}

// autonyms are the languages speech recognition hears, by whisper's codes, in their own names, as a language list names them for anyone to find their own; they are names, not words of the interface.
var autonyms = map[string]string{
	"en": "English", "zh": "中文", "ja": "日本語", "ko": "한국어", "fr": "Français", "es": "Español", "de": "Deutsch",
	"pt": "Português", "it": "Italiano", "ru": "Русский", "nl": "Nederlands", "pl": "Polski", "uk": "Українська",
	"tr": "Türkçe", "ar": "العربية", "hi": "हिन्दी", "id": "Bahasa Indonesia", "vi": "Tiếng Việt", "th": "ไทย",
}

// Languages is a film's languages, the one spoken and the one its subtitles are translated into, as 日本語 → 简体中文; the spoken one alone for a film not translated, "" when neither is known. The desktop names the translation in its own name already (sync.Film.Translation).
func Languages(f sync.Film) string {
	spoken := autonyms[f.Spoken]
	if spoken == "" {
		spoken = strings.ToUpper(f.Spoken)
	}
	switch {
	case spoken != "" && f.Translation != "":
		return spoken + " → " + f.Translation
	case spoken != "":
		return spoken
	}
	return f.Translation
}

// Facts is a film's languages, length and size, as a row of the films lists it: 日本語 → 简体中文 · 12:04 · 320.5 MB.
func Facts(f sync.Film) string {
	var parts []string
	if l := Languages(f); l != "" {
		parts = append(parts, l)
	}
	if f.DurationMS > 0 {
		parts = append(parts, Duration(time.Duration(f.DurationMS)*time.Millisecond))
	}
	if f.VideoSize > 0 {
		parts = append(parts, Size(f.VideoSize))
	}
	return strings.Join(parts, " · ")
}

// Poster is a film's poster at width, in the 16:9 of a video, on a tile of the surface's color until it loads or where the film has none.
func Poster(cx *core.Scope, source func() core.ImageSource, width core.Points, testID string) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{TestID: testID, Style: core.Style{Width: core.Pt(width), AspectRatio: 16.0 / 9, CornerRadius: 6, Overflow: core.Hidden, FlexShrink: 0},
		StyleFunc: ui.Filled(colors.Inset),
		Children: []core.Node{core.Image{SourceFunc: source, ResizeMode: core.Cover, ElementsHidden: true,
			Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0)}}}}
}

// Row is a row of a list: a press target the width of the list with a leading view, a title, a line under it and a trailing view, each optional but the title.
type Row struct {
	Leading  core.Node
	Title    func() string
	Detail   func() string
	Trailing core.Node
	OnPress  func()
	TestID   string
}

// Render implements core.Node.
func (r Row) Render(cx *core.Scope) core.Node {
	colors := ui.ColorsOf(cx)
	texts := []core.Node{core.Text{ContentFunc: r.Title, Style: Body.Merge(core.Style{FontWeight: core.FontWeightMedium}), StyleFunc: ui.Colored(colors.Text)}}
	if r.Detail != nil {
		texts = append(texts, core.Show{When: func() bool { return r.Detail() != "" }, Then: func(cx *core.Scope) core.Node {
			return core.Text{ContentFunc: r.Detail, Style: Caption, StyleFunc: ui.Colored(colors.Muted)}
		}})
	}
	children := []core.Node{}
	if r.Leading != nil {
		children = append(children, r.Leading)
	}
	children = append(children, core.View{Style: core.Style{FlexGrow: 1, FlexShrink: 1, MinWidth: core.Pt(0), Gap: 2}, Children: texts})
	if r.Trailing != nil {
		children = append(children, r.Trailing)
	}
	v := core.View{TestID: r.TestID, Children: children,
		Style:     core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Gap: 12, Padding: core.Symmetric(10, 16), MinHeight: core.Pt(52)},
		StyleFunc: func() core.Style { return core.Style{Active: &core.Style{Background: colors.Pressed()}} }}
	if r.OnPress != nil {
		v.Role = core.RoleButton
		v.OnPress = func(core.PressEvent) { r.OnPress() }
	}
	return v
}

// SectionTitle is the heading over a group of rows, with an optional note at its end, as the space the downloads use.
func SectionTitle(cx *core.Scope, title func() string, note func() string, testID string) core.Node {
	colors := ui.ColorsOf(cx)
	children := []core.Node{core.Text{ContentFunc: title, Role: core.RoleHeader, Style: Section.Merge(core.Style{FlexGrow: 1, FlexShrink: 1}), StyleFunc: ui.Colored(colors.Muted)}}
	if note != nil {
		children = append(children, core.Text{ContentFunc: note, Style: Caption, StyleFunc: ui.Colored(colors.Muted)})
	}
	return core.View{TestID: testID, Style: core.Style{Direction: core.Row, AlignItems: core.AlignCenter, Padding: core.Edges{Top: 20, Right: 16, Bottom: 6, Left: 16}}, Children: children}
}

// Status is where the film under key stands on the phone, as its rows and its page say it: downloading or paused at a share of its video, why its download stopped, or downloaded; "" for a film not on the phone.
func Status(cx *core.Scope, s *session.Session, key string) string {
	if t, ok := s.Transfer(key); ok {
		percent := int(t.Fraction() * 100)
		switch t.State {
		case session.Running:
			return locales.FilmDownloading(cx, percent)
		case session.Paused:
			return locales.FilmPaused(cx, percent)
		}
		return ProblemText(cx, t.Problem)
	}
	if f, ok := s.Film(key); ok && f.Done() {
		return locales.FilmDownloaded(cx)
	}
	return ""
}

// ProblemText is why a download stopped, as the film's row and page say it.
func ProblemText(cx *core.Scope, p session.Problem) string {
	switch p {
	case session.NoSpace:
		return locales.FilmNoSpace(cx)
	case session.Gone:
		return locales.FilmGone(cx)
	case session.Unpaired, session.Certificate:
		return locales.FilmUnpaired(cx)
	}
	return locales.FilmStopped(cx)
}
