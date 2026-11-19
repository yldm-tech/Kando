// Package ankiconnect is the source of Kando's official AnkiConnect plugin (kando-plugin.json, plugin.tsx, nsgo.yaml), which is not built into the app: .github/scripts/official-plugins.sh signs it with nsgo publish and official.Wrap makes the .kandoplugin of it. Its tests sign this folder as it is with a key they make, stand in for Kando's official root with it, and run the plugin in Kando's own sandbox against a fake AnkiConnect.
package ankiconnect

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"slices"
	"strings"
	"sync"
	"testing"
	"time"

	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/official"
	"kando/apps/kando/plugins/script"

	"github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/tsx"
	"github.com/nsgo-dev/nsgo/tsx/pkg"
	"go.yaml.in/yaml/v3"
)

// source is the plugin's folder, the one this file is in, found from the file's own path rather than the working directory, which nsgo test -target runs the test binary elsewhere than.
func source(t *testing.T) string {
	t.Helper()
	_, file, _, ok := runtime.Caller(0)
	if !ok {
		t.Fatal("no path of the test's source")
	}
	return filepath.Dir(file)
}

// endpoint is the one address the plugin may reach, AnkiConnect's.
const endpoint = "http://127.0.0.1:8765/"

// anki is a fake AnkiConnect: the actions the plugin uses, on an in-memory collection, with the faults a real one has. Every request it hears is kept, by method and URL as the plugin sent it.
type anki struct {
	mu sync.Mutex
	// version is what the version action answers; noPermission makes requestPermission an unsupported action, as AnkiConnect older than 6; permission is what requestPermission answers ("granted" when ""), and apiKey whether it says it requires a key.
	version      int
	noPermission bool
	permission   string
	apiKey       bool
	// wholeBatch fails addNotes as a whole, adding nothing, when one of its notes cannot be added, as newer AnkiConnect does; else it answers null for that one, as older ones do. lying makes canAddNotes say yes to every note, as for a note added from elsewhere in between.
	wholeBatch, lying bool
	// notJSON answers what is not AnkiConnect; delay holds every answer that long, or until the request is given up, as a busy Anki does.
	notJSON bool
	delay   time.Duration

	models  map[string][]string
	decks   map[string]bool
	notes   []note
	media   map[string]string
	heard   []string
	actions []string
	// batches are the sizes of the addNotes calls.
	batches []int
}

// note is a note of the fake collection.
type note struct {
	Deck   string            `json:"deckName"`
	Model  string            `json:"modelName"`
	Fields map[string]string `json:"fields"`
	Tags   []string          `json:"tags"`
}

// newAnki is a fake AnkiConnect of version 6 whose collection has the stock note types, their fields named as a Chinese collection names them, and no notes.
func newAnki() *anki {
	return &anki{version: 6, models: map[string][]string{"Basic": {"正面", "背面"}, "Basic (and reversed card)": {"正面", "背面"}}, decks: map[string]bool{"Default": true}, media: map[string]string{}}
}

// first is the note's first field, which Anki tells duplicates apart by.
func (a *anki) first(n note) string {
	fields := a.models[n.Model]
	if len(fields) == 0 {
		return ""
	}
	return n.Fields[fields[0]]
}

// canAdd reports whether n can be added: its deck and note type exist, its first field is not empty and no note of its type in its deck has it.
func (a *anki) canAdd(n note) bool {
	if !a.decks[n.Deck] || a.models[n.Model] == nil || a.first(n) == "" {
		return false
	}
	return !slices.ContainsFunc(a.notes, func(o note) bool { return o.Deck == n.Deck && o.Model == n.Model && a.first(o) == a.first(n) })
}

func (a *anki) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	a.mu.Lock()
	delay := a.delay
	a.mu.Unlock()
	if delay > 0 {
		select {
		case <-time.After(delay):
		case <-r.Context().Done():
			return
		}
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.notJSON {
		w.Write([]byte("<html>a router's page</html>"))
		return
	}
	var req struct {
		Action  string          `json:"action"`
		Version int             `json:"version"`
		Params  json.RawMessage `json:"params"`
	}
	body, _ := io.ReadAll(r.Body)
	if err := json.Unmarshal(body, &req); err != nil {
		http.Error(w, "bad json", http.StatusBadRequest)
		return
	}
	a.actions = append(a.actions, req.Action)
	result, err := a.do(req.Action, req.Params)
	reply := map[string]any{"result": result, "error": nil}
	if err != nil {
		reply = map[string]any{"result": nil, "error": err.Error()}
	}
	w.Header().Set("Content-Type", "application/json")
	json.NewEncoder(w).Encode(reply)
}

