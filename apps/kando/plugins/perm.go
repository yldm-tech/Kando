package plugins

import "slices"

// Perm is a permission a script plugin asks for: the name of the host handler it calls (core.Host), which is the name it has in the package's signed capabilities (pkg.Capabilities.Host) and in the grant the user gave (Grant.Perms). docs/plugin-system.md 5.3.
type Perm string

// The permissions this version has a handler for. A plugin that asks for another is refused (ErrUnknownPerm): kando.candidates.offer has no handler, as a translator offers other translations of a line through its candidates export instead (Translator, CandidatesExport).
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
	// PermGlossarySuggest proposes terms for the open film's glossary, which the player's 术语 tab lists for the user to add or dismiss: a proposal alone, never a change of the glossary, nor of a term it holds.
	PermGlossarySuggest Perm = "kando.glossary.suggest"
	// PermLLMChat asks the user's AI service, within the plugin's daily budget of tokens.
	PermLLMChat Perm = "kando.llm.chat"
	// PermCredFetch sends a request with a secret the user gave Kando for the plugin (Manifest.Credentials), which Kando puts in, only to the hosts, methods and paths the credential names.
	PermCredFetch Perm = "kando.net.credentialFetch"
	// PermSourceFetch sends the requests of the plugin's media sources (MediaSource) to the server the user filled in for each source and to no other, with the source's secrets put in by Kando, which the plugin never sees (docs/media-sources.md 11.3).
	PermSourceFetch Perm = "kando.source.fetch"
	// PermToast shows a short message at the bottom of the window.
	PermToast Perm = "kando.toast"
	// PermOpenURL opens an https page in the browser, once the user agreed to each.
	PermOpenURL Perm = "kando.ui.openURL"
	// PermSettingsGet reads the plugin's own settings, which needs no grant: a plugin that declares it always has it.
	PermSettingsGet Perm = "kando.settings.get"
)

// Perms are the permissions this version has, in the order the install confirmation and the details list them.
var Perms = []Perm{PermFilmCurrent, PermLineCurrent, PermLinesSeen, PermSavedRead, PermWordsRead, PermSoundRead, PermGlossarySuggest, PermLLMChat, PermCredFetch, PermSourceFetch, PermToast, PermOpenURL, PermSettingsGet}

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

// Events are the events of the app a script plugin may hear (Manifest.Events), each with scalars alone (docs/plugin-system.md 5.3). line.changed reaches only the view of a panel of the plugin's (Panel) while it shows, and only while the plugin may read the line playing.
var Events = []string{"film.opened", "job.completed", "llm.changed", EventLineChanged}

// EventLineChanged is the event a panel hears as the line playing changes, with the index of the line now playing, -1 for none; the line itself is kando.line.current's to give.
const EventLineChanged = "line.changed"

// PermGroup is the group a permission is listed under on a script plugin's details (docs/plugin-system.md 7.3), in the order Groups gives.
type PermGroup int

const (
	// GroupRead reads the user's content: the film, its lines, the saved lines, the words, their sound, and the plugin's own settings. It holds what no other group claims.
	GroupRead PermGroup = iota
	// GroupChange changes what the user sees: a toast in Kando's window, and terms proposed for the open film's glossary, which the user adds or dismisses.
	GroupChange
	// GroupNetwork reaches out of Kando: the network rules, a credential sent through Kando, the requests of its media sources and a page opened in the browser, whose site hears what its address says.
	GroupNetwork
	// GroupAI uses the user's AI service and its tokens.
	GroupAI
)

// Groups are the groups of permissions in the order the details list them.
var Groups = []PermGroup{GroupRead, GroupChange, GroupNetwork, GroupAI}

// Group is the group p is listed under. A plugin's network rules, granted as a whole, are GroupNetwork's too.
func (p Perm) Group() PermGroup {
	switch p {
	case PermToast, PermGlossarySuggest:
		return GroupChange
	case PermOpenURL, PermCredFetch, PermSourceFetch:
		return GroupNetwork
	case PermLLMChat:
		return GroupAI
	}
	return GroupRead
}

// Missing reports whether g, what the user granted the script plugin m, leaves out something m needs: a permission it does not list as optional and that needs a grant, or its network rules. The list's row says so (docs/plugin-system.md 7.1).
func Missing(m Manifest, g Grant) bool {
	if m.Capabilities == nil {
		return false
	}
	g = g.Intersect(m.Declared())
	for _, p := range perms(m.Capabilities.Host) {
		if !p.Free() && !slices.Contains(m.Optional, p) && !g.Has(p) {
			return true
		}
	}
	return m.Capabilities.Network != nil && len(m.Capabilities.Network.HTTP) > 0 && len(g.Network) == 0
}

// Online reports whether the script plugin m needs what offline mode takes away (State.Effective): its network rules, a credential or the user's AI service, among what it does not list as optional.
func Online(m Manifest) bool {
	if m.Capabilities == nil {
		return false
	}
	if m.Capabilities.Network != nil && len(m.Capabilities.Network.HTTP) > 0 {
		return true
	}
	for _, p := range perms(m.Capabilities.Host) {
		if (p == PermCredFetch || p == PermLLMChat) && !slices.Contains(m.Optional, p) {
			return true
		}
	}
	return false
}
