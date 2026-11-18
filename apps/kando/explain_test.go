package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"kando/apps/kando/film"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/settings"
	"kando/packages/core"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// explainReply is what explainLLM answers a line's explanation with.
const explainReply = `{"meaning":"河曲是河流的大弯道。","breakdown":"sweeping · 大幅度的\ncurves · 弯道","translation":"「被称为」点出这是术语。","tone":"纪录片旁白，平稳。","related":"be known as"}`

// explainLLM is an AI service at a local endpoint that explains every line with explainReply and answers every follow-up question (a message holding 追问：) with a sentence; asks are the user messages it was sent, in order.
func explainLLM(t *testing.T) (media.Tools, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var asks []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct{ Role, Content string }
		}
		if r.Header.Get("Authorization") != "Bearer k" || json.NewDecoder(r.Body).Decode(&req) != nil {
			http.Error(w, "unexpected", http.StatusUnauthorized)
			return
		}
		user := req.Messages[len(req.Messages)-1].Content
		mu.Lock()
		asks = append(asks, user)
		mu.Unlock()
		reply := explainReply
		if strings.Contains(user, "追问：") {
			reply = "called 用来引出名称。"
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": reply}}}})
	}))
	t.Cleanup(srv.Close)
	return media.Tools{Translator: media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m1"}}, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), asks...)
	}
}

// explainOpen opens rivers of paletteLibrary held at at, its lines read.
func explainOpen(t *testing.T, tools media.Tools, at time.Duration) (*app, *nsgotest.App, chan func()) {
	t.Helper()
	a, tt, updates := paletteLibrary(t, tools)
	item, _ := a.paletteFilm("rivers")
	a.OpenAt(item, at)
	waitUntil(t, tt, a, updates, func() bool { return a.player.subs.ready })
	refresh(tt, a)
	return a, tt, updates
}

// TestExplainLineShowsInThePanel checks that I, in a film with lines, has the side panel, shown though it was hidden, explain the line playing in place of its tabs' content: it sends that line, past the furthest point played as well, with the lines just before it and none after it, and shows the line, the explanation's sections under their headings, the glossary's terms the line holds and which model wrote it, here one on this computer; 展开 adds the tone and related expressions, and ‹ 字幕 goes back to the list.
func TestExplainLineShowsInThePanel(t *testing.T) {
	tools, asks := explainLLM(t)
	a, tt, updates := explainOpen(t, tools, 71*time.Second)
	msgs := testlocale.Messages()
	apply(tt, a, func() {
		a.player.panel = false
		a.gloss.store.Set("", core.Term{Source: "meander", Target: "河曲", Scope: core.ScopeGlobal})
	})
	// The panel slides out before it goes.
	tt.Advance(time.Second)
	tt.Key("i")
	if !a.player.panel || a.player.filmSide != film.SideExplain || !a.explain.line.busy {
		t.Fatalf("I: panel %v side %q line %+v", a.player.panel, a.player.filmSide, a.explain.line)
	}
	if !hasText(tt, msgs.ExplainBusy("m1")) || !tt.Exists("id:film-explain >> "+whole("These sweeping curves are called meanders.")) {
		t.Fatalf("I: texts %q", tt.Texts())
	}
	waitUntil(t, tt, a, updates, func() bool { return !a.explain.line.busy })
	for _, want := range []string{msgs.ExplainMeaning(), "河曲是河流的大弯道。", msgs.ExplainBreakdown(), "sweeping · 大幅度的\ncurves · 弯道", msgs.ExplainTranslation(), msgs.ExplainTerms(), "meander → 河曲", msgs.ExplainNoteLocal("m1")} {
		if !hasText(tt, want) {
			t.Fatalf("missing %q in %q", want, tt.Texts())
		}
	}
	if hasText(tt, "纪录片旁白，平稳。") {
		t.Fatal("the tone shows before 展开")
	}
	sent := strings.Join(asks(), "\n")
	for _, want := range []string{"要讲解的一句：[1:10] These sweeping curves are called meanders.\n译文：这些大幅度的弯道被称为河曲。", "数千年间，河道被切割得越来越深。", "Every word in the subtitles can be looked up."} {
		if !strings.Contains(sent, want) {
			t.Fatalf("sent %q, want %q in it", sent, want)
		}
	}
	if strings.Contains(sent, "Every river begins") {
		t.Fatalf("sent lines far from the one explained: %q", sent)
	}
	tt.Press("id:film-explain-more")
	if !hasText(tt, "纪录片旁白，平稳。") || !hasText(tt, "be known as") || tt.Exists("id:film-explain-more") {
		t.Fatal("展开 did not show the tone and related expressions")
	}
	tt.Press("id:film-explain-back")
	if a.player.filmSide != "" || a.explain.line.open || !tt.Exists("id:film-search") {
		t.Fatalf("‹ 字幕 left side %q, line %+v", a.player.filmSide, a.explain.line)
	}
	// The same line again shows what was said of it, without asking again.
	apply(tt, a, func() { a.ExplainLine(4) })
	if a.explain.line.busy || a.explain.line.sections.Meaning != "河曲是河流的大弯道。" || len(asks()) != 1 {
		t.Fatalf("explaining the line again asked %d times, line %+v", len(asks()), a.explain.line)
	}
}

