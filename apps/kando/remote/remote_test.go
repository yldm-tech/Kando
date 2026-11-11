package remote

import (
	"context"
	"encoding/json"
	"errors"
	"net"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"testing"
	"time"
)

// shortSocket is a socket path short enough for macOS's 104-byte limit, which t.TempDir's can pass.
func shortSocket(t *testing.T) string {
	t.Helper()
	dir, err := os.MkdirTemp("", "kr")
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { os.RemoveAll(dir) })
	return filepath.Join(dir, "k.sock")
}

func serveEcho(t *testing.T, path string) *Server {
	t.Helper()
	s, err := Listen(path, func(method string, params json.RawMessage) (any, error) {
		switch method {
		case MethodFilm:
			p, err := Params[IDParams](params)
			if err != nil {
				return nil, err
			}
			if p.ID != "f1" {
				return nil, Errorf(CodeNotFound, "no film %s", p.ID)
			}
			return Film{ID: "f1", Title: "Rivers"}, nil
		case MethodStatus:
			return nil, errors.New("disk on fire")
		}
		return nil, Errorf(CodeUnknownMethod, "%s", method)
	})
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { s.Close() })
	return s
}

func TestCallRoundTrip(t *testing.T) {
	path := shortSocket(t)
	t.Setenv("KANDO_SOCKET", path)
	serveEcho(t, path)
	c, err := Dial(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	var film Film
	if err := c.Call(context.Background(), MethodFilm, IDParams{ID: "f1"}, &film); err != nil || film.Title != "Rivers" {
		t.Fatalf("film = %+v, %v", film, err)
	}
	if err := c.Call(context.Background(), MethodFilm, IDParams{ID: "nope"}, &film); !IsCode(err, CodeNotFound) {
		t.Fatalf("err = %v, want not_found", err)
	}
	if err := c.Call(context.Background(), MethodStatus, nil, nil); !IsCode(err, CodeFailed) {
		t.Fatalf("err = %v, want failed", err)
	}
	if err := c.Call(context.Background(), "bogus", nil, nil); !IsCode(err, CodeUnknownMethod) {
		t.Fatalf("err = %v, want unknown_method", err)
	}
	if err := c.Call(context.Background(), MethodFilm, json.RawMessage(`[1]`), nil); !IsCode(err, CodeInvalid) {
		t.Fatalf("err = %v, want invalid_request", err)
	}
}

func TestParseError(t *testing.T) {
	path := shortSocket(t)
	serveEcho(t, path)
	conn, err := net.Dial("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	defer conn.Close()
	if _, err := conn.Write([]byte("not json\n")); err != nil {
		t.Fatal(err)
	}
	var resp Response
	if err := json.NewDecoder(conn).Decode(&resp); err != nil || resp.Error == nil || resp.Error.Code != CodeParse {
		t.Fatalf("resp = %+v, %v", resp, err)
	}
}

func TestListenReplacesStaleSocketButNotLiveOne(t *testing.T) {
	path := shortSocket(t)
	stale, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	stale.(*net.UnixListener).SetUnlinkOnClose(false)
	stale.Close()
	s := serveEcho(t, path)
	info, err := os.Lstat(path)
	if err != nil || info.Mode().Type() != os.ModeSocket {
		t.Fatalf("socket %v, %v", info, err)
	}
	// Windows has no permission bits, which Go reports as 0666: there the socket's folder's access list keeps it the user's (privateFolder).
	if runtime.GOOS != "windows" && info.Mode().Perm() != 0o600 {
		t.Fatalf("socket mode %v", info.Mode())
	}
	if _, err := Listen(path, nil); !errors.Is(err, ErrInUse) {
		t.Fatalf("second Listen err = %v, want ErrInUse", err)
	}
	s.Close()
	if _, err := os.Stat(path); !os.IsNotExist(err) {
		t.Fatalf("Close left the socket: %v", err)
	}
}

func TestDialLaunchesTheApp(t *testing.T) {
	path := shortSocket(t)
	t.Setenv("KANDO_SOCKET", path)
	if _, err := Dial(context.Background(), false); !errors.Is(err, ErrNotRunning) {
		t.Fatalf("err = %v, want ErrNotRunning", err)
	}
	saved := Launch
	t.Cleanup(func() { Launch = saved })
	launched := false
	Launch = func() error {
		launched = true
		serveEcho(t, path)
		return nil
	}
	c, err := Dial(context.Background(), true)
	if err != nil || !launched {
		t.Fatalf("Dial = %v, launched %v", err, launched)
	}
	c.Close()
}

func TestOnUIWaitsForTheAnswer(t *testing.T) {
	post := func(fn func()) { go fn() }
	v, err := OnUI(context.Background(), post, func() (int, error) { return 7, nil })
	if v != 7 || err != nil {
		t.Fatalf("OnUI = %d, %v", v, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := OnUI(ctx, func(func()) {}, func() (int, error) { return 0, nil }); !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v", err)
	}
}

func TestListenLeavesAFileThatIsNotASocket(t *testing.T) {
	path := shortSocket(t)
	if err := os.WriteFile(path, []byte("notes"), 0o600); err != nil {
		t.Fatal(err)
	}
	if _, err := Listen(path, nil); err == nil || !strings.Contains(err.Error(), path) {
		t.Fatalf("Listen err = %v, want one naming %s", err, path)
	}
	if data, err := os.ReadFile(path); err != nil || string(data) != "notes" {
		t.Fatalf("the file is now %q, %v", data, err)
	}
}

func TestListenRefusesAFolderOthersCanWrite(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("folder modes are Unix's")
	}
	path := shortSocket(t)
	dir := filepath.Dir(path)
	if err := os.Chmod(dir, 0o777); err != nil {
		t.Fatal(err)
	}
	if _, err := Listen(path, nil); err == nil || !strings.Contains(err.Error(), dir) {
		t.Fatalf("Listen in a world-writable folder err = %v", err)
	}
	if err := os.Chmod(dir, 0o700); err != nil {
		t.Fatal(err)
	}
	serveEcho(t, path)
}

func TestSocketPathWithoutAConfigDirectory(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("Windows takes the config directory from AppData")
	}
	t.Setenv("KANDO_SOCKET", "")
	t.Setenv("HOME", "")
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv("XDG_RUNTIME_DIR", "/run/user/1000")
	if got := SocketPath(); got != "/run/user/1000/kando.sock" {
		t.Fatalf("SocketPath = %q, want the runtime folder's", got)
	}
	t.Setenv("XDG_RUNTIME_DIR", "")
	if got, want := SocketPath(), filepath.Join(os.TempDir(), "Kando", "kando.sock"); got != want {
		t.Fatalf("SocketPath = %q, want %q", got, want)
	}
}

// serveLate answers each request line with its ID after delay, as an app busy past the client's deadline would.
func serveLate(t *testing.T, path string, delay time.Duration) {
	t.Helper()
	ln, err := net.Listen("unix", path)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { ln.Close() })
	go func() {
		for {
			conn, err := ln.Accept()
			if err != nil {
				return
			}
			go func() {
				defer conn.Close()
				dec := json.NewDecoder(conn)
				enc := json.NewEncoder(conn)
				for {
					var req Request
					if dec.Decode(&req) != nil {
						return
					}
					time.Sleep(delay)
					if enc.Encode(Response{ID: req.ID, Result: json.RawMessage(`{}`)}) != nil {
						return
					}
				}
			}()
		}
	}()
}

