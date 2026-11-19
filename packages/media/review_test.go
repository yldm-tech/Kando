package media

import (
	"context"
	"encoding/json"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"kando/packages/core"
)

// reviewingRedo is redoPipeline with a final proofread that suggests a consistency change to the first line and a fluency change to the last, or fails with err when it is set.
type reviewingRedo struct {
	*redoPipeline
	reviews int
	err     error
}

func (p *reviewingRedo) Review(_ context.Context, original, translated []core.Segment, title, language string) ([]core.Proof, error) {
	p.reviews++
	if p.err != nil {
		return nil, p.err
	}
	if title != "video" || language != "日本語" {
		p.t.Errorf("reviewed %q in %q", title, language)
	}
	first, last := translated[0], translated[len(translated)-1]
	return []core.Proof{
		{Span: core.Span{Start: first.Start, End: first.End}, Kind: core.ProofConsistency, Old: first.Text, New: "ハロー。", Why: "表記を統一"},
		{Span: core.Span{Start: last.Start, End: last.End}, Kind: core.ProofFluency, Old: last.Text, New: "またね。", Why: "自然に"},
	}, nil
}

// Translating a film again with the final proofread asked for writes the consistency change into its subtitles at once and keeps the fluency change waiting, both kept beside the video; translating again without it drops what the earlier pass found.
func TestRunKeepsTheFinalProofreadsChanges(t *testing.T) {
	job := doneFilm(t, "你好。", "", "再见。")
	job.Redo = core.RedoTranslation
	p := &reviewingRedo{redoPipeline: &redoPipeline{t: t}}
	ctx := WithReview(context.Background(), core.ProofAutoConsistency)
	done := Run(ctx, p, job, "日本語", func(core.Job) {})
	if done.Stage != core.StageComplete || p.reviews != 1 {
		t.Fatalf("done %+v, reviewed %d times", done, p.reviews)
	}
	want := []string{"ハロー。\nHello there.", "日本語:How are you?\nHow are you?", "日本語:Goodbye.\nGoodbye."}
	if got := bilingualTexts(t, done); !reflect.DeepEqual(got, want) {
		t.Fatalf("bilingual %q, want %q", got, want)
	}
	proofs, err := LoadProofs(ProofsPath(done.VideoPath))
	if err != nil || len(proofs) != 2 {
		t.Fatalf("proofs %+v, %v", proofs, err)
	}
	if proofs[0].Status != core.ProofApplied || proofs[0].Old != "日本語:Hello there." || proofs[1].Status != core.ProofPending {
		t.Fatalf("proofs %+v", proofs)
	}

	// Accepting the waiting change writes it into the subtitles; rolling the pass back puts the applied one back too, and keeps what was accepted.
	subtitles := done.SubtitlePath
	accepted := core.DecideProof(proofs, 1, true)
	if err := DecideProofs(subtitles, proofs, accepted); err != nil {
		t.Fatal(err)
	}
	back := core.RollBackProofs(accepted)
	if err := DecideProofs(subtitles, accepted, back); err != nil {
		t.Fatal(err)
	}
	want = []string{"日本語:Hello there.\nHello there.", "日本語:How are you?\nHow are you?", "またね。\nGoodbye."}
	if got := bilingualTexts(t, done); !reflect.DeepEqual(got, want) {
		t.Fatalf("after review %q, want %q", got, want)
	}
	if kept, _ := LoadProofs(ProofsPath(subtitles)); kept[0].Status != core.ProofRejected || kept[1].Status != core.ProofAccepted {
		t.Fatalf("kept %+v", kept)
	}

	done.Redo, done.Stage = core.RedoTranslation, core.StageQueued
	again := Run(context.Background(), p, done, "日本語", func(core.Job) {})
	if again.Stage != core.StageComplete || p.reviews != 1 {
		t.Fatalf("again %+v, reviewed %d times", again, p.reviews)
	}
	if _, err := os.Stat(ProofsPath(done.VideoPath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("an earlier pass's changes outlived the translation they were made to: %v", err)
	}
}

// A final proofread that fails leaves the translation as it was and keeps no changes; translating the missing lines alone does not proofread the film again.
func TestRunExportsTheTranslationWhenTheFinalProofreadFails(t *testing.T) {
	job := doneFilm(t, "你好。", "", "再见。")
	job.Redo = core.RedoTranslation
	p := &reviewingRedo{redoPipeline: &redoPipeline{t: t}, err: errors.New("translation API returned 503")}
	done := Run(WithReview(context.Background(), core.ProofAutoAll), p, job, "日本語", func(core.Job) {})
	if done.Stage != core.StageComplete || bilingualTexts(t, done)[0] != "日本語:Hello there.\nHello there." {
		t.Fatalf("done %+v, %q", done, bilingualTexts(t, done))
	}
	if _, err := os.Stat(ProofsPath(done.VideoPath)); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("a failed pass kept changes: %v", err)
	}
	p.err = nil
	done.Redo, done.Stage = core.RedoMissing, core.StageQueued
	if again := Run(WithReview(context.Background(), core.ProofAutoAll), p, done, "日本語", func(core.Job) {}); again.Stage != core.StageComplete || p.reviews != 1 {
		t.Fatalf("again %+v, reviewed %d times", again, p.reviews)
	}
}

