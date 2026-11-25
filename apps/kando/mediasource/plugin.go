package mediasource

import (
	"context"
	"errors"
	"io"
	"maps"
	"strings"
	"sync"
	"time"
	"unicode/utf8"

	"kando/apps/kando/plugins"
)

// The limits of what a plugin's source answers (docs/media-sources.md 11.2): the entries of one folder, and how long Kando reads a file from where its open said before it asks again, by default and at most.
const (
	listMax    = 5000
	openTTL    = 600 * time.Second
	openTTLMax = 86400 * time.Second
)

// The codes a plugin's list, stat or open fails with that Kando reads (docs/media-sources.md 11.2): auth, its server refused the source's secrets, which the user is asked to enter again; not-found, nothing is at the path; unavailable, its server cannot be reached.
const (
	PluginAuth        = "auth"
	PluginNotFound    = "not-found"
	PluginUnavailable = "unavailable"
)

// ErrBadReply is the error of a plugin's list, stat or open that answered with something of another shape than docs/media-sources.md 11.2 says.
var ErrBadReply = errors.New("mediasource: the plugin's answer is not of the shape asked for")

// PluginError is the failure a plugin's export rejected with, by its code (PluginAuth, PluginNotFound, PluginUnavailable or another), with its message.
type PluginError struct {
	Code    string
	Message string
}

func (e *PluginError) Error() string {
	return "mediasource: the plugin failed with " + e.Code + ": " + e.Message
}

// Is makes a PluginError of PluginNotFound ErrNotExist, so a stat of nothing reads as remotefs's does.
func (e *PluginError) Is(target error) bool { return target == ErrNotExist && e.Code == PluginNotFound }

// PluginCall calls the export of the plugin with args and gives back what it answered: what the app gives a plugin's kind to reach its code, which runs on the engine's goroutine. A rejection with a code is a *PluginError.
type PluginCall func(ctx context.Context, export string, args ...any) (any, error)

// PluginKind is the Kind of the media source src the plugin of ID plugin contributes (docs/media-sources.md 11.1), titled and its fields named in a language by the manifest's texts, whose Open is open's.
func PluginKind(plugin string, src plugins.MediaSource, open func(ctx context.Context, s Source, secrets map[string]string) (Driver, error)) Kind {
	text := func(t plugins.Text) func(string) string {
		if t == nil {
			return nil
		}
		return func(locale string) string { return t.In(locale) }
	}
	k := Kind{ID: PluginKindID(plugin, src.ID), Title: text(src.Title), Icon: src.Icon, Origin: Origin{Plugin: plugin}, Open: open}
	for _, f := range src.Fields {
		field := Field{Key: f.Key, Type: FieldType(f.Type), Title: text(f.Title), Hint: text(f.Hint)}
		if a := f.Auth; a != nil {
			field.Auth = &Auth{Basic: a.Basic, Header: a.Header, Format: a.Format, Login: a.Login}
		}
		k.Fields = append(k.Fields, field)
	}
	return k
}

// PluginSourceArg is what a plugin's export is given of the source s of its kind k (docs/media-sources.md 11.2): its id for the plugin, opaque, and the values of its fields that are neither secret nor Kando's own (PinKey).
func PluginSourceArg(k Kind, id string, s Source) map[string]any {
	config := map[string]any{}
	for _, f := range k.Fields {
		if f.Type == FieldSecret {
			continue
		}
		if v, ok := s.Config[f.Key]; ok {
			config[f.Key] = v
		}
	}
	return map[string]any{"id": id, "config": config}
}

// PluginDriver reads a source of a plugin's kind (docs/media-sources.md 11.2, 11.3): List and Stat ask the plugin's list and stat, and Open asks its open where a file is read from, then reads it itself through Fetcher, so a video's bytes never pass through the plugin's code. Where open said is kept for its ttl, and asked again once it ran out or the server refused it (401, 403). Done runs once as it closes, as the app forgets the source's fetches.
type PluginDriver struct {
	Call    PluginCall
	Source  map[string]any
	Fetcher *Fetcher
	Done    func()

	mu     sync.Mutex
	opens  map[string]openTarget
	closed bool
}

// openTarget is where open said a file is read from, until when.
type openTarget struct {
	t     OpenTarget
	until time.Time
}

