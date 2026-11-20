package main

import (
	"cmp"
	"net/url"
	"strings"

	"kando/apps/kando/film"
	"kando/apps/kando/films"
	"kando/apps/kando/lines"
	"kando/apps/kando/plugins"
	"kando/apps/kando/saved"
	"kando/packages/core"

	"github.com/nsgo-dev/nsgo/base"
	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/i18n"
)

// What a script plugin calls the app with: its own host's handlers (core.Host), one per permission, the handler's name being the permission's (docs/plugin-system.md 5.3). The host is the plugin's alone and the handlers run on the engine's goroutine with the app's authority, so each checks, at every call, that the plugin is still the one running under this host and that what it may do now (State.Effective) holds the permission, and counts the call against its quota; a refusal is the error code "denied", or "quota", never the app's own words. They answer with plain values: no path, no address of an AI service, no key.

// The error codes a refused host call fails with.
const (
	pluginsDenied = "denied"
	pluginsQuota  = "quota"
	pluginsBad    = "bad-request"
	pluginsBusy   = "busy"
)

// pluginsSoundMargin is how many reads of a sound an export the user started may make beyond one per line it was given, for a read the plugin tries again.
const pluginsSoundMargin = 8

// pluginsSoundAllowance is an export's allowance of sound reads: the export it belongs to and the reads left.
type pluginsSoundAllowance struct {
	gen, left int
}

// pluginsAllowSounds lets the plugin id read the sound of lines lines plus pluginsSoundMargin times without counting toward its quota of host calls a minute, while an export the user started (the Saved Lines sheet's, pluginsExportTo) runs: an export of a few hundred lines with their sound would otherwise run out of the minute's sixty calls after the sixtieth line. It returns the end of the allowance, which the export calls as its call ends, however it ends; reads left then are not kept for later, and a plugin's own calls, from a command or a timer outside an export, are held to the quota as before. The allowance is the plugin's while the export runs, so a timer of its own may use it then, never past the count.
func (a *app) pluginsAllowSounds(id string, lines int) (end func()) {
	s := &a.plugins.script
	if s.sounds == nil {
		s.sounds = map[string]pluginsSoundAllowance{}
	}
	s.soundsGen++
	gen := s.soundsGen
	s.sounds[id] = pluginsSoundAllowance{gen: gen, left: lines + pluginsSoundMargin}
	return func() {
		if s.sounds[id].gen == gen {
			delete(s.sounds, id)
		}
	}
}

// pluginsSoundAllowed reports whether the plugin id may read a sound under an export's allowance now, and counts it.
func (a *app) pluginsSoundAllowed(id string) bool {
	s := &a.plugins.script
	al, ok := s.sounds[id]
	if !ok || al.left <= 0 {
		return false
	}
	al.left--
	s.sounds[id] = al
	return true
}

// pluginsOwnCode reports whether code, the code of an error a plugin rejected a call with, is the plugin's own rather than one Kando or the runtime gave it: not empty, not one of the codes Kando refuses a host call with, and not the runtime's (ETIMEDOUT of a host call that timed out, ERR_… of a refusal), whose words are not written for the user.
func pluginsOwnCode(code string) bool {
	switch code {
	case "", "ETIMEDOUT", pluginsDenied, pluginsQuota, pluginsBad, pluginsBusy, pluginsUnavailable, pluginsOverBudget, pluginsCutOff, pluginsTimedOut, pluginsFailed, pluginsNoFFmpeg, pluginsNotFound, pluginsTooLarge, pluginsNoSecret:
		return false
	}
	return !strings.HasPrefix(code, "ERR_")
}

// pluginsRefuse fails c with code.
func pluginsRefuse(c *ns.HostCall, code string) {
	c.Reject(&ns.Error{Code: code, Message: code})
}

// pluginsHost gives the host of the plugin id its handlers: a handler for each permission this version has, which the plugin reaches only as far as its sandbox's grant and the handler's own check let it.
func (a *app) pluginsHost(id string, host *ns.Host) {
	handlers := map[plugins.Perm]func(l *pluginsScript, c *ns.HostCall){
		plugins.PermFilmCurrent: a.pluginsFilmCurrent,
		plugins.PermLineCurrent: a.pluginsLineCurrent,
		plugins.PermLinesSeen:   a.pluginsLinesSeen,
		plugins.PermSavedRead:   a.pluginsSavedRead,
		plugins.PermWordsRead:   a.pluginsWordsRead,
		plugins.PermSoundRead:   a.pluginsSoundRead,
		plugins.PermLLMChat:     a.pluginsLLMChat,
		plugins.PermCredFetch:   a.pluginsCredentialFetch,
		plugins.PermToast:       a.pluginsToastCall,
		plugins.PermOpenURL:     a.pluginsOpenURLCall,
		plugins.PermSettingsGet: a.pluginsSettingsGet,
	}
	for perm, fn := range handlers {
		host.Handle(string(perm), a.pluginsGuard(id, host, perm, fn))
	}
}

