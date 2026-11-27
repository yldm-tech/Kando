package phonesync

import (
	"context"
	"errors"
	"fmt"
	"testing"
	"time"

	"kando/packages/media"
	psync "kando/packages/sync"
)

// The line explained goes with at most before lines before it, trimmed, and never one after it.
func TestExplainLinesLeaveOutWhatComesAfter(t *testing.T) {
	req := psync.LineExplain{Index: 2, Lines: []psync.Cue{
		{StartMS: 0, Original: "One."}, {StartMS: 1000, Original: " Two. ", Translation: " 二。 "}, {StartMS: 2000, Original: "Three."}, {StartMS: 3000, Original: "Four."}}}
	before, line := ExplainLines(req, 1)
	if len(before) != 1 || before[0].Original != "Two." || before[0].Translation != "二。" || before[0].Start != time.Second {
		t.Fatalf("before %+v", before)
	}
	if line.Original != "Three." || line.Start != 2*time.Second {
		t.Fatalf("line %+v", line)
	}
	if before, _ := ExplainLines(psync.LineExplain{Lines: req.Lines}, 3); len(before) != 0 {
		t.Fatalf("the first line went with %+v", before)
	}
}

// StudyErr tells a phone why the AI service failed by a code and a phrase of its own, never with what the service answered: an account refused is no_ai, anything else failed, with the kind of failure; an error the app gave a code keeps it.
func TestStudyErrKeepsWhatTheServiceSaid(t *testing.T) {
	body := `{"error":"Top up at https://pay.example.com: 1,000 credits for $9.99"}`
	for _, tc := range []struct {
		err  error
		code string
		msg  string
	}{
		{errors.New("translation API returned 402 Payment Required: " + body), psync.CodeNoAI, studyRefused},
		{errors.New("translation API returned 401 Unauthorized: " + body), psync.CodeNoAI, studyRefused},
		{errors.New("translation API returned 400 Bad Request: " + body), psync.CodeFailed, studyFailed + " (rejected)"},
		{fmt.Errorf("explain: %w", context.DeadlineExceeded), psync.CodeFailed, studyFailed + " (timeout)"},
		{errors.New(body), psync.CodeFailed, studyFailed},
		{fmt.Errorf("explain: %w", media.ErrMonthlyCap), psync.CodeNoAI, studyCapped},
		{psync.Errorf(psync.CodeNoAI, "%s", StudyNoAI), psync.CodeNoAI, StudyNoAI},
	} {
		var e *psync.CallError
		if got := StudyErr(tc.err); !errors.As(got, &e) || e.Code != tc.code || e.Message != tc.msg {
			t.Errorf("%v: %v, want %s: %s", tc.err, got, tc.code, tc.msg)
		}
	}
	if StudyErr(nil) != nil {
		t.Error("no error is an error")
	}
}