// List asks the plugin's list(source, dir) what lies in dir: at most 5000 entries, each a name that is one segment of a path, neither . nor .., in valid UTF-8, with a size, a modification time in RFC 3339 and a version; one that is not is left out, as is a name met twice.
func (d *PluginDriver) List(ctx context.Context, dir string) ([]Entry, error) {
	dir, err := CleanPath(dir)
	if err != nil {
		return nil, err
	}
	v, err := d.Call(ctx, plugins.MediaSourceList, d.Source, dir)
	if err != nil {
		return nil, d.why(err)
	}
	items, ok := v.([]any)
	if !ok || len(items) > listMax {
		return nil, ErrBadReply
	}
	seen := map[string]bool{}
	out := make([]Entry, 0, len(items))
	for _, item := range items {
		e, ok := pluginEntry(item)
		if !ok || seen[e.Name] {
			continue
		}
		seen[e.Name] = true
		out = append(out, e)
	}
	return out, nil
}

// Stat asks the plugin's stat(source, path) of p: an entry, or null for nothing there (ErrNotExist).
func (d *PluginDriver) Stat(ctx context.Context, p string) (Entry, error) {
	p, err := CleanPath(p)
	if err != nil {
		return Entry{}, err
	}
	v, err := d.Call(ctx, plugins.MediaSourceStat, d.Source, p)
	if err != nil {
		return Entry{}, d.why(err)
	}
	if v == nil {
		return Entry{}, ErrNotExist
	}
	e, ok := pluginEntry(v)
	if !ok {
		return Entry{}, ErrBadReply
	}
	return e, nil
}

// Open reads p from offset, length bytes or to its end with -1, from where the plugin's open said (Fetcher.Open), asking it again once when the server refuses what it said.
func (d *PluginDriver) Open(ctx context.Context, p string, offset, length int64) (io.ReadCloser, error) {
	p, err := CleanPath(p)
	if err != nil {
		return nil, err
	}
	if p == "" {
		return nil, ErrInvalidPath
	}
	for attempt := 0; ; attempt++ {
		t, err := d.target(ctx, p, attempt > 0)
		if err != nil {
			return nil, err
		}
		r, err := d.Fetcher.Open(ctx, t, offset, length)
		var status ErrFetchStatus
		if attempt == 0 && errors.As(err, &status) && status.Auth() {
			continue
		}
		return r, err
	}
}

// target is where open said p is read from: the one kept while its ttl lasts, unless again, else asked anew.
func (d *PluginDriver) target(ctx context.Context, p string, again bool) (OpenTarget, error) {
	d.mu.Lock()
	kept, ok := d.opens[p]
	d.mu.Unlock()
	if ok && !again && time.Now().Before(kept.until) {
		return kept.t, nil
	}
	v, err := d.Call(ctx, plugins.MediaSourceOpen, d.Source, p)
	if err != nil {
		return OpenTarget{}, err
	}
	t, ok := pluginTarget(v)
	if !ok {
		return OpenTarget{}, ErrBadReply
	}
	d.mu.Lock()
	if d.opens == nil {
		d.opens = map[string]openTarget{}
	}
	d.opens[p] = openTarget{t: t, until: time.Now().Add(t.TTL)}
	d.mu.Unlock()
	return t, nil
}

// why is err, a failure of the plugin's list or stat, or the certificate its server was refused for meanwhile, which is why and what the user can act on (CertError).
func (d *PluginDriver) why(err error) error {
	if cert := d.Fetcher.TakeCertError(); cert != nil {
		return cert
	}
	return err
}

// Close forgets where open said the files are, and runs Done once.
func (d *PluginDriver) Close() error {
	d.mu.Lock()
	done := !d.closed
	d.closed, d.opens = true, nil
	d.mu.Unlock()
	if done && d.Done != nil {
		d.Done()
	}
	return nil
}

