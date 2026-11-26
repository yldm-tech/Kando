// Package plexlink signs a Plex account in for a media source of a kind whose token field says so (mediasource.Auth.Link, docs/media-sources.md 11.5): Kando asks plex.tv for a PIN, the person signs in to Plex in their browser at AuthURL, in whatever way their account does (password, Google, Apple, two-step), Kando waits for the PIN to be given a token, then lists the account's servers with the token each one takes and the address of each that answers. It does not import any interface package.
package plexlink

import (
	"context"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/url"
	"slices"
	"strings"
	"sync"
	"time"
)

// The addresses of plex.tv: the API, and the page the person signs in at.
const (
	DefaultAPI  = "https://plex.tv"
	DefaultAuth = "https://app.plex.tv/auth"
)

// ErrExpired is Wait's error once the PIN expired before the person signed in.
var ErrExpired = errors.New("plexlink: the PIN expired")

// ErrRefused is the error of plex.tv refusing a request, as a token it does not take.
var ErrRefused = errors.New("plexlink: plex.tv refused the request")

// errNotFound is plex.tv answering 404, as it does of a PIN once it expired.
var errNotFound = errors.New("plexlink: plex.tv has no such thing")

// Client talks to plex.tv for one sign-in: HTTP sends its requests (http.DefaultClient when nil), API and Auth are plex.tv's addresses (DefaultAPI and DefaultAuth when empty), Product names Kando to Plex, which lists the sign-in under it in the account's devices, and ID is the client's identifier there (NewID).
type Client struct {
	HTTP    *http.Client
	API     string
	Auth    string
	Product string
	ID      string
}

// NewID is a client identifier for a sign-in: random, so each source signed in is a device of its own in the Plex account, which the person can remove there to take its token back.
func NewID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

// PIN is a sign-in plex.tv waits for: its ID, the code the sign-in page is opened with, and when it expires.
type PIN struct {
	ID      int64
	Code    string
	Expires time.Time
}

// replyMax bounds what is read of a reply of plex.tv, which a list of an account's servers stays far below.
const replyMax = 4 << 20

// do sends a request of method to path under the API, with the token when one is given, and decodes its JSON reply into out.
func (c *Client) do(ctx context.Context, method, path, token string, out any) error {
	api := c.API
	if api == "" {
		api = DefaultAPI
	}
	req, err := http.NewRequestWithContext(ctx, method, strings.TrimSuffix(api, "/")+path, nil)
	if err != nil {
		return err
	}
	req.Header.Set("Accept", "application/json")
	req.Header.Set("X-Plex-Product", c.Product)
	req.Header.Set("X-Plex-Client-Identifier", c.ID)
	if token != "" {
		req.Header.Set("X-Plex-Token", token)
	}
	hc := c.HTTP
	if hc == nil {
		hc = http.DefaultClient
	}
	resp, err := hc.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	if resp.StatusCode == http.StatusUnauthorized || resp.StatusCode == http.StatusForbidden {
		return ErrRefused
	}
	if resp.StatusCode == http.StatusNotFound {
		return errNotFound
	}
	if resp.StatusCode/100 != 2 {
		return fmt.Errorf("plexlink: plex.tv answered %s", resp.Status)
	}
	return json.NewDecoder(io.LimitReader(resp.Body, replyMax)).Decode(out)
}

// NewPIN asks plex.tv for a PIN to sign in with.
func (c *Client) NewPIN(ctx context.Context) (PIN, error) {
	var r struct {
		ID        int64  `json:"id"`
		Code      string `json:"code"`
		ExpiresIn int    `json:"expiresIn"`
	}
	if err := c.do(ctx, http.MethodPost, "/api/v2/pins?strong=true", "", &r); err != nil {
		return PIN{}, err
	}
	if r.ID == 0 || r.Code == "" {
		return PIN{}, errors.New("plexlink: plex.tv gave no PIN")
	}
	if r.ExpiresIn <= 0 {
		r.ExpiresIn = 1800
	}
	return PIN{ID: r.ID, Code: r.Code, Expires: time.Now().Add(time.Duration(r.ExpiresIn) * time.Second)}, nil
}

// AuthURL is the page the person signs in to Plex at for p, which gives p its token.
func (c *Client) AuthURL(p PIN) string {
	auth := c.Auth
	if auth == "" {
		auth = DefaultAuth
	}
	q := url.Values{"clientID": {c.ID}, "code": {p.Code}, "context[device][product]": {c.Product}}
	return auth + "#?" + q.Encode()
}

// Wait asks plex.tv every every whether the person signed in for p, and gives back the token they did with; ErrExpired once p expired, or plex.tv no longer knows it, ctx's error once it is done. A request that fails otherwise is asked again at the next turn, as the network may come back.
func (c *Client) Wait(ctx context.Context, p PIN, every time.Duration) (string, error) {
	tick := time.NewTicker(every)
	defer tick.Stop()
	for {
		var r struct {
			AuthToken string `json:"authToken"`
		}
		err := c.do(ctx, http.MethodGet, fmt.Sprintf("/api/v2/pins/%d", p.ID), "", &r)
		switch {
		case err == nil && r.AuthToken != "":
			return r.AuthToken, nil
		case errors.Is(err, ErrRefused), errors.Is(err, errNotFound):
			return "", ErrExpired
		}
		if !time.Now().Before(p.Expires) {
			return "", ErrExpired
		}
		select {
		case <-ctx.Done():
			return "", ctx.Err()
		case <-tick.C:
		}
	}
}

