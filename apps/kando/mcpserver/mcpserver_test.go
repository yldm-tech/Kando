package mcpserver

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"

	"github.com/modelcontextprotocol/go-sdk/mcp"

	"kando/apps/kando/remote"
)

// fakeApp answers the socket as the running Kando would, from canned data, and records each request's params by method.
type fakeApp struct {
	mu     sync.Mutex
	params map[string]json.RawMessage
	calls  map[string]int
}

var (
	sampleStatus = remote.Status{Version: "1.2.3", LibraryDir: "/lib", Ready: true, Readiness: "Ready", Tools: map[string]string{"yt-dlp": "/bin/yt-dlp", "ffmpeg": "", "whisper-cli": "/bin/whisper-cli"}, SpeechModel: "ggml-large.bin", TranslationModel: "gpt", Language: "简体中文", Running: "j1", Queued: 2}
	sampleFilms  = []remote.Film{
		{ID: "f1", Title: "Rivers of Light", Source: "youtube", Link: "https://youtu.be/x", Stage: "complete", Progress: 1, Subtitles: "/lib/a.bilingual.srt"},
		{ID: "f2", Title: "Local Walk", Source: "local", Progress: -1},
	}
	sampleJobs = []remote.Job{
		{ID: "j1", Source: "https://youtu.be/y", Stage: "transcribing", Progress: 0.4, Status: "Transcribing 40%"},
		{ID: "j0", Source: "https://youtu.be/x", Stage: "complete", Progress: 1, FilmID: "f1"},
	}
	sampleCues = []remote.Cue{
		{StartMS: 0, EndMS: 1500, Text: "Hello there", Translation: "你好"},
		{StartMS: 1500, EndMS: 3725, Text: "Untranslated\nline"},
		{StartMS: 3_725_250, EndMS: 3_727_000, Text: "Goodbye", Translation: "再见"},
	}
)

func (f *fakeApp) handle(method string, params json.RawMessage) (any, error) {
	f.mu.Lock()
	f.params[method] = params
	f.calls[method]++
	f.mu.Unlock()
	switch method {
	case remote.MethodStatus:
		return sampleStatus, nil
	case remote.MethodFilms:
		p, err := remote.Params[remote.FilmsParams](params)
		if err != nil {
			return nil, err
		}
		var out []remote.Film
		for _, film := range sampleFilms {
			if (p.Source == "" || film.Source == p.Source) && strings.Contains(strings.ToLower(film.Title), strings.ToLower(p.Query)) {
				out = append(out, film)
			}
		}
		return out, nil
	case remote.MethodFilm, remote.MethodSubtitles:
		p, err := remote.Params[remote.IDParams](params)
		if err != nil {
			return nil, err
		}
		if p.ID != "f1" {
			return nil, remote.Errorf(remote.CodeNotFound, "no film %s", p.ID)
		}
		if method == remote.MethodFilm {
			return sampleFilms[0], nil
		}
		return sampleCues, nil
	case remote.MethodJobs:
		return sampleJobs, nil
	case remote.MethodAdd:
		p, err := remote.Params[remote.AddParams](params)
		if err != nil {
			return nil, err
		}
		result := remote.AddResult{}
		for _, source := range p.Sources {
			if strings.HasPrefix(source, "https://") {
				result.Jobs = append(result.Jobs, remote.Job{ID: "j9", Source: source, Stage: "queued"})
			} else {
				result.Skipped = append(result.Skipped, remote.AddFailed{Source: source, Reason: "not a video"})
			}
		}
		return result, nil
	case remote.MethodCancel:
		p, err := remote.Params[remote.IDParams](params)
		if err != nil {
			return nil, err
		}
		return remote.Job{ID: p.ID, Source: "https://youtu.be/y", Stage: "failed", Canceled: true}, nil
	case remote.MethodRetry, remote.MethodRemove:
		p, err := remote.Params[remote.IDParams](params)
		if err != nil {
			return nil, err
		}
		if p.ID != "j0" {
			return nil, remote.Errorf(remote.CodeNotFound, "no job %s", p.ID)
		}
		if method == remote.MethodRemove {
			return nil, nil
		}
		return remote.Job{ID: p.ID, Source: "https://youtu.be/x", Stage: "queued"}, nil
	case remote.MethodSites:
		return []remote.Site{{Name: "Vimeo", Domain: "vimeo.com"}}, nil
	case remote.MethodShow:
		return nil, nil
	}
	return nil, remote.Errorf(remote.CodeUnknownMethod, "%s", method)
}