// pluginEntry is an entry of a plugin's list or stat, {name, dir, size, modified, version}, and whether it is one: a name that is one segment of a path (no slash, backslash or NUL, neither . nor ..) in valid UTF-8, a size that is a whole number not below 0 (a number as the script engine gives it, plugins.Number), a modified time in RFC 3339 when it has one.
func pluginEntry(v any) (Entry, bool) {
	m, ok := v.(map[string]any)
	if !ok {
		return Entry{}, false
	}
	name, _ := m["name"].(string)
	if name == "" || name == "." || name == ".." || strings.ContainsAny(name, "/\\\x00") || !utf8.ValidString(name) || len(name) > 1024 {
		return Entry{}, false
	}
	e := Entry{Name: name}
	e.Dir, _ = m["dir"].(bool)
	if size, ok := m["size"]; ok && size != nil {
		n, ok := plugins.Number(size)
		if !ok || n < 0 || n != float64(int64(n)) {
			return Entry{}, false
		}
		e.Size = int64(n)
	}
	if modified, ok := m["modified"].(string); ok && modified != "" {
		t, err := time.Parse(time.RFC3339, modified)
		if err != nil {
			return Entry{}, false
		}
		e.Modified = t
	}
	if version, ok := m["version"].(string); ok && len(version) <= 256 {
		e.Version = version
	}
	return e, true
}

// pluginTarget is what a plugin's open answered, {path, query, header, ttl} or {url, header, ttl}, and whether it is that: strings for the path, the url and the values of query and header, and a ttl in seconds, 600 when none and at most 86400.
func pluginTarget(v any) (OpenTarget, bool) {
	m, ok := v.(map[string]any)
	if !ok {
		return OpenTarget{}, false
	}
	var t OpenTarget
	t.Path, _ = m["path"].(string)
	t.URL, _ = m["url"].(string)
	if (t.Path == "") == (t.URL == "") {
		return OpenTarget{}, false
	}
	strs := func(key string) (map[string]string, bool) {
		raw, ok := m[key]
		if !ok || raw == nil {
			return nil, true
		}
		obj, ok := raw.(map[string]any)
		if !ok {
			return nil, false
		}
		out := map[string]string{}
		for k, v := range obj {
			s, ok := v.(string)
			if !ok {
				return nil, false
			}
			out[k] = s
		}
		return out, true
	}
	if t.Query, ok = strs("query"); !ok {
		return OpenTarget{}, false
	}
	if t.Header, ok = strs("header"); !ok {
		return OpenTarget{}, false
	}
	t.TTL = openTTL
	if raw, ok := m["ttl"]; ok && raw != nil {
		n, ok := plugins.Number(raw)
		if !ok || n <= 0 {
			return OpenTarget{}, false
		}
		t.TTL = min(time.Duration(n*float64(time.Second)), openTTLMax)
	}
	return t, true
}

// PluginRequest is a call of kando.source.fetch as the plugin sent it (docs/media-sources.md 11.3), {source, method, path, query, header, body, login}, its source the id the plugin knows it by, and whether it is of that shape.
func PluginRequest(data any) (source string, r FetchRequest, ok bool) {
	m, isMap := data.(map[string]any)
	if !isMap {
		return "", r, false
	}
	source, _ = m["source"].(string)
	r.Method, _ = m["method"].(string)
	if r.Method == "" {
		r.Method = "GET"
	}
	r.Path, _ = m["path"].(string)
	r.Body, _ = m["body"].(string)
	r.Login, _ = m["login"].(bool)
	for _, part := range []struct {
		key string
		to  *map[string]string
	}{{"query", &r.Query}, {"header", &r.Header}} {
		raw, has := m[part.key]
		if !has || raw == nil {
			continue
		}
		obj, isObj := raw.(map[string]any)
		if !isObj {
			return "", r, false
		}
		*part.to = map[string]string{}
		for k, v := range obj {
			s, isStr := v.(string)
			if !isStr {
				return "", r, false
			}
			(*part.to)[k] = s
		}
	}
	return source, r, source != ""
}

// PluginReply is a reply of kando.source.fetch as the plugin is given it: {status, header, body}, its body as text.
func PluginReply(r FetchReply) map[string]any {
	header := map[string]any{}
	for k, v := range r.Header {
		header[k] = v
	}
	return map[string]any{"status": r.Status, "header": header, "body": trimBody(r.Body)}
}

// Clone is f with its maps of its own, as a source's fetches each hold one, so the secrets of one never change another's; the certificate a clone is refused for is f's too (TakeCertError).
func (f *Fetcher) Clone() *Fetcher {
	if f.refused == nil {
		f.refused = &refusal{}
	}
	c := *f
	c.Config, c.Secrets = maps.Clone(f.Config), maps.Clone(f.Secrets)
	return &c
}
