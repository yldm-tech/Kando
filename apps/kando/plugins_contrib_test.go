package main

import (
	"context"
	"crypto/tls"
	"encoding/base64"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"kando/apps/kando/films"
	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/lookup"
	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/script"
	"kando/apps/kando/settings"
	"kando/apps/kando/subexport"
	"kando/packages/media"
	"kando/packages/study"
	psync "kando/packages/sync"

	ns "github.com/nsgo-dev/nsgo/core"
	"github.com/nsgo-dev/nsgo/nsgotest"
	"github.com/nsgo-dev/nsgo/tsx/pkg"
)

// pluginContribFields are the fields of the test plugin acme.wiki: a lookup source, a subtitle format, an export target and an annotator, the AI service, the sound of a line and a DeepL credential.
const pluginContribFields = `"capabilities": {"host": ["kando.llm.chat", "kando.sound.read", "kando.net.credentialFetch"]},
  "contributes": {
    "lookups": [{"id": "wiki", "export": "lookUp", "label": {"en": "Wiki", "zh-Hans": "维基"}}],
    "subtitleFormats": [{"id": "lrc", "export": "exportSubtitles", "extension": "lrc", "label": {"en": "LRC lyrics", "zh-Hans": "LRC 歌词"}}],
    "exporters": [{"id": "notes", "export": "exportNotes", "takes": ["lines", "words"], "label": {"en": "Wiki cards", "zh-Hans": "维基卡片"}}, {"id": "slow", "export": "exportSlow", "takes": ["lines"], "label": {"en": "Never done", "zh-Hans": "永远在发"}}],
    "annotators": [{"id": "first", "export": "annotate", "label": {"en": "First two", "zh-Hans": "前两个字"}}]
  },
  "credentials": [{"id": "deepl", "title": {"en": "DeepL API key", "zh-Hans": "DeepL API 密钥"}, "header": "Authorization", "format": "DeepL-Auth-Key {secret}", "hosts": ["api.deepl.com"], "methods": ["POST"], "pathPrefixes": ["/v2/translate"]}]`

// pluginContribCode is acme.wiki's code: each contribution answers in a way the test can tell, and each host call says what it answered or the code it was refused with.
const pluginContribCode = `import { host, Secrets } from "nsgo"
async function attempt(name, data, timeout) {
  try { return await host.call(name, data || {}, { timeout: timeout || 150000 }) } catch (e) { return "refused " + e.code }
}
export async function lookUp(q) {
  if (q.word === "nothing") return null
  return { word: q.word, reading: "wi-ki", meaning: "from the wiki: " + q.line + " in " + q.language }
}
export async function exportSubtitles(id, cues) {
  return id + "\n" + cues.map((c) => c.startMs + "-" + c.endMs + " " + c.lines.map((l) => l.text).join("/")).join("\n")
}
export async function exportNotes(items, settings) {
  if (items.some((i) => i.original === "fail")) {
    const e = new Error("Anki is not\u202e running.\nStart it.")
    e.code = "anki-unreachable"
    throw e
  }
  if (items.some((i) => i.original === "passed on")) {
    const e = new Error("quota")
    e.code = "quota"
    throw e
  }
  return { sent: items.filter((i) => i.kind === "line").length, skipped: items.filter((i) => i.kind === "word").length, soundless: items.filter((i) => i.original === "quiet").length }
}
export function exportSlow() { return new Promise(() => {}) }
export async function loopFetch() {
  for (let i = 0; i < 1000; i++) await attempt("kando.net.credentialFetch", { credential: "deepl", method: "POST", url: "https://api.deepl.com/v2/translate/slow", header: {}, body: "" })
  return "never stopped"
}
export async function annotate(lang, lines) {
  return lines.map((l) => (l.length >= 2 ? [{ start: 0, end: 2, ruby: "wk" }, { start: 0, end: 99, tone: "warn" }] : []))
}
export function chat(maxTokens, timeout) { return attempt("kando.llm.chat", { system: "be short", user: "hi", maxTokens }, timeout) }
export function sound(data) { return attempt("kando.sound.read", data) }
export function fetchIt(method, url, header) { return attempt("kando.net.credentialFetch", { credential: "deepl", method, url, header: header || { "Content-Type": "application/json" }, body: "{}" }) }
export async function secret() { try { return await Secrets.get("credential.deepl") } catch (e) { return "refused " + e.code } }
`

