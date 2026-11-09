package remote

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
	"runtime"
	"strings"
	"sync"
	"time"
)

// Client is a connection to the running app. Its calls are answered one after another; it is safe to use from several goroutines. A call whose request or answer goes wrong on the way, as one that times out before its answer comes, closes the connection, as an answer still on its way would otherwise be read as the next call's: every later call fails at once with ErrBroken, and the caller dials again.
type Client struct {
	mu     sync.Mutex
	conn   net.Conn
	reader *bufio.Reader
	next   int64
	// broken is why the connection was closed, nil while it is open.
	broken error
}

// ErrBroken is Call's error on a Client whose connection an earlier call or Close closed.
var ErrBroken = errors.New("the connection to Kando is closed; dial again")

// ErrNotRunning is Dial's error when the app is not running and launch is false, or when it was launched and did not open its socket in time.
var ErrNotRunning = errors.New("Kando is not running")

// Launch starts the app in the background when Dial finds it is not running. Tests replace it.
var Launch = launchApp

// Dial connects to the app at SocketPath. When it is not running and launch is set, Dial starts it (Launch) and waits up to Timeout for its socket.
func Dial(ctx context.Context, launch bool) (*Client, error) {
	path := SocketPath()
	conn, err := dial(path)
	if err == nil {
		return newClient(conn), nil
	}
	if !launch {
		return nil, fmt.Errorf("%w (no socket at %s)", ErrNotRunning, path)
	}
	if err := Launch(); err != nil {
		return nil, fmt.Errorf("start Kando: %w", err)
	}
	deadline := time.Now().Add(Timeout)
	for {
		if conn, err := dial(path); err == nil {
			return newClient(conn), nil
		}
		if time.Now().After(deadline) {
			return nil, fmt.Errorf("%w: it did not open %s within %s", ErrNotRunning, path, Timeout)
		}
		select {
		case <-ctx.Done():
			return nil, ctx.Err()
		case <-time.After(200 * time.Millisecond):
		}
	}
}

func dial(path string) (net.Conn, error) {
	return net.DialTimeout("unix", path, 2*time.Second)
}

func newClient(conn net.Conn) *Client {
	return &Client{conn: conn, reader: bufio.NewReaderSize(conn, 64*1024)}
}

// Call sends method with params and decodes the result into out, which may be nil. An error the app answered is an *Error. Any other error but one of encoding params or decoding the result into out closes the connection (see Client).
func (c *Client) Call(ctx context.Context, method string, params, out any) error {
	c.mu.Lock()
	defer c.mu.Unlock()
	if c.broken != nil {
		return fmt.Errorf("%w (%v)", ErrBroken, c.broken)
	}
	c.next++
	req := Request{ID: c.next, Method: method}
	if params != nil {
		data, err := json.Marshal(params)
		if err != nil {
			return err
		}
		req.Params = data
	}
	line, err := json.Marshal(req)
	if err != nil {
		return err
	}
	resp, err := c.exchange(ctx, line)
	if err != nil {
		c.closeWith(err)
		return err
	}
	if resp.ID != req.ID {
		err := fmt.Errorf("answer %d to request %d", resp.ID, req.ID)
		c.closeWith(err)
		return err
	}
	if resp.Error != nil {
		return resp.Error
	}
	if out == nil || len(resp.Result) == 0 {
		return nil
	}
	return json.Unmarshal(resp.Result, out)
}

// exchange writes a request line and reads its answer, within Timeout or ctx's deadline, whichever comes first.
func (c *Client) exchange(ctx context.Context, line []byte) (Response, error) {
	deadline := time.Now().Add(Timeout)
	if d, ok := ctx.Deadline(); ok && d.Before(deadline) {
		deadline = d
	}
	_ = c.conn.SetDeadline(deadline)
	if _, err := c.conn.Write(append(line, '\n')); err != nil {
		return Response{}, err
	}
	answer, err := c.reader.ReadBytes('\n')
	if err != nil {
		return Response{}, err
	}
	var resp Response
	if err := json.Unmarshal(answer, &resp); err != nil {
		return Response{}, err
	}
	return resp, nil
}

// closeWith closes the connection after err, which later calls report. The caller holds mu.
func (c *Client) closeWith(err error) {
	c.broken = err
	_ = c.conn.Close()
}

// Close ends the connection. A call waiting for its answer returns with the read's error, and later calls with ErrBroken.
func (c *Client) Close() error {
	err := c.conn.Close()
	c.mu.Lock()
	if c.broken == nil {
		c.broken = errors.New("closed")
	}
	c.mu.Unlock()
	return err
}

// launchApp starts the app this program belongs to, without bringing it to the front: on macOS the .app bundle the executable is in, through open; elsewhere, or for an executable outside a bundle, the executable itself with no arguments, in a session of its own so it outlives the command.
func launchApp() error {
	exe, err := os.Executable()
	if err != nil {
		return err
	}
	if resolved, err := filepath.EvalSymlinks(exe); err == nil {
		exe = resolved
	}
	name, args := launchCommand(runtime.GOOS, exe, os.Getenv("KANDO_SOCKET") != "")
	if name == "open" {
		return exec.Command(name, args...).Run()
	}
	cmd := exec.Command(name, args...)
	cmd.Stdin, cmd.Stdout, cmd.Stderr = nil, nil, nil
	detach(cmd)
	if err := cmd.Start(); err != nil {
		return err
	}
	return cmd.Process.Release()
}

// launchCommand is the program and arguments launchApp runs on goos to start the app of the executable exe. On macOS an executable inside a .app bundle starts the bundle through open -g, in the background; open passes on the caller's environment but brings a running copy of the bundle forward instead of starting another, so with KANDO_SOCKET set (ownSocket), which names a socket the running copy may not listen on, it adds -n to start a copy that does. Anything else runs exe itself.
func launchCommand(goos, exe string, ownSocket bool) (name string, args []string) {
	if goos == "darwin" {
		if i := strings.Index(exe, ".app/Contents/MacOS/"); i >= 0 {
			if ownSocket {
				return "open", []string{"-g", "-n", exe[:i+len(".app")]}
			}
			return "open", []string{"-g", exe[:i+len(".app")]}
		}
	}
	return exe, nil
}
