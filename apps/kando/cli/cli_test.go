package cli

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"kando/apps/kando/remote"
)

// call is one request the fake app was asked.
type call struct {
	method string
	params json.RawMessage
}

// fakeApp answers on a short socket in place of the running app, with what handle returns, and keeps what it was asked.
type fakeApp struct {
	mu     sync.Mutex
	calls  []call
	handle func(method string, params json.RawMessage) (any, error)
}

func (f *fakeApp) asked() []call {
	f.mu.Lock()
	defer f.mu.Unlock()
	return slices.Clone(f.calls)
}

// lastParams decodes the params of the last request for method into out.
func (f *fakeApp) lastParams(t *testing.T, method string, out any) {
	t.Helper()
	calls := f.asked()
	for i := len(calls) - 1; i >= 0; i-- {
		if calls[i].method == method {
			if err := json.Unmarshal(calls[i].params, out); err != nil {
				t.Fatalf("params of %s: %v", method, err)
			}
			return
		}
	}
	t.Fatalf("the app was not asked %s; asked %+v", method, calls)
}

// shortSocket points KANDO_SOCKET at a path short enough for macOS's 104-byte limit on socket paths, which t.TempDir's can pass, and returns it.
func shortSocket(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "k")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	path := filepath.Join(dir, "k.sock")
	t.Setenv("KANDO_SOCKET", path)
	return path
}

// noLaunch makes starting the app a test failure, so no test opens the real one.
func noLaunch(t *testing.T) {
	t.Helper()
	old := remote.Launch
	remote.Launch = func() error {
		t.Error("the command started the app")
		return errors.New("no app in tests")
	}
	t.Cleanup(func() { remote.Launch = old })
}

func startFake(t *testing.T, handle func(method string, params json.RawMessage) (any, error)) *fakeApp {
	t.Helper()
	path := shortSocket(t)
	noLaunch(t)
	f := &fakeApp{handle: handle}
	f.listen(t, path)
	return f
}

