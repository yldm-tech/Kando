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

// defaults is the app's own defaults domain alone: a list of strings under a key, nil while unset, and a string, "" while unset. It does not fall through to the global domain as NSUserDefaults' search list does, whose AppleLanguages are the device's languages: read so, the device's languages would pass for a language the user chose for the app, and be put back as one, holding the app to the device's first language even once the device's changes.
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
	// The user's own are languages NSGo did not write. NSGo writes the language it forces alone; iOS Settings writes the language chosen with its region, before the device's others (zh-Hans-JP, ja-JP, en-JP), so even one that starts as NSGo's does is the user's. One the user picks in iOS Settings that is the very one NSGo wrote is not told from it, and goes as NSGo's does.
	users := len(langs) > 0 && (pinned == "" || len(langs) > 1 || langs[0] != pinned)
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
		// NSGo, following the system again, removes AppleLanguages whose first language is the one it wrote, as the user's may be (en-JP is not en, but a list of the user's may start with the very tag): with its record gone, it leaves them be.
		d.remove(nsgoLanguage)
	}
}