// contribApp is the film "talk" open in an app whose plugins folder holds acme.wiki, granted everything it declares and turned on, with tools as the pipeline; key stands in for Kando's official root.
func contribApp(t *testing.T, tools media.Tools) (*app, *nsgotest.App, chan func(), films.Film, string) {
	t.Helper()
	key := newPluginsKey(t)
	dir := pluginScriptFolder(t, key, "acme.wiki", "1.0.0", pluginContribFields, pluginContribCode)
	a, tt, updates, item := playerFilm(t, 12, tools, nsgotest.PendingAlerts(), nsgotest.Secrets(map[string]string{}))
	root := filepath.Join(t.TempDir(), "plugins")
	pluginsScriptInstalled(t, root, dir)
	a.plugins.root = root
	a.plugins.script.official = key.public
	a.plugins.script.watch = script.Watch{Beat: 100 * time.Millisecond, Stall: time.Minute, Stalls: 3, Deadline: time.Minute}
	a.pluginsScan()
	waitUntil(t, tt, a, updates, func() bool {
		return a.plugins.scanned && !a.plugins.busy && !a.plugins.inboxBusy && a.plugins.script.verified != nil
	})
	scriptsGrant(t, a, tt, updates, "acme.wiki", string(plugins.PermLLMChat), string(plugins.PermSoundRead), string(plugins.PermCredFetch))
	playerOpenFilm(t, a, tt, updates, item)
	playerLoadFilm(tt, a, 12*time.Second)
	return a, tt, updates, item, root
}

