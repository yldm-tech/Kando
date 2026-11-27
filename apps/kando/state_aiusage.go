package main

import (
	"errors"
	"log"
	"os"
	"path/filepath"
	"sync/atomic"
	"time"

	"kando/apps/kando/aiusage"
	"kando/apps/kando/locales"
	"kando/packages/core"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
)

// aiusageUI is the ledger of the AI services' usage, month by month (package aiusage), which Settings › AI 服务 shows for the month under way. The translator tells every request it sends (media.ChatTranslator.Meter), from whichever goroutine sent it; the report comes back to the engine's goroutine through update, the ledger's only writer, which writes ai-usage.json beside settings.json after each. It stays on this computer.
//
// The user may set a monthly limit of tokens (savedAIUsage), off by default, for every service alike. The translator asks it before each request (media.ChatTranslator.Budget), from the goroutine that would send it, which reads limit's copy, never the ledger: the engine's goroutine puts a new copy there whenever the limit or the month's tokens change (aiusageShare). A request that would go over it is not sent (media.ErrMonthlyCap): a job's translation stops there, as offline mode stops it, and the film is made without it, its card saying why and offering 「AI 服务」; raising the limit, or turning it off, translates again the films it stopped (aiusageResume).
type aiusageUI struct {
	ledger aiusage.Ledger
	// path is the ledger's file, "" for an app without settings.json, as a test's, or for a damaged file that could not be copied aside, which keeps the ledger in memory alone.
	path string
	// now is the clock that says which month a request counts in.
	now func() time.Time
	// tokens is the monthly limit of tokens the user set, 0 for none (aiusage.CleanLimit).
	tokens int64
	// limit is what the translator's Budget reads, from any goroutine: the limit and the tokens counted in the month it was taken in.
	limit *atomic.Pointer[aiusage.Limit]
}

func newAIUsageUI() aiusageUI {
	return aiusageUI{now: time.Now, limit: new(atomic.Pointer[aiusage.Limit])}
}

// savedAIUsage is the monthly limit of tokens, 0 for none.
type savedAIUsage struct {
	AIUsageLimit int64 `json:"aiUsageMonthlyLimit,omitempty"`
}

func (s *savedAIUsage) clean() { s.AIUsageLimit = aiusage.CleanLimit(s.AIUsageLimit) }

// aiusageHooks: the ledger and the limit are read at startup, and the pipeline's translator is given the meter and the limit then, before any job or request; every change of the tools starts from the pipeline's, so they keep them.
func (a *app) aiusageHooks() areaHooks {
	return areaHooks{
		save: func(saved *savedSettings) { saved.savedAIUsage = savedAIUsage{AIUsageLimit: a.aiusage.tokens} },
		restore: func(saved savedSettings) {
			a.aiusage.tokens = aiusage.CleanLimit(saved.AIUsageLimit)
			a.aiusageLoad()
			a.aiusageAttach()
		},
	}
}

// aiusageLoad reads the ledger beside settings.json. One that cannot be read starts empty, and the file is copied to ai-usage.json.bad first, as settings.json is, so the next request's save does not lose the months it held; when even that copy cannot be made, the ledger stays in memory and the file is left as it is.
func (a *app) aiusageLoad() {
	if a.settingsPath == "" {
		return
	}
	path := filepath.Join(filepath.Dir(a.settingsPath), aiusage.FileName)
	ledger, err := aiusage.Load(path)
	a.aiusage.ledger = ledger
	if err == nil {
		a.aiusage.path = path
		return
	}
	log.Printf("aiusage: %v", err)
	data, err := os.ReadFile(path)
	if err == nil {
		err = os.WriteFile(path+".bad", data, 0o600)
	}
	if err != nil {
		log.Printf("aiusage: %v; the ledger is not saved this time", err)
		return
	}
	a.aiusage.path = path
}

// aiusageAttach gives the pipeline's translator the meter and the limit. The meter may run on any goroutine, so it hands the request to the engine's through update, read here on the engine's goroutine rather than from wherever the meter runs, where aiusageRecord counts it. The limit is asked on any goroutine too, so it reads the copy aiusageShare keeps, with the clock taken here.
func (a *app) aiusageAttach() {
	a.aiusageShare()
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return
	}
	update := a.update
	tools.Translator.Meter = func(pass, host string, u media.ChatUsage) {
		update(func() { a.aiusageRecord(pass, host, u) })
	}
	limit, now := a.aiusage.limit, a.aiusage.now
	tools.Translator.Budget = func(_ string, estimate int) error {
		if l := limit.Load(); l != nil && l.Refuses(aiusage.MonthOf(now()), estimate) {
			return media.ErrMonthlyCap
		}
		return nil
	}
	a.pipeline = tools
}