// pluginsGuard is fn behind the checks every call passes: the plugin id runs under host still, as the host it was loaded with or the one of the view of its panel shown now, what it may do now holds perm, and its quota, which its panel shares, is not used up.
func (a *app) pluginsGuard(id string, host *ns.Host, perm plugins.Perm, fn func(l *pluginsScript, c *ns.HostCall)) func(*ns.HostCall) {
	return func(c *ns.HostCall) {
		l := a.plugins.script.loaded[id]
		if l == nil || l.host != host && l.panel != host || !a.pluginsEffective(l.pack).Has(perm) {
			pluginsRefuse(c, pluginsDenied)
			return
		}
		// A line's sound read for an export the user started is metered against that export, not the minute's quota (pluginsAllowSounds).
		if !(perm == plugins.PermSoundRead && a.pluginsSoundAllowed(id)) && !a.pluginsQuota(id) {
			pluginsRefuse(c, pluginsQuota)
			return
		}
		fn(l, c)
		a.Changed()
	}
}

// pluginsArg is the number field name of what a call sent, or def when it sent none, between 1 and max.
func pluginsArg(c *ns.HostCall, name string, def, max int) int {
	m, _ := c.Data.(map[string]any)
	n, ok := m[name].(float64)
	if !ok || !(n >= 1) {
		return def
	}
	// Clamped as a float first: a number past int's range converts to nonsense.
	return int(min(n, float64(max)))
}

// pluginsFilmCurrent answers kando.film.current: the film open, or null, by the id this plugin knows it by, its title, the language it is heard in, the one it is translated into and where it plays.
func (a *app) pluginsFilmCurrent(l *pluginsScript, c *ns.HostCall) {
	f, ok := a.openFilm()
	if !ok {
		c.Resolve(nil)
		return
	}
	c.Resolve(map[string]any{
		"film":       a.pluginsFilmID(l, cmp.Or(f.VideoPath, f.Key)),
		"title":      f.Title,
		"spoken":     films.Heard(f),
		"language":   core.TargetTag(f.Language),
		"positionMs": a.PlayHead().Milliseconds(),
	})
}

// pluginsLine is the line cue at index i as a plugin reads it (plugins.LineValue).
func pluginsLine(i int, cue core.Segment) map[string]any {
	translation, original := lines.Split(cue.Text)
	return plugins.LineValue(i, cue.Start.Milliseconds(), cue.End.Milliseconds(), original, translation)
}

// pluginsCurrentLine is the index of the line playing in the open film, or between lines the one just heard, -1 with none.
func (a *app) pluginsCurrentLine() int {
	cues := a.playerCues()
	if len(cues) == 0 {
		return -1
	}
	prev, cur, _ := film.CueNear(cues, a.playerLineAt())
	if cur >= 0 {
		return cur
	}
	return prev
}

// pluginsLineCurrent answers kando.line.current: the line playing, or null.
func (a *app) pluginsLineCurrent(_ *pluginsScript, c *ns.HostCall) {
	i := a.pluginsCurrentLine()
	if i < 0 {
		c.Resolve(nil)
		return
	}
	c.Resolve(pluginsLine(i, a.playerCues()[i]))
}

// pluginsLinesSeen answers kando.lines.seen: the last lines, limit of them (pluginsSeenDefault, at most pluginsListMax), of those watched of the open film, never past the furthest point it played to nor past the line playing (lines.Seen, as the recap counts them), so no line not heard yet reaches a plugin.
func (a *app) pluginsLinesSeen(_ *pluginsScript, c *ns.HostCall) {
	f, ok := a.openFilm()
	cues := a.playerCues()
	if !ok || len(cues) == 0 {
		c.Resolve([]any{})
		return
	}
	horizon := a.palette.furthest[f.VideoPath]
	cur := a.pluginsCurrentLine()
	var out []any
	for i, cue := range cues {
		if !lines.Seen(cue.Start, horizon) && i > cur {
			break
		}
		out = append(out, pluginsLine(i, cue))
	}
	limit := pluginsArg(c, "limit", pluginsSeenDefault, pluginsListMax)
	if len(out) > limit {
		out = out[len(out)-limit:]
	}
	if out == nil {
		out = []any{}
	}
	c.Resolve(out)
}

// pluginsSavedRead answers kando.saved.read: the saved lines of the library open, the newest first, limit of them (at most pluginsListMax), each with the id of its film for this plugin, never its path.
func (a *app) pluginsSavedRead(l *pluginsScript, c *ns.HostCall) {
	book := a.savedBook()
	out := []any{}
	for i := len(book.Lines) - 1; i >= 0 && len(out) < pluginsArg(c, "limit", pluginsListMax, pluginsListMax); i-- {
		out = append(out, a.pluginsSavedLine(l, book.Lines[i]))
	}
	c.Resolve(out)
}

