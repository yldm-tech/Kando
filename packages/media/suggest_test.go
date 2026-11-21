package media

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/packages/core"
)

// suggestingRedo is redoPipeline that suggests terms, or fails with err when it is set.
type suggestingRedo struct {
	*redoPipeline
	terms []core.Term
	asked int
	err   error
}

func (p *suggestingRedo) SuggestTerms(_ context.Context, original, translated []core.Segment, title, language string) ([]core.Term, error) {
	p.asked++
	if len(original) != len(translated) || title != "Talk" || language != "日本語" {
		p.t.Errorf("asked about %d lines for %d, %q in %q", len(translated), len(original), title, language)
	}
	return p.terms, p.err
}

// The suggestions read the translated lines with their originals, and keep a term only when the batch's lines hold it, the film's lines hold it at least twice and its glossary does not; the ones the most lines hold come first.
func TestSuggestTermsKeepsWhatTheFilmHolds(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if !strings.Contains(req.Messages[0].Content, `"Rivers"`) || !strings.Contains(req.Messages[0].Content, "简体中文") {
			t.Errorf("the prompt names neither the title nor the language: %s", req.Messages[0].Content)
		}
		var lines []struct{ Original, Translation string }
		_ = json.Unmarshal([]byte(req.Messages[1].Content), &lines)
		if len(lines) != 4 {
			t.Errorf("sent %+v, want the 4 translated lines", lines)
		}
		reply := "```json\n" + `[
			{"term": "meander", "translation": "曲流"},
			{"term": "Nile", "translation": "尼罗河"},
			{"term": "delta", "translation": "三角洲"},
			{"term": "Danube", "translation": "多瑙河"},
			{"term": "Amazon", "translation": "亚马孙河"},
			{"term": "Meander", "translation": "河曲"}
		]` + "\n```"
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": reply}}}})
	}))
	defer server.Close()
	var original, translated []core.Segment
	for i, pair := range [][2]string{
		{"The Nile ends in a delta.", "尼罗河在三角洲入海。"},
		{"A meander forms.", "形成曲流。"},
		{"Hmm.", ""},
		{"The meander grows; the Nile floods.", "曲流变大，尼罗河泛滥。"},
		{"Another meander, another delta.", "又一个曲流，又一个三角洲。"},
	} {
		span := core.Segment{Start: time.Duration(i) * time.Second, End: time.Duration(i+1) * time.Second}
		original = append(original, core.Segment{Start: span.Start, End: span.End, Text: pair[0]})
		translated = append(translated, core.Segment{Start: span.Start, End: span.End, Text: pair[1]})
	}
	ctx := WithGlossary(context.Background(), []core.Term{{Source: "Delta", Target: "三角洲", Scope: core.ScopeGlobal}})
	got, err := ChatTranslator{BaseURL: server.URL, Model: "m"}.SuggestTerms(ctx, original, translated, "Rivers", "简体中文")
	if err != nil {
		t.Fatal(err)
	}
	want := []core.Term{
		{Source: "meander", Target: "曲流", Scope: core.ScopeFilm, Origin: core.OriginAuto},
		{Source: "Nile", Target: "尼罗河", Scope: core.ScopeFilm, Origin: core.OriginAuto},
	}
	if !slices.Equal(got, want) {
		t.Fatalf("suggested %+v, want %+v", got, want)
	}

	// Offline, a service off this computer is not asked.
	if _, err := (ChatTranslator{BaseURL: "https://api.example.com/v1", Model: "m", Offline: true}).SuggestTerms(ctx, original, translated, "Rivers", "简体中文"); !errors.Is(err, ErrOffline) {
		t.Fatalf("offline suggestions answered %v", err)
	}
}