// TestPluginContribLookup checks a lookup source of a script plugin: it is asked after the dictionary packs and before the AI service, given the word, its line and the language to answer in; its entry shows on the card named by the plugin; a word it does not have goes on to the AI service.
func TestPluginContribLookup(t *testing.T) {
	srv, _ := fakeLLM(t, `{"word":"nothing","meaning":"无"}`)
	a, tt, updates, _, _ := contribApp(t, media.Tools{Translator: media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m1"}})
	look := func(word string) vocabLookup {
		apply(tt, a, func() { a.LookUpWord(0, word) })
		scriptsSettle(t, a, tt, updates, func() bool { return !a.vocab.lookup.busy })
		return a.vocab.lookup
	}
	l := look("Hello")
	if l.failed || l.source != "plugin:acme.wiki:wiki" || !strings.HasPrefix(l.entry.Meaning, "from the wiki: Hello.") || l.entry.Reading != "wi-ki" {
		t.Fatalf("the lookup %+v", l)
	}
	refresh(tt, a)
	if got := tt.Find("id:film-lookup-source").Text; got != testlocale.Messages().VocabLookupSourcePlugin("Anki 发送器", "维基") {
		t.Errorf("the card's source %q does not name the plugin and its source", got)
	}
	// A phone's lookup says the same (psync.SourcePlugin).
	var sources map[string]psync.WordSource
	onEngine(a, func() {
		sources = a.phoneWordSources(a.vocabResolve(lookup.Query{Word: "Hello", Line: "Hello.", Language: "简体中文"}, "talk").ask)
	})
	if got := sources["plugin:acme.wiki:wiki"]; got != (psync.WordSource{Kind: psync.SourcePlugin, Name: "维基", Plugin: "Anki 发送器"}) {
		t.Errorf("a phone's source %+v", got)
	}
	if l := look("nothing"); l.source != "" || l.entry.Meaning != "无" {
		t.Errorf("a word the plugin does not have: %+v", l)
	}
	// It is not asked once it is turned off.
	onEngine(a, func() { a.SetPluginEnabled("acme.wiki", false) })
	scriptsSettle(t, a, tt, updates, func() bool { return a.plugins.script.loaded["acme.wiki"] == nil && !a.plugins.saving })
	var ask vocabAsk
	onEngine(a, func() {
		ask = a.vocabResolve(lookup.Query{Word: "Bye", Line: "Bye.", Language: "简体中文"}, "talk")
	})
	for _, d := range ask.ask.Dicts {
		if strings.Contains(d.ID(), "acme.wiki") {
			t.Error("a plugin turned off is asked")
		}
	}
}

// TestPluginContribSubtitleFormat checks a subtitle format of a script plugin: the dialog offers it after Kando's own; the file it writes in the film's folder has the format's extension and the text the plugin made of the cues Kando's own would write, with the user's choices applied.
func TestPluginContribSubtitleFormat(t *testing.T) {
	a, tt, updates, _, _ := contribApp(t, media.Tools{})
	apply(tt, a, a.OpenSubexport)
	if !tt.Exists("id:export-format-acme.wiki-lrc") {
		t.Fatalf("the dialog has no plugin format; texts %q", tt.Texts())
	}
	tt.Press("id:export-format-acme.wiki-lrc")
	refresh(tt, a)
	path := a.SubexportPath()
	if !strings.HasSuffix(path, "talk.dual.lrc") {
		t.Fatalf("the path %q", path)
	}
	tt.Press("id:export-to-folder")
	scriptsSettle(t, a, tt, updates, func() bool { return !a.export.open })
	data, err := os.ReadFile(path)
	if err != nil || !strings.HasPrefix(string(data), "lrc\n1000-2500 ") || !strings.Contains(string(data), "你好。") {
		t.Fatalf("the file %q, %v", data, err)
	}
	// Saved with the plugin turned off, the choice reads as SubRip.
	onEngine(a, func() { a.SetPluginEnabled("acme.wiki", false) })
	scriptsSettle(t, a, tt, updates, func() bool { return a.plugins.script.loaded["acme.wiki"] == nil })
	if c := a.SubexportChoice(); c.Format != "srt" || !strings.HasSuffix(a.SubexportPath(), ".srt") {
		t.Errorf("without the plugin %+v, %q", c, a.SubexportPath())
	}
	// Changing another choice meanwhile keeps the plugin's format remembered for when it is back.
	onEngine(a, func() {
		c := a.SubexportChoice()
		c.Content = "original"
		a.SetSubexportChoice(c)
	})
	if got := a.export.choice; got.Format != "plugin:acme.wiki:lrc" || got.Content != "original" {
		t.Errorf("remembered %+v", got)
	}
	if (subexport.Choice{Format: "plugin:acme.wiki:lrc"}).Clean().Format != "plugin:acme.wiki:lrc" {
		t.Error("Clean drops a plugin's format")
	}
}

// TestPluginContribExporter checks an export target of a script plugin: the Saved Lines sheet offers it; the export sends it the lines and words without asking for a folder and says what it answered.
func TestPluginContribExporter(t *testing.T) {
	a, tt, updates, _, _ := contribApp(t, media.Tools{})
	onEngine(a, func() {
		book, _ := study.Load(a.snap().LibraryDir)
		book.Lines = append(book.Lines, study.Line{ID: "l1", Video: "talk/talk.mp4", Original: "hello", Film: "talk"}, study.Line{ID: "l2", Video: "talk/talk.mp4", Original: "bye", Film: "talk", Start: 4 * time.Second})
		book.AddWord(study.Word{Text: "hello", Spoken: "en"})
		a.saved.book = book
	})
	id := pluginsContribID("acme.wiki", "notes")
	found := false
	onEngine(a, func() {
		for _, x := range a.ExportTargets() {
			found = found || x.ID == id
		}
		a.SetExportTarget(id)
	})
	if !found || a.ExportTarget() != id {
		t.Fatalf("targets: found %t, chosen %q", found, a.ExportTarget())
	}
	onEngine(a, a.ExportSaved)
	scriptsSettle(t, a, tt, updates, func() bool { return a.ExportState().Finished })
	e := a.ExportState()
	if e.Plugin != "Anki 发送器" || e.Sent != 2 || e.Skipped != 1 || e.Err != "" {
		t.Fatalf("the export %+v", e)
	}
	wantText(t, tt, testlocale.Messages().SavedExportedPlugin(2, 1, "Anki 发送器"))

	// A plugin that rejects the export on purpose, with a code, says why, cleaned to one line of what can be seen.
	onEngine(a, func() {
		a.saved.book.Lines = append(a.saved.book.Lines, study.Line{ID: "l3", Video: "talk/talk.mp4", Original: "fail", Film: "talk"})
		a.ExportSaved()
	})
	scriptsSettle(t, a, tt, updates, func() bool { return a.ExportState().Finished })
	if e := a.ExportState(); e.Err == "" || e.Reason != "Anki is not running. Start it." {
		t.Fatalf("a coded failure: %+v", e)
	}
	wantText(t, tt, testlocale.Messages().SavedExportPluginFailedReason("Anki 发送器", "Anki is not running. Start it."))

	// A refusal of Kando's the plugin passes on is not a reason of its own: the sheet says only that it failed.
	onEngine(a, func() {
		a.saved.book.Lines[len(a.saved.book.Lines)-1].Original = "passed on"
		a.ExportSaved()
	})
	scriptsSettle(t, a, tt, updates, func() bool { return a.ExportState().Finished })
	if e := a.ExportState(); e.Err == "" || e.Reason != "" {
		t.Fatalf("a refusal passed on: %+v", e)
	}
	wantText(t, tt, testlocale.Messages().SavedExportPluginFailed("Anki 发送器"))

	// Lines the plugin sent without their sound are counted on the sheet.
	onEngine(a, func() {
		a.saved.book.Lines[len(a.saved.book.Lines)-1].Original = "quiet"
		a.ExportSaved()
	})
	scriptsSettle(t, a, tt, updates, func() bool { return a.ExportState().Finished })
	if e := a.ExportState(); e.Err != "" || e.Sent != 3 || e.Soundless != 1 {
		t.Fatalf("lines without sound: %+v", e)
	}
	wantText(t, tt, testlocale.Messages().SavedExportedPluginSoundless(3, 1, 1, "Anki 发送器"))
	onEngine(a, func() { a.saved.book.Lines = a.saved.book.Lines[:len(a.saved.book.Lines)-1] })

	// An export to a plugin that never answers, stopped, says it stopped, not that it sent nothing.
	onEngine(a, func() {
		a.SetExportTarget(pluginsContribID("acme.wiki", "slow"))
		a.ExportSaved()
	})
	scriptsSettle(t, a, tt, updates, func() bool { return a.ExportState().Busy })
	onEngine(a, a.StopExport)
	refresh(tt, a)
	if e := a.ExportState(); !e.Stopped || e.Busy {
		t.Fatalf("stopped: %+v", e)
	}
	wantText(t, tt, testlocale.Messages().SavedExportStopped())
	if hasText(tt, testlocale.Messages().SavedExportedPlugin(0, 0, "Anki 发送器")) {
		t.Error("a stopped export says it sent nothing")
	}
}

// TestPluginContribAnnotator checks an annotator of a script plugin: the open film's lines are marked by it, its offsets read as JavaScript counts them, its mark out of the line dropped, and its marks show as Kando's own do.
func TestPluginContribAnnotator(t *testing.T) {
	a, tt, updates, _, _ := contribApp(t, media.Tools{})
	scriptsSettle(t, a, tt, updates, func() bool { return a.CueMarks(0) != nil })
	marks := a.CueMarks(0)
	// The original is Hello.; the plugin's line under 99 characters is out of it and dropped.
	if len(marks) != 1 || marks[0].Start != 0 || marks[0].End != 2 || marks[0].Ruby != "wk" {
		t.Fatalf("marks %+v", marks)
	}
	if !strings.Contains(a.plugins.marks.key.annotators, "acme.wiki") {
		t.Errorf("the key %+v", a.plugins.marks.key)
	}
}

// TestPluginLLMChatBudget checks kando.llm.chat: it answers with the service's reply and model, never its address or key; it counts the tokens against the plugin's daily budget and writes them to the audit log; a request that may take more than what is left is refused; and offline, for a cloud service, it is not granted.
func TestPluginLLMChatBudget(t *testing.T) {
	srv, seen := fakeLLM(t, "pong")
	a, tt, updates, _, root := contribApp(t, media.Tools{Translator: media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m1"}})
	got, err := scriptsCall(t, a, tt, updates, "acme.wiki", "chat", 50)
	m, _ := got.(map[string]any)
	if err != nil || m["text"] != "pong" || m["model"] != "m1" || strings.Contains(fmt.Sprint(got), srv.URL) || strings.Contains(fmt.Sprint(got), "Bearer") {
		t.Fatalf("chat: %v, %v", got, err)
	}
	if len(*seen) != 2 || (*seen)[0] != "be short" {
		t.Errorf("sent %q", *seen)
	}
	spent := a.pluginsSpent("acme.wiki")
	if spent <= 0 {
		t.Fatalf("spent %d", spent)
	}
	scriptsSettle(t, a, tt, updates, func() bool {
		data, _ := os.ReadFile(plugins.AuditPath(root))
		return strings.Contains(string(data), "llm-chat acme.wiki")
	})
	onEngine(a, func() { a.SetPluginBudget("acme.wiki", plugins.MinBudget) })
	scriptsSettle(t, a, tt, updates, func() bool { return !a.plugins.saving && a.plugins.state.Budget("acme.wiki") == plugins.MinBudget })
	if got, _ := scriptsCall(t, a, tt, updates, "acme.wiki", "chat", 2000); got != "refused budget" {
		t.Errorf("over budget: %v", got)
	}
}

// TestPluginLLMChatTimeoutIsNotAFailure checks that a plugin whose kando.llm.chat it timed out itself (ErrHostTimeout, ETIMEDOUT) is not counted a failure by the breaker, however often, as the slowness is the AI service's.
func TestPluginLLMChatTimeoutIsNotAFailure(t *testing.T) {
	hold := make(chan struct{})
	srv := chatServer(t, func() (int, string) { return http.StatusOK, "" }, hold)
	t.Cleanup(func() { close(hold) })
	a, tt, updates, _, _ := contribApp(t, media.Tools{Translator: media.ChatTranslator{BaseURL: srv.URL, APIKey: "k", Model: "m1"}})
	for range 5 {
		got, err := scriptsCallAdvancing(t, a, tt, updates, "acme.wiki", "chat", 10, 20)
		if err != nil || !strings.HasPrefix(fmt.Sprint(got), "refused ") {
			t.Fatalf("chat: %v, %v", got, err)
		}
	}
	if a.plugins.script.loaded["acme.wiki"] == nil {
		t.Fatal("the breaker stopped the plugin")
	}
}

// scriptsCallAdvancing is scriptsCall with the app's clock moved on as it waits, so a host call's own timeout, which runs on that clock, passes.
func scriptsCallAdvancing(t *testing.T, a *app, tt *nsgotest.App, updates chan func(), id, name string, args ...any) (any, error) {
	t.Helper()
	var got struct {
		v    any
		err  error
		done bool
	}
	onEngine(a, func() {
		a.pluginsRunner().Call(a.plugins.script.loaded[id].plugin, name, args, func(v any, err error) { got.v, got.err, got.done = v, err, true })
	})
	deadline := time.Now().Add(5 * time.Second)
	for !got.done {
		if time.Now().After(deadline) {
			t.Fatal("timed out waiting for the plugin")
		}
		select {
		case fn := <-updates:
			onEngine(a, fn)
		default:
		}
		tt.Advance(50 * time.Millisecond)
	}
	return got.v, got.err
}

// TestPluginSoundRead checks kando.sound.read: the sound of a saved line, cut by ffmpeg as the Anki export cuts it, base64; refused without ffmpeg.
func TestPluginSoundRead(t *testing.T) {
	ffmpeg := fakeTool(t, filepath.Join(t.TempDir(), "bin"), "ffmpeg")
	a, tt, updates, _, _ := contribApp(t, media.Tools{FFmpeg: ffmpeg})
	onEngine(a, func() {
		book, _ := study.Load(a.snap().LibraryDir)
		book.Lines = append(book.Lines, study.Line{ID: "l1", Video: "talk/talk.mp4", Start: time.Second, End: 2500 * time.Millisecond, Original: "Hello.", Film: "talk"})
		a.saved.book = book
	})
	got, err := scriptsCall(t, a, tt, updates, "acme.wiki", "sound", map[string]any{"saved": "l1"})
	m, _ := got.(map[string]any)
	data, _ := base64.StdEncoding.DecodeString(fmt.Sprint(m["base64"]))
	if err != nil || m["mime"] != "audio/mp4" || !strings.HasPrefix(string(data), "kando fake sound ") {
		t.Fatalf("sound: %v, %v", got, err)
	}
	if got, _ := scriptsCall(t, a, tt, updates, "acme.wiki", "sound", map[string]any{"saved": "nope"}); got != "refused not-found" {
		t.Errorf("a line not saved: %v", got)
	}
	onEngine(a, func() { a.pipeline = media.Tools{} })
	if got, _ := scriptsCall(t, a, tt, updates, "acme.wiki", "sound", map[string]any{"saved": "l1"}); got != "refused no-ffmpeg" {
		t.Errorf("without ffmpeg: %v", got)
	}
}

// credentialServer is a TLS server standing in for every host, which a client of the test reaches whatever host a request names, and the requests it heard, each its method, host, path and Authorization; /v2/translate/away redirects to /v2/translate/leak.
func credentialServer(t *testing.T) (*http.Client, func() []string) {
	t.Helper()
	var mu sync.Mutex
	var heard []string
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		heard = append(heard, r.Method+" "+r.Host+r.URL.Path+" "+r.Header.Get("Authorization")+" "+r.Header.Get("Cookie"))
		mu.Unlock()
		if r.URL.Path == "/v2/translate/slow" {
			time.Sleep(300 * time.Millisecond)
		}
		if r.URL.Path == "/v2/translate/away" {
			http.Redirect(w, r, "https://api.deepl.com/v2/translate/leak", http.StatusFound)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		w.Header().Set("Set-Cookie", "session=1")
		w.Header().Set("X-Trace", "abc")
		w.Write([]byte(`{"translations":[{"text":"你好"}]}`))
	}))
	t.Cleanup(srv.Close)
	addr := srv.Listener.Addr().String()
	client := &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}}
	return client, func() []string {
		mu.Lock()
		defer mu.Unlock()
		return append([]string(nil), heard...)
	}
}

