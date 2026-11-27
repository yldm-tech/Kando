package main

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/aiusage"
	"kando/apps/kando/films"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/lookup"
	"kando/packages/core"
	"kando/packages/media"
	psync "kando/packages/sync"
)

// A paired phone looks a word up and has a line explained through the desktop: without an AI service the desktop says so; with one it asks it, as the picture's lookup and Explain Line do, in the film's language and with its title, keeps the word among the words looked up here, and answers it again from there.
func TestPhoneSyncLooksUpAndExplainsThroughTheAIServiceSet(t *testing.T) {
	a, tt, updates, dir, _ := phoneSyncApp(t)
	phoneSyncOn(t, a, tt, updates)
	c := phoneClient(t, a, "")
	if _, err := phonePair(t, a, tt, updates, c, a.phone.offer.Code); err != nil {
		t.Fatal(err)
	}
	h, err := phoneCall(t, a, tt, updates, c.Hello)
	if err != nil || !slices.Contains(h.Ops, psync.OpLookUp) || !slices.Contains(h.Ops, psync.OpExplain) {
		t.Fatalf("hello %+v, %v", h, err)
	}
	req := psync.WordLookup{Library: phoneLibraryID(dir), Film: "yt", Word: "Hello", Line: "Hello there"}
	_, err = phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.WordEntry, error) { return c.LookUp(ctx, req) })
	phoneWantCode(t, err, http.StatusConflict, psync.CodeNoAI)

	srv, seen := fakeLLM(t, `{"word":"hello","reading":"/həˈləʊ/","meaning":"int. 你好"}`)
	onEngine(a, func() {
		a.pipeline = media.Tools{Translator: media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m1"}}
		a.prefs.language = "简体中文"
	})
	e, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.WordEntry, error) { return c.LookUp(ctx, req) })
	if err != nil || e.Word != "hello" || e.Reading != "/həˈləʊ/" || e.Meaning != "int. 你好" || e.Language != "简体中文" {
		t.Fatalf("lookup %+v, %v", e, err)
	}
	sent := strings.Join(*seen, "\n")
	for _, want := range []string{"影片：Rust Talk", "字幕：Hello there", "要查的词：Hello", "用简体中文解释这个词"} {
		if !strings.Contains(sent, want) {
			t.Fatalf("sent %q, want %q in it", sent, want)
		}
	}
	asked := len(*seen)
	if e, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.WordEntry, error) { return c.LookUp(ctx, req) }); err != nil || e.Meaning != "int. 你好" || len(*seen) != asked {
		t.Fatalf("looked up again: %+v, %v, the AI service asked %d times", e, err, len(*seen)-asked)
	}

	srv, seen = fakeLLM(t, `{"meaning":"打招呼。","breakdown":"hello · 你好","translation":"","tone":"轻松","related":""}`)
	onEngine(a, func() {
		a.pipeline = media.Tools{Translator: media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m1"}}
	})
	x, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.Explanation, error) {
		return c.Explain(ctx, psync.LineExplain{Library: phoneLibraryID(dir), Film: "yt", Index: 1, Lines: []psync.Cue{
			{StartMS: 0, Original: "Hi.", Translation: "嗨。"}, {StartMS: 1000, Original: "Hello there", Translation: "你好"}}})
	})
	if err != nil || x.Model != "m1" || x.Language != "简体中文" || x.Text != "打招呼。\n\nhello · 你好\n\n轻松" {
		t.Fatalf("explain %+v, %v", x, err)
	}
	sent = strings.Join(*seen, "\n")
	for _, want := range []string{"[0:00] 嗨。 / Hi.", "要讲解的一句：[0:01] Hello there", "用简体中文讲解这一句的原文"} {
		if !strings.Contains(sent, want) {
			t.Fatalf("sent %q, want %q in it", sent, want)
		}
	}
}

