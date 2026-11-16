// Package locales says the phone app's interface in the language the phone shows. The catalogs are the YAML files in this directory, a file per language (en.yaml is the source, the others translate it), and catalog_gen.go is the code go tool nsgo writes from them: Xxx(cx, ...) says a message in the language the app shows under cx, In(tag).Xxx(...) in a language of the caller's choice. The app follows the system's language among Interface.
package locales

// Interface are the languages the app's interface is written in, the tags of its catalogs, as the desktop app's (kando/apps/kando/locales).
var Interface = []string{"en", "zh-Hans", "zh-Hant", "ja", "ko"}
