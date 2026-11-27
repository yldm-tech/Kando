// Package syslang keeps the language the user chose for Kando in iOS Settings (Settings › Kando › Language) while the app's own language, picked in its Settings › Language, stands in its place, and gives it back once the app follows the system again.
//
// iOS keeps an app's language as AppleLanguages in the app's own defaults, which NSGo also writes to force the language Scope.SetLocale picks for what the system says in the app from its next launch. Following the system again, NSGo removes AppleLanguages when it holds what NSGo wrote (NSGoLanguage), so a language the user had chosen for the app in iOS Settings before is gone with it, and the app follows the device's language from then on: Automatic changed meaning after picking English and going back.
//
// TODO(nsgo#2711): apple.PinLanguage overwrites the app's own AppleLanguages without keeping what it held, and removes it when it follows the user again (internal/platform/apple/system.go:331-362), so a per-app language set in iOS Settings is lost; once NSGo keeps and gives it back itself, drop this package and its call in App. https://github.com/nsgo-dev/nsgo/issues/2711
package syslang

// The keys of the app's defaults: the languages iOS launches the app in, what NSGo last wrote there, and the user's own that the app keeps while NSGo's stand in their place.
const (
	appleLanguages = "AppleLanguages"
	nsgoLanguage   = "NSGoLanguage"
	keptLanguages  = "KandoSystemLanguages"
)

// defaults is the app's own defaults: a list of strings under a key, nil while unset, and a string, "" while unset.
type defaults interface {
	strings(key string) []string
	string(key string) string
	setStrings(key string, v []string)
	remove(key string)
}

// keep is Keep on d: with lang, the language the app is about to force, it keeps the user's own languages of the app aside while they are what d holds; with "" it puts them back in place of NSGo's, unless the user has chosen others since, so NSGo, following the system again, leaves them be.
func keep(d defaults, lang string) {
	langs := d.strings(appleLanguages)
	pinned := d.string(nsgoLanguage)
	// The user's own are languages NSGo did not write, as iOS Settings sets them.
	users := len(langs) > 0 && (pinned == "" || langs[0] != pinned)
	if lang != "" {
		if users {
			d.setStrings(keptLanguages, langs)
		}
		return
	}
	kept := d.strings(keptLanguages)
	if kept == nil {
		return
	}
	d.remove(keptLanguages)
	if !users && pinned != "" {
		d.setStrings(appleLanguages, kept)
	}
}