// do runs the action with its params.
func (a *anki) do(action string, raw json.RawMessage) (any, error) {
	var p struct {
		Deck          string   `json:"deck"`
		ModelName     string   `json:"modelName"`
		InOrderFields []string `json:"inOrderFields"`
		Notes         []note   `json:"notes"`
		Note          note     `json:"note"`
		Filename      string   `json:"filename"`
		Data          string   `json:"data"`
	}
	if len(raw) > 0 {
		if err := json.Unmarshal(raw, &p); err != nil {
			return nil, err
		}
	}
	switch action {
	case "requestPermission":
		if a.noPermission {
			return nil, errors.New("unsupported action")
		}
		return map[string]any{"permission": cmpOr(a.permission, "granted"), "requireApikey": a.apiKey, "version": a.version}, nil
	case "version":
		return a.version, nil
	case "createDeck":
		a.decks[p.Deck] = true
		return 1, nil
	case "modelNames":
		var names []string
		for n := range a.models {
			names = append(names, n)
		}
		slices.Sort(names)
		return names, nil
	case "modelFieldNames":
		if a.models[p.ModelName] == nil {
			return nil, fmt.Errorf("model was not found: %s", p.ModelName)
		}
		return a.models[p.ModelName], nil
	case "createModel":
		if a.models[p.ModelName] != nil {
			return nil, errors.New("Model name already exists")
		}
		a.models[p.ModelName] = p.InOrderFields
		return map[string]any{"name": p.ModelName}, nil
	case "canAddNotes":
		out := make([]bool, len(p.Notes))
		for i, n := range p.Notes {
			out[i] = a.lying || a.canAdd(n)
		}
		return out, nil
	case "storeMediaFile":
		if _, err := base64.StdEncoding.DecodeString(p.Data); err != nil || p.Filename == "" {
			return nil, errors.New("bad media")
		}
		a.media[p.Filename] = p.Data
		return p.Filename, nil
	case "addNotes":
		a.batches = append(a.batches, len(p.Notes))
		if a.wholeBatch {
			for _, n := range p.Notes {
				if !a.canAdd(n) {
					return nil, errors.New("['cannot create note because it is a duplicate']")
				}
			}
		}
		out := make([]any, len(p.Notes))
		for i, n := range p.Notes {
			if a.canAdd(n) {
				a.notes = append(a.notes, n)
				out[i] = len(a.notes)
			}
		}
		return out, nil
	case "addNote":
		if !a.canAdd(p.Note) {
			return nil, errors.New("cannot create note because it is a duplicate")
		}
		a.notes = append(a.notes, p.Note)
		return len(a.notes), nil
	}
	return nil, errors.New("unsupported action")
}

// counts are how many notes and media files the collection has, and how many times action ran.
func (a *anki) counts(action string) (notes, media, ran int) {
	a.mu.Lock()
	defer a.mu.Unlock()
	for _, x := range a.actions {
		if x == action {
			ran++
		}
	}
	return len(a.notes), len(a.media), ran
}

func cmpOr(s, def string) string {
	if s == "" {
		return def
	}
	return s
}

// network stands in for the network the plugin's fetches go through (tsx.WithFetcher): every request is kept as the plugin sent it, by method and URL, and given to the fake AnkiConnect of the moment, served by a real HTTP server whatever host and port the request names; with no fake, nothing listens, as when Anki is not running.
type network struct {
	mu    sync.Mutex
	anki  *anki
	heard []string
	srv   *httptest.Server
	tr    *http.Transport
}

func newNetwork(t *testing.T) *network {
	n := &network{}
	n.srv = httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		n.mu.Lock()
		a := n.anki
		n.mu.Unlock()
		a.ServeHTTP(w, r)
	}))
	t.Cleanup(n.srv.Close)
	addr := n.srv.Listener.Addr().String()
	n.tr = &http.Transport{DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
		return (&net.Dialer{}).DialContext(ctx, network, addr)
	}}
	t.Cleanup(n.tr.CloseIdleConnections)
	return n
}

// use makes a the AnkiConnect that answers, nil for none.
func (n *network) use(a *anki) {
	n.mu.Lock()
	defer n.mu.Unlock()
	n.anki = a
	n.heard = nil
}

// fetch is the plugin's fetches' Fetcher.
func (n *network) fetch(req *http.Request) (*http.Response, error) {
	n.mu.Lock()
	n.heard = append(n.heard, req.Method+" "+req.URL.String())
	a := n.anki
	n.mu.Unlock()
	if a == nil {
		return nil, &net.OpError{Op: "dial", Net: "tcp", Err: errors.New("connection refused")}
	}
	return n.tr.RoundTrip(req)
}

