package main

import (
	"log"
	"path/filepath"
	"time"

	"kando/apps/kando/aiusage"
	"kando/packages/media"
)

// aiusageUI is the ledger of the AI services' usage, month by month (package aiusage), which Settings › AI 服务 shows for the month under way. The translator tells every request it sends (media.ChatTranslator.Meter), from whichever goroutine sent it; the report comes back to the engine's goroutine through update, the ledger's only writer, which writes ai-usage.json beside settings.json after each. It stays on this computer.
type aiusageUI struct {
	ledger aiusage.Ledger
	// path is the ledger's file, "" for an app without settings.json, as a test's, which keeps the ledger in memory alone.
	path string
	// now is the clock that says which month a request counts in.
	now func() time.Time
}

func newAIUsageUI() aiusageUI { return aiusageUI{now: time.Now} }

// aiusageHooks: the ledger is read at startup, and the pipeline's translator is given the meter then, before any job or request; every change of the tools starts from the pipeline's, so they keep it.
func (a *app) aiusageHooks() areaHooks {
	return areaHooks{
		restore: func(savedSettings) {
			a.aiusageLoad()
			a.aiusageAttach()
		},
	}
}

// aiusageLoad reads the ledger beside settings.json. One that cannot be read starts empty, and the next request writes over it.
func (a *app) aiusageLoad() {
	if a.settingsPath == "" {
		return
	}
	a.aiusage.path = filepath.Join(filepath.Dir(a.settingsPath), aiusage.FileName)
	ledger, err := aiusage.Load(a.aiusage.path)
	if err != nil {
		log.Printf("aiusage: %v", err)
	}
	a.aiusage.ledger = ledger
}

// aiusageAttach gives the pipeline's translator the meter (aiusageMeter).
func (a *app) aiusageAttach() {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return
	}
	tools.Translator.Meter = a.aiusageMeter
	a.pipeline = tools
}

// aiusageMeter is the translator's meter: it may run on any goroutine, so it hands the request to the engine's (update), where aiusageRecord counts it.
func (a *app) aiusageMeter(pass, host string, u media.ChatUsage) {
	a.update(func() { a.aiusageRecord(pass, host, u) })
}

// aiusageRecord counts one request in the month under way and writes the ledger, on the engine's goroutine.
func (a *app) aiusageRecord(pass, host string, u media.ChatUsage) {
	a.aiusage.ledger.Add(aiusage.MonthOf(a.aiusage.now()), pass, host, u.Prompt, u.Completion, u.Estimated)
	if a.aiusage.path == "" {
		return
	}
	if err := a.aiusage.ledger.Save(a.aiusage.path); err != nil {
		log.Printf("aiusage: %v", err)
	}
}
