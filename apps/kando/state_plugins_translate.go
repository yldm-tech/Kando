package main

import (
	"cmp"
	"context"
	"errors"
	"slices"
	"strings"
	"time"

	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/script"
	"kando/apps/kando/settings"
	"kando/apps/kando/settings/sectionid"
	kcore "kando/packages/core"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
)

// Script plugins' translators (docs/plugin-system.md E14): Settings › 翻译 › 翻译来源 chooses Kando's AI service, the default, or the translator of a script plugin turned on. A job then translates its film's lines with the plugin (media.Tools.Engine, named by the translator's title in media.Tools.EngineName), through the runner like any call of a plugin, at most plugins.TranslateBatch lines a call, each under the plugin's watchdog, which pauses while the plugin waits for Kando (credentialFetch, llm.chat). Proofreading, the final proofread and term suggestions stay with the AI service, and are left out when it is not set up (media.Tools.chats). 这句不对 (⇧E) asks the plugin for other translations of a line when it exports candidates (media.Tools.Candidates), the AI service otherwise. The choice is kept in the translation's part of settings.json (savedTranslate.TranslateSource); while its plugin is turned off or removed, or no longer has the translator, the films are translated with the AI service, which Settings › 翻译 and, once a run, the message bar say.

// savedTranslateSource is a script plugin's translator chosen as the translation source: the plugin's id, the translator's id, its title as it was shown when it was chosen, which names it once its plugin is gone and before the plugins folder is read, and whether it runs on this computer (plugins.Translator.Local), which offline mode asks before the folder is read.
type savedTranslateSource struct {
	Plugin     string `json:"plugin"`
	Translator string `json:"translator"`
	Title      string `json:"title,omitempty"`
	Local      bool   `json:"local,omitempty"`
}

// valid reports whether s names a plugin and a translator as their ids can be, a title of a line at most.
func (s *savedTranslateSource) valid() bool {
	s.Title = strings.TrimSpace(s.Title)
	if len([]rune(s.Title)) > 200 || strings.ContainsAny(s.Title, "\r\n") {
		s.Title = ""
	}
	return s.Plugin != "" && s.Translator != "" && !strings.Contains(s.Plugin, ":") && !strings.Contains(s.Translator, ":")
}

// pluginsTranslate is what the translation sources keep in the plugins' state: the state of the one chosen when the app last followed it (pluginsTranslateFollow), and whether the message bar said this run that it is gone.
type pluginsTranslate struct {
	state    pluginsSourceState
	goneSaid bool
}

// pluginsSourceState is what became of a translator chosen as the translation source.
type pluginsSourceState int

const (
	// sourceNone: none is chosen; the AI service translates.
	sourceNone pluginsSourceState = iota
	// sourcePending: its plugin is turned on and will run, but the plugins folder is not read yet, as at launch, or the plugin is still being verified or loaded. A job waits for it (pluginsTranslateWait).
	sourcePending
	// sourceActive: its plugin runs.
	sourceActive
	// sourceBroken: its plugin is turned on and adds it, but cannot run: refused as it was verified, turned off by Kando (health.json), paused for a revocation list out of date, or with credentials where the system has no secure store (pluginsRunnable). Jobs fail as not running rather than translate with the AI service.
	sourceBroken
	// sourceGone: its plugin is no longer installed and turned on, or no longer adds it; the AI service translates, as the settings and the message bar say.
	sourceGone
)

// pluginsTranslateWait is how long a job's translation waits for the plugin of the translator chosen while it is pending, as at launch, before it fails as not running; pluginsTranslatePoll how often it looks.
const (
	pluginsTranslateWait = 60 * time.Second
	pluginsTranslatePoll = 250 * time.Millisecond
)

// pluginsTranslateTimeout bounds a call of a translator's translate or candidates: the watchdog's minute for the call and its five minutes of Kando's waiting on its behalf, and a minute more for the calls before it in the runner's queue.
var pluginsTranslateTimeout = script.DefaultWatch.Deadline + script.DefaultWatch.Paused + time.Minute

// pluginsTranslators are the translators of the script plugins turned on, in the order the plugins are listed, each with its plugin's id and name. A plugin turned on that is still being verified or loaded is among them: its translator is chosen by what the user turned on, not by what runs this instant.
func (a *app) pluginsTranslators() []pluginsContribution[plugins.Translator] {
	var out []pluginsContribution[plugins.Translator]
	for _, p := range a.plugins.packs {
		if p.Kind != plugins.KindScript || p.Contributes == nil || !a.plugins.state.EnabledPack(p) {
			continue
		}
		for _, t := range p.Contributes.Translators {
			out = append(out, pluginsContribution[plugins.Translator]{id: p.ID, name: p.Name, of: t})
		}
	}
	return out
}