// requests are the requests heard since use.
func (n *network) requests() []string {
	n.mu.Lock()
	defer n.mu.Unlock()
	return slices.Clone(n.heard)
}

// engine is a graph of its own on a goroutine of its own, the engine's in the app, as plugins/script's tests run one.
type engine struct {
	root  *core.Scope
	posts chan func()
}

func newEngine(t *testing.T) *engine {
	t.Helper()
	e := &engine{posts: make(chan func(), 1<<14)}
	ready := make(chan struct{})
	go func() {
		e.root = core.NewRoot(core.RootOptions{Post: e.post})
		// TODO(nsgo#838): tsx.Load fails with a nil pointer on a graph without a tree, which its documentation does not say; drop the tree once Load needs none https://github.com/nsgo-dev/nsgo/issues/838
		core.NewTree(e.root, nil)
		close(ready)
		for fn := range e.posts {
			if fn == nil {
				e.root.Dispose()
				return
			}
			fn()
		}
	}()
	<-ready
	t.Cleanup(func() {
		done := make(chan struct{})
		e.posts <- func() { close(done) }
		<-done
		e.posts <- nil
	})
	return e
}

func (e *engine) post(fn func()) { e.posts <- fn }

func (e *engine) do(fn func()) {
	done := make(chan struct{})
	e.post(func() {
		defer close(done)
		fn()
	})
	<-done
}

// sounds is the host's kando.sound.read: the sound of each saved line it has, by id, refusing the others as Kando does a line with no sound; past limit calls, it refuses with quota, as Kando does a plugin's 61st host call in a minute. calls counts the calls.
type sounds struct {
	mu    sync.Mutex
	clips map[string]string
	limit int
	calls int
}

func (s *sounds) handle(c *core.HostCall) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.calls++
	if s.limit > 0 && s.calls > s.limit {
		c.Reject(&core.Error{Code: "quota", Message: "quota"})
		return
	}
	m, _ := c.Data.(map[string]any)
	id, _ := m["saved"].(string)
	clip, ok := s.clips[id]
	if !ok {
		c.Reject(&core.Error{Code: "not-found", Message: "not-found"})
		return
	}
	c.Resolve(map[string]any{"mime": "audio/mp4", "base64": clip, "durationMs": 1200})
}

func (s *sounds) count() int {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.calls
}

// plugin is the AnkiConnect plugin signed and installed as Kando installs it, running on its own engine.
type plugin struct {
	e      *engine
	r      *script.Runner
	v      script.Verified
	net    *network
	sounds *sounds
	// soundless is what the last export answered of lines sent without their sound; stopped are the reasons the runner stopped the plugin for, which no test expects (engine's goroutine).
	soundless int
	stopped   []string
}

