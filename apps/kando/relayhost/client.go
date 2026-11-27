// Package relayhost is 看懂托管 (Kando Plus's hosted phone relay) on the desktop's side: it asks EveryAPI for a short-lived key to a folder of EveryAPI's own bucket, renews it before it lapses, and hands Phone Sync's storage relay a bucket (Bucket) that works in that folder with that key, as the user's own bucket of 账号与上传 does. The phone needs nothing of it: it reads the presigned links its state.json carries, whoever's bucket they point at.
//
// The service. POST {Base}/api/kando/relay/credentials, with the EveryAPI key of the user as a bearer token, answers the key (Credentials); GET {Base}/api/kando/plus answers whether Kando Plus is active (Plus). EveryAPI enforces everything: whether the user has Plus, the 7-day grace after it ends, the storage quota. This package only follows what it says.
package relayhost

import (
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"
)

// DefaultBase is EveryAPI's API, where the hosted relay's endpoints are.
const DefaultBase = "https://api.everyapi.ai"

// PlansURL is the page of EveryAPI's dashboard where Kando Plus is subscribed to.
const PlansURL = "https://app.everyapi.ai/subscriptions"

// Poll is how often the desktop lists the phones' inboxes while the relay runs in the hosted bucket, in place of the user's own bucket's sync.RelayPoll (20 seconds). Every listing is a class A operation EveryAPI pays for, by the desktop and not by the phone: at 20 seconds a desktop left on all month lists about 130,000 times, at 60 seconds about 43,000, a third. A phone's call then waits at most a minute for its answer through the relay, which is already the path of a phone away from its computer; the state.json a phone follows is still written as the jobs change.
const Poll = 60 * time.Second

// The errors EveryAPI answers with, which the 手机同步 page words differently. Their texts name no key.
var (
	// ErrNoKey is the error when Kando has no EveryAPI key to ask with: the AI service is not EveryAPI, or EveryAPI has no key and nobody signed in.
	ErrNoKey = errors.New("no EveryAPI key to ask for the hosted relay with")
	// ErrUnauthorized is EveryAPI refusing the key (401).
	ErrUnauthorized = errors.New("EveryAPI does not take the key")
	// ErrPlusRequired is EveryAPI's plus_required: the account has no active Kando Plus, its grace ended or it never had one.
	ErrPlusRequired = errors.New("Kando Plus is not active")
	// ErrRelayDisabled is EveryAPI's relay_disabled: the hosted relay is off on its side.
	ErrRelayDisabled = errors.New("the hosted relay is off on EveryAPI's side")
	// ErrQuotaExceeded is EveryAPI's quota_exceeded: the account's storage is full.
	ErrQuotaExceeded = errors.New("the hosted relay's storage is full")
)

// Client asks EveryAPI for the hosted relay. It is safe for concurrent use.
type Client struct {
	// Base is EveryAPI's API, "" for DefaultBase. Tests point it at a server of their own.
	Base string
	// HTTP sends the requests, nil for a client that gives up after 30 seconds.
	HTTP *http.Client
	// Key is the user's EveryAPI key, asked anew (renew) once after EveryAPI refused the one before; ErrNoKey when there is none. It is called on the goroutine that asks.
	Key func(ctx context.Context, renew bool) (string, error)
}

// Credentials is the key to the hosted bucket EveryAPI answers: the S3-compatible endpoint, region and bucket, the folder every object of this account goes under (Prefix), the session key, when it lapses, the account's quota and use in bytes, and whether it may write now.
type Credentials struct {
	Endpoint        string   `json:"endpoint"`
	Region          string   `json:"region"`
	Bucket          string   `json:"bucket"`
	Prefix          string   `json:"prefix"`
	AccessKeyID     string   `json:"access_key_id"`
	SecretAccessKey string   `json:"secret_access_key"`
	SessionToken    string   `json:"session_token"`
	ExpiresAt       unixTime `json:"expires_at"`
	QuotaBytes      int64    `json:"quota_bytes"`
	UsedBytes       int64    `json:"used_bytes"`
	WriteAllowed    bool     `json:"write_allowed"`
}

// Full reports whether the account's storage is full: what it uses reached its quota.
func (c Credentials) Full() bool { return c.QuotaBytes > 0 && c.UsedBytes >= c.QuotaBytes }

