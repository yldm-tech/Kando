package main

import (
	"context"
	"crypto/tls"
	"encoding/json"
	"errors"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"testing"
	"time"

	"kando/apps/kando/jobfailure"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/script"
	"kando/apps/kando/settings"
	"kando/apps/kando/settings/sectionid"
	"kando/apps/kando/sidebar"
	"kando/packages/core"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// pluginMTFields are the fields of the test plugin acme.mt: a translator of any language, one from English into Japanese alone, and one of any language on this computer.
const pluginMTFields = `"contributes": {"translators": [
    {"id": "caps", "title": {"en": "Shouty MT", "zh-Hans": "大写翻译"}, "languages": "any"},
    {"id": "ja", "title": {"en": "Into Japanese", "zh-Hans": "译成日语"}, "languages": {"from": ["en"], "to": ["ja"]}},
    {"id": "here", "title": {"en": "On this computer", "zh-Hans": "本机翻译"}, "languages": "any", "local": true}
  ]}`

// pluginMTCode is acme.mt's code: translate answers each line in capitals and keeps what it was given, quota as a line rejects with the code quota, short answers a line fewer; candidates offers the line in capitals, its current translation and a blank, twice over.
const pluginMTCode = `const batches = []
export async function translate(lines, from, to, context) {
  batches.push(lines.length + " " + from + " " + to + " " + context.translator)
  if (lines.includes("quota")) { const e = new Error("Quota Exceeded"); e.code = "quota"; throw e }
  if (lines.includes("short")) return lines.slice(1)
  return lines.map((l) => l.toUpperCase())
}
export async function candidates(line, from, to, context) {
  batches.push("candidates " + line + " " + to + " " + context.current + " " + context.before.length)
  return [line.toUpperCase() + "!", context.current, " ", line.toUpperCase() + "!"]
}
export function seen() { return batches }
`

// translatorApp starts an app whose plugins folder holds acme.mt, turned on, with media.Tools as its pipeline and no AI service.
func translatorApp(t *testing.T) (*app, *nsgotest.App, chan func()) {
	t.Helper()
	key := newPluginsKey(t)
	a, tt, updates, _ := scriptsApp(t, key, pluginScriptFolder(t, key, "acme.mt", "1.0.0", pluginMTFields, pluginMTCode))
	scriptsGrant(t, a, tt, updates, "acme.mt")
	onEngine(a, func() { a.pipeline = media.Tools{} })
	return a, tt, updates
}

// translateWith translates texts into language with tools off the engine's goroutine, as a job does, while the engine runs what the translator posts, and returns the lines and the error.
func translateWith(t *testing.T, a *app, tt *nsgotest.App, updates chan func(), tools media.Tools, language string, texts ...string) ([]string, error) {
	t.Helper()
	segments := make([]core.Segment, len(texts))
	for i, text := range texts {
		segments[i] = core.Segment{Text: text}
	}
	var got []core.Segment
	var err error
	done := make(chan struct{})
	go func() {
		got, err = tools.Translate(context.Background(), segments, language)
		close(done)
	}()
	scriptsSettle(t, a, tt, updates, func() bool {
		select {
		case <-done:
			return true
		default:
			return false
		}
	})
	out := make([]string, len(got))
	for i, s := range got {
		out[i] = s.Text
	}
	return out, err
}

// jobTools are the tools a job added now runs with (queuePipeline).
func jobTools(t *testing.T, a *app) media.Tools {
	t.Helper()
	var tools media.Tools
	onEngine(a, func() { tools, _ = a.queuePipeline(core.Job{}).(media.Tools) })
	return tools
}

// TestPluginTranslatorTranslatesTheFilms checks a script plugin's translator chosen in Settings › 翻译: it is offered beside the AI service, listed among what its plugin adds, kept in settings.json, and a job then translates with it, named by its title, 50 lines a call at most, its lines in order, and without an AI service the films are not untranslatable; its failures are coded: a rejection with quota, an answer of fewer lines, a target it does not take before anything is sent.
func TestPluginTranslatorTranslatesTheFilms(t *testing.T) {
	a, tt, updates := translatorApp(t)
	m := testlocale.Messages()
	if !a.untranslatable() {
		t.Fatal("no AI service and no source chosen, yet translatable")
	}
	var sources []settings.TranslateSource
	onEngine(a, func() { sources = a.TranslateSources("zh-Hans") })
	if fmt.Sprint(sources) != "[{plugin:acme.mt:caps 大写翻译} {plugin:acme.mt:ja 译成日语} {plugin:acme.mt:here 本机翻译}]" {
		t.Fatalf("sources %v", sources)
	}
	tt = settingsOpenAt(a, tt, settings.SectionTranslate)
	if !tt.Exists("id:settings-translate-source") {
		t.Fatalf("no source drop-down; texts %q", tt.Texts())
	}
	onEngine(a, func() { a.SetTranslateSource("plugin:acme.mt:caps") })
	refresh(tt, a)
	wantText(t, tt, m.SettingsTranslateSourceHint())
	tt = settingsOpenAt(a, tt, settings.SectionPlugins)
	tt.Press("id:settings-plugins-details-acme.mt")
	if !hasText(tt, "· "+m.SettingsPluginsAddsTranslator("大写翻译")) {
		t.Errorf("the details do not say it adds the translator; texts %q", tt.Texts())
	}
	var saved savedSettings
	onEngine(a, func() { a.areasSave(&saved) })
	if s := saved.TranslateSource; s == nil || *s != (savedTranslateSource{Plugin: "acme.mt", Translator: "caps", Title: "大写翻译"}) {
		t.Fatalf("saved %+v", saved.TranslateSource)
	}
	if a.untranslatable() {
		t.Fatal("a plugin's translator chosen, yet untranslatable for want of an AI service")
	}
	tools := jobTools(t, a)
	if tools.Engine == nil || tools.EngineName != "大写翻译" {
		t.Fatalf("the job's tools: %T %q", tools.Engine, tools.EngineName)
	}
	texts := make([]string, 120)
	for i := range texts {
		texts[i] = fmt.Sprintf("line %d", i)
	}
	got, err := translateWith(t, a, tt, updates, tools, "简体中文", texts...)
	if err != nil || len(got) != 120 || got[0] != "LINE 0" || got[119] != "LINE 119" {
		t.Fatalf("translated %d lines, first %q, %v", len(got), got, err)
	}
	seen, _ := scriptsCall(t, a, tt, updates, "acme.mt", "seen")
	if fmt.Sprint(seen) != "[50  zh-Hans caps 50  zh-Hans caps 20  zh-Hans caps]" {
		t.Fatalf("the plugin was given %v", seen)
	}
	_, err = translateWith(t, a, tt, updates, tools, "简体中文", "hello", "quota")
	if name, ok := media.EngineOf(err); !ok || name != "大写翻译" || err.Error() != "quota: Quota Exceeded" {
		t.Fatalf("a quota: %q %v", name, err)
	}
	if reason, section := jobfailure.SourceHint(m, "大写翻译", err.Error()); reason != m.PrecheckSourceQuota("大写翻译") || section != "" {
		t.Errorf("the quota's card: %q %q", reason, section)
	}
	if _, err = translateWith(t, a, tt, updates, tools, "简体中文", "short", "two"); err == nil || !strings.HasPrefix(err.Error(), plugins.TranslateBadReply) {
		t.Fatalf("an answer of fewer lines: %v", err)
	}
	onEngine(a, func() { a.SetTranslateSource("plugin:acme.mt:ja") })
	before, _ := scriptsCall(t, a, tt, updates, "acme.mt", "seen")
	if _, err = translateWith(t, a, tt, updates, jobTools(t, a), "简体中文", "hello"); err == nil || !strings.HasPrefix(err.Error(), plugins.TranslateLanguage) {
		t.Fatalf("a target it does not take: %v", err)
	}
	if after, _ := scriptsCall(t, a, tt, updates, "acme.mt", "seen"); fmt.Sprint(after) != fmt.Sprint(before) {
		t.Fatalf("lines were sent for a target it does not take: %v", after)
	}
}

// TestPluginTranslatorGone checks that a translator chosen whose plugin is turned off is gone: the films are translated with the AI service again, 翻译 offers the AI service and says so, and the message bar says so once; turned on again, the plugin's translator translates again.
func TestPluginTranslatorGone(t *testing.T) {
	a, tt, updates := translatorApp(t)
	m := testlocale.Messages()
	onEngine(a, func() { a.SetTranslateSource("plugin:acme.mt:caps") })
	onEngine(a, func() { a.SetPluginEnabled("acme.mt", false) })
	scriptsSettle(t, a, tt, updates, func() bool { return a.plugins.script.loaded["acme.mt"] == nil && !a.plugins.saving })
	if a.TranslateSource() != "" || a.TranslateSourceGone() != "大写翻译" {
		t.Fatalf("gone: %q %q", a.TranslateSource(), a.TranslateSourceGone())
	}
	if tools := jobTools(t, a); tools.Engine != nil {
		t.Fatal("a job still translates with a plugin turned off")
	}
	if !a.untranslatable() {
		t.Fatal("no AI service and the source gone, yet translatable")
	}
	if n := a.lib.notice; !n.Open || n.Text != m.SettingsTranslateSourceGone("大写翻译") || n.Section != settings.SectionTranslate {
		t.Fatalf("the message bar: %+v", n)
	}
	onEngine(a, func() { a.lib.notice.Open = false; a.Changed() })
	refresh(tt, a)
	if a.lib.notice.Open {
		t.Fatal("the message bar said it twice")
	}
	tt = settingsOpenAt(a, tt, settings.SectionTranslate)
	wantText(t, tt, m.SettingsTranslateSourceGone("大写翻译"))
	scriptsGrant(t, a, tt, updates, "acme.mt")
	if a.TranslateSourceGone() != "" || jobTools(t, a).Engine == nil {
		t.Fatal("turned on again, the plugin does not translate")
	}
}

// TestPluginTranslatorOffersCandidates checks 这句不对 (⇧E) with a script plugin's translator chosen whose plugin exports candidates: it is asked rather than the AI service, which is not set up, given the line, the target and the lines around it, and its translations show, without the line's current translation, blanks or repeats.
func TestPluginTranslatorOffersCandidates(t *testing.T) {
	a, tt, updates, _ := correctNamesOpen(t, media.Tools{})
	if apply(tt, a, a.StartCandidates); !a.Candidates().NeedsAI {
		t.Fatalf("no AI service and no source: %+v", a.Candidates())
	}
	onEngine(a, a.CloseCandidates)
	key := newPluginsKey(t)
	root := filepath.Join(t.TempDir(), "plugins")
	pluginsScriptInstalled(t, root, pluginScriptFolder(t, key, "acme.mt", "1.0.0", pluginMTFields, pluginMTCode))
	a.plugins.root = root
	a.plugins.script.official = key.public
	a.plugins.script.watch = script.Watch{Beat: 100 * time.Millisecond, Stall: time.Minute, Stalls: 3, Deadline: time.Minute}
	onEngine(a, a.pluginsScan)
	scriptsSettle(t, a, tt, updates, func() bool { return a.plugins.scanned && a.plugins.script.verified != nil && !a.plugins.inboxBusy })
	scriptsGrant(t, a, tt, updates, "acme.mt")
	onEngine(a, func() { a.SetTranslateSource("plugin:acme.mt:caps") })
	apply(tt, a, a.StartCandidates)
	scriptsSettle(t, a, tt, updates, func() bool { return a.Candidates().Open && !a.Candidates().Busy })
	c := a.Candidates()
	if c.NeedsAI || c.Say != nil || strings.Join(c.Options, "|") != "JON IS HERE.!" {
		t.Fatalf("candidates %+v", c)
	}
	seen, _ := scriptsCall(t, a, tt, updates, "acme.mt", "seen")
	if !strings.Contains(fmt.Sprint(seen), "candidates Jon is here. zh-Hans 约翰来了。 0") {
		t.Fatalf("the plugin was asked %v", seen)
	}
}

// deeplServer is a TLS server standing in for DeepL's two API hosts, which a client of the test reaches whatever host a request names: a key ending in :fx is taken at api-free.deepl.com alone and any other at api.deepl.com alone, as DeepL does, with 403 at the other; /v2/usage answers how much of the key's limit is used, which setUsed sets; /v2/translate answers each text in capitals after its target_lang, and 456 for a text QUOTA. It returns a client that reaches it and what it heard since it was last asked: each request's host, path, Authorization, and for a translation its target_lang, source_lang and how many texts, and every body.
func deeplServer(t *testing.T) (client *http.Client, heard func() (requests, bodies []string), setUsed func(int)) {
	t.Helper()
	var mu sync.Mutex
	var requests, bodies []string
	used := 0
	srv := httptest.NewTLSServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		var body struct {
			Text       []string `json:"text"`
			TargetLang string   `json:"target_lang"`
			SourceLang string   `json:"source_lang"`
		}
		raw := make([]byte, 1<<16)
		n, _ := r.Body.Read(raw)
		json.Unmarshal(raw[:n], &body)
		auth := r.Header.Get("Authorization")
		mu.Lock()
		line := r.Host + r.URL.Path + " " + auth
		if r.URL.Path == "/v2/translate" {
			line += fmt.Sprintf(" %s %s %d", body.TargetLang, body.SourceLang, len(body.Text))
		}
		requests = append(requests, line)
		bodies = append(bodies, string(raw[:n]))
		count := used
		mu.Unlock()
		key := strings.TrimPrefix(auth, "DeepL-Auth-Key ")
		if !strings.HasPrefix(auth, "DeepL-Auth-Key k-") || strings.HasSuffix(key, ":fx") != (r.Host == "api-free.deepl.com") {
			http.Error(w, `{"message":"Wrong endpoint or key"}`, http.StatusForbidden)
			return
		}
		w.Header().Set("Content-Type", "application/json")
		if r.URL.Path == "/v2/usage" {
			fmt.Fprintf(w, `{"character_count": %d, "character_limit": 500000}`, count)
			return
		}
		out := make([]map[string]string, len(body.Text))
		for i, text := range body.Text {
			if text == "QUOTA" {
				http.Error(w, `{"message":"Quota exceeded"}`, 456)
				return
			}
			out[i] = map[string]string{"text": "[" + body.TargetLang + "]" + strings.ToUpper(text)}
		}
		json.NewEncoder(w).Encode(map[string]any{"translations": out})
	}))
	t.Cleanup(srv.Close)
	addr := srv.Listener.Addr().String()
	client = &http.Client{Transport: &http.Transport{
		DialContext: func(ctx context.Context, network, _ string) (net.Conn, error) {
			return (&net.Dialer{}).DialContext(ctx, network, addr)
		},
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
	}}
	heard = func() ([]string, []string) {
		mu.Lock()
		defer mu.Unlock()
		r, b := requests, bodies
		requests, bodies = nil, nil
		return r, b
	}
	setUsed = func(n int) {
		mu.Lock()
		defer mu.Unlock()
		used = n
	}
	return client, heard, setUsed
}

