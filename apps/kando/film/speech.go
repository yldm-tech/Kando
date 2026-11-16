package film

import (
	"slices"

	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/ui"
)

// speechLanguage is the info tab's spoken language of a done film: a drop-down of the languages the speech recognition can be told to hear, showing the one heard in the film, spoken, until the user names another (SetFilmSpeechLanguage), and, while the one named is not the one heard, a button that transcribes the film again in it, as Process Again does. Naming a language changes nothing the info tab is built from, so the drop-down keeps the focus; spoken is, so the tab is built again once the film was heard anew.
func (s *screen) speechLanguage(cx *ns.Scope, spoken string) ns.Node {
	a := s.a
	codes := media.SpeechLanguages()
	// A language whisper detected that the settings do not offer is still the film's, so it is one to go back to.
	if spoken != "" && !slices.Contains(codes, spoken) {
		codes = append(slices.Clone(codes), spoken)
	}
	options := make([]ui.Option[string], len(codes))
	for i, code := range codes {
		options[i] = ui.Option[string]{Value: code, Label: subtitle.SpokenLanguageName(cx, code)}
	}
	named := live.Memo(cx, a, func() string { return s.item().SpeechLanguage })
	value := func() string {
		if code := named(); code != "" {
			return code
		}
		return spoken
	}
	choose := ui.Select[string]{TestID: "film-info-speech-language", Label: locales.FilmSpokenLanguage(cx), Options: options, Value: value,
		Placeholder: locales.FilmSpokenLanguageUnknown(cx), OnChange: live.On(a, a.SetFilmSpeechLanguage), Style: ns.Style{Width: ns.Pct(100)}}
	again := ns.Show{When: func() bool { return named() != "" && named() != spoken }, Then: func(cx *ns.Scope) ns.Node {
		// The button names the language, which changes only as another is named.
		return ns.Dynamic(func(cx *ns.Scope) ns.Node {
			return ns.View{Style: ns.Style{AlignSelf: ns.AlignStart}, Children: []ns.Node{
				ui.Button{TestID: "film-info-transcribe-again", Text: locales.FilmTranscribeAgain(cx, subtitle.SpokenLanguageName(cx, named())), Variant: ui.ButtonSecondary, Size: ui.SizeSmall,
					OnPress: live.Do(a, a.ReprocessFilm)},
			}}
		})
	}}
	return ns.View{Style: ns.Style{Padding: ns.Symmetric(10, 12), Gap: 8}, Children: []ns.Node{choose, again}}
}