// pluginsTranslatorState is the translator id of the plugin plugin, as a plugin turned on adds it, and what became of it (pluginsSourceState); before the plugins folder is read it is pending and not known.
func (a *app) pluginsTranslatorState(plugin, id string) (pluginsContribution[plugins.Translator], pluginsSourceState) {
	if !a.plugins.scanned {
		return pluginsContribution[plugins.Translator]{}, sourcePending
	}
	i := slices.IndexFunc(a.pluginsTranslators(), func(t pluginsContribution[plugins.Translator]) bool { return t.id == plugin && t.of.ID == id })
	if i < 0 {
		return pluginsContribution[plugins.Translator]{}, sourceGone
	}
	t := a.pluginsTranslators()[i]
	switch p, _ := a.pluginsPack(plugin); {
	case a.plugins.script.loaded[plugin] != nil:
		return t, sourceActive
	case a.pluginsRunnable(p), a.pluginsVerifying(p):
		return t, sourcePending
	}
	return t, sourceBroken
}

// pluginsPack is the pack id installed, and whether there is one.
func (a *app) pluginsPack(id string) (plugins.Pack, bool) {
	i := slices.IndexFunc(a.plugins.packs, func(p plugins.Pack) bool { return p.ID == id })
	if i < 0 {
		return plugins.Pack{}, false
	}
	return a.plugins.packs[i], true
}

// pluginsVerifying reports whether the script plugin p waits for its package to be verified for its content: no verification has answered yet, or the last one did not cover this content and did not refuse it either, as while the folder is read again.
func (a *app) pluginsVerifying(p plugins.Pack) bool {
	s := &a.plugins.script
	if s.verified == nil {
		return true
	}
	if v, ok := s.verified[p.ID]; ok && v.Hash == p.Hash {
		return false
	}
	return !slices.ContainsFunc(s.issues, func(i plugins.Issue) bool { return i.Dir == p.Dir })
}

// pluginsTranslatorChosen is the translator chosen in Settings › 翻译 (prefs.translateSource), as far as it is known, and what became of it.
func (a *app) pluginsTranslatorChosen() (pluginsContribution[plugins.Translator], pluginsSourceState) {
	src := a.prefs.translateSource
	if src.Plugin == "" {
		return pluginsContribution[plugins.Translator]{}, sourceNone
	}
	return a.pluginsTranslatorState(src.Plugin, src.Translator)
}

// pluginsTranslateName is the name of the translator chosen, in the interface's language: its title, or as it was chosen while it is not known, else its plugin's id.
func (a *app) pluginsTranslateName() string {
	src := a.prefs.translateSource
	if t, state := a.pluginsTranslatorChosen(); state != sourceGone && t.id != "" {
		return t.of.Title.In(a.messages().Language())
	}
	return cmp.Or(src.Title, src.Plugin)
}

// TranslateSources are the translators of the script plugins turned on as 翻译来源 offers them, each named by its title in lang (settings.SourcesApp).
func (a *app) TranslateSources(lang string) []settings.TranslateSource {
	var out []settings.TranslateSource
	for _, t := range a.pluginsTranslators() {
		out = append(out, settings.TranslateSource{Value: pluginsContribID(t.id, t.of.ID), Name: t.of.Title.In(lang)})
	}
	return out
}

// TranslateSource is the value of the translator chosen, "" for the AI service, and "" while it is gone (settings.SourcesApp).
func (a *app) TranslateSource() string {
	if !a.pluginsTranslating() {
		return ""
	}
	src := a.prefs.translateSource
	return pluginsContribID(src.Plugin, src.Translator)
}

// SetTranslateSource chooses the translator value names, "" for the AI service, and saves the settings; a value that names no translator of a plugin turned on changes nothing (settings.SourcesApp). The sidebar's status line and the offer to translate the films follow at the next change (pluginsTranslateFollow).
func (a *app) SetTranslateSource(value string) {
	next := savedTranslateSource{}
	if value != "" {
		i := slices.IndexFunc(a.pluginsTranslators(), func(t pluginsContribution[plugins.Translator]) bool { return pluginsContribID(t.id, t.of.ID) == value })
		if i < 0 {
			return
		}
		t := a.pluginsTranslators()[i]
		next = savedTranslateSource{Plugin: t.id, Translator: t.of.ID, Title: t.of.Title.In(a.messages().Language()), Local: t.of.Local}
	}
	if a.prefs.translateSource == next {
		return
	}
	a.prefs.translateSource, a.plugins.translate.goneSaid = next, false
	a.Save()
}

