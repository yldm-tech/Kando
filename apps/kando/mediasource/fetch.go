package mediasource

import (
	"bytes"
	"context"
	"crypto/sha256"
	"crypto/tls"
	"crypto/x509"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"log"
	"mime"
	"net"
	"net/http"
	"net/netip"
	"net/url"
	"slices"
	"strconv"
	"strings"
	"sync"
	"syscall"
	"time"

	"kando/apps/kando/plugins"
	"kando/packages/media"
)

// The limits of kando.source.fetch (docs/media-sources.md 11.3): the headers a plugin may set on a request, the bytes of its body and of the reply's, and the redirects Kando follows as it reads a file.
const (
	FetchHeadersMax = 16
	FetchBodyMax    = 1 << 20
	FetchReplyMax   = 4 << 20
	redirectsMax    = 5
	headerValueMax  = 1024
)

// FetchMethods are the methods a plugin's source may send: reading and listing, and POST under the paths its manifest names (plugins.MediaSource.PostPaths). None writes, removes or moves a file the way PUT, DELETE, MOVE, COPY or MKCOL would.
var FetchMethods = []string{http.MethodGet, http.MethodHead, http.MethodPost, "PROPFIND", "REPORT"}

// fetchRefused are the headers a plugin may not set on a request: those the request itself or Kando decide, and those many servers read as the request's method in place of its own, which would let a POST or a GET do what FetchMethods leaves out, as a DELETE.
var fetchRefused = []string{"host", "cookie", "content-length", "range", "connection", "transfer-encoding", "te", "upgrade", "proxy-authorization", "expect", "origin", "referer", "user-agent", "accept-encoding", "if-range", "x-http-method-override", "x-http-method", "x-method-override"}

// replyHeaders are the headers of a reply a plugin is given.
var replyHeaders = []string{"Content-Type", "ETag", "Last-Modified", "Content-Length"}

// PinKey is the key of a plugin's source's Config that holds the fingerprint of the self-signed certificate the user trusted for its endpoint (CertError). No field of a plugin's kind can have it, as a field's key has no dot.
const PinKey = "tls.fingerprint"

// The errors of a Fetcher. ErrFetchDenied is a request its source's rules do not let go: a method, a path outside the endpoint, a POST outside its prefixes, a header a plugin may not set, a redirect to http. ErrFetchTooLarge is a request's body or a reply over its limit. An offline Fetcher refuses an endpoint on the internet with media.ErrOffline.
var (
	ErrFetchDenied   = errors.New("mediasource: the source's rules do not allow the request")
	ErrFetchTooLarge = errors.New("mediasource: the request or its reply is too large")
)

// CertError is the error of an https endpoint whose certificate Kando cannot verify, as a NAS's self-signed one, or that is not the one the user trusted (Pinned): Fingerprint is the certificate's SHA-256 fingerprint (Fingerprint), which the user compares with the server's before trusting it (docs/media-sources.md 12.2, 17.5), Err why the system did not verify it.
type CertError struct {
	Fingerprint string
	Pinned      string
	Err         error
}

func (e *CertError) Error() string {
	if e.Pinned != "" {
		return "mediasource: the server's certificate " + e.Fingerprint + " is not the one trusted, " + e.Pinned
	}
	return fmt.Sprintf("mediasource: the server's certificate %s is not trusted: %v", e.Fingerprint, e.Err)
}

func (e *CertError) Unwrap() error { return e.Err }

// Fingerprint is a certificate's SHA-256 fingerprint as browsers show it: its hexadecimal pairs in capitals joined by colons, after "SHA256 ".
func Fingerprint(der []byte) string {
	sum := sha256.Sum256(der)
	pairs := make([]string, len(sum))
	for i, b := range sum {
		pairs[i] = fmt.Sprintf("%02X", b)
	}
	return "SHA256 " + strings.Join(pairs, ":")
}