// deeplFolder is the official DeepL plugin's source in the repository, found from this file's own place (runtime.Caller), as `nsgo test -target` runs the test binary from another folder than the package's; relative to the package's folder only when the build left no absolute path.
var deeplFolder = func() string {
	rel := filepath.Join("..", "..", "plugins", "official", "kando.deepl")
	if _, file, _, ok := runtime.Caller(0); ok && filepath.IsAbs(file) {
		return filepath.Join(filepath.Dir(file), rel)
	}
	return rel
}()

// TestOfficialDeepL checks Kando's official DeepL plugin (plugins/official/kando.deepl), packed with a throwaway root standing in for the official one, against a fake DeepL: without a key it fails as no-credential; a Free key (:fx) is sent to api-free.deepl.com alone, a Pro key to api.deepl.com once the host it knew refused it, and only by Kando, in the Authorization header, never in what the plugin's code composes nor answers; Kando's tags become DeepL's codes, Chinese with its script, English American by default and Portuguese Brazilian; blank lines stay blank and are not sent; the line count is kept; DeepL's 456 and an exhausted limit fail as quota, a key refused at both hosts as auth, each worded on the card by the source's name.
func TestOfficialDeepL(t *testing.T) {
	code, err := os.ReadFile(filepath.Join(deeplFolder, script.Entry))
	if err != nil {
		t.Fatal(err)
	}
	if strings.Contains(string(code), "Secrets") || strings.Contains(string(code), "Storage") {
		t.Fatal("the plugin's code reaches for a store of its own")
	}
	key := newPluginsKey(t)
	priv, _, err := script.DevKey(key.secret)
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	if err := script.PackFolder(deeplFolder, dir, priv); err != nil {
		t.Fatal(err)
	}
	a, tt, updates, _ := scriptsApp(t, key, dir)
	scriptsGrant(t, a, tt, updates, "kando.deepl", string(plugins.PermCredFetch))
	client, heard, setUsed := deeplServer(t)
	m := testlocale.Messages()
	onEngine(a, func() {
		a.plugins.script.client = client
		a.pipeline = media.Tools{}
		a.SetTranslateSource("plugin:kando.deepl:deepl")
	})
	tools := jobTools(t, a)
	if tools.EngineName != "DeepL" {
		t.Fatalf("named %q", tools.EngineName)
	}
	cardOf := func(err error) string {
		t.Helper()
		name, ok := media.EngineOf(err)
		if !ok {
			t.Fatalf("not the source's failure: %v", err)
		}
		reason, _ := jobfailure.SourceHint(m, name, err.Error())
		return reason
	}
	_, err = translateWith(t, a, tt, updates, tools, "简体中文", "hello")
	if got := cardOf(err); got != m.PrecheckSourceNoCredential("DeepL") {
		t.Fatalf("no key: %q (%v)", got, err)
	}
	secrets := []string{"k-free-123:fx", "k-pro-456"}
	setKey := func(k string) {
		onEngine(a, func() { a.SetPluginSecret("kando.deepl", plugins.SecretKey("deepl"), k) })
		scriptsSettle(t, a, tt, updates, func() bool { return a.PluginSecretKept("kando.deepl", plugins.SecretKey("deepl")) })
	}
	noKeyIn := func(what string, v any) {
		t.Helper()
		for _, s := range secrets {
			if strings.Contains(fmt.Sprint(v), strings.TrimPrefix(s, "k-")) {
				t.Fatalf("the key reached the plugin's %s: %v", what, v)
			}
		}
	}
	setKey(secrets[0])
	heard()
	texts := make([]string, 60)
	for i := range texts {
		texts[i] = fmt.Sprintf("line %d", i)
	}
	texts[7] = "  "
	got, err := translateWith(t, a, tt, updates, tools, "简体中文", texts...)
	if err != nil || len(got) != 60 || got[0] != "[ZH-HANS]LINE 0" || got[7] != "" || got[59] != "[ZH-HANS]LINE 59" {
		t.Fatalf("translated %q, %v", got, err)
	}
	noKeyIn("answer", got)
	requests, bodies := heard()
	want := []string{
		"api-free.deepl.com/v2/usage DeepL-Auth-Key k-free-123:fx",
		"api-free.deepl.com/v2/translate DeepL-Auth-Key k-free-123:fx ZH-HANS  49",
		"api-free.deepl.com/v2/translate DeepL-Auth-Key k-free-123:fx ZH-HANS  10",
	}
	if strings.Join(requests, "\n") != strings.Join(want, "\n") {
		t.Fatalf("DeepL heard\n%s\nwant\n%s", strings.Join(requests, "\n"), strings.Join(want, "\n"))
	}
	noKeyIn("requests' bodies", bodies)
	for _, tc := range []struct{ to, from, want string }{{"zh-Hant", "ja", "ZH-HANT JA"}, {"en", "zh", "EN-US ZH"}, {"pt", "", "PT-BR "}, {"en-GB", "en", "EN-GB EN"}, {"de", "", "DE "}, {"de", "nb", "DE NB"}, {"de", "no", "DE NB"}} {
		v, err := scriptsCall(t, a, tt, updates, "kando.deepl", "translate", []any{"a"}, tc.from, tc.to, map[string]any{"translator": "deepl"})
		requests, _ := heard()
		if err != nil || len(requests) != 1 || !strings.HasSuffix(requests[0], " "+tc.want+" 1") {
			t.Errorf("%s from %q: %v %v, heard %q", tc.to, tc.from, v, err, requests)
		}
		noKeyIn("answer", v)
	}
	_, err = translateWith(t, a, tt, updates, tools, "简体中文", "fine", "QUOTA")
	if got := cardOf(err); got != m.PrecheckSourceQuota("DeepL") {
		t.Fatalf("DeepL's 456: %q (%v)", got, err)
	}
	setKey(secrets[1])
	heard()
	got, err = translateWith(t, a, tt, updates, tools, "English", "你好")
	if err != nil || len(got) != 1 || got[0] != "[EN-US]你好" {
		t.Fatalf("a Pro key: %q %v", got, err)
	}
	requests, _ = heard()
	if last := requests[len(requests)-1]; !strings.HasPrefix(last, "api.deepl.com/v2/translate DeepL-Auth-Key k-pro-456 EN-US") {
		t.Fatalf("a Pro key went to %q", requests)
	}
	setKey("bad-key")
	_, err = translateWith(t, a, tt, updates, tools, "English", "你好")
	if got := cardOf(err); got != m.PrecheckSourceAuth("DeepL") {
		t.Fatalf("a refused key: %q (%v)", got, err)
	}
	setKey(secrets[0])
	setUsed(500000)
	_, err = translateWith(t, a, tt, updates, tools, "English", "你好")
	if got := cardOf(err); got != m.PrecheckSourceQuota("DeepL") {
		t.Fatalf("an exhausted limit: %q (%v)", got, err)
	}
	// Where the system has no secure store the plugin, which has a credential, cannot run: DeepL stays the source, the films fail as not running rather than go to the AI service, and the sidebar says so, with 插件 to fix it.
	onEngine(a, func() {
		a.pipeline = readyTools(t)
		a.side.ready.checked = true
		a.plugins.script.noSecrets = true
		a.pluginsScriptsSync()
	})
	scriptsSettle(t, a, tt, updates, func() bool { return a.plugins.script.loaded["kando.deepl"] == nil })
	if a.TranslateSourceGone() != "" || !a.pluginsTranslating() || a.pluginsTranslateBroken() != "DeepL" {
		t.Fatalf("unable to run: gone %q, translating %v, broken %q", a.TranslateSourceGone(), a.pluginsTranslating(), a.pluginsTranslateBroken())
	}
	if r := a.side.ready.status; r.Section != settings.SectionPlugins || r.Message == nil {
		t.Fatalf("the sidebar: %+v", r)
	}
	_, err = translateWith(t, a, tt, updates, jobTools(t, a), "English", "你好")
	if got := cardOf(err); got != m.PrecheckSourceNotRunning("DeepL") {
		t.Fatalf("unable to run: %q (%v)", got, err)
	}
}