// TestPluginCredentialFetch checks kando.net.credentialFetch (docs/plugin-system.md 6.3): with no key kept it is refused; with one, Kando puts it in the credential's header for a request within the credential, keeps Content-Type alone of the reply's headers and drops Set-Cookie; a request to another host, with another method or to another path is refused before it is sent; a redirect is not followed; the plugin's own code cannot read the key, nor set the credential's header itself; offline it is refused.
func TestPluginCredentialFetch(t *testing.T) {
	client, heard := credentialServer(t)
	a, tt, updates, _, _ := contribApp(t, media.Tools{})
	a.plugins.script.client = client
	const url = "https://api.deepl.com/v2/translate"
	if got, _ := scriptsCall(t, a, tt, updates, "acme.wiki", "fetchIt", "POST", url); got != "refused no-credential" {
		t.Fatalf("no key kept: %v", got)
	}
	onEngine(a, func() { a.SetPluginSecret("acme.wiki", plugins.SecretKey("deepl"), "s3cret-key") })
	scriptsSettle(t, a, tt, updates, func() bool { return a.PluginSecretKept("acme.wiki", plugins.SecretKey("deepl")) })
	got, err := scriptsCall(t, a, tt, updates, "acme.wiki", "fetchIt", "POST", url)
	m, _ := got.(map[string]any)
	if err != nil || fmt.Sprint(m["status"]) != "200" || fmt.Sprint(m["header"]) != "map[Content-Type:application/json]" || !strings.Contains(fmt.Sprint(m["body"]), "你好") {
		t.Fatalf("within the credential: %v, %v", got, err)
	}
	if strings.Contains(fmt.Sprint(got), "s3cret") {
		t.Fatal("the key reached the plugin")
	}
	if h := heard(); len(h) != 1 || h[0] != "POST api.deepl.com/v2/translate DeepL-Auth-Key s3cret-key " {
		t.Fatalf("the server heard %q", h)
	}
	for _, tc := range []struct{ method, url string }{
		{"GET", url},
		{"POST", "https://api.deepl.com.evil.example/v2/translate"},
		{"POST", "https://api.deepl.com/v2/translated"},
		{"POST", "https://api.deepl.com/v1/other"},
		{"POST", "http://api.deepl.com/v2/translate"},
		{"POST", "https://api.deepl.com:8443/v2/translate"},
		{"POST", "https://user@api.deepl.com/v2/translate"},
		{"POST", "https://api.deepl.com/v2/translate/../admin"},
		{"POST", "https://api.deepl.com/v2/translate/%2e%2e/admin"},
	} {
		if got, _ := scriptsCall(t, a, tt, updates, "acme.wiki", "fetchIt", tc.method, tc.url); got != "refused denied" {
			t.Errorf("%s %s: %v", tc.method, tc.url, got)
		}
	}
	if h := heard(); len(h) != 1 {
		t.Fatalf("a refused request was sent: %q", h)
	}
	got, _ = scriptsCall(t, a, tt, updates, "acme.wiki", "fetchIt", "POST", url+"/away")
	if m, _ := got.(map[string]any); fmt.Sprint(m["status"]) != "302" {
		t.Errorf("a redirect: %v", got)
	}
	for _, h := range heard() {
		if strings.Contains(h, "leak") {
			t.Fatalf("the redirect was followed: %q", heard())
		}
	}
	if got, _ := scriptsCall(t, a, tt, updates, "acme.wiki", "fetchIt", "POST", url, map[string]any{"Authorization": "Bearer mine"}); got != "refused bad-request" {
		t.Errorf("the plugin's own Authorization: %v", got)
	}
	if got, _ := scriptsCall(t, a, tt, updates, "acme.wiki", "fetchIt", "POST", url, map[string]any{"Cookie": "a=b"}); got != "refused bad-request" {
		t.Errorf("a Cookie: %v", got)
	}
	if got, _ := scriptsCall(t, a, tt, updates, "acme.wiki", "secret"); strings.Contains(fmt.Sprint(got), "s3cret") {
		t.Fatalf("the plugin's Secrets read the key: %v", got)
	}
	onEngine(a, func() { a.privacy.offline = true; a.Changed() })
	scriptsSettle(t, a, tt, updates, func() bool {
		l := a.plugins.script.loaded["acme.wiki"]
		return l != nil && !l.grant.Has(plugins.PermCredFetch)
	})
	if got, _ := scriptsCall(t, a, tt, updates, "acme.wiki", "fetchIt", "POST", url); !strings.HasPrefix(fmt.Sprint(got), "refused ") {
		t.Errorf("offline: %v", got)
	}
}

