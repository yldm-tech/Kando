package sync

import (
	"bytes"
	"context"
	"crypto/subtle"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/netip"
	"strconv"
	"strings"
)

// Client talks to Kando on a desktop at one address, trusting only the certificate whose SHA-256 is Fingerprint. Token is the device token the desktop gave at pairing, which Pair sets.
type Client struct {
	Addr        netip.AddrPort
	Fingerprint []byte
	Token       string
	http        *http.Client
}

// ErrFingerprint is the error of a request to a desktop whose certificate is not the one the client pins.
var ErrFingerprint = errors.New("the desktop's certificate is not the one paired with")

// Error is the desktop's answer with a status of 400 or more.
type Error struct {
	Status int
	ErrorBody
}

func (e *Error) Error() string {
	if e.Message == "" {
		return fmt.Sprintf("kando sync: %d %s", e.Status, e.Code)
	}
	return fmt.Sprintf("kando sync: %d %s: %s", e.Status, e.Code, e.Message)
}

// NewClient is a client of the desktop at addr whose certificate's fingerprint is fingerprint, with the device token token, "" before pairing.
func NewClient(addr netip.AddrPort, fingerprint []byte, token string) *Client {
	pinned := append([]byte(nil), fingerprint...)
	transport := &http.Transport{
		TLSClientConfig: &tls.Config{
			// The desktop's certificate is self-signed: no authority vouches for it, so the client checks it is the one pinned instead (VerifyConnection), which tls runs whatever InsecureSkipVerify says.
			InsecureSkipVerify: true,
			MinVersion:         tls.VersionTLS13,
			VerifyConnection: func(cs tls.ConnectionState) error {
				if len(cs.PeerCertificates) == 0 || subtle.ConstantTimeCompare(Fingerprint(cs.PeerCertificates[0].Raw), pinned) != 1 {
					return ErrFingerprint
				}
				return nil
			},
		},
		ForceAttemptHTTP2: true,
	}
	return &Client{Addr: addr, Fingerprint: pinned, Token: token, http: &http.Client{Transport: transport}}
}

// FetchFingerprint connects to the desktop at addr without pinning its certificate and returns the certificate's fingerprint, for a phone paired by hand to show its short form (ShortFingerprint) before it trusts it.
func FetchFingerprint(ctx context.Context, addr netip.AddrPort) ([]byte, error) {
	dialer := &tls.Dialer{Config: &tls.Config{InsecureSkipVerify: true, MinVersion: tls.VersionTLS13}}
	conn, err := dialer.DialContext(ctx, "tcp", addr.String())
	if err != nil {
		return nil, err
	}
	defer conn.Close()
	certs := conn.(*tls.Conn).ConnectionState().PeerCertificates
	if len(certs) == 0 {
		return nil, ErrFingerprint
	}
	return Fingerprint(certs[0].Raw), nil
}

// Close closes the connections the client keeps open.
func (c *Client) Close() { c.http.CloseIdleConnections() }

// Pair pairs the phone with the desktop with the code it shows, and keeps the device token it answers in Token.
func (c *Client) Pair(ctx context.Context, req PairRequest) (PairResponse, error) {
	body, err := json.Marshal(req)
	if err != nil {
		return PairResponse{}, err
	}
	var out PairResponse
	if err := c.getJSON(ctx, http.MethodPost, PathPair, body, &out); err != nil {
		return PairResponse{}, err
	}
	c.Token = out.Token
	return out, nil
}

// Libraries are the desktop's libraries, locked ones left out.
func (c *Client) Libraries(ctx context.Context) ([]Library, error) {
	var out Libraries
	err := c.getJSON(ctx, http.MethodGet, PathLibraries, nil, &out)
	return out.Libraries, err
}

// Films are the films of library lib a phone can play or study, those that keep no video among them (Film.SubtitlesOnly).
func (c *Client) Films(ctx context.Context, lib string) ([]Film, error) {
	var out Films
	err := c.getJSON(ctx, http.MethodGet, FilmsPath(lib)+"?"+QueryWith+"="+WithSubtitlesOnly, nil, &out)
	return out.Films, err
}