// A paired phone's explanation comes with its sections, which it heads as the side panel does; a follow-up question goes to the AI service with the line, the lines before it, what was explained and the questions asked before, never a line after it; and the part of a film watched sums up from the lines the phone sends, each item at one of them.
func TestPhoneSyncExplainsInSectionsAnswersFollowUpsAndSumsUp(t *testing.T) {
	a, tt, updates, dir, _ := phoneSyncApp(t)
	phoneSyncOn(t, a, tt, updates)
	c := phoneClient(t, a, "")
	if _, err := phonePair(t, a, tt, updates, c, a.phone.offer.Code); err != nil {
		t.Fatal(err)
	}
	h, err := phoneCall(t, a, tt, updates, c.Hello)
	if err != nil || !slices.Contains(h.Ops, psync.OpExplainAsk) || !slices.Contains(h.Ops, psync.OpRecap) {
		t.Fatalf("hello %+v, %v", h, err)
	}
	lib := phoneLibraryID(dir)
	line := psync.LineExplain{Library: lib, Film: "yt", Index: 1, Lines: []psync.Cue{
		{StartMS: 0, Original: "Hi.", Translation: "嗨。"}, {StartMS: 1000, Original: "Hello there", Translation: "你好"}, {StartMS: 2000, Original: "The ending.", Translation: "结局。"}}}

	srv, _ := fakeLLM(t, `{"meaning":"打招呼。","breakdown":"hello · 你好","translation":"","tone":"轻松","related":""}`)
	onEngine(a, func() {
		a.pipeline = media.Tools{Translator: media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m1"}}
		a.prefs.language = "简体中文"
	})
	x, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.Explanation, error) { return c.Explain(ctx, line) })
	want := []psync.ExplainSection{{Kind: psync.ExplainMeaning, Text: "打招呼。"}, {Kind: psync.ExplainBreakdown, Text: "hello · 你好"}, {Kind: psync.ExplainTone, Text: "轻松"}}
	// The fake AI service listens on this computer, so the model is told the phone as running here.
	if err != nil || !slices.Equal(x.Sections, want) || !x.Local {
		t.Fatalf("explain %+v, %v", x, err)
	}

	srv, seen := fakeLLM(t, "因为是早上。")
	onEngine(a, func() {
		a.pipeline = media.Tools{Translator: media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m2"}}
	})
	ans, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.ExplainAnswer, error) {
		return c.Ask(ctx, psync.ExplainAsk{LineExplain: line, Explained: x.Sections, Asked: []psync.ExplainQA{{Question: "谁说的？", Answer: "主角。"}}, Question: "为什么这么说？"})
	})
	if err != nil || ans.Text != "因为是早上。" || ans.Model != "m2" || ans.Language != "简体中文" {
		t.Fatalf("ask %+v, %v", ans, err)
	}
	sent := strings.Join(*seen, "\n")
	for _, want := range []string{"[0:00] 嗨。 / Hi.", "要讲解的一句：[0:01] Hello there", "之前的讲解：\n打招呼。\nhello · 你好", "问：谁说的？\n答：主角。", "追问：为什么这么说？"} {
		if !strings.Contains(sent, want) {
			t.Fatalf("sent %q, want %q in it", sent, want)
		}
	}
	if strings.Contains(sent, "The ending.") {
		t.Fatalf("a line after the one asked about was sent: %q", sent)
	}

	srv, seen = fakeLLM(t, `{"items":[{"at":"0:00","text":"有人打招呼"},{"at":"0:09","text":"不存在的一行"},{"at":"0:01","text":"又一句问候"}]}`)
	onEngine(a, func() {
		a.pipeline = media.Tools{Translator: media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m1"}}
	})
	r, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.Recap, error) {
		return c.Recap(ctx, psync.FilmRecap{Library: lib, Film: "yt", Lines: line.Lines[:2]})
	})
	wantItems := []psync.RecapItem{{StartMS: 0, Text: "有人打招呼"}, {StartMS: 1000, Text: "又一句问候"}}
	if err != nil || !slices.Equal(r.Items, wantItems) || r.Model != "m1" || r.Language != "简体中文" {
		t.Fatalf("recap %+v, %v", r, err)
	}
	sent = strings.Join(*seen, "\n")
	if !strings.Contains(sent, "影片：Rust Talk") || !strings.Contains(sent, "[0:01] 你好 / Hello there") || strings.Contains(sent, "The ending.") {
		t.Fatalf("the recap sent %q", sent)
	}

	onEngine(a, func() { a.pipeline = media.Tools{} })
	_, err = phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.Recap, error) {
		return c.Recap(ctx, psync.FilmRecap{Library: lib, Film: "yt", Lines: line.Lines[:2]})
	})
	phoneWantCode(t, err, http.StatusConflict, psync.CodeNoAI)
}

