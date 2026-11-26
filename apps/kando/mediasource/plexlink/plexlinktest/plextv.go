// Package plexlinktest is a fake plex.tv for the tests of signing in to Plex (package plexlink): it hands out PINs, gives one its token once the test says the person signed in, and lists the servers of that token's account, answering as plex.tv's API v2 does.
package plexlinktest

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
	"testing"
)

// Connection is an address of a server the fake lists, as plex.tv's resources do.
type Connection struct {
	Protocol string `json:"protocol"`
	Address  string `json:"address"`
	Port     int    `json:"port"`
	URI      string `json:"uri"`
	Local    bool   `json:"local"`
	Relay    bool   `json:"relay"`
}

// Resource is a device of the account the fake lists: a server provides "server".
type Resource struct {
	Name             string       `json:"name"`
	ClientIdentifier string       `json:"clientIdentifier"`
	Provides         string       `json:"provides"`
	Owned            bool         `json:"owned"`
	AccessToken      string       `json:"accessToken"`
	Connections      []Connection `json:"connections"`
}

// TV is the fake plex.tv at URL: Token is the token the account signs in with, Resources what it lists to that token. Requests counts the requests it took, by method and path.
type TV struct {
	URL       string
	Token     string
	Resources []Resource

	mu       sync.Mutex
	pins     map[int64]*pin
	next     int64
	requests map[string]int
	expired  bool
}

type pin struct {
	code, client string
	signedIn     bool
}

// New starts a fake plex.tv for t, which stops with it.
func New(t *testing.T, token string) *TV {
	tv := &TV{Token: token, pins: map[int64]*pin{}, next: 1000, requests: map[string]int{}}
	srv := httptest.NewServer(http.HandlerFunc(tv.serve))
	t.Cleanup(srv.Close)
	tv.URL = srv.URL
	return tv
}

// SignIn is the person signing in at the page of the PIN of code, which gives it the token at its next check.
func (tv *TV) SignIn(code string) bool {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	for _, p := range tv.pins {
		if p.code == code {
			p.signedIn = true
			return true
		}
	}
	return false
}

// Expire makes every PIN given out expired, as plex.tv's are after half an hour.
func (tv *TV) Expire() {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	tv.expired = true
}

// Count is how many requests of method to path the fake took.
func (tv *TV) Count(method, path string) int {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	return tv.requests[method+" "+path]
}

// Pins are the codes of the PINs given out.
func (tv *TV) Pins() []string {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	var out []string
	for _, p := range tv.pins {
		out = append(out, p.code)
	}
	return out
}

func (tv *TV) serve(w http.ResponseWriter, r *http.Request) {
	tv.mu.Lock()
	defer tv.mu.Unlock()
	path := r.URL.Path
	if strings.HasPrefix(path, "/api/v2/pins/") {
		tv.requests[r.Method+" /api/v2/pins/"]++
	} else {
		tv.requests[r.Method+" "+path]++
	}
	client := r.Header.Get("X-Plex-Client-Identifier")
	if client == "" || r.Header.Get("X-Plex-Product") == "" {
		http.Error(w, "no client", http.StatusBadRequest)
		return
	}
	w.Header().Set("Content-Type", "application/json")
	switch {
	case r.Method == http.MethodPost && path == "/api/v2/pins":
		tv.next++
		p := &pin{code: fmt.Sprintf("code%d", tv.next), client: client}
		tv.pins[tv.next] = p
		_ = json.NewEncoder(w).Encode(map[string]any{"id": tv.next, "code": p.code, "expiresIn": 1800, "authToken": nil})
	case r.Method == http.MethodGet && strings.HasPrefix(path, "/api/v2/pins/"):
		id, _ := strconv.ParseInt(strings.TrimPrefix(path, "/api/v2/pins/"), 10, 64)
		p := tv.pins[id]
		if p == nil || p.client != client || tv.expired {
			http.Error(w, `{"errors":[{"code":1020,"message":"Code not found or expired"}]}`, http.StatusNotFound)
			return
		}
		var token any
		if p.signedIn {
			token = tv.Token
		}
		_ = json.NewEncoder(w).Encode(map[string]any{"id": id, "code": p.code, "authToken": token})
	case r.Method == http.MethodGet && path == "/api/v2/resources":
		if r.Header.Get("X-Plex-Token") != tv.Token {
			http.Error(w, `{"errors":[{"code":1001,"message":"User could not be authenticated"}]}`, http.StatusUnauthorized)
			return
		}
		if tv.Resources == nil {
			_, _ = w.Write([]byte("[]"))
			return
		}
		_ = json.NewEncoder(w).Encode(tv.Resources)
	default:
		http.NotFound(w, r)
	}
}

// Identity is a fake Plex Media Server's /identity of the machine id, which Pick asks a server's addresses.
func Identity(t *testing.T, id string) *httptest.Server {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path != "/identity" {
			http.NotFound(w, r)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{"MediaContainer": map[string]any{"size": 0, "claimed": true, "machineIdentifier": id}})
	}))
	t.Cleanup(srv.Close)
	return srv
}