// signed signs this folder as it is with a key the test makes, which stands in for Kando's official root (script.PackFolder certifies it for the publisher kando), wraps the package as the release does (official.Wrap) and stages the .kandoplugin as Kando installs it (plugins.Stage), verifying it as an official plugin (script.Verify) and reading what its code says of itself (script.Probe).
func signed(t *testing.T) *plugin {
	t.Helper()
	secret, public, err := script.NewDevKey()
	if err != nil {
		t.Fatal(err)
	}
	key, _, err := script.DevKey(secret)
	if err != nil {
		t.Fatal(err)
	}
	packed := t.TempDir()
	if err := script.PackFolder(source(t), packed, key); err != nil {
		t.Fatal(err)
	}
	manifest, err := os.ReadFile(filepath.Join(source(t), plugins.ManifestName))
	if err != nil {
		t.Fatal(err)
	}
	pkgPath, _ := filepath.Glob(filepath.Join(packed, "*.nsgopkg"))
	if len(pkgPath) != 1 {
		t.Fatalf("packed %v", pkgPath)
	}
	data, err := os.ReadFile(pkgPath[0])
	if err != nil {
		t.Fatal(err)
	}
	if _, err := official.VerifyPackage(data, public); err != nil {
		t.Fatal(err)
	}
	zipped, id, version, err := official.Wrap(manifest, filepath.Base(pkgPath[0]), data)
	if err != nil {
		t.Fatal(err)
	}
	if id != "kando.ankiconnect" || official.FileName(id, version) != "kando.ankiconnect-"+version+".kandoplugin" {
		t.Fatalf("wrapped %s %s", id, version)
	}
	file := filepath.Join(t.TempDir(), official.FileName(id, version))
	if err := os.WriteFile(file, zipped, 0o644); err != nil {
		t.Fatal(err)
	}
	root := filepath.Join(t.TempDir(), "plugins")
	_, p, err := plugins.Stage(root, file)
	if err != nil {
		t.Fatal(err)
	}
	roots := script.Roots{Official: public}
	trust, err := script.OpenTrust(root, roots)
	if err != nil {
		t.Fatal(err)
	}
	v, err := script.Verify(trust, roots, p)
	if err != nil {
		t.Fatal(err)
	}
	if v.Source != plugins.SourceOfficial {
		t.Fatalf("the plugin comes from %q, not Kando", v.Source)
	}
	e := newEngine(t)
	e.do(func() { err = script.Probe(e.root, v) })
	if err != nil {
		t.Fatalf("what the code says of itself is not its manifest: %v", err)
	}
	out := &plugin{e: e, v: v, net: newNetwork(t), sounds: &sounds{clips: map[string]string{}}}
	// The runner's deadline and pauses are Kando's (script.DefaultWatch); only the heartbeat is lenient, as the race detector slows the engine down.
	w := script.DefaultWatch
	w.Stall = time.Minute
	out.r = &script.Runner{Post: e.post, Watch: w, Stopped: func(id, hash, reason string) { out.stopped = append(out.stopped, reason) }}
	t.Cleanup(func() {
		var stopped []string
		e.do(func() { stopped = out.stopped })
		if len(stopped) > 0 {
			t.Errorf("the runner stopped the plugin: %v", stopped)
		}
	})
	return out
}

// load runs the plugin with grant, in Kando's sandbox (script.Load), its fetches going through p.net and its host answering kando.sound.read with p.sounds.
func (p *plugin) load(t *testing.T, grant plugins.Grant) *script.Plugin {
	t.Helper()
	host := core.NewHost()
	host.Handle(string(plugins.PermSoundRead), p.sounds.handle)
	var pl *script.Plugin
	var err error
	p.e.do(func() {
		cx := p.e.root.Child()
		tsx.WithFetcher(func(cx *core.Scope) core.Node {
			pl, err = script.Load(cx, p.v, grant, host)
			return nil
		}, p.net.fetch)(cx)
	})
	if err != nil {
		t.Fatal(err)
	}
	return pl
}

// export calls exportNotes(items, settings) and returns how many it sent and skipped, or its error.
func (p *plugin) export(t *testing.T, pl *script.Plugin, items []map[string]any, settings map[string]any) (sent, skipped int, err error) {
	t.Helper()
	args := make([]any, len(items))
	for i, it := range items {
		args[i] = it
	}
	done := make(chan struct{})
	var v any
	p.e.post(func() {
		p.r.Call(pl, "exportNotes", []any{args, settings}, func(got any, e error) {
			v, err = got, e
			close(done)
		})
	})
	select {
	case <-done:
	case <-time.After(time.Minute):
		t.Fatal("exportNotes did not answer within a minute")
	}
	if err != nil {
		return 0, 0, err
	}
	s, k, q, ok := plugins.Exported(v)
	if !ok {
		t.Fatalf("exportNotes answered %v", v)
	}
	p.soundless = q
	return s, k, nil
}

// granted is everything the plugin declares, as the user grants it on its details.
func (p *plugin) granted() plugins.Grant {
	return plugins.Grant{Perms: p.v.Manifest.Declared().Perms, Network: p.v.Manifest.Declared().Network}
}

// line is a saved line as Kando gives it to an export target.
func line(id, original, translation string) map[string]any {
	return map[string]any{"kind": "line", "id": id, "film": "f_0123456789abcdef", "title": "Talk", "original": original, "translation": translation, "note": "", "startMs": 1000, "endMs": 2500, "savedAt": 1}
}

// word is a word of the vocabulary as Kando gives it to an export target.
func word(text, reading, meaning string) map[string]any {
	return map[string]any{"kind": "word", "id": "w-" + text, "text": text, "spoken": "ja", "reading": reading, "meaning": meaning, "language": "zh-Hans", "context": "猫が好き", "contextTranslation": "喜欢猫", "film": "f_0123456789abcdef", "title": "Talk", "savedAt": 1}
}

