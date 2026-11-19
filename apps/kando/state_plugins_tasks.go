package main

import (
	"cmp"
	"context"
	"errors"
	"net/http"
	"slices"
	"strings"
	"time"

	"kando/apps/kando/film"
	"kando/apps/kando/lines"
	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	cmdpalette "kando/apps/kando/palette"
	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/task"
	"kando/apps/kando/settings"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
	nsdesktop "github.com/nsgo-dev/nsgo/desktop"
	"github.com/nsgo-dev/nsgo/i18n"
)

// Task packs (docs/plugin-system.md E6): a pack the user installed and turned on asks the user's AI service its own question about the line playing, the word looked up or the film as far as it was watched, from the command palette, the 学习 menu and the player's ⋯ (TaskMenu), and the side panel shows the reply in place of the subtitle list, as Explain Line does. Only the lines the learner has reached are sent (task.Request). A pack spends the user's tokens, so it is installed off (plugins.State.Enabled) and runs only within its daily budget (plugins.State.Budget), which the tokens each run took, as the service counted them or as estimated (media.ChatWith), count against; what each pack spent today is kept in the plugins folder's usage.json, so a restart does not give it its budget again.

// pluginsTask is the run of a task pack the side panel shows (film.TaskResult): the pack's id, name, fields and how they show, as it was when it ran; the film's video; what it ran on, the line or the word (subject), or how far the film was watched (horizon); busy while the AI service is asked, failed when no reply came, with say saying why and fix the settings that help ("ai" or "plugins", "" for none); the reply's items, the model that wrote it and whether it ran on this computer, and the tokens it took.
type pluginsTask struct {
	open         bool
	id           string
	name         plugins.Text
	fields       []plugins.TaskField
	render       string
	video        string
	subject      string
	horizon      time.Duration
	scope        string
	busy, failed bool
	say          func(*ns.Scope) string
	fix          string
	items        []task.Item
	model        string
	spent        int
}

const (
	// pluginsTaskTimeout is how long a task's run may take.
	pluginsTaskTimeout = 90 * time.Second
	// pluginsTaskBefore is how many lines before the line or the word's line go with it, as Explain Line sends.
	pluginsTaskBefore = explainBefore
)

// pluginsTaskKeys are the task packs turned on, a row each of the 学习 menu and the command palette, which runs it; a row shows while there is something to run it on (pluginsTaskCan).
func (a *app) pluginsTaskKeys(cx *ns.Scope) []shortcut {
	var out []shortcut
	lang := i18n.Language(cx)
	for _, pack := range a.pluginsOn(plugins.KindTask) {
		id, scope := pack.ID, pack.Task.Scope
		out = append(out, shortcut{Group: groupStudy, Label: locales.PluginsTaskCommand(cx, pack.Name.In(lang)),
			When: func() bool { return a.pluginsTaskCan(scope) }, Run: func() { a.pluginsRunTask(id) }})
	}
	return out
}

// TaskMenu is the task packs that can run now, an item each, which runs it, as the player's ⋯ lists them (film.Tasker); the same rows as the 学习 menu's.
func (a *app) TaskMenu(cx *ns.Scope) []nsdesktop.MenuItem {
	var out []nsdesktop.MenuItem
	for _, row := range a.pluginsTaskKeys(cx) {
		if row.When() {
			out = append(out, nsdesktop.MenuItem{Label: row.Label, OnSelect: live.Do(a, row.Run)})
		}
	}
	return out
}

// pluginsTaskCan reports whether a task of scope can run now: on a film open in the player with its lines read, and for a word while a word looked up shows its card.
func (a *app) pluginsTaskCan(scope string) bool {
	if a.openKey == "" || a.player.film.video == "" || len(a.playerCues()) == 0 {
		return false
	}
	return scope != plugins.ScopeWord || a.vocab.lookup.open
}

