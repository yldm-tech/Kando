package main

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"kando/apps/kando/aiusage"
	"kando/packages/media"
)

// aiusageUI is the ledger of the AI services' usage, month by month (package aiusage), which Settings › AI 服务 shows for the month under way. The translator tells every request it sends (media.ChatTranslator.Meter), from whichever goroutine sent it; the report comes back to the engine's goroutine through update, the ledger's only writer, which writes ai-usage.json beside settings.json after each. It stays on this computer.
type aiusageUI struct {
	ledger aiusage.Ledger
	// path is the ledger's file, "" for an app without settings.json, as a test's, or for a damaged file that could not be copied aside, which keeps the ledger in memory alone.
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

// aiusageAttach gives the pipeline's translator the meter. The meter may run on any goroutine, so it hands the request to the engine's through update, read here on the engine's goroutine rather than from wherever the meter runs, where aiusageRecord counts it.
func (a *app) aiusageAttach() {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return
	}
	update := a.update
	tools.Translator.Meter = func(pass, host string, u media.ChatUsage) {
		update(func() { a.aiusageRecord(pass, host, u) })
	}
	a.pipeline = tools
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
