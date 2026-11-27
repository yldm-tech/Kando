package media

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"kando/packages/core"
)

// budgetAsk is one call of ChatTranslator.Budget.
type budgetAsk struct {
	pass     string
	estimate int
}

// budgetRecorder is a Budget that allows the first allow requests and refuses the rest with ErrMonthlyCap, keeping what it was asked.
type budgetRecorder struct {
	mu    sync.Mutex
	allow int
	asks  []budgetAsk
}

func (b *budgetRecorder) budget(pass string, estimate int) error {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.asks = append(b.asks, budgetAsk{pass, estimate})
	if len(b.asks) > b.allow {
		return ErrMonthlyCap
	}
	return nil
}

func (b *budgetRecorder) all() []budgetAsk {
	b.mu.Lock()
	defer b.mu.Unlock()
	return append([]budgetAsk(nil), b.asks...)
}

// TestBudgetRefusesBeforeSending checks that a request Budget refuses is not sent and not metered, and that Chat returns ErrMonthlyCap, with the pass and the estimate of the prompt it would have sent.
func TestBudgetRefusesBeforeSending(t *testing.T) {
	var sent atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent.Add(1)
		usageReply(w, "hi", 5, 1)
	}))
	t.Cleanup(srv.Close)
	var rec meterRecorder
	var b budgetRecorder
	tr := ChatTranslator{BaseURL: srv.URL, Model: "m", Meter: rec.meter, Budget: b.budget}
	reply, err := tr.ChatWith(context.Background(), ChatRequest{System: "be brief", User: "hello there"})
	if !errors.Is(err, ErrMonthlyCap) || reply.Usage.Total() != 0 {
		t.Fatalf("reply %+v, err %v", reply, err)
	}
	if sent.Load() != 0 || len(rec.all()) != 0 {
		t.Fatalf("%d sent, metered %+v", sent.Load(), rec.all())
	}
	want := EstimateTokens("be brief") + EstimateTokens("hello there")
	if asks := b.all(); len(asks) != 1 || asks[0] != (budgetAsk{PassChat, want}) {
		t.Fatalf("asked %+v, want one of %s, %d", asks, PassChat, want)
	}
}

// TestBudgetIsAskedAgainBeforeARetry checks that a request the server failed is asked of Budget again before it is sent again, and that the retry Budget refuses is neither sent nor metered: one request reached the server, metered once.
func TestBudgetIsAskedAgainBeforeARetry(t *testing.T) {
	old := retryBaseDelay
	retryBaseDelay = time.Millisecond
	t.Cleanup(func() { retryBaseDelay = old })
	var sent atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent.Add(1)
		http.Error(w, "busy", http.StatusServiceUnavailable)
	}))
	t.Cleanup(srv.Close)
	var rec meterRecorder
	b := budgetRecorder{allow: 1}
	tr := ChatTranslator{BaseURL: srv.URL, Model: "m", Meter: rec.meter, Budget: b.budget}
	_, err := tr.Translate(context.Background(), []core.Segment{{Text: "a"}}, "English")
	if !errors.Is(err, ErrMonthlyCap) {
		t.Fatalf("err %v", err)
	}
	if sent.Load() != 1 || len(rec.all()) != 1 || len(b.all()) != 2 {
		t.Fatalf("%d sent, metered %+v, asked %+v", sent.Load(), rec.all(), b.all())
	}
	for _, ask := range b.all() {
		if ask.pass != PassTranslate || ask.estimate <= 0 {
			t.Fatalf("asked %+v", ask)
		}
	}
}

// TestATranslationStopsAtTheMonthlyCap checks that a translation whose batches Budget refuses from the second on stops there with ErrMonthlyCap, rather than leave the rest untranslated and go on, which a failed job's text tells as TranslateCap; and that proofreading stops at it too.
func TestATranslationStopsAtTheMonthlyCap(t *testing.T) {
	var sent atomic.Int64
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		sent.Add(1)
		var req chatRequest
		json.NewDecoder(r.Body).Decode(&req)
		var lines []string
		json.Unmarshal([]byte(req.Messages[1].Content), &lines)
		usageReply(w, upper(lines), 10, 2)
	}))
	t.Cleanup(srv.Close)
	segments := make([]core.Segment, 30)
	for i := range segments {
		segments[i] = core.Segment{Text: fmt.Sprintf("line %d", i)}
	}
	b := budgetRecorder{allow: 1}
	tr := ChatTranslator{BaseURL: srv.URL, Model: "m", BatchSize: 10, Budget: b.budget}
	out, err := tr.Translate(context.Background(), segments, "English")
	var untranslated *UntranslatedError
	if !errors.Is(err, ErrMonthlyCap) || errors.As(err, &untranslated) || out != nil {
		t.Fatalf("%d lines, err %v", len(out), err)
	}
	if sent.Load() != 1 || len(b.all()) != 2 {
		t.Fatalf("%d sent, asked %d times", sent.Load(), len(b.all()))
	}
	if kind := ClassifyTranslate(fmt.Errorf("translate: %w", err).Error()); kind != TranslateCap {
		t.Fatalf("classified %q", kind)
	}
	if skippableBatch(err) || transient(err) || !refusedUnsent(err) {
		t.Fatal("a refused request must stop the step, not be skipped or waited out")
	}
	var none budgetRecorder
	tr.Budget = none.budget
	if _, err := tr.Proofread(context.Background(), segments, "Talk"); !errors.Is(err, ErrMonthlyCap) {
		t.Fatalf("proofread: %v", err)
	}
	if sent.Load() != 1 {
		t.Fatalf("%d sent", sent.Load())
	}
}
