package browserext

import (
	"context"
	"encoding/json"
	"errors"
	"io"
	"os"
	"strings"

	"kando/apps/kando/remote"
)

// HostName is the name the extension connects to (runtime.connectNative) and the host's manifests are registered by: Kando's app ID, ai.yldm.kando, which keeps to the characters both browsers take in a host's name, lower-case letters, digits, underscores and dots.
const HostName = "ai.yldm.kando"

// GeckoID is the extension's ID in Firefox (manifest.json's browser_specific_settings.gecko.id), which the Firefox host manifest allows (allowed_extensions) and Firefox passes the host.
const GeckoID = "kando@yldm.ai"

// CodeNotRunning is the code of the host's answer when the app is not running and the request does not start it (MethodBrowserState), as the kando command line names it.
const CodeNotRunning = "not_running"

// IsHost reports whether args, the arguments the executable was started with after its name, are those a browser starts a native messaging host with, so the executable serves as one (Main) rather than opening its window or running a command:
//   - Chrome and the browsers built on Chromium pass the origin of the extension that connected, chrome-extension://<id>/, and on Windows also --parent-window=<handle> (https://developer.chrome.com/docs/extensions/develop/concepts/native-messaging#native-messaging-host-protocol);
//   - Firefox passes the path of the host's manifest, the file HostName.json, and the ID of the extension (https://developer.mozilla.org/en-US/docs/Mozilla/Add-ons/WebExtensions/Native_messaging#app_side).
func IsHost(args []string) bool {
	switch {
	case len(args) == 0:
		return false
	case strings.HasPrefix(args[0], "chrome-extension://"):
		for _, arg := range args[1:] {
			if !strings.HasPrefix(arg, "--parent-window=") {
				return false
			}
		}
		return len(args) <= 2
	}
	// The manifest's name is read after either separator, as Windows writes the path with backslashes.
	return len(args) == 2 && args[1] == GeckoID && args[0][strings.LastIndexAny(args[0], `/\`)+1:] == HostName+".json"
}

// Main serves the extension on the standard input and output until the browser closes them, and returns the exit code. It opens no window: it only reads and writes messages and talks to the app's socket. It leaves the browser's signals as they are, so the browser's SIGTERM ends it when the extension disconnects.
func Main() int {
	if err := Serve(context.Background(), os.Stdin, os.Stdout, DialApp); err != nil {
		return 1
	}
	return 0
}

// Caller is a connection to the app: remote.Client, or a fake in tests.
type Caller interface {
	Call(ctx context.Context, method string, params, out any) error
	Closed() bool
	Close() error
}

// Dialer connects to the app, starting it when it is not running and launch is set (remote.Dial).
type Dialer func(ctx context.Context, launch bool) (Caller, error)

// DialApp is the Dialer of the app at remote.SocketPath.
func DialApp(ctx context.Context, launch bool) (Caller, error) {
	c, err := remote.Dial(ctx, launch)
	if err != nil {
		return nil, err
	}
	return c, nil
}

// request is a message the extension sends: a call of one of the app's browser methods (relayed), with an ID the answer carries back.
type request struct {
	ID     int64           `json:"id"`
	Method string          `json:"method"`
	Params json.RawMessage `json:"params,omitempty"`
}

// answer is the host's message for the request with the same ID: the app's result, or an error.
type answer struct {
	ID     int64           `json:"id"`
	Result json.RawMessage `json:"result,omitempty"`
	Error  *remote.Error   `json:"error,omitempty"`
}

// relayed are the methods the extension may call, and whether each starts the app when it is not running: asking what the library has of a page's videos does not, as every page of a video site the user opens asks it, while adding a video and showing it do, as the kando command line does.
var relayed = map[string]bool{
	remote.MethodBrowserState: false,
	remote.MethodBrowserAdd:   true,
	remote.MethodBrowserShow:  true,
}

// Serve answers the extension's requests read from in, one after another, writing each answer to out, until in ends (nil) or a message cannot be read or written (its error). It connects to the app through dial at the first request, and again once that connection is closed, as when the app quit and started again. A request for another method than the extension's (relayed), or one that is not JSON, is answered with an error and the host goes on.
func Serve(ctx context.Context, in io.Reader, out io.Writer, dial Dialer) error {
	var app Caller
	defer func() {
		if app != nil {
			_ = app.Close()
		}
	}()
	for {
		data, err := ReadMessage(in)
		if errors.Is(err, io.EOF) {
			return nil
		}
		if err != nil {
			return err
		}
		var req request
		var res answer
		if err := json.Unmarshal(data, &req); err != nil {
			res.Error = remote.Errorf(remote.CodeParse, "%v", err)
		} else {
			res.ID = req.ID
			res.Result, res.Error = relay(ctx, &app, dial, req)
		}
		line, err := json.Marshal(res)
		if err != nil {
			return err
		}
		if err := WriteMessage(out, line); err != nil {
			return err
		}
	}
}

// relay passes req to the app over *app, dialing it first when there is no connection or it is closed.
func relay(ctx context.Context, app *Caller, dial Dialer, req request) (json.RawMessage, *remote.Error) {
	launch, ok := relayed[req.Method]
	if !ok {
		return nil, remote.Errorf(remote.CodeUnknownMethod, "unknown method %q", req.Method)
	}
	if *app != nil && (*app).Closed() {
		_ = (*app).Close()
		*app = nil
	}
	if *app == nil {
		c, err := dial(ctx, launch)
		if errors.Is(err, remote.ErrNotRunning) {
			return nil, remote.Errorf(CodeNotRunning, "%v", err)
		}
		if err != nil {
			return nil, remote.Errorf(remote.CodeFailed, "%v", err)
		}
		*app = c
	}
	var params any
	if len(req.Params) > 0 {
		params = req.Params
	}
	var result json.RawMessage
	err := (*app).Call(ctx, req.Method, params, &result)
	var failed *remote.Error
	switch {
	case errors.As(err, &failed):
		return nil, failed
	case err != nil:
		return nil, remote.Errorf(remote.CodeFailed, "%v", err)
	}
	return result, nil
}