// wantOnlyAnki fails t unless every request the plugin made since use went to AnkiConnect's endpoint with POST, the one rule it is granted.
func wantOnlyAnki(t *testing.T, n *network) {
	t.Helper()
	heard := n.requests()
	if len(heard) == 0 {
		t.Fatal("the plugin reached nothing")
	}
	for _, h := range heard {
		if h != "POST "+endpoint {
			t.Fatalf("the plugin sent %q, beyond its one rule", h)
		}
	}
}

// TestPluginAnkiConnectExports checks the export target end to end in Kando's sandbox: on a collection without Kando's note type it makes it and the deck, sends the lines with their sound stored in Anki's media and the words with their reading and meaning, as HTML, under the tags of the settings; a line with no sound goes without; the same text twice and an empty one are skipped; sending again skips what Anki has, without storing a sound again; and every request went to AnkiConnect's one endpoint.
func TestPluginAnkiConnectExports(t *testing.T) {
	p := signed(t)
	pl := p.load(t, p.granted())
	a := newAnki()
	p.net.use(a)
	p.sounds.clips["l1"] = base64.StdEncoding.EncodeToString([]byte("m4a bytes"))
	items := []map[string]any{
		line("l1", "猫が<好き>&", "我喜欢猫"),
		line("l2", "おはよう", "早上好"),
		line("l3", "おはよう", "早上好（又一次）"),
		line("l4", "", "nothing to show"),
		word("猫", "ねこ", "cat"),
	}
	settings := map[string]any{"deck": "Japanese::Kando", "noteType": "kando", "sound": true, "tags": "kando  anime,jp"}
	sent, skipped, err := p.export(t, pl, items, settings)
	if err != nil || sent != 3 || skipped != 2 {
		t.Fatalf("sent %d, skipped %d, %v", sent, skipped, err)
	}
	wantOnlyAnki(t, p.net)
	a.mu.Lock()
	if !slices.Equal(a.models["Kando"], []string{"Text", "Meaning", "Reading", "Context", "Sound", "Source"}) || !a.decks["Japanese::Kando"] {
		t.Errorf("the note type %v, decks %v", a.models["Kando"], a.decks)
	}
	if len(a.notes) != 3 {
		t.Fatalf("notes %+v", a.notes)
	}
	l1, l2, w := a.notes[0], a.notes[1], a.notes[2]
	if l1.Fields["Text"] != "猫が&lt;好き&gt;&amp;" || l1.Fields["Meaning"] != "我喜欢猫" || l1.Fields["Sound"] != "[sound:kando-l1.m4a]" || l1.Fields["Source"] != "Talk" || l1.Deck != "Japanese::Kando" || l1.Model != "Kando" {
		t.Errorf("the first line %+v", l1)
	}
	if !slices.Equal(l1.Tags, []string{"kando", "anime", "jp"}) {
		t.Errorf("tags %v", l1.Tags)
	}
	if l2.Fields["Sound"] != "" || l2.Fields["Text"] != "おはよう" {
		t.Errorf("a line with no sound %+v", l2)
	}
	if w.Fields["Text"] != "猫" || w.Fields["Reading"] != "ねこ" || w.Fields["Meaning"] != "cat" || w.Fields["Context"] != "猫が好き<br>喜欢猫" {
		t.Errorf("the word %+v", w)
	}
	if got := a.media["kando-l1.m4a"]; got != p.sounds.clips["l1"] || len(a.media) != 1 {
		t.Errorf("media %v", a.media)
	}
	a.mu.Unlock()
	calls := p.sounds.count()

	// Sent again, Anki has them all.
	sent, skipped, err = p.export(t, pl, items, settings)
	if err != nil || sent != 0 || skipped != 5 {
		t.Fatalf("again: sent %d, skipped %d, %v", sent, skipped, err)
	}
	if notes, _, stored := a.counts("storeMediaFile"); notes != 3 || stored != 1 {
		t.Errorf("again: %d notes, the sound stored %d times", notes, stored)
	}
	if p.sounds.count() != calls {
		t.Error("the sound of a line Anki has was read again")
	}
	if _, _, made := a.counts("createModel"); made != 1 {
		t.Errorf("the note type was made %d times", made)
	}
}

