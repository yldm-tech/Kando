package main

import (
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"kando/apps/kando/aiusage"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/settings"
	"kando/packages/core"
	"kando/packages/media"
)

// aiusageLLM is an AI service at a local endpoint that translates every batch it is sent, a JSON array of lines, into as many lines, and says each request took 100 prompt and 20 completion tokens; requests counts what it was sent.
func aiusageLLM(t *testing.T) (media.Tools, *atomic.Int64) {
	t.Helper()
	var requests atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct{ Role, Content string }
		}
		if r.Header.Get("Authorization") != "Bearer k" || json.NewDecoder(r.Body).Decode(&req) != nil {
			http.Error(w, "unexpected", http.StatusUnauthorized)
			return
		}
		requests.Add(1)
		var lines []string
		json.Unmarshal([]byte(req.Messages[len(req.Messages)-1].Content), &lines)
		out := make([]string, len(lines))
		for i := range lines {
			out[i] = fmt.Sprintf("译文%d", i+1)
		}
		content, _ := json.Marshal(out)
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": string(content)}}},
			"usage":   map[string]int{"prompt_tokens": 100, "completion_tokens": 20},
		})
	}))
	t.Cleanup(srv.Close)
	return media.Tools{Translator: media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m1"}}, &requests
}

// A job that translates a film's missing lines counts every request it sent to the AI service, with the tokens the service said, under translate and the service's host, in the month under way.
func TestATranslationJobCountsItsAIUsage(t *testing.T) {
	tools, requests := aiusageLLM(t)
	lib := filepath.Join(t.TempDir(), "我的影片.kando")
	video := filepath.Join(lib, "Talk", "video.mp4")
	if err := os.MkdirAll(filepath.Dir(video), 0o755); err != nil {
		t.Fatal(err)
	}
	base := video[:len(video)-len(".mp4")]
	for path, body := range map[string]string{
		video:                   "v",
		base + ".srt":           "1\n00:00:01,000 --> 00:00:02,000\nHello.\n\n2\n00:00:02,000 --> 00:00:03,000\nHow are you?\n\n3\n00:00:03,000 --> 00:00:04,000\nBye.\n\n",
		base + ".bilingual.srt": "1\n00:00:01,000 --> 00:00:02,000\n你好。\nHello.\n\n2\n00:00:02,000 --> 00:00:03,000\nHow are you?\n\n3\n00:00:03,000 --> 00:00:04,000\n再见。\nBye.\n\n",
	} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	a, tt, updates := newTestApp(t, tools)
	a.aiusage.now = func() time.Time { return time.Date(2026, 11, 20, 12, 0, 0, 0, time.Local) }
	a.aiusageAttach()
	a.edit(func(s *session) { s.LibraryDir = lib })
	a.library.Jobs = []core.Job{{
		ID: "talk", Source: "https://www.youtube.com/watch?v=jNQXAC9IVRw", Language: "简体中文", Stage: core.StageComplete,
		VideoPath: video, SubtitlePath: base + ".bilingual.srt", LibraryDir: lib, Untranslated: 1, CompletedAt: time.Now(),
	}}
	a.refreshLibrary()
	refresh(tt, a)
	tt.Press("补译未译的句子")
	settle(t, a, tt, updates)
	// The meter's reports come back through update, the last of them with the job's own.
	waitUntil(t, tt, a, updates, func() bool { return a.AIUsage().Total().Requests == requests.Load() })
	if job := stageOf(t, a, "talk"); job.Stage != core.StageComplete || job.Untranslated != 0 {
		t.Fatalf("job %+v", job)
	}
	n := requests.Load()
	month := a.AIUsage()
	want := aiusage.Count{Prompt: 100 * n, Completion: 20 * n, Requests: n}
	if n == 0 || month.Month != "2026-11" || month.ByPass[media.PassTranslate] != want || month.ByHost["127.0.0.1"] != want || len(month.ByPass) != 1 {
		t.Fatalf("%d requests sent, counted %+v", n, month)
	}
}

