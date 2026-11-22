package media

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"kando/packages/core"
)

// logged collects what the standard logger writes until the test ends.
type logged struct {
	mu  sync.Mutex
	buf bytes.Buffer
}

func (l *logged) Write(p []byte) (int, error) {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Write(p)
}

func (l *logged) String() string {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.String()
}

func (l *logged) Len() int {
	l.mu.Lock()
	defer l.mu.Unlock()
	return l.buf.Len()
}

func TestGapsTextNamesEachRunOfEmptyLines(t *testing.T) {
	lines := make([]core.Segment, 10)
	for i := range lines {
		lines[i] = core.Segment{Start: time.Duration(i) * time.Minute, End: time.Duration(i)*time.Minute + 30*time.Second}
	}
	empty := map[int]bool{1: true, 2: true, 3: true, 6: true}
	got := gapsText(lines, func(i int) bool { return empty[i] })
	if want := "0:01:00–0:03:30 (3), 0:06:00–0:06:30 (1)"; got != want {
		t.Fatalf("gaps = %q, want %q", got, want)
	}
	many := make([]core.Segment, 3*maxLoggedGaps)
	for i := range many {
		many[i] = core.Segment{Start: time.Duration(i) * time.Second, End: time.Duration(i)*time.Second + time.Second}
	}
	// Every other line empty, but the first two: the longest run is named, and no more than maxLoggedGaps runs.
	got = gapsText(many, func(i int) bool { return i < 2 || i%2 == 1 })
	if runs := strings.Count(got, "("); runs != maxLoggedGaps || !strings.HasPrefix(got, "0:00:00–0:00:02 (2)") {
		t.Fatalf("gaps = %q", got)
	}
}

// A batch left untranslated says so in the log, with the job and where in the film its lines are, and not their words.
func TestChatTranslatorLogsTheLinesItLeavesUntranslated(t *testing.T) {
	logs := captureLog(t)
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		json.NewDecoder(r.Body).Decode(&req)
		var lines []string
		json.Unmarshal([]byte(req.Messages[1].Content), &lines)
		content := upper(lines)
		if lines[0] == "secret 0" {
			content = `["broken"]`
		}
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": content}}},
		})
	}))
	t.Cleanup(srv.Close)
	segments := make([]core.Segment, translateBatch+3)
	for i := range segments {
		segments[i] = core.Segment{Start: time.Duration(i) * time.Second, End: time.Duration(i)*time.Second + time.Second, Text: fmt.Sprintf("secret %d", i)}
	}
	tr := ChatTranslator{BaseURL: srv.URL, Model: "m"}
	_, err := tr.Translate(withLogJob(context.Background(), "j1"), segments, "English")
	var partial *UntranslatedError
	if !errors.As(err, &partial) {
		t.Fatalf("err = %v", err)
	}
	text := logs.String()
	want := fmt.Sprintf("job j1: lines 0:00:00–0:00:%02d (%d): translate: %d lines left untranslated, a half of batch 1 of 2 failed", translateBatch, translateBatch, translateBatch/2)
	if !strings.Contains(text, want) || !strings.Contains(text, "sending it again in halves") {
		t.Fatalf("log:\n%s\nwant a line with %q", text, want)
	}
	if strings.Contains(text, "secret") {
		t.Fatalf("the log has the lines' words:\n%s", text)
	}
}

// A film made with lines left untranslated says where they are once it is done, as a reply that left them empty does not fail.
func TestMakeLogsWhereTheFilmIsLeftUntranslated(t *testing.T) {
	logs := captureLog(t)
	job, _ := makeJob(t)
	job.SpeechLanguage = "ja"
	f := &makeFilm{length: 150 * time.Second, language: "ja", translate: func(s string) (string, error) {
		switch s {
		case "line 40s", "line 50s", "line 1m0s":
			return "", nil
		}
		return "译:" + s, nil
	}}
	done, _ := makeFilmOf(t, f, job, 0)
	if done.Stage != core.StageComplete || done.Untranslated != 3 {
		t.Fatalf("job = %s untranslated %d", done.Stage, done.Untranslated)
	}
	text := logs.String()
	if !strings.Contains(text, "job "+job.ID+": made with 3 of 15 lines untranslated: 0:00:40–0:01:02 (3)") || !strings.Contains(text, "draft: the translation left lines empty without an error (0:00:40–") {
		t.Fatalf("log:\n%s", text)
	}
}

// A service given up on says so, and how much of the film it leaves untranslated.
func TestMakeLogsATranslationGivenUp(t *testing.T) {
	logs := captureLog(t)
	job, _ := makeJob(t)
	job.SpeechLanguage = "ja"
	f := &makeFilm{length: 150 * time.Second, language: "ja", translate: func(string) (string, error) { return "", errors.New("502 bad gateway") }}
	if done, _ := makeFilmOf(t, f, job, 0); done.Stage != core.StageComplete {
		t.Fatalf("job = %s", done.Stage)
	}
	text := logs.String()
	for _, want := range []string{"draft of cells 0:00:00–0:00:30 failed: 502 bad gateway", "draft: translation given up for the cells left", "made with 15 of 15 lines untranslated, the translation given up: 502 bad gateway: 0:00:00–"} {
		if !strings.Contains(text, want) {
			t.Fatalf("log:\n%s\nwant %q", text, want)
		}
	}
}

// An error that names the AI service's address, as a request that timed out does, is logged without it.
func TestLogfLeavesOutWebAddresses(t *testing.T) {
	logs := captureLog(t)
	logf(withLogJob(context.Background(), "j1"), "translate: %v", errors.New(`Post "https://api.example.com/v1/chat/completions": context deadline exceeded`))
	if text := logs.String(); strings.Contains(text, "example.com") || !strings.Contains(text, `job j1: translate: Post "<address>": context deadline exceeded`) {
		t.Fatalf("log: %s", text)
	}
}

// A reply whose line is not a string fails without the reply's words in its error, which the log keeps.
func TestReplyLinesSaysWhatALineIsWithoutItsWords(t *testing.T) {
	for content, want := range map[string]string{
		`["a", ["secret", "words"]]`: "line 2 is an array of 2 strings that starts with its line",
		`["a", {"secret": 1}]`:       "line 2 is an object",
		`["a", 3]`:                   "line 2 is a number",
	} {
		var reply chatResponse
		body, _ := json.Marshal(map[string]any{"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": content}}}})
		if err := json.Unmarshal(body, &reply); err != nil {
			t.Fatal(err)
		}
		texts := []string{"a", "secret"}
		_, err := replyLines(reply, texts)
		if err == nil || !strings.HasSuffix(err.Error(), want) || strings.Contains(err.Error(), "secret") {
			t.Errorf("%s: err = %v, want one ending %q", content, err, want)
		}
	}
}