// TestPluginAnkiConnectBasic checks the stock note types: the text goes in the first field and the rest, the sound included, in the second, whatever the collection named them; a note type the collection does not have is a coded failure that says so.
func TestPluginAnkiConnectBasic(t *testing.T) {
	p := signed(t)
	pl := p.load(t, p.granted())
	a := newAnki()
	p.net.use(a)
	p.sounds.clips["l1"] = base64.StdEncoding.EncodeToString([]byte("m4a"))
	sent, skipped, err := p.export(t, pl, []map[string]any{line("l1", "猫", "cat"), word("犬", "いぬ", "dog")}, map[string]any{"deck": "Kando", "noteType": "basic-reversed", "sound": true, "tags": ""})
	if err != nil || sent != 2 || skipped != 0 {
		t.Fatalf("sent %d, skipped %d, %v", sent, skipped, err)
	}
	a.mu.Lock()
	n0, n1 := a.notes[0], a.notes[1]
	if n0.Model != "Basic (and reversed card)" || n0.Fields["正面"] != "猫" || n0.Fields["背面"] != "cat<br>[sound:kando-l1.m4a]<br>Talk" || !slices.Equal(n0.Tags, []string{"kando"}) {
		t.Errorf("the line %+v", n0)
	}
	if n1.Fields["正面"] != "犬" || n1.Fields["背面"] != "いぬ<br>dog<br>猫が好き<br>喜欢猫<br>Talk" {
		t.Errorf("the word %+v", n1)
	}
	if a.models["Kando"] != nil {
		t.Error("Kando's note type was made though Basic was chosen")
	}
	delete(a.models, "Basic")
	a.mu.Unlock()
	_, _, err = p.export(t, pl, []map[string]any{line("l2", "鳥", "bird")}, map[string]any{"noteType": "basic"})
	if core.ErrorCode(err) != "anki-note-type" || !strings.Contains(err.Error(), `"Basic"`) {
		t.Fatalf("no Basic: %v", err)
	}
}

// TestPluginAnkiConnectBatches checks that the notes go to Anki at most a hundred to an addNotes, and that a newer AnkiConnect that fails addNotes as a whole, for a note it said it could add, still gets the others, one by one, the one it refused counted as skipped.
func TestPluginAnkiConnectBatches(t *testing.T) {
	p := signed(t)
	pl := p.load(t, p.granted())
	a := newAnki()
	p.net.use(a)
	var items []map[string]any
	for i := range 250 {
		items = append(items, word(fmt.Sprintf("w%03d", i), "", "m"))
	}
	sent, skipped, err := p.export(t, pl, items, map[string]any{"noteType": "kando"})
	if err != nil || sent != 250 || skipped != 0 {
		t.Fatalf("sent %d, skipped %d, %v", sent, skipped, err)
	}
	a.mu.Lock()
	if !slices.Equal(a.batches, []int{100, 100, 50}) {
		t.Errorf("addNotes took %v", a.batches)
	}
	a.wholeBatch, a.lying = true, true
	a.mu.Unlock()
	sent, skipped, err = p.export(t, pl, []map[string]any{word("w000", "", "m"), word("new1", "", "m"), word("new2", "", "m")}, map[string]any{"noteType": "kando"})
	if err != nil || sent != 2 || skipped != 1 {
		t.Fatalf("a batch refused as a whole: sent %d, skipped %d, %v", sent, skipped, err)
	}
	if notes, _, one := a.counts("addNote"); notes != 252 || one != 3 {
		t.Errorf("%d notes, addNote ran %d times", notes, one)
	}
}

// TestPluginAnkiConnectFailures checks the failures the user can act on, each a coded error Kando shows under the plugin's name and does not count against it: Anki not running, AnkiConnect refusing Kando, asking for an API key, too old to have requestPermission or speaking an older version, and something else answering on the port; none of them adds a note.
func TestPluginAnkiConnectFailures(t *testing.T) {
	p := signed(t)
	pl := p.load(t, p.granted())
	items := []map[string]any{line("l1", "猫", "cat")}
	for _, tc := range []struct {
		name  string
		anki  func(a *anki)
		code  string
		gone  bool
		words string
	}{
		{name: "Anki not running", gone: true, code: "anki-unreachable", words: "not running"},
		{name: "refused", anki: func(a *anki) { a.permission = "denied" }, code: "anki-denied"},
		{name: "an API key", anki: func(a *anki) { a.apiKey = true }, code: "anki-api-key"},
		{name: "too old to ask", anki: func(a *anki) { a.noPermission = true }, code: "anki-version"},
		{name: "version 5", anki: func(a *anki) { a.version = 5 }, code: "anki-version", words: "version 5"},
		{name: "not AnkiConnect", anki: func(a *anki) { a.notJSON = true }, code: "anki-bad-reply"},
	} {
		t.Run(tc.name, func(t *testing.T) {
			a := newAnki()
			if tc.anki != nil {
				tc.anki(a)
			}
			if tc.gone {
				p.net.use(nil)
			} else {
				p.net.use(a)
			}
			_, _, err := p.export(t, pl, items, map[string]any{})
			var coded *core.Error
			if !errors.As(err, &coded) || coded.Code != tc.code || !strings.Contains(coded.Message, tc.words) {
				t.Fatalf("%v, want code %s", err, tc.code)
			}
			wantOnlyAnki(t, p.net)
			if notes, _, _ := a.counts(""); notes != 0 {
				t.Errorf("%d notes added", notes)
			}
		})
	}
	var closed bool
	p.e.do(func() { closed = pl.Closed() })
	if closed {
		t.Fatal("the failures stopped the plugin")
	}
}