// pluginsSavedLine is the saved line as the plugin l reads it: the id of its film for this plugin, never its path.
func (a *app) pluginsSavedLine(l *pluginsScript, line saved.Line) map[string]any {
	return map[string]any{"id": line.ID, "film": a.pluginsFilmID(l, line.Video), "title": line.Film, "original": line.Original, "translation": line.Translation,
		"note": line.Note, "startMs": line.Start.Milliseconds(), "endMs": line.End.Milliseconds(), "savedAt": line.SavedAt.UnixMilli()}
}

// pluginsWord is the word of the vocabulary as the plugin l reads it.
func (a *app) pluginsWord(l *pluginsScript, w saved.Word) map[string]any {
	return map[string]any{"id": w.ID, "text": w.Text, "spoken": w.Spoken, "reading": w.Reading, "meaning": w.Meaning, "language": w.Language,
		"context": w.Context, "contextTranslation": w.ContextTranslation, "film": a.pluginsFilmID(l, w.Video), "title": w.Film, "savedAt": w.SavedAt.UnixMilli()}
}

// pluginsWordsRead answers kando.words.read: the words of the vocabulary, the newest first, limit of them (at most pluginsListMax).
func (a *app) pluginsWordsRead(l *pluginsScript, c *ns.HostCall) {
	book := a.savedBook()
	out := []any{}
	for i := len(book.Words) - 1; i >= 0 && len(out) < pluginsArg(c, "limit", pluginsListMax, pluginsListMax); i-- {
		out = append(out, a.pluginsWord(l, book.Words[i]))
	}
	c.Resolve(out)
}

// pluginsToastCall answers kando.toast: it shows the text sent, a string or {text}, as a toast under the plugin's name.
func (a *app) pluginsToastCall(l *pluginsScript, c *ns.HostCall) {
	text, _ := c.Data.(string)
	if m, ok := c.Data.(map[string]any); ok {
		text, _ = m["text"].(string)
	}
	if strings.TrimSpace(text) == "" {
		pluginsRefuse(c, pluginsBad)
		return
	}
	a.pluginsToast(l.pack, text)
	c.Resolve(true)
}

// pluginsOpenURLCall answers kando.ui.openURL: an https page, sent as a string or {url}, opens in the browser once the user agreed in a system alert that names its host and the plugin, each time; the call answers whether it opened. One page waits for an answer at a time.
func (a *app) pluginsOpenURLCall(l *pluginsScript, c *ns.HostCall) {
	raw, _ := c.Data.(string)
	if m, ok := c.Data.(map[string]any); ok {
		raw, _ = m["url"].(string)
	}
	u, err := url.Parse(raw)
	if err != nil || u.Scheme != "https" || u.Hostname() == "" || u.User != nil || len(raw) > 2048 || strings.ContainsFunc(raw, func(r rune) bool { return r < ' ' || r == 0x7f }) {
		pluginsRefuse(c, pluginsBad)
		return
	}
	s := &a.plugins.script
	if s.asking || a.cx == nil {
		pluginsRefuse(c, pluginsBusy)
		return
	}
	s.asking = true
	// The user may take as long as they like to answer: the call waiting for it is not held to its deadline nor blamed for the window's pauses meanwhile.
	resume := a.pluginsRunner().PauseFor(l.plugin)
	m := a.messages()
	open := m.PluginsOpenURLOpen()
	name := l.pack.Name.In(i18n.Language(a.cx))
	release := a.holdModal()
	id := l.pack.ID
	base.ShowAlertDialog(a.cx, base.AlertDialog{
		Title:       m.PluginsOpenURLTitle(u.Hostname()),
		Description: m.PluginsOpenURLMessage(name, raw),
		Actions:     []ns.AlertAction{{Label: m.SettingsPluginsCancel(), Role: ns.AlertCancel}, {Label: open}},
	}, func(label string) {
		release()
		resume()
		s.asking = false
		ok := label == open
		if ok {
			a.menuOpenLink(raw)
		}
		answer := "declined"
		if ok {
			answer = "opened"
		}
		a.pluginsAudit(plugins.AuditOpenURL, id, answer+" "+u.Hostname())
		c.Resolve(ok)
		a.Changed()
	})
}

// pluginsSettingsGet answers kando.settings.get: the values of the plugin's own settings, its secrets left out (Manifest.Resolve).
func (a *app) pluginsSettingsGet(l *pluginsScript, c *ns.HostCall) {
	c.Resolve(l.pack.Resolve(a.plugins.script.values[l.pack.ID]))
}