// A word looked up on the picture answers the phone's lookup of it from the same cache, for a film whose speech language the user named over the one heard and whose subtitles are in its own language: the phone's query is the picture's (vocabQuery), and the AI service is not asked.
func TestPhoneSyncLookUpSharesThePicturesCache(t *testing.T) {
	a, tt, updates, dir, _ := phoneSyncApp(t)
	onEngine(a, func() {
		for i := range a.library.Jobs {
			if a.library.Jobs[i].ID == "yt" {
				a.library.Jobs[i].SpokenLanguage, a.library.Jobs[i].SpeechLanguage, a.library.Jobs[i].SameLanguage = "en", "ja", true
			}
		}
	})
	phoneSyncOn(t, a, tt, updates)
	c := phoneClient(t, a, "")
	if _, err := phonePair(t, a, tt, updates, c, a.phone.offer.Code); err != nil {
		t.Fatal(err)
	}
	srv, seen := fakeLLM(t, `{"word":"hello","meaning":"from the AI"}`)
	var q lookup.Query
	onEngine(a, func() {
		a.pipeline = media.Tools{Translator: media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m1"}}
		a.prefs.language = "English"
		i := slices.IndexFunc(a.library.Jobs, func(j core.Job) bool { return j.ID == "yt" })
		item := films.FromJob(a.library.Jobs[i], "", "")
		q = vocabQuery(item, a.prefs.language, "Hello", "Hello there")
		a.vocabCache().Put(q, lookup.Entry{Word: "hello", Meaning: "int. 你好"}, time.Now())
	})
	if q.Spoken != "ja" || q.Language != "简体中文" {
		t.Fatalf("the picture's query %+v", q)
	}
	req := psync.WordLookup{Library: phoneLibraryID(dir), Film: "yt", Word: "Hello", Line: "Hello there"}
	e, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.WordEntry, error) { return c.LookUp(ctx, req) })
	if err != nil || e.Meaning != "int. 你好" || e.Language != "简体中文" || len(*seen) != 0 {
		t.Fatalf("lookup %+v, %v, the AI service asked %d times", e, err, len(*seen))
	}
}

// A read-only phone looks words up and has lines explained, as these change nothing, but a word it looks up is not kept among the words looked up on this computer: a read-only phone changes nothing here.
func TestPhoneSyncReadOnlyPhoneLooksUpWithoutKeepingTheWord(t *testing.T) {
	a, tt, updates, dir, _ := phoneSyncApp(t)
	phoneSyncOn(t, a, tt, updates)
	c := phoneClient(t, a, "")
	res, err := phonePair(t, a, tt, updates, c, a.phone.offer.Code)
	if err != nil {
		t.Fatal(err)
	}
	srv, seen := fakeLLM(t, `{"word":"hello","meaning":"int. 你好"}`)
	var q lookup.Query
	onEngine(a, func() {
		a.pipeline = media.Tools{Translator: media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m1"}}
		a.prefs.language = "简体中文"
		a.phoneSyncSetReadOnly(res.DeviceID, true)
		i := slices.IndexFunc(a.library.Jobs, func(j core.Job) bool { return j.ID == "yt" })
		q = vocabQuery(films.FromJob(a.library.Jobs[i], "", ""), a.prefs.language, "Hello", "Hello there")
	})
	h, err := phoneCall(t, a, tt, updates, c.Hello)
	if err != nil || !h.ReadOnly {
		t.Fatalf("hello %+v, %v", h, err)
	}
	req := psync.WordLookup{Library: phoneLibraryID(dir), Film: "yt", Word: "Hello", Line: "Hello there"}
	e, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.WordEntry, error) { return c.LookUp(ctx, req) })
	if err != nil || e.Meaning != "int. 你好" || len(*seen) == 0 {
		t.Fatalf("lookup %+v, %v, the AI service asked %d times", e, err, len(*seen))
	}
	var kept bool
	onEngine(a, func() { _, kept = a.vocabCache().Get(q) })
	if kept {
		t.Fatal("a read-only phone's lookup was kept among the words looked up on this computer")
	}
	srv, _ = fakeLLM(t, `{"meaning":"打招呼。"}`)
	onEngine(a, func() {
		a.pipeline = media.Tools{Translator: media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m1"}}
	})
	x, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.Explanation, error) {
		return c.Explain(ctx, psync.LineExplain{Library: phoneLibraryID(dir), Film: "yt", Lines: []psync.Cue{{Original: "Hello there", Translation: "你好"}}})
	})
	if err != nil || x.Text != "打招呼。" {
		t.Fatalf("explain %+v, %v", x, err)
	}
}