// ParseEndpoint is the address a plugin's source reaches, as the user filled it in (EndpointKey): https anywhere, or http only on this computer or the local network, by an address that is a loopback, private or link-local one, or a name under .local, .lan or .home.arpa (docs/media-sources.md 11.3), whose password then crosses the network unencrypted, which the form says. It takes no user, query or fragment; its path, "/" for none, is the root every request of the source stays under.
func ParseEndpoint(raw string) (*url.URL, error) {
	u, err := url.Parse(strings.TrimSpace(raw))
	if err != nil {
		return nil, err
	}
	if u.User != nil || u.RawQuery != "" || u.Fragment != "" || u.Opaque != "" || u.Host == "" || u.Hostname() == "" {
		return nil, fmt.Errorf("mediasource: %q is not a server's address", raw)
	}
	switch u.Scheme {
	case "https":
	case "http":
		if !PlainHTTPHost(u.Hostname()) {
			return nil, fmt.Errorf("mediasource: http is only for a server on the local network, not %s", u.Hostname())
		}
	default:
		return nil, fmt.Errorf("mediasource: %q is not an http or https address", raw)
	}
	if u.Path == "" {
		u.Path = "/"
	}
	if !strings.HasSuffix(u.Path, "/") {
		u.Path += "/"
	}
	u.RawPath = ""
	return u, nil
}

// PlainHTTPHost reports whether a source's endpoint on host may be http: an address that is a loopback, private or link-local one, or a name under .local, .lan or .home.arpa, as a NAS on the local network has.
func PlainHTTPHost(host string) bool {
	host = strings.ToLower(strings.TrimSuffix(host, "."))
	if addr, err := netip.ParseAddr(strings.Trim(host, "[]")); err == nil {
		return localAddr(addr)
	}
	return host == "localhost" || strings.HasSuffix(host, ".local") || strings.HasSuffix(host, ".lan") || strings.HasSuffix(host, ".home.arpa")
}

// FetchRequest is a request a plugin's source sends through Kando (kando.source.fetch): its method, a path relative to the endpoint's, the query, the plugin's headers and its body; Login marks a login request, whose body Kando writes the source's login secrets into ({{secret:<key>}}).
type FetchRequest struct {
	Method string
	Path   string
	Query  map[string]string
	Header map[string]string
	Body   string
	Login  bool
}

// FetchReply is what a plugin is given of the reply: its status, the headers of replyHeaders it had, and its body.
type FetchReply struct {
	Status int
	Header map[string]string
	Body   []byte
}

// OpenTarget is where a plugin's open export says a file is read from (docs/media-sources.md 11.2): Path and Query relative to the endpoint, or URL, an address of the endpoint's origin; Header the plugin's headers for it; TTL how long Kando may read it from there before it asks again.
type OpenTarget struct {
	Path   string
	Query  map[string]string
	URL    string
	Header map[string]string
	TTL    time.Duration
}

// Fetcher sends a plugin's source's requests (docs/media-sources.md 11.3): only to its endpoint, under the endpoint's path, with the methods of FetchMethods, POST only under the source's PostPaths, the secrets put in as each field says (plugins.MediaSourceAuth), Config holding the source's values that are not secret, as the user a Basic secret goes with,, and its replies kept to FetchReplyMax. It follows no redirect for the plugin; reading a file (Open) follows up to five, keeping the plugin's headers and the secrets only on the endpoint's origin and going elsewhere only by https. An https endpoint's certificate is verified by the system, or by its fingerprint once the user trusted it (Pin). Offline (Offline) it connects only to addresses on the local network, judged as it dials, so a name that resolves elsewhere is refused.
type Fetcher struct {
	Endpoint *url.URL
	Source   plugins.MediaSource
	Config   map[string]string
	Secrets  map[string]string
	Pin      string
	Offline  bool
	// OfflineNow reports whether offline mode is on as a request is sent, for a fetcher that outlives a change of it, as a driver's reading files does; nil when only Offline says.
	OfflineNow func() bool
	// Roots verifies the endpoint's certificate in place of the system's roots, as a test's server's; nil for the system's.
	Roots *x509.CertPool
	// Dial connects in place of a net.Dialer, as a test's does; nil for one.
	Dial func(ctx context.Context, network, addr string) (net.Conn, error)
	// Log notes a server that ignored a Range (Open); nil for the standard logger.
	Log func(format string, args ...any)

	// refused is the last certificate the endpoint was refused for, shared by f's clones (Clone), as the plugin hears only that its request failed (TakeCertError).
	refused *refusal
}

// refusal is the last CertError of a source's fetches.
type refusal struct {
	mu   sync.Mutex
	cert *CertError
}

// note keeps err when it is a CertError, for TakeCertError.
func (f *Fetcher) note(err error) error {
	var cert *CertError
	if f.refused != nil && errors.As(err, &cert) {
		f.refused.mu.Lock()
		f.refused.cert = cert
		f.refused.mu.Unlock()
	}
	return err
}