// TestExplainTakesFollowUpQuestions checks that a question typed under the explanation goes, at Return, with the line, what was explained and the question, shows at once and is answered under it, and the field empties.
func TestExplainTakesFollowUpQuestions(t *testing.T) {
	tools, asks := explainLLM(t)
	a, tt, updates := explainOpen(t, tools, 71*time.Second)
	msgs := testlocale.Messages()
	apply(tt, a, func() { a.ExplainLine(4) })
	waitUntil(t, tt, a, updates, func() bool { return !a.explain.line.busy })
	tt.Type("id:film-explain-ask", "为什么用 called？")
	tt.Submit("id:film-explain-ask")
	if a.explain.line.question != "" || len(a.explain.line.asked) != 1 || !hasText(tt, "为什么用 called？") || !hasText(tt, msgs.ExplainAnswering()) {
		t.Fatalf("the question asked: %+v, texts %q", a.explain.line.asked, tt.Texts())
	}
	waitUntil(t, tt, a, updates, func() bool { return !a.explain.line.asked[0].busy })
	if !hasText(tt, "called 用来引出名称。") {
		t.Fatalf("no answer in %q", tt.Texts())
	}
	last := asks()[len(asks())-1]
	for _, want := range []string{"These sweeping curves are called meanders.", "之前的讲解：\n河曲是河流的大弯道。", "追问：为什么用 called？"} {
		if !strings.Contains(last, want) {
			t.Fatalf("sent %q, want %q in it", last, want)
		}
	}
	// An empty question asks nothing.
	tt.Submit("id:film-explain-ask")
	if len(a.explain.line.asked) != 1 {
		t.Fatal("an empty question was asked")
	}
}

// TestExplainSavesTheLineToNotes checks that 存这句到笔记 saves the line explained, with what the explanation says it means as its note, and says so; pressed again it only says the line is in the notes already, and a line saved before without a note takes the explanation's.
func TestExplainSavesTheLineToNotes(t *testing.T) {
	tools, _ := explainLLM(t)
	a, tt, updates := explainOpen(t, tools, 71*time.Second)
	msgs := testlocale.Messages()
	apply(tt, a, func() { a.ExplainLine(4) })
	waitUntil(t, tt, a, updates, func() bool { return !a.explain.line.busy })
	if !hasText(tt, msgs.ExplainSave()) {
		t.Fatalf("no save button in %q", tt.Texts())
	}
	tt.Press("id:film-explain-save")
	lines := a.savedBook().Lines
	if len(lines) != 1 || lines[0].Original != "These sweeping curves are called meanders." || lines[0].Note != "河曲是河流的大弯道。" || !a.LineSaved(4) {
		t.Fatalf("saved %+v", lines)
	}
	if a.player.film.osd != msgs.ExplainOsdNoted() || !hasText(tt, msgs.ExplainSaved()) {
		t.Fatalf("osd %q, texts %q", a.player.film.osd, tt.Texts())
	}
	tt.Press("id:film-explain-save")
	if len(a.savedBook().Lines) != 1 || a.player.film.osd != msgs.ExplainOsdInNotes() {
		t.Fatalf("pressed again: %+v, osd %q", a.savedBook().Lines, a.player.film.osd)
	}
	// A line saved with B, without a note, takes the explanation's.
	apply(tt, a, func() {
		a.ToggleSaveLine(4)
		a.ToggleSaveLine(4)
	})
	if l := a.savedBook().Lines; len(l) != 1 || l[0].Note != "" {
		t.Fatalf("saved with B: %+v", l)
	}
	tt.Press("id:film-explain-save")
	if l := a.savedBook().Lines; len(l) != 1 || l[0].Note != "河曲是河流的大弯道。" || a.player.film.osd != msgs.ExplainOsdNoted() {
		t.Fatalf("saved again: %+v", l)
	}
}

