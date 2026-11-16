package media

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"net/url"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"sync"
	"testing"
	"time"

	"kando/packages/core"
)

// chatPipeline is a local video's pipeline whose whisper hears heard and whose translation goes through the ChatTranslator at a test server, as Tools does, with the subtitles exported beside the video.
type chatPipeline struct {
	heard      []core.Segment
	translator ChatTranslator
}

func (p *chatPipeline) Check() error { return nil }

func (p *chatPipeline) Download(_ context.Context, source string) (string, error) {
	u, err := url.Parse(source)
	if err != nil {
		return "", err
	}
	path, ok := core.FilePath(u)
	if !ok {
		return "", fmt.Errorf("not a local file: %s", source)
	}
	return path, nil
}

func (p *chatPipeline) Transcribe(context.Context, string) ([]core.Segment, error) {
	return p.heard, nil
}

func (p *chatPipeline) Translate(ctx context.Context, segments []core.Segment, language string) ([]core.Segment, error) {
	return p.translator.Translate(ctx, segments, language)
}

func (p *chatPipeline) Export(ctx context.Context, video string, original, translated []core.Segment) (string, error) {
	return exportSubtitles(ctx, video, original, translated)
}

// batchChat is a chat endpoint that translates each batch into upper case and records the first line of each batch it was sent. stop, when it returns true for a batch's first line, answers that batch as the test asks instead: with an error, or by cancelling the job.
type batchChat struct {
	mu   sync.Mutex
	sent []string
	stop func(first string) bool
	fail func(w http.ResponseWriter, r *http.Request)
}

func (c *batchChat) serve(t *testing.T) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var req chatRequest
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		var lines []string
		if err := json.Unmarshal([]byte(req.Messages[len(req.Messages)-1].Content), &lines); err != nil {
			http.Error(w, err.Error(), http.StatusBadRequest)
			return
		}
		c.mu.Lock()
		c.sent = append(c.sent, lines[0])
		stop := c.stop != nil && c.stop(lines[0])
		c.mu.Unlock()
		if stop {
			c.fail(w, r)
			return
		}
		json.NewEncoder(w).Encode(map[string]any{
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": upper(lines)}}},
		})
	}))
	t.Cleanup(srv.Close)
	return srv
}

// sentSince is the first lines of the batches sent since the first n.
func (c *batchChat) sentSince(n int) []string {
	c.mu.Lock()
	defer c.mu.Unlock()
	return append([]string(nil), c.sent[n:]...)
}

// filmLines are n lines of speech, a second each, "line 0" to "line n-1".
func filmLines(n int) []core.Segment {
	out := make([]core.Segment, n)
	for i := range out {
		out[i] = core.Segment{Start: time.Duration(i) * time.Second, End: time.Duration(i+1)*time.Second - time.Millisecond, Text: fmt.Sprintf("line %d", i)}
	}
	return out
}

// firstLines are the first lines of the batches from batch from (counting from 0) to the last of n lines.
func firstLines(from, n int) []string {
	var out []string
	for start := from * translateBatch; start < n; start += translateBatch {
		out = append(out, fmt.Sprintf("line %d", start))
	}
	return out
}

