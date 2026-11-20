package index

import (
	"context"
	"crypto/ed25519"
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kando/apps/kando/plugins"
)

// DefaultURL is where Kando reads the community index: the community plugin repository, yldm-tech/kando-plugins, signs it on every merge and deploys it with the pack files to its GitHub Pages, and a Cloudflare Worker serves both under Kando's landing site, the index here and the pack files beside it under plugin/packs/ on the same host (docs/plugin-system.md 15). The app's updates come from elsewhere, the GitHub releases (nsgo.yaml's release and updates).
const DefaultURL = "https://kando.yldm.ai/plugin/index.json"

// RefreshEvery is how often at most Kando asks for a newer index on its own; the user can ask at any time.
const RefreshEvery = 24 * time.Hour

// The time a read of the index and a download may take.
const (
	fetchTimeout    = 30 * time.Second
	downloadTimeout = 10 * time.Minute
)

// The files of the cache: the last index file read as it came (cacheName), and where it came from with its ETag and when it was last asked for (metaName).
const (
	cacheName = "index.json"
	metaName  = "index-meta.json"
	metaMax   = 64 << 10
)

// CacheDir is the folder of the plugins folder root where the last index is kept. Its name starts with a dot, so plugins.Scan, which reads the folders of the kinds alone, never takes it for a pack.
func CacheDir(root string) string { return filepath.Join(root, ".community") }

// Meta is what Kando knows of the index it keeps: the address it came from, the ETag the server gave it, and when Kando last asked for it, whether a new one came or the server said it had not changed.
type Meta struct {
	URL     string    `json:"url"`
	ETag    string    `json:"etag,omitempty"`
	Fetched time.Time `json:"fetched"`
}

// Source is the index Kando reads: from URL, checked with Key, kept in Dir (CacheDir), asked for with Client, or a client of its own when nil. Every request it makes follows a redirect only to the same host over https.
type Source struct {
	URL    string
	Key    ed25519.PublicKey
	Dir    string
	Client *http.Client
}

// ErrOtherAddress is a copy kept of the index of another address than the source's, or of an address not known: it is no copy of this source's index, neither shown nor compared with what comes.
var ErrOtherAddress = errors.New("the plugin list kept came from another address")

// Cached is the index kept in the cache, checked again as when it came (Parse), and what is known of it; an error when there is none, it came from another address (ErrOtherAddress) or it no longer checks, as when the key changed.
func (s Source) Cached() (Index, Meta, error) {
	if s.Key == nil {
		return Index{}, Meta{}, ErrNotConfigured
	}
	meta := s.meta()
	if meta.URL != s.URL {
		return Index{}, meta, fmt.Errorf("%w: %s", ErrOtherAddress, meta.URL)
	}
	data, err := readSmall(filepath.Join(s.Dir, cacheName), MaxBytes)
	if err != nil {
		return Index{}, meta, err
	}
	idx, err := Parse(data, s.Key)
	return idx, meta, err
}

// meta is what is known of the index kept, the zero Meta when nothing is.
func (s Source) meta() Meta {
	var m Meta
	data, err := readSmall(filepath.Join(s.Dir, metaName), metaMax)
	if err == nil && json.Unmarshal(data, &m) != nil {
		m = Meta{}
	}
	return m
}

// Due reports whether Kando should ask for the index on its own at now: it keeps none, or one from another address, or it last asked RefreshEvery or more ago.
func (s Source) Due(now time.Time) bool {
	m := s.meta()
	return m.URL != s.URL || m.Fetched.IsZero() || now.Sub(m.Fetched) >= RefreshEvery || now.Before(m.Fetched)
}

// Refresh asks for the index at URL, with the ETag of the one kept when it came from there too, and keeps what comes: the index kept when the server says it has not changed (304), else the new one once it checks (Parse) and is not older than the one kept from the same address (ErrRollback); a copy of another address is neither kept on a 304 nor compared (Cached). The file is read up to MaxBytes and no further. It returns the index and what is known of it now. Call it off the engine's goroutine; never while Kando is offline.
func (s Source) Refresh(ctx context.Context, now time.Time) (Index, Meta, error) {
	if s.Key == nil {
		return Index{}, Meta{}, ErrNotConfigured
	}
	if err := plugins.CheckHTTPS(s.URL); err != nil {
		return Index{}, Meta{}, err
	}
	cached, meta, cachedErr := s.Cached()
	ctx, cancel := context.WithTimeout(ctx, fetchTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, s.URL, nil)
	if err != nil {
		return Index{}, Meta{}, err
	}
	req.Header.Set("Accept", "application/json")
	if cachedErr == nil && meta.URL == s.URL && meta.ETag != "" {
		req.Header.Set("If-None-Match", meta.ETag)
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return Index{}, Meta{}, err
	}
	defer resp.Body.Close()
	switch {
	case resp.StatusCode == http.StatusNotModified && cachedErr == nil:
		meta.Fetched = now
		return cached, meta, s.writeMeta(meta)
	case resp.StatusCode != http.StatusOK:
		return Index{}, Meta{}, fmt.Errorf("%w: %s", ErrHTTP, resp.Status)
	case resp.ContentLength > MaxBytes:
		return Index{}, Meta{}, fmt.Errorf("%w: the index is %d bytes", ErrTooLarge, resp.ContentLength)
	}
	data, err := io.ReadAll(io.LimitReader(resp.Body, MaxBytes+1))
	if err != nil {
		return Index{}, Meta{}, err
	}
	if len(data) > MaxBytes {
		return Index{}, Meta{}, fmt.Errorf("%w: the index is over %d bytes", ErrTooLarge, MaxBytes)
	}
	idx, err := Parse(data, s.Key)
	if err != nil {
		return Index{}, Meta{}, err
	}
	if cachedErr == nil && idx.Generated.Before(cached.Generated) {
		return Index{}, Meta{}, fmt.Errorf("%w: it was made %s, the one kept %s", ErrRollback, idx.Generated.Format(time.RFC3339), cached.Generated.Format(time.RFC3339))
	}
	if err := writeAtomic(s.Dir, cacheName, data); err != nil {
		return Index{}, Meta{}, err
	}
	meta = Meta{URL: s.URL, ETag: resp.Header.Get("ETag"), Fetched: now}
	return idx, meta, s.writeMeta(meta)
}