// pluginsRunTask runs the task pack id on what its scope names, as its row of the palette or the 学习 menu does: the film holds, and the side panel, shown if it was hidden, says what the AI service set in 「AI 服务」 answers, asked off the engine's goroutine with the pack's prompt and the host's rules (task.Prompt). The line is the one selected for correcting, else the one playing, or between lines the one just heard, as I picks for Explain Line; the word is the one whose card shows; the film so far is its lines up to the furthest point played. Without an AI service that can answer, past the pack's budget for today, or with nothing watched yet, the panel says why.
func (a *app) pluginsRunTask(id string) {
	i := slices.IndexFunc(a.pluginsOn(plugins.KindTask), func(p plugins.Pack) bool { return p.ID == id })
	item, ok := a.openFilm()
	cues := a.playerCues()
	if i < 0 || !ok || a.player.film.video == "" || len(cues) == 0 {
		return
	}
	pack := a.pluginsOn(plugins.KindTask)[i]
	t := *pack.Task
	unheard := false
	run := pluginsTask{open: true, id: id, name: pack.Name, fields: t.Reply.Fields, render: t.Render, video: a.player.film.video, scope: t.Scope}
	in := task.Input{Film: cmp.Or(item.Name, item.Title)}
	switch t.Scope {
	case plugins.ScopeWord:
		l := a.vocab.lookup
		if !l.open || l.cue < 0 || l.cue >= len(cues) {
			return
		}
		in.Word, run.subject = l.word, l.word
		in.Line, in.Before = task.LineAt(cues, l.cue, pluginsTaskBefore)
	case plugins.ScopeLine:
		line := a.correctSelected()
		if line < 0 {
			prev, cur, next := film.CueNear(cues, a.playerLineAt())
			for _, i := range []int{next, prev, cur} {
				if i >= 0 {
					line = i
				}
			}
		}
		if line < 0 {
			return
		}
		in.Line, in.Before = task.LineAt(cues, line, pluginsTaskBefore)
		run.subject = cmp.Or(in.Line.Original, in.Line.Translation)
		// A line selected past the furthest point played, or the next one before the first is heard, would give away what is to come.
		unheard = !lines.Seen(in.Line.Start, max(a.palette.furthest[run.video], a.playerLineAt()))
	default:
		run.horizon = a.palette.furthest[run.video]
		for _, cue := range cues {
			if run.horizon <= 0 || !lines.Seen(cue.Start, run.horizon) {
				break
			}
			translation, original := lines.Split(cue.Text)
			in.Seen = append(in.Seen, lines.New(cue.Start, translation, original))
		}
	}
	if p := a.Play(); p != nil && p.Playing() {
		p.Toggle()
	}
	a.pluginsTaskStop()
	a.player.film.styleView = false
	a.player.panel = true
	a.player.filmSide = film.SideTask
	a.plugins.task = run
	r := &a.plugins.task
	switch {
	case t.Scope == plugins.ScopeFilm && len(in.Seen) == 0:
		r.failed, r.say = true, locales.PaletteNotWatched
		return
	case unheard:
		r.failed, r.say = true, locales.PluginsTaskNotHeard
		return
	}
	tr, why := a.paletteTranslator(locales.PluginsTaskNoAI)
	if why != nil {
		r.failed, r.say, r.fix = true, why, "ai"
		return
	}
	budget := a.plugins.state.Budget(id)
	left := budget - a.pluginsSpent(id)
	if left <= 0 {
		r.failed, r.fix = true, "plugins"
		r.say = func(cx *ns.Scope) string { return locales.PluginsTaskOverBudget(cx, budget) }
		return
	}
	system := task.Prompt(t, paletteAnswerLanguage(item, a.prefs.language))
	// The run may take what its prompt and the longest reply it allows take; the film so far keeps only its latest lines that fit what is left of the budget, a line no more than a token a character (media.EstimateTokens), and a run that does not fit even so is not sent.
	if t.Scope == plugins.ScopeFilm {
		room := left - t.Tokens() - media.EstimateTokens(system) - media.EstimateTokens(task.Request(t, task.Input{Film: in.Film}))
		in.Seen = task.Trim(in.Seen, max(min(room, task.ContextRunes), 0))
	}
	user := task.Request(t, in)
	if need := media.EstimateTokens(system) + media.EstimateTokens(user) + t.Tokens(); need > left || (t.Scope == plugins.ScopeFilm && len(in.Seen) == 0) {
		r.failed, r.fix = true, "plugins"
		r.say = func(cx *ns.Scope) string { return locales.PluginsTaskTooCostly(cx, need, left) }
		return
	}
	model := strings.TrimSpace(tr.Model)
	r.model, r.busy = model, true
	r.say = func(cx *ns.Scope) string { return locales.PluginsTaskBusy(cx, model) }
	req := media.ChatRequest{System: system, User: user, MaxTokens: t.Tokens(), Header: http.Header{"X-Title": {"Kando plugin " + id}}}
	a.plugins.taskGen++
	gen, update := a.plugins.taskGen, a.update
	// answered is the reply's items and the tokens it took.
	type answered struct {
		items  []task.Item
		tokens int
	}
	ask := func(ctx context.Context) (answered, error) {
		reply, err := tr.ChatWith(ctx, req)
		// The tokens are the user's however the reply reads, whether or not the panel still waits for it, and for a request that failed once it reached the service too (media.ChatWith counts it), so they are counted apart from the reply.
		tokens := reply.Usage.Total()
		if tokens > 0 {
			update(func() { a.pluginsSpend(id, tokens) })
		}
		if err != nil {
			return answered{tokens: tokens}, err
		}
		items, err := task.Parse(t, reply.Text)
		return answered{items, tokens}, err
	}
	a.plugins.taskCancel = aiAsk(a, pluginsTaskTimeout, ask, func() bool { return gen == a.plugins.taskGen && a.plugins.task.open }, func(got answered, err error) {
		r := &a.plugins.task
		a.plugins.taskCancel = nil
		r.busy, r.say, r.spent = false, nil, got.tokens
		switch {
		case errors.Is(err, media.ErrReplyCutOff):
			// The model reached the limit the pack's reply allows: the pack's limit, not the settings or a reply that reads wrong.
			limit := req.MaxTokens
			r.failed = true
			r.say = func(cx *ns.Scope) string { return locales.PluginsTaskCutOff(cx, limit) }
			return
		case err != nil:
			msg := err.Error()
			r.failed = true
			r.say = func(cx *ns.Scope) string { return locales.PluginsTaskFailed(cx, msg) }
			// A reply that is not what the pack asked for is the pack's or the model's, not the settings'.
			if !errors.Is(err, task.ErrEmpty) && !errors.Is(err, task.ErrMalformed) {
				r.fix = "ai"
			}
			return
		}
		r.items = got.items
	})
}

