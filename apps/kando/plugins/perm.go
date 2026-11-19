package plugins

import "slices"

// Perm is a permission a script plugin asks for: the name of the host handler it calls (core.Host), which is the name it has in the package's signed capabilities (pkg.Capabilities.Host) and in the grant the user gave (Grant.Perms). docs/plugin-system.md 5.3.
type Perm string

// The permissions this version has a handler for. A plugin that asks for another is refused (ErrUnknownPerm): kando.llm.chat, kando.sound.read, kando.glossary.suggest, kando.candidates.offer and kando.net.credentialFetch come with their phase.
const (
	// PermFilmCurrent reads the film that is open: its id for this plugin, title, spoken language, the language it is translated into and where it plays.
	PermFilmCurrent Perm = "kando.film.current"
	// PermLineCurrent reads the line that is playing: its index, original and translation.
	PermLineCurrent Perm = "kando.line.current"
	// PermLinesSeen reads the lines of the open film up to the one playing, never past it.
	PermLinesSeen Perm = "kando.lines.seen"
	// PermSavedRead reads the saved lines, without a file path.
	PermSavedRead Perm = "kando.saved.read"
	// PermWordsRead reads the words of the vocabulary.
	PermWordsRead Perm = "kando.words.read"
	// PermToast shows a short message at the bottom of the window.
	PermToast Perm = "kando.toast"
	// PermOpenURL opens an https page in the browser, once the user agreed to each.
	PermOpenURL Perm = "kando.ui.openURL"
	// PermSettingsGet reads the plugin's own settings, which needs no grant: a plugin that declares it always has it.
	PermSettingsGet Perm = "kando.settings.get"
)

// Perms are the permissions this version has, in the order the install confirmation and the details list them.
var Perms = []Perm{PermFilmCurrent, PermLineCurrent, PermLinesSeen, PermSavedRead, PermWordsRead, PermToast, PermOpenURL, PermSettingsGet}

// Known reports whether p is a permission this version has.
func (p Perm) Known() bool { return slices.Contains(Perms, p) }

// Free reports whether p needs no grant: a plugin has it as soon as it declares it.
func (p Perm) Free() bool { return p == PermSettingsGet }

// Library reports whether p reads the library, which no plugin may while it is locked.
func (p Perm) Library() bool {
	switch p {
	case PermFilmCurrent, PermLineCurrent, PermLinesSeen, PermSavedRead, PermWordsRead:
		return true
	}
	return false
}

// Subtitles reports whether p reads the words of the lines, which someone else may have written: a plugin that does and may also reach the network can be led by them to send what it reads (docs/plugin-system.md 6.2), which Risky flags.
func (p Perm) Subtitles() bool { return p == PermLineCurrent || p == PermLinesSeen }

// Risky reports whether g together reads the lines' words and reaches the network, the combination the install confirmation and the details flag on a line of its own.
func Risky(g Grant) bool {
	return len(g.Network) > 0 && slices.ContainsFunc(g.Perms, Perm.Subtitles)
}

// Events are the events of the app a script plugin may hear (Manifest.Events), each with scalars alone (docs/plugin-system.md 5.3).
var Events = []string{"film.opened", "job.completed", "llm.changed"}
