package main

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sync/atomic"
	"testing"
	"time"

	"kando/apps/kando/aiusage"
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
