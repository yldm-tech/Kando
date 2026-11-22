package settings

import (
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"
	kcore "kando/packages/core"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// online is the 在线视频 card of 通用: whether a link's film takes the subtitles its uploader put on the site as its original lines, checked against the speech, the picture its download asks for (仅字幕 keeps none, and says so under it), and whether films are cut at the chapters their site marks.
type online struct{ s screen }

// Render implements core.Node.
func (o online) Render(cx *core.Scope) core.Node {
	a := o.s.a
	platform := form.SwitchRow{Title: locales.SettingsGeneralPlatformSubtitles(cx), Subtitle: locales.SettingsGeneralPlatformSubtitlesHint(cx), TestID: "settings-platform-subtitles",
		On: live.Func(a, a.PlatformSubtitles),
		OnChange: live.On(a, func(on bool) {
			a.SetPlatformSubtitles(on)
			a.Save()
		})}
	quality := ui.Select[kcore.Quality]{
		Options: []ui.Option[kcore.Quality]{
			{Value: kcore.QualityBest, LabelFunc: func() string { return locales.SettingsGeneralQualityBest(cx) }},
			{Value: kcore.Quality720, LabelFunc: func() string { return locales.SettingsGeneralQuality720(cx) }},
			{Value: kcore.QualitySubtitles, LabelFunc: func() string { return locales.SettingsGeneralQualitySubtitles(cx) }},
		},
		LabelFunc: func() string { return locales.SettingsGeneralQuality(cx) }, TestID: "settings-download-quality",
		Value: live.Func(a, a.DownloadQuality),
		OnChange: live.On(a, func(q kcore.Quality) {
			if q != a.DownloadQuality() {
				a.SetDownloadQuality(q)
				a.Save()
			}
		}),
		Style: core.Style{Width: core.Pt(140)}}
	chapters := form.SwitchRow{Title: locales.SettingsGeneralChapters(cx), Subtitle: locales.SettingsGeneralChaptersHint(cx), TestID: "settings-import-chapters",
		On: live.Func(a, a.ImportChapters),
		OnChange: live.On(a, func(on bool) {
			a.SetImportChapters(on)
			a.Save()
		})}
	return core.Group([]core.Node{
		form.Label(locales.SettingsGeneralOnline(cx), 14),
		form.Card(platform, form.Separator(true), form.Row{Title: locales.SettingsGeneralQuality(cx), NoGroup: true, Trailing: []core.Node{quality}, SubtitleFunc: live.Func(a, func() string {
			// 仅字幕 keeps no video, which the user is told before a download goes without it, and how the film is watched then: streamed where this computer streams it, or after downloading its video (media.Streams).
			switch {
			case a.DownloadQuality() != kcore.QualitySubtitles:
				return ""
			case media.StreamsHere():
				return locales.SettingsGeneralQualitySubtitlesHint(cx)
			default:
				return locales.SettingsGeneralQualitySubtitlesHintNoStream(cx)
			}
		})}, form.Separator(true), chapters),
	})
}
