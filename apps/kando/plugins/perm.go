package plugins

import "slices"

// Perm is a permission a script plugin asks for: the name of the host handler it calls (core.Host), which is the name it has in the package's signed capabilities (pkg.Capabilities.Host) and in the grant the user gave (Grant.Perms). docs/plugin-system.md 5.3.
type Perm string

// The permissions this version has a handler for. A plugin that asks for another is refused (ErrUnknownPerm): kando.glossary.suggest has no handler, as Kando has no place yet where a glossary term is proposed for the user to take or leave (docs/plugin-system.md 15, P3b), and kando.candidates.offer none either, as a translator offers other translations of a line through its candidates export instead (Translator, CandidatesExport).
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
	// PermSoundRead reads the sound of a saved line or of the line playing, cut out of its video.
	PermSoundRead Perm = "kando.sound.read"
	// PermLLMChat asks the user's AI service, within the plugin's daily budget of tokens.
	PermLLMChat Perm = "kando.llm.chat"
	// PermCredFetch sends a request with a secret the user gave Kando for the plugin (Manifest.Credentials), which Kando puts in, only to the hosts, methods and paths the credential names.
	PermCredFetch Perm = "kando.net.credentialFetch"
	// PermToast shows a short message at the bottom of the window.
	PermToast Perm = "kando.toast"
	// PermOpenURL opens an https page in the browser, once the user agreed to each.
	PermOpenURL Perm = "kando.ui.openURL"
	// PermSettingsGet reads the plugin's own settings, which needs no grant: a plugin that declares it always has it.
	PermSettingsGet Perm = "kando.settings.get"
)

// Perms are the permissions this version has, in the order the install confirmation and the details list them.
var Perms = []Perm{PermFilmCurrent, PermLineCurrent, PermLinesSeen, PermSavedRead, PermWordsRead, PermSoundRead, PermLLMChat, PermCredFetch, PermToast, PermOpenURL, PermSettingsGet}

// Known reports whether p is a permission this version has.
func (p Perm) Known() bool { return slices.Contains(Perms, p) }

// Free reports whether p needs no grant: a plugin has it as soon as it declares it.
func (p Perm) Free() bool { return p == PermSettingsGet }

// Library reports whether p reads the library, which no plugin may while it is locked.
func (p Perm) Library() bool {
	switch p {
	case PermFilmCurrent, PermLineCurrent, PermLinesSeen, PermSavedRead, PermWordsRead, PermSoundRead:
		return true
	}
	return false
}

// Subtitles reports whether p reads the words of the lines, which someone else may have written: a plugin that does and may also reach the network can be led by them to send what it reads (docs/plugin-system.md 6.2), which Risky flags.
func (p Perm) Subtitles() bool { return p == PermLineCurrent || p == PermLinesSeen }

// Risky reports whether the plugin m, granted g, together takes in what someone else may have written, the words of the lines, by a permission or because its contributions are given them (Manifest.ReadsLines), or the replies of the user's AI service, and reaches out of Kando, by the network or with a credential: the combination the install confirmation and the details flag on a line of its own (docs/plugin-system.md 6.2).
func Risky(m Manifest, g Grant) bool {
	reads := m.ReadsLines() || g.Has(PermLLMChat) || slices.ContainsFunc(g.Perms, Perm.Subtitles)
	sends := len(g.Network) > 0 || g.Has(PermCredFetch)
	return reads && sends
}

// Events are the events of the app a script plugin may hear (Manifest.Events), each with scalars alone (docs/plugin-system.md 5.3).
var Events = []string{"film.opened", "job.completed", "llm.changed"}
