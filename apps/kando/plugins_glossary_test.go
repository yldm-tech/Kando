package main

import (
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/script"
	"kando/packages/core"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// pluginSuggestFields are the fields of the test plugin acme.places: it reads the film open and proposes terms for its glossary.
const pluginSuggestFields = `"capabilities": {"host": ["kando.film.current", "kando.glossary.suggest"]}`

// pluginSuggestCode is acme.places' code: suggest proposes terms, for the film open when film is "current", and says what the call answered or the code it was refused with.
const pluginSuggestCode = `import { host } from "nsgo"
export async function suggest(terms, film) {
  const data = { terms }
  if (film === "current") data.film = (await host.call("kando.film.current", {})).film
  else if (film) data.film = film
  try { return await host.call("kando.glossary.suggest", data) } catch (e) { return "refused " + e.code }
}
`

// TestPluginGlossarySuggest checks kando.glossary.suggest: refused until granted; granted, the terms a plugin proposes for the film open are listed with the AI service's suggestions under the plugin's name, none the glossary holds, locked or not, nor one suggested already, and nothing goes into the glossary until the user adds one; adding is a step Undo takes back, dismissing keeps it from being proposed again; past the limits the call is refused whole, a film other than the one open is not found, the plugins' proposals for a film stop at plugins.SuggestHeld, a plugin's term dismissed leaves the list though the suggestions file cannot be read, and the call is unavailable while the glossary is still on the film before and not found with no film open.
func TestPluginGlossarySuggest(t *testing.T) {
	key := newPluginsKey(t)
	dir := pluginScriptFolder(t, key, "acme.places", "1.0.0", pluginSuggestFields, pluginSuggestCode)
	a, tt, updates, item := playerFilm(t, 12, nil, nsgotest.PendingAlerts(), nsgotest.Secrets(map[string]string{}))
	root := filepath.Join(t.TempDir(), "plugins")
	pluginsScriptInstalled(t, root, dir)
	a.plugins.root = root
	a.plugins.script.official = key.public
	a.plugins.script.watch = script.Watch{Beat: 100 * time.Millisecond, Stall: time.Minute, Stalls: 3, Deadline: time.Minute}
	a.pluginsScan()
	waitUntil(t, tt, a, updates, func() bool {
		return a.plugins.scanned && !a.plugins.busy && !a.plugins.inboxBusy && a.plugins.script.verified != nil
	})

	// The film's own Nile is locked, Danube is every film's term and the AI service found delta.
	if err := media.SaveTerms(media.TermsPath(item.VideoPath), []core.Term{{Source: "Nile", Target: "尼罗河", Scope: core.ScopeFilm, Locked: true}}); err != nil {
		t.Fatal(err)
	}
	path := media.SuggestionsPath(item.VideoPath)
	if err := media.SaveSuggestions(path, media.Suggestions{Terms: []core.Term{{Source: "delta", Target: "三角洲"}}}); err != nil {
		t.Fatal(err)
	}
	a.glossaryLoad()
	a.gloss.store.Set("", core.Term{Source: "Danube", Target: "多瑙河", Scope: core.ScopeGlobal})
	scriptsGrant(t, a, tt, updates, "acme.places", string(plugins.PermFilmCurrent))
	playerOpenFilm(t, a, tt, updates, item)
	playerLoadFilm(tt, a, 12*time.Second)
	waitUntil(t, tt, a, updates, func() bool { return a.gloss.filmRead && len(a.TermSuggestions()) == 1 })

	term := func(source, target string) any { return map[string]any{"source": source, "target": target} }
	suggest := func(film string, terms ...any) any {
		t.Helper()
		got, err := scriptsCall(t, a, tt, updates, "acme.places", "suggest", terms, film)
		if err != nil {
			t.Fatal(err)
		}
		if m, ok := got.(map[string]any); ok {
			n, _ := plugins.Number(m["added"])
			return int(n)
		}
		return got
	}
	// Not granted, its sandbox refuses the call before the handler hears it.
	if got := suggest("current", term("Amazon", "亚马孙河")); got != "refused ERR_ACCESS_DENIED" || len(a.TermSuggestions()) != 1 {
		t.Fatalf("not granted, the call answered %v and the suggestions are %+v", got, a.TermSuggestions())
	}

	scriptsGrant(t, a, tt, updates, "acme.places", string(plugins.PermGlossarySuggest))
	waitUntil(t, tt, a, updates, func() bool {
		l := a.plugins.script.loaded["acme.places"]
		return l != nil && !a.plugins.saving && a.pluginsEffective(l.pack).Has(plugins.PermGlossarySuggest)
	})
	if got := suggest("current", term("nile", "Nile River"), term("danube", "Donau"), term("Delta", "河口"), term("Amazon", "亚马孙河"), term("amazon", "亚马逊")); got != 1 {
		t.Fatalf("the call answered %v", got)
	}
	want := []core.Term{{Source: "delta", Target: "三角洲", Scope: core.ScopeFilm, Origin: core.OriginAuto}, {Source: "Amazon", Target: "亚马孙河", Scope: core.ScopeFilm, Origin: core.OriginAuto}}
	if got := a.TermSuggestions(); !reflect.DeepEqual(got, want) {
		t.Fatalf("suggested %+v", got)
	}
	if own := a.gloss.film; len(own) != 1 || own[0].Target != "尼罗河" || !own[0].Locked {
		t.Fatalf("the film's own terms became %+v", own)
	}

	// The 术语 tab lists Amazon under the plugin's name, and delta, the AI service's, under none.
	tt.Press("id:film-tab-terms")
	msgs := testlocale.Messages()
	wantText(t, tt, "Amazon → 亚马孙河")
	if names := tt.Query("id:film-term-suggestion-plugin"); len(names) != 1 || names[0].Text != msgs.GlossarySuggestFromPlugin("Anki 发送器") {
		t.Fatalf("the plugin's name: %q", tt.Texts())
	}

	// Added, it is the film's own term, made by a machine and not locked; undone, it is suggested again.
	tt.Press("id:film-term-suggestion-add >> nth=1")
	if i := slices.IndexFunc(a.gloss.film, func(t core.Term) bool { return t.Source == "Amazon" }); i < 0 || a.gloss.film[i] != (core.Term{Source: "Amazon", Target: "亚马孙河", Scope: core.ScopeFilm, Origin: core.OriginAuto}) {
		t.Fatalf("added %+v", a.gloss.film)
	}
	if got := a.TermSuggestions(); len(got) != 1 || got[0].Source != "delta" {
		t.Fatalf("after adding, suggested %+v", got)
	}
	wantText(t, tt, msgs.GlossaryStepAccept("亚马孙河"))
	tt.Press("id:film-edit-log-undo")
	if got := a.TermSuggestions(); len(got) != 2 || got[1].Source != "Amazon" {
		t.Fatalf("undone, suggested %+v", got)
	}

	// Dismissed, it is kept with the film's dismissed suggestions, and the plugin cannot propose it again.
	tt.Press("id:film-term-suggestion-dismiss >> nth=1")
	saved, err := media.LoadSuggestions(path)
	if err != nil || !reflect.DeepEqual(saved.Dismissed, []string{"Amazon"}) || len(saved.Terms) != 1 {
		t.Fatalf("the file holds %+v, %v", saved, err)
	}
	if got := suggest("", term("Amazon", "亚马孙河")); got != 0 || len(a.TermSuggestions()) != 1 {
		t.Fatalf("proposed again, the call answered %v and the suggestions are %+v", got, a.TermSuggestions())
	}

	// Past the limits the call is refused whole; a film other than the one open is not found.
	many := make([]any, plugins.SuggestMax+1)
	for i := range many {
		many[i] = term(fmt.Sprintf("place %d", i), "地方")
	}
	for name, terms := range map[string][]any{
		"too many terms":  many,
		"a long source":   {term(strings.Repeat("a", plugins.SuggestLongest+1), "x")},
		"no translation":  {term("Congo", " ")},
		"not a term list": {"Congo"},
	} {
		if got := suggest("", terms...); got != "refused bad-request" {
			t.Errorf("%s: the call answered %v", name, got)
		}
	}
	if got := suggest("f_elsewhere", term("Congo", "刚果河")); got != "refused not-found" {
		t.Errorf("another film: the call answered %v", got)
	}
	if len(a.TermSuggestions()) != 1 {
		t.Fatalf("refused calls suggested %+v", a.TermSuggestions())
	}

	// Sent before the film's own terms are read, as a plugin hearing film.opened may, the terms are kept, and listed once they are read, but for the one the film holds.
	own := a.gloss.film
	apply(tt, a, func() { a.gloss.film, a.gloss.filmRead = nil, false })
	if got := suggest("current", term("Congo", "刚果河"), term("NILE", "Nile River")); got != 2 || len(a.TermSuggestions()) != 0 {
		t.Fatalf("before the terms are read, the call answered %v and the suggestions are %+v", got, a.TermSuggestions())
	}
	apply(tt, a, func() { a.gloss.film, a.gloss.filmRead = own, true })
	if got := a.TermSuggestions(); len(got) != 2 || got[1].Source != "Congo" {
		t.Fatalf("once the terms are read, suggested %+v", got)
	}
	apply(tt, a, func() { a.gloss.suggest.plugin = nil })

	// While the film's suggestions file cannot be read, a plugin's term dismissed leaves the list all the same, though the file keeps nothing.
	good, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, []byte("{"), 0o644); err != nil {
		t.Fatal(err)
	}
	apply(tt, a, func() { a.gloss.suggest.ok = false })
	if got := suggest("current", term("Congo", "刚果河")); got != 1 || len(a.TermSuggestions()) != 1 || a.TermSuggestions()[0].Source != "Congo" {
		t.Fatalf("with the file unreadable, the call answered %v and the suggestions are %+v", got, a.TermSuggestions())
	}
	apply(tt, a, func() { a.DismissSuggestion("Congo") })
	if got := a.TermSuggestions(); len(got) != 0 {
		t.Fatalf("dismissed with the file unreadable, suggested %+v", got)
	}
	if err := os.WriteFile(path, good, 0o644); err != nil {
		t.Fatal(err)
	}
	apply(tt, a, func() { a.gloss.suggest.ok, a.gloss.suggest.plugin = true, nil })

	// However often a plugin proposes, a film holds at most plugins.SuggestHeld of the plugins' terms; Amazon, dismissed, holds none.
	held := 0
	for round := 0; held < plugins.SuggestHeld; round++ {
		batch := make([]any, plugins.SuggestMax)
		for i := range batch {
			batch[i] = term(fmt.Sprintf("river %d-%d", round, i), "河")
		}
		got := suggest("current", batch...)
		n, ok := got.(int)
		if !ok || n != min(plugins.SuggestMax, plugins.SuggestHeld-held) {
			t.Fatalf("round %d answered %v with %d held", round, got, held)
		}
		held += n
	}
	if got := suggest("current", term("Congo", "刚果河")); got != 0 || len(a.TermSuggestions()) != 1+plugins.SuggestHeld {
		t.Fatalf("past the limit, the call answered %v and %d are suggested", got, len(a.TermSuggestions()))
	}

	// A film just opened whose glossary is still the film's before, as the player has not moved to its video yet, is unavailable, and nothing sent lands on the film before; with no film open the call is not found.
	apply(tt, a, func() { a.player.film.video = filepath.Join(filepath.Dir(item.VideoPath), "before.mp4") })
	if got := suggest("current", term("Congo", "刚果河")); got != "refused unavailable" || len(a.gloss.suggest.plugin) != 0 {
		t.Fatalf("with the glossary on another video, the call answered %v and the plugins' terms are %+v", got, a.gloss.suggest.plugin)
	}
	apply(tt, a, a.closeFilm)
	if got := suggest("", term("Congo", "刚果河")); got != "refused not-found" || len(a.gloss.suggest.plugin) != 0 {
		t.Fatalf("with no film open, the call answered %v and the plugins' terms are %+v", got, a.gloss.suggest.plugin)
	}
}
