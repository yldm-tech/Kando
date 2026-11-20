package main

import (
	"cmp"
	"errors"
	"slices"
	"time"

	"kando/apps/kando/plugins"
	"kando/packages/core"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/i18n"
)

// Terms suggested for the glossary: once a job translates a film, it asks the AI service for the names and terms its lines come back to (media.WithTermSuggestions), unless offline mode keeps it from a service off this computer, and keeps them beside the video (media.SuggestionsPath). The open film's 术语 tab lists those its glossary does not hold yet, each added to the film's own terms with a press, as a step Undo takes back (glossaryChange), or dismissed, which keeps it from being suggested again. A script plugin granted kando.glossary.suggest proposes terms for the open film as well (glossarySuggestFromPlugin), which the same list shows under the plugin's name until another film is open, after the AI service's.

// glossarySuggest is the open film's suggestions: video is the film's video, mod when its subtitles were read as they were when the suggestions were last read, read whether they have been, so a job translating the film again has them read again; list is what the file holds once read (ok false for a file that could not be read, which is then not written over), gen telling a read from one of a film opened before; plugin is what script plugins proposed for the film since it was opened, kept in memory alone, at most plugins.SuggestHeld.
type glossarySuggest struct {
	video  string
	mod    time.Time
	read   bool
	list   media.Suggestions
	ok     bool
	gen    int
	plugin []glossaryPluginTerm
}

// glossaryPluginTerm is a term a script plugin proposed for the open film: the term, and the id and name of the plugin.
type glossaryPluginTerm struct {
	term core.Term
	id   string
	name plugins.Text
}

// glossarySuggestFollow reads the open film's suggestions off the engine's goroutine once another film is open, and again once its subtitles are read again after a job rewrote them, keeping the ones it holds until then, and reports whether it dropped them for another film's.
func (a *app) glossarySuggestFollow() bool {
	g := &a.gloss.suggest
	video := a.gloss.video
	dropped := false
	if video != g.video {
		dropped = len(g.list.Terms) > 0 || len(g.plugin) > 0
		*g = glossarySuggest{video: video, gen: g.gen + 1}
	}
	if video == "" {
		return dropped
	}
	var mod time.Time
	if subs := a.player.subs; a.openKey != "" && subs.key == a.openKey && subs.ready {
		mod = subs.mod
	}
	if g.read && mod.Equal(g.mod) {
		return dropped
	}
	g.mod, g.read = mod, true
	g.gen++
	gen, update := g.gen, a.update
	go func() {
		list, err := media.LoadSuggestions(media.SuggestionsPath(video))
		update(func() {
			if a.gloss.suggest.gen != gen {
				return
			}
			a.gloss.suggest.list, a.gloss.suggest.ok = list, err == nil
		})
	}()
	return dropped
}

// TermSuggestions are the terms suggested for the open film that its glossary does not hold, the AI service's first, the ones the most lines hold first, then those script plugins proposed, in the order they did, each source once and none dismissed; none while its own terms cannot be changed (TermsWritable).
func (a *app) TermSuggestions() []core.Term {
	g := a.gloss.suggest
	if !a.TermsWritable() {
		return nil
	}
	have := a.FilmTerms()
	var out []core.Term
	add := func(t core.Term) {
		held := func(h core.Term) bool { return core.SameTerm(h.Source, t.Source) }
		if !slices.ContainsFunc(have, held) && !slices.ContainsFunc(out, held) && !slices.ContainsFunc(g.list.Dismissed, func(d string) bool { return core.SameTerm(d, t.Source) }) {
			out = append(out, t)
		}
	}
	if g.ok {
		for _, t := range g.list.Terms {
			add(t)
		}
	}
	for _, p := range g.plugin {
		add(p.term)
	}
	return out
}