// TranslateSourceGone is the name of the translator chosen whose plugin is no longer installed and turned on, or no longer adds it, so the films are translated with the AI service; "" while there is none (settings.SourcesApp).
func (a *app) TranslateSourceGone() string {
	if _, state := a.pluginsTranslatorChosen(); state != sourceGone {
		return ""
	}
	return a.pluginsTranslateName()
}

// pluginsTranslateFollow follows what became of the translator chosen (pluginsHooks' afterChange): when that changed, as when it was chosen, its plugin loaded or turned off, the sidebar's status line is checked again and the library offers again to translate the films it could not; once a run, the message bar says it is gone, with Settings › 翻译 to choose another. It reports whether it changed anything.
func (a *app) pluginsTranslateFollow() bool {
	_, state := a.pluginsTranslatorChosen()
	changed := false
	if state != a.plugins.translate.state {
		a.plugins.translate.state = state
		if a.side.ready.checked {
			a.checkReadiness(false)
		}
		a.libraryOfferTranslate()
		changed = true
	}
	if state == sourceGone && !a.plugins.translate.goneSaid {
		a.plugins.translate.goneSaid = true
		a.warn(a.messages().SettingsTranslateSourceGone(a.pluginsTranslateName()), sectionid.Translate)
		changed = true
	}
	return changed
}

// pluginsTranslating reports whether a script plugin's translator translates the films in place of the AI service: one is chosen and it is not gone, though it may still be loading or unable to run.
func (a *app) pluginsTranslating() bool {
	_, state := a.pluginsTranslatorChosen()
	return state == sourcePending || state == sourceActive || state == sourceBroken
}

// pluginsTranslateBroken is the name of the translator chosen when its plugin cannot run (sourceBroken), "" otherwise.
func (a *app) pluginsTranslateBroken() string {
	if _, state := a.pluginsTranslatorChosen(); state != sourceBroken {
		return ""
	}
	return a.pluginsTranslateName()
}

// pluginsTranslateLocal reports whether the translator chosen runs on this computer: as its plugin says, or as it said when it was chosen while the plugins folder is not read yet.
func (a *app) pluginsTranslateLocal() bool {
	if t, _ := a.pluginsTranslatorChosen(); t.id != "" {
		return t.of.Local
	}
	return a.prefs.translateSource.Local
}

// pluginsTranslateTools is tools translating with the translator chosen (media.Tools.Engine), named by its title in the interface's language, for a film spoken in spoken (whisper's code, "" when not known), which candidates are asked from; tools as they are when none is chosen or it is gone. Which plugin translates is settled as each translation starts (pluginsTranslator.ready), so a job made before the plugins folder was read, as one restored at launch, waits for it rather than translating with the AI service. Its candidates are asked of the plugin only while it runs and exports candidates; otherwise of the AI service.
func (a *app) pluginsTranslateTools(tools media.Tools, spoken string) media.Tools {
	if !a.pluginsTranslating() {
		return tools
	}
	src := a.prefs.translateSource
	engine := pluginsTranslator{a: a, plugin: src.Plugin, id: src.Translator, local: a.pluginsTranslateLocal(), spoken: spoken}
	tools.Engine, tools.EngineName = engine, a.pluginsTranslateName()
	if l := a.plugins.script.loaded[src.Plugin]; l != nil && l.pack.Candidater(l.plugin.Exports()) {
		tools.Engine = pluginsCandidater{engine}
	}
	return tools
}

// pluginsTranslator is a script plugin's translator as a translation source (media.SegmentTranslator), which a job's goroutine calls: each call goes to the engine's goroutine through update, to the plugin through the runner.
type pluginsTranslator struct {
	a          *app
	plugin, id string
	local      bool
	// spoken is the language the film is spoken in for Candidates, whisper's code, "" when not known.
	spoken string
}

// Local reports whether the translator runs on this computer, which offline mode keeps (media.Tools.Engine).
func (t pluginsTranslator) Local() bool { return t.local }