// TestPluginCredentialSettings checks the details of a plugin with a credential: the key's field, the line naming the hosts it is only sent to, and its warning when a plugin that is not Kando's own sends it to hosts Kando does not know for a service its name does not say; the budget of the AI service it asks.
func TestPluginCredentialSettings(t *testing.T) {
	a, tt, _, _, _ := contribApp(t, media.Tools{})
	m := testlocale.Messages()
	tt = settingsOpenAt(a, tt, settings.SectionPlugins)
	tt.Press("id:settings-plugins-details-acme.wiki")
	for _, id := range []string{"settings-plugins-credential-acme.wiki-deepl", "settings-plugins-credential-acme.wiki-deepl-hosts", "settings-plugins-budget-acme.wiki"} {
		if !tt.Exists("id:" + id) {
			t.Errorf("no %s; texts %q", id, tt.Texts())
		}
	}
	wantText(t, tt, m.SettingsPluginsCredentialHosts("api.deepl.com"))
	if tt.Exists("id:settings-plugins-credential-acme.wiki-deepl-warning") {
		t.Error("DeepL's own host is warned of")
	}
	for _, s := range []string{m.SettingsPluginsAddsLookup("维基"), m.SettingsPluginsAddsSubtitleFormat("LRC 歌词"), m.SettingsPluginsAddsExporter("维基卡片"), m.SettingsPluginsAddsAnnotator("前两个字")} {
		if !hasText(tt, "· "+s) {
			t.Errorf("the details do not say %q", s)
		}
	}
	// Reading the lines (a lookup source) with a credential is flagged.
	if !tt.Exists("id:settings-plugins-risky-acme.wiki") {
		t.Error("not flagged as risky")
	}
}

