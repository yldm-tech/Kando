// Package locales says Kando's interface in the language it shows. The catalogs are the YAML files in this directory, a file per language (en.yaml is the source, the others translate it), and catalog_gen.go is the code go tool nsgo writes from them: Xxx(cx, ...) says a message in the language the app shows under cx and follows it as it changes, In(tag).Xxx(...) says it in a language of the caller's choice.
package locales

import "slices"

// Interface are the languages Kando's interface is written in, the tags of its catalogs, in the order Settings › 通用 lists them. Source, the catalogs' own (catalog_gen.go), is the one the others fall back to; Default is the one Kando shows until the user picks another.
var Interface = []string{"en", "zh-Hans", "zh-Hant", "ja", "ko"}

const Default = "zh-Hans"

// Supported reports whether tag is one of Interface.
func Supported(tag string) bool { return slices.Contains(Interface, tag) }

// Language is the tag of the catalog msgs say their messages in, one of Interface, for text that comes with the messages from elsewhere, as a plugin pack's names by language (plugins.Text.In).
func (msgs Messages) Language() string { return msgs.lang }