// Translating a film again with suggestions asked for keeps them beside the video, leaving out the ones dismissed; a request that fails keeps what the film had, and a job that does not ask leaves them as they are.
func TestMakeKeepsTheSuggestedTerms(t *testing.T) {
	job := doneFilm(t, "你好。", "", "再见。")
	job.Redo = core.RedoTranslation
	path := SuggestionsPath(job.VideoPath)
	if err := SaveSuggestions(path, Suggestions{Dismissed: []string{"Goodbye"}}); err != nil {
		t.Fatal(err)
	}
	terms := []core.Term{{Source: "Hello", Target: "ハロー", Scope: core.ScopeFilm, Origin: core.OriginAuto}, {Source: "goodbye", Target: "さよなら", Scope: core.ScopeFilm, Origin: core.OriginAuto}}
	p := &suggestingRedo{redoPipeline: &redoPipeline{t: t}, terms: terms}
	done := Make(WithTermSuggestions(context.Background()), p, job, "日本語", "mac", nil, func(core.Job) {})
	if done.Stage != core.StageComplete || p.asked != 1 {
		t.Fatalf("done %+v, asked %d times", done, p.asked)
	}
	got, err := LoadSuggestions(path)
	if err != nil || !slices.Equal(got.Terms, terms[:1]) || !slices.Equal(got.Dismissed, []string{"Goodbye"}) {
		t.Fatalf("kept %+v, %v", got, err)
	}

	job.Redo = core.RedoTranslation
	failing := &suggestingRedo{redoPipeline: &redoPipeline{t: t}, err: errors.New("boom")}
	if done := Make(WithTermSuggestions(context.Background()), failing, job, "日本語", "mac", nil, func(core.Job) {}); done.Stage != core.StageComplete || failing.asked != 1 {
		t.Fatalf("a failed request failed the job: %+v", done)
	}
	if got, _ := LoadSuggestions(path); !slices.Equal(got.Terms, terms[:1]) {
		t.Fatalf("a failed request left %+v", got)
	}
	job.Redo = core.RedoTranslation
	quiet := &suggestingRedo{redoPipeline: &redoPipeline{t: t}}
	if Make(context.Background(), quiet, job, "日本語", "mac", nil, func(core.Job) {}); quiet.asked != 0 {
		t.Fatal("a job that did not ask for suggestions asked for them")
	}
}

func TestSuggestionsFile(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "film.mp4")
	if err := os.WriteFile(video, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	path := SuggestionsPath(video)
	if path != filepath.Join(dir, ".film.suggestions.json") {
		t.Fatalf("path %q", path)
	}
	s := Suggestions{Terms: []core.Term{{Source: "Nile", Target: "尼罗河"}, {Source: "delta", Target: "三角洲"}}}
	if err := SaveSuggestions(path, s); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(Companions(video), path) {
		t.Fatal("the suggestions do not go with the video")
	}
	s = s.Dismiss("NILE")
	if err := SaveSuggestions(path, s); err != nil {
		t.Fatal(err)
	}
	got, err := LoadSuggestions(path)
	if err != nil || len(got.Terms) != 1 || got.Terms[0].Source != "delta" || got.Terms[0].Origin != core.OriginAuto || !slices.Equal(got.Dismissed, []string{"NILE"}) {
		t.Fatalf("read %+v, %v", got, err)
	}
	if err := SaveSuggestions(path, Suggestions{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("an empty file was kept: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"version": 2, "terms": []}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SaveSuggestions(path, s); !errors.Is(err, ErrSuggestionsNewer) {
		t.Fatalf("a newer file was written over: %v", err)
	}
}

// Dismissing a suggestion reads the file as it is: suggestions a job kept since the window read them stay, and a source dismissed before stays dismissed when the job keeps new ones.
func TestDismissSuggestionKeepsWhatAJobWroteMeanwhile(t *testing.T) {
	video := filepath.Join(t.TempDir(), "film.mp4")
	if err := KeepSuggestions(video, []core.Term{{Source: "Nile", Target: "尼罗河"}}); err != nil {
		t.Fatal(err)
	}
	// A job translating the film again keeps new suggestions after the window read the first ones.
	if err := KeepSuggestions(video, []core.Term{{Source: "Nile", Target: "尼罗河"}, {Source: "delta", Target: "三角洲"}}); err != nil {
		t.Fatal(err)
	}
	got, err := DismissSuggestion(video, "Nile")
	if err != nil || len(got.Terms) != 1 || got.Terms[0].Source != "delta" || !slices.Equal(got.Dismissed, []string{"Nile"}) {
		t.Fatalf("dismissed, the suggestions are %+v, %v", got, err)
	}
	if err := KeepSuggestions(video, []core.Term{{Source: "Nile", Target: "尼罗河"}, {Source: "basin", Target: "流域"}}); err != nil {
		t.Fatal(err)
	}
	kept, err := LoadSuggestions(SuggestionsPath(video))
	if err != nil || len(kept.Terms) != 1 || kept.Terms[0].Source != "basin" || !slices.Equal(kept.Dismissed, []string{"Nile"}) {
		t.Fatalf("a job kept %+v, %v; want Nile left dismissed", kept, err)
	}
}