func (f *fakeApp) lastParams(method string) json.RawMessage {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.params[method]
}

func (f *fakeApp) count(method string) int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.calls[method]
}

// shortSocket is a socket path short enough for macOS's 104-byte limit, which t.TempDir's can pass, set as KANDO_SOCKET for the test. Launching the app is replaced with an error, so a test never starts the real Kando.
func shortSocket(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "k")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "k.sock")
	t.Setenv("KANDO_SOCKET", path)
	launch := remote.Launch
	remote.Launch = func() error { return errors.New("launch disabled in tests") }
	t.Cleanup(func() { remote.Launch = launch })
	return path
}

func serveFake(t *testing.T, path string, f *fakeApp) *remote.Server {
	t.Helper()
	s, err := remote.Listen(path, f.handle)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func newFake() *fakeApp {
	return &fakeApp{params: map[string]json.RawMessage{}, calls: map[string]int{}}
}

// connect is an MCP client session on a new server through the SDK's in-memory transports.
func connect(t *testing.T) *mcp.ClientSession {
	t.Helper()
	serverT, clientT := mcp.NewInMemoryTransports()
	ss, err := NewServer("1.2.3").Connect(t.Context(), serverT, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ss.Close() })
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(t.Context(), clientT, nil)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { cs.Close() })
	return cs
}

func call(t *testing.T, cs *mcp.ClientSession, name string, args map[string]any) *mcp.CallToolResult {
	t.Helper()
	res, err := cs.CallTool(t.Context(), &mcp.CallToolParams{Name: name, Arguments: args})
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return res
}

// text is the result's text content, its blocks joined by newlines.
func text(t *testing.T, res *mcp.CallToolResult) string {
	t.Helper()
	var parts []string
	for _, c := range res.Content {
		tc, ok := c.(*mcp.TextContent)
		if !ok {
			t.Fatalf("content %T, want text", c)
		}
		parts = append(parts, tc.Text)
	}
	return strings.Join(parts, "\n")
}

// structured decodes the result's structured content into a T.
func structured[T any](t *testing.T, res *mcp.CallToolResult) T {
	t.Helper()
	var v T
	if res.IsError {
		t.Fatalf("tool error: %s", text(t, res))
	}
	data, err := json.Marshal(res.StructuredContent)
	if err != nil {
		t.Fatal(err)
	}
	if err := json.Unmarshal(data, &v); err != nil {
		t.Fatalf("structured content %s: %v", data, err)
	}
	return v
}

func TestListTools(t *testing.T) {
	cs := connect(t)
	res, err := cs.ListTools(t.Context(), nil)
	if err != nil {
		t.Fatal(err)
	}
	type hints struct {
		readOnly, destructive, idempotent, openWorld bool
	}
	want := map[string]hints{
		"kando_status":        {readOnly: true, idempotent: true},
		"kando_list_films":    {readOnly: true, idempotent: true},
		"kando_get_film":      {readOnly: true, idempotent: true},
		"kando_get_subtitles": {readOnly: true, idempotent: true},
		"kando_add_videos":    {openWorld: true},
		"kando_list_jobs":     {readOnly: true, idempotent: true},
		"kando_cancel_job":    {destructive: true, idempotent: true},
		"kando_retry_job":     {destructive: true, openWorld: true},
		"kando_remove_job":    {destructive: true, idempotent: true},
		"kando_search_sites":  {readOnly: true, idempotent: true},
		"kando_open":          {idempotent: true},
	}
	if len(res.Tools) != len(want) {
		t.Errorf("%d tools, want %d", len(res.Tools), len(want))
	}
	for _, tool := range res.Tools {
		h, ok := want[tool.Name]
		if !ok {
			t.Errorf("unexpected tool %s", tool.Name)
			continue
		}
		if tool.Description == "" || tool.InputSchema == nil || tool.OutputSchema == nil {
			t.Errorf("%s: description, input or output schema missing", tool.Name)
		}
		a := tool.Annotations
		if a == nil || a.DestructiveHint == nil || a.OpenWorldHint == nil || a.Title == "" {
			t.Errorf("%s: annotations %+v incomplete", tool.Name, a)
			continue
		}
		got := hints{a.ReadOnlyHint, *a.DestructiveHint, a.IdempotentHint, *a.OpenWorldHint}
		if got != h {
			t.Errorf("%s: hints %+v, want %+v", tool.Name, got, h)
		}
	}

	var subtitles *mcp.Tool
	for _, tool := range res.Tools {
		if tool.Name == "kando_get_subtitles" {
			subtitles = tool
		}
	}
	schema, _ := json.Marshal(subtitles.InputSchema)
	for _, want := range []string{`"enum":["both","original","translation"]`, `"enum":["text","srt"]`, `"required":["id"]`, `"max_cues"`} {
		if !strings.Contains(string(schema), want) {
			t.Errorf("kando_get_subtitles input schema %s lacks %s", schema, want)
		}
	}
}

