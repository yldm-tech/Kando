package media

import (
	"context"
	"encoding/json"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"slices"
	"sync"
	"testing"
	"time"

	"kando/packages/core"
)

// meterReport is one call of ChatTranslator.Meter.
type meterReport struct {
	pass, host string
	usage      ChatUsage
}

// meterRecorder collects what Meter is told, from whichever goroutine tells it.
type meterRecorder struct {
	mu      sync.Mutex
	reports []meterReport
}

func (m *meterRecorder) meter(pass, host string, u ChatUsage) {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.reports = append(m.reports, meterReport{pass, host, u})
}

func (m *meterRecorder) all() []meterReport {
	m.mu.Lock()
	defer m.mu.Unlock()
	return slices.Clone(m.reports)
}

// usageReply writes a reply with content and, when prompt or completion is above 0, the usage the service counted.
func usageReply(w http.ResponseWriter, content string, prompt, completion int) {
	reply := map[string]any{"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": content}}}}
	if prompt > 0 || completion > 0 {
		reply["usage"] = map[string]int{"prompt_tokens": prompt, "completion_tokens": completion}
	}
	json.NewEncoder(w).Encode(reply)
}

// TestMeterCountsARetryAndTheReplyThatCame checks that a request the server failed is metered as the estimate of its prompt, and the retry that was answered with the usage the service counted, both under translate and the endpoint's host.
func TestMeterCountsARetryAndTheReplyThatCame(t *testing.T) {
	old := retryBaseDelay
	retryBaseDelay = time.Millisecond
	t.Cleanup(func() { retryBaseDelay = old })
	var calls int
	var body []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		var req chatRequest
		json.NewDecoder(r.Body).Decode(&req)
		body, _ = json.Marshal(req)
		if calls == 1 {
			http.Error(w, "busy", http.StatusServiceUnavailable)
			return
		}
		usageReply(w, req.Messages[1].Content, 90, 12)
	}))
	t.Cleanup(srv.Close)
	var rec meterRecorder
	tr := ChatTranslator{BaseURL: srv.URL, Model: "m", Meter: rec.meter}
	if _, err := tr.Translate(context.Background(), []core.Segment{{Text: "a"}}, "English"); err != nil {
		t.Fatal(err)
	}
	want := []meterReport{
		{PassTranslate, "127.0.0.1", ChatUsage{Prompt: promptEstimate(body), Estimated: true}},
		{PassTranslate, "127.0.0.1", ChatUsage{Prompt: 90, Completion: 12}},
	}
	if got := rec.all(); !slices.Equal(got, want) || want[0].usage.Prompt == 0 {
		t.Fatalf("metered %+v, want %+v", got, want)
	}
}

// TestMeterCountsEachHalfOfASplitBatch checks that a batch cut off and sent again in halves is metered once for every request the server got.
func TestMeterCountsEachHalfOfASplitBatch(t *testing.T) {
	var sends int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sends++
		var req chatRequest
		json.NewDecoder(r.Body).Decode(&req)
		var lines []string
		json.Unmarshal([]byte(req.Messages[1].Content), &lines)
		content, finish := upper(lines), "stop"
		if len(lines) == 8 {
			content, finish = "```json\n"+content[:len(content)-5], "length"
		}
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": content}, "finish_reason": finish}},
			"usage":   map[string]int{"prompt_tokens": 10 * len(lines), "completion_tokens": len(lines)},
		})
	}))
	t.Cleanup(srv.Close)
	segments := make([]core.Segment, 8)
	for i := range segments {
		segments[i] = core.Segment{Text: fmt.Sprintf("line %d", i)}
	}
	var rec meterRecorder
	if _, err := (ChatTranslator{BaseURL: srv.URL, Model: "m", Meter: rec.meter}).Translate(context.Background(), segments, "English"); err != nil {
		t.Fatal(err)
	}
	got := rec.all()
	if sends != 3 || len(got) != sends {
		t.Fatalf("%d requests sent, %d metered: %+v", sends, len(got), got)
	}
	prompt := 0
	for _, r := range got {
		if r.pass != PassTranslate || r.usage.Estimated {
			t.Fatalf("metered %+v", r)
		}
		prompt += r.usage.Prompt
	}
	if prompt != 80+40+40 {
		t.Fatalf("prompt tokens %d", prompt)
	}
}