// Plus is the account's Kando Plus as EveryAPI tells it.
type Plus struct {
	Active    bool     `json:"active"`
	PlanName  string   `json:"plan_name"`
	RenewsAt  unixTime `json:"renews_at"`
	ExpiresAt unixTime `json:"expires_at"`
	Relay     Usage    `json:"relay"`
}

// Usage is the hosted relay's storage: its quota and what it uses, in bytes.
type Usage struct {
	QuotaBytes int64 `json:"quota_bytes"`
	UsedBytes  int64 `json:"used_bytes"`
}

// Credentials asks EveryAPI for a new key to the hosted bucket.
func (c *Client) Credentials(ctx context.Context) (Credentials, error) {
	var out Credentials
	if err := c.do(ctx, http.MethodPost, "/api/kando/relay/credentials", &out); err != nil {
		return Credentials{}, err
	}
	if out.Endpoint == "" || out.Bucket == "" || out.AccessKeyID == "" || out.SecretAccessKey == "" || out.SessionToken == "" || out.ExpiresAt.IsZero() {
		return Credentials{}, errors.New("EveryAPI answered an incomplete key for the hosted relay")
	}
	return out, nil
}

// Plus asks EveryAPI whether the account has Kando Plus, and how much of the hosted relay's storage it uses.
func (c *Client) Plus(ctx context.Context) (Plus, error) {
	var out Plus
	err := c.do(ctx, http.MethodGet, "/api/kando/plus", &out)
	return out, err
}

// do sends method to path with the key, renewed once when EveryAPI refuses it, and reads the answer into out.
func (c *Client) do(ctx context.Context, method, path string, out any) error {
	if c.Key == nil {
		return ErrNoKey
	}
	for renew := range 2 {
		key, err := c.Key(ctx, renew == 1)
		if err != nil {
			return err
		}
		if key == "" {
			return ErrNoKey
		}
		err = c.send(ctx, method, path, key, out)
		if !errors.Is(err, ErrUnauthorized) {
			return err
		}
	}
	return ErrUnauthorized
}

// maxAnswer is the most of an answer read: a key and a few numbers.
const maxAnswer = 64 << 10

func (c *Client) send(ctx context.Context, method, path, key string, out any) error {
	base := strings.TrimRight(c.Base, "/")
	if base == "" {
		base = DefaultBase
	}
	var body io.Reader
	if method == http.MethodPost {
		body = bytes.NewReader([]byte("{}"))
	}
	req, err := http.NewRequestWithContext(ctx, method, base+path, body)
	if err != nil {
		return err
	}
	req.Header.Set("Authorization", "Bearer "+key)
	req.Header.Set("Accept", "application/json")
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	client := c.HTTP
	if client == nil {
		client = &http.Client{Timeout: 30 * time.Second}
	}
	resp, err := client.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, maxAnswer))
	if err != nil {
		return err
	}
	switch {
	case resp.StatusCode == http.StatusUnauthorized:
		return ErrUnauthorized
	case resp.StatusCode < 200 || resp.StatusCode > 299:
		// The answer's own words stay out of the error, which the log and the page show: only its code, which names one of the reasons above.
		var e struct {
			Error struct {
				Code string `json:"code"`
			} `json:"error"`
		}
		_ = json.Unmarshal(data, &e)
		switch e.Error.Code {
		case "plus_required":
			return ErrPlusRequired
		case "relay_disabled":
			return ErrRelayDisabled
		case "quota_exceeded":
			return ErrQuotaExceeded
		}
		return fmt.Errorf("EveryAPI answered HTTP %d for the hosted relay", resp.StatusCode)
	}
	if err := json.Unmarshal(data, out); err != nil {
		return fmt.Errorf("EveryAPI's answer for the hosted relay does not read: %w", err)
	}
	return nil
}

// unixTime is a time EveryAPI sends as seconds since 1970, or as an RFC 3339 string; 0, null and "" are none.
type unixTime struct{ time.Time }

func (u *unixTime) UnmarshalJSON(data []byte) error {
	s := strings.Trim(string(data), `"`)
	if s == "" || s == "null" || s == "0" {
		u.Time = time.Time{}
		return nil
	}
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		u.Time = time.Unix(n, 0)
		return nil
	}
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		return err
	}
	u.Time = t
	return nil
}
