// Package look is what the phone app's screens draw alike: the sizes of their text, a row of a list and the card it sits in, a film's poster, the buttons, an empty page, a tab's root page and the bar of a page's actions, and how they write a size, a length and a language.
package look

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"kando/apps/mobile/locales"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"
	kcore "kando/packages/core"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// Heavy is the weight of the design's large titles and sheet titles, between bold and black.
const Heavy core.FontWeight = 800

// Text sizes on a phone, after the design's: a page's large title (PageTitle picks Android's where it runs), a section's heading (SectionHeading picks Android's), a row's or a sheet's title, the body, the line under a title, a caption, a field's label and a note under a row.
var (
	Display        = core.Style{FontSize: 34, LineHeight: 41, FontWeight: Heavy, LetterSpacing: -0.6}
	DisplayAndroid = core.Style{FontSize: 28, LineHeight: 36, FontWeight: core.FontWeightMedium}
	Heading        = core.Style{FontSize: 22, LineHeight: 28, FontWeight: core.FontWeightBold, LetterSpacing: -0.3}
	HeadingAndroid = core.Style{FontSize: 20, LineHeight: 28, FontWeight: core.FontWeightMedium}
	Title          = core.Style{FontSize: 17, LineHeight: 22, FontWeight: core.FontWeightSemibold}
	Body           = core.Style{FontSize: 16, LineHeight: 21}
	Subhead        = core.Style{FontSize: 15, LineHeight: 20}
	Caption        = core.Style{FontSize: 13, LineHeight: 18}
	Section        = core.Style{FontSize: 13, LineHeight: 18, FontWeight: core.FontWeightSemibold}
	Footnote       = core.Style{FontSize: 12, LineHeight: 16}
)

// PageTitle is the large title of a tab's page: the design's 34pt heavy title, or Material's 28pt medium one on Android.
func PageTitle(cx *core.Scope) core.Style {
	if cx.Env().OS() == core.OSAndroid {
		return DisplayAndroid
	}
	return Display
}

// SectionHeading is the heading of a section of a page's content: the design's 22pt bold, or Material's 20pt medium on Android.
func SectionHeading(cx *core.Scope) core.Style {
	if cx.Env().OS() == core.OSAndroid {
		return HeadingAndroid
	}
	return Heading
}

// SubPageTitle is the title of a page a tab's page opens in Android's top app bar: Material's title large, 22pt regular.
var SubPageTitle = core.Style{FontSize: 22, LineHeight: 28, FontWeight: core.FontWeightNormal}

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

// Poster is a film's poster at width, in the 16:9 of a video, on a tile of the surface's color until it loads or where the film has none, rimmed so its edge shows on a dark page too.
func Poster(cx *core.Scope, source func() core.ImageSource, width core.Points, testID string) core.Node {
	colors := ui.ColorsOf(cx)
	return core.View{TestID: testID, Style: core.Style{Width: core.Pt(width), AspectRatio: 16.0 / 9, CornerRadius: 8, BorderWidth: 1, Overflow: core.Hidden, FlexShrink: 0},
		StyleFunc: func() core.Style {
			return core.Style{Background: colors.Inset(), BorderColor: ui.Alpha(colors.Text(), 0.08)}
		},
		Children: []core.Node{core.Image{SourceFunc: source, ResizeMode: core.Cover, ElementsHidden: true,
			Style: core.Style{Position: core.Absolute, Left: core.Pt(0), Top: core.Pt(0), Right: core.Pt(0), Bottom: core.Pt(0)}}}}
}

// LocalPoster is the poster of a film kept on the phone, none until it is downloaded.
func LocalPoster(s *session.Session, f store.Film) core.ImageSource {
	if f.Poster == "" {
		return core.ImageSource{}
	}
	return core.Src(kcore.FileURL(filepath.Join(s.Store().FilmDir(f.Desktop, f.Film.ID), f.Poster)))
}

// FilmPoster is the poster of the film kept under key, as its page and its row in the Library show it: the file the phone keeps, else, for a film streamed from its computer without one, the poster its computer gives, read once and kept by the session (session.Poster); none while neither is had.
func FilmPoster(cx *core.Scope, s *session.Session, key func() string) func() core.ImageSource {
	type source struct {
		c           *sync.Client
		desktop, id string
	}
	remote := core.NewResource(cx, func() (source, bool) {
		f, kept := s.Film(key())
		c := s.Client(f.Desktop)
		return source{c, f.Desktop, f.Film.ID}, kept && c != nil && f.Film.Poster != "" && f.Poster == ""
	}, func(ctx context.Context, src source, _ core.ResourceInfo) ([]byte, error) {
		return s.Poster(ctx, src.c, src.desktop, src.id)
	})
	return func() core.ImageSource {
		if f, ok := s.Film(key()); ok && f.Poster != "" {
			return LocalPoster(s, f)
		}
		if data, ok := remote.Get(); ok && len(data) > 0 {
			return core.ImageSource{Data: data}
		}
		return core.ImageSource{}
	}
}

// Status is where the film under key stands on the phone, as its rows and its page say it: downloading or paused at a share of its video, why its download stopped, downloaded, or its lines kept for a film kept without its video; for a film not on the phone, that its computer no longer serves it once a download of it found so (session.FilmGone), else "". Of a film of a source and the computer's film of the same file, it is where the copy that stands for both stands (session.Copy).
func Status(cx *core.Scope, s *session.Session, key string) string {
	key = s.Copy(key)
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
		if !f.HasVideo() {
			return locales.FilmLinesSaved(cx)
		}
		return locales.FilmDownloaded(cx)
	}
	if s.FilmGone(key) {
		return locales.FilmGone(cx)
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
