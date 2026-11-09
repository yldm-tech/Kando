package locales

import "slices"

// Interface are the languages Kando's interface is written in, the tags of its catalogs, in the order Settings › 通用 lists them. Source is the one the others fall back to; Default is the one Kando shows until the user picks another.
var Interface = []string{"en", "zh-Hans", "zh-Hant", "ja", "ko"}

const (
	Source  = "en"
	Default = "zh-Hans"
)

// Supported reports whether tag is one of Interface.
func Supported(tag string) bool { return slices.Contains(Interface, tag) }
