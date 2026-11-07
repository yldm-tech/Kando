package media

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"reflect"
	"strings"
	"testing"
	"time"

	"kando/packages/core"
)

// fakeChat answers each request with reply(lines), recording the requests it saw.
func fakeChat(t *testing.T, reply func(lines []string) string) (*httptest.Server, *[]chatRequest) {
	t.Helper()
	var seen []chatRequest
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer secret" {
			http.Error(w, "unexpected request "+r.URL.Path, http.StatusUnauthorized)
			return
		}
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		seen = append(seen, req)
		var lines []string
		if err := json.Unmarshal([]byte(req.Messages[1].Content), &lines); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": reply(lines)}}},
		})
	}))
	t.Cleanup(srv.Close)
	return srv, &seen
}

func upper(lines []string) string {
	out := make([]string, len(lines))
	for i, l := range lines {
		out[i] = strings.ToUpper(l)
	}
	data, _ := json.Marshal(out)
	return "```json\n" + string(data) + "\n```"
}

func TestChatTranslatorBatchesAndKeepsTiming(t *testing.T) {
	srv, seen := fakeChat(t, upper)
	segments := make([]core.Segment, translateBatch+3)
	for i := range segments {
		segments[i] = core.Segment{Start: time.Duration(i) * time.Second, End: time.Duration(i+1) * time.Second, Text: fmt.Sprintf("line %d", i)}
	}

	tr := ChatTranslator{BaseURL: srv.URL + "/v1/", APIKey: "secret", Model: "test-model"}
	got, err := tr.Translate(context.Background(), segments, "English")
	if err != nil {
		t.Fatal(err)
	}
	if len(*seen) != 2 {
		t.Fatalf("made %d requests, want 2", len(*seen))
	}
	if req := (*seen)[0]; req.Model != "test-model" || !strings.Contains(req.Messages[0].Content, "English") {
		t.Fatalf("unexpected request: %#v", req)
	}
	if len(got) != len(segments) {
		t.Fatalf("got %d segments", len(got))
	}
	for i, s := range got {
		if s.Text != fmt.Sprintf("LINE %d", i) || s.Start != segments[i].Start || s.End != segments[i].End {
			t.Fatalf("segment %d = %#v", i, s)
		}
	}
}

func TestChatTranslatorRejectsWrongLineCount(t *testing.T) {
	srv, _ := fakeChat(t, func([]string) string { return `["only one"]` })
	tr := ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "secret", Model: "m"}
	_, err := tr.Translate(context.Background(), []core.Segment{{Text: "a"}, {Text: "b"}}, "English")
	if err == nil || !strings.Contains(err.Error(), "1 lines for 2") {
		t.Fatalf("err = %v", err)
	}
}

func TestChatTranslatorReportsHTTPErrors(t *testing.T) {
	srv, _ := fakeChat(t, upper)
	tr := ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "wrong", Model: "m"}
	_, err := tr.Translate(context.Background(), []core.Segment{{Text: "a"}}, "English")
	if err == nil || !strings.Contains(err.Error(), "401") {
		t.Fatalf("err = %v", err)
	}
}

// flakyChat fails with the given statuses in order, then translates by echoing.
func flakyChat(t *testing.T, statuses ...int) (*httptest.Server, *int) {
	t.Helper()
	old := retryBaseDelay
	retryBaseDelay = time.Millisecond
	t.Cleanup(func() { retryBaseDelay = old })
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls <= len(statuses) {
			http.Error(w, `{"error":{"code":"channel:capacity_limited"}}`, statuses[calls-1])
			return
		}
		var req chatRequest
		json.NewDecoder(r.Body).Decode(&req)
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": req.Messages[1].Content}}},
		})
	}))
	t.Cleanup(srv.Close)
	return srv, &calls
}

func TestChatTranslatorRetriesCapacityErrors(t *testing.T) {
	srv, calls := flakyChat(t, http.StatusTooManyRequests, http.StatusServiceUnavailable)
	tr := ChatTranslator{BaseURL: srv.URL, Model: "m"}
	got, err := tr.Translate(context.Background(), []core.Segment{{Text: "hello"}}, "English")
	if err != nil {
		t.Fatal(err)
	}
	if *calls != 3 || got[0].Text != "hello" {
		t.Fatalf("calls = %d, got %#v", *calls, got)
	}
}

func TestChatTranslatorGivesUpAfterMaxAttempts(t *testing.T) {
	srv, calls := flakyChat(t, http.StatusTooManyRequests, http.StatusTooManyRequests, http.StatusTooManyRequests, http.StatusTooManyRequests)
	tr := ChatTranslator{BaseURL: srv.URL, Model: "m"}
	_, err := tr.Translate(context.Background(), []core.Segment{{Text: "hello"}}, "English")
	if err == nil || !strings.Contains(err.Error(), "capacity_limited") || *calls != maxAttempts {
		t.Fatalf("calls = %d, err = %v", *calls, err)
	}
}

func TestChatTranslatorDoesNotRetryClientErrors(t *testing.T) {
	srv, calls := flakyChat(t, http.StatusUnauthorized)
	tr := ChatTranslator{BaseURL: srv.URL, Model: "m"}
	if _, err := tr.Translate(context.Background(), []core.Segment{{Text: "hello"}}, "English"); err == nil || *calls != 1 {
		t.Fatalf("calls = %d, err = %v", *calls, err)
	}
}

