package media

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/packages/core"
)

// Each batch is sent with the glossary's terms its own lines hold, the locked ones as translations to use as they are, and a batch that holds none is sent without a glossary.
func TestTranslateSendsEachBatchTheTermsItsLinesHold(t *testing.T) {
	srv, seen := fakeChat(t, upper)
	segments := make([]core.Segment, translateBatch+1)
	for i := range segments {
		segments[i] = core.Segment{Start: time.Duration(i) * time.Second, End: time.Duration(i+1) * time.Second, Text: "plain line"}
	}
	segments[3].Text = "The meander cuts in."
	segments[translateBatch].Text = "Sediment piles up."
	glossary := []core.Term{
		{Source: "meander", Target: "河曲", Scope: core.ScopeFilm, Locked: true},
		{Source: "sediment", Target: "泥沙", Scope: core.ScopeGlobal},
		{Source: "delta", Target: "三角洲", Scope: core.ScopeSeries},
	}
	tr := ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "secret", Model: "m"}
	if _, err := tr.Translate(WithGlossary(context.Background(), glossary), segments, "简体中文"); err != nil {
		t.Fatal(err)
	}
	if len(*seen) != 2 {
		t.Fatalf("%d requests, want 2", len(*seen))
	}
	first, second := (*seen)[0].Messages[0].Content, (*seen)[1].Messages[0].Content
	if !strings.Contains(first, `{"term":"meander","translation":"河曲"}`) || !strings.Contains(first, "exactly as given") || strings.Contains(first, "泥沙") || strings.Contains(first, "三角洲") {
		t.Fatalf("first batch's instruction:\n%s", first)
	}
	if !strings.Contains(second, `{"term":"sediment","translation":"泥沙"}`) || strings.Contains(second, "exactly as given") || strings.Contains(second, "河曲") {
		t.Fatalf("second batch's instruction:\n%s", second)
	}
	if !strings.HasSuffix(first, subtitleReplyRule) || !strings.HasSuffix(second, subtitleReplyRule) {
		t.Fatal("the reply rule is no longer last")
	}

	*seen = nil
	if _, err := tr.Translate(context.Background(), segments[:2], "简体中文"); err != nil {
		t.Fatal(err)
	}
	if prompt := (*seen)[0].Messages[0].Content; strings.Contains(prompt, "Glossary") {
		t.Fatalf("a translation without a glossary was sent one:\n%s", prompt)
	}
}

func TestTermsPathIsBesideTheFilmsWhicheverFileNamesIt(t *testing.T) {
	dir := t.TempDir()
	want := filepath.Join(dir, ".video.terms.json")
	for _, name := range []string{"video.mp4", "video.srt", "video.kando.srt", "video.bilingual.srt"} {
		if got := TermsPath(filepath.Join(dir, name)); got != want {
			t.Fatalf("%s: %s, want %s", name, got, want)
		}
	}
}

func TestTermsRoundTripAsTheFilmsOwn(t *testing.T) {
	video := filepath.Join(t.TempDir(), "video.mp4")
	path := TermsPath(video)
	if terms, err := LoadTerms(path); err != nil || terms != nil {
		t.Fatalf("no file: %v %v", terms, err)
	}
	terms := []core.Term{{Source: "meander", Target: "河曲", Scope: core.ScopeGlobal, Locked: true, Origin: core.OriginUser}, {Source: "delta"}}
	if err := SaveTerms(path, terms); err != nil {
		t.Fatal(err)
	}
	got, err := LoadTerms(path)
	want := []core.Term{{Source: "meander", Target: "河曲", Scope: core.ScopeFilm, Locked: true, Origin: core.OriginUser}}
	if err != nil || !reflect.DeepEqual(got, want) {
		t.Fatalf("read back %+v, %v; want %+v", got, err, want)
	}
	if err := os.WriteFile(video, []byte("x"), 0o644); err != nil {
		t.Fatal(err)
	}
	if !slices.Contains(Companions(video), path) {
		t.Fatalf("Companions %v do not hold the terms file", Companions(video))
	}
	if err := SaveTerms(path, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("a film without terms kept its file: %v", err)
	}
}

func TestSaveTermsLeavesANewerFile(t *testing.T) {
	path := TermsPath(filepath.Join(t.TempDir(), "video.mp4"))
	newer := []byte(`{"version": 2, "terms": [{"source": "a", "target": "b", "scope": "film", "note": "kept"}]}`)
	if err := os.WriteFile(path, newer, 0o600); err != nil {
		t.Fatal(err)
	}
	if err := SaveTerms(path, []core.Term{{Source: "c", Target: "d"}}); err != ErrTermsNewer {
		t.Fatalf("SaveTerms over a newer file: %v", err)
	}
	if data, _ := os.ReadFile(path); string(data) != string(newer) {
		t.Fatalf("the newer file was written over: %s", data)
	}
}

// Proofreading is told the glossary's terms as the lines write them, so a misheard term is corrected to the user's spelling; without a glossary it is told none.
func TestProofreadNamesTheGlossarysTerms(t *testing.T) {
	var prompts []string
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		prompts = append(prompts, req.Messages[0].Content)
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": "[]"}}}})
	}))
	defer server.Close()
	tr := ChatTranslator{BaseURL: server.URL, Model: "m"}
	lines := []core.Segment{{Text: "the oxbo lake"}}
	glossary := []core.Term{{Source: "oxbow lake", Target: "牛轭湖"}}
	if _, err := tr.Proofread(WithGlossary(context.Background(), glossary), lines, "Rivers"); err != nil {
		t.Fatal(err)
	}
	if _, err := tr.Proofread(context.Background(), lines, "Rivers"); err != nil {
		t.Fatal(err)
	}
	if len(prompts) != 2 || !strings.HasSuffix(prompts[0], `["oxbow lake"]`) || strings.Contains(prompts[0], "牛轭湖") || strings.Contains(prompts[1], "oxbow lake") {
		t.Fatalf("prompts %q", prompts)
	}
}