// phoneBillingBodies are what an AI service out of credit answers, in each of the interface's languages: a 402 whose body links to where credit is bought, prices it and tells the user to top up.
var phoneBillingBodies = map[string]string{
	"en":      `{"error":{"message":"Insufficient balance. Top up at https://pay.example.com/topup: 1,000 credits for $9.99."}}`,
	"zh-Hans": `{"error":{"message":"余额不足，请前往 https://pay.example.com/topup 充值：¥68 购买 1000 点数。"}}`,
	"zh-Hant": `{"error":{"message":"餘額不足，請至 https://pay.example.com/topup 儲值：NT$300 購買 1000 點數。"}}`,
	"ja":      `{"error":{"message":"残高が不足しています。https://pay.example.com/topup でチャージしてください：¥1,200 で 1000 クレジット。"}}`,
	"ko":      `{"error":{"message":"잔액이 부족합니다. https://pay.example.com/topup 에서 충전하세요: ₩12,000에 1000 크레딧."}}`,
}

// phoneBillingWords are what a phone must never be told of an AI service's billing: a link, a price, and topping up, buying or a balance in any of the interface's languages, the desktop's own words for an account out of credit among them (PrecheckTranslateQuota).
var phoneBillingWords = []string{"http", "example.com", "$", "¥", "₩", "9.99", "1,200", "12,000", "top up", "top it up", "credit", "balance", "insufficient", "充值", "购买", "余额", "儲值", "購買", "餘額", "チャージ", "残高", "クレジット", "충전", "잔액", "크레딧"}

// phoneWantNoBilling fails t when what, JSON sent to a phone, holds any of phoneBillingWords.
func phoneWantNoBilling(t *testing.T, name string, what any) {
	t.Helper()
	data, err := json.Marshal(what)
	if err != nil {
		t.Fatal(err)
	}
	text := strings.ToLower(string(data))
	for _, w := range phoneBillingWords {
		if strings.Contains(text, strings.ToLower(w)) {
			t.Fatalf("%s tells the phone %q: %s", name, w, data)
		}
	}
}

// billingLLM is an AI service that answers every request 402 Payment Required with body.
func billingLLM(t *testing.T, body string) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusPaymentRequired)
		_, _ = w.Write([]byte(body))
	}))
	t.Cleanup(srv.Close)
	return srv
}