func TestReadTools(t *testing.T) {
	path := shortSocket(t)
	f := newFake()
	serveFake(t, path, f)
	cs := connect(t)

	status := structured[remote.Status](t, call(t, cs, "kando_status", nil))
	if status.Version != "1.2.3" || !status.Ready || status.Tools["yt-dlp"] != "/bin/yt-dlp" || status.Running != "j1" || status.Queued != 2 {
		t.Errorf("status = %+v", status)
	}

	films := structured[listFilmsOutput](t, call(t, cs, "kando_list_films", map[string]any{"source": "youtube", "query": "RIVERS"}))
	if len(films.Films) != 1 || films.Films[0].ID != "f1" {
		t.Errorf("films = %+v", films)
	}
	if got := string(f.lastParams(remote.MethodFilms)); got != `{"source":"youtube","query":"RIVERS"}` {
		t.Errorf("films params = %s", got)
	}
	none := structured[listFilmsOutput](t, call(t, cs, "kando_list_films", map[string]any{"query": "nothing"}))
	if none.Films == nil || len(none.Films) != 0 {
		t.Errorf("no films = %#v, want an empty list", none.Films)
	}

	film := structured[remote.Film](t, call(t, cs, "kando_get_film", map[string]any{"id": "f1"}))
	if film.Title != "Rivers of Light" || film.Subtitles != "/lib/a.bilingual.srt" {
		t.Errorf("film = %+v", film)
	}

	jobs := structured[listJobsOutput](t, call(t, cs, "kando_list_jobs", nil))
	if len(jobs.Jobs) != 2 || jobs.Jobs[0].Stage != "transcribing" || jobs.Jobs[1].FilmID != "f1" {
		t.Errorf("jobs = %+v", jobs)
	}

	sites := structured[searchSitesOutput](t, call(t, cs, "kando_search_sites", map[string]any{"query": "vimeo"}))
	if len(sites.Sites) != 1 || sites.Sites[0].Domain != "vimeo.com" {
		t.Errorf("sites = %+v", sites)
	}
	if got := string(f.lastParams(remote.MethodSites)); got != `{"query":"vimeo","limit":20}` {
		t.Errorf("sites params = %s, want the default limit", got)
	}
	call(t, cs, "kando_search_sites", map[string]any{"query": "vimeo", "limit": 3})
	if got := string(f.lastParams(remote.MethodSites)); got != `{"query":"vimeo","limit":3}` {
		t.Errorf("sites params = %s", got)
	}
}