// A translation that stops partway, at its third batch of five, keeps the two it translated: the job run again sends only the third batch on, and its subtitles hold every line, with nothing left beside the video but them. It stops by failing, as on a key the API rejects, or by being cancelled, as the 取消 of the card or quitting Kando does.
func TestRunResumesATranslationFromTheBatchItStoppedAt(t *testing.T) {
	n := translateBatch*4 + 3
	third := fmt.Sprintf("line %d", 2*translateBatch)
	for name, stopWith := range map[string]func(cancel context.CancelFunc) func(http.ResponseWriter, *http.Request){
		"failed": func(context.CancelFunc) func(http.ResponseWriter, *http.Request) {
			return func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "invalid key", http.StatusForbidden) }
		},
		"cancelled": func(cancel context.CancelFunc) func(http.ResponseWriter, *http.Request) {
			return func(_ http.ResponseWriter, r *http.Request) {
				cancel()
				<-r.Context().Done()
			}
		},
	} {
		t.Run(name, func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())
			defer cancel()
			stopped := false
			chat := &batchChat{stop: func(first string) bool {
				if first == third && !stopped {
					stopped = true
					return true
				}
				return false
			}}
			chat.fail = stopWith(cancel)
			p := &chatPipeline{heard: filmLines(n), translator: ChatTranslator{BaseURL: chat.serve(t).URL, Model: "m"}}
			video := filepath.Join(t.TempDir(), "Talk", "video.mp4")
			if err := os.MkdirAll(filepath.Dir(video), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(video, []byte("v"), 0o644); err != nil {
				t.Fatal(err)
			}
			job, _ := core.NewJob(core.FileURL(video))

			failed := Run(ctx, p, job, "English", func(core.Job) {})
			if failed.Stage != core.StageFailed || !strings.HasPrefix(failed.Err.Error(), "translate: ") || !failed.Transcribed {
				t.Fatalf("first run %+v", failed)
			}
			if got := chat.sentSince(0); !reflect.DeepEqual(got, firstLines(0, 3*translateBatch)) {
				t.Fatalf("first run sent the batches starting %q", got)
			}
			if _, err := os.Stat(translationCheckpointPath(video)); err != nil {
				t.Fatalf("nothing kept of the batches translated: %v", err)
			}

			before := len(chat.sentSince(0))
			failed.Stage, failed.Err = core.StageQueued, nil
			done := Run(context.Background(), p, failed, "English", func(core.Job) {})
			if done.Stage != core.StageComplete || done.Untranslated != 0 {
				t.Fatalf("retry %+v", done)
			}
			if got, want := chat.sentSince(before), firstLines(2, n); !reflect.DeepEqual(got, want) {
				t.Fatalf("the retry sent the batches starting %q, want %q", got, want)
			}
			texts := bilingualTexts(t, done)
			if len(texts) != n {
				t.Fatalf("%d cues for %d lines", len(texts), n)
			}
			for i, text := range texts {
				if want := fmt.Sprintf("LINE %d\nline %d", i, i); text != want {
					t.Fatalf("cue %d is %q, want %q", i, text, want)
				}
			}
			entries, err := os.ReadDir(filepath.Dir(video))
			if err != nil {
				t.Fatal(err)
			}
			var names []string
			for _, entry := range entries {
				names = append(names, entry.Name())
			}
			if want := []string{"video.bilingual.srt", "video.mp4", "video.srt"}; !reflect.DeepEqual(names, want) {
				t.Fatalf("the video's folder holds %q, want %q", names, want)
			}
		})
	}
}

// What a translation kept is for the same lines into the same language only: translating them into another language, or other lines, sends every batch.
func TestACheckpointIsForItsLinesAndLanguage(t *testing.T) {
	n := translateBatch*2 + 1
	second := fmt.Sprintf("line %d", translateBatch)
	stopped := false
	chat := &batchChat{
		stop: func(first string) bool {
			if first == second && !stopped {
				stopped = true
				return true
			}
			return false
		},
		fail: func(w http.ResponseWriter, _ *http.Request) { http.Error(w, "invalid key", http.StatusForbidden) },
	}
	tr := ChatTranslator{BaseURL: chat.serve(t).URL, Model: "m"}
	ctx := withCheckpoint(context.Background(), filepath.Join(t.TempDir(), ".video.translation.json"))
	lines := filmLines(n)
	if _, err := tr.Translate(ctx, lines, "English"); err == nil {
		t.Fatal("the translation did not stop")
	}

	before := len(chat.sentSince(0))
	if _, err := tr.Translate(ctx, lines, "日本語"); err != nil {
		t.Fatal(err)
	}
	if got, want := chat.sentSince(before), firstLines(0, n); !reflect.DeepEqual(got, want) {
		t.Fatalf("another language sent %q, want %q", got, want)
	}

	before = len(chat.sentSince(0))
	other := filmLines(n)
	other[0].Text = "line zero"
	got, err := tr.Translate(ctx, other, "日本語")
	if err != nil {
		t.Fatal(err)
	}
	if sent := chat.sentSince(before); len(sent) != 3 || got[0].Text != "LINE ZERO" {
		t.Fatalf("other lines sent %q, first line %q", sent, got[0].Text)
	}
}
