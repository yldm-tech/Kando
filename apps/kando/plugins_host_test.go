package main

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/apps/kando/films"
	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/script"
	"kando/apps/kando/shelf"
	"kando/packages/media"
	"kando/packages/study"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// pluginHostFields are the fields of the test plugins acme.film and acme.twin: every permission this version has a handler for, a credential for a host no request of the tests reaches, and the events film.opened and llm.changed.
const pluginHostFields = `"capabilities": {"host": ["kando.film.current", "kando.line.current", "kando.lines.seen", "kando.saved.read", "kando.words.read", "kando.sound.read", "kando.llm.chat", "kando.net.credentialFetch", "kando.toast", "kando.ui.openURL", "kando.settings.get", "kando.glossary.suggest"]},
  "credentials": [{"id": "svc", "title": {"en": "Service key", "zh-Hans": "服务密钥"}, "header": "Authorization", "format": "Bearer {secret}", "hosts": ["api.example.com"], "methods": ["POST"], "pathPrefixes": ["/v1"]}],
  "events": ["film.opened", "llm.changed"]`

// pluginHostCode is their code: call calls the host handler name with data and answers what it answered or the code it was refused with; events is what the plugin heard; twice calls name a second time while the first call still waits, and answers what the second answered.
const pluginHostCode = `import { host } from "nsgo"
const heard = []
host.on("film.opened", (e) => { heard.push(["film.opened", e]) })
host.on("llm.changed", (e) => { heard.push(["llm.changed", e]) })
export async function call(name, data) {
  try { return await host.call(name, data || {}, { timeout: 150000 }) } catch (e) { return "refused " + e.code }
}
export function events() { return heard }
export async function twice(name, data) {
  call(name, data)
  return await call(name, data)
}
`

// pluginHostKey is the AI service's key in the tests of the host handlers, which no handler may answer with.
const pluginHostKey = "sk-host-test-0123456789"

// hostApp is the film "talk", heard in Japanese as the user said, in an app whose plugins folder holds acme.film and acme.twin, each granted every permission it declares and turned on, with an AI service at its own address and key and a fake ffmpeg; the film is not open yet. It returns the app, the film and the AI service's address.
func hostApp(t *testing.T) (*app, *nsgotest.App, chan func(), films.Film, string) {
	t.Helper()
	key := newPluginsKey(t)
	srv := chatServer(t, func() (int, string) { return http.StatusOK, "" }, nil)
	ffmpeg := fakeTool(t, filepath.Join(t.TempDir(), "bin"), "ffmpeg")
	tools := media.Tools{FFmpeg: ffmpeg, Translator: media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: pluginHostKey, Model: "m1"}}
	a, tt, updates, _ := playerFilm(t, 12, tools, nsgotest.PendingAlerts(), nsgotest.Secrets(map[string]string{}))
	apply(tt, a, func() { a.library.Jobs[0].SpeechLanguage = "ja" })
	item, ok := films.Find(a.films(), "talk")
	if !ok || item.SpeechLanguage != "ja" || item.Spoken != "" {
		t.Fatalf("the film is %+v", item)
	}
	root := filepath.Join(t.TempDir(), "plugins")
	pluginsScriptInstalled(t, root, pluginScriptFolder(t, key, "acme.film", "1.0.0", pluginHostFields, pluginHostCode), pluginScriptFolder(t, key, "acme.twin", "1.0.0", pluginHostFields, pluginHostCode))
	a.plugins.root = root
	a.plugins.script.official = key.public
	a.plugins.script.watch = script.Watch{Beat: 100 * time.Millisecond, Stall: time.Minute, Stalls: 3, Deadline: time.Minute}
	a.pluginsScan()
	waitUntil(t, tt, a, updates, func() bool {
		return a.plugins.scanned && !a.plugins.busy && !a.plugins.inboxBusy && a.plugins.script.verified != nil
	})
	var perms []string
	for _, p := range plugins.Perms {
		if !p.Free() {
			perms = append(perms, string(p))
		}
	}
	scriptsGrant(t, a, tt, updates, "acme.film", perms...)
	scriptsGrant(t, a, tt, updates, "acme.twin", perms...)
	return a, tt, updates, item, srv.URL
}

// hostCall calls the host handler name for the plugin id with data, through its call export, and returns what it answered.
func hostCall(t *testing.T, a *app, tt *nsgotest.App, updates chan func(), id string, name plugins.Perm, data map[string]any) any {
	t.Helper()
	got, err := scriptsCall(t, a, tt, updates, id, "call", string(name), data)
	if err != nil {
		t.Fatalf("%s: %v", name, err)
	}
	return got
}