// TakeCertError is the certificate the endpoint was last refused for by f or one of its clones, and forgets it: nil when there was none since. A plugin hears only that its request failed (unavailable), so what Kando says of a failed listing asks it here whether a certificate was why.
func (f *Fetcher) TakeCertError() *CertError {
	if f.refused == nil {
		return nil
	}
	f.refused.mu.Lock()
	defer f.refused.mu.Unlock()
	cert := f.refused.cert
	f.refused.cert = nil
	return cert
}

// client is the HTTP client of f's requests: its own transport, which checks the endpoint's certificate as f says and dials only the local network offline, and redirect, which decides the redirects it follows (nil for none).
func (f *Fetcher) client(redirect func(req *http.Request, via []*http.Request) error) *http.Client {
	dialer := &net.Dialer{Timeout: 30 * time.Second}
	if f.Offline || f.OfflineNow != nil && f.OfflineNow() {
		dialer.Control = func(_, address string, _ syscall.RawConn) error {
			host, _, err := net.SplitHostPort(address)
			if err != nil {
				return err
			}
			if addr, err := netip.ParseAddr(host); err != nil || !localAddr(addr) {
				return media.ErrOffline
			}
			return nil
		}
	}
	dial := dialer.DialContext
	if f.Dial != nil {
		dial = f.Dial
	}
	endpoint := f.Endpoint.Hostname()
	transport := &http.Transport{
		Proxy:               nil,
		DialContext:         dial,
		TLSHandshakeTimeout: 30 * time.Second,
		IdleConnTimeout:     90 * time.Second,
		// The TLS connection is made here rather than by the transport, so the certificate is checked knowing the host dialed, an address among them, for which the transport gives no server name: by the system, which tells the certificate of one it refused, for the user to trust (CertError); and the endpoint's, once the user trusted one, by its fingerprint alone, as a NAS's own certificate rarely names the address it is reached by.
		DialTLSContext: func(ctx context.Context, network, addr string) (net.Conn, error) {
			host, _, err := net.SplitHostPort(addr)
			if err != nil {
				return nil, err
			}
			isEndpoint := strings.EqualFold(host, endpoint)
			raw, err := dial(ctx, network, addr)
			if err != nil {
				return nil, err
			}
			conn := tls.Client(raw, f.tlsConfig(host, isEndpoint))
			if err := conn.HandshakeContext(ctx); err != nil {
				raw.Close()
				var refused *tls.CertificateVerificationError
				if isEndpoint && errors.As(err, &refused) && len(refused.UnverifiedCertificates) > 0 {
					return nil, &CertError{Fingerprint: Fingerprint(refused.UnverifiedCertificates[0].Raw), Err: refused.Err}
				}
				return nil, err
			}
			return conn, nil
		},
	}
	if redirect == nil {
		redirect = func(*http.Request, []*http.Request) error { return http.ErrUseLastResponse }
	}
	return &http.Client{Transport: transport, CheckRedirect: redirect}
}

// tlsConfig is how the server at host is reached: its certificate verified by the system (by Roots in a test) for host; or, for the endpoint once the user trusted a certificate of its (Pin), that certificate alone, by its fingerprint, which takes the place of the system's verification, as trust on first use does for an SFTP server's key.
func (f *Fetcher) tlsConfig(host string, endpoint bool) *tls.Config {
	cfg := &tls.Config{ServerName: host, MinVersion: tls.VersionTLS12, RootCAs: f.Roots}
	if !endpoint || f.Pin == "" {
		return cfg
	}
	pin := f.Pin
	cfg.InsecureSkipVerify = true // The certificate is checked against the fingerprint the user trusted, in VerifyConnection.
	cfg.VerifyConnection = func(cs tls.ConnectionState) error {
		if len(cs.PeerCertificates) == 0 {
			return errors.New("mediasource: the server sent no certificate")
		}
		if got := Fingerprint(cs.PeerCertificates[0].Raw); got != pin {
			return &CertError{Fingerprint: got, Pinned: pin}
		}
		return nil
	}
	return cfg
}

// target is the address of path and query under the endpoint, ErrFetchDenied when path is not plain (a segment . or .., an empty one, an escaped slash or dot) and so might leave the endpoint's path.
func (f *Fetcher) target(p string, query map[string]string) (*url.URL, error) {
	rel := "/" + strings.TrimPrefix(p, "/")
	if !plainRelative(rel) {
		return nil, fmt.Errorf("%w: path %q", ErrFetchDenied, p)
	}
	u := *f.Endpoint
	segments := strings.Split(strings.TrimPrefix(rel, "/"), "/")
	for i, s := range segments {
		unescaped, err := url.PathUnescape(s)
		if err != nil {
			return nil, fmt.Errorf("%w: path %q", ErrFetchDenied, p)
		}
		segments[i] = unescaped
	}
	u.Path = strings.TrimSuffix(f.Endpoint.Path, "/") + "/" + strings.Join(segments, "/")
	u.RawPath = ""
	if len(query) > 0 {
		q := url.Values{}
		for k, v := range query {
			q.Set(k, v)
		}
		u.RawQuery = q.Encode()
	}
	return &u, nil
}

