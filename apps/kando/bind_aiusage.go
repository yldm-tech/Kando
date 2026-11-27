package main

import "kando/apps/kando/aiusage"

// The AI 服务 page's part of settings.App that shows the usage (state_aiusage.go).

func (a *app) AIUsage() aiusage.Month {
	return a.aiusage.ledger.Month(aiusage.MonthOf(a.aiusage.now()))
}
