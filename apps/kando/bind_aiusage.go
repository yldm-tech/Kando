package main

import "kando/apps/kando/aiusage"

// The AI 服务 page's part of settings.App that shows the usage and sets the monthly limit (state_aiusage.go).

func (a *app) AIUsage() aiusage.Month {
	return a.aiusage.ledger.Month(aiusage.MonthOf(a.aiusage.now()))
}

func (a *app) AIUsageLimit() int64 { return a.aiusage.tokens }

func (a *app) SetAIUsageLimit(tokens int64) { a.aiusageUseLimit(tokens) }