// TestPluginAnkiConnectGrant checks that the plugin reaches only what the user granted: without the network it is refused before any request leaves, with a code that says so; without kando.sound.read it sends the notes without sound and stops asking after the first refusal; once Kando refuses for its quota, the rest of the lines go without asking.
func TestPluginAnkiConnectGrant(t *testing.T) {
	p := signed(t)
	a := newAnki()
	p.net.use(a)
	offline := p.load(t, plugins.Grant{Perms: p.granted().Perms})
	_, _, err := p.export(t, offline, []map[string]any{line("l1", "猫", "cat")}, map[string]any{})
	if core.ErrorCode(err) != "no-network" {
		t.Fatalf("without the network: %v", err)
	}
	if heard := p.net.requests(); len(heard) != 0 {
		t.Fatalf("a request left without the network granted: %q", heard)
	}

	silent := p.load(t, plugins.Grant{Network: p.granted().Network})
	p.sounds.clips["l1"] = "bTRh"
	p.sounds.clips["l2"] = "bTRh"
	sent, _, err := p.export(t, silent, []map[string]any{line("l1", "猫", "cat"), line("l2", "犬", "dog")}, map[string]any{"sound": true})
	if err != nil || sent != 2 || p.soundless != 2 {
		t.Fatalf("without the sound: sent %d, %d without sound, %v", sent, p.soundless, err)
	}
	wantOnlyAnki(t, p.net)
	if _, media, _ := a.counts(""); p.sounds.count() != 0 || media != 0 {
		t.Errorf("the sound was read without kando.sound.read: %d calls, %d media files", p.sounds.count(), media)
	}

	full := p.load(t, p.granted())
	p.sounds.mu.Lock()
	p.sounds.limit = 1
	for _, id := range []string{"l3", "l4", "l5"} {
		p.sounds.clips[id] = "bTRh"
	}
	p.sounds.mu.Unlock()
	sent, _, err = p.export(t, full, []map[string]any{line("l3", "鳥", "bird"), line("l4", "魚", "fish"), line("l5", "馬", "horse")}, map[string]any{"sound": true})
	if err != nil || sent != 3 || p.soundless != 2 {
		t.Fatalf("over the quota: sent %d, %d without sound, %v", sent, p.soundless, err)
	}
	if _, media, _ := a.counts(""); p.sounds.count() != 2 || media != 1 {
		t.Errorf("over the quota: %d sound calls, %d media files", p.sounds.count(), media)
	}

	// The setting off reads no sound at all.
	p.sounds.mu.Lock()
	p.sounds.limit = 0
	p.sounds.mu.Unlock()
	before := p.sounds.count()
	if _, _, err := p.export(t, full, []map[string]any{line("l6", "牛", "cow")}, map[string]any{"sound": false}); err != nil || p.sounds.count() != before || p.soundless != 0 {
		t.Errorf("sound off: %v, %d calls, %d without sound", err, p.sounds.count()-before, p.soundless)
	}
}

// TestPluginAnkiConnectManyLines checks an export of more lines than Kando's quota of host calls a minute allows (60): Kando meters a sound read for an export against the export (docs/plugin-system.md 5.3), so every one of 200 lines goes in with its sound and none is reported without it.
func TestPluginAnkiConnectManyLines(t *testing.T) {
	p := signed(t)
	pl := p.load(t, p.granted())
	a := newAnki()
	p.net.use(a)
	var items []map[string]any
	for i := range 200 {
		id := fmt.Sprintf("l%03d", i)
		p.sounds.clips[id] = "bTRh"
		items = append(items, line(id, "line "+id, "行"))
	}
	sent, skipped, err := p.export(t, pl, items, map[string]any{"sound": true})
	if err != nil || sent != 200 || skipped != 0 || p.soundless != 0 {
		t.Fatalf("sent %d, skipped %d, %d without sound, %v", sent, skipped, p.soundless, err)
	}
	if _, media, stored := a.counts("storeMediaFile"); media != 200 || stored != 200 || p.sounds.count() != 200 {
		t.Errorf("%d media files, stored %d times, %d sound calls", media, stored, p.sounds.count())
	}
}