// The final proofread sends the translated lines alone with their originals, and keeps a change only when it names a line as it was sent and is of a kind it knows; a change that writes a locked term otherwise is dropped.
func TestReviewKeepsOnlyChangesItCanTrust(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		_ = json.NewDecoder(r.Body).Decode(&req)
		if !strings.Contains(req.Messages[0].Content, `"Tokyo Story"`) || !strings.Contains(req.Messages[0].Content, "简体中文") {
			t.Errorf("the prompt names neither the title nor the language: %s", req.Messages[0].Content)
		}
		if !strings.Contains(req.Messages[0].Content, `"translation":"杰克"`) {
			t.Errorf("the prompt does not hold the locked term: %s", req.Messages[0].Content)
		}
		var lines []struct {
			ID          int    `json:"id"`
			Original    string `json:"original"`
			Translation string `json:"translation"`
		}
		_ = json.Unmarshal([]byte(req.Messages[1].Content), &lines)
		if len(lines) != 3 || lines[1].Original != "Jack is here." {
			t.Errorf("sent %+v", lines)
		}
		reply := `[
			{"id": 1, "translation": "你好。", "revised": "您好。", "kind": "fluency", "why": "更礼貌"},
			{"id": 2, "translation": "杰克来了。", "revised": "捷克来了。", "kind": "consistency", "why": "统一人名"},
			{"id": 3, "translation": "不是这句", "revised": "再会。", "kind": "fluency", "why": "x"},
			{"id": 3, "translation": "再见。", "revised": "再会。", "kind": "style", "why": "x"}
		]`
		_ = json.NewEncoder(w).Encode(map[string]any{"choices": []any{map[string]any{"message": map[string]string{"role": "assistant", "content": reply}}}})
	}))
	defer server.Close()
	at := func(i int) (time.Duration, time.Duration) {
		return time.Duration(i) * time.Second, time.Duration(i+1) * time.Second
	}
	var original, translated []core.Segment
	for i, pair := range [][2]string{{"Hello.", "你好。"}, {"Jack is here.", "杰克来了。"}, {"Hmm.", ""}, {"Bye.", "再见。"}} {
		s, e := at(i)
		original = append(original, core.Segment{Start: s, End: e, Text: pair[0]})
		translated = append(translated, core.Segment{Start: s, End: e, Text: pair[1]})
	}
	ctx := WithGlossary(context.Background(), []core.Term{{Source: "Jack", Target: "杰克", Locked: true}})
	got, err := ChatTranslator{BaseURL: server.URL, Model: "m"}.Review(ctx, original, translated, "Tokyo Story", "简体中文")
	if err != nil {
		t.Fatal(err)
	}
	if len(got) != 1 || got[0].New != "您好。" || got[0].Kind != core.ProofFluency || got[0].Span != (core.Span{Start: 0, End: time.Second}) || got[0].Why != "更礼貌" {
		t.Fatalf("kept %+v", got)
	}
}

func TestProofsFileRoundTripAndNewerFile(t *testing.T) {
	dir := t.TempDir()
	video := filepath.Join(dir, "film.mp4")
	path := ProofsPath(filepath.Join(dir, "film.bilingual.srt"))
	if path != ProofsPath(video) || filepath.Base(path) != ".film.proofs.json" {
		t.Fatalf("ProofsPath = %q", path)
	}
	proofs := []core.Proof{{Span: core.Span{Start: time.Second, End: 2 * time.Second}, Kind: core.ProofMeaning, Old: "a", New: "b", Why: "c", Status: core.ProofPending}}
	if err := SaveProofs(path, proofs); err != nil {
		t.Fatal(err)
	}
	if got, err := LoadProofs(path); err != nil || !reflect.DeepEqual(got, proofs) {
		t.Fatalf("read back %+v, %v", got, err)
	}
	if err := os.WriteFile(video, []byte("v"), 0o644); err != nil {
		t.Fatal(err)
	}
	if companions := Companions(video); len(companions) != 1 || companions[0] != path {
		t.Fatalf("companions %q", companions)
	}
	if err := os.WriteFile(path, []byte(`{"version": 9, "changes": []}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SaveProofs(path, nil); !errors.Is(err, ErrProofsNewer) {
		t.Fatalf("wrote over a newer file: %v", err)
	}
	if err := os.WriteFile(path, []byte(`{"version": 1, "changes": []}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := SaveProofs(path, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(path); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("no changes should leave no file: %v", err)
	}
}