// TermSuggestionPlugin is the name, in the language the interface shows under cx, of the script plugin that proposed the term suggested for source, "" for one the AI service found (film.Glossary).
func (a *app) TermSuggestionPlugin(cx *ns.Scope, source string) string {
	g := a.gloss.suggest
	if g.ok && slices.ContainsFunc(g.list.Terms, func(t core.Term) bool { return core.SameTerm(t.Source, source) }) {
		return ""
	}
	i := slices.IndexFunc(g.plugin, func(p glossaryPluginTerm) bool { return core.SameTerm(p.term.Source, source) })
	if i < 0 {
		return ""
	}
	return cmp.Or(g.plugin[i].name.In(i18n.Language(cx)), g.plugin[i].id)
}

// glossarySuggestFromPlugin keeps the terms the script plugin id, named name, proposed for the open film, to list them with its suggestions, and returns how many it took: none the glossary holds, another suggestion has or the user dismissed, and none past plugins.SuggestHeld; false while no film with a video is open. Terms sent before the film's own terms or its suggestions file are read are kept all the same, as a plugin hearing film.opened calls before they are, and TermSuggestions leaves out those the glossary turns out to hold. The glossary itself is changed only when the user adds one (AcceptSuggestion), so a plugin never writes over a term, locked or not.
func (a *app) glossarySuggestFromPlugin(id string, name plugins.Text, terms []core.Term) (int, bool) {
	if a.gloss.video == "" {
		return 0, false
	}
	g := &a.gloss.suggest
	have := a.FilmTerms()
	taken := 0
	for _, t := range terms {
		if len(g.plugin) >= plugins.SuggestHeld {
			break
		}
		same := func(h core.Term) bool { return core.SameTerm(h.Source, t.Source) }
		if slices.ContainsFunc(have, same) || g.ok && slices.ContainsFunc(g.list.Terms, same) ||
			slices.ContainsFunc(g.list.Dismissed, func(d string) bool { return core.SameTerm(d, t.Source) }) ||
			slices.ContainsFunc(g.plugin, func(p glossaryPluginTerm) bool { return same(p.term) }) {
			continue
		}
		g.plugin = append(g.plugin, glossaryPluginTerm{term: t, id: id, name: name})
		taken++
	}
	return taken, true
}

// AcceptSuggestion adds the term suggested for source to the open film's own terms, as a step Undo takes back; undone, it is suggested again.
func (a *app) AcceptSuggestion(source string) {
	list := a.TermSuggestions()
	i := slices.IndexFunc(list, func(t core.Term) bool { return core.SameTerm(t.Source, source) })
	if i < 0 {
		return
	}
	a.glossaryChange(correctKindTermAccept, list[i])
}

// DismissSuggestion takes the term suggested for source off the open film's suggestions for good: translating the film again does not suggest it again, nor does a plugin while the film is open. One a plugin proposed is kept with the film's dismissed suggestions as well (media.DismissSuggestion), so no plugin proposes it again once the film is opened again; when that file cannot be written, it still leaves the list until the film is opened again.
func (a *app) DismissSuggestion(source string) {
	g := &a.gloss.suggest
	same := func(t core.Term) bool { return core.SameTerm(t.Source, source) }
	fromPlugin := slices.ContainsFunc(g.plugin, func(p glossaryPluginTerm) bool { return same(p.term) })
	if !fromPlugin && (!g.ok || !slices.ContainsFunc(g.list.Terms, same)) {
		return
	}
	// Read again as the file holds them, which a job translating the film again may have just written.
	next, err := media.DismissSuggestion(g.video, source)
	if err != nil {
		if errors.Is(err, media.ErrSuggestionsNewer) {
			a.flash(a.messages().GlossaryNewer())
		} else {
			a.flash(a.messages().GlossarySaveFailed())
		}
		// One a plugin proposed still leaves the list while the film is open, so a suggestions file that cannot be read or written does not keep it there for good.
		g.plugin = slices.DeleteFunc(g.plugin, func(p glossaryPluginTerm) bool { return same(p.term) })
		return
	}
	// What the file holds now is newer than a read still on its way (glossarySuggestFollow), which would bring the term back.
	g.list, g.ok = next, true
	g.gen++
	g.plugin = slices.DeleteFunc(g.plugin, func(p glossaryPluginTerm) bool { return same(p.term) })
}
