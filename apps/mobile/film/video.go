package film

import (
	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/store"
	"kando/packages/sync"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/icons/lucide"
	"github.com/nsgo-dev/nsgo/ui"
)

// VideoPrompt is what shows where the video would of film f of desktop's library whose lines the phone studies without the picture: one its computer keeps no video of (sync.Film.SubtitlesOnly), or one streamed away from the computer's network from a storage that holds only its lines. It says that the video is not on the computer, with the button that has the computer download it where it may; that the computer is downloading it; that the video is on the computer, which the phone downloads on its network; or, on that network once the computer has it, the button that downloads it to the phone, with where that download stands. testID names it; the buttons' IDs follow it.
func VideoPrompt(cx *core.Scope, s *session.Session, desktop string, f sync.Film, testID string) core.Node {
	colors := ui.ColorsOf(cx)
	key := store.Key(desktop, f.ID)
	computer := func() string { d, _ := s.Desktop(desktop); return d.Name }
	// listed is the film as its computer lists it now, which says once it has the video; as the phone has it until then.
	listed := func() (sync.Film, string) {
		if e, ok := s.Listed(key); ok {
			return e.Film, e.Library.Name
		}
		return f, ""
	}
	// onComputer is a video the computer has; ready one the phone can download now, on the computer's network rather than through the storage.
	onComputer := func() bool { got, _ := listed(); return !got.SubtitlesOnly }
	ready := func() bool { return onComputer() && s.Client(desktop) != nil && !s.Queue(desktop).Relayed }
	coming := func() bool { return s.VideoComing(desktop, f.ID) }
	// phase is which action the prompt offers, which changes far less often than a download's progress.
	phase := core.NewMemo(cx, func() videoPhase {
		switch {
		case ready():
			// A download paused or failed offers to go on with it.
			if t, moving := s.Transfer(key); moving && t.State == session.Running {
				return videoDownloading
			}
			return videoReady
		case onComputer() || coming() || !s.Queue(desktop).Can(sync.OpFilmVideo):
			return videoWaiting
		}
		return videoAsk
	})
	text := func(say func() string, style core.Style, color func() core.Color, id string) core.Node {
		return core.Text{TestID: id, ContentFunc: say, LiveRegion: core.LiveRegionPolite, Style: style.Merge(core.Style{TextAlign: core.TextAlignCenter}), StyleFunc: ui.Colored(color)}
	}
	return core.View{TestID: testID, Style: core.Style{AlignItems: core.AlignCenter, JustifyContent: core.JustifyCenter, Gap: 10, Padding: core.All(20)}, Children: []core.Node{
		ui.IconOf(lucide.Captions, 30, colors.Muted),
		text(func() string { return locales.FilmSubtitlesOnly(cx) }, look.Title, colors.Text, testID+"-title"),
		text(func() string {
			switch {
			case ready():
				return locales.FilmVideoReady(cx, computer())
			case onComputer():
				return locales.FilmVideoAtHome(cx, computer())
			case coming():
				return locales.FilmVideoComing(cx, computer())
			}
			return locales.FilmNoVideo(cx, computer())
		}, look.Subhead, colors.Muted, testID+"-status"),
		core.Dynamic(func(cx *core.Scope) core.Node {
			var button core.Node
			switch phase.Get() {
			case videoDownloading:
				return text(func() string { return look.Status(cx, s, key) }, look.Subhead, colors.Accent, testID+"-progress")
			case videoReady:
				button = look.Button{TestID: testID + "-download", Kind: look.Tinted, Icon: lucide.Download, Text: func() string { return locales.FilmDownloadVideo(cx) },
					OnPress: func() { got, lib := listed(); s.Download(desktop, lib, got) }}
			case videoAsk:
				button = look.Button{TestID: testID + "-ask", Kind: look.Tinted, Icon: lucide.Download, Text: func() string { return locales.FilmVideoOnComputer(cx, computer()) },
					Busy: func() bool { return s.FilmBusy(desktop, f.ID) }, OnPress: func() { s.DownloadVideoOnComputer(desktop, f) }}
			default:
				return core.View{}
			}
			return core.View{Style: core.Style{Width: core.Pct(100), MaxWidth: core.Pt(320), Padding: core.Edges{Top: 4}}, Children: []core.Node{button}}
		}),
	}}
}

// videoPhase is what VideoPrompt offers.
type videoPhase uint8

const (
	// videoAsk offers to have the computer download the video.
	videoAsk videoPhase = iota
	// videoWaiting offers nothing: the computer has it while the phone is away from its network, is downloading it, or does not take the phone's asking.
	videoWaiting
	// videoReady offers to download to the phone the video the computer has now, and videoDownloading says how far that download is.
	videoReady
	videoDownloading
)