// hostSave saves the line of item that starts at a second, and the word "Hello" looked up in it, as the player's buttons save them: by the video's path under the library (study.Book.Key).
func hostSave(a *app, item films.Film) {
	onEngine(a, func() {
		book, _ := study.Load(a.snap().LibraryDir)
		video := book.Key(item.VideoPath)
		book.Lines = append(book.Lines, study.Line{ID: "l1", Video: video, Start: time.Second, End: 2500 * time.Millisecond, Original: "Hello.", Translation: "你好。", Film: item.Title})
		book.Words = append(book.Words, study.Word{ID: "w1", Text: "hello", Spoken: "en", Context: "Hello.", Video: video, Start: time.Second, Film: item.Title})
		a.saved.book = book
	})
}

// TestPluginHostFilm checks kando.film.current, film.opened and the ids of films: with no film open it answers null; with one open, its id for the plugin, title, the language it is heard in, which falls back to the one the user named before the speech recognition heard one, the language it is translated into and where it plays; film.opened says the same film and language; a line saved from the film and a word looked up in it name the film by the same id in kando.saved.read and kando.words.read, though they keep its path under the library; another plugin knows the film by another id, and has a host of its own.
func TestPluginHostFilm(t *testing.T) {
	a, tt, updates, item, _ := hostApp(t)
	if got := hostCall(t, a, tt, updates, "acme.film", plugins.PermFilmCurrent, nil); got != nil {
		t.Fatalf("film.current with no film open: %v", got)
	}
	if got := hostCall(t, a, tt, updates, "acme.film", plugins.PermLinesSeen, nil); fmt.Sprint(got) != "[]" {
		t.Errorf("lines.seen with no film open: %v", got)
	}
	playerOpenFilm(t, a, tt, updates, item)
	playerLoadFilm(tt, a, 12*time.Second)
	scriptsSettle(t, a, tt, updates, func() bool { return a.plugins.script.opened == a.openKey })

	cur, _ := hostCall(t, a, tt, updates, "acme.film", plugins.PermFilmCurrent, nil).(map[string]any)
	id, _ := cur["film"].(string)
	if _, ok := cur["positionMs"].(float64); !ok || !strings.HasPrefix(id, "f_") || cur["title"] != "talk" || cur["spoken"] != "ja" || cur["language"] != "zh-Hans" {
		t.Fatalf("film.current: %v", cur)
	}
	heard, _ := scriptsCall(t, a, tt, updates, "acme.film", "events")
	var opened []string
	for _, e := range heard.([]any) {
		if s := fmt.Sprint(e); strings.HasPrefix(s, "[film.opened ") {
			opened = append(opened, s)
		}
	}
	if want := fmt.Sprintf("[film.opened map[film:%s spoken:ja]]", id); len(opened) != 1 || opened[0] != want {
		t.Errorf("heard %v, want %s", heard, want)
	}

	hostSave(a, item)
	lines, _ := hostCall(t, a, tt, updates, "acme.film", plugins.PermSavedRead, nil).([]any)
	words, _ := hostCall(t, a, tt, updates, "acme.film", plugins.PermWordsRead, nil).([]any)
	if len(lines) != 1 || lines[0].(map[string]any)["film"] != id {
		t.Errorf("saved.read names the film otherwise than film.current (%s): %v", id, lines)
	}
	if len(words) != 1 || words[0].(map[string]any)["film"] != id {
		t.Errorf("words.read names the film otherwise than film.current (%s): %v", id, words)
	}

	twin, _ := hostCall(t, a, tt, updates, "acme.twin", plugins.PermFilmCurrent, nil).(map[string]any)
	if twin["film"] == id || !strings.HasPrefix(fmt.Sprint(twin["film"]), "f_") {
		t.Errorf("two plugins know the film by %v and %s", twin["film"], id)
	}
	film, other := a.plugins.script.loaded["acme.film"], a.plugins.script.loaded["acme.twin"]
	if film.host == nil || other.host == nil || film.host == other.host {
		t.Error("the two plugins share a host")
	}
}

