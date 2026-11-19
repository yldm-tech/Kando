package main

import (
	"errors"
	"slices"
	"time"

	"kando/packages/core"
	"kando/packages/media"
)

// Terms suggested for the glossary: once a job translates a film, it asks the AI service for the names and terms its lines come back to (media.WithTermSuggestions), unless offline mode keeps it from a service off this computer, and keeps them beside the video (media.SuggestionsPath). The open film's 术语 tab lists those its glossary does not hold yet, each added to the film's own terms with a press, as a step Undo takes back (glossaryChange), or dismissed, which keeps it from being suggested again.

// glossarySuggest is the open film's suggestions: video is the film's video, mod when its subtitles were read as they were when the suggestions were last read, read whether they have been, so a job translating the film again has them read again; list is what the file holds once read (ok false for a file that could not be read, which is then not written over), gen telling a read from one of a film opened before.
type glossarySuggest struct {
	video string
	mod   time.Time
	read  bool
	list  media.Suggestions
	ok    bool
	gen   int
}

// glossarySuggestFollow reads the open film's suggestions off the engine's goroutine once another film is open, and again once its subtitles are read again after a job rewrote them, keeping the ones it holds until then, and reports whether it dropped them for another film's.
func (a *app) glossarySuggestFollow() bool {
	g := &a.gloss.suggest
	video := a.gloss.video
	dropped := false
	if video != g.video {
		dropped = len(g.list.Terms) > 0
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

// TermSuggestions are the terms suggested for the open film that its glossary does not hold, the ones the most lines hold first; none while its own terms cannot be changed (TermsWritable).
func (a *app) TermSuggestions() []core.Term {
	g := a.gloss.suggest
	if !g.ok || len(g.list.Terms) == 0 || !a.TermsWritable() {
		return nil
	}
	have := a.FilmTerms()
	var out []core.Term
	for _, t := range g.list.Terms {
		if !slices.ContainsFunc(have, func(h core.Term) bool { return core.SameTerm(h.Source, t.Source) }) {
			out = append(out, t)
		}
	}
	return out
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

// DismissSuggestion takes the term suggested for source off the open film's suggestions for good: translating the film again does not suggest it again.
func (a *app) DismissSuggestion(source string) {
	g := &a.gloss.suggest
	if !g.ok || !slices.ContainsFunc(g.list.Terms, func(t core.Term) bool { return core.SameTerm(t.Source, source) }) {
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
		return
	}
	g.list = next
}