func TestChangingTools(t *testing.T) {
	path := shortSocket(t)
	f := newFake()
	serveFake(t, path, f)
	cs := connect(t)

	added := structured[remote.AddResult](t, call(t, cs, "kando_add_videos", map[string]any{"sources": []string{"https://youtu.be/z", "notes.txt"}}))
	if len(added.Jobs) != 1 || added.Jobs[0].ID != "j9" || len(added.Skipped) != 1 || added.Skipped[0].Reason != "not a video" {
		t.Errorf("added = %+v", added)
	}
	params, err := remote.Params[remote.AddParams](f.lastParams(remote.MethodAdd))
	if err != nil {
		t.Fatal(err)
	}
	if wd, _ := os.Getwd(); params.Dir != wd {
		t.Errorf("add dir = %q, want the server's directory %q", params.Dir, wd)
	}
	call(t, cs, "kando_add_videos", map[string]any{"sources": []string{"a.mp4"}, "dir": "/videos"})
	if params, _ := remote.Params[remote.AddParams](f.lastParams(remote.MethodAdd)); params.Dir != "/videos" {
		t.Errorf("add dir = %q, want /videos", params.Dir)
	}

	job := structured[remote.Job](t, call(t, cs, "kando_cancel_job", map[string]any{"id": "j1"}))
	if job.ID != "j1" || !job.Canceled {
		t.Errorf("canceled job = %+v", job)
	}

	retried := structured[remote.Job](t, call(t, cs, "kando_retry_job", map[string]any{"id": "j0"}))
	if retried.ID != "j0" || retried.Stage != "queued" {
		t.Errorf("retried job = %+v", retried)
	}
	if got := string(f.lastParams(remote.MethodRetry)); got != `{"id":"j0"}` {
		t.Errorf("retry params = %s", got)
	}
	if res := call(t, cs, "kando_retry_job", map[string]any{"id": "j7"}); !res.IsError || !strings.Contains(text(t, res), "no job j7") {
		t.Errorf("retry of an unknown job = %v %q", res.IsError, text(t, res))
	}

	res := call(t, cs, "kando_remove_job", map[string]any{"id": "j0"})
	if removed := structured[removeJobOutput](t, res); removed.Removed != "j0" || !strings.Contains(text(t, res), "Job j0 was removed") {
		t.Errorf("remove = %+v, %q", removed, text(t, res))
	}
	if got := string(f.lastParams(remote.MethodRemove)); got != `{"id":"j0"}` {
		t.Errorf("remove params = %s", got)
	}
	if res := call(t, cs, "kando_remove_job", map[string]any{"id": "j7"}); !res.IsError || !strings.Contains(text(t, res), "no job j7") {
		t.Errorf("remove of an unknown job = %v %q", res.IsError, text(t, res))
	}

	res = call(t, cs, "kando_open", map[string]any{"id": "f1"})
	if shown := structured[openOutput](t, res); shown.Shown != "f1" || !strings.Contains(text(t, res), "film f1") {
		t.Errorf("open = %+v, %q", shown, text(t, res))
	}
	if got := string(f.lastParams(remote.MethodShow)); got != `{"id":"f1"}` {
		t.Errorf("show params = %s", got)
	}
	res = call(t, cs, "kando_open", nil)
	if !strings.Contains(text(t, res), "library") || string(f.lastParams(remote.MethodShow)) != `{"id":""}` {
		t.Errorf("open library = %q, params %s", text(t, res), f.lastParams(remote.MethodShow))
	}
}

func TestSubtitles(t *testing.T) {
	path := shortSocket(t)
	serveFake(t, path, newFake())
	cs := connect(t)

	res := call(t, cs, "kando_get_subtitles", map[string]any{"id": "f1"})
	wantText := "[00:00:00] 你好 | Hello there\n[00:00:01] Untranslated line\n[01:02:05] 再见 | Goodbye"
	if got := text(t, res); got != wantText {
		t.Errorf("text =\n%s\nwant\n%s", got, wantText)
	}
	out := structured[subtitlesOutput](t, res)
	if out.Which != "both" || out.Total != 3 || out.Shown != 3 || out.Cut || len(out.Cues) != 3 || out.Cues[0].Translation != "你好" {
		t.Errorf("structured = %+v", out)
	}

	res = call(t, cs, "kando_get_subtitles", map[string]any{"id": "f1", "which": "original"})
	if got := text(t, res); got != "[00:00:00] Hello there\n[00:00:01] Untranslated line\n[01:02:05] Goodbye" {
		t.Errorf("original text = %q", got)
	}
	if out := structured[subtitlesOutput](t, res); out.Cues[0].Translation != "" {
		t.Errorf("original cues keep the translation: %+v", out.Cues)
	}

	res = call(t, cs, "kando_get_subtitles", map[string]any{"id": "f1", "which": "translation"})
	if got := text(t, res); got != "[00:00:00] 你好\n[01:02:05] 再见" {
		t.Errorf("translation text = %q", got)
	}
	if out := structured[subtitlesOutput](t, res); out.Total != 2 || out.Cues[0].Text != "" {
		t.Errorf("translation structured = %+v", out)
	}

	res = call(t, cs, "kando_get_subtitles", map[string]any{"id": "f1", "format": "srt"})
	wantSRT := "1\n00:00:00,000 --> 00:00:01,500\n你好\nHello there\n\n2\n00:00:01,500 --> 00:00:03,725\nUntranslated\nline\n\n3\n01:02:05,250 --> 01:02:07,000\n再见\nGoodbye\n"
	if got := text(t, res); got != wantSRT {
		t.Errorf("srt =\n%q\nwant\n%q", got, wantSRT)
	}

	res = call(t, cs, "kando_get_subtitles", map[string]any{"id": "f1", "max_cues": 2})
	if len(res.Content) != 2 {
		t.Fatalf("truncated result has %d content blocks, want the text and a note", len(res.Content))
	}
	got := text(t, res)
	if !strings.HasPrefix(got, "[00:00:00] 你好 | Hello there\n[00:00:01] Untranslated line\nTruncated: these are the first 2 of 3 subtitle lines, up to 00:00:03.") {
		t.Errorf("truncated text = %q", got)
	}
	if out := structured[subtitlesOutput](t, res); out.Total != 3 || out.Shown != 2 || !out.Cut || len(out.Cues) != 2 {
		t.Errorf("truncated structured = %+v", out)
	}

	res = call(t, cs, "kando_get_subtitles", map[string]any{"id": "f1", "which": "both", "max_cues": 3})
	if len(res.Content) != 1 || structured[subtitlesOutput](t, res).Cut {
		t.Errorf("a cap at the length truncates: %q", text(t, res))
	}

	res = call(t, cs, "kando_get_subtitles", map[string]any{"id": "f1", "which": "everything"})
	if !res.IsError {
		t.Errorf("which everything was accepted: %q", text(t, res))
	}
}