// Server is a Plex Media Server the account reaches: its name, its machine identifier, whether the account owns it rather than another shares it, the token it takes from this account (its own for a server shared), and the addresses plex.tv knows it at, in the order Pick tries them.
type Server struct {
	Name        string
	ID          string
	Owned       bool
	Token       string
	Connections []string
}

// Servers are the servers of the account of token, those owned first, each by name.
func (c *Client) Servers(ctx context.Context, token string) ([]Server, error) {
	var r []struct {
		Name             string `json:"name"`
		ClientIdentifier string `json:"clientIdentifier"`
		Provides         string `json:"provides"`
		Owned            bool   `json:"owned"`
		AccessToken      string `json:"accessToken"`
		Connections      []struct {
			Protocol string `json:"protocol"`
			Address  string `json:"address"`
			Port     int    `json:"port"`
			URI      string `json:"uri"`
			Local    bool   `json:"local"`
			Relay    bool   `json:"relay"`
		} `json:"connections"`
	}
	if err := c.do(ctx, http.MethodGet, "/api/v2/resources?includeHttps=1&includeRelay=0", token, &r); err != nil {
		return nil, err
	}
	var out []Server
	for _, res := range r {
		if !slices.Contains(strings.Split(res.Provides, ","), "server") || res.ClientIdentifier == "" {
			continue
		}
		s := Server{Name: res.Name, ID: res.ClientIdentifier, Owned: res.Owned, Token: res.AccessToken}
		if s.Token == "" {
			s.Token = token
		}
		if s.Name == "" {
			s.Name = res.ClientIdentifier
		}
		// On the local network first, the plex.direct address with its certificate, then the same server over plain http, which a router that refuses DNS answers of local addresses (rebinding protection) leaves as the only way; then those across the internet. A relay is left out: Plex limits its speed far below a film's.
		var local, plain, remote []string
		for _, cn := range res.Connections {
			if cn.Relay || !strings.HasPrefix(cn.URI, "http") {
				continue
			}
			if !cn.Local {
				remote = append(remote, cn.URI)
				continue
			}
			local = append(local, cn.URI)
			if cn.Protocol == "https" && cn.Address != "" && cn.Port > 0 {
				plain = append(plain, fmt.Sprintf("http://%s:%d", cn.Address, cn.Port))
			}
		}
		for _, u := range slices.Concat(local, plain, remote) {
			if !slices.Contains(s.Connections, u) {
				s.Connections = append(s.Connections, u)
			}
		}
		out = append(out, s)
	}
	slices.SortStableFunc(out, func(a, b Server) int {
		if a.Owned != b.Owned {
			if a.Owned {
				return -1
			}
			return 1
		}
		return strings.Compare(strings.ToLower(a.Name), strings.ToLower(b.Name))
	})
	return out, nil
}

// probeTimeout bounds how long Pick waits for one address of a server.
const probeTimeout = 4 * time.Second

// Pick is the first of s's Connections, in their order, at which s answers as itself, tried all at once with hc (http.DefaultClient when nil); ok is false when none answered, and the address then the first, so the source's Test says why it is not reached.
func Pick(ctx context.Context, hc *http.Client, s Server) (address string, ok bool) {
	if len(s.Connections) == 0 {
		return "", false
	}
	if hc == nil {
		hc = http.DefaultClient
	}
	answered := make([]bool, len(s.Connections))
	var wg sync.WaitGroup
	for i, u := range s.Connections {
		wg.Add(1)
		go func() {
			defer wg.Done()
			answered[i] = identity(ctx, hc, u) == s.ID
		}()
	}
	wg.Wait()
	for i, yes := range answered {
		if yes {
			return s.Connections[i], true
		}
	}
	return s.Connections[0], false
}

// identity is the machine identifier of the Plex Media Server at address, "" when it does not answer as one in time. /identity takes no token.
func identity(ctx context.Context, hc *http.Client, address string) string {
	ctx, cancel := context.WithTimeout(ctx, probeTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, strings.TrimSuffix(address, "/")+"/identity", nil)
	if err != nil {
		return ""
	}
	req.Header.Set("Accept", "application/json")
	resp, err := hc.Do(req)
	if err != nil {
		return ""
	}
	defer resp.Body.Close()
	var r struct {
		MediaContainer struct {
			MachineIdentifier string `json:"machineIdentifier"`
		} `json:"MediaContainer"`
	}
	if resp.StatusCode != http.StatusOK || json.NewDecoder(io.LimitReader(resp.Body, 1<<16)).Decode(&r) != nil {
		return ""
	}
	return r.MediaContainer.MachineIdentifier
}