// readyTools are tools with every program and a speech model, and no AI service, so the sidebar's status line comes to what translates.
func readyTools(t *testing.T) media.Tools {
	t.Helper()
	model := filepath.Join(t.TempDir(), "ggml-base.bin")
	if err := os.WriteFile(model, []byte("model"), 0o644); err != nil {
		t.Fatal(err)
	}
	return media.Tools{YTDLP: "/bin/true", FFmpeg: "/bin/true", Whisper: "/bin/true", WhisperModel: model}
}

// TestPluginTranslatorAtLaunch checks a job made at launch, before the plugins folder is read, with a plugin's translator chosen, as settings.json restores it: the job translates with the plugin once it loads, never with the AI service, and the films are not untranslatable meanwhile; a job stopped while it waits sends nothing to the plugin.
func TestPluginTranslatorAtLaunch(t *testing.T) {
	key := newPluginsKey(t)
	a, tt, updates := newTestApp(t, &fakePipeline{}, nsgotest.PendingAlerts(), nsgotest.Secrets(map[string]string{}))
	onEngine(a, func() {
		a.pipeline = media.Tools{}
		a.restoreTranslate(savedSettings{savedTranslate: savedTranslate{TranslateSource: &savedTranslateSource{Plugin: "acme.mt", Translator: "caps", Title: "大写翻译"}}})
	})
	if a.untranslatable() || a.TranslateSourceGone() != "" {
		t.Fatalf("before the folder is read: untranslatable %v, gone %q", a.untranslatable(), a.TranslateSourceGone())
	}
	tools := jobTools(t, a)
	if tools.Engine == nil || tools.EngineName != "大写翻译" {
		t.Fatalf("the job's tools before the folder is read: %T %q", tools.Engine, tools.EngineName)
	}
	stopped, stop := context.WithCancel(context.Background())
	stop()
	if _, err := tools.Translate(stopped, []core.Segment{{Text: "never"}}, "简体中文"); !errors.Is(err, context.Canceled) {
		t.Fatalf("a job stopped while it waits: %v", err)
	}
	var got []core.Segment
	var err error
	done := make(chan struct{})
	go func() {
		got, err = tools.Translate(context.Background(), []core.Segment{{Text: "hello"}, {Text: "world"}}, "简体中文")
		close(done)
	}()
	root := filepath.Join(t.TempDir(), "plugins")
	pluginsScriptInstalled(t, root, pluginScriptFolder(t, key, "acme.mt", "1.0.0", pluginMTFields, pluginMTCode))
	onEngine(a, func() {
		a.plugins.root = root
		a.plugins.script.official = key.public
		a.plugins.script.watch = script.Watch{Beat: 100 * time.Millisecond, Stall: time.Minute, Stalls: 3, Deadline: time.Minute}
		a.pluginsScan()
	})
	scriptsSettle(t, a, tt, updates, func() bool { return a.plugins.scanned && a.plugins.script.verified != nil && !a.plugins.inboxBusy })
	scriptsGrant(t, a, tt, updates, "acme.mt")
	scriptsSettle(t, a, tt, updates, func() bool {
		select {
		case <-done:
			return true
		default:
			return false
		}
	})
	if err != nil || len(got) != 2 || got[0].Text != "HELLO" || got[1].Text != "WORLD" {
		t.Fatalf("translated at launch: %+v %v", got, err)
	}
	if seen, _ := scriptsCall(t, a, tt, updates, "acme.mt", "seen"); fmt.Sprint(seen) != "[2  zh-Hans caps]" {
		t.Fatalf("the plugin was given %v", seen)
	}
}