// TestPluginHostLinesSeen checks kando.lines.seen and kando.line.current: before the first line plays no line is given, and as it plays that line alone, never one after it.
func TestPluginHostLinesSeen(t *testing.T) {
	a, tt, updates, item, _ := hostApp(t)
	playerOpenFilm(t, a, tt, updates, item)
	playerLoadFilm(tt, a, 12*time.Second)
	if got := hostCall(t, a, tt, updates, "acme.film", plugins.PermLinesSeen, nil); fmt.Sprint(got) != "[]" {
		t.Errorf("lines.seen before the first line: %v", got)
	}
	if got := hostCall(t, a, tt, updates, "acme.film", plugins.PermLineCurrent, nil); got != nil {
		t.Errorf("line.current before the first line: %v", got)
	}
	playerPlayTo(tt, a, 1500*time.Millisecond)
	seen, _ := hostCall(t, a, tt, updates, "acme.film", plugins.PermLinesSeen, map[string]any{"limit": 10}).([]any)
	if len(seen) != 1 || seen[0].(map[string]any)["index"] != float64(0) || seen[0].(map[string]any)["original"] != "Hello." {
		t.Errorf("lines.seen on the first line: %v", seen)
	}
	if got, _ := hostCall(t, a, tt, updates, "acme.film", plugins.PermLineCurrent, nil).(map[string]any); got["index"] != float64(0) {
		t.Errorf("line.current on the first line: %v", got)
	}
}

// TestPluginHostRepliesTellNoSecret calls every host handler, granted, on an open film with a saved line, as a table, and checks that each answers, and that no answer holds the AI service's address or key, a path of the library, or a field named for an address or a key (docs/plugin-system.md 9). kando.sound.read is asked for the line playing ({current: true}). kando.ui.openURL waits for the user's answer in a system alert, and answers whether the page opened alone, which TestPluginScriptOpenURLWaitsForTheUser checks.
func TestPluginHostRepliesTellNoSecret(t *testing.T) {
	a, tt, updates, item, base := hostApp(t)
	playerOpenFilm(t, a, tt, updates, item)
	playerLoadFilm(tt, a, 12*time.Second)
	playerPlayTo(tt, a, 1500*time.Millisecond)
	hostSave(a, item)
	calls := map[plugins.Perm]struct {
		data map[string]any
		want func(v any) bool
	}{
		plugins.PermFilmCurrent: {nil, func(v any) bool { m, _ := v.(map[string]any); return m["title"] == "talk" }},
		plugins.PermLineCurrent: {nil, func(v any) bool { m, _ := v.(map[string]any); return m["original"] == "Hello." }},
		plugins.PermLinesSeen:   {map[string]any{"limit": 5}, func(v any) bool { l, _ := v.([]any); return len(l) == 1 }},
		plugins.PermSavedRead:   {nil, func(v any) bool { l, _ := v.([]any); return len(l) == 1 }},
		plugins.PermWordsRead:   {nil, func(v any) bool { l, _ := v.([]any); return len(l) == 1 }},
		plugins.PermSoundRead: {map[string]any{"current": true}, func(v any) bool {
			m, _ := v.(map[string]any)
			data, _ := base64.StdEncoding.DecodeString(fmt.Sprint(m["base64"]))
			return m["mime"] == "audio/mp4" && strings.HasPrefix(string(data), "kando fake sound ")
		}},
		plugins.PermLLMChat: {map[string]any{"system": "be short", "user": "hi", "maxTokens": 50}, func(v any) bool { m, _ := v.(map[string]any); return m["text"] == "pong" }},
		// No secret is kept for the credential, so nothing is sent.
		plugins.PermCredFetch:       {map[string]any{"credential": "svc", "method": "POST", "url": "https://api.example.com/v1/x", "header": map[string]any{}, "body": ""}, func(v any) bool { return v == "refused no-credential" }},
		plugins.PermToast:           {map[string]any{"text": "hello"}, func(v any) bool { return v == true }},
		plugins.PermSettingsGet:     {nil, func(v any) bool { _, ok := v.(map[string]any); return ok }},
		plugins.PermGlossarySuggest: {map[string]any{"terms": []any{map[string]any{"source": "Amazon", "target": "亚马孙河"}}}, func(v any) bool { m, _ := v.(map[string]any); n, _ := plugins.Number(m["added"]); return n == 1 }},
	}
	dir := a.snap().LibraryDir
	secrets := []string{base, strings.TrimPrefix(base, "http://"), pluginHostKey, dir, filepath.ToSlash(dir), item.VideoPath, `"baseURL"`, `"apiKey"`, `"BaseURL"`, `"APIKey"`}
	for _, perm := range plugins.Perms {
		c, ok := calls[perm]
		if !ok {
			if perm != plugins.PermOpenURL {
				t.Errorf("%s is not called", perm)
			}
			continue
		}
		got := hostCall(t, a, tt, updates, "acme.film", perm, c.data)
		if !c.want(got) {
			t.Errorf("%s answered %v", perm, got)
		}
		data, err := json.Marshal(got)
		if err != nil {
			t.Fatal(err)
		}
		for _, s := range secrets {
			if strings.Contains(string(data), s) {
				t.Errorf("%s answered %s, which holds %q", perm, data, s)
			}
		}
	}
}

