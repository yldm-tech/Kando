// Package sectionid names the sections of the settings, as App.OpenSettings in the main package takes them. It imports nothing, so a page that sends the user to a section, as the player sends a blocked upload to 账号与上传, links to it without importing package settings, which may import the player's look (package subtitle). The settings package's Section constants are these values.
package sectionid

const (
	General   = "general"
	Speech    = "speech"
	AI        = "ai"
	Translate = "translate"
	Look      = "look"
	Shortcuts = "shortcuts"
	Account   = "account"
)