func TestAppErrorsAreToolErrors(t *testing.T) {
	path := shortSocket(t)
	serveFake(t, path, newFake())
	cs := connect(t)

	for _, tc := range []struct {
		tool string
		args map[string]any
		want string
	}{
		{"kando_get_film", map[string]any{"id": "nope"}, "not_found: no film nope"},
		{"kando_get_subtitles", map[string]any{"id": "nope"}, "not_found: no film nope"},
	} {
		res := call(t, cs, tc.tool, tc.args)
		if !res.IsError || text(t, res) != tc.want {
			t.Errorf("%s = %v %q, want error %q", tc.tool, res.IsError, text(t, res), tc.want)
		}
	}
}

func TestNotReady(t *testing.T) {
	path := shortSocket(t)
	s, err := remote.Listen(path, func(method string, params json.RawMessage) (any, error) {
		return nil, remote.Errorf(remote.CodeNotReady, "open a library folder first")
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	cs := connect(t)
	res := call(t, cs, "kando_add_videos", map[string]any{"sources": []string{"https://youtu.be/z"}})
	if !res.IsError || text(t, res) != "not_ready: open a library folder first" {
		t.Errorf("add = %v %q", res.IsError, text(t, res))
	}
}

func TestNotRunning(t *testing.T) {
	shortSocket(t)
	cs := connect(t)
	res := call(t, cs, "kando_status", nil)
	if !res.IsError || !strings.Contains(text(t, res), "start Kando: launch disabled in tests") {
		t.Errorf("status without the app = %v %q", res.IsError, text(t, res))
	}
}

// TestRedial restarts the app between calls: each call goes through on a new connection, adding videos too, as the old connection refused the request without the app reading it.
func TestRedial(t *testing.T) {
	path := shortSocket(t)
	first := newFake()
	s := serveFake(t, path, first)
	cs := connect(t)
	if res := call(t, cs, "kando_status", nil); res.IsError {
		t.Fatalf("status = %q", text(t, res))
	}

	s.Close()
	second := newFake()
	s = serveFake(t, path, second)
	if res := call(t, cs, "kando_list_jobs", nil); res.IsError || second.count(remote.MethodJobs) != 1 {
		t.Fatalf("jobs after a restart = %v %q, %d calls", res.IsError, text(t, res), second.count(remote.MethodJobs))
	}

	s.Close()
	third := newFake()
	serveFake(t, path, third)
	if res := call(t, cs, "kando_add_videos", map[string]any{"sources": []string{"https://youtu.be/z"}}); res.IsError || third.count(remote.MethodAdd) != 1 {
		t.Errorf("add after a restart = %v %q, %d calls", res.IsError, text(t, res), third.count(remote.MethodAdd))
	}
}

// TestLostAddAnswer is an app that reads a request to add videos, or to retry a job, and quits before it answers: the request is not sent again, as the videos or the job may be queued, and the error says to look at the jobs. A read-only request lost the same way is sent again.
func TestLostAddAnswer(t *testing.T) {
	path := shortSocket(t)
	listener, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { listener.Close() })
	var mu sync.Mutex
	got := map[string]int{}
	go func() {
		for {
			conn, err := listener.Accept()
			if err != nil {
				return
			}
			// jobs.list and jobs.remove are answered from their second request on, and the connection stays open after an answer. Every other request, and the first of those two, gets no answer and the connection closes, as when the app quits after reading it.
			go func() {
				defer conn.Close()
				r := bufio.NewReader(conn)
				for {
					line, err := r.ReadBytes('\n')
					if err != nil {
						return
					}
					var req remote.Request
					json.Unmarshal(line, &req)
					mu.Lock()
					got[req.Method]++
					n := got[req.Method]
					mu.Unlock()
					switch {
					case req.Method == remote.MethodJobs && n > 1:
						json.NewEncoder(conn).Encode(remote.Response{ID: req.ID, Result: json.RawMessage(`[]`)})
					case req.Method == remote.MethodRemove && n > 1:
						json.NewEncoder(conn).Encode(remote.Response{ID: req.ID, Error: remote.Errorf(remote.CodeNotFound, "no job %q in the open library", "j0")})
					default:
						return
					}
				}
			}()
		}
	}()
	cs := connect(t)

	res := call(t, cs, "kando_add_videos", map[string]any{"sources": []string{"https://youtu.be/z"}})
	if !res.IsError || !strings.Contains(text(t, res), "check kando_list_jobs before adding them again") {
		t.Errorf("add with a lost answer = %v %q", res.IsError, text(t, res))
	}
	res = call(t, cs, "kando_retry_job", map[string]any{"id": "j0"})
	if !res.IsError || !strings.Contains(text(t, res), "check kando_list_jobs before retrying it again") {
		t.Errorf("retry with a lost answer = %v %q", res.IsError, text(t, res))
	}
	res = call(t, cs, "kando_list_jobs", nil)
	if out := structured[listJobsOutput](t, res); len(out.Jobs) != 0 {
		t.Errorf("jobs = %+v", out)
	}
	res = call(t, cs, "kando_remove_job", map[string]any{"id": "j0"})
	if res.IsError || structured[removeJobOutput](t, res).Removed != "j0" {
		t.Errorf("remove with a lost answer = %v %q, want it removed, as the first request did", res.IsError, text(t, res))
	}
	mu.Lock()
	defer mu.Unlock()
	if got[remote.MethodAdd] != 1 || got[remote.MethodRetry] != 1 || got[remote.MethodRemove] != 2 || got[remote.MethodJobs] != 2 {
		t.Errorf("requests = %v, want the add and the retry once and the remove and the jobs twice", got)
	}
}

// runEnv makes the test binary serve Run on its standard input and output instead of running the tests, for TestRunOnStdio.
const runEnv = "KANDO_MCP_TEST_RUN"

func TestMain(m *testing.M) {
	if os.Getenv(runEnv) == "1" {
		if err := Run(context.Background(), "9.9.9"); err != nil {
			fmt.Fprintln(os.Stderr, err)
			os.Exit(1)
		}
		os.Exit(0)
	}
	os.Exit(m.Run())
}

// TestRunOnStdio serves Run in a child process, as `kando mcp` does, and talks to it over its standard input and output, which fails if anything but the protocol is written there.
func TestRunOnStdio(t *testing.T) {
	path := shortSocket(t)
	serveFake(t, path, newFake())
	cmd := exec.Command(os.Args[0])
	cmd.Env = append(os.Environ(), runEnv+"=1")
	cmd.Stderr = os.Stderr
	cs, err := mcp.NewClient(&mcp.Implementation{Name: "test", Version: "0"}, nil).Connect(t.Context(), &mcp.CommandTransport{Command: cmd}, nil)
	if err != nil {
		t.Fatal(err)
	}
	if info := cs.InitializeResult().ServerInfo; info.Name != "kando" || info.Version != "9.9.9" {
		t.Errorf("server info = %+v", info)
	}
	if status := structured[remote.Status](t, call(t, cs, "kando_status", nil)); status.LibraryDir != "/lib" {
		t.Errorf("status = %+v", status)
	}
	if err := cs.Close(); err != nil {
		t.Errorf("close: %v", err)
	}
	if cmd.ProcessState == nil || !cmd.ProcessState.Success() {
		t.Errorf("server process ended with %v", cmd.ProcessState)
	}
}
