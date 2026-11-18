package settings

import (
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings/form"
	kcore "kando/packages/core"

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
	quality := core.Dynamic(func(cx *core.Scope) core.Node {
		// TODO(nsgo#145): give ui.Select LabelFunc twins for its name and its options' labels so they follow the interface language without building the drop-down again, then drop this Dynamic https://github.com/nsgo-dev/nsgo/issues/145
		options := []ui.Option[kcore.Quality]{
			{Value: kcore.QualityBest, Label: locales.SettingsGeneralQualityBest(cx)},
			{Value: kcore.Quality720, Label: locales.SettingsGeneralQuality720(cx)},
			{Value: kcore.QualitySubtitles, Label: locales.SettingsGeneralQualitySubtitles(cx)},
		}
		return ui.Select[kcore.Quality]{Options: options, Label: locales.SettingsGeneralQuality(cx), TestID: "settings-download-quality",
			Value: live.Func(a, a.DownloadQuality),
			OnChange: live.On(a, func(q kcore.Quality) {
				if q != a.DownloadQuality() {
					a.SetDownloadQuality(q)
					a.Save()
				}
			}),
			Style: core.Style{Width: core.Pt(140)}}
	})
	chapters := form.SwitchRow{Title: locales.SettingsGeneralChapters(cx), Subtitle: locales.SettingsGeneralChaptersHint(cx), TestID: "settings-import-chapters",
		On: live.Func(a, a.ImportChapters),
		OnChange: live.On(a, func(on bool) {
			a.SetImportChapters(on)
			a.Save()
		})}
	return core.Group([]core.Node{
		form.Label(locales.SettingsGeneralOnline(cx), 14),
		form.Card(platform, form.Separator(true), form.Row{Title: locales.SettingsGeneralQuality(cx), NoGroup: true, Trailing: []core.Node{quality}, SubtitleFunc: live.Func(a, func() string {
			// 仅字幕 keeps no video, which the user is told before a download goes without it.
			if a.DownloadQuality() == kcore.QualitySubtitles {
				return locales.SettingsGeneralQualitySubtitlesHint(cx)
			}
			return ""
		})}, form.Separator(true), chapters),
	})
}