// plainRelative reports whether p, a path relative to the endpoint written from a slash, names one place a server cannot read as another: no segment . or .., no empty segment but a last one, and no escaped slash, backslash or dot.
func plainRelative(p string) bool {
	lower := strings.ToLower(p)
	if strings.Contains(p, "\\") || strings.Contains(lower, "%2f") || strings.Contains(lower, "%5c") || strings.Contains(lower, "%2e") || strings.ContainsAny(p, "?#\x00") {
		return false
	}
	segments := strings.Split(strings.TrimPrefix(p, "/"), "/")
	for i, s := range segments {
		if s == "." || s == ".." || s == "" && i < len(segments)-1 {
			return false
		}
	}
	return true
}

// header is the plugin's headers h as a request sends them, ErrFetchDenied for too many, one a plugin may not set, or a value with a line break or over 1024 bytes; then the source's secrets put in as their fields say, over the plugin's headers of the same name. A secret not given is left out, so the server refuses the request and the plugin hears auth.
func (f *Fetcher) header(h map[string]string) (http.Header, error) {
	if len(h) > FetchHeadersMax {
		return nil, fmt.Errorf("%w: %d headers", ErrFetchDenied, len(h))
	}
	out := http.Header{}
	for k, v := range h {
		lower := strings.ToLower(k)
		if !validHeader(k) || slices.Contains(fetchRefused, lower) || strings.HasPrefix(lower, "sec-") || strings.HasPrefix(lower, "proxy-") || strings.ContainsAny(v, "\r\n\x00") || len(v) > headerValueMax {
			return nil, fmt.Errorf("%w: header %q", ErrFetchDenied, k)
		}
		out.Set(k, v)
	}
	f.inject(out)
	return out, nil
}

// inject puts the source's secrets into h as their fields say: HTTP Basic, or a header of the field's format.
func (f *Fetcher) inject(h http.Header) {
	for _, field := range f.Source.Fields {
		a := field.Auth
		secret := f.Secrets[field.Key]
		if a == nil || secret == "" {
			continue
		}
		switch {
		case a.Basic != nil:
			user := f.Config[a.Basic[0]]
			h.Set("Authorization", "Basic "+base64.StdEncoding.EncodeToString([]byte(user+":"+secret)))
		case a.Header != "":
			format := a.Format
			if format == "" {
				format = plugins.SecretMark
			}
			h.Set(a.Header, strings.Replace(format, plugins.SecretMark, secret, 1))
		}
	}
}

// validHeader reports whether name is a header's name.
func validHeader(name string) bool {
	if name == "" || len(name) > 64 {
		return false
	}
	for _, c := range []byte(name) {
		if !(c >= 'a' && c <= 'z' || c >= 'A' && c <= 'Z' || c >= '0' && c <= '9' || c == '-') {
			return false
		}
	}
	return true
}

// body is the request's body: the plugin's, with the source's login secrets put in place of {{secret:<key>}} for a login request alone, escaped as the body's Content-Type reads them (loginEscape); ErrFetchTooLarge over FetchBodyMax.
func (f *Fetcher) body(r FetchRequest) (string, error) {
	if len(r.Body) > FetchBodyMax {
		return "", ErrFetchTooLarge
	}
	if !r.Login {
		return r.Body, nil
	}
	escape := loginEscape(r.Header)
	body := r.Body
	for _, field := range f.Source.Fields {
		if field.Auth != nil && field.Auth.Login {
			body = strings.ReplaceAll(body, "{{secret:"+field.Key+"}}", escape(f.Secrets[field.Key]))
		}
	}
	return body, nil
}

