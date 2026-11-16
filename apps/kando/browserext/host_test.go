package browserext

import (
	"context"
	"encoding/json"
	"io"
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"kando/apps/kando/remote"
)

// The executable serves as the host only when a browser starts it so: Chrome with the extension's origin, and on Windows the handle of its window; Firefox with the manifest's path and the extension's ID. A command of the kando command line, a link or a file to open are not the host's.
func TestIsHostReadsTheBrowsersArguments(t *testing.T) {
	origin := ChromeOrigin()
	for _, c := range []struct {
		args []string
		want bool
	}{
		{[]string{origin}, true},
		{[]string{origin, "--parent-window=0"}, true},
		{[]string{origin, "--parent-window=1234"}, true},
		{[]string{"/Users/me/Library/Application Support/Mozilla/NativeMessagingHosts/ai.yldm.kando.json", GeckoID}, true},
		{[]string{`C:\Users\me\AppData\Roaming\Kando\NativeMessagingHosts\firefox\ai.yldm.kando.json`, GeckoID}, true},

		{nil, false},
		{[]string{"status"}, false},
		{[]string{"add", origin}, false},
		{[]string{"https://www.youtube.com/watch?v=jNQXAC9IVRw"}, false},
		{[]string{"/Users/me/Movies/talk.mp4"}, false},
		{[]string{origin, "--parent-window=0", "more"}, false},
		{[]string{origin, "status"}, false},
		{[]string{"/tmp/other.json", GeckoID}, false},
		{[]string{"/tmp/ai.yldm.kando.json", "other@example.com"}, false},
		{[]string{"/tmp/ai.yldm.kando.json"}, false},
	} {
		if got := IsHost(c.args); got != c.want {
			t.Errorf("IsHost(%q) = %v, want %v", c.args, got, c.want)
		}
	}
}

// The extension's ID in Chromium is the one Chromium derives from the manifest's key, which the host's manifest allows.
func TestChromeIDFollowsTheKey(t *testing.T) {
	if got := ChromeID(); got != "cnbehdpnehadddahobnficmaicoaeggn" {
		t.Fatalf("ChromeID = %q", got)
	}
	if ChromeOrigin() != "chrome-extension://cnbehdpnehadddahobnficmaicoaeggn/" {
		t.Fatalf("ChromeOrigin = %q", ChromeOrigin())
	}
}

// fakeApp is the app's socket for the host's tests: it answers the browser methods with what it was asked, and counts the calls.
type fakeApp struct {
	mu     sync.Mutex
	server *remote.Server
	calls  []string
}

func (f *fakeApp) listen(t *testing.T, path string) {
	t.Helper()
	server, err := remote.Listen(path, func(method string, params json.RawMessage) (any, error) {
		f.mu.Lock()
		f.calls = append(f.calls, method+" "+string(params))
		f.mu.Unlock()
		if method == remote.MethodBrowserAdd {
			return nil, remote.Errorf(remote.CodeNotReady, "no library")
		}
		return map[string]any{"method": method, "params": params}, nil
	})
	if err != nil {
		t.Fatal(err)
	}
	f.mu.Lock()
	f.server = server
	f.mu.Unlock()
}

func (f *fakeApp) close() {
	f.mu.Lock()
	defer f.mu.Unlock()
	if f.server != nil {
		f.server.Close()
		f.server = nil
	}
}

// hostSession runs Serve on pipes and sends it requests, as the browser does.
type hostSession struct {
	in   *io.PipeWriter
	out  *io.PipeReader
	done chan error
}

func startHost(t *testing.T) *hostSession {
	t.Helper()
	inR, inW := io.Pipe()
	outR, outW := io.Pipe()
	s := &hostSession{in: inW, out: outR, done: make(chan error, 1)}
	go func() {
		s.done <- Serve(context.Background(), inR, outW, DialApp)
		outW.Close()
	}()
	t.Cleanup(func() { inW.Close() })
	return s
}