func (s Source) writeMeta(m Meta) error {
	data, err := json.Marshal(m)
	if err != nil {
		return err
	}
	return writeAtomic(s.Dir, metaName, data)
}

// Download downloads the file of the entry e into a new file in dir and returns its path: over https alone, redirected only within the host, read up to e's size and no further, and kept only when its size and SHA-256 are e's. The file is the pack as the user would pick it; installing it is the app's, through the same Stage and install confirmation as any file. Call it off the engine's goroutine; never while Kando is offline.
func (s Source) Download(ctx context.Context, e Entry, dir string) (path string, err error) {
	if err := plugins.CheckHTTPS(e.URL); err != nil {
		return "", err
	}
	ctx, cancel := context.WithTimeout(ctx, downloadTimeout)
	defer cancel()
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, e.URL, nil)
	if err != nil {
		return "", err
	}
	resp, err := s.client().Do(req)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		return "", fmt.Errorf("%w: %s", ErrHTTP, resp.Status)
	}
	if resp.ContentLength > e.Size {
		return "", fmt.Errorf("%w: the file is %d bytes, the index says %d", ErrTooLarge, resp.ContentLength, e.Size)
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", err
	}
	f, err := os.CreateTemp(dir, "download-*.zip")
	if err != nil {
		return "", err
	}
	defer func() {
		if cerr := f.Close(); err == nil {
			err = cerr
		}
		if err != nil {
			os.Remove(f.Name())
			path = ""
		}
	}()
	h := sha256.New()
	n, err := io.Copy(io.MultiWriter(f, h), io.LimitReader(resp.Body, e.Size+1))
	switch {
	case err != nil:
		return "", err
	case n > e.Size:
		return "", fmt.Errorf("%w: the file is over the %d bytes the index says", ErrTooLarge, e.Size)
	case n != e.Size:
		return "", fmt.Errorf("%w: %d bytes, the index says %d", ErrChecksum, n, e.Size)
	case hex.EncodeToString(h.Sum(nil)) != e.SHA256:
		return "", fmt.Errorf("%w: its SHA-256 differs", ErrChecksum)
	}
	return f.Name(), nil
}

// client is the source's client, which follows a redirect only to the host asked first, over https, at most five times.
func (s Source) client() *http.Client {
	c := http.Client{}
	if s.Client != nil {
		c = *s.Client
	}
	c.CheckRedirect = func(req *http.Request, via []*http.Request) error {
		if len(via) >= 5 || req.URL.Scheme != "https" || !strings.EqualFold(req.URL.Host, via[0].URL.Host) {
			return fmt.Errorf("%w: %s", ErrRedirect, req.URL.Host)
		}
		return nil
	}
	return &c
}

// readSmall reads the regular file name, refusing one over max bytes.
func readSmall(name string, max int64) ([]byte, error) {
	info, err := os.Lstat(name)
	if err != nil {
		return nil, err
	}
	if !info.Mode().IsRegular() || info.Size() > max {
		return nil, fmt.Errorf("%w: %s", ErrTooLarge, filepath.Base(name))
	}
	f, err := os.Open(name)
	if err != nil {
		return nil, err
	}
	defer f.Close()
	data, err := io.ReadAll(io.LimitReader(f, max+1))
	if err == nil && int64(len(data)) > max {
		err = fmt.Errorf("%w: %s", ErrTooLarge, filepath.Base(name))
	}
	return data, err
}

// writeAtomic writes data to name in dir through a temporary file renamed over it, so a reader never sees half a file.
func writeAtomic(dir, name string, data []byte) error {
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.CreateTemp(dir, "."+name+"-*")
	if err != nil {
		return err
	}
	_, err = f.Write(data)
	if cerr := f.Close(); err == nil {
		err = cerr
	}
	if err == nil {
		err = os.Rename(f.Name(), filepath.Join(dir, name))
	}
	if err != nil {
		os.Remove(f.Name())
	}
	return err
}