// Nothing of an AI service's billing reaches a phone, whatever the interface's language (yldm-tech/Kando's phone boundary): a job whose translation the service refused for an account out of credit, whose body links to where credit is bought and prices it, is told as FailAIUnavailable with no message, neither the body nor the desktop's own words to top the account up; a translation source over its quota alike; and a lookup, an explanation, a follow-up question and a recap the service refused so answer CodeNoAI with a phrase of the desktop's own.
func TestPhoneSyncTellsAPhoneNothingOfTheAIServicesBilling(t *testing.T) {
	for _, lang := range []string{"en", "zh-Hans", "zh-Hant", "ja", "ko"} {
		t.Run(lang, func(t *testing.T) {
			testlocale.Use(t, lang)
			body := phoneBillingBodies[lang]
			a, tt, updates, dir, _ := phoneSyncApp(t)
			phoneSyncOn(t, a, tt, updates)
			c := phoneClient(t, a, "")
			if _, err := phonePair(t, a, tt, updates, c, a.phone.offer.Code); err != nil {
				t.Fatal(err)
			}

			link := "https://www.youtube.com/watch?v=jNQXAC9IVRw"
			failed := []core.Job{
				{ID: "quota", Source: link, Stage: core.StageFailed, Err: media.ReadStageError("translate: translation API returned 402 Payment Required: " + body)},
				{ID: "source", Source: link, Stage: core.StageFailed, Err: &media.StageError{Stage: media.StageTranslate, Source: "DeepL", Err: errors.New("quota: " + body)}},
				{ID: "other", Source: link, Stage: core.StageFailed, Err: media.ReadStageError("translate: translation API returned 400 Bad Request: " + body)},
			}
			for _, job := range failed {
				var got psync.Job
				onEngine(a, func() { got = a.phoneJobOf(job) })
				// The job's own link is the video's, which the phone sent.
				told := got
				told.Link = ""
				phoneWantNoBilling(t, "job "+job.ID, told)
				if got.Failure == nil || got.Failure.Message != "" || got.Failure.Step != psync.StepTranslate {
					t.Fatalf("job %s's failure %+v", job.ID, got.Failure)
				}
				if job.ID != "other" && (got.Failure.Code != psync.FailAIUnavailable || got.Failure.Remedy != psync.RemedyDesktop) {
					t.Fatalf("job %s's failure %+v, want %s", job.ID, got.Failure, psync.FailAIUnavailable)
				}
			}

			srv := billingLLM(t, body)
			onEngine(a, func() {
				a.pipeline = media.Tools{Translator: media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m1"}}
				a.prefs.language = "简体中文"
			})
			lib := phoneLibraryID(dir)
			line := psync.LineExplain{Library: lib, Film: "yt", Index: 1, Lines: []psync.Cue{{StartMS: 0, Original: "Hi."}, {StartMS: 1000, Original: "Hello there"}}}
			calls := map[string]func(ctx context.Context) (any, error){
				"lookup": func(ctx context.Context) (any, error) {
					return c.LookUp(ctx, psync.WordLookup{Library: lib, Film: "yt", Word: "Hello", Line: "Hello there"})
				},
				"explain": func(ctx context.Context) (any, error) { return c.Explain(ctx, line) },
				"ask": func(ctx context.Context) (any, error) {
					return c.Ask(ctx, psync.ExplainAsk{LineExplain: line, Question: "Why?"})
				},
				"recap": func(ctx context.Context) (any, error) {
					return c.Recap(ctx, psync.FilmRecap{Library: lib, Film: "yt", Lines: line.Lines})
				},
			}
			for name, call := range calls {
				_, err := phoneCall(t, a, tt, updates, call)
				phoneWantCode(t, err, http.StatusConflict, psync.CodeNoAI)
				var e *psync.Error
				errors.As(err, &e)
				phoneWantNoBilling(t, name, e.ErrorBody)
			}
		})
	}
}

// A phone's explanation, and a job, the desktop's monthly limit refused are told as the AI service on the computer that cannot answer for now, with no word of the desktop's for the limit, which names its settings; nothing is sent to the AI service.
func TestPhoneSyncTellsTheMonthlyLimitAsAnAIServiceUnavailable(t *testing.T) {
	a, tt, updates, dir, _ := phoneSyncApp(t)
	phoneSyncOn(t, a, tt, updates)
	c := phoneClient(t, a, "")
	if _, err := phonePair(t, a, tt, updates, c, a.phone.offer.Code); err != nil {
		t.Fatal(err)
	}
	srv, seen := fakeLLM(t, `{"meaning":"打招呼。"}`)
	onEngine(a, func() {
		a.pipeline = media.Tools{Translator: media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m1"}}
		a.prefs.language = "简体中文"
		a.aiusageAttach()
		a.aiusage.ledger.Add(aiusage.MonthOf(a.aiusage.now()), media.PassChat, "localhost", aiusage.MinLimit, 0, false)
		a.SetAIUsageLimit(aiusage.MinLimit)
	})
	_, err := phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.Explanation, error) {
		return c.Explain(ctx, psync.LineExplain{Library: phoneLibraryID(dir), Film: "yt", Index: 0, Lines: []psync.Cue{{StartMS: 0, Original: "Hello there", Translation: "你好"}}})
	})
	phoneWantCode(t, err, http.StatusConflict, psync.CodeNoAI)
	var e *psync.Error
	if errors.As(err, &e); strings.Contains(e.Message, "Settings") || len(*seen) != 0 {
		t.Fatalf("explain: %v, the AI service asked %d times", err, len(*seen))
	}
	job := core.Job{ID: "capped", Source: "https://www.youtube.com/watch?v=jNQXAC9IVRw", Stage: core.StageFailed, Redo: core.RedoMissing, Err: media.ReadStageError("translate: " + media.ErrMonthlyCap.Error())}
	var got psync.Job
	onEngine(a, func() { got = a.phoneJobOf(job) })
	if f := got.Failure; f == nil || f.Code != psync.FailAIUnavailable || f.Message != "" || f.Remedy != psync.RemedyDesktop {
		t.Fatalf("the job's failure %+v", got.Failure)
	}
}