func TestCallAfterATimeoutFailsFastInsteadOfReadingTheLateAnswer(t *testing.T) {
	path := shortSocket(t)
	t.Setenv("KANDO_SOCKET", path)
	serveLate(t, path, 300*time.Millisecond)
	c, err := Dial(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	defer c.Close()
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	if err := c.Call(ctx, MethodStatus, nil, nil); err == nil || errors.Is(err, ErrBroken) {
		t.Fatalf("first call err = %v, want its timeout", err)
	}
	time.Sleep(400 * time.Millisecond) // the late answer has been written by now
	for range 2 {
		start := time.Now()
		err := c.Call(context.Background(), MethodStatus, nil, nil)
		if !errors.Is(err, ErrBroken) {
			t.Fatalf("later call err = %v, want ErrBroken", err)
		}
		if time.Since(start) > 100*time.Millisecond {
			t.Fatalf("later call took %s, want it to fail at once", time.Since(start))
		}
	}
}

func TestCallAfterCloseFailsWithErrBroken(t *testing.T) {
	path := shortSocket(t)
	t.Setenv("KANDO_SOCKET", path)
	serveEcho(t, path)
	c, err := Dial(context.Background(), false)
	if err != nil {
		t.Fatal(err)
	}
	c.Close()
	if err := c.Call(context.Background(), MethodFilm, IDParams{ID: "f1"}, nil); !errors.Is(err, ErrBroken) {
		t.Fatalf("err = %v, want ErrBroken", err)
	}
}

func TestOnUISkipsFnWhenCtxEndedFirst(t *testing.T) {
	posted := make(chan func(), 1)
	ctx, cancel := context.WithCancel(context.Background())
	ran := false
	done := make(chan error, 1)
	go func() {
		_, err := OnUI(ctx, func(fn func()) { posted <- fn }, func() (int, error) {
			ran = true
			return 1, nil
		})
		done <- err
	}()
	fn := <-posted
	cancel()
	if err := <-done; !errors.Is(err, context.Canceled) {
		t.Fatalf("err = %v, want context.Canceled", err)
	}
	fn() // the UI thread comes to it after the caller gave up
	if ran {
		t.Fatal("fn ran after OnUI answered that ctx ended")
	}
}

func TestOnUIWaitsForFnThatStartedBeforeCtxEnded(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	started := make(chan struct{})
	release := make(chan struct{})
	post := func(fn func()) { go fn() }
	done := make(chan error, 1)
	var v int
	go func() {
		var err error
		v, err = OnUI(ctx, post, func() (int, error) {
			close(started)
			<-release
			return 7, nil
		})
		done <- err
	}()
	<-started
	cancel()
	close(release)
	if err := <-done; err != nil || v != 7 {
		t.Fatalf("OnUI = %d, %v, want fn's answer", v, err)
	}
}

func TestLaunchCommand(t *testing.T) {
	bundle := "/Applications/Kando.app"
	exe := bundle + "/Contents/MacOS/Kando"
	for _, tc := range []struct {
		goos, exe string
		ownSocket bool
		name      string
		args      []string
	}{
		{"darwin", exe, false, "open", []string{"-g", bundle}},
		{"darwin", exe, true, "open", []string{"-g", "-n", bundle}},
		{"darwin", "/usr/local/bin/kando", true, "/usr/local/bin/kando", nil},
		{"linux", "/opt/kando/kando", true, "/opt/kando/kando", nil},
		{"windows", `C:\Kando\Kando.exe`, false, `C:\Kando\Kando.exe`, nil},
	} {
		name, args := launchCommand(tc.goos, tc.exe, tc.ownSocket)
		if name != tc.name || !slices.Equal(args, tc.args) {
			t.Errorf("launchCommand(%s, %s, %v) = %s %q, want %s %q", tc.goos, tc.exe, tc.ownSocket, name, args, tc.name, tc.args)
		}
	}
}