// TestPluginAnkiConnectSlowAnki checks a busy Anki that takes 12 s to answer each request: the export gives up a request it has no time left for (abort), within the time Kando's runner allows a call, with a coded anki-timeout that says what went in, so the runner never stops the plugin as slow (the plugin's cleanup checks it).
func TestPluginAnkiConnectSlowAnki(t *testing.T) {
	if testing.Short() {
		t.Skip("waits on a slow Anki for most of a minute")
	}
	p := signed(t)
	pl := p.load(t, p.granted())
	a := newAnki()
	a.delay = 12 * time.Second
	p.net.use(a)
	start := time.Now()
	_, _, err := p.export(t, pl, []map[string]any{line("l1", "猫", "cat")}, map[string]any{})
	took := time.Since(start)
	var coded *core.Error
	if !errors.As(err, &coded) || coded.Code != "anki-timeout" || !strings.Contains(coded.Message, "0 notes went in") {
		t.Fatalf("%v", err)
	}
	if took > script.DefaultWatch.Deadline-5*time.Second {
		t.Errorf("the export took %v, near the runner's deadline of %v", took, script.DefaultWatch.Deadline)
	}
	var closed bool
	p.e.do(func() { closed = pl.Closed() })
	if closed {
		t.Fatal("the plugin was stopped")
	}
}

// TestPluginAnkiConnectPublishes checks that nsgo.yaml, which go tool nsgo publish signs the package from, says what kando-plugin.json says: the app ID Kando requires of the id, the version, and exactly the permissions and HTTP rules it declares, so the package it signs is the one Kando's checks expect (script.Verify refuses a package that signed anything else).
func TestPluginAnkiConnectPublishes(t *testing.T) {
	data, err := os.ReadFile(filepath.Join(source(t), "nsgo.yaml"))
	if err != nil {
		t.Fatal(err)
	}
	var y struct {
		Name         string `yaml:"name"`
		ID           string `yaml:"id"`
		Version      string `yaml:"version"`
		Capabilities struct {
			Host    []string `yaml:"host"`
			Network struct {
				HTTP []struct {
					Host         string   `yaml:"host"`
					Port         int      `yaml:"port"`
					Schemes      []string `yaml:"schemes"`
					Methods      []string `yaml:"methods"`
					Paths        []string `yaml:"paths"`
					PathPrefixes []string `yaml:"path_prefixes"`
				} `yaml:"http"`
			} `yaml:"network"`
		} `yaml:"capabilities"`
	}
	dec := yaml.NewDecoder(strings.NewReader(string(data)))
	dec.KnownFields(true)
	if err := dec.Decode(&y); err != nil {
		t.Fatal(err)
	}
	raw, err := os.ReadFile(filepath.Join(source(t), plugins.ManifestName))
	if err != nil {
		t.Fatal(err)
	}
	m, err := plugins.Parse(raw)
	if err != nil {
		t.Fatal(err)
	}
	if y.Name == "" || y.ID != script.AppIDPrefix+m.ID || y.Version != m.Version {
		t.Errorf("nsgo.yaml is %q %s %s, the manifest %s %s", y.Name, y.ID, y.Version, m.ID, m.Version)
	}
	d := m.Declared()
	var perms []string
	for _, p := range d.Perms {
		perms = append(perms, string(p))
	}
	if !slices.Equal(sorted(y.Capabilities.Host), sorted(perms)) {
		t.Errorf("nsgo.yaml signs the permissions %v, the manifest declares %v", y.Capabilities.Host, perms)
	}
	var rules []pkg.HTTPRule
	for _, r := range y.Capabilities.Network.HTTP {
		rules = append(rules, pkg.HTTPRule{Host: r.Host, Port: r.Port, Schemes: r.Schemes, Methods: r.Methods, Paths: r.Paths, PathPrefixes: r.PathPrefixes})
	}
	got, _ := json.Marshal(rules)
	want, _ := json.Marshal(d.Network)
	if string(got) != string(want) {
		t.Errorf("nsgo.yaml signs the rules %s, the manifest declares %s", got, want)
	}
	if string(want) != `[{"host":"127.0.0.1","methods":["POST"],"schemes":["http"],"port":8765,"paths":["/"]}]` {
		t.Errorf("the plugin reaches %s, more than AnkiConnect's endpoint", want)
	}
}

func sorted(s []string) []string {
	s = slices.Clone(s)
	slices.Sort(s)
	return s
}
