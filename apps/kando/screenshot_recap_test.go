package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	cmdpalette "kando/apps/kando/palette"
	"kando/packages/core"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestScreenshotsRecap renders the 前情提要 card of the prototype's j1, watched to 1:40 and opened where it stopped, as the design's Main shows it, with the recap an AI service on this machine's loopback writes; the same card without an AI service, which shows lines watched; and the side panel explaining These sweeping curves are called meanders., expanded, with a follow-up question answered, as the design's 解释 view.
func TestScreenshotsRecap(t *testing.T) {
	f := newScreenshotFixture(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req struct {
			Messages []struct{ Role, Content string }
		}
		json.NewDecoder(r.Body).Decode(&req)
		user := req.Messages[len(req.Messages)-1].Content
		var reply string
		switch {
		case strings.Contains(user, "追问："):
			reply = "be called 用来引出一个名称，这里是在介绍术语「河曲」；也可以说 be known as。"
		case strings.Contains(user, "要讲解的一句："):
			reply = `{"meaning":"这些大幅度的弯道叫作河曲。","breakdown":"sweeping · 大幅度的、弧线舒展的\ncurves · 弯道\nare called … · 被称为……（介绍术语的常用句式）","translation":"","tone":"纪录片旁白，平稳地引出术语。","related":"be known as / be referred to as：和 be called 一样用来引出名称"}`
		default:
			reply = `{"items":[{"at":"` + cmdpalette.Clock(protoCues[0].Start) + `","text":"河流从高山上的一股细流开始，一路裹挟泥沙。"},{"at":"` + cmdpalette.Clock(protoCues[3].Start) + `","text":"泥沙像砂纸一样打磨河床，河道越切越深，形成 V 形谷。"},{"at":"` + cmdpalette.Clock(protoCues[6].Start) + `","text":"到了下游地势变缓，河流开始左右摆动。"}]}`
		}
		json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": reply}}}})
	}))
	t.Cleanup(srv.Close)
	ai := func(a *app) {
		tools := f.pipeline.(media.Tools)
		tools.Translator = media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "qwen2.5:7b"}
		a.pipeline = tools
	}
	// opened opens j1 held where it stopped, at 1:40, which is as far as it was watched, and waits for the card.
	opened := func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		a.palette.furthest = map[string]time.Duration{f.job(t, "j1").VideoPath: 100 * time.Second}
		screenshotOpen(t, a, tt, updates, "j1", 100*time.Second)
		screenshotLoad(tt, a)
		screenshotWait(t, tt, a, updates, "recap", func() bool { r := a.Recap(); return r.Open && !r.Busy })
	}
	f.shoot(t, "player-recap", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		ai(a)
		opened(t, a, tt, updates)
		if len(a.Recap().Items) != 3 || a.Recap().Excerpts {
			t.Fatalf("recap %+v", a.Recap())
		}
	})
	f.shoot(t, "player-recap-lines", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		opened(t, a, tt, updates)
		if !a.Recap().Excerpts {
			t.Fatalf("recap without AI %+v", a.Recap())
		}
	})
	f.shoot(t, "player-explain", func(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
		ai(a)
		a.gloss.store.Set("", core.Term{Source: "meander", Target: "河曲", Scope: core.ScopeGlobal, Locked: true})
		screenshotOpen(t, a, tt, updates, "j1", protoCues[7].Start)
		screenshotLoad(tt, a)
		onEngine(a, func() { a.ExplainLine(7) })
		screenshotWait(t, tt, a, updates, "explanation", func() bool { return !a.explain.line.busy })
		tt.Press("id:film-explain-more")
		tt.Type("id:film-explain-ask", "为什么用 are called？")
		tt.Submit("id:film-explain-ask")
		screenshotWait(t, tt, a, updates, "answer", func() bool { return len(a.explain.line.asked) == 1 && !a.explain.line.asked[0].busy })
	})
}