// pluginsTaskFollow closes the task's run once its film is not the one open, the side panel going back to the subtitle list, or once the panel shows something else in its place, as Explain Line or a tab, which stops a run no one sees; it reports whether it did.
func (a *app) pluginsTaskFollow() bool {
	r := a.plugins.task
	if !r.open || (a.openKey != "" && a.player.play != nil && a.player.film.video == r.video && a.player.filmSide == film.SideTask) {
		return false
	}
	a.pluginsTaskClose()
	return true
}

// pluginsTaskStop stops the run on its way, whose reply is then dropped; the tokens it took are still counted, as the service answered.
func (a *app) pluginsTaskStop() {
	a.plugins.taskGen++
	if a.plugins.taskCancel != nil {
		a.plugins.taskCancel()
		a.plugins.taskCancel = nil
	}
}

// pluginsTaskClose forgets the run and shows the subtitle list again where the panel showed it.
func (a *app) pluginsTaskClose() {
	a.pluginsTaskStop()
	a.plugins.task = pluginsTask{}
	if a.player.filmSide == film.SideTask {
		a.player.filmSide = ""
	}
}

// pluginsDay is today, as usage.json counts it: the local date.
func (a *app) pluginsDay() string { return a.saved.now().Format(time.DateOnly) }

// pluginsSpent is how many tokens the task pack id spent today.
func (a *app) pluginsSpent(id string) int { return a.plugins.usage.Spent(a.pluginsDay(), id) }

