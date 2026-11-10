package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"kando/packages/media"

	"github.com/egoist/mygo/ui"
)

// TestScreenshotsPalette renders the command palette in the prototype's four states over the open film, which has been watched to 1:40 as the prototype's: just opened, a query that finds lines, one that finds commands, and the answer to a question, from an AI service on this machine's loopback that answers as the prototype's; a query over the library (screenshot_helpers_test.go), and one that finds 帮助 › 更新说明.
func TestScreenshotsPalette(t *testing.T) {
	f := newScreenshotFixture(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		reply := `{"answer":"0:30 讲到：「数千年间，河道被切割得越来越深。」 0:50 讲到：「到了下游，地势变缓，河流也慢了下来。」","refs":["0:30","0:50"]}`
		json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": reply}}}})
	}))
	t.Cleanup(srv.Close)
	watched := func(a *app) {
		a.palette.furthest = map[string]time.Duration{f.job(t, "j1").VideoPath: 100 * time.Second}
	}
	open := func(a *app, tt *ui.Tester, updates chan func(), query string) {
		watched(a)
		screenshotOpen(t, a, tt, updates, "j1", protoCues[0].Start)
		tt.Key(ui.Cmd, ui.KeyK)
		tt.Frame()
		screenshotWait(t, tt, updates, "subtitles", func() bool { return !a.palette.reading })
		if query != "" {
			tt.Type(query)
			tt.Frame()
		}
	}
	f.shoot(t, "palette", func(a *app, tt *ui.Tester, updates chan func()) { open(a, tt, updates, "") })
	f.shoot(t, "palette-query", func(a *app, tt *ui.Tester, updates chan func()) { open(a, tt, updates, "河") })
	f.shoot(t, "palette-cmds", func(a *app, tt *ui.Tester, updates chan func()) { open(a, tt, updates, "字幕") })
	f.shoot(t, "palette-answer", func(a *app, tt *ui.Tester, updates chan func()) {
		tools := f.pipeline.(media.Tools)
		tools.Translator = media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "本机模型"}
		a.pipeline = tools
		open(a, tt, updates, "河流为什么会越切越深")
		tt.Key(0, ui.KeyEnter)
		tt.Frame()
		screenshotWait(t, tt, updates, "answer", func() bool { return a.palette.answer != nil && !a.palette.answer.busy })
	})
	f.shoot(t, "palette-library", func(a *app, tt *ui.Tester, updates chan func()) {
		watched(a)
		tt.Key(ui.Cmd, ui.KeyK)
		tt.Frame()
		screenshotWait(t, tt, updates, "subtitles", func() bool { return !a.palette.reading })
		tt.Type("河")
		tt.Frame()
	})
	// 帮助 › 更新说明 is a command of the palette too, beside 检查更新….
	f.shoot(t, "palette-release-notes", func(a *app, tt *ui.Tester, updates chan func()) {
		tt.Key(ui.Cmd, ui.KeyK)
		tt.Frame()
		screenshotWait(t, tt, updates, "subtitles", func() bool { return !a.palette.reading })
		tt.Type("更新")
		tt.Frame()
		if !tt.HasText("更新说明") {
			t.Fatalf("the palette does not offer 更新说明: %q", tt.Texts())
		}
	})
}