func TestChatTranslatorStopsRetryingWhenCancelled(t *testing.T) {
	srv, _ := flakyChat(t, http.StatusTooManyRequests, http.StatusTooManyRequests)
	retryBaseDelay = time.Hour
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	tr := ChatTranslator{BaseURL: srv.URL, Model: "m"}
	if _, err := tr.Translate(ctx, []core.Segment{{Text: "hello"}}, "English"); err != context.DeadlineExceeded {
		t.Fatalf("err = %v", err)
	}
}

func TestChatTranslatorRenewsRejectedLogin(t *testing.T) {
	var seen []string
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		seen = append(seen, r.Header.Get("Authorization"))
		if len(seen) == 1 {
			http.Error(w, `{"error":{"code":"invalid_api_key"}}`, http.StatusUnauthorized)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": `["hello"]`}}},
		})
	}))
	t.Cleanup(srv.Close)
	var renews int
	tr := ChatTranslator{
		Model: "m",
		Credential: func(_ context.Context, renew bool) (Endpoint, error) {
			renews++
			key := "old"
			if renew {
				key = "new"
			}
			return Endpoint{BaseURL: srv.URL, APIKey: key}, nil
		},
	}
	got, err := tr.Translate(context.Background(), []core.Segment{{Text: "a"}}, "English")
	if err != nil {
		t.Fatal(err)
	}
	if got[0].Text != "hello" || renews != 2 {
		t.Fatalf("renews = %d, got %#v", renews, got)
	}
	if !reflect.DeepEqual(seen, []string{"Bearer old", "Bearer new"}) {
		t.Fatalf("authorization = %v", seen)
	}
}

func TestTranslatePromptAndContext(t *testing.T) {
	srv, seen := fakeChat(t, upper)
	segments := make([]core.Segment, translateBatch+1)
	for i := range segments {
		segments[i] = core.Segment{Text: fmt.Sprintf("line %d", i)}
	}
	tr := ChatTranslator{
		BaseURL: srv.URL + "/v1", APIKey: "secret", Model: "m", Context: true,
		Prompt: "Translate into {{language}} in a formal tone.",
	}
	if _, err := tr.Translate(context.Background(), segments, "日本語"); err != nil {
		t.Fatal(err)
	}
	first := (*seen)[0].Messages[0].Content
	second := (*seen)[1].Messages[0].Content
	if !strings.Contains(first, "Translate into 日本語 in a formal tone.") || strings.Contains(first, "Earlier lines") {
		t.Fatalf("first prompt = %q", first)
	}
	if !strings.Contains(second, "line 0") || !strings.Contains(second, "Earlier lines") {
		t.Fatalf("second prompt = %q", second)
	}
}

func TestChatTranslatorRequiresLanguage(t *testing.T) {
	if _, err := (ChatTranslator{}).Translate(context.Background(), []core.Segment{{Text: "a"}}, " "); err == nil {
		t.Fatal("expected an error")
	}
}

func TestPing(t *testing.T) {
	if err := (ChatTranslator{BaseURL: "https://example.com/v1"}).Ping(context.Background()); err == nil {
		t.Fatal("expected an empty model to fail")
	}
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/v1/chat/completions" || r.Header.Get("Authorization") != "Bearer secret" {
			http.Error(w, "no", http.StatusUnauthorized)
			return
		}
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		if req.Model != "m" || len(req.Messages) != 1 {
			http.Error(w, "bad request", http.StatusBadRequest)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": "ok"}}},
		})
	}))
	defer srv.Close()
	tr := ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "secret", Model: "m", Client: srv.Client()}
	if err := tr.Ping(context.Background()); err != nil {
		t.Fatal(err)
	}
}

func TestChatTranslatorReportsEachBatchAndRetryWait(t *testing.T) {
	srv, _ := flakyChat(t, http.StatusTooManyRequests)
	segments := make([]core.Segment, translateBatch+3)
	for i := range segments {
		segments[i] = core.Segment{Text: fmt.Sprintf("line %d", i)}
	}
	var steps []core.Progress
	ctx := WithTrace(context.Background(), &Trace{Progress: func(p core.Progress) { steps = append(steps, p) }})
	tr := ChatTranslator{BaseURL: srv.URL, Model: "m"}
	if _, err := tr.Translate(ctx, segments, "English"); err != nil {
		t.Fatal(err)
	}
	// Batch 1 of 2 starts, waits once for the rate limit, carries on, then batch 2 starts and the translation ends.
	if len(steps) != 5 {
		t.Fatalf("steps = %+v", steps)
	}
	first, waiting, resumed, second, done := steps[0], steps[1], steps[2], steps[3], steps[4]
	if first.Batch != 1 || first.Batches != 2 || first.BatchSize != translateBatch || first.Fraction != 0 || first.RetryIn != 0 {
		t.Fatalf("first = %+v", first)
	}
	if waiting.RetryIn <= 0 || waiting.Attempt != 1 || waiting.Attempts != maxAttempts-1 || waiting.Batch != 1 {
		t.Fatalf("waiting = %+v", waiting)
	}
	if resumed.RetryIn != 0 || resumed.Batch != 1 {
		t.Fatalf("resumed = %+v", resumed)
	}
	if second.Batch != 2 || second.Fraction != float64(translateBatch)/float64(len(segments)) {
		t.Fatalf("second = %+v", second)
	}
	if !done.Known || done.Fraction != 1 {
		t.Fatalf("done = %+v", done)
	}
}