// aiusageShare puts the limit and this month's tokens where the translator's Budget reads them. Engine goroutine only.
func (a *app) aiusageShare() {
	month := aiusage.MonthOf(a.aiusage.now())
	a.aiusage.limit.Store(&aiusage.Limit{Month: month, Used: a.aiusage.ledger.Month(month).Total().Tokens(), Tokens: a.aiusage.tokens})
}

// aiusageRecord counts one request in the month under way and writes the ledger, on the engine's goroutine.
func (a *app) aiusageRecord(pass, host string, u media.ChatUsage) {
	a.aiusage.ledger.Add(aiusage.MonthOf(a.aiusage.now()), pass, host, u.Prompt, u.Completion, u.Estimated)
	a.aiusageShare()
	if a.aiusage.path == "" {
		return
	}
	if err := a.aiusage.ledger.Save(a.aiusage.path); err != nil {
		log.Printf("aiusage: %v", err)
	}
}

// aiusageUseLimit sets the monthly limit to tokens (aiusage.CleanLimit) and saves it; a limit raised or turned off translates again the films the old one stopped (aiusageResume), which stop again quietly if it still refuses them.
func (a *app) aiusageUseLimit(tokens int64) {
	tokens = aiusage.CleanLimit(tokens)
	if tokens == a.aiusage.tokens {
		return
	}
	was := a.aiusage.tokens
	a.aiusage.tokens = tokens
	a.aiusageShare()
	if err := a.saveSettings(); err != nil {
		a.saveErr = a.messages().SettingsSaveFailed(err.Error())
	}
	if was > 0 && (tokens == 0 || tokens > was) {
		a.aiusageResume()
	}
}

// aiusageResume queues, behind the jobs waiting, the done films of the open library the limit stopped, to translate their lines left (core.RedoMissing), or translate them again as their redo asked (core.RedoTranslation): those made without their translation from where the limit refused it (core.Job.TranslationErr), and those whose translation again since launch ended at it (jobQueue.redoEnded). They end quietly, as a refill does.
func (a *app) aiusageResume() {
	for _, job := range a.library.Jobs {
		redo, into, ok := a.aiusageStopped(job)
		if !ok || job.ID == a.queue.active {
			continue
		}
		ready, err := a.queueRetranslatable(job, redo, into)
		if err != nil {
			continue
		}
		if a.queue.refills == nil {
			a.queue.refills = map[string]bool{}
		}
		a.queue.refills[job.ID] = true
		delete(a.lib.quotes, job.SubtitlePath)
		a.queueAdd(ready)
	}
}

// aiusageStopped reports whether job is a done film the monthly limit stopped, and how to translate it again: its lines left for one made without them, or as the redo that ended at the limit asked.
func (a *app) aiusageStopped(job core.Job) (redo core.Redo, into string, ok bool) {
	if job.Stage != core.StageComplete || job.SameLanguage || job.TranslationSource != "" {
		return "", "", false
	}
	if media.ClassifyTranslate(job.TranslationErr) == media.TranslateCap {
		return core.RedoMissing, "", true
	}
	ended, was := a.queue.redoEnded[job.ID]
	if !was || ended.Err == nil || media.ClassifyTranslate(ended.Err.Error()) != media.TranslateCap {
		return "", "", false
	}
	switch ended.Redo {
	case core.RedoMissing:
		return core.RedoMissing, "", job.Untranslated > 0
	case core.RedoTranslation:
		return core.RedoTranslation, ended.RedoLanguage, true
	}
	return "", "", false
}

// aiusageSay is say of what err says, in the language shown when it is drawn, or, for a request the monthly limit refused (media.ErrMonthlyCap), what the limit says instead, as an explanation, an answer, a lookup or a task's card says why it did not come.
func aiusageSay(err error, say func(cx *ns.Scope, msg string) string) func(cx *ns.Scope) string {
	if errors.Is(err, media.ErrMonthlyCap) {
		return locales.AiusageCapReached
	}
	msg := err.Error()
	return func(cx *ns.Scope) string { return say(cx, msg) }
}