// TestMeterCountsTheRequestARenewedKeyReplaced checks that a request whose key the API rejected is metered, as the estimate of its prompt, besides the one sent again with the new key.
func TestMeterCountsTheRequestARenewedKeyReplaced(t *testing.T) {
	var calls int
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		calls++
		if calls == 1 {
			http.Error(w, `{"error":{"code":"invalid_api_key"}}`, http.StatusUnauthorized)
			return
		}
		usageReply(w, "hi", 7, 2)
	}))
	t.Cleanup(srv.Close)
	var rec meterRecorder
	tr := ChatTranslator{Model: "m", Meter: rec.meter, Credential: func(context.Context, bool) (Endpoint, error) {
		return Endpoint{BaseURL: srv.URL, APIKey: "k"}, nil
	}}
	if _, err := tr.Chat(context.Background(), "system", "user"); err != nil {
		t.Fatal(err)
	}
	prompt := EstimateTokens("system") + EstimateTokens("user")
	want := []meterReport{
		{PassChat, "127.0.0.1", ChatUsage{Prompt: prompt, Estimated: true}},
		{PassChat, "127.0.0.1", ChatUsage{Prompt: 7, Completion: 2}},
	}
	if got := rec.all(); !slices.Equal(got, want) {
		t.Fatalf("metered %+v, want %+v", got, want)
	}
}

// TestMeterEstimatesAReplyWithoutUsage checks that a reply the service counted nothing of is metered as the estimate of the messages sent and the text received, marked estimated.
func TestMeterEstimatesAReplyWithoutUsage(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		usageReply(w, "这是回答", 0, 0)
	}))
	t.Cleanup(srv.Close)
	var rec meterRecorder
	tr := ChatTranslator{BaseURL: srv.URL, Model: "m", Meter: rec.meter}
	if _, err := tr.Chat(context.Background(), "You explain.", "这句话什么意思？"); err != nil {
		t.Fatal(err)
	}
	want := []meterReport{{PassChat, "127.0.0.1", ChatUsage{
		Prompt:     EstimateTokens("You explain.") + EstimateTokens("这句话什么意思？"),
		Completion: EstimateTokens("这是回答"),
		Estimated:  true,
	}}}
	if got := rec.all(); !slices.Equal(got, want) {
		t.Fatalf("metered %+v, want %+v", got, want)
	}
}

// TestMeterNamesThePassOfEachKindOfRequest checks the pass each method meters its requests under, and that a caller's Pass is kept by ChatWith.
func TestMeterNamesThePassOfEachKindOfRequest(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		usageReply(w, "[]", 5, 1)
	}))
	t.Cleanup(srv.Close)
	original := []core.Segment{{Text: "Rivers carve valleys."}}
	translated := []core.Segment{{Text: "河流切出山谷。"}}
	ctx := context.Background()
	for _, c := range []struct {
		want string
		run  func(tr ChatTranslator)
	}{
		{PassProofread, func(tr ChatTranslator) { tr.Proofread(ctx, original, "Rivers") }},
		{PassReview, func(tr ChatTranslator) { tr.Review(ctx, original, translated, "Rivers", "简体中文") }},
		{PassSuggest, func(tr ChatTranslator) { tr.SuggestTerms(ctx, original, translated, "Rivers", "简体中文") }},
		{PassChat, func(tr ChatTranslator) { tr.Chat(ctx, "s", "u") }},
		{PassChat, func(tr ChatTranslator) {
			tr.Candidates(ctx, CandidateRequest{Language: "简体中文", Line: core.Line{Original: "a", Translation: "甲"}})
		}},
		{PassPlugin, func(tr ChatTranslator) { tr.Pass = PassPlugin; tr.ChatWith(ctx, ChatRequest{System: "s", User: "u"}) }},
	} {
		var rec meterRecorder
		c.run(ChatTranslator{BaseURL: srv.URL, Model: "m", Meter: rec.meter})
		got := rec.all()
		if len(got) != 1 || got[0].pass != c.want {
			t.Errorf("want one request under %q, metered %+v", c.want, got)
		}
	}
}

// TestMeterLeavesOutARequestThatNeverWent checks that a request that could not reach the service is not metered.
func TestMeterLeavesOutARequestThatNeverWent(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(http.ResponseWriter, *http.Request) {}))
	url := srv.URL
	srv.Close()
	var rec meterRecorder
	tr := ChatTranslator{BaseURL: url, Model: "m", Attempts: 1, Meter: rec.meter}
	if _, err := tr.Chat(context.Background(), "s", "u"); err == nil {
		t.Fatal("chat with a closed server succeeded")
	}
	if got := rec.all(); len(got) != 0 {
		t.Fatalf("metered %+v", got)
	}
}