func (s *hostSession) send(t *testing.T, raw string) answer {
	t.Helper()
	if err := WriteMessage(s.in, []byte(raw)); err != nil {
		t.Fatal(err)
	}
	data, err := ReadMessage(s.out)
	if err != nil {
		t.Fatal(err)
	}
	var a answer
	if err := json.Unmarshal(data, &a); err != nil {
		t.Fatalf("answer %q: %v", data, err)
	}
	return a
}

// The host relays the extension's calls to the app's socket and their answers back by ID: asking about a page's videos does not start an app that is not running, while adding one does, as the command line does; a call the extension has no business making, or that is not JSON, is answered with an error and the host goes on. Once the app quits, the host connects again to the one that starts next.
func TestHostRelaysToTheApp(t *testing.T) {
	dir, err := os.MkdirTemp("", "k")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	sock := filepath.Join(dir, "k.sock")
	t.Setenv("KANDO_SOCKET", sock)
	saved, savedTimeout := remote.Launch, remote.Timeout
	t.Cleanup(func() { remote.Launch, remote.Timeout = saved, savedTimeout })
	remote.Timeout = 5 * time.Second
	app := &fakeApp{}
	t.Cleanup(app.close)
	launched := 0
	remote.Launch = func() error {
		launched++
		app.listen(t, sock)
		return nil
	}

	host := startHost(t)
	got := host.send(t, `{"id":1,"method":"browser.state","params":{"urls":["https://youtu.be/jNQXAC9IVRw"]}}`)
	if got.ID != 1 || got.Error == nil || got.Error.Code != CodeNotRunning || launched != 0 {
		t.Fatalf("asking without the app: %+v, launched %d", got, launched)
	}
	got = host.send(t, `{"id":2,"method":"browser.add","params":{"url":"https://youtu.be/jNQXAC9IVRw","title":"Me at the zoo"}}`)
	if got.ID != 2 || got.Error == nil || got.Error.Code != remote.CodeNotReady || got.Error.Message != "no library" || launched != 1 {
		t.Fatalf("adding starts the app and relays its error: %+v, launched %d", got, launched)
	}
	got = host.send(t, `{"id":3,"method":"browser.state","params":{"urls":["https://youtu.be/jNQXAC9IVRw"]}}`)
	var echoed struct {
		Method string          `json:"method"`
		Params json.RawMessage `json:"params"`
	}
	if got.ID != 3 || got.Error != nil || json.Unmarshal(got.Result, &echoed) != nil || echoed.Method != remote.MethodBrowserState || string(echoed.Params) != `{"urls":["https://youtu.be/jNQXAC9IVRw"]}` {
		t.Fatalf("asking with the app: %+v %s", got, got.Result)
	}
	got = host.send(t, `{"id":4,"method":"jobs.remove","params":{"id":"x"}}`)
	if got.ID != 4 || got.Error == nil || got.Error.Code != remote.CodeUnknownMethod {
		t.Fatalf("a method that is not the extension's: %+v", got)
	}
	got = host.send(t, `not json`)
	if got.Error == nil || got.Error.Code != remote.CodeParse {
		t.Fatalf("a message that is not JSON: %+v", got)
	}
	if len(app.calls) != 2 {
		t.Fatalf("the app heard %q", app.calls)
	}

	app.close()
	got = host.send(t, `{"id":5,"method":"browser.state","params":{"urls":[]}}`)
	if got.ID != 5 || got.Error == nil || got.Error.Code != CodeNotRunning {
		t.Fatalf("asking once the app quit: %+v", got)
	}
	app.listen(t, sock)
	got = host.send(t, `{"id":6,"method":"browser.show","params":{"url":"https://youtu.be/jNQXAC9IVRw"}}`)
	if got.ID != 6 || got.Error != nil || launched != 1 {
		t.Fatalf("showing with the app started again: %+v, launched %d", got, launched)
	}

	host.in.Close()
	if err := <-host.done; err != nil {
		t.Fatalf("Serve ended with %v once the browser closed its input", err)
	}
}