// pluginsCheckWait is how long CheckTranslate waits for the engine's goroutine to answer before it lets the translation go ahead, which then fails as it would.
const pluginsCheckWait = 5 * time.Second

// CheckTranslate reports what the translator needs and is missing before a film is translated (media.TranslateChecker): a no-credential failure for a plugin with credentials none of whose secrets is kept, as Kando last read them for its details (pluginsNoCredential), so the job says so at once rather than once its lines are sent, without reading the secure store. Anything else, and an engine's goroutine that does not answer within pluginsCheckWait, passes: the translation then fails as it would.
func (t pluginsTranslator) CheckTranslate() error {
	got := make(chan bool, 1)
	t.a.update(func() { got <- t.a.pluginsNoCredential(t.plugin) })
	timer := time.NewTimer(pluginsCheckWait)
	defer timer.Stop()
	select {
	case missing := <-got:
		if missing {
			return plugins.NewTranslateError(plugins.TranslateNoCredential, "")
		}
	case <-timer.C:
	}
	return nil
}

// pluginsNoCredential reports whether the script plugin id declares credentials and none of their secrets is kept, as Kando last read or changed them (pluginsScripts.secrets); false for a plugin without credentials, or whose secrets are not read yet, or one of whose secrets the secure store failed to read (pluginsReadKept), where the key may well be kept and the plugin's own fetch is left to find out, and without a secure store (pluginsScripts.noSecrets), where no key can be kept and the plugin stays off, which its translation then says (not-running) rather than ask for a key the user cannot fill in.
func (a *app) pluginsNoCredential(id string) bool {
	p, ok := a.pluginsPack(id)
	kept, read := a.plugins.script.secrets[id]
	if !ok || !read || len(p.Credentials) == 0 || a.plugins.script.noSecrets {
		return false
	}
	for _, c := range p.Credentials {
		if isKept, read := kept[plugins.SecretKey(c.ID)]; isKept || !read {
			return false
		}
	}
	return true
}

// Translate translates segments into language with the plugin's translate(lines, from, to, context), at most plugins.TranslateBatch lines a call, once the plugin runs (ready): lines the segments' texts, from the language the film is heard in (media.SpeechLanguageOf) as a language tag (plugins.SpokenTag), "" when not known, to the target's language tag, or language as it is for a target Kando has no tag for, given to a translator of any language alone; context {translator}, the translator's id. Each answer must be as many lines, in their order (plugins.TranslatedLines). A target or a spoken language the translator does not take fails as plugins.TranslateLanguage before anything is sent.
func (t pluginsTranslator) Translate(ctx context.Context, segments []kcore.Segment, language string) ([]kcore.Segment, error) {
	tr, err := t.ready(ctx)
	if err != nil {
		return nil, err
	}
	from, to, err := pluginsTranslatePair(tr, plugins.SpokenTag(media.SpeechLanguageOf(ctx)), language)
	if err != nil {
		return nil, err
	}
	out := make([]kcore.Segment, 0, len(segments))
	for start := 0; start < len(segments); start += plugins.TranslateBatch {
		batch := segments[start:min(start+plugins.TranslateBatch, len(segments))]
		lines := make([]any, len(batch))
		for i, s := range batch {
			lines[i] = s.Text
		}
		v, err := t.call(ctx, plugins.TranslateExport, []any{lines, from, to, map[string]any{"translator": t.id}})
		if err != nil {
			return nil, err
		}
		got, ok := plugins.TranslatedLines(v, len(batch))
		if !ok {
			return nil, plugins.NewTranslateError(plugins.TranslateBadReply, "")
		}
		for i, s := range batch {
			s.Text = got[i]
			out = append(out, s)
		}
	}
	return out, nil
}

// pluginsTranslatePair is what the translator tr is given to translate from the spoken language from, a language tag, "" when not known, into the target language, a job's Language: from, and the target's tag, or language as it is for a target Kando has no tag for; a plugins.TranslateLanguage failure when tr does not take them.
func pluginsTranslatePair(tr plugins.Translator, from, language string) (string, string, error) {
	to := kcore.TargetTag(language)
	if !tr.Takes(from, to) {
		return "", "", plugins.NewTranslateError(plugins.TranslateLanguage, strings.TrimSpace(from+" → "+cmp.Or(to, language)))
	}
	return from, cmp.Or(to, strings.TrimSpace(language)), nil
}