// Explain Line asks the AI service under chat, which the ledger counts as such.
func TestExplainingALineCountsAsChat(t *testing.T) {
	tools, _ := explainLLM(t)
	a, tt, updates := explainOpen(t, tools, 71*time.Second)
	a.aiusageAttach()
	tt.Key("i")
	waitUntil(t, tt, a, updates, func() bool { return !a.explain.line.busy && a.AIUsage().Total().Requests > 0 })
	month := a.AIUsage()
	if c := month.ByPass[media.PassChat]; c.Requests != 1 || c.Estimated == 0 || c.Estimated != c.Tokens() || len(month.ByPass) != 1 {
		t.Fatalf("counted %+v", month)
	}
}

// Startup gives the pipeline's translator the meter, which a change of the AI service afterwards keeps, as it starts from the pipeline's tools.
func TestRestoreAttachesTheAIUsageMeter(t *testing.T) {
	a := newApp(cloudTools(), func(fn func()) { fn() })
	a.restoreSettings(savedSettings{})
	if a.pipeline.(media.Tools).Translator.Meter == nil {
		t.Fatal("no meter after startup")
	}
	a.prefs.llmBase, a.prefs.llmModel = "http://127.0.0.1:11434/v1", "llama"
	a.applyLLM()
	if a.pipeline.(media.Tools).Translator.Meter == nil {
		t.Fatal("the meter went with a change of the AI service")
	}
}

// What was counted is written beside settings.json as it is counted, and the next launch reads it back.
func TestAIUsageIsKeptAcrossLaunches(t *testing.T) {
	dir := t.TempDir()
	now := func() time.Time { return time.Date(2026, 11, 20, 12, 0, 0, 0, time.Local) }
	a := newApp(cloudTools(), func(fn func()) { fn() })
	a.settingsPath = filepath.Join(dir, "settings.json")
	a.aiusage.now = now
	a.restoreSettings(savedSettings{})
	meter := a.pipeline.(media.Tools).Translator.Meter
	meter(media.PassProofread, "api.everyapi.ai", media.ChatUsage{Prompt: 300, Completion: 40})
	meter(media.PassProofread, "api.everyapi.ai", media.ChatUsage{Prompt: 30, Estimated: true})
	if _, err := os.Stat(filepath.Join(dir, aiusage.FileName)); err != nil {
		t.Fatal(err)
	}
	b := newApp(cloudTools(), func(fn func()) { fn() })
	b.settingsPath = a.settingsPath
	b.aiusage.now = now
	b.restoreSettings(savedSettings{})
	want := aiusage.Count{Prompt: 330, Completion: 40, Requests: 2, Estimated: 30}
	if got := b.AIUsage(); got.Month != "2026-11" || got.ByPass[media.PassProofread] != want || got.ByHost["api.everyapi.ai"] != want {
		t.Fatalf("after a restart %+v", got)
	}
}