// Subtitles are the lines of film id's subtitles.
func (c *Client) Subtitles(ctx context.Context, id string) ([]Cue, error) {
	var out Subtitles
	err := c.getJSON(ctx, http.MethodGet, SubtitlesPath(id), nil, &out)
	return out.Cues, err
}

// Poster is film id's poster: the image file's bytes and its content type.
func (c *Client) Poster(ctx context.Context, id string) (data []byte, contentType string, err error) {
	resp, err := c.do(ctx, http.MethodGet, PosterPath(id), nil, nil)
	if err != nil {
		return nil, "", err
	}
	defer resp.Body.Close()
	data, err = io.ReadAll(resp.Body)
	return data, resp.Header.Get("Content-Type"), err
}

// Part is a part of a film's video the desktop answers: Body reads the bytes from Start to End, both included, of the Total the file holds, and ETag names the file as it is, which a later request sends as IfRange so a part of a file that changed meanwhile comes whole instead. ContentType is the file's type as the desktop names it, such as video/mp4, which a phone keeping the file names it by.
type Part struct {
	Body        io.ReadCloser
	Start, End  int64
	Total       int64
	ETag        string
	ContentType string
}

// Video asks for film id's video from byte from to byte to, both included; a negative to asks for the rest of the file. With ifRange, the ETag of an earlier part, a file that changed since answers whole, from byte 0. The caller closes the Part's Body.
func (c *Client) Video(ctx context.Context, id string, from, to int64, ifRange string) (*Part, error) {
	header := http.Header{}
	rng := "bytes=" + strconv.FormatInt(from, 10) + "-"
	if to >= 0 {
		rng += strconv.FormatInt(to, 10)
	}
	header.Set("Range", rng)
	if ifRange != "" {
		header.Set("If-Range", ifRange)
	}
	resp, err := c.do(ctx, http.MethodGet, VideoPath(id), nil, header)
	if err != nil {
		return nil, err
	}
	part := &Part{Body: resp.Body, ETag: resp.Header.Get("ETag"), ContentType: resp.Header.Get("Content-Type")}
	if resp.StatusCode == http.StatusPartialContent {
		if _, err := fmt.Sscanf(resp.Header.Get("Content-Range"), "bytes %d-%d/%d", &part.Start, &part.End, &part.Total); err != nil {
			resp.Body.Close()
			return nil, fmt.Errorf("kando sync: Content-Range %q: %v", resp.Header.Get("Content-Range"), err)
		}
		return part, nil
	}
	part.Total = resp.ContentLength
	part.End = resp.ContentLength - 1
	return part, nil
}

// getJSON sends a request and decodes its JSON answer into out.
func (c *Client) getJSON(ctx context.Context, method, path string, body []byte, out any) error {
	header := http.Header{}
	if body != nil {
		header.Set("Content-Type", "application/json")
	}
	resp, err := c.do(ctx, method, path, body, header)
	if err != nil {
		return err
	}
	defer resp.Body.Close()
	return json.NewDecoder(resp.Body).Decode(out)
}

// do sends a request with the token, and turns an answer of 400 or more into an *Error.
func (c *Client) do(ctx context.Context, method, path string, body []byte, header http.Header) (*http.Response, error) {
	resp, err := c.request(ctx, method, path, body, header)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 400 {
		return resp, nil
	}
	defer resp.Body.Close()
	e := &Error{Status: resp.StatusCode}
	data, _ := io.ReadAll(io.LimitReader(resp.Body, 64<<10))
	if json.Unmarshal(data, &e.ErrorBody) != nil {
		e.Message = strings.TrimSpace(string(data))
	}
	return nil, e
}

// request sends a request with the token and answers the response whatever its status.
func (c *Client) request(ctx context.Context, method, path string, body []byte, header http.Header) (*http.Response, error) {
	var reader io.Reader
	if body != nil {
		reader = bytes.NewReader(body)
	}
	req, err := http.NewRequestWithContext(ctx, method, "https://"+c.Addr.String()+path, reader)
	if err != nil {
		return nil, err
	}
	for k, v := range header {
		req.Header[k] = v
	}
	if c.Token != "" {
		req.Header.Set("Authorization", "Bearer "+c.Token)
	}
	return c.http.Do(req)
}