// TestPluginTranslatorFollowsReadinessAndOffline checks that choosing a plugin's translator brings the sidebar's status line and the offer to translate up to date, that the final proofread's row shows only with an AI service set up, and that offline mode asks the translator chosen whether it runs on this computer, not the AI service: a cloud source refused with a local AI service, a local source kept with a cloud one.
func TestPluginTranslatorFollowsReadinessAndOffline(t *testing.T) {
	a, tt, _ := translatorApp(t)
	m := testlocale.Messages()
	tools := readyTools(t)
	onEngine(a, func() {
		a.pipeline = tools
		a.side.ready.checked = true
		a.checkReadiness(false)
	})
	if r := a.side.ready.status; r.Section != settings.SectionAI {
		t.Fatalf("no AI service: %+v", r)
	}
	apply(tt, a, func() { a.SetTranslateSource("plugin:acme.mt:caps") })
	if r := a.side.ready.status; r.State != sidebar.Ready || !a.lib.translateOffered {
		t.Fatalf("a plugin's source chosen: %+v, offered %v", r, a.lib.translateOffered)
	}
	onEngine(a, func() { a.prefs.review = true })
	if a.layersReviews() {
		t.Fatal("a final proofread row without an AI service")
	}
	tools.Translator = media.ChatTranslator{BaseURL: "http://127.0.0.1:11434/v1", Model: "qwen"}
	onEngine(a, func() { a.pipeline = tools })
	if !a.layersReviews() {
		t.Fatal("no final proofread row with an AI service")
	}
	apply(tt, a, func() { a.privacy.offline = true })
	if r := a.readiness(); r.State != sidebar.Missing || r.Section != sectionid.Privacy || !a.privacyNeedsNetwork(core.StageTranslating) {
		t.Fatalf("offline, a cloud source and a local AI service: %+v", r)
	}
	if msg := a.readiness().Message; msg == nil || msg(a.cx) != m.PrecheckSourceOffline("大写翻译") {
		t.Fatal("the status line does not name the source")
	}
	tools.Translator.BaseURL = "https://api.example.com/v1"
	onEngine(a, func() { a.pipeline = tools })
	apply(tt, a, func() { a.SetTranslateSource("plugin:acme.mt:here") })
	if r := a.readiness(); r.State != sidebar.Ready || a.privacyNeedsNetwork(core.StageTranslating) {
		t.Fatalf("offline, a local source and a cloud AI service: %+v", r)
	}
	if engine, ok := jobTools(t, a).Engine.(interface{ Local() bool }); !ok || !engine.Local() {
		t.Fatal("the local source's engine is not local")
	}
}