// TestPluginCredentialWithoutSecretStore checks that a plugin with credentials does not run where the system has no secure store, and says why.
func TestPluginCredentialWithoutSecretStore(t *testing.T) {
	a, tt, updates, _, _ := contribApp(t, media.Tools{})
	onEngine(a, func() {
		a.plugins.script.noSecrets = true
		a.pluginsScriptsSync()
	})
	scriptsSettle(t, a, tt, updates, func() bool { return a.plugins.script.loaded["acme.wiki"] == nil })
	tt = settingsOpenAt(a, tt, settings.SectionPlugins)
	tt.Press("id:settings-plugins-details-acme.wiki")
	wantText(t, tt, testlocale.Messages().SettingsPluginsNoSecretStore())
}

// TestPluginRevocationsFetched checks the daily revocation list: a 404 is no list, and the plugins go on; a list that revokes a plugin is taken, and the plugin is closed and says it was withdrawn; a list past its NotAfter pauses a plugin that reaches the network and is not Kando's own.
func TestPluginRevocationsFetched(t *testing.T) {
	key := newPluginsKey(t)
	dir := pluginScriptFolder(t, key, "acme.anki", "1.0.0", pluginAnkiFields, pluginAnkiCode)
	var mu sync.Mutex
	var list []byte
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		mu.Lock()
		defer mu.Unlock()
		if list == nil {
			http.NotFound(w, r)
			return
		}
		w.Write(list)
	}))
	t.Cleanup(srv.Close)
	a, tt, updates, root := scriptsApp(t, key, dir)
	a.plugins.script.revocationsURL = srv.URL
	scriptsGrant(t, a, tt, updates, "acme.anki", "network")
	onEngine(a, a.pluginsCheckRevocations)
	scriptsSettle(t, a, tt, updates, func() bool { return !a.plugins.script.revoking && !a.plugins.script.revokedAt.IsZero() })
	if a.plugins.script.loaded["acme.anki"] == nil {
		t.Fatal("no list stopped the plugin")
	}
	// The next fetch is due a day later; the file of the plugins folder says when it was.
	if script.RevocationsChecked(root).IsZero() {
		t.Fatal("the fetch was not recorded")
	}
	priv, _, _ := script.DevKey(key.secret)
	p := a.pluginsScriptPacks()[0]
	revoked, err := pkg.Revoke(priv, 1, time.Now().Add(time.Hour), nil, []string{p.Script.SHA256})
	if err != nil {
		t.Fatal(err)
	}
	data, _ := revoked.MarshalJSON()
	mu.Lock()
	list = data
	mu.Unlock()
	os.Remove(filepath.Join(root, ".revocations-checked"))
	onEngine(a, func() {
		a.plugins.script.revokedAt = time.Time{}
		a.pluginsCheckRevocations()
	})
	scriptsSettle(t, a, tt, updates, func() bool { return a.plugins.script.loaded["acme.anki"] == nil && len(a.plugins.script.issues) == 1 })
	tt = settingsOpenAt(a, tt, settings.SectionPlugins)
	wantText(t, tt, testlocale.Messages().PluginsErrRevoked())
}

