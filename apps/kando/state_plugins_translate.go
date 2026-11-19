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

// savedTranslateSource is a script plugin's translator chosen as the translation source: the plugin's id, the translator's id, and its title as it was shown when it was chosen, which names it once its plugin is gone.
type savedTranslateSource struct {
	Plugin     string `json:"plugin"`
	Translator string `json:"translator"`
	Title      string `json:"title,omitempty"`
}

// valid reports whether s names a plugin and a translator as their ids can be, a title of a line at most.
func (s *savedTranslateSource) valid() bool {
	s.Title = strings.TrimSpace(s.Title)
	if len([]rune(s.Title)) > 200 || strings.ContainsAny(s.Title, "\r\n") {
		s.Title = ""
	}
	return s.Plugin != "" && s.Translator != "" && !strings.Contains(s.Plugin, ":") && !strings.Contains(s.Translator, ":")
}

// pluginsTranslateWait is how long a job's translation waits for the plugin of the translator chosen to load, as at startup, before it fails as not running; pluginsTranslatePoll how often it looks.
const (
	pluginsTranslateWait = 30 * time.Second
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

// pluginsTranslatorChosen is the translator chosen in Settings › 翻译 (prefs.translateSource), and whether one is chosen and there: its plugin installed, turned on and still adding it.
func (a *app) pluginsTranslatorChosen() (pluginsContribution[plugins.Translator], bool) {
	src := a.prefs.translateSource
	if src.Plugin == "" {
		return pluginsContribution[plugins.Translator]{}, false
	}
	for _, t := range a.pluginsTranslators() {
		if t.id == src.Plugin && t.of.ID == src.Translator {
			return t, true
		}
	}
	return pluginsContribution[plugins.Translator]{}, false
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
	t, ok := a.pluginsTranslatorChosen()
	if !ok {
		return ""
	}
	return pluginsContribID(t.id, t.of.ID)
}

// SetTranslateSource chooses the translator value names, "" for the AI service, and saves the settings; a value that names no translator of a plugin turned on changes nothing (settings.SourcesApp).
func (a *app) SetTranslateSource(value string) {
	next := savedTranslateSource{}
	if value != "" {
		i := slices.IndexFunc(a.pluginsTranslators(), func(t pluginsContribution[plugins.Translator]) bool { return pluginsContribID(t.id, t.of.ID) == value })
		if i < 0 {
			return
		}
		t := a.pluginsTranslators()[i]
		next = savedTranslateSource{Plugin: t.id, Translator: t.of.ID, Title: t.of.Title.In(a.messages().Language())}
	}
	if a.prefs.translateSource == next {
		return
	}
	a.prefs.translateSource, a.prefs.sourceGoneSaid = next, false
	a.Save()
}

// TranslateSourceGone is the name of the translator chosen once the plugins folder was read and its plugin is no longer installed and turned on, or no longer adds it, so the films are translated with the AI service; "" while there is none (settings.SourcesApp). It is named by its title as it was chosen, else by its plugin's id.
func (a *app) TranslateSourceGone() string {
	src := a.prefs.translateSource
	if src.Plugin == "" || !a.plugins.scanned {
		return ""
	}
	if _, ok := a.pluginsTranslatorChosen(); ok {
		return ""
	}
	return cmp.Or(src.Title, src.Plugin)
}

// pluginsTranslateFollow says in the message bar, once a run, that the translator chosen is gone (TranslateSourceGone), with Settings › 翻译 to choose another, and reports whether it did (settingsHooks' afterChange).
func (a *app) pluginsTranslateFollow() bool {
	gone := a.TranslateSourceGone()
	if gone == "" || a.prefs.sourceGoneSaid {
		return false
	}
	a.prefs.sourceGoneSaid = true
	a.warn(a.messages().SettingsTranslateSourceGone(gone), sectionid.Translate)
	return true
}

// pluginsTranslating reports whether a script plugin's translator translates the films now, in place of the AI service.
func (a *app) pluginsTranslating() bool {
	_, ok := a.pluginsTranslatorChosen()
	return ok
}

// pluginsTranslateTools is tools translating with the translator chosen (media.Tools.Engine), named by its title in the interface's language, for a film spoken in spoken (whisper's code, "" when not known), which candidates are asked from; tools as they are when none is chosen or it is gone. Its candidates are asked of the plugin only while it runs and exports candidates; otherwise of the AI service.
func (a *app) pluginsTranslateTools(tools media.Tools, spoken string) media.Tools {
	t, ok := a.pluginsTranslatorChosen()
	if !ok {
		return tools
	}
	engine := pluginsTranslator{a: a, plugin: t.id, tr: t.of, spoken: spoken}
	tools.Engine, tools.EngineName = engine, t.of.Title.In(a.messages().Language())
	if l := a.plugins.script.loaded[t.id]; l != nil && l.pack.Candidater(l.plugin.Exports()) {
		tools.Engine = pluginsCandidater{engine}
	}
	return tools
}

// pluginsTranslator is a script plugin's translator as a translation source (media.SegmentTranslator), which a job's goroutine calls: each call goes to the engine's goroutine through update, to the plugin through the runner.
type pluginsTranslator struct {
	a      *app
	plugin string
	tr     plugins.Translator
	// spoken is the language the film is spoken in for Candidates, whisper's code, "" when not known.
	spoken string
}

// Local reports whether the translator runs on this computer, which offline mode keeps (media.Tools.Engine).
func (t pluginsTranslator) Local() bool { return t.tr.Local }

// Translate translates segments into language with the plugin's translate(lines, from, to, context), at most plugins.TranslateBatch lines a call: lines the segments' texts, from the language the film is heard in (media.SpeechLanguageOf), "" when not known, to the target's language tag, or language as it is for a target Kando has no tag for, given to a translator of any language alone; context {translator}, the translator's id. Each answer must be as many lines, in their order (plugins.TranslatedLines). A target or a spoken language the translator does not take fails as plugins.TranslateLanguage before anything is sent.
func (t pluginsTranslator) Translate(ctx context.Context, segments []kcore.Segment, language string) ([]kcore.Segment, error) {
	from, to := media.SpeechLanguageOf(ctx), kcore.TargetTag(language)
	if !t.tr.Takes(from, to) {
		return nil, plugins.NewTranslateError(plugins.TranslateLanguage, strings.TrimSpace(from+" → "+cmp.Or(to, language)))
	}
	if to == "" {
		to = strings.TrimSpace(language)
	}
	out := make([]kcore.Segment, 0, len(segments))
	for start := 0; start < len(segments); start += plugins.TranslateBatch {
		batch := segments[start:min(start+plugins.TranslateBatch, len(segments))]
		lines := make([]any, len(batch))
		for i, s := range batch {
			lines[i] = s.Text
		}
		v, err := t.call(ctx, plugins.TranslateExport, []any{lines, from, to, map[string]any{"translator": t.tr.ID}})
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

// call calls the function export of the plugin with args on the engine's goroutine, once it runs, waiting pluginsTranslateWait at most for it to load while it is turned on, and returns what it answered, or its failure as a plugins.TranslateError: one with a code it rejected with, one that is not running, or what it said. It gives up as ctx ends, or after pluginsTranslateTimeout.
func (t pluginsTranslator) call(ctx context.Context, export string, args []any) (any, error) {
	type answer struct {
		v   any
		err error
	}
	got := make(chan answer, 1)
	waitUntil := time.Now().Add(pluginsTranslateWait)
	var try func()
	try = func() {
		a := t.a
		on := slices.ContainsFunc(a.pluginsTranslators(), func(c pluginsContribution[plugins.Translator]) bool { return c.id == t.plugin })
		if a.plugins.script.loaded[t.plugin] == nil && on && time.Now().Before(waitUntil) && ctx.Err() == nil {
			time.AfterFunc(pluginsTranslatePoll, func() { a.update(try) })
			return
		}
		a.pluginsContribCall(t.plugin, export, args, func(v any, err error) { got <- answer{v, err} })
	}
	t.a.update(try)
	timer := time.NewTimer(pluginsTranslateTimeout)
	defer timer.Stop()
	select {
	case r := <-got:
		return r.v, pluginsTranslateErr(r.err)
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

// Candidates asks the plugin's candidates(line, from, to, context) for other translations of req.Line: line its original, from the language the film is spoken in, "" when not known, to the target's language tag, context {translator, current, before, after}, the line's current translation and the originals of the lines around it. A target or a spoken language the translator does not take fails as plugins.TranslateLanguage before anything is sent.
func (t pluginsCandidater) Candidates(ctx context.Context, req media.CandidateRequest) ([]string, error) {
	from, to := t.spoken, kcore.TargetTag(req.Language)
	if !t.tr.Takes(from, to) {
		return nil, plugins.NewTranslateError(plugins.TranslateLanguage, strings.TrimSpace(from+" → "+cmp.Or(to, req.Language)))
	}
	if to == "" {
		to = strings.TrimSpace(req.Language)
	}
	originals := func(ls []kcore.Line) []any {
		out := make([]any, len(ls))
		for i, l := range ls {
			out[i] = l.Original
		}
		return out
	}
	extra := map[string]any{"translator": t.tr.ID, "current": req.Line.Translation, "before": originals(req.Before), "after": originals(req.After)}
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
