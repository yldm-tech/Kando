package film

import (
	"kando/apps/kando/films/sitename"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	kcore "kando/packages/core"

	"github.com/nsgo-dev/nsgo/base"
	"github.com/nsgo-dev/nsgo/core"
)

// Streamer is what the player asks of the app for a film that keeps no video (Item.SubtitlesOnly, 仅字幕), which streams from its site when it is watched.
type Streamer interface {
	// FilmStreams reports whether the open film's video can stream on this computer, which only a YouTube video on macOS can for now.
	FilmStreams() bool
	// DownloadVideo downloads the open film's video beside its subtitles, so it plays offline; while offline mode is on the download waits until it is off (Item.VideoComing says it is coming).
	DownloadVideo()
	// StreamOnline turns offline mode off and streams the open film.
	StreamOnline()
	// StreamAgain looks the open film's stream up again after it failed.
	StreamAgain()
}

// PlayErr's IDs for a film that keeps no video and does not stream: offline mode is on (ErrStreamOffline), its site or this computer cannot stream it (ErrStreamNone), or looking its stream up or playing it failed (ErrStreamFailed). The page shows streamCard for them rather than the stage's note.
const (
	ErrStreamOffline = "stream-offline"
	ErrStreamNone    = "stream-none"
	ErrStreamFailed  = "stream-failed"
)

// StreamErr reports whether id is one of the PlayErr IDs of a film that does not stream.
func StreamErr(id string) bool {
	return id == ErrStreamOffline || id == ErrStreamNone || id == ErrStreamFailed
}

// siteName names the site a film streams from, as the cards over the picture and in 信息 name it.
func siteName(cx *core.Scope, source string) string {
	link, ok := kcore.ParseLink(source)
	switch {
	case !ok:
		return ""
	case link.Platform == kcore.PlatformYouTube:
		return "YouTube"
	case link.Platform == kcore.PlatformBilibili:
		return locales.SourceBilibili(cx)
	}
	if name := sitename.In(locales.For(cx), link.Site); name != "" {
		return name
	}
	return link.Host
}

// streamGlass is the glass of the card over the picture of a film that does not stream, as the prototype's offline card.
var streamGlass = glassSpec{fill: core.RGBA(0x1C1C20, 0.86), blur: 28, ring: 0.2, radius: 18}