// TestPluginStaleRevocationsPause checks pluginsStale: an expired list pauses a plugin of another publisher that reaches the network, never Kando's own, nor one of this computer alone.
func TestPluginStaleRevocationsPause(t *testing.T) {
	a := &app{}
	online := plugins.Pack{Manifest: plugins.Manifest{ID: "acme.anki", Kind: plugins.KindScript, Capabilities: &pkg.Capabilities{Network: &pkg.Network{HTTP: []pkg.HTTPRule{{Host: "x.example.com", Methods: []string{"POST"}, Paths: []string{"/"}}}}}}, Source: plugins.SourceVerified}
	local := plugins.Pack{Manifest: plugins.Manifest{ID: "acme.local", Kind: plugins.KindScript}, Source: plugins.SourceVerified}
	official := online
	official.ID, official.Source = "kando.anki", plugins.SourceOfficial
	expired := &pkg.Package{ExpiredRevocations: []string{"root"}}
	a.plugins.script.verified = map[string]script.Verified{"acme.anki": {Package: expired}, "acme.local": {Package: expired}, "kando.anki": {Package: expired}}
	if !a.pluginsStale(online) || a.pluginsStale(local) || a.pluginsStale(official) {
		t.Errorf("stale: network %t, local %t, official %t", a.pluginsStale(online), a.pluginsStale(local), a.pluginsStale(official))
	}
}