// TestMeterHost checks the host Meter reports: lower case, without the port.
func TestMeterHost(t *testing.T) {
	for in, want := range map[string]string{
		"https://API.EveryAPI.ai/v1":  "api.everyapi.ai",
		"http://127.0.0.1:11434/v1":   "127.0.0.1",
		" https://api.openai.com/v1 ": "api.openai.com",
		"http://[::1]:8080/v1":        "::1",
		"":                            "",
	} {
		if got := MeterHost(in); got != want {
			t.Errorf("MeterHost(%q) = %q, want %q", in, got, want)
		}
	}
}

// hostChat answers every request, whatever host it was addressed to, recording the headers each came with; its Client sends every connection to it, so a translator can address EveryAPI's or another host by name.
func hostChat(t *testing.T) (*http.Client, *[]http.Header) {
	t.Helper()
	var mu sync.Mutex
	var seen []http.Header
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		seen = append(seen, r.Header.Clone())
		mu.Unlock()
		usageReply(w, `["ok"]`, 3, 1)
	}))
	t.Cleanup(srv.Close)
	addr := srv.Listener.Addr().String()
	var dialer net.Dialer
	client := &http.Client{Transport: &http.Transport{DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return dialer.DialContext(ctx, network, addr)
	}}}
	return client, &seen
}

// TestEveryAPIRequestsSayKandoSentThem checks that a request to EveryAPI carries X-Title: Kando once, one to another service none, and a caller's own X-Title replaces Kando's rather than going beside it, while Authorization stays the key's.
func TestEveryAPIRequestsSayKandoSentThem(t *testing.T) {
	ctx := context.Background()
	for _, c := range []struct {
		name, base string
		header     http.Header
		title      []string
	}{
		{"EveryAPI", "http://api.everyapi.ai/v1", nil, []string{"Kando"}},
		{"EveryAPI at its root", "http://EveryAPI.ai:8443/v1", nil, []string{"Kando"}},
		{"another service", "http://api.openai.com/v1", nil, nil},
		{"a look-alike", "http://everyapi.ai.example.com/v1", nil, nil},
		{"a plugin on EveryAPI", "http://api.everyapi.ai/v1", http.Header{"X-Title": {"Kando plugin p"}, "Authorization": {"Bearer stolen"}}, []string{"Kando plugin p"}},
		{"a plugin elsewhere", "http://api.openai.com/v1", http.Header{"x-title": {"Kando plugin p"}}, []string{"Kando plugin p"}},
	} {
		client, seen := hostChat(t)
		tr := ChatTranslator{BaseURL: c.base, APIKey: "secret", Model: "m", Client: client}
		if _, err := tr.ChatWith(ctx, ChatRequest{System: "s", User: "u", Header: c.header}); err != nil {
			t.Fatalf("%s: %v", c.name, err)
		}
		if len(*seen) != 1 {
			t.Fatalf("%s: %d requests", c.name, len(*seen))
		}
		h := (*seen)[0]
		if got := h.Values("X-Title"); !slices.Equal(got, c.title) {
			t.Errorf("%s: X-Title %q, want %q", c.name, got, c.title)
		}
		if got := h.Values("Authorization"); !slices.Equal(got, []string{"Bearer secret"}) {
			t.Errorf("%s: Authorization %q", c.name, got)
		}
	}
}

// TestEveryAPITranslationsSayKandoSentThem checks that a translation's requests, not only ChatWith's, carry X-Title: Kando to EveryAPI.
func TestEveryAPITranslationsSayKandoSentThem(t *testing.T) {
	client, seen := hostChat(t)
	tr := ChatTranslator{BaseURL: "http://api.everyapi.ai/v1", APIKey: "secret", Model: "m", Client: client}
	if _, err := tr.Translate(context.Background(), []core.Segment{{Text: "a"}}, "English"); err != nil {
		t.Fatal(err)
	}
	if len(*seen) == 0 {
		t.Fatal("no request sent")
	}
	for _, h := range *seen {
		if got := h.Values("X-Title"); !slices.Equal(got, []string{"Kando"}) {
			t.Fatalf("X-Title %q", got)
		}
	}
}