// TestExplainSaysWhyNoExplanationCame checks that without an AI service, or with a cloud one in offline mode, the panel says why and offers the AI service's settings, as a follow-up question does; and that when the service fails it says no explanation came back.
func TestExplainSaysWhyNoExplanationCame(t *testing.T) {
	msgs := testlocale.Messages()
	for _, tc := range []struct {
		name  string
		tools media.Tools
		want  string
	}{
		{"none", media.Tools{Translator: media.ChatTranslator{BaseURL: "https://api.openai.com/v1"}}, msgs.ExplainNoAI()},
		{"offline", media.Tools{Translator: media.ChatTranslator{BaseURL: "https://api.openai.com/v1", APIKey: "k", Model: "m1", Offline: true}}, msgs.PrivacyNoCloudAI()},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a, tt, _ := explainOpen(t, tc.tools, 0)
			apply(tt, a, func() { a.ExplainLine(1) })
			if !hasText(tt, tc.want) || !tt.Exists("id:film-explain-settings") || tt.Exists("id:film-explain-note") {
				t.Fatalf("texts %q", tt.Texts())
			}
			apply(tt, a, func() { a.explain.line.question = "why?"; a.AskExplain() })
			if q := a.explain.line.asked; len(q) != 1 || !q[0].failed || q[0].say(a.cx) != tc.want {
				t.Fatalf("question without AI: %+v", q)
			}
			tt.Press("id:film-explain-settings")
			wantSettings(t, a, settings.SectionAI)
		})
	}
	srv, _ := fakeLLM(t, "{}")
	a, tt, updates := explainOpen(t, media.Tools{Translator: media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "wrong", Model: "m1"}}, 0)
	apply(tt, a, func() { a.ExplainLine(1) })
	waitUntil(t, tt, a, updates, func() bool { return !a.explain.line.busy })
	if l := a.explain.line; !l.failed || !l.needsAI || !strings.HasPrefix(l.say(a.cx), msgs.ExplainFailed("")) {
		t.Fatalf("failed explanation: %+v", l)
	}
}

// TestExplainAnswersInTheFilmsLanguage checks that a line of a film translated into another language than the one the settings name now is explained in the film's, and that the footnote says which model wrote it and that a cloud one is not on this computer.
func TestExplainAnswersInTheFilmsLanguage(t *testing.T) {
	srv, seen := fakeLLM(t, `{"meaning":"說明。"}`)
	tools := media.Tools{Translator: media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m1"}}
	a, tt, updates := paletteLibrary(t, tools)
	a.prefs.language = "简体中文"
	a.library.Jobs[0].Language = "繁體中文"
	refresh(tt, a)
	paletteOpenFilm(t, a, tt, updates, "rivers")
	apply(tt, a, func() { a.ExplainLine(1) })
	waitUntil(t, tt, a, updates, func() bool { return !a.explain.line.busy })
	if sent := strings.Join(*seen, "\n"); !strings.Contains(sent, "用繁體中文") {
		t.Fatalf("sent %q, want it explained in 繁體中文", sent)
	}
	apply(tt, a, func() { a.explain.line.local = false })
	if note := a.Explanation().Note(a.cx); note != testlocale.Messages().ExplainNote("m1") {
		t.Fatalf("a cloud model's footnote %q", note)
	}
}

// TestExplainGoesWithItsFilm checks that the explanation closes, and the panel goes back to the subtitle list, once another film opens.
func TestExplainGoesWithItsFilm(t *testing.T) {
	tools, _ := explainLLM(t)
	a, tt, updates := explainOpen(t, tools, 0)
	apply(tt, a, func() { a.ExplainLine(1) })
	item, _ := a.paletteFilm("market")
	apply(tt, a, func() { a.Open(item) })
	waitUntil(t, tt, a, updates, func() bool { return a.player.subs.ready })
	if a.explain.line.open || a.player.filmSide != "" || tt.Exists("id:film-explain") {
		t.Fatalf("another film kept the explanation: side %q", a.player.filmSide)
	}
}

// TestExplainGoesWhenItsFilmCloses checks that closing the film closes the explanation and stops what it asks, so opening the same film again shows the subtitle list, not the old explanation.
func TestExplainGoesWhenItsFilmCloses(t *testing.T) {
	tools, _ := explainLLM(t)
	a, tt, updates := explainOpen(t, tools, 0)
	apply(tt, a, func() { a.ExplainLine(1) })
	if !a.explain.line.busy || len(a.explain.cancel) == 0 {
		t.Fatalf("explain: line %+v", a.explain.line)
	}
	gen := a.explain.gen
	apply(tt, a, a.closeFilm)
	if a.explain.line.open || a.player.filmSide != "" || a.explain.cancel != nil || a.explain.gen == gen {
		t.Fatalf("closing the film kept the explanation: side %q line %+v", a.player.filmSide, a.explain.line)
	}
	item, _ := a.paletteFilm("rivers")
	apply(tt, a, func() { a.Open(item) })
	waitUntil(t, tt, a, updates, func() bool { return a.player.subs.ready })
	refresh(tt, a)
	if a.explain.line.open || a.player.filmSide != "" || tt.Exists("id:film-explain") {
		t.Fatalf("the same film opened again with the old explanation: side %q", a.player.filmSide)
	}
}