// pluginsSpend counts n tokens the task pack id spent today and keeps the count in usage.json; the file is small, and the vocabulary's cache is written on the engine's goroutine the same way.
func (a *app) pluginsSpend(id string, n int) {
	if n <= 0 {
		return
	}
	a.plugins.usage.Add(a.pluginsDay(), id, n)
	if root := a.pluginsRoot(); root != "" {
		if err := a.plugins.usage.Save(root); err != nil {
			a.plugins.err = err
		}
	}
}

// pluginsSetBudget sets the daily budget of tokens of the task pack id, or of the script plugin id that asks the AI service (kando.llm.chat), within plugins.MinBudget and plugins.MaxBudget, in state.json (pluginsApplyState); a pack without an entry keeps the switch it shows.
func (a *app) pluginsSetBudget(id string, tokens int) {
	i := slices.IndexFunc(a.plugins.packs, func(p plugins.Pack) bool { return p.ID == id })
	tokens = min(max(tokens, plugins.MinBudget), plugins.MaxBudget)
	if a.pluginsRoot() == "" || i < 0 || a.plugins.busy || tokens == a.plugins.state.Budget(id) {
		return
	}
	pack := a.plugins.packs[i]
	if pack.Kind == plugins.KindScript {
		// A script plugin's entry is for its content and signer (State.GrantEntry): setting its budget turns no content on the user did not.
		if pack.Signer == "" {
			return
		}
		e := a.plugins.state.GrantEntry(pack)
		e.Budget = tokens
		a.pluginsSaveState(a.pluginsNextState(func(s *plugins.State) { s.Set(e) }))
		return
	}
	on := a.plugins.state.EnabledPack(pack)
	next := a.pluginsNextState(func(s *plugins.State) {
		e, ok := s.Entry(id)
		if !ok {
			e.Enabled = on
		}
		e.ID, e.Hash, e.Budget = id, pack.Hash, tokens
		s.Set(e)
	})
	a.pluginsApplyState(next)
}

// TaskResult is the task's run, as the side panel shows it (film.Tasker).
func (a *app) TaskResult() film.TaskResult {
	r := a.plugins.task
	if !r.open {
		return film.TaskResult{}
	}
	out := film.TaskResult{Open: true, Busy: r.busy, Failed: r.failed, Say: r.say, Render: r.render,
		Title: func(cx *ns.Scope) string { return r.name.In(i18n.Language(cx)) },
		Labels: func(cx *ns.Scope) []string {
			labels := make([]string, len(r.fields))
			for i, f := range r.fields {
				labels[i] = f.Label.In(i18n.Language(cx))
			}
			return labels
		}}
	switch subject, horizon := r.subject, r.horizon; r.scope {
	case plugins.ScopeFilm:
		out.Subject = func(cx *ns.Scope) string { return locales.PluginsTaskSoFar(cx, cmdpalette.Clock(horizon)) }
	default:
		out.Subject = func(*ns.Scope) string { return subject }
	}
	switch r.fix {
	case "ai":
		out.Settings = locales.ExplainSettings
	case "plugins":
		out.Settings = locales.PluginsTaskOpenPlugins
	}
	for _, it := range r.items {
		out.Items = append(out.Items, []string(it))
	}
	if !r.busy && !r.failed {
		model, spent, id := r.model, r.spent, r.id
		out.Note = func(cx *ns.Scope) string {
			return locales.PluginsTaskNote(cx, model, spent, max(a.plugins.state.Budget(id)-a.pluginsSpent(id), 0))
		}
	}
	return out
}

// CloseTask is ‹ 字幕: the side panel goes back to the subtitle list (film.Tasker).
func (a *app) CloseTask() { a.pluginsTaskClose() }

// TaskSettings opens the settings that let the task run: the AI service's, or Settings › 插件 for a pack over its budget (film.Tasker).
func (a *app) TaskSettings() {
	if a.plugins.task.fix == "plugins" {
		a.OpenSettings(settings.SectionPlugins)
		return
	}
	a.OpenSettings(settings.SectionAI)
}