// TestPluginHostLockedLibrary checks that while the open library is locked a plugin reads nothing of it, whatever it was granted: every permission that reads the library is refused, and those that do not, as a toast, still answer.
func TestPluginHostLockedLibrary(t *testing.T) {
	a, tt, updates, item, _ := hostApp(t)
	playerOpenFilm(t, a, tt, updates, item)
	playerLoadFilm(tt, a, 12*time.Second)
	hostSave(a, item)
	if lines, _ := hostCall(t, a, tt, updates, "acme.film", plugins.PermSavedRead, nil).([]any); len(lines) != 1 {
		t.Fatalf("saved.read before the lock: %v", lines)
	}
	apply(tt, a, func() {
		a.library.SetLock(a.snap().LibraryDir, shelf.Lock{Algorithm: "test", Iterations: 1, Salt: []byte("salt"), Key: []byte("key")})
	})
	scriptsSettle(t, a, tt, updates, func() bool {
		l := a.plugins.script.loaded["acme.film"]
		return l != nil && !l.grant.Has(plugins.PermSavedRead)
	})
	for _, perm := range plugins.Perms {
		if !perm.Library() {
			continue
		}
		got := hostCall(t, a, tt, updates, "acme.film", perm, map[string]any{"current": true, "saved": "l1"})
		if !strings.Contains(strings.ToLower(fmt.Sprint(got)), "denied") {
			t.Errorf("%s while the library is locked: %v", perm, got)
		}
	}
	if got := hostCall(t, a, tt, updates, "acme.film", plugins.PermToast, map[string]any{"text": "hello"}); got != true {
		t.Errorf("toast while the library is locked: %v", got)
	}
}

// TestPluginHostHearsLLMChanged checks that the plugins that hear llm.changed hear it, with no data, once the AI service's settings changed.
func TestPluginHostHearsLLMChanged(t *testing.T) {
	a, tt, updates, _, _ := hostApp(t)
	onEngine(a, a.areasLLMChanged)
	refresh(tt, a)
	for _, id := range []string{"acme.film", "acme.twin"} {
		heard, _ := scriptsCall(t, a, tt, updates, id, "events")
		list, _ := heard.([]any)
		if !slices.ContainsFunc(list, func(e any) bool { return fmt.Sprint(e) == "[llm.changed map[]]" }) {
			t.Errorf("%s heard %v", id, heard)
		}
	}
}

// TestPluginHostLLMChatRefusals checks the codes kando.llm.chat refuses with past its grant: unavailable without an AI service, cut-off for a reply the model stopped at its token limit, and busy for a second request while the plugin's first one still waits.
func TestPluginHostLLMChatRefusals(t *testing.T) {
	a, tt, updates, _, _ := hostApp(t)
	ask := map[string]any{"system": "be short", "user": "hi", "maxTokens": 50}
	ffmpeg := a.pipeline.(media.Tools).FFmpeg
	serve := func(translator media.ChatTranslator) {
		onEngine(a, func() { a.pipeline = media.Tools{FFmpeg: ffmpeg, Translator: translator} })
	}

	serve(media.ChatTranslator{})
	if got := hostCall(t, a, tt, updates, "acme.film", plugins.PermLLMChat, ask); got != "refused unavailable" {
		t.Errorf("without an AI service: %v", got)
	}

	cut := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(`{"choices":[{"message":{"role":"assistant","content":"po"},"finish_reason":"length"}]}`))
	}))
	t.Cleanup(cut.Close)
	serve(media.ChatTranslator{BaseURL: cut.URL + "/v1", APIKey: pluginHostKey, Model: "m1"})
	if got := hostCall(t, a, tt, updates, "acme.film", plugins.PermLLMChat, ask); got != "refused cut-off" {
		t.Errorf("a reply cut off: %v", got)
	}

	hold := make(chan struct{})
	slow := chatServer(t, func() (int, string) { return http.StatusOK, "" }, hold)
	// Registered after the server's Close, so it runs first and lets the waiting request end.
	t.Cleanup(func() { close(hold) })
	serve(media.ChatTranslator{BaseURL: slow.URL + "/v1", APIKey: pluginHostKey, Model: "m1"})
	got, err := scriptsCall(t, a, tt, updates, "acme.film", "twice", string(plugins.PermLLMChat), ask)
	if err != nil || got != "refused busy" {
		t.Errorf("a second request while the first waits: %v, %v", got, err)
	}
}
