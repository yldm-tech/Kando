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
	"slices"
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

// Films are the films of library lib a phone can play or study, those that keep no video (Film.SubtitlesOnly) and those of sound alone (Film.Sound) among them.
func (c *Client) Films(ctx context.Context, lib string) ([]Film, error) {
	var out Films
	err := c.getJSON(ctx, http.MethodGet, FilmsPath(lib)+"?"+QueryWith+"="+WithSubtitlesOnly+","+WithSound, nil, &out)
	return out.Films, err
}

// Making are the films of library lib being made (Film.Making), which a phone watches while they are made (OpFilmWatch); a desktop from before them lists none.
func (c *Client) Making(ctx context.Context, lib string) ([]Film, error) {
	var out Films
	if err := c.getJSON(ctx, http.MethodGet, FilmsPath(lib)+"?"+QueryWith+"="+WithMaking, nil, &out); err != nil {
		return nil, err
	}
	return slices.DeleteFunc(out.Films, func(f Film) bool { return !f.Making }), nil
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

// Part is a part of a film's video the desktop answers: Body reads the bytes from Start to End, both included, of the Total the file holds, and ETag names the file as it is (Validator), which a later request sends as IfRange so a part of a file that changed meanwhile comes whole instead. ContentType is the file's type as the desktop names it, such as video/mp4, which a phone keeping the file names it by.
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
	part := &Part{Body: resp.Body, ETag: Validator(resp.Header), ContentType: resp.Header.Get("Content-Type")}
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

// Validator is what names the file an answer of a video is of, for a later request's If-Range: its ETag, else its Last-Modified date, which is all a desktop sends of a film it serves from a NAS or a storage location that keeps no ETag (remotefs.Handler); "" for neither, when a part kept cannot be gone on with.
func Validator(h http.Header) string {
	if etag := h.Get("ETag"); etag != "" {
		return etag
	}
	return h.Get("Last-Modified")
}

// ServeVideo answers r, a player's request for film id's video, with the desktop's answer: r's method, GET or HEAD, and its Range, If-Range and If-None-Match go to the desktop over the pinned connection with the device's token, and the status, the headers that say what is sent, and the body come back as the desktop sends them. It serves a player that opens a URL and cannot pin a certificate or send a token, such as the system's, from a loopback server of the phone's.
func (c *Client) ServeVideo(w http.ResponseWriter, r *http.Request, id string) {
	if r.Method != http.MethodGet && r.Method != http.MethodHead {
		w.Header().Set("Allow", "GET, HEAD")
		http.Error(w, "method not allowed", http.StatusMethodNotAllowed)
		return
	}
	header := http.Header{}
	for _, k := range []string{"Range", "If-Range", "If-None-Match"} {
		if v := r.Header.Get(k); v != "" {
			header.Set(k, v)
		}
	}
	resp, err := c.request(r.Context(), r.Method, VideoPath(id), nil, header)
	if err != nil {
		http.Error(w, "the computer cannot be reached", http.StatusBadGateway)
		return
	}
	defer resp.Body.Close()
	for _, k := range []string{"Content-Type", "Content-Length", "Content-Range", "Accept-Ranges", "ETag", "Last-Modified"} {
		if v := resp.Header.Get(k); v != "" {
			w.Header().Set(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	if r.Method == http.MethodGet {
		_, _ = io.Copy(w, resp.Body)
	}
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
