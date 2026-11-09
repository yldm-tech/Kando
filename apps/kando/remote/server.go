package remote

import (
	"bufio"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"os"
	"path/filepath"
	"sync"
	"sync/atomic"
	"time"
)

// Handler answers one request: the method's result, which is written as JSON, or an error, which an *Error carries as it is and anything else as CodeFailed. It runs on the goroutine of the request's connection, so the app reads and changes its state through OnUI, and reads files outside it.
type Handler func(method string, params json.RawMessage) (any, error)

// Server is the app's listening socket.
type Server struct {
	path     string
	listener net.Listener
	handle   Handler
	wg       sync.WaitGroup
	mu       sync.Mutex
	conns    map[net.Conn]bool
	closed   bool
}

// ErrInUse is Listen's error when another app already listens at the path.
var ErrInUse = errors.New("another Kando is listening on the socket")

// Listen starts answering requests at path with handle, each connection on a goroutine of its own. A socket left at path by an app that ended without closing it is replaced; one another app listens on is not (ErrInUse), and anything at path that is not a socket, as a file KANDO_SOCKET names by mistake, is left as it is and fails Listen. Only the user may connect: the socket's folder is made for the user alone when it does not exist, and on Unix one that exists must be the user's and writable by no one else (privateFolder), as in a folder others can write to, as a shared temporary folder, another user could put a socket of their own in its place; the socket itself is the user's alone.
func Listen(path string, handle Handler) (*Server, error) {
	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o700); err != nil {
		return nil, err
	}
	if err := privateFolder(dir); err != nil {
		return nil, err
	}
	if info, err := os.Lstat(path); err == nil {
		if info.Mode().Type() != os.ModeSocket {
			return nil, fmt.Errorf("%s is not a socket: Kando listens there only in place of a socket, so move the file away or set KANDO_SOCKET to another path", path)
		}
		if conn, err := net.DialTimeout("unix", path, time.Second); err == nil {
			conn.Close()
			return nil, ErrInUse
		}
		if err := os.Remove(path); err != nil {
			return nil, err
		}
	}
	listener, err := net.Listen("unix", path)
	if err != nil {
		return nil, err
	}
	if err := os.Chmod(path, 0o600); err != nil {
		listener.Close()
		return nil, err
	}
	s := &Server{path: path, listener: listener, handle: handle, conns: map[net.Conn]bool{}}
	s.wg.Add(1)
	go s.accept()
	return s, nil
}

func (s *Server) accept() {
	defer s.wg.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			return
		}
		s.mu.Lock()
		if s.closed {
			s.mu.Unlock()
			conn.Close()
			return
		}
		s.conns[conn] = true
		s.mu.Unlock()
		s.wg.Add(1)
		go s.serve(conn)
	}
}

// maxLine caps a request line, so a client cannot make the app hold an unbounded one.
const maxLine = 1 << 20

func (s *Server) serve(conn net.Conn) {
	defer s.wg.Done()
	defer func() {
		s.mu.Lock()
		delete(s.conns, conn)
		s.mu.Unlock()
		conn.Close()
	}()
	scanner := bufio.NewScanner(conn)
	scanner.Buffer(make([]byte, 64*1024), maxLine)
	enc := json.NewEncoder(conn)
	for scanner.Scan() {
		if err := enc.Encode(s.answer(scanner.Bytes())); err != nil {
			return
		}
	}
}

// answer is the response to one request line.
func (s *Server) answer(line []byte) Response {
	var req Request
	if err := json.Unmarshal(line, &req); err != nil {
		return Response{Error: Errorf(CodeParse, "%v", err)}
	}
	if req.Method == "" {
		return Response{ID: req.ID, Error: Errorf(CodeInvalid, "no method")}
	}
	result, err := s.handle(req.Method, req.Params)
	if err != nil {
		var e *Error
		if !errors.As(err, &e) {
			e = &Error{Code: CodeFailed, Message: err.Error()}
		}
		return Response{ID: req.ID, Error: e}
	}
	data, err := json.Marshal(result)
	if err != nil {
		return Response{ID: req.ID, Error: Errorf(CodeFailed, "%v", err)}
	}
	return Response{ID: req.ID, Result: data}
}

// Path is the socket's path.
func (s *Server) Path() string { return s.path }

// Close stops listening, ends the open connections, waits for their answers to finish and removes the socket.
func (s *Server) Close() error {
	s.mu.Lock()
	if s.closed {
		s.mu.Unlock()
		return nil
	}
	s.closed = true
	err := s.listener.Close()
	for conn := range s.conns {
		conn.Close()
	}
	s.mu.Unlock()
	s.wg.Wait()
	_ = os.Remove(s.path)
	return err
}

// Params decodes params into a value of type T, as a handler reads its method's params. Missing params decode as T's zero value; ones that do not fit T are a CodeInvalid error.
func Params[T any](params json.RawMessage) (T, error) {
	var v T
	if len(params) == 0 || string(params) == "null" {
		return v, nil
	}
	if err := json.Unmarshal(params, &v); err != nil {
		return v, Errorf(CodeInvalid, "params: %v", err)
	}
	return v, nil
}

// OnUI runs fn through post, which runs a function on the app's UI thread, and waits for its answer, or for ctx. The app's Handler uses it, as requests arrive on the connections' goroutines. fn runs at most once, and only when the caller gets its answer: when ctx ends before the UI thread comes to it, fn does not run at all and OnUI answers ctx's error, so a request the client was told failed, as jobs.add after its timeout, changes nothing later; when ctx ends while fn runs, OnUI waits for fn and answers what it returned.
func OnUI[T any](ctx context.Context, post func(func()), fn func() (T, error)) (T, error) {
	type answer struct {
		v   T
		err error
	}
	// state is uiWaiting until the posted function or OnUI claims it: uiRunning when fn starts, uiAbandoned when ctx ends first.
	var state atomic.Int32
	done := make(chan answer, 1)
	post(func() {
		if !state.CompareAndSwap(uiWaiting, uiRunning) {
			return
		}
		v, err := fn()
		done <- answer{v, err}
	})
	select {
	case a := <-done:
		return a.v, a.err
	case <-ctx.Done():
		if state.CompareAndSwap(uiWaiting, uiAbandoned) {
			var zero T
			return zero, ctx.Err()
		}
		a := <-done
		return a.v, a.err
	}
}

// The states of OnUI's posted function.
const (
	uiWaiting int32 = iota
	uiRunning
	uiAbandoned
)
