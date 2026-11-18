package media

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"slices"
	"strings"
	"testing"

	"kando/packages/core"
)

// candidateChat answers every request with reply and keeps what it was sent.
func candidateChat(t *testing.T, reply string) (*httptest.Server, *[]chatRequest) {
	t.Helper()
	var seen []chatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		seen = append(seen, req)
		json.NewEncoder(w).Encode(map[string]any{"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": reply}}}})
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

// TestCandidatesSendTheLineWithItsContext checks that Candidates sends the line the viewer says is wrong with the lines around it, its current translation, the target language and the glossary terms the lines hold, and returns the model's other translations of it: without the current one, repeats or blanks, at most CandidatesMax.
func TestCandidatesSendTheLineWithItsContext(t *testing.T) {
	srv, seen := candidateChat(t, "```json\n[\"曲流是河流的大弯道。\", \"河曲是河流的大弯道。\", \"  \", \"曲流是河流的大弯道。\", \"这些弯曲叫曲流。\", \"蜿蜒的河道叫曲流。\", \"第五个\"]\n```")
	tr := ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m"}
	got, err := tr.Candidates(context.Background(), CandidateRequest{
		Language: "简体中文",
		Line:     core.Line{Original: "These sweeping curves are called meanders.", Translation: "河曲是河流的大弯道。"},
		Before:   []core.Line{{Original: "Rivers carve valleys.", Translation: "河流切出山谷。"}},
		After:    []core.Line{{Original: "Meanders move over time.", Translation: "河曲会随时间移动。"}},
		Terms:    []core.Term{{Source: "meanders", Target: "曲流", Locked: true}, {Source: "delta", Target: "三角洲"}},
	})
	if err != nil {
		t.Fatal(err)
	}
	if want := []string{"曲流是河流的大弯道。", "这些弯曲叫曲流。", "蜿蜒的河道叫曲流。"}; !slices.Equal(got, want) {
		t.Fatalf("candidates %q, want %q", got, want)
	}
	if len(*seen) != 1 {
		t.Fatalf("%d requests", len(*seen))
	}
	sent := (*seen)[0].Messages
	system, user := sent[0].Content, sent[1].Content
	for _, want := range []string{"简体中文", "meanders", "曲流"} {
		if !strings.Contains(system, want) {
			t.Errorf("the instruction lacks %q: %s", want, system)
		}
	}
	if strings.Contains(system, "三角洲") {
		t.Errorf("the instruction names a term no line holds: %s", system)
	}
	for _, want := range []string{"Rivers carve valleys.", "河流切出山谷。", "These sweeping curves are called meanders.", "河曲是河流的大弯道。", "Meanders move over time."} {
		if !strings.Contains(user, want) {
			t.Errorf("the message lacks %q: %s", want, user)
		}
	}
}

// TestCandidatesNeedAList checks that a reply that is not a list of translations is an error, and one that only repeats the current translation gives none.
func TestCandidatesNeedAList(t *testing.T) {
	srv, _ := candidateChat(t, "I think it is fine.")
	tr := ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m"}
	if _, err := tr.Candidates(context.Background(), CandidateRequest{Language: "简体中文", Line: core.Line{Original: "Hi.", Translation: "嗨。"}}); err == nil {
		t.Fatal("a reply in prose gave no error")
	}
	srv, _ = candidateChat(t, `["嗨。"]`)
	tr.BaseURL = srv.URL + "/v1"
	got, err := tr.Candidates(context.Background(), CandidateRequest{Language: "简体中文", Line: core.Line{Original: "Hi.", Translation: "嗨。"}})
	if err != nil || len(got) != 0 {
		t.Fatalf("got %q, %v", got, err)
	}
}