// loginEscape is how a secret is written into the body of a login request of the headers header, by its Content-Type: as the characters of a JSON string for JSON (application/json, or a type ending in +json), where {{secret:<key>}} stands inside the string's quotes; as a form's value for application/x-www-form-urlencoded; as it is otherwise. So a password with a quote, a backslash or an ampersand reaches the server as itself rather than breaking the body, or adding to it.
func loginEscape(header map[string]string) func(string) string {
	var contentType string
	for k, v := range header {
		if strings.EqualFold(k, "Content-Type") {
			contentType = v
		}
	}
	mediaType, _, _ := mime.ParseMediaType(contentType)
	switch {
	case mediaType == "application/json" || strings.HasSuffix(mediaType, "+json"):
		return func(s string) string {
			quoted, _ := json.Marshal(s)
			return string(quoted[1 : len(quoted)-1])
		}
	case mediaType == "application/x-www-form-urlencoded":
		return url.QueryEscape
	}
	return func(s string) string { return s }
}

// Fetch sends r for the plugin and gives back its reply, ErrFetchDenied when the source's rules do not let it go: a method not in FetchMethods, a POST outside the source's PostPaths, a login request that is not a POST, a path that is not plain, a header a plugin may not set. It follows no redirect, so a reply to elsewhere goes back to the plugin as it is, and keeps only replyHeaders of the reply's headers, so no Set-Cookie reaches the plugin.
func (f *Fetcher) Fetch(ctx context.Context, r FetchRequest) (FetchReply, error) {
	if !slices.Contains(FetchMethods, r.Method) {
		return FetchReply{}, fmt.Errorf("%w: method %q", ErrFetchDenied, r.Method)
	}
	if r.Method == http.MethodPost && !f.Source.AllowsPost("/"+strings.TrimPrefix(r.Path, "/")) {
		return FetchReply{}, fmt.Errorf("%w: POST to %q", ErrFetchDenied, r.Path)
	}
	// The login secrets go only into the body of a POST to a login path the manifest names, not into a request whose reply might echo them, as a PROPFIND's or a GET's.
	if r.Login && r.Method != http.MethodPost {
		return FetchReply{}, fmt.Errorf("%w: a login %s", ErrFetchDenied, r.Method)
	}
	u, err := f.target(r.Path, r.Query)
	if err != nil {
		return FetchReply{}, err
	}
	h, err := f.header(r.Header)
	if err != nil {
		return FetchReply{}, err
	}
	body, err := f.body(r)
	if err != nil {
		return FetchReply{}, err
	}
	req, err := http.NewRequestWithContext(ctx, r.Method, u.String(), strings.NewReader(body))
	if err != nil {
		return FetchReply{}, err
	}
	req.Header = h
	client := f.client(nil)
	// Each request has a transport of its own, whose connections go with it.
	defer client.CloseIdleConnections()
	resp, err := client.Do(req)
	if err != nil {
		return FetchReply{}, f.note(fetchErr(err))
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(io.LimitReader(resp.Body, FetchReplyMax+1))
	if err != nil {
		return FetchReply{}, err
	}
	if len(data) > FetchReplyMax {
		return FetchReply{}, ErrFetchTooLarge
	}
	out := FetchReply{Status: resp.StatusCode, Header: map[string]string{}, Body: data}
	for _, k := range replyHeaders {
		if v := resp.Header.Get(k); v != "" {
			out.Header[k] = v
		}
	}
	return out, nil
}

// fetchErr is err of a request, with what made it fail inside the transport brought out: offline mode refusing an address on the internet, or a certificate not trusted.
func fetchErr(err error) error {
	var cert *CertError
	switch {
	case errors.As(err, &cert):
		return cert
	case errors.Is(err, media.ErrOffline):
		return media.ErrOffline
	}
	return err
}

// ErrFetchStatus is the error of reading a file that the server answered with a status other than 200 or 206, which Status holds; Auth reports 401 and 403, for which the plugin's open is asked again (PluginDriver).
type ErrFetchStatus struct{ Status int }

func (e ErrFetchStatus) Error() string {
	return "mediasource: the server answered " + strconv.Itoa(e.Status)
}

// Auth reports whether the server refused the request for its credentials or their age.
func (e ErrFetchStatus) Auth() bool {
	return e.Status == http.StatusUnauthorized || e.Status == http.StatusForbidden
}

// Open reads the file at t from offset, length bytes or to its end with -1 (docs/media-sources.md 11.3): a GET with a Range, the plugin's headers and the source's secrets, to t's path under the endpoint or to its URL, which must be of the endpoint's origin. It follows up to five redirects: on the endpoint's origin with every header, elsewhere only by https and without the plugin's headers or the secrets, as a CDN a server sends a download to. A server that ignores the Range (200) has the bytes before offset skipped, which the log notes. A status other than 200 or 206 is ErrFetchStatus, 404 ErrNotExist.
func (f *Fetcher) Open(ctx context.Context, t OpenTarget, offset, length int64) (io.ReadCloser, error) {
	var u *url.URL
	if t.URL != "" {
		parsed, err := url.Parse(t.URL)
		if err != nil || !sameOrigin(parsed, f.Endpoint) || parsed.User != nil {
			return nil, fmt.Errorf("%w: %q is not of the source's server", ErrFetchDenied, t.URL)
		}
		if !strings.HasPrefix(parsed.Path, f.Endpoint.Path) || !plainRelative(parsed.EscapedPath()) {
			return nil, fmt.Errorf("%w: %q is not under the source's address", ErrFetchDenied, t.URL)
		}
		u = parsed
	} else {
		var err error
		if u, err = f.target(t.Path, t.Query); err != nil {
			return nil, err
		}
	}
	h, err := f.header(t.Header)
	if err != nil {
		return nil, err
	}
	plugin := map[string]bool{}
	for k := range t.Header {
		plugin[http.CanonicalHeaderKey(k)] = true
	}
	if offset > 0 || length >= 0 {
		end := ""
		if length >= 0 {
			end = strconv.FormatInt(offset+length-1, 10)
		}
		h.Set("Range", "bytes="+strconv.FormatInt(offset, 10)+"-"+end)
	}
	redirect := func(req *http.Request, via []*http.Request) error {
		if len(via) > redirectsMax {
			return fmt.Errorf("%w: more than %d redirects", ErrFetchDenied, redirectsMax)
		}
		if sameOrigin(req.URL, f.Endpoint) {
			return nil
		}
		if req.URL.Scheme != "https" {
			return fmt.Errorf("%w: a redirect to %s", ErrFetchDenied, req.URL.Scheme)
		}
		// Off the endpoint's origin only the Range goes on: the plugin's headers and the secrets stay with the endpoint.
		kept := http.Header{}
		if r := req.Header.Get("Range"); r != "" {
			kept.Set("Range", r)
		}
		req.Header = kept
		return nil
	}
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, u.String(), nil)
	if err != nil {
		return nil, err
	}
	req.Header = h
	client := f.client(redirect)
	resp, err := client.Do(req)
	if err != nil {
		client.CloseIdleConnections()
		return nil, f.note(fetchErr(err))
	}
	// The reading has a transport of its own, whose connections go as the reader closes.
	resp.Body = readCloser{resp.Body, closers{resp.Body, closeIdle{client}}}
	switch resp.StatusCode {
	case http.StatusPartialContent:
		return resp.Body, nil
	case http.StatusOK:
		if offset > 0 {
			logf := f.Log
			if logf == nil {
				logf = log.Printf
			}
			logf("media source: %s ignored a Range; skipping %d bytes", f.Endpoint.Host, offset)
			if _, err := io.CopyN(io.Discard, resp.Body, offset); err != nil {
				resp.Body.Close()
				return nil, err
			}
		}
		if length >= 0 {
			return readCloser{io.LimitReader(resp.Body, length), resp.Body}, nil
		}
		return resp.Body, nil
	case http.StatusNotFound:
		resp.Body.Close()
		return nil, ErrNotExist
	}
	resp.Body.Close()
	return nil, ErrFetchStatus{Status: resp.StatusCode}
}

// readCloser reads r and closes c.
type readCloser struct {
	io.Reader
	c io.Closer
}

func (r readCloser) Close() error { return r.c.Close() }

// closers closes each in turn, giving back the first error.
type closers []io.Closer

func (cs closers) Close() error {
	var first error
	for _, c := range cs {
		if err := c.Close(); err != nil && first == nil {
			first = err
		}
	}
	return first
}

// closeIdle closes the idle connections of a client as it closes.
type closeIdle struct{ c *http.Client }

func (c closeIdle) Close() error {
	c.c.CloseIdleConnections()
	return nil
}

// sameOrigin reports whether a and b have the same scheme, host and port, a port left out being the scheme's own.
func sameOrigin(a, b *url.URL) bool {
	port := func(u *url.URL) string {
		if p := u.Port(); p != "" {
			return p
		}
		if u.Scheme == "https" {
			return "443"
		}
		return "80"
	}
	return strings.EqualFold(a.Scheme, b.Scheme) && strings.EqualFold(a.Hostname(), b.Hostname()) && port(a) == port(b)
}

// trimBody is b as text, its invalid UTF-8 replaced, as a plugin is given a reply's body.
func trimBody(b []byte) string { return string(bytes.ToValidUTF8(b, []byte("�"))) }