// TestPluginContribAnnotatorForPhones checks that a paired phone's film is marked by the script plugins' annotators too, as the desktop computes its marks for it (pluginsMarksFor).
func TestPluginContribAnnotatorForPhones(t *testing.T) {
	a, tt, updates, _, _ := contribApp(t, media.Tools{})
	var w *pluginsMarkWait
	var found pluginsMarks
	onEngine(a, func() {
		var state pluginsMarkState
		found, w, state = a.pluginsMarksFor("/films/other.mp4", "", false, []string{"Good morning.", "x"})
		if state != pluginsMarksFound {
			t.Errorf("state %d", state)
		}
	})
	if w != nil {
		scriptsSettle(t, a, tt, updates, func() bool {
			select {
			case <-w.done:
				return true
			default:
				return false
			}
		})
		found = w.marks
	}
	if len(found.lines) != 2 || len(found.lines[0]) != 1 || found.lines[0][0].Ruby != "wk" || len(found.lines[1]) != 0 {
		t.Fatalf("the phone's marks %+v", found)
	}
}

// TestPluginWaitingIsCapped checks that a call that awaits host calls in a loop, each a request Kando sends on its behalf to a slow service, is stopped once the app waited for it longer than the watchdog allows in all (script.Watch.Paused), so it does not hold the other plugins' calls for ever.
func TestPluginWaitingIsCapped(t *testing.T) {
	client, _ := credentialServer(t)
	a, tt, updates, _, _ := contribApp(t, media.Tools{})
	a.plugins.script.client = client
	onEngine(a, func() { a.SetPluginSecret("acme.wiki", plugins.SecretKey("deepl"), "k") })
	scriptsSettle(t, a, tt, updates, func() bool { return a.PluginSecretKept("acme.wiki", plugins.SecretKey("deepl")) })
	onEngine(a, func() { a.pluginsRunner().Watch.Paused = time.Second })
	var got struct {
		err  error
		done bool
	}
	onEngine(a, func() {
		a.pluginsRunner().Call(a.plugins.script.loaded["acme.wiki"].plugin, "loopFetch", nil, func(_ any, err error) { got.err, got.done = err, true })
	})
	deadline := time.Now().Add(10 * time.Second)
	for !got.done {
		if time.Now().After(deadline) {
			t.Fatal("the looping call was not stopped")
		}
		select {
		case fn := <-updates:
			onEngine(a, func() { fn(); a.Changed() })
		default:
			time.Sleep(5 * time.Millisecond)
		}
		tt.Settle()
	}
	if !errors.Is(got.err, script.ErrStopped) {
		t.Fatalf("the call ended with %v", got.err)
	}
	if h, ok := plugins.HealthOf(a.plugins.script.health, "acme.wiki", a.pluginsScriptPacks()[0].Hash); !ok || h.Reason != plugins.ReasonSlow {
		t.Errorf("health %+v", a.plugins.script.health)
	}
}

// TestPluginDetailsFollowTheLanguage checks that the words of a script plugin's details, what it adds and where its key goes among them, follow a switch of the interface's language.
func TestPluginDetailsFollowTheLanguage(t *testing.T) {
	a, tt, _, _, _ := contribApp(t, media.Tools{})
	tt = settingsOpenAt(a, tt, settings.SectionPlugins)
	tt.Press("id:settings-plugins-details-acme.wiki")
	wantText(t, tt, "· "+testlocale.Messages().SettingsPluginsAddsLookup("维基"))
	apply(tt, a, func() { a.SetInterfaceLanguage("en") })
	if !tt.Exists("id:settings-plugins-credential-acme.wiki-deepl") {
		tt.Press("id:settings-plugins-details-acme.wiki")
	}
	en := locales.In("en")
	wantText(t, tt, "· "+en.SettingsPluginsAddsLookup("Wiki"))
	wantText(t, tt, en.SettingsPluginsCredentialHosts("api.deepl.com"))
}

// TestPluginExportReasonIsThePluginsOwn checks that the sheet shows why an export target failed only when the plugin rejected with a code of its own: never for a refusal of Kando's it passed on, a host call that timed out (ETIMEDOUT) or the runtime's refusals (ERR_…), whose words are not written for the user, nor for an error without a code.
func TestPluginExportReasonIsThePluginsOwn(t *testing.T) {
	for code, shown := range map[string]bool{
		"anki-unreachable": true, "my-own": true,
		"": false, "ETIMEDOUT": false, "ERR_ACCESS_DENIED": false, "ERR_QUOTA": false,
		"denied": false, "quota": false, "bad-request": false, "busy": false, "unavailable": false, "budget": false, "cut-off": false,
		"timeout": false, "failed": false, "no-ffmpeg": false, "not-found": false, "too-large": false, "no-credential": false,
	} {
		a := &app{}
		a.saved.export = savedExport{finished: true, plugin: "Anki", err: &ns.Error{Code: code, Message: "why"}}
		if got := a.ExportState().Reason; (got == "why") != shown {
			t.Errorf("code %q: reason %q", code, got)
		}
	}
	a := &app{}
	a.saved.export = savedExport{finished: true, plugin: "Anki", err: errors.New("why")}
	if got := a.ExportState().Reason; got != "" {
		t.Errorf("an error without a code: reason %q", got)
	}
}
