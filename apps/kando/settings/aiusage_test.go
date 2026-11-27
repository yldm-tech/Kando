package settings

import (
	"testing"

	"kando/apps/kando/aiusage"
	"kando/apps/kando/locales/testlocale"
)

// 本月用量 lists the passes that sent something this month, in their order, with their tokens and the requests of the month, and follows the month as requests are counted.
func TestAIUsageShowsThePassesThatSentSomething(t *testing.T) {
	a := newFakeApp()
	a.section = SectionAI
	a.usage = aiusage.Month{Month: "2026-11", ByPass: map[string]aiusage.Count{
		"translate": {Prompt: 1000, Completion: 234, Requests: 3},
		"chat":      {Prompt: 50, Completion: 10, Requests: 1},
	}}
	app := start(t, a)
	msgs := testlocale.Messages()
	for _, id := range []string{"settings-aiusage", "settings-aiusage-translate", "settings-aiusage-chat", "settings-aiusage-requests"} {
		if !app.Exists("id:" + id) {
			t.Fatalf("no %s: %q", id, app.Texts())
		}
	}
	for _, id := range []string{"settings-aiusage-proofread", "settings-aiusage-review", "settings-aiusage-suggest", "settings-aiusage-plugin", "settings-aiusage-none"} {
		if app.Exists("id:" + id) {
			t.Fatalf("%s shows: %q", id, app.Texts())
		}
	}
	if app.Find("id:settings-aiusage-translate").Frame.Origin.Y >= app.Find("id:settings-aiusage-chat").Frame.Origin.Y {
		t.Fatal("chat comes before translate")
	}
	for _, text := range []string{msgs.AiusageTitle(), msgs.AiusagePassTranslate(), msgs.AiusageTokens(1234), msgs.AiusagePassChat(), msgs.AiusageTokens(60), msgs.AiusageRequests(), msgs.AiusageRequestsCount(4)} {
		if !app.Exists(whole(text)) {
			t.Fatalf("no %q: %q", text, app.Texts())
		}
	}
	if app.Exists(whole(msgs.AiusageEstimatedNote())) {
		t.Fatalf("the note shows with nothing estimated: %q", app.Texts())
	}
	change(app, a, func() {
		c := a.usage.ByPass["translate"]
		c.Prompt, c.Requests = c.Prompt+1000, c.Requests+1
		a.usage.ByPass["translate"] = c
	})
	if !app.Exists(whole(msgs.AiusageTokens(2234))) || !app.Exists(whole(msgs.AiusageRequestsCount(5))) {
		t.Fatalf("the counts did not follow: %q", app.Texts())
	}
}

// A count with tokens Kando estimated starts with ≈, and the note under the card says what it means.
func TestAIUsageMarksEstimates(t *testing.T) {
	a := newFakeApp()
	a.section = SectionAI
	a.usage = aiusage.Month{Month: "2026-11", ByPass: map[string]aiusage.Count{
		"translate": {Prompt: 1000, Completion: 200, Requests: 2},
		"plugin":    {Prompt: 300, Completion: 40, Requests: 1, Estimated: 340},
	}}
	app := start(t, a)
	msgs := testlocale.Messages()
	for _, text := range []string{msgs.AiusageTokens(1200), msgs.AiusageTokensEstimated(340), msgs.AiusageEstimatedNote()} {
		if !app.Exists(whole(text)) {
			t.Fatalf("no %q: %q", text, app.Texts())
		}
	}
}

// A month with nothing sent says so in one row, and the card lists the passes once one sends something.
func TestAIUsageSaysWhenNothingWasSent(t *testing.T) {
	a := newFakeApp()
	a.section = SectionAI
	app := start(t, a)
	msgs := testlocale.Messages()
	if !app.Exists("id:settings-aiusage-none") || !app.Exists(whole(msgs.AiusageNone())) || app.Exists("id:settings-aiusage-requests") {
		t.Fatalf("texts %q", app.Texts())
	}
	change(app, a, func() {
		a.usage = aiusage.Month{Month: "2026-11", ByPass: map[string]aiusage.Count{"review": {Prompt: 10, Completion: 5, Requests: 1}}}
	})
	if app.Exists("id:settings-aiusage-none") || !app.Exists("id:settings-aiusage-review") || !app.Exists(whole(msgs.AiusageTokens(15))) {
		t.Fatalf("texts %q", app.Texts())
	}
}
