// Package loopback is an HTTP server on the phone's loopback address that serves the system's player what it cannot open itself: the player opens a URL, and cannot pin the computer's self-signed certificate or send the phone's token, so a film the computer is still making plays from a URL of this server, which forwards each request to the computer (sync.Client.ServeVideo). It keeps to what the desktop's storage proxy keeps to (kando/apps/kando/storage, Proxy): it listens on 127.0.0.1 alone, each thing it serves has a random token of its own in its URL and no route lists anything, it answers only a request whose Host is its own address, so a web page whose name a DNS rebinding points at 127.0.0.1 cannot talk to it, and it sends no CORS header. iOS lets the player reach it by the app's NSAllowsLocalNetworking, Android by its loopback cleartext policy (nsgo.yaml).
package loopback

import (
	"crypto/rand"
	"fmt"
	"io"
	"log"
	"net"
	"net/http"
	"net/url"
	"strings"
	"sync"
	"time"
)

// Server is the loopback server.
type Server struct {
	listener net.Listener
	server   *http.Server
	// host is the address requests must name in their Host header: the listener's, 127.0.0.1:port.
	host string

	mu sync.Mutex
	// served is what each token serves, and tokens the token of each key given to URL.
	served map[string]http.Handler
	tokens map[string]string
}

// Start starts a Server on a free port of 127.0.0.1.
func Start() (*Server, error) {
	listener, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		return nil, err
	}
	s := &Server{listener: listener, host: listener.Addr().String(), served: map[string]http.Handler{}, tokens: map[string]string{}}
	s.server = &http.Server{Handler: s, ReadHeaderTimeout: 10 * time.Second, ErrorLog: log.New(io.Discard, "", 0)}
	go s.server.Serve(listener)
	return s, nil
}

// URL is the address h is served at under key, http://127.0.0.1:port/token/name, the name telling a player what it opens; asking again under the same key gives the same URL, serving the h given first, until it is revoked (Revoke).
func (s *Server) URL(key, name string, h http.Handler) string {
	s.mu.Lock()
	token, ok := s.tokens[key]
	if !ok {
		token = rand.Text()
		s.tokens[key] = token
		s.served[token] = h
	}
	s.mu.Unlock()
	return fmt.Sprintf("http://%s/%s/%s", s.host, token, url.PathEscape(name))
}

// Revoke stops serving what key was given: a request with its token is 404 from then on, one being answered goes on, and URL under the key gives a new token.
func (s *Server) Revoke(key string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if token, ok := s.tokens[key]; ok {
		delete(s.tokens, key)
		delete(s.served, token)
	}
}

// ServeHTTP serves what the token starting the request's path was given: 421 for a request whose Host is not the server's own address, and 404 for a path without a known token.
func (s *Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	if r.Host != s.host {
		http.Error(w, "misdirected request", http.StatusMisdirectedRequest)
		return
	}
	token, _, _ := strings.Cut(strings.TrimPrefix(r.URL.Path, "/"), "/")
	s.mu.Lock()
	h, ok := s.served[token]
	s.mu.Unlock()
	if !ok || token == "" {
		http.NotFound(w, r)
		return
	}
	// What the player reads is not kept by anything on the way, so a revoked URL serves nothing from a cache either.
	w.Header().Set("Cache-Control", "no-store")
	h.ServeHTTP(w, r)
}

// Close stops the server and ends the requests it is serving.
func (s *Server) Close() error { return s.server.Close() }