// streamCard is the card over the picture of a film that keeps no video and does not stream now (StreamErr): offline mode is on, its video cannot stream here, or the stream failed. It says why and offers what plays it: turning offline mode off, looking the stream up again, or downloading the video, which says it is scheduled once it is (Item.VideoComing).
func (s *screen) streamCard() core.Node {
	a, cx := s.a, s.cx
	err := live.Memo(cx, a, func() string {
		if id := a.PlayErr(); StreamErr(id) {
			return id
		}
		return ""
	})
	return core.Show{When: func() bool { return err() != "" }, Then: func(cx *core.Scope) core.Node {
		muted := core.RGBA(0xFFFFFF, 0.8)
		site := func() string { return siteName(cx, s.item().Source) }
		title := func() string {
			switch err() {
			case ErrStreamOffline:
				return locales.PlayerStreamOffline(cx)
			case ErrStreamNone:
				return locales.PlayerStreamNone(cx)
			}
			return locales.PlayerStreamFailed(cx)
		}
		detail := func() string {
			switch err() {
			case ErrStreamOffline:
				return locales.PlayerStreamOfflineDetail(cx, site())
			case ErrStreamNone:
				return locales.PlayerStreamNoneDetail(cx, site())
			}
			return locales.PlayerStreamFailedDetail(cx, site())
		}
		coming := live.Func(a, func() bool { return s.item().VideoComing })
		download := func() string {
			switch {
			case coming():
				return locales.PlayerStreamQueued(cx)
			case err() == ErrStreamOffline:
				return locales.PlayerStreamDownloadLater(cx)
			}
			return locales.PlayerStreamDownload(cx)
		}
		downloadButton := disabledWhen(lookupButton(cx, download, "film-stream-download", live.Do(a, a.DownloadVideo), nil), coming)
		buttons := core.View{Style: core.Style{Direction: core.Row, Wrap: core.Wrap, Gap: 8, JustifyContent: core.JustifyCenter, Margin: core.Edges{Top: 4}}, Children: []core.Node{
			core.Show{When: func() bool { return err() == ErrStreamOffline }, Then: func(cx *core.Scope) core.Node {
				return lookupButton(cx, func() string { return locales.PlayerStreamGoOnline(cx) }, "film-stream-online", live.Do(a, a.StreamOnline), nil)
			}},
			core.Show{When: func() bool { return err() == ErrStreamFailed }, Then: func(cx *core.Scope) core.Node {
				return lookupButton(cx, func() string { return locales.PlayerStreamRetry(cx) }, "film-stream-retry", live.Do(a, a.StreamAgain), nil)
			}},
			downloadButton,
		}}
		card := core.View{TestID: "film-stream", Role: core.RoleAlertDialog, LabelFunc: title,
			Style: streamGlass.style().Merge(core.Style{Width: core.Pct(100), MaxWidth: core.Pt(420), Gap: 8, Padding: core.All(18), AlignItems: core.AlignCenter}),
			Children: []core.Node{
				core.Text{TestID: "film-stream-title", ContentFunc: title, Role: core.RoleHeader, Style: core.Style{FontSize: 15, LineHeight: 20, FontWeight: core.FontWeightBold, Color: onGlass, TextAlign: core.TextAlignCenter}},
				core.Text{TestID: "film-stream-detail", ContentFunc: detail, Style: core.Style{FontSize: 12, LineHeight: 18, Color: muted, TextAlign: core.TextAlignCenter}},
				buttons,
			}}
		return core.View{PointerEvents: core.PointerEventsBoxNone,
			Style: core.Style{Position: core.Absolute, Left: core.Pt(24), Top: core.Pt(0), Bottom: core.Pt(0), Direction: core.Row, AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter},
			StyleFunc: func() core.Style {
				return core.Style{Right: core.Pt(s.roomRight() + 24), Transition: roomTransition}
			},
			Children: []core.Node{card}}
	}}
}

// disabledWhen is button, a lookupButton, taking no presses and dimmed while off holds, as the download of a video already scheduled.
func disabledWhen(button core.Node, off func() bool) core.Node {
	b := button.(base.Button)
	b.Disabled = off
	b.Style.Disabled = &core.Style{Opacity: core.Alpha(0.55)}
	return b
}

// streamInfo is the card of 信息 for a film that keeps no video: that it streams when watched, or here has to be downloaded first, and the button that downloads its video, which says it is scheduled once it is. nil for any other film.
func (s *screen) streamInfo(cx *core.Scope, st infoState) core.Node {
	if !st.subtitlesOnly {
		return nil
	}
	a := s.a
	title, text := locales.FilmInfoStreamTitle(cx), locales.FilmInfoStream(cx)
	if !st.streams {
		title, text = locales.FilmsSubtitlesOnly(cx), locales.FilmInfoStreamNone(cx)
	}
	if st.videoComing {
		text = locales.FilmInfoStreamQueued(cx)
	}
	label := func() string {
		if st.videoComing {
			return locales.PlayerStreamQueued(cx)
		}
		return locales.PlayerStreamDownload(cx)
	}
	button := disabledWhen(lookupButton(cx, label, "film-info-download", live.Do(a, a.DownloadVideo), nil), func() bool { return st.videoComing })
	return core.View{TestID: "film-info-stream", Role: core.RoleGroup, Label: title,
		Style: core.Style{CornerRadius: 14, Background: core.RGBA(0x64AAFF, 0.16), Shadows: []core.BoxShadow{{Spread: 0.5, Color: core.RGBA(0x8CBEFF, 0.4), Inset: true}}, Padding: core.Symmetric(10, 12), Gap: 8},
		Children: []core.Node{
			core.Text{Content: title, Style: core.Style{FontSize: 13, LineHeight: 17, FontWeight: core.FontWeightBold, Color: onGlass}},
			core.Text{Content: text, Style: core.Style{FontSize: 12, LineHeight: 17, Color: onGlass, Opacity: core.Alpha(0.85)}},
			core.View{Style: core.Style{AlignSelf: core.AlignStart}, Children: []core.Node{button}},
		}}
}