func (f *fakeApp) listen(t *testing.T, path string) {
	t.Helper()
	s, err := remote.Listen(path, func(method string, params json.RawMessage) (any, error) {
		f.mu.Lock()
		f.calls = append(f.calls, call{method: method, params: append(json.RawMessage(nil), params...)})
		f.mu.Unlock()
		return f.handle(method, params)
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
}

var (
	cannedStatus = remote.Status{
		Version:          "1.2.3",
		LibraryDir:       "/Users/me/Movies/Kando",
		Ready:            true,
		Readiness:        "Ready",
		Tools:            map[string]string{"yt-dlp": "/bin/yt-dlp", "ffmpeg": "/bin/ffmpeg", "whisper-cli": ""},
		SpeechModel:      "ggml-large-v3.bin",
		TranslationModel: "gpt-5",
		Language:         "Chinese",
		Running:          "j1",
		Queued:           2,
	}
	cannedFilms = []remote.Film{
		{ID: "f1", Title: "Rivers of Japan", Source: "youtube", Link: "https://youtu.be/x", Stage: "transcribing", Progress: 0.42, JobID: "j1"},
		{ID: "f2", Title: "Home video", Source: "local", Progress: -1},
	}
	cannedFilm = remote.Film{ID: "f1", Title: "Rivers of Japan", Source: "youtube", Link: "https://youtu.be/x", Stage: "complete", Progress: 1, Status: "完成", JobID: "j1", Language: "Chinese", VideoPath: "/lib/rivers.mp4", Subtitles: "/lib/rivers.bilingual.srt", OriginalSubtitles: "/lib/rivers.srt"}
	cannedCues = []remote.Cue{
		{StartMS: 1500, EndMS: 3000, Text: "Hello there.", Translation: "你好。"},
		{StartMS: 3_725_250, EndMS: 3_727_000, Text: "Untranslated."},
	}
	cannedJobs = []remote.Job{
		{ID: "j1", Source: "https://youtu.be/x", Title: "Rivers of Japan", Stage: "downloading", Progress: 0.5},
		{ID: "j2", Source: "/tmp/a.mp4", Stage: "queued"},
		{ID: "j3", Source: "https://b23.tv/y", Title: "Cats", Stage: "failed", Error: "network down"},
		{ID: "j4", Source: "https://youtu.be/z", Stage: "failed", Canceled: true},
	}
	cannedRetried = remote.Job{ID: "j3", Source: "https://b23.tv/y", Title: "Cats", Stage: "queued"}
	cannedSites   = []remote.Site{{Name: "YouTube", Domain: "youtube.com"}, {Name: "Bilibili", Domain: "bilibili.com"}}
)

// cannedApp answers every method with the canned results, and not_found for ids other than f1, j1 and, for retry and remove, j3.
func cannedApp(method string, params json.RawMessage) (any, error) {
	switch method {
	case remote.MethodStatus:
		return cannedStatus, nil
	case remote.MethodFilms:
		return cannedFilms, nil
	case remote.MethodFilm, remote.MethodSubtitles, remote.MethodShow:
		p, err := remote.Params[remote.IDParams](params)
		if err != nil {
			return nil, err
		}
		if method == remote.MethodShow && p.ID == "" {
			return nil, nil
		}
		if p.ID != "f1" {
			return nil, remote.Errorf(remote.CodeNotFound, "No film %s.", p.ID)
		}
		switch method {
		case remote.MethodFilm:
			return cannedFilm, nil
		case remote.MethodSubtitles:
			return cannedCues, nil
		}
		return nil, nil
	case remote.MethodJobs:
		return cannedJobs, nil
	case remote.MethodCancel:
		p, err := remote.Params[remote.IDParams](params)
		if err != nil {
			return nil, err
		}
		if p.ID != "j1" {
			return nil, remote.Errorf(remote.CodeNotFound, "No job %s.", p.ID)
		}
		return remote.Job{ID: "j1", Source: "https://youtu.be/x", Title: "Rivers of Japan", Stage: "failed", Canceled: true}, nil
	case remote.MethodRetry, remote.MethodRemove:
		p, err := remote.Params[remote.IDParams](params)
		if err != nil {
			return nil, err
		}
		if p.ID != "j3" {
			return nil, remote.Errorf(remote.CodeNotFound, "No job %s.", p.ID)
		}
		if method == remote.MethodRemove {
			return nil, nil
		}
		return cannedRetried, nil
	case remote.MethodMove:
		p, err := remote.Params[remote.MoveParams](params)
		if err != nil {
			return nil, err
		}
		if p.ID != "f1" {
			return nil, remote.Errorf(remote.CodeNotFound, "No film %s.", p.ID)
		}
		return remote.Moved{ID: "f1", Library: "/Users/me/Movies/English.kando", Name: "English"}, nil
	case remote.MethodSites:
		return cannedSites, nil
	case remote.MethodAdd:
		return remote.AddResult{Jobs: []remote.Job{{ID: "j9", Source: "https://youtu.be/new", Stage: "queued"}}}, nil
	}
	return nil, remote.Errorf(remote.CodeUnknownMethod, "%s", method)
}

func runCtx(ctx context.Context, args ...string) (int, string, string) {
	var stdout, stderr bytes.Buffer
	code := Run(ctx, args, &stdout, &stderr, "1.2.3")
	return code, stdout.String(), stderr.String()
}

func run(args ...string) (int, string, string) {
	return runCtx(context.Background(), args...)
}

func TestOpensWindow(t *testing.T) {
	dir, err := os.MkdirTemp("", "k")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	clip := filepath.Join(dir, "clip.mp4")
	if err := os.WriteFile(clip, nil, 0o644); err != nil {
		t.Fatal(err)
	}
	// A file named like a command in the current folder does not stop the command.
	if err := os.WriteFile(filepath.Join(dir, "status"), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	for _, args := range [][]string{
		nil,
		{},
		{"-psn_0_123456"},
		{"-NSDocumentRevisionsDebugMode", "YES"},
		{"-AppleLanguages", "(ja)"},
		{clip},
		{"clip.mp4"},
		{dir},
		{"kando://film/f1"},
		{"https://youtu.be/x"},
	} {
		if !OpensWindow(args) {
			t.Errorf("OpensWindow(%q) = false, want the window", args)
		}
		if code, ok := Main(args, "1.2.3"); ok || code != 0 {
			t.Errorf("Main(%q) = %d, %v, want 0, false", args, code, ok)
		}
	}
	for _, args := range [][]string{
		{"stauts"},
		{"bogus"},
		{"missing.mp4"},
		{"--json", "status"},
		{"-v"},
		{"--bogus"},
		{"status"},
		{"help", "-h", "--help", "version", "--version", "list", "show", "subtitles", "jobs", "add", "cancel", "retry", "remove", "move", "sites", "open", "mcp"},
	} {
		for _, name := range args {
			if OpensWindow([]string{name}) {
				t.Errorf("OpensWindow(%q) = true, want the command line", name)
			}
		}
	}
}

func TestUnknownCommand(t *testing.T) {
	f := startFake(t, cannedApp)
	for _, tt := range []struct {
		args []string
		want string
	}{
		{[]string{"stauts"}, `kando: unknown command "stauts"`},
		{[]string{"bogus", "f1"}, `kando: unknown command "bogus"`},
		{[]string{"-v"}, `kando: unknown flag "-v"`},
		{[]string{"--json"}, "kando: --json needs a command after it"},
		{[]string{"--json", "bogus"}, `kando: unknown command "bogus"`},
	} {
		code, stdout, stderr := run(tt.args...)
		if code != 2 || stdout != "" || !strings.HasPrefix(stderr, tt.want+"\n") || !strings.Contains(stderr, "Usage: kando <command>") || !strings.Contains(stderr, "subtitles") {
			t.Errorf("%q = %d, %q, %q, want exit 2, %q and the usage", tt.args, code, stdout, stderr, tt.want)
		}
	}
	if calls := f.asked(); len(calls) != 0 {
		t.Errorf("unknown commands asked the app %+v", calls)
	}
}

func TestLeadingJSON(t *testing.T) {
	startFake(t, cannedApp)
	for _, args := range [][]string{{"--json", "status"}, {"-json", "status"}, {"--json=true", "status"}} {
		code, stdout, stderr := run(args...)
		if code != 0 || stderr != "" {
			t.Errorf("%q = %d, %q", args, code, stderr)
			continue
		}
		assertSameJSON(t, args, stdout, cannedStatus)
	}
	code, stdout, stderr := run("--json", "subtitles", "f1", "--original")
	if code != 0 || stderr != "" {
		t.Errorf("--json subtitles = %d, %q", code, stderr)
	}
	assertSameJSON(t, []string{"--json", "subtitles"}, stdout, []remote.Cue{{StartMS: 1500, EndMS: 3000, Text: "Hello there."}, {StartMS: 3_725_250, EndMS: 3_727_000, Text: "Untranslated."}})
	// version takes no --json, and says so as its usage error.
	if code, stdout, stderr := run("--json", "version"); code != 2 || stdout != "" || !strings.Contains(stderr, "kando version: flag provided but not defined: -json") {
		t.Errorf("--json version = %d, %q, %q", code, stdout, stderr)
	}
}

func TestVersionNeedsNoApp(t *testing.T) {
	shortSocket(t)
	noLaunch(t)
	for _, args := range [][]string{{"version"}, {"--version"}} {
		code, stdout, stderr := run(args...)
		if code != 0 || stdout != "kando 1.2.3\n" || stderr != "" {
			t.Errorf("%q = %d, %q, %q", args, code, stdout, stderr)
		}
	}
}

func TestHelp(t *testing.T) {
	shortSocket(t)
	noLaunch(t)
	for _, args := range [][]string{{"help"}, {"-h"}, {"--help"}} {
		code, stdout, _ := run(args...)
		if code != 0 || !strings.Contains(stdout, "Usage: kando <command>") || !strings.Contains(stdout, "subtitles") {
			t.Errorf("%q = %d, %q", args, code, stdout)
		}
	}
	code, stdout, _ := run("help", "add")
	if code != 0 || !strings.Contains(stdout, "Usage: kando add <link-or-file>... [--wait] [--yes] [--json]") {
		t.Errorf("help add = %d, %q", code, stdout)
	}
	code, stdout, _ = run("list", "-h")
	if code != 0 || !strings.Contains(stdout, "Usage: kando list [--source S] [--language L] [query] [--json]") {
		t.Errorf("list -h = %d, %q", code, stdout)
	}
	if code, _, stderr := run("help", "bogus"); code != 2 || !strings.Contains(stderr, `unknown command "bogus"`) {
		t.Errorf("help bogus = %d, %q", code, stderr)
	}
	if code, _, stderr := run("bogus"); code != 2 || !strings.Contains(stderr, "kando help") {
		t.Errorf("bogus = %d, %q", code, stderr)
	}
}

func TestHelpListsFlags(t *testing.T) {
	shortSocket(t)
	noLaunch(t)
	wantSubtitles := "Usage: kando subtitles <film-id> [--format srt|text|json] [--original|--translation] [--from T] [--to T] [--until watched] [--json]\n" +
		"\n" +
		"Print a film's subtitles, or part of them.\n" +
		"\n" +
		"Flags:\n" +
		"  --format string  srt, text or json (default srt)\n" +
		"  --from string    only the lines that start at this time or later: seconds, mm:ss or hh:mm:ss\n" +
		"  --json           print the app's answer as JSON\n" +
		"  --original       only the original speech\n" +
		"  --to string      only the lines that start before this time\n" +
		"  --translation    only the translation\n" +
		"  --until string   watched: only the lines up to how far the film was played, giving nothing away of the rest\n"
	for _, args := range [][]string{{"help", "subtitles"}, {"subtitles", "-h"}, {"subtitles", "f1", "--help"}} {
		if code, stdout, stderr := run(args...); code != 0 || stdout != wantSubtitles || stderr != "" {
			t.Errorf("%q = %d\nstdout:\n%s\nwant:\n%s\nstderr: %q", args, code, stdout, wantSubtitles, stderr)
		}
	}
	for _, tt := range []struct {
		command string
		flags   []string
	}{
		{"status", []string{"--json"}},
		{"list", []string{"--source string", "--json"}},
		{"add", []string{"--wait", "--json"}},
		{"retry", []string{"--wait", "--json"}},
		{"remove", []string{"--json"}},
		{"move", []string{"--to string", "--json"}},
		{"sites", []string{"--limit int", "--json"}},
		{"search", []string{"--film string", "--until string", "--limit int", "--json"}},
		{"open", []string{"--at string", "--json"}},
	} {
		_, stdout, _ := run("help", tt.command)
		for _, flag := range tt.flags {
			if !strings.Contains(stdout, "Flags:\n") || !strings.Contains(stdout, "  "+flag+" ") {
				t.Errorf("help %s = %q, want it to list %s", tt.command, stdout, flag)
			}
		}
	}
	// A command without flags shows no empty Flags heading.
	for _, command := range []string{"version", "mcp", "help"} {
		if code, stdout, _ := run("help", command); code != 0 || strings.Contains(stdout, "Flags:") || !strings.Contains(stdout, "Usage: kando "+command) {
			t.Errorf("help %s = %d, %q", command, code, stdout)
		}
	}
}

func TestTextOutput(t *testing.T) {
	startFake(t, cannedApp)
	for _, tt := range []struct {
		args       []string
		want       string
		wantStderr string
	}{
		{
			args: []string{"status"},
			want: "version            1.2.3\n" +
				"library            /Users/me/Movies/Kando\n" +
				"ready              yes\n" +
				"readiness          Ready\n" +
				"speech model       ggml-large-v3.bin\n" +
				"translation model  gpt-5\n" +
				"language           Chinese\n" +
				"running            j1\n" +
				"queued             2\n" +
				"ffmpeg             /bin/ffmpeg\n" +
				"whisper-cli        missing\n" +
				"yt-dlp             /bin/yt-dlp\n",
		},
		{
			args: []string{"list"},
			want: "ID  SOURCE   STAGE             TITLE\n" +
				"f1  youtube  transcribing 42%  Rivers of Japan\n" +
				"f2  local    -                 Home video\n",
		},
		{
			args: []string{"show", "f1"},
			want: "id                  f1\n" +
				"title               Rivers of Japan\n" +
				"source              youtube\n" +
				"link                https://youtu.be/x\n" +
				"stage               complete\n" +
				"status              完成\n" +
				"job                 j1\n" +
				"language            Chinese\n" +
				"video               /lib/rivers.mp4\n" +
				"subtitles           /lib/rivers.bilingual.srt\n" +
				"original subtitles  /lib/rivers.srt\n",
		},
		{
			args: []string{"subtitles", "f1"},
			want: "1\n00:00:01,500 --> 00:00:03,000\n你好。\nHello there.\n\n" +
				"2\n01:02:05,250 --> 01:02:07,000\nUntranslated.\n\n",
		},
		{
			args: []string{"subtitles", "f1", "--original"},
			want: "1\n00:00:01,500 --> 00:00:03,000\nHello there.\n\n" +
				"2\n01:02:05,250 --> 01:02:07,000\nUntranslated.\n\n",
		},
		{
			args: []string{"subtitles", "--translation", "f1"},
			want: "1\n00:00:01,500 --> 00:00:03,000\n你好。\n\n",
		},
		{
			args: []string{"subtitles", "f1", "--format", "text"},
			want: "[00:00:01] 你好。 | Hello there.\n[01:02:05] Untranslated.\n",
		},
		{
			args: []string{"subtitles", "f1", "--format=text", "--original"},
			want: "[00:00:01] Hello there.\n[01:02:05] Untranslated.\n",
		},
		{
			args: []string{"jobs"},
			want: "j1  downloading 50%  Rivers of Japan\n" +
				"j2  queued           /tmp/a.mp4\n" +
				"j3  failed           Cats  network down\n" +
				"j4  canceled         https://youtu.be/z\n",
		},
		{
			args: []string{"cancel", "j1"},
			want: "j1  canceled  Rivers of Japan\n",
		},
		{
			args: []string{"retry", "j3"},
			want: "j3  queued  Cats\n",
		},
		{args: []string{"remove", "j3"}, want: ""},
		{args: []string{"move", "f1", "--to", "English"}, want: "f1\t/Users/me/Movies/English.kando\n"},
		{
			args: []string{"sites", "you"},
			want: "YouTube   youtube.com\nBilibili  bilibili.com\n",
		},
		{
			args: []string{"add", "https://youtu.be/new"},
			want: "j9\thttps://youtu.be/new\n",
		},
		{args: []string{"open"}, want: ""},
		{args: []string{"open", "f1"}, want: ""},
	} {
		code, stdout, stderr := run(tt.args...)
		if code != 0 || stdout != tt.want || stderr != tt.wantStderr {
			t.Errorf("%q = %d\nstdout:\n%s\nwant:\n%s\nstderr: %q", tt.args, code, stdout, tt.want, stderr)
		}
	}
}

func TestEmptyLists(t *testing.T) {
	startFake(t, func(method string, params json.RawMessage) (any, error) {
		return []any{}, nil
	})
	for _, tt := range []struct {
		args   []string
		stderr string
	}{
		{[]string{"list"}, "No films.\n"},
		{[]string{"jobs"}, "No jobs.\n"},
		{[]string{"sites", "nothing"}, "No sites found.\n"},
	} {
		code, stdout, stderr := run(tt.args...)
		if code != 0 || stdout != "" || stderr != tt.stderr {
			t.Errorf("%q = %d, %q, %q", tt.args, code, stdout, stderr)
		}
	}
}

func TestJSONOutput(t *testing.T) {
	startFake(t, cannedApp)
	for _, tt := range []struct {
		args []string
		want any
	}{
		{[]string{"status", "--json"}, cannedStatus},
		{[]string{"list", "rivers", "--json"}, cannedFilms},
		{[]string{"show", "f1", "--json"}, cannedFilm},
		{[]string{"subtitles", "f1", "--json"}, cannedCues},
		{[]string{"subtitles", "f1", "--format", "json", "--original"}, []remote.Cue{
			{StartMS: 1500, EndMS: 3000, Text: "Hello there."},
			{StartMS: 3_725_250, EndMS: 3_727_000, Text: "Untranslated."},
		}},
		{[]string{"subtitles", "f1", "--json", "--translation"}, []remote.Cue{
			{StartMS: 1500, EndMS: 3000, Translation: "你好。"},
		}},
		{[]string{"subtitles", "f1", "--format=json"}, cannedCues},
		{[]string{"jobs", "--json"}, cannedJobs},
		{[]string{"cancel", "--json", "j1"}, remote.Job{ID: "j1", Source: "https://youtu.be/x", Title: "Rivers of Japan", Stage: "failed", Canceled: true}},
		{[]string{"retry", "j3", "--json"}, cannedRetried},
		{[]string{"remove", "--json", "j3"}, nil},
		{[]string{"sites", "--json"}, cannedSites},
		{[]string{"add", "https://youtu.be/new", "--json"}, remote.AddResult{Jobs: []remote.Job{{ID: "j9", Source: "https://youtu.be/new", Stage: "queued"}}}},
		{[]string{"open", "--json"}, nil},
	} {
		args := tt.args
		code, stdout, stderr := run(args...)
		if code != 0 || stderr != "" {
			t.Errorf("%q = %d, stderr %q", args, code, stderr)
			continue
		}
		if !strings.Contains(stdout, "\n  ") && tt.want != nil {
			t.Errorf("%q printed unindented JSON: %q", args, stdout)
		}
		assertSameJSON(t, args, stdout, tt.want)
	}
}

// assertSameJSON checks that got is want marshaled, compared as decoded values.
func assertSameJSON(t *testing.T, args []string, got string, want any) {
	t.Helper()
	wantData, err := json.Marshal(want)
	if err != nil {
		t.Fatal(err)
	}
	var gotValue, wantValue any
	if err := json.Unmarshal([]byte(got), &gotValue); err != nil {
		t.Errorf("%q printed %q, not JSON: %v", args, got, err)
		return
	}
	if err := json.Unmarshal(wantData, &wantValue); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(gotValue, wantValue) {
		t.Errorf("%q printed\n%s\nwant\n%s", args, got, wantData)
	}
}

func TestParamsSent(t *testing.T) {
	f := startFake(t, cannedApp)
	dir, err := os.MkdirTemp("", "kd")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	t.Chdir(dir)
	cwd, err := os.Getwd()
	if err != nil {
		t.Fatal(err)
	}

	if code, _, stderr := run("list", "rivers", "of", "--source", "youtube", "japan"); code != 0 {
		t.Fatalf("list = %d, %q", code, stderr)
	}
	var films remote.FilmsParams
	f.lastParams(t, remote.MethodFilms, &films)
	if films != (remote.FilmsParams{Source: "youtube", Query: "rivers of japan"}) {
		t.Errorf("films params = %+v", films)
	}

	if code, _, stderr := run("add", "clip.mp4", "--", "--wait"); code != 0 {
		t.Fatalf("add = %d, %q", code, stderr)
	}
	var add remote.AddParams
	f.lastParams(t, remote.MethodAdd, &add)
	if !reflect.DeepEqual(add, remote.AddParams{Sources: []string{"clip.mp4", "--wait"}, Dir: cwd}) {
		t.Errorf("add params = %+v, want the sources and dir %s", add, cwd)
	}

	if code, _, stderr := run("sites", "--limit", "3", "you", "tube"); code != 0 {
		t.Fatalf("sites = %d, %q", code, stderr)
	}
	var sites remote.SitesParams
	f.lastParams(t, remote.MethodSites, &sites)
	if sites != (remote.SitesParams{Query: "you tube", Limit: 3}) {
		t.Errorf("sites params = %+v", sites)
	}
	// A library named by its folder is sent by its absolute path; one named by its name as it is.
	lib := t.TempDir()
	t.Chdir(filepath.Dir(lib))
	for _, tt := range []struct{ to, want string }{{"English", "English"}, {filepath.Base(lib), lib}} {
		if code, _, stderr := run("move", "f1", "--to", tt.to); code != 0 {
			t.Fatalf("move --to %s = %d, %q", tt.to, code, stderr)
		}
		var move remote.MoveParams
		f.lastParams(t, remote.MethodMove, &move)
		if got, _ := filepath.EvalSymlinks(move.To); move.ID != "f1" || (move.To != tt.want && got != tt.want) {
			t.Errorf("move --to %s sent %+v, want %q", tt.to, move, tt.want)
		}
	}

	for _, tt := range []struct {
		args   []string
		method string
		id     string
	}{
		{[]string{"show", "f1"}, remote.MethodFilm, "f1"},
		{[]string{"subtitles", "f1"}, remote.MethodSubtitles, "f1"},
		{[]string{"cancel", "j1"}, remote.MethodCancel, "j1"},
		{[]string{"retry", "j3"}, remote.MethodRetry, "j3"},
		{[]string{"remove", "j3"}, remote.MethodRemove, "j3"},
		{[]string{"open", "f1"}, remote.MethodShow, "f1"},
		{[]string{"open"}, remote.MethodShow, ""},
	} {
		if code, _, stderr := run(tt.args...); code != 0 {
			t.Fatalf("%q = %d, %q", tt.args, code, stderr)
		}
		var p remote.IDParams
		f.lastParams(t, tt.method, &p)
		if p.ID != tt.id {
			t.Errorf("%q sent id %q, want %q", tt.args, p.ID, tt.id)
		}
	}
}

func TestUsageErrors(t *testing.T) {
	f := startFake(t, cannedApp)
	for _, args := range [][]string{
		{"status", "extra"},
		{"list", "--bogus"},
		{"show"},
		{"show", "f1", "f2"},
		{"subtitles"},
		{"subtitles", "f1", "--format", "xml"},
		{"subtitles", "f1", "--format"},
		{"subtitles", "f1", "--original", "--translation"},
		{"jobs", "j1", "j2"},
		{"search"},
		{"search", "  "},
		{"search", "word", "--limit", "-1"},
		{"search", "word", "--until", "tomorrow"},
		{"subtitles", "f1", "--from", "1:75"},
		{"subtitles", "f1", "--to", "soon"},
		{"subtitles", "f1", "--until", "tomorrow"},
		{"open", "--at", "1:00"},
		{"open", "f1", "--at", "later"},
		{"add"},
		{"add", "--wait"},
		{"cancel"},
		{"retry"},
		{"retry", "j3", "j4"},
		{"retry", "--wait"},
		{"retry", "j3", "--bogus"},
		{"remove"},
		{"remove", "j3", "j4"},
		{"remove", "j3", "--wait"},
		{"move"},
		{"move", "f1"},
		{"move", "f1", "f2", "--to", "English"},
		{"move", "--to", "English"},
		{"sites", "--limit", "-1"},
		{"sites", "--limit", "many"},
		{"open", "f1", "f2"},
		{"mcp", "extra"},
		{"version", "extra"},
		{"version", "--json"},
		{"help", "a", "b"},
	} {
		code, stdout, stderr := run(args...)
		if code != 2 || stdout != "" || !strings.Contains(stderr, "kando "+args[0]+": ") || !strings.Contains(stderr, "Usage: kando "+args[0]) {
			t.Errorf("%q = %d, %q, %q, want a usage error", args, code, stdout, stderr)
		}
	}
	// With --json, which --format json sets, a usage error is the one JSON value on standard output, as any other error is.
	code, stdout, stderr := run("subtitles", "f1", "--format", "json", "--original", "--translation")
	if code != 2 || stderr != "" || !strings.Contains(stdout, `"code": "usage"`) {
		t.Errorf("a usage error with --json = %d, %q, %q", code, stdout, stderr)
	}
	if calls := f.asked(); len(calls) != 0 {
		t.Errorf("usage errors asked the app %+v", calls)
	}
}

func TestRemoteErrors(t *testing.T) {
	startFake(t, func(method string, params json.RawMessage) (any, error) {
		switch method {
		case remote.MethodAdd:
			return nil, remote.Errorf(remote.CodeNotReady, "Choose a folder for the library first.")
		case remote.MethodStatus:
			return nil, errors.New("disk on fire")
		}
		return cannedApp(method, params)
	})
	for _, tt := range []struct {
		args   []string
		code   int
		stderr string
	}{
		{[]string{"show", "nope"}, 4, "kando show: No film nope.\n"},
		{[]string{"cancel", "j7"}, 4, "kando cancel: No job j7.\n"},
		{[]string{"retry", "j7"}, 4, "kando retry: No job j7.\n"},
		{[]string{"retry", "j7", "--wait"}, 4, "kando retry: No job j7.\n"},
		{[]string{"remove", "j7"}, 4, "kando remove: No job j7.\n"},
		{[]string{"move", "f7", "--to", "English"}, 4, "kando move: No film f7.\n"},
		{[]string{"add", "https://youtu.be/x"}, 3, "kando add: Choose a folder for the library first.\n"},
		{[]string{"status"}, 1, "kando status: disk on fire\n"},
	} {
		code, stdout, stderr := run(tt.args...)
		if code != tt.code || stdout != "" || stderr != tt.stderr {
			t.Errorf("%q = %d, %q, %q, want %d and %q", tt.args, code, stdout, stderr, tt.code, tt.stderr)
		}
	}
	// With --json the error is the one JSON value on standard output, with its code, and nothing goes to standard error.
	for _, tt := range []struct {
		args []string
		code int
		want string
	}{
		{[]string{"subtitles", "nope", "--json"}, 4, `{"error": {"code": "not_found", "message": "No film nope."}}`},
		{[]string{"--json", "add", "https://youtu.be/x"}, 3, `{"error": {"code": "not_ready", "message": "Choose a folder for the library first."}}`},
		{[]string{"status", "--json"}, 1, `{"error": {"code": "failed", "message": "disk on fire"}}`},
	} {
		code, stdout, stderr := run(tt.args...)
		if code != tt.code || stderr != "" {
			t.Errorf("%q = %d, %q, want %d and nothing on stderr", tt.args, code, stderr, tt.code)
		}
		var want any
		if err := json.Unmarshal([]byte(tt.want), &want); err != nil {
			t.Fatal(err)
		}
		assertSameJSON(t, tt.args, stdout, want)
	}
}

func TestAddSkipped(t *testing.T) {
	startFake(t, func(method string, params json.RawMessage) (any, error) {
		if method == remote.MethodAdd {
			return remote.AddResult{
				Jobs:    []remote.Job{{ID: "j9", Source: "https://youtu.be/new", Stage: "queued"}},
				Skipped: []remote.AddFailed{{Source: "nothing.txt", Reason: "Not a video."}},
			}, nil
		}
		return cannedApp(method, params)
	})
	code, stdout, stderr := run("add", "https://youtu.be/new", "nothing.txt")
	if code != 1 || stdout != "j9\thttps://youtu.be/new\n" || stderr != "kando add: skipped nothing.txt: Not a video.\n" {
		t.Errorf("add = %d, %q, %q", code, stdout, stderr)
	}
}

func TestAddAllSkippedJSON(t *testing.T) {
	skipped := remote.AddResult{Skipped: []remote.AddFailed{{Source: "nothing.txt", Reason: "Not a video."}}}
	startFake(t, func(method string, params json.RawMessage) (any, error) {
		if method == remote.MethodAdd {
			return skipped, nil
		}
		return cannedApp(method, params)
	})
	for _, args := range [][]string{{"add", "nothing.txt", "--json"}, {"add", "nothing.txt", "--json", "--wait"}} {
		code, stdout, stderr := run(args...)
		if code != 1 || stderr != "kando add: skipped nothing.txt: Not a video.\n" {
			t.Errorf("%q = %d, %q", args, code, stderr)
		}
		assertSameJSON(t, args, stdout, skipped)
	}
}

// playlistApp lists a playlist of three videos, the second already in the library, and a channel it cannot read, and queues what it is given.
func playlistApp(method string, params json.RawMessage) (any, error) {
	switch method {
	case remote.MethodLists:
		p, err := remote.Params[remote.ListsParams](params)
		if err != nil {
			return nil, err
		}
		var lists []remote.List
		for _, source := range p.Sources {
			if source == "https://www.youtube.com/@gone" {
				lists = append(lists, remote.List{Source: source, Videos: []remote.ListVideo{}, Error: "This channel does not exist."})
				continue
			}
			lists = append(lists, remote.List{Source: source, Kind: "playlist", Title: "French in Action", Videos: []remote.ListVideo{
				{URL: "https://youtu.be/v1"}, {URL: "https://youtu.be/v2", InLibrary: true}, {URL: "https://youtu.be/v3"},
			}})
		}
		return lists, nil
	case remote.MethodAdd:
		p, err := remote.Params[remote.AddParams](params)
		if err != nil {
			return nil, err
		}
		res := remote.AddResult{Jobs: []remote.Job{}}
		for i, source := range p.Sources {
			res.Jobs = append(res.Jobs, remote.Job{ID: fmt.Sprintf("j%d", i+1), Source: source, Stage: "queued"})
		}
		return res, nil
	}
	return cannedApp(method, params)
}

// answerWith makes kando add's question read answer from a terminal, or, with terminal false, find no one to ask.
func answerWith(t *testing.T, terminal bool, answer string) {
	t.Helper()
	oldIn, oldTerminal := stdin, stdinTerminal
	stdin, stdinTerminal = strings.NewReader(answer), func() bool { return terminal }
	t.Cleanup(func() { stdin, stdinTerminal = oldIn, oldTerminal })
}

const testPlaylist = "https://www.youtube.com/playlist?list=PLx0sYbCqOb8"

// kando add puts a playlist's videos in its place, without those the library has, says what the list holds, and asks first unless --yes is given.
func TestAddPlaylist(t *testing.T) {
	f := startFake(t, playlistApp)
	answerWith(t, false, "")
	code, stdout, stderr := run("add", testPlaylist, "https://youtu.be/x", "--yes")
	if code != 0 || stdout != "j1\thttps://youtu.be/v1\nj2\thttps://youtu.be/v3\nj3\thttps://youtu.be/x\n" || stderr != "French in Action: 3 videos (playlist), 1 already in the library\n" {
		t.Fatalf("add --yes = %d, %q, %q", code, stdout, stderr)
	}
	var lists remote.ListsParams
	f.lastParams(t, remote.MethodLists, &lists)
	if !slices.Equal(lists.Sources, []string{testPlaylist}) {
		t.Errorf("listed %q, want only the playlist", lists.Sources)
	}

	answerWith(t, true, "y\n")
	if code, _, stderr := run("add", testPlaylist); code != 0 || !strings.HasSuffix(stderr, "Add 2 videos? [y/N] ") {
		t.Errorf("add answered y = %d, %q", code, stderr)
	}
	before := len(f.asked())
	answerWith(t, true, "\n")
	if code, _, stderr := run("add", testPlaylist); code != 1 || !strings.HasSuffix(stderr, "kando add: nothing added\n") {
		t.Errorf("add answered no = %d, %q", code, stderr)
	}
	for _, c := range f.asked()[before:] {
		if c.method == remote.MethodAdd {
			t.Error("declining added the videos")
		}
	}
}

// Without a terminal to ask on, kando add does not add a playlist's videos unless --yes says to; a list the app cannot read is skipped with why, and the others still added.
func TestAddPlaylistWithoutATerminal(t *testing.T) {
	f := startFake(t, playlistApp)
	answerWith(t, false, "")
	code, _, stderr := run("add", testPlaylist)
	if code != 2 || !strings.Contains(stderr, "pass --yes") {
		t.Errorf("add without a terminal = %d, %q", code, stderr)
	}
	for _, c := range f.asked() {
		if c.method == remote.MethodAdd {
			t.Error("added without asking")
		}
	}
	code, stdout, stderr := run("add", "https://www.youtube.com/@gone", "https://youtu.be/x", "--yes")
	if code != 1 || stdout != "j1\thttps://youtu.be/x\n" || stderr != "kando add: skipped https://www.youtube.com/@gone: This channel does not exist.\n" {
		t.Errorf("add a channel that is gone = %d, %q, %q", code, stdout, stderr)
	}
}

// waitApp queues two jobs and answers jobs.list from steps, one step a call, repeating the last.
func waitApp(steps [][]remote.Job) func(string, json.RawMessage) (any, error) {
	var mu sync.Mutex
	n := 0
	return func(method string, params json.RawMessage) (any, error) {
		switch method {
		case remote.MethodAdd:
			return remote.AddResult{Jobs: []remote.Job{
				{ID: "a", Source: "https://youtu.be/a", Stage: "queued"},
				{ID: "b", Source: "/tmp/b.mp4", Stage: "queued"},
			}}, nil
		case remote.MethodJobs:
			mu.Lock()
			defer mu.Unlock()
			step := steps[min(n, len(steps)-1)]
			n++
			// The app's other jobs are listed too, and are not waited for.
			return append([]remote.Job{{ID: "other", Source: "x", Stage: "downloading", Progress: 0.1}}, step...), nil
		}
		return nil, remote.Errorf(remote.CodeUnknownMethod, "%s", method)
	}
}

func fastPoll(t *testing.T) {
	old := pollInterval
	pollInterval = time.Millisecond
	t.Cleanup(func() { pollInterval = old })
}

func TestAddWaitComplete(t *testing.T) {
	fastPoll(t)
	startFake(t, waitApp([][]remote.Job{
		{{ID: "a", Source: "https://youtu.be/a", Stage: "downloading", Progress: 0.1}, {ID: "b", Source: "/tmp/b.mp4", Stage: "queued"}},
		{{ID: "a", Source: "https://youtu.be/a", Stage: "downloading", Progress: 0.1}, {ID: "b", Source: "/tmp/b.mp4", Stage: "queued"}},
		{{ID: "a", Source: "https://youtu.be/a", Title: "A", Stage: "transcribing", Progress: -1}, {ID: "b", Source: "/tmp/b.mp4", Stage: "queued"}},
		{{ID: "a", Source: "https://youtu.be/a", Title: "A", Stage: "complete", Progress: 1}, {ID: "b", Source: "/tmp/b.mp4", Stage: "translating", Progress: 0.5}},
		{{ID: "a", Source: "https://youtu.be/a", Title: "A", Stage: "complete", Progress: 1}, {ID: "b", Source: "/tmp/b.mp4", Stage: "complete", Progress: 1}},
	}))
	code, stdout, stderr := run("add", "https://youtu.be/a", "/tmp/b.mp4", "--wait")
	want := "a\thttps://youtu.be/a\n" +
		"b\t/tmp/b.mp4\n" +
		"a  downloading 10%  https://youtu.be/a\n" +
		"b  queued  /tmp/b.mp4\n" +
		"a  transcribing  A\n" +
		"a  complete  A\n" +
		"b  translating 50%  /tmp/b.mp4\n" +
		"b  complete  /tmp/b.mp4\n"
	if code != 0 || stdout != want || stderr != "" {
		t.Errorf("add --wait = %d\nstdout:\n%s\nwant:\n%s\nstderr: %q", code, stdout, want, stderr)
	}
}

func TestAddWaitFailed(t *testing.T) {
	fastPoll(t)
	startFake(t, waitApp([][]remote.Job{
		{{ID: "a", Source: "https://youtu.be/a", Stage: "failed", Error: "network down"}, {ID: "b", Source: "/tmp/b.mp4", Stage: "exporting", Progress: 0.99}},
		// b leaves the queue without ending, as a waiting job canceled in the window does.
		{{ID: "a", Source: "https://youtu.be/a", Stage: "failed", Error: "network down"}},
	}))
	code, stdout, stderr := run("add", "--wait", "https://youtu.be/a", "/tmp/b.mp4")
	want := "a\thttps://youtu.be/a\n" +
		"b\t/tmp/b.mp4\n" +
		"a  failed  https://youtu.be/a  network down\n" +
		"b  exporting 99%  /tmp/b.mp4\n" +
		"b  failed  /tmp/b.mp4  it is no longer in the queue\n"
	if code != 1 || stdout != want || stderr != "" {
		t.Errorf("add --wait = %d\nstdout:\n%s\nwant:\n%s\nstderr: %q", code, stdout, want, stderr)
	}
}

func TestAddWaitJSON(t *testing.T) {
	fastPoll(t)
	startFake(t, waitApp([][]remote.Job{
		{{ID: "a", Source: "https://youtu.be/a", Stage: "downloading", Progress: 0.3}, {ID: "b", Source: "/tmp/b.mp4", Stage: "failed", Canceled: true}},
		{{ID: "a", Source: "https://youtu.be/a", Stage: "complete", Progress: 1, Subtitles: "/lib/a.bilingual.srt"}, {ID: "b", Source: "/tmp/b.mp4", Stage: "failed", Canceled: true}},
	}))
	code, stdout, stderr := run("add", "https://youtu.be/a", "/tmp/b.mp4", "--wait", "--json")
	if code != 1 {
		t.Errorf("add --wait --json = %d, want 1 for the canceled job", code)
	}
	assertSameJSON(t, []string{"add --wait --json"}, stdout, remote.AddResult{Jobs: []remote.Job{
		{ID: "a", Source: "https://youtu.be/a", Stage: "complete", Progress: 1, Subtitles: "/lib/a.bilingual.srt"},
		{ID: "b", Source: "/tmp/b.mp4", Stage: "failed", Canceled: true},
	}})
	wantProgress := "a  downloading 30%  https://youtu.be/a\nb  canceled  /tmp/b.mp4\na  complete  https://youtu.be/a\n"
	if stderr != wantProgress {
		t.Errorf("progress on stderr = %q, want %q", stderr, wantProgress)
	}
}

func TestAddWaitInterrupted(t *testing.T) {
	fastPoll(t)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	polled := make(chan struct{}, 1)
	startFake(t, func(method string, params json.RawMessage) (any, error) {
		if method == remote.MethodJobs {
			select {
			case polled <- struct{}{}:
			default:
			}
		}
		return waitApp([][]remote.Job{{{ID: "a", Stage: "downloading", Progress: 0.1}, {ID: "b", Stage: "queued"}}})(method, params)
	})
	go func() {
		<-polled
		cancel()
	}()
	code, _, stderr := runCtx(ctx, "add", "a", "b", "--wait")
	if code != 1 || !strings.Contains(stderr, "kando add: interrupted") {
		t.Errorf("interrupted add --wait = %d, %q", code, stderr)
	}
}

// retryWaitApp queues job j3 again and answers jobs.list from steps, one step a call, repeating the last.
func retryWaitApp(steps [][]remote.Job) func(string, json.RawMessage) (any, error) {
	var mu sync.Mutex
	n := 0
	return func(method string, params json.RawMessage) (any, error) {
		switch method {
		case remote.MethodRetry:
			return cannedRetried, nil
		case remote.MethodJobs:
			mu.Lock()
			defer mu.Unlock()
			step := steps[min(n, len(steps)-1)]
			n++
			return step, nil
		}
		return nil, remote.Errorf(remote.CodeUnknownMethod, "%s", method)
	}
}

func TestRetryWait(t *testing.T) {
	fastPoll(t)
	startFake(t, retryWaitApp([][]remote.Job{
		{{ID: "j3", Source: "https://b23.tv/y", Title: "Cats", Stage: "downloading", Progress: 0.2}},
		{{ID: "j3", Source: "https://b23.tv/y", Title: "Cats", Stage: "complete", Progress: 1, Subtitles: "/lib/cats.bilingual.srt"}},
	}))
	code, stdout, stderr := run("retry", "j3", "--wait")
	want := "j3  downloading 20%  Cats\n" +
		"j3  complete  Cats\n"
	if code != 0 || stdout != want || stderr != "" {
		t.Errorf("retry --wait = %d\nstdout:\n%s\nwant:\n%s\nstderr: %q", code, stdout, want, stderr)
	}

	startFake(t, retryWaitApp([][]remote.Job{
		{{ID: "j3", Source: "https://b23.tv/y", Title: "Cats", Stage: "failed", Error: "network down"}},
	}))
	code, stdout, stderr = run("retry", "--wait", "--json", "j3")
	if code != 1 || stderr != "j3  failed  Cats  network down\n" {
		t.Errorf("retry --wait --json = %d, stderr %q, want 1 and the progress", code, stderr)
	}
	assertSameJSON(t, []string{"retry --wait --json"}, stdout, remote.Job{ID: "j3", Source: "https://b23.tv/y", Title: "Cats", Stage: "failed", Error: "network down"})
}

func TestStartsTheApp(t *testing.T) {
	path := shortSocket(t)
	old, oldTimeout := remote.Launch, remote.Timeout
	t.Cleanup(func() { remote.Launch, remote.Timeout = old, oldTimeout })
	remote.Timeout = 5 * time.Second
	launched := 0
	f := &fakeApp{handle: cannedApp}
	remote.Launch = func() error {
		launched++
		f.listen(t, path)
		return nil
	}
	code, stdout, stderr := run("version")
	if code != 0 || launched != 0 {
		t.Fatalf("version = %d, %q, launched %d times", code, stdout, launched)
	}
	code, stdout, stderr = run("jobs", "--json")
	if code != 0 || launched != 1 || stderr != "Starting Kando…\n" {
		t.Fatalf("jobs = %d, %q, %q, launched %d times", code, stdout, stderr, launched)
	}
	assertSameJSON(t, []string{"jobs --json"}, stdout, cannedJobs)
	// Now that it runs, the next command connects without starting it again or saying so.
	if code, _, stderr := run("status"); code != 0 || launched != 1 || stderr != "" {
		t.Errorf("status = %d, %q, launched %d times", code, stderr, launched)
	}
}

func TestCannotStartTheApp(t *testing.T) {
	shortSocket(t)
	old := remote.Launch
	t.Cleanup(func() { remote.Launch = old })
	remote.Launch = func() error { return errors.New("no app here") }
	code, stdout, stderr := run("status")
	if code != 1 || stdout != "" || stderr != "Starting Kando…\nkando status: start Kando: no app here\n" {
		t.Errorf("status = %d, %q, %q", code, stdout, stderr)
	}
}

// hostile has the escape sequences a title or a subtitle from a website could carry: OSC 0, which retitles the terminal window, a CSI that clears the screen, a C1 CSI, a bare byte that is not UTF-8 and a carriage return that would draw over the line.
const hostile = "\x1b]0;pwned\x07Evil\x1b[2J\u009b31m\x9b\rcat"

// tamed is hostile as text output prints it.
const tamed = "\ufffd]0;pwned\ufffdEvil\ufffd[2J\ufffd31m\ufffd\ufffdcat"

// tamedLine is hostile as a line of an .srt prints it, and tamedWords as the text format does.
const (
	tamedLine  = "\ufffd]0;pwned\ufffdEvil\ufffd[2J\ufffd31m\ufffdcat"
	tamedWords = "\ufffd]0;pwned\ufffdEvil\ufffd[2J\ufffd31m\ufffd cat"
)

func TestControlCharactersEscaped(t *testing.T) {
	fastPoll(t)
	startFake(t, func(method string, params json.RawMessage) (any, error) {
		switch method {
		case remote.MethodFilms:
			return []remote.Film{{ID: "f1", Title: hostile, Source: "youtube"}}, nil
		case remote.MethodFilm:
			return remote.Film{ID: "f1", Title: hostile, Source: "web\nsite"}, nil
		case remote.MethodJobs:
			return []remote.Job{{ID: "j1", Source: "x", Title: hostile, Stage: "failed", Error: hostile}}, nil
		case remote.MethodSubtitles:
			return []remote.Cue{{StartMS: 0, EndMS: 1000, Text: "line one\r\n" + hostile, Translation: "\t" + hostile}}, nil
		case remote.MethodAdd:
			return remote.AddResult{Jobs: []remote.Job{{ID: "j1", Source: hostile}}, Skipped: []remote.AddFailed{{Source: hostile, Reason: hostile}}}, nil
		case remote.MethodCancel:
			return nil, remote.Errorf(remote.CodeNotFound, "%s", hostile)
		}
		return cannedApp(method, params)
	})
	for _, tt := range []struct {
		args       []string
		stdout     string
		stderr     string
		wantStatus int
	}{
		{args: []string{"list"}, stdout: "ID  SOURCE   STAGE  TITLE\nf1  youtube  -      " + tamed + "\n"},
		{args: []string{"show", "f1"}, stdout: "id      f1\ntitle   " + tamed + "\nsource  web\ufffdsite\n"},
		{args: []string{"jobs"}, stdout: "j1  failed  " + tamed + "  " + tamed + "\n"},
		// The .srt keeps the cue's line breaks and drops the carriage returns; the text format puts each cue on one line, so they are spaces there.
		{args: []string{"subtitles", "f1"}, stdout: "1\n00:00:00,000 --> 00:00:01,000\n" + tamedLine + "\nline one\n" + tamedLine + "\n\n"},
		{args: []string{"subtitles", "f1", "--format", "text"}, stdout: "[00:00:00] " + tamedWords + " | line one " + tamedWords + "\n"},
		{args: []string{"add", "x"}, stdout: "j1\t" + tamed + "\n", stderr: "kando add: skipped " + tamed + ": " + tamed + "\n", wantStatus: 1},
		{args: []string{"add", "x", "--wait"}, stdout: "j1\t" + tamed + "\nj1  failed  " + tamed + "  " + tamed + "\n", stderr: "kando add: skipped " + tamed + ": " + tamed + "\n", wantStatus: 1},
		{args: []string{"cancel", "j1"}, stderr: "kando cancel: " + tamed + "\n", wantStatus: 4},
	} {
		code, stdout, stderr := run(tt.args...)
		if code != tt.wantStatus || stdout != tt.stdout || stderr != tt.stderr {
			t.Errorf("%q = %d\nstdout: %q\nwant:   %q\nstderr: %q\nwant:   %q", tt.args, code, stdout, tt.stdout, stderr, tt.stderr)
		}
		if strings.ContainsAny(stdout+stderr, "\x1b\x07\u009b\r") {
			t.Errorf("%q printed a control character: %q", tt.args, stdout+stderr)
		}
	}
	// --json keeps the text as it is, escaped by JSON.
	code, stdout, _ := run("show", "f1", "--json")
	if code != 0 || !strings.Contains(stdout, `"title": "\u001b]0;pwned\u0007Evil`) {
		t.Errorf("show --json = %d, %q", code, stdout)
	}
}

func TestNeedsConsole(t *testing.T) {
	all := stdio{in: true, out: true, err: true}
	for _, tt := range []struct {
		args []string
		have stdio
		want bool
	}{
		{[]string{"status"}, all, false},
		{[]string{"status"}, stdio{}, true},
		{[]string{"status"}, stdio{in: true, out: true}, true},
		{[]string{"jobs"}, stdio{out: true, err: true}, true},
		// Piped by an agent, mcp keeps its streams and borrows no console even without standard error.
		{[]string{"mcp"}, stdio{in: true, out: true}, false},
		{[]string{"mcp"}, all, false},
		{[]string{"mcp"}, stdio{}, true},
	} {
		if got := needsConsole(tt.args, tt.have); got != tt.want {
			t.Errorf("needsConsole(%q, %+v) = %v, want %v", tt.args, tt.have, got, tt.want)
		}
	}
}

// TestSearch checks that kando search sends the words and its flags, prints a row for each line found with when it shows, and says so when none is.
func TestSearch(t *testing.T) {
	found := []remote.Line{
		{FilmID: "f1", Title: "Rivers of Japan", StartMS: 1500, EndMS: 3000, Text: "Hello there.", Translation: "你好。"},
		{FilmID: "f2", Title: "Home video", StartMS: 3_725_250, EndMS: 3_727_000, Text: "Hello\nagain"},
	}
	f := startFake(t, func(method string, params json.RawMessage) (any, error) {
		if method != remote.MethodSearch {
			return cannedApp(method, params)
		}
		p, err := remote.Params[remote.SearchParams](params)
		if err != nil {
			return nil, err
		}
		if p.Query == "nothing" {
			return []remote.Line{}, nil
		}
		return found, nil
	})
	code, stdout, stderr := run("search", "hello", "there", "--film", "f1", "--until", "watched", "--limit", "5")
	want := "FILM  AT        TITLE            LINE\n" +
		"f1    00:00:01  Rivers of Japan  你好。 | Hello there.\n" +
		"f2    01:02:05  Home video       Hello again\n"
	if code != 0 || stdout != want || stderr != "" {
		t.Errorf("search = %d\nstdout:\n%s\nwant:\n%s\nstderr: %q", code, stdout, want, stderr)
	}
	var p remote.SearchParams
	f.lastParams(t, remote.MethodSearch, &p)
	if p != (remote.SearchParams{Query: "hello there", FilmID: "f1", Until: "watched", Limit: 5}) {
		t.Errorf("search params = %+v", p)
	}
	code, stdout, _ = run("search", "hello", "--json")
	if code != 0 {
		t.Errorf("search --json = %d", code)
	}
	assertSameJSON(t, []string{"search", "--json"}, stdout, found)
	if code, stdout, stderr := run("search", "nothing"); code != 0 || stdout != "" || stderr != "No lines found.\n" {
		t.Errorf("search nothing = %d, %q, %q", code, stdout, stderr)
	}
}

// TestSubtitlesRange checks that kando subtitles sends the part of the subtitles --from, --to and --until ask for, the times as a person writes them.
func TestSubtitlesRange(t *testing.T) {
	f := startFake(t, cannedApp)
	for _, tt := range []struct {
		args []string
		want remote.SubtitlesParams
	}{
		{[]string{"subtitles", "f1"}, remote.SubtitlesParams{ID: "f1"}},
		{[]string{"subtitles", "f1", "--from", "12:34", "--to", "1:02:03.5"}, remote.SubtitlesParams{ID: "f1", FromMS: 754_000, ToMS: 3_723_500}},
		{[]string{"subtitles", "--from=90", "f1", "--until", "watched"}, remote.SubtitlesParams{ID: "f1", FromMS: 90_000, Until: "watched"}},
	} {
		if code, _, stderr := run(tt.args...); code != 0 {
			t.Fatalf("%q = %d, %q", tt.args, code, stderr)
		}
		var p remote.SubtitlesParams
		f.lastParams(t, remote.MethodSubtitles, &p)
		if p != tt.want {
			t.Errorf("%q sent %+v, want %+v", tt.args, p, tt.want)
		}
	}
}

// TestOpenAt checks that kando open --at sends where to play the film from, and that a film the window cannot show yet still succeeds, saying why the library shows instead.
func TestOpenAt(t *testing.T) {
	f := startFake(t, func(method string, params json.RawMessage) (any, error) {
		if method != remote.MethodShow {
			return cannedApp(method, params)
		}
		p, err := remote.Params[remote.ShowParams](params)
		if err != nil {
			return nil, err
		}
		if p.ID == "queued" {
			return remote.ShowResult{Reason: remote.ShowInQueue}, nil
		}
		return remote.ShowResult{Shown: p.ID, AtMS: p.AtMS}, nil
	})
	if code, stdout, stderr := run("open", "f1", "--at", "1:05"); code != 0 || stdout != "" || stderr != "" {
		t.Errorf("open --at = %d, %q, %q", code, stdout, stderr)
	}
	var p remote.ShowParams
	f.lastParams(t, remote.MethodShow, &p)
	if p.ID != "f1" || p.AtMS == nil || *p.AtMS != 65_000 {
		t.Errorf("open --at sent %+v", p)
	}
	if code, _, _ := run("open", "f1"); code != 0 {
		t.Errorf("open = %d", code)
	}
	p = remote.ShowParams{}
	f.lastParams(t, remote.MethodShow, &p)
	if p.ID != "f1" || p.AtMS != nil {
		t.Errorf("open without --at sent %+v", p)
	}
	if code, stdout, stderr := run("open", "queued"); code != 0 || stdout != "" || stderr != "kando open: queued is still being processed; showing the library\n" {
		t.Errorf("open queued = %d, %q, %q", code, stdout, stderr)
	}
	code, stdout, _ := run("open", "f1", "--at", "3", "--json")
	at := int64(3000)
	if code != 0 {
		t.Errorf("open --json = %d", code)
	}
	assertSameJSON(t, []string{"open", "--json"}, stdout, remote.ShowResult{Shown: "f1", AtMS: &at})
}

// TestJobsOne checks that kando jobs with an id shows that job alone, from jobs.get.
func TestJobsOne(t *testing.T) {
	f := startFake(t, func(method string, params json.RawMessage) (any, error) {
		if method == remote.MethodJob {
			return cannedJobs[0], nil
		}
		return cannedApp(method, params)
	})
	if code, stdout, stderr := run("jobs", "j1"); code != 0 || stdout != "j1  downloading 50%  Rivers of Japan\n" || stderr != "" {
		t.Errorf("jobs j1 = %d, %q, %q", code, stdout, stderr)
	}
	var p remote.IDParams
	f.lastParams(t, remote.MethodJob, &p)
	if p.ID != "j1" {
		t.Errorf("jobs j1 sent %+v", p)
	}
}

// TestAddDuplicates checks that a video the library already has counts as added: its job is printed and waited for, and the command succeeds, so running it again does; one whose job failed still fails it.
func TestAddDuplicates(t *testing.T) {
	fastPoll(t)
	startFake(t, func(method string, params json.RawMessage) (any, error) {
		switch method {
		case remote.MethodAdd:
			p, err := remote.Params[remote.AddParams](params)
			if err != nil {
				return nil, err
			}
			res := remote.AddResult{Jobs: []remote.Job{}}
			for _, source := range p.Sources {
				switch source {
				case "done":
					res.Skipped = append(res.Skipped, remote.AddFailed{Source: source, Reason: "Already in the library.", JobID: "jd", Stage: "complete", FilmID: "jd"})
				case "running":
					res.Skipped = append(res.Skipped, remote.AddFailed{Source: source, Reason: "Already in the queue.", JobID: "jr", Stage: "downloading"})
				case "broken":
					res.Skipped = append(res.Skipped, remote.AddFailed{Source: source, Reason: "No language.", JobID: "jb", Stage: "failed"})
				}
			}
			return res, nil
		case remote.MethodJobs:
			return []remote.Job{{ID: "jd", Source: "done", Stage: "complete", Progress: 1, FilmID: "jd"}, {ID: "jr", Source: "running", Stage: "complete", Progress: 1, FilmID: "jr"}}, nil
		}
		return cannedApp(method, params)
	})
	code, stdout, stderr := run("add", "done", "running")
	if code != 0 || stdout != "jd\tdone\njr\trunning\n" || stderr != "kando add: done: Already in the library.\nkando add: running: Already in the queue.\n" {
		t.Errorf("add duplicates = %d, %q, %q", code, stdout, stderr)
	}
	code, stdout, _ = run("add", "done", "running", "--wait", "--json")
	if code != 0 {
		t.Errorf("add duplicates --wait = %d", code)
	}
	assertSameJSON(t, []string{"add", "--wait", "--json"}, stdout, remote.AddResult{Jobs: []remote.Job{}, Skipped: []remote.AddFailed{
		{Source: "done", Reason: "Already in the library.", JobID: "jd", Stage: "complete", FilmID: "jd"},
		{Source: "running", Reason: "Already in the queue.", JobID: "jr", Stage: "complete", FilmID: "jr"},
	}})
	if code, _, stderr := run("add", "broken"); code != 1 || stderr != "kando add: skipped broken: No language.\n" {
		t.Errorf("add a duplicate that failed = %d, %q", code, stderr)
	}
}

// TestStatusLibraries checks that kando status lists the libraries the app knows, the open one marked, a locked one without its count.
func TestStatusLibraries(t *testing.T) {
	status := cannedStatus
	status.Libraries = []remote.Library{
		{Path: "/Users/me/Movies/Kando", Name: "Kando", Open: true, Films: 12},
		{Path: "/Volumes/Disk/Lectures.kando", Name: "Lectures", Films: 1},
		{Path: "/Volumes/Gone/Old.kando", Name: "Old", Missing: true},
		{Path: "/Users/me/Movies/Private.kando", Name: "Private", Locked: true},
	}
	startFake(t, func(method string, params json.RawMessage) (any, error) {
		if method == remote.MethodStatus {
			return status, nil
		}
		return cannedApp(method, params)
	})
	_, stdout, _ := run("status")
	for _, want := range []string{
		"libraries          * Kando  12 films  /Users/me/Movies/Kando\n",
		"                     Lectures  1 film  /Volumes/Disk/Lectures.kando\n",
		"                     Old  missing  /Volumes/Gone/Old.kando\n",
		"                     Private  locked  /Users/me/Movies/Private.kando\n",
	} {
		if !strings.Contains(stdout, want) {
			t.Errorf("status = %q, want %q", stdout, want)
		}
	}
}

// TestNotRunningExitCode checks that a command whose app did not open its socket in time exits with 5, and with --json says not_running.
func TestNotRunningExitCode(t *testing.T) {
	shortSocket(t)
	old, oldTimeout := remote.Launch, remote.Timeout
	t.Cleanup(func() { remote.Launch, remote.Timeout = old, oldTimeout })
	remote.Launch = func() error { return nil }
	remote.Timeout = 10 * time.Millisecond
	code, stdout, _ := run("status", "--json")
	if code != 5 || !strings.Contains(stdout, `"code": "not_running"`) {
		t.Errorf("status = %d, %q", code, stdout)
	}
}

// A job that completes without translating its film did not do what kando add --wait asked: its line says so and why, and the command fails.
func TestAddWaitNotTranslated(t *testing.T) {
	fastPoll(t)
	startFake(t, waitApp([][]remote.Job{
		{{ID: "a", Source: "https://youtu.be/a", Title: "A", Stage: "complete", Progress: 1, TranslationError: "The AI service doesn't accept this key."}, {ID: "b", Source: "/tmp/b.mp4", Stage: "complete", Progress: 1}},
	}))
	code, stdout, _ := run("add", "https://youtu.be/a", "/tmp/b.mp4", "--wait")
	want := "a\thttps://youtu.be/a\n" +
		"b\t/tmp/b.mp4\n" +
		"a  complete, not translated  A  The AI service doesn't accept this key.\n" +
		"b  complete  /tmp/b.mp4\n"
	if code != 1 || stdout != want {
		t.Errorf("add --wait = %d\nstdout:\n%s\nwant:\n%s", code, stdout, want)
	}
}

// kando add says the app's note on what it queued, as that the subtitles will not be translated.
func TestAddNote(t *testing.T) {
	startFake(t, func(method string, params json.RawMessage) (any, error) {
		if method == remote.MethodAdd {
			return remote.AddResult{Jobs: []remote.Job{{ID: "j9", Source: "https://youtu.be/new", Stage: "queued"}}, Note: "Added. Without an AI service, subtitles will be in the original language only."}, nil
		}
		return cannedApp(method, params)
	})
	code, stdout, stderr := run("add", "https://youtu.be/new")
	if code != 0 || stdout != "j9\thttps://youtu.be/new\n" || stderr != "kando add: Added. Without an AI service, subtitles will be in the original language only.\n" {
		t.Errorf("add = %d, %q, %q", code, stdout, stderr)
	}
}

// kando translate asks the app to translate a film's missing lines and prints its job; with --wait it waits for it as retry --wait does.
func TestTranslate(t *testing.T) {
	fastPoll(t)
	f := startFake(t, func(method string, params json.RawMessage) (any, error) {
		switch method {
		case remote.MethodTranslate:
			return remote.Job{ID: "j1", Source: "https://youtu.be/x", Title: "Rivers of Japan", Stage: "queued"}, nil
		case remote.MethodJobs:
			return []remote.Job{{ID: "j1", Source: "https://youtu.be/x", Title: "Rivers of Japan", Stage: "complete", Progress: 1}}, nil
		}
		return cannedApp(method, params)
	})
	code, stdout, _ := run("translate", "f1")
	if code != 0 || stdout != "j1  queued  Rivers of Japan\n" {
		t.Errorf("translate = %d, %q", code, stdout)
	}
	var p remote.IDParams
	f.lastParams(t, remote.MethodTranslate, &p)
	if p.ID != "f1" {
		t.Errorf("translated %q", p.ID)
	}
	if code, stdout, _ := run("translate", "f1", "--wait"); code != 0 || stdout != "j1  complete  Rivers of Japan\n" {
		t.Errorf("translate --wait = %d, %q", code, stdout)
	}
	if code, _, stderr := run("translate"); code != 2 || !strings.Contains(stderr, "needs a film id") {
		t.Errorf("translate without an id = %d, %q", code, stderr)
	}
}

// A job jobs.list no longer lists, as one making the subtitles of a video of the library's folder that failed, is asked for alone, and ends as the app says it did.
func TestWaitAsksForAJobTheListLeftOut(t *testing.T) {
	fastPoll(t)
	wait := waitApp([][]remote.Job{{{ID: "a", Source: "https://youtu.be/a", Stage: "complete", Progress: 1}}})
	startFake(t, func(method string, params json.RawMessage) (any, error) {
		if method == remote.MethodJob {
			return remote.Job{ID: "b", Source: "/tmp/b.mp4", Stage: "failed", Error: "transcribe: no speech", FilmID: "/lib/b.mp4"}, nil
		}
		return wait(method, params)
	})
	code, stdout, _ := run("add", "https://youtu.be/a", "/tmp/b.mp4", "--wait")
	if code != 1 || !strings.HasSuffix(stdout, "b  failed  /tmp/b.mp4  transcribe: no speech\n") {
		t.Errorf("add --wait = %d, %q", code, stdout)
	}
}

// kando add lists a folder's videos through the app, by its absolute path, says what it holds, and adds them once asked, as it does a playlist's.
func TestAddFolder(t *testing.T) {
	dir := t.TempDir()
	if err := os.Mkdir(filepath.Join(dir, "Course"), 0o755); err != nil {
		t.Fatal(err)
	}
	t.Chdir(dir)
	f := startFake(t, func(method string, params json.RawMessage) (any, error) {
		if method == remote.MethodLists {
			p, err := remote.Params[remote.ListsParams](params)
			if err != nil {
				return nil, err
			}
			return []remote.List{{Source: p.Sources[0], Kind: remote.ListFolder, Title: "Course", NotVideos: 1, Videos: []remote.ListVideo{
				{URL: "file:///c/1.mp4"}, {URL: "file:///c/2.mp4", InLibrary: true}, {URL: "file:///c/3.mp4"},
			}}}, nil
		}
		return playlistApp(method, params)
	})
	answerWith(t, true, "y\n")
	code, stdout, stderr := run("add", "Course")
	if code != 0 || stdout != "j1\tfile:///c/1.mp4\nj2\tfile:///c/3.mp4\n" || stderr != "Course: 3 videos (folder), 1 file that is not a video left out, 1 already in the library\nAdd 2 videos? [y/N] " {
		t.Fatalf("add a folder = %d, %q, %q", code, stdout, stderr)
	}
	var lists remote.ListsParams
	f.lastParams(t, remote.MethodLists, &lists)
	want, _ := filepath.Abs("Course")
	if !slices.Equal(lists.Sources, []string{want}) {
		t.Errorf("listed %q, want %q", lists.Sources, want)
	}
}

// kando list --language asks for the films heard in a language, and kando show says the language heard in a film and how its translation went.
func TestFilmLanguages(t *testing.T) {
	film := cannedFilm
	film.SpokenLanguage, film.SpeechLanguage, film.Language = "ja", "ja", ""
	film.TranslationError, film.Untranslated = "Choose a model in “AI Service” first", 12
	f := startFake(t, func(method string, params json.RawMessage) (any, error) {
		if method == remote.MethodFilm {
			return film, nil
		}
		return cannedApp(method, params)
	})
	if code, _, _ := run("list", "--language", "ja"); code != 0 {
		t.Errorf("list --language = %d", code)
	}
	var p remote.FilmsParams
	f.lastParams(t, remote.MethodFilms, &p)
	if p.Spoken != "ja" {
		t.Errorf("films params %+v", p)
	}
	code, stdout, _ := run("show", "f1")
	for _, want := range []string{"not translated      Choose a model in “AI Service” first\n", "untranslated lines  12\n", "spoken language     ja\n", "speech language     ja\n"} {
		if code != 0 || !strings.Contains(stdout, want) {
			t.Errorf("show = %d, %q, want it to hold %q", code, stdout, want)
		}
	}
	if strings.Contains(stdout, "\nlanguage ") {
		t.Errorf("show names a language for a film that was not translated: %q", stdout)
	}
}