// A plugin's kando.llm.chat asks the AI service under plugin, not chat, though it goes through the command palette's translator.
func TestAPluginsChatCountsAsPlugin(t *testing.T) {
	srv, _ := fakeLLM(t, "pong")
	a, tt, updates, _, _ := contribApp(t, media.Tools{Translator: media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m1"}})
	onEngine(a, a.aiusageAttach)
	if got, err := scriptsCall(t, a, tt, updates, "acme.wiki", "chat", 50); err != nil || got.(map[string]any)["text"] != "pong" {
		t.Fatalf("chat: %v, %v", got, err)
	}
	waitUntil(t, tt, a, updates, func() bool { return a.AIUsage().Total().Requests > 0 })
	month := a.AIUsage()
	if month.ByPass[media.PassPlugin].Requests != 1 || len(month.ByPass) != 1 {
		t.Fatalf("counted %+v", month)
	}
}

// A task pack's run asks the AI service under plugin, with the tokens the service said.
func TestAPluginTaskCountsAsPlugin(t *testing.T) {
	tools, asks := pluginTaskLLM(t)
	a, tt, updates, _ := pluginTaskOpen(t, tools, 20*time.Second, pluginTaskSoFar)
	onEngine(a, a.aiusageAttach)
	apply(tt, a, func() { a.pluginsRunTask("example.who") })
	waitUntil(t, tt, a, updates, func() bool { return !a.plugins.task.busy && a.AIUsage().Total().Requests > 0 })
	month := a.AIUsage()
	want := aiusage.Count{Prompt: 100, Completion: 20, Requests: 1}
	if len(asks()) != 1 || month.ByPass[media.PassPlugin] != want || len(month.ByPass) != 1 {
		t.Fatalf("%d asks, counted %+v", len(asks()), month)
	}
}

// A ledger file that cannot be read is copied to ai-usage.json.bad before the next request's save writes over it.
func TestADamagedLedgerIsKeptAside(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, aiusage.FileName)
	if err := os.WriteFile(path, []byte(`{"months": [`), 0o600); err != nil {
		t.Fatal(err)
	}
	a := newApp(cloudTools(), func(fn func()) { fn() })
	a.settingsPath = filepath.Join(dir, "settings.json")
	a.restoreSettings(savedSettings{})
	a.pipeline.(media.Tools).Translator.Meter(media.PassChat, "localhost", media.ChatUsage{Prompt: 10, Completion: 2})
	if bad, err := os.ReadFile(path + ".bad"); err != nil || string(bad) != `{"months": [` {
		t.Fatalf("ai-usage.json.bad: %q, %v", bad, err)
	}
	if l, err := aiusage.Load(path); err != nil || l.Month(aiusage.MonthOf(time.Now())).Total().Requests != 1 {
		t.Fatalf("the ledger after the save: %+v, %v", l, err)
	}
}

// aiusageTalk is a done film of three lines in a library of its own with its second line left untranslated, as TestATranslationJobCountsItsAIUsage's.
func aiusageTalk(t *testing.T) (lib string, job core.Job) {
	t.Helper()
	lib = filepath.Join(t.TempDir(), "我的影片.kando")
	video := filepath.Join(lib, "Talk", "video.mp4")
	if err := os.MkdirAll(filepath.Dir(video), 0o755); err != nil {
		t.Fatal(err)
	}
	base := video[:len(video)-len(".mp4")]
	for path, body := range map[string]string{
		video:                   "v",
		base + ".srt":           "1\n00:00:01,000 --> 00:00:02,000\nHello.\n\n2\n00:00:02,000 --> 00:00:03,000\nHow are you?\n\n3\n00:00:03,000 --> 00:00:04,000\nBye.\n\n",
		base + ".bilingual.srt": "1\n00:00:01,000 --> 00:00:02,000\n你好。\nHello.\n\n2\n00:00:02,000 --> 00:00:03,000\nHow are you?\n\n3\n00:00:03,000 --> 00:00:04,000\n再见。\nBye.\n\n",
	} {
		if err := os.WriteFile(path, []byte(body), 0o644); err != nil {
			t.Fatal(err)
		}
	}
	return lib, core.Job{
		ID: "talk", Source: "https://www.youtube.com/watch?v=jNQXAC9IVRw", Language: "简体中文", Stage: core.StageComplete,
		VideoPath: video, SubtitlePath: base + ".bilingual.srt", LibraryDir: lib, Untranslated: 1, CompletedAt: time.Now(),
	}
}

// With the month's tokens at the monthly limit, translating a film's lines left sends nothing: the film stays as it was and the message bar says the limit is reached, offering 「AI 服务」. Raising the limit translates them, by itself.
func TestTheMonthlyLimitStopsATranslationAndRaisingItResumes(t *testing.T) {
	tools, requests := aiusageLLM(t)
	lib, job := aiusageTalk(t)
	a, tt, updates := newTestApp(t, tools)
	a.aiusage.now = func() time.Time { return time.Date(2026, 11, 20, 12, 0, 0, 0, time.Local) }
	a.aiusageAttach()
	a.aiusage.ledger.Add("2026-11", media.PassTranslate, "api.everyapi.ai", aiusage.MinLimit, 0, false)
	a.SetAIUsageLimit(aiusage.MinLimit)
	a.edit(func(s *session) { s.LibraryDir = lib })
	a.library.Jobs = []core.Job{job}
	a.refreshLibrary()
	refresh(tt, a)
	tt.Press("补译未译的句子")
	settle(t, a, tt, updates)
	waitUntil(t, tt, a, updates, func() bool { _, ended := a.queue.redoEnded["talk"]; return ended && a.queue.active == "" })
	if got := stageOf(t, a, "talk"); requests.Load() != 0 || got.Stage != core.StageComplete || got.Untranslated != 1 {
		t.Fatalf("%d requests sent, job %+v", requests.Load(), got)
	}
	m := testlocale.Messages()
	if n := a.lib.notice; !n.Open || !strings.Contains(n.Text, m.AiusageCapReached()) || n.Section != settings.SectionAI {
		t.Fatalf("notice %+v", n)
	}
	a.SetAIUsageLimit(2 * aiusage.MinLimit)
	settle(t, a, tt, updates)
	waitUntil(t, tt, a, updates, func() bool { return stageOf(t, a, "talk").Untranslated == 0 && a.queue.active == "" })
	if requests.Load() == 0 {
		t.Fatal("nothing sent once the limit was raised")
	}
	if saved := loadSavedSettings(a.settingsPath); a.settingsPath != "" && saved.AIUsageLimit != 2*aiusage.MinLimit {
		t.Fatalf("saved limit %d", saved.AIUsageLimit)
	}
}

// Explain Line at the monthly limit sends nothing and says the limit is reached, rather than what the error says.
func TestExplainingALineAtTheMonthlyLimitSaysSo(t *testing.T) {
	tools, asks := explainLLM(t)
	a, tt, updates := explainOpen(t, tools, 71*time.Second)
	a.aiusageAttach()
	a.aiusage.ledger.Add(aiusage.MonthOf(a.aiusage.now()), media.PassChat, "localhost", aiusage.MinLimit, 0, false)
	a.SetAIUsageLimit(aiusage.MinLimit)
	tt.Key("i")
	waitUntil(t, tt, a, updates, func() bool { return !a.explain.line.busy && a.explain.line.failed })
	if len(asks()) != 0 {
		t.Fatalf("asked %q", asks())
	}
	if !tt.Exists(whole(testlocale.Messages().AiusageCapReached())) {
		t.Fatalf("texts %q", tt.Texts())
	}
}

// The limit is saved with the settings as tokens, 0 and nothing for none, and comes back within its bounds; the translator's Budget reads it with what was counted since, in the month it was counted in.
func TestTheMonthlyLimitIsSavedAndShared(t *testing.T) {
	now := time.Date(2026, 11, 30, 23, 0, 0, 0, time.Local)
	a := newApp(cloudTools(), func(fn func()) { fn() })
	a.aiusage.now = func() time.Time { return now }
	a.restoreSettings(savedSettings{savedAIUsage: savedAIUsage{AIUsageLimit: 50_000}})
	budget := a.pipeline.(media.Tools).Translator.Budget
	if a.AIUsageLimit() != 50_000 || budget == nil || budget(media.PassTranslate, 50_000) != nil {
		t.Fatalf("limit %d", a.AIUsageLimit())
	}
	a.pipeline.(media.Tools).Translator.Meter(media.PassTranslate, "api.everyapi.ai", media.ChatUsage{Prompt: 40_000, Completion: 5_000})
	if err := budget(media.PassChat, 5_001); !errors.Is(err, media.ErrMonthlyCap) {
		t.Fatalf("over the limit: %v", err)
	}
	if err := budget(media.PassChat, 5_000); err != nil {
		t.Fatalf("at the limit: %v", err)
	}
	now = now.Add(2 * time.Hour)
	if err := budget(media.PassChat, 50_000); err != nil {
		t.Fatalf("a new month: %v", err)
	}
	var saved savedSettings
	a.areasSave(&saved)
	if saved.AIUsageLimit != 50_000 {
		t.Fatalf("saved %d", saved.AIUsageLimit)
	}
	a.SetAIUsageLimit(0)
	saved = savedSettings{}
	a.areasSave(&saved)
	if data, _ := json.Marshal(saved.savedAIUsage); saved.AIUsageLimit != 0 || string(data) != "{}" {
		t.Fatalf("off saved as %s", data)
	}
	for in, want := range map[int64]int64{-1: 0, 5: aiusage.MinLimit, aiusage.MaxLimit * 2: aiusage.MaxLimit} {
		s := savedAIUsage{AIUsageLimit: in}
		s.clean()
		if s.AIUsageLimit != want {
			t.Fatalf("clean %d = %d, want %d", in, s.AIUsageLimit, want)
		}
	}
}