// ready waits on the engine's goroutine until the plugin runs, pluginsTranslateWait at most while it is pending (as at launch), and returns its translator as it declares it now; it fails as not running for one gone or unable to run, or still pending once the wait is over, and with ctx's error, before anything is sent, once ctx ends.
func (t pluginsTranslator) ready(ctx context.Context) (plugins.Translator, error) {
	type answer struct {
		tr  plugins.Translator
		err error
	}
	got := make(chan answer, 1)
	waitUntil := time.Now().Add(pluginsTranslateWait)
	var try func()
	try = func() {
		if err := ctx.Err(); err != nil {
			got <- answer{err: err}
			return
		}
		c, state := t.a.pluginsTranslatorState(t.plugin, t.id)
		switch {
		case state == sourceActive:
			got <- answer{tr: c.of}
		case state == sourcePending && time.Now().Before(waitUntil):
			time.AfterFunc(pluginsTranslatePoll, func() { t.a.update(try) })
		default:
			got <- answer{err: plugins.NewTranslateError(plugins.TranslateNotRunning, "")}
		}
	}
	t.a.update(try)
	select {
	case r := <-got:
		return r.tr, r.err
	case <-ctx.Done():
		return plugins.Translator{}, ctx.Err()
	}
}

// call calls the function export of the plugin with args on the engine's goroutine and returns what it answered, or its failure as a plugins.TranslateError: one with a code it rejected with, one that is not running, or what it said. It gives up as ctx ends, before the plugin is called when ctx ended first, or after pluginsTranslateTimeout.
func (t pluginsTranslator) call(ctx context.Context, export string, args []any) (any, error) {
	type answer struct {
		v   any
		err error
	}
	got := make(chan answer, 1)
	t.a.update(func() {
		if err := ctx.Err(); err != nil {
			got <- answer{err: err}
			return
		}
		t.a.pluginsContribCall(t.plugin, export, args, func(v any, err error) { got <- answer{v, pluginsTranslateErr(err)} })
	})
	timer := time.NewTimer(pluginsTranslateTimeout)
	defer timer.Stop()
	select {
	case r := <-got:
		return r.v, r.err
	case <-ctx.Done():
		return nil, ctx.Err()
	case <-timer.C:
		return nil, plugins.NewTranslateError(plugins.TranslateUnavailable, "the plugin did not answer in time")
	}
}

// pluginsTranslateErr is err, a translator's call's failure, as a plugins.TranslateError: a plugin not running or stopped as not running, a rejection with a code as that code, else what it said; nil for none.
func pluginsTranslateErr(err error) error {
	var coded *ns.Error
	switch {
	case err == nil:
		return nil
	case errors.Is(err, errPluginsNotRunning), errors.Is(err, script.ErrStopped):
		return plugins.NewTranslateError(plugins.TranslateNotRunning, "")
	case errors.As(err, &coded) && coded.Code != "":
		return plugins.NewTranslateError(coded.Code, coded.Message)
	}
	return plugins.NewTranslateError("", err.Error())
}

// pluginsCandidater is a translator whose plugin exports candidates, which offers other translations of a line (media.Tools.Candidates).
type pluginsCandidater struct{ pluginsTranslator }

// Candidates asks the plugin's candidates(line, from, to, context) for other translations of req.Line, once it runs (ready): line its original, from the language the film is spoken in as a language tag (plugins.SpokenTag), "" when not known, to the target's language tag, context {translator, current, before, after}, the line's current translation and the originals of the lines around it. A target or a spoken language the translator does not take fails as plugins.TranslateLanguage before anything is sent.
func (t pluginsCandidater) Candidates(ctx context.Context, req media.CandidateRequest) ([]string, error) {
	tr, err := t.ready(ctx)
	if err != nil {
		return nil, err
	}
	from, to, err := pluginsTranslatePair(tr, plugins.SpokenTag(t.spoken), req.Language)
	if err != nil {
		return nil, err
	}
	originals := func(ls []kcore.Line) []any {
		out := make([]any, len(ls))
		for i, l := range ls {
			out[i] = l.Original
		}
		return out
	}
	extra := map[string]any{"translator": t.id, "current": req.Line.Translation, "before": originals(req.Before), "after": originals(req.After)}
	v, err := t.call(ctx, plugins.CandidatesExport, []any{req.Line.Original, from, to, extra})
	if err != nil {
		return nil, err
	}
	got, ok := plugins.CandidateLines(v)
	if !ok {
		return nil, plugins.NewTranslateError(plugins.TranslateBadReply, "")
	}
	return got, nil
}
