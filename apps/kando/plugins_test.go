package main

import (
	"archive/zip"
	"bytes"
	"context"
	"encoding/json"
	"errors"
	"io/fs"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"
	"testing"
	"time"

	"kando/apps/kando/film/subtitle"
	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/dict"
	"kando/apps/kando/saved"
	"kando/apps/kando/settings"
	"kando/packages/core"
	"kando/packages/core/segment"
	"kando/packages/media"
	psync "kando/packages/sync"

	"github.com/nsgo-dev/nsgo/nsgotest"
)

// TestPluginReservedNames checks that plugins.ReservedNames, which the plugins package keeps itself as it imports no interface package, holds Kando's name and the name of every built-in AI service in every language of the interface, so a pack can take none of them.
func TestPluginReservedNames(t *testing.T) {
	for _, tag := range locales.Interface {
		m := locales.In(tag)
		names := []string{m.AppName()}
		for _, preset := range settings.Presets(m) {
			names = append(names, preset.Name)
		}
		for _, name := range names {
			if !slices.Contains(plugins.ReservedNames, name) {
				t.Errorf("%s: %q is not in plugins.ReservedNames", tag, name)
			}
		}
	}
}

// pluginGlossary is a glossary pack's manifest from Japanese into Simplified Chinese, the language newTestApp's films translate into.
const pluginGlossary = `{
  "schema": 1,
  "id": "example.anime-ja-zh",
  "kind": "glossary",
  "version": "1.2.0",
  "license": "CC-BY-4.0",
  "author": "Example",
  "name": {"en": "Anime terms", "zh-Hans": "动画常用词"},
  "glossary": {"from": "ja", "to": "zh-Hans", "terms": [{"source": "先輩", "target": "前辈"}, {"source": "お兄ちゃん", "target": "哥哥"}]}
}`

// pluginService is an ai-service pack's manifest.
const pluginService = `{
  "schema": 1,
  "id": "acme.corp-gateway",
  "kind": "ai-service",
  "version": "1.0.0",
  "license": "MIT",
  "name": {"en": "Company gateway", "zh-Hans": "公司网关"},
  "service": {"baseURL": "https://llm.example.com/v1", "model": "qwen3-32b", "keyURL": "https://keys.example.com/new", "group": "cloud"}
}`

// pluginFolder writes a pack folder holding manifest and returns it.
func pluginFolder(t *testing.T, manifest string) string {
	t.Helper()
	dir := t.TempDir()
	if err := os.WriteFile(filepath.Join(dir, plugins.ManifestName), []byte(manifest), 0o644); err != nil {
		t.Fatal(err)
	}
	return dir
}

// pluginZip writes a pack zip holding manifest and returns its path.
func pluginZip(t *testing.T, manifest string) string {
	t.Helper()
	var b bytes.Buffer
	w := zip.NewWriter(&b)
	f, err := w.Create(plugins.ManifestName)
	if err == nil {
		_, err = f.Write([]byte(manifest))
	}
	if err == nil {
		err = w.Close()
	}
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(t.TempDir(), "pack.zip")
	if err := os.WriteFile(path, b.Bytes(), 0o644); err != nil {
		t.Fatal(err)
	}
	return path
}

// pluginInstalled installs the packs of manifests into root as an install the user confirmed would.
func pluginInstalled(t *testing.T, root string, manifests ...string) {
	t.Helper()
	for _, manifest := range manifests {
		staged, p, err := plugins.Stage(root, pluginFolder(t, manifest))
		if err != nil {
			t.Fatal(err)
		}
		if err := plugins.Swap(root, staged, &p); err != nil {
			t.Fatal(err)
		}
	}
}

// pluginInstalledWith installs into root the pack of manifest with its files (name to content), as an install the user confirmed would.
func pluginInstalledWith(t *testing.T, root, manifest string, files map[string][]byte) {
	t.Helper()
	dir := pluginFolder(t, manifest)
	for name, data := range files {
		if err := os.WriteFile(filepath.Join(dir, name), data, 0o644); err != nil {
			t.Fatal(err)
		}
	}
	staged, p, err := plugins.Stage(root, dir)
	if err != nil {
		t.Fatal(err)
	}
	if err := plugins.Swap(root, staged, &p); err != nil {
		t.Fatal(err)
	}
}

// pluginsApp starts the app with its plugins folder in a new temporary folder, holding the packs of manifests, and waits until it has read it, as its ready does.
func pluginsApp(t *testing.T, manifests ...string) (*app, *nsgotest.App, chan func(), string) {
	t.Helper()
	a, tt, updates := newTestApp(t, &fakePipeline{}, nsgotest.PendingAlerts())
	root := filepath.Join(t.TempDir(), "plugins")
	pluginInstalled(t, root, manifests...)
	a.plugins.root = root
	a.pluginsScan()
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.scanned && !a.plugins.busy && !a.plugins.inboxBusy })
	return a, tt, updates, root
}

// TestPluginsPageInstallsAPack checks Settings › 插件 from nothing installed to a glossary pack turned off: the page says none is installed; a pack dropped on it is installed only after its confirmation, which says what it adds and where it comes from; the pack then shows in its kind's group with its switch on, and turning it off is saved in state.json.
func TestPluginsPageInstallsAPack(t *testing.T) {
	a, tt, updates, root := pluginsApp(t)
	m := testlocale.Messages()
	tt = settingsOpenAt(a, tt, settings.SectionPlugins)
	if !tt.Exists("id:settings-plugins-none") {
		t.Fatalf("the empty page; texts %q", tt.Texts())
	}

	tt.DropFiles("id:settings-plugins", fileURLs(pluginFolder(t, pluginGlossary))...)
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.pending != nil })
	if !tt.Exists("id:plugins-install-confirm") || !hasText(tt, m.SettingsPluginsInstallTitle("动画常用词")) || !hasText(tt, m.SettingsPluginsGlossaryDetail(2, m.SettingsSpeechLanguageJa(), m.SettingsLanguageSimplifiedChinese())) || !hasText(tt, m.SettingsPluginsInstallSource()) {
		t.Fatalf("the confirmation; texts %q", tt.Texts())
	}
	if packs, _ := plugins.Scan(root); len(packs) != 0 {
		t.Fatal("the pack was installed before it was confirmed")
	}
	if !a.modalOpen() {
		t.Error("the confirmation holds no modal flag")
	}

	tt.Press("id:plugins-install-ok")
	waitUntil(t, tt, a, updates, func() bool { return !a.plugins.busy && len(a.plugins.packs) == 1 })
	if a.plugins.pending != nil || a.modalOpen() {
		t.Error("the confirmation stayed open after the install")
	}
	if !tt.Exists("id:settings-plugins-row-example.anime-ja-zh") || !hasText(tt, m.SettingsPluginsKindGlossary()) || !hasText(tt, m.SettingsPluginsVersionAuthor("1.2.0", "Example")) {
		t.Fatalf("the installed pack; texts %q", tt.Texts())
	}
	if !tt.Exists("id:settings-plugins-switch-example.anime-ja-zh") || !a.plugins.state.Enabled("example.anime-ja-zh") {
		t.Fatal("the pack installed turned off")
	}

	tt.Press("id:settings-plugins-switch-example.anime-ja-zh")
	waitUntil(t, tt, a, updates, func() bool { return !a.plugins.busy })
	state, err := plugins.LoadState(root)
	if err != nil || state.Enabled("example.anime-ja-zh") {
		t.Fatalf("state.json after turning the pack off: %+v, %v", state, err)
	}
}

// TestPluginsPageUninstalls checks that 卸载… under a pack's details asks first, and that confirming removes the pack's folder and its entry of state.json.
func TestPluginsPageUninstalls(t *testing.T) {
	a, tt, updates, root := pluginsApp(t, pluginGlossary)
	tt = settingsOpenAt(a, tt, settings.SectionPlugins)
	tt.Press("id:settings-plugins-details-example.anime-ja-zh")
	if !tt.Exists("id:settings-plugins-detail-example.anime-ja-zh") || !hasText(tt, testlocale.Messages().SettingsPluginsLicense("CC-BY-4.0")) {
		t.Fatalf("the details; texts %q", tt.Texts())
	}
	tt.Press("id:settings-plugins-uninstall-example.anime-ja-zh")
	if packs, _ := plugins.Scan(root); len(packs) != 1 {
		t.Fatal("uninstalled before it was confirmed")
	}
	tt.Press("id:settings-plugins-uninstall-confirm-example.anime-ja-zh")
	waitUntil(t, tt, a, updates, func() bool { return !a.plugins.busy && len(a.plugins.packs) == 0 })
	if packs, _ := plugins.Scan(root); len(packs) != 0 {
		t.Fatalf("still installed: %+v", packs)
	}
	// The pack is remembered as missing until it is forgotten.
	if !tt.Exists("id:settings-plugins-forget-example.anime-ja-zh") {
		t.Fatalf("the missing pack; texts %q", tt.Texts())
	}
	tt.Press("id:settings-plugins-forget-example.anime-ja-zh")
	if len(a.plugins.order) != 0 || !tt.Exists("id:settings-plugins-none") {
		t.Fatalf("after forgetting it: order %q; texts %q", a.plugins.order, tt.Texts())
	}
}

// TestPluginsUninstallRefusedOverANewerState checks that while state.json is a newer Kando's, which this one does not write over (plugins.ErrStateFormat), an uninstall is refused before the pack's folder is removed, and the page says why, rather than removing the pack and then saying it failed.
func TestPluginsUninstallRefusedOverANewerState(t *testing.T) {
	a, tt, updates, root := pluginsApp(t, pluginGlossary)
	if err := os.WriteFile(filepath.Join(root, "state.json"), []byte(`{"format": 99, "revision": 1, "plugins": []}`), 0o644); err != nil {
		t.Fatal(err)
	}
	a.plugins.scanned = false
	onEngine(a, a.pluginsScan)
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.scanned && !a.plugins.inboxBusy })
	apply(tt, a, func() { a.UninstallPlugin("example.anime-ja-zh") })
	if a.plugins.busy || !errors.Is(a.plugins.err, plugins.ErrStateFormat) || a.PluginsErr() != testlocale.Messages().SettingsPluginsStateNewer() {
		t.Fatalf("the uninstall over a newer state: busy %v, err %v, says %q", a.plugins.busy, a.plugins.err, a.PluginsErr())
	}
	if packs, _ := plugins.Scan(root); len(packs) != 1 {
		t.Fatalf("removed over a newer state: %+v", packs)
	}
}

// TestPluginsUninstalledThoughTheStateWasNotSaved checks that a pack whose folder was removed is uninstalled even when state.json cannot be saved after it: the pack leaves the list, and the page says it was done and that which packs are on was not saved, rather than that the uninstall failed.
func TestPluginsUninstalledThoughTheStateWasNotSaved(t *testing.T) {
	a, tt, updates, root := pluginsApp(t, pluginGlossary)
	// A folder where state.json goes, which no file can be renamed over.
	if err := os.MkdirAll(filepath.Join(root, "state.json", "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	apply(tt, a, func() { a.UninstallPlugin("example.anime-ja-zh") })
	waitUntil(t, tt, a, updates, func() bool { return !a.plugins.busy && len(a.plugins.packs) == 0 })
	if packs, _ := plugins.Scan(root); len(packs) != 0 {
		t.Fatalf("still installed: %+v", packs)
	}
	var unsaved pluginsStateUnsaved
	done := testlocale.Messages().PluginsErrStateUnsaved("")
	if !errors.As(a.plugins.err, &unsaved) || !strings.HasPrefix(a.PluginsErr(), done) || len(a.PluginsErr()) == len(done) {
		t.Fatalf("the page says %q (%v)", a.PluginsErr(), a.plugins.err)
	}
}

// TestPluginsInboxAsksFirst checks that a pack the command line dropped into the inbox (kando plugin install) is offered in the install confirmation and never installed without it: it leaves the inbox, and 取消 installs nothing.
func TestPluginsInboxAsksFirst(t *testing.T) {
	a, tt, updates, root := pluginsApp(t)
	if _, err := plugins.Drop(root, pluginZip(t, pluginGlossary)); err != nil {
		t.Fatal(err)
	}
	a.pluginsCheckInbox()
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.pending != nil })
	if !tt.Exists("id:plugins-install-confirm") {
		t.Fatalf("no confirmation for the inbox's pack; texts %q", tt.Texts())
	}
	if files, _ := plugins.Inbox(root); len(files) != 0 {
		t.Errorf("the inbox still holds %q", files)
	}
	tt.Press("id:plugins-install-cancel")
	if a.plugins.pending != nil || a.modalOpen() {
		t.Error("the confirmation stayed open")
	}
	waitUntil(t, tt, a, updates, func() bool { return !a.plugins.busy && !a.plugins.inboxBusy })
	if packs, _ := plugins.Scan(root); len(packs) != 0 {
		t.Fatalf("installed without confirmation: %+v", packs)
	}
}

// TestPluginsInstallSaysWhyNot checks that a file that is not a pack this version installs is refused in a dialog that says why, in the interface's words.
func TestPluginsInstallSaysWhyNot(t *testing.T) {
	a, tt, updates, root := pluginsApp(t)
	a.InstallPlugins([]string{pluginFolder(t, `{"schema": 2, "id": "example.script", "kind": "script"}`)})
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.pending != nil })
	if !tt.Exists("id:plugins-install-failed") || !hasText(tt, testlocale.Messages().PluginsErrSchema()) {
		t.Fatalf("the refusal; texts %q", tt.Texts())
	}
	tt.Press("id:plugins-install-done")
	if a.plugins.pending != nil {
		t.Error("the refusal stayed open")
	}
	if packs, _ := plugins.Scan(root); len(packs) != 0 {
		t.Fatalf("installed %+v", packs)
	}
}

// TestPluginsIssues checks that a folder of the plugins folder that is not a valid pack is listed with why.
func TestPluginsIssues(t *testing.T) {
	root := filepath.Join(t.TempDir(), "plugins")
	if err := os.MkdirAll(filepath.Join(root, "glossary", "example.broken"), 0o755); err != nil {
		t.Fatal(err)
	}
	a, tt, updates := newTestApp(t, &fakePipeline{})
	a.plugins.root = root
	a.pluginsScan()
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.scanned })
	tt = settingsOpenAt(a, tt, settings.SectionPlugins)
	if !tt.Exists("id:settings-plugins-issue-example.broken") || !hasText(tt, testlocale.Messages().PluginsErrBadManifest()) {
		t.Fatalf("the issue; texts %q", tt.Texts())
	}
}

// TestPluginsDamagedStateKeepsPacksOff checks that while state.json cannot be read every pack is off, and that turning one on saves the others off rather than turning them all on again (plugins.State.Save).
func TestPluginsDamagedStateKeepsPacksOff(t *testing.T) {
	root := filepath.Join(t.TempDir(), "plugins")
	pluginInstalled(t, root, pluginGlossary, pluginService)
	if err := os.WriteFile(filepath.Join(root, "state.json"), []byte("{}"), 0o644); err != nil {
		t.Fatal(err)
	}
	a, tt, updates := newTestApp(t, &fakePipeline{})
	a.plugins.root = root
	a.pluginsScan()
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.scanned && !a.plugins.busy && !a.plugins.inboxBusy })
	if a.plugins.state.Enabled("example.anime-ja-zh") || a.plugins.state.Enabled("acme.corp-gateway") || a.PluginsNote() == "" {
		t.Fatalf("a damaged state: %+v, note %q", a.plugins.state, a.PluginsNote())
	}
	a.SetPluginEnabled("example.anime-ja-zh", true)
	waitUntil(t, tt, a, updates, func() bool { return !a.plugins.busy })
	state, err := plugins.LoadState(root)
	if err != nil || !state.Enabled("example.anime-ja-zh") || state.Enabled("acme.corp-gateway") || a.PluginsNote() != "" {
		t.Fatalf("saved %+v, %v, note %q", state, err, a.PluginsNote())
	}
}

// TestSavedPluginsClean checks that clean trims and dedupes the order of the packs and keeps the ids of packs not installed now.
func TestSavedPluginsClean(t *testing.T) {
	s := savedPlugins{PluginOrder: []string{" example.a ", "example.a", "", "example.missing"}}
	s.clean()
	if !slices.Equal(s.PluginOrder, []string{"example.a", "example.missing"}) {
		t.Fatalf("cleaned to %q", s.PluginOrder)
	}
}

// TestPluginTermsJoinTheGlossary checks that a glossary pack's terms join the films it is for, after the user's own: a job translated from Japanese into Simplified Chinese gets them, its locked term for the same words wins, a job into English gets none, and a pack turned off adds nothing. The open film's glossary (FilmTerms) has them too.
func TestPluginTermsJoinTheGlossary(t *testing.T) {
	a, tt, updates, _ := pluginsApp(t, pluginGlossary)
	a.gloss.store.Set("", core.Term{Source: "先輩", Target: "学长", Scope: core.ScopeGlobal, Origin: core.OriginUser, Locked: true})
	job := core.Job{ID: "j", Language: "简体中文", SpokenLanguage: "ja", VideoPath: filepath.Join(t.TempDir(), "film.mp4")}
	_, terms := a.glossaryStart(job)
	target := func(terms []core.Term, source string) string {
		for _, term := range terms {
			if term.Source == source {
				return term.Target
			}
		}
		return ""
	}
	if target(terms, "先輩") != "学长" || target(terms, "お兄ちゃん") != "哥哥" {
		t.Fatalf("terms %+v", terms)
	}
	english := job
	english.Language = "English"
	if _, terms := a.glossaryStart(english); target(terms, "お兄ちゃん") != "" {
		t.Errorf("a job into English got the pack's terms: %+v", terms)
	}
	// A job that names no language, as one an older version left, is translated into the settings' (queueRun).
	unnamed := job
	unnamed.Language = ""
	a.prefs.language = "简体中文"
	if _, terms := a.glossaryStart(unnamed); target(terms, "お兄ちゃん") != "哥哥" {
		t.Errorf("a job naming no language got %+v", terms)
	}
	a.prefs.language = "English"
	if _, terms := a.glossaryStart(unnamed); target(terms, "お兄ちゃん") != "" {
		t.Errorf("a job naming no language, the settings' English, got %+v", terms)
	}
	korean := job
	korean.SpokenLanguage = "ko"
	if _, terms := a.glossaryStart(korean); target(terms, "お兄ちゃん") != "" {
		t.Errorf("a job heard in Korean got the pack's terms: %+v", terms)
	}

	a.library.Jobs = []core.Job{job}
	a.gloss.video = job.VideoPath
	if target(a.FilmTerms(), "お兄ちゃん") != "" {
		t.Errorf("the open film's glossary listed a pack's term before its lines were loaded: %+v", a.FilmTerms())
	}
	pluginsFilmLines(a, job, "お兄ちゃん、待って！")
	if target(a.FilmTerms(), "お兄ちゃん") != "哥哥" {
		t.Errorf("the open film's glossary %+v", a.FilmTerms())
	}

	a.SetPluginEnabled("example.anime-ja-zh", false)
	waitUntil(t, tt, a, updates, func() bool { return !a.plugins.busy })
	if _, terms := a.glossaryStart(job); target(terms, "お兄ちゃん") != "" {
		t.Errorf("a pack turned off added %+v", terms)
	}
}

// pluginsFilmLines opens the film of job with lines whose originals are originals, as loaded subtitles, so its glossary (FilmTerms) lists the packs' terms they hold.
func pluginsFilmLines(a *app, job core.Job, originals ...string) {
	a.openKey, a.player.film.video = job.ID, job.VideoPath
	subs := subtitleView{key: job.ID, ready: true}
	for _, o := range originals {
		subs.cues = append(subs.cues, core.Segment{Text: o})
		subs.from = append(subs.from, core.Corrected{Line: core.Line{Original: o}, Edit: -1})
	}
	a.player.subs = subs
}

// TestPluginAIService checks an ai-service pack end to end: its service joins Settings › AI's list after Kando's own with its host, choosing it fills in its endpoint and model, its key is kept under pack.<id> when another service is chosen and through clean once the pack is uninstalled, and 获取 API Key names the page's host in an alert before it opens anything.
func TestPluginAIService(t *testing.T) {
	llmEnv(t, "", "", "")
	a, tt, updates, root := pluginsApp(t, pluginService)
	a.pipeline = media.Tools{Translator: media.ChatTranslator{BaseURL: "https://api.openai.com/v1", Model: "gpt-4.1-mini"}}
	a.syncLLM(a.pipeline.(media.Tools))
	const id = "pack.acme.corp-gateway"
	presets := a.Presets()
	if last := presets[len(presets)-1]; last.ID != id || last.Plugin != "acme.corp-gateway" || last.Name != "公司网关" || last.BaseURL != "https://llm.example.com/v1" {
		t.Fatalf("presets end with %+v", last)
	}
	tt = settingsOpenAt(a, tt, settings.SectionAI)
	tt.Press("id:settings-llm-provider")
	if !hasText(tt, testlocale.Messages().SettingsAIPluginService("公司网关", "llm.example.com")) {
		t.Fatalf("the drop-down; texts %q", tt.Texts())
	}
	tt.Key("escape")

	a.SelectLLM(id)
	refresh(tt, a)
	if got := translator(t, a); got.BaseURL != "https://llm.example.com/v1" || got.Model != "qwen3-32b" || a.prefs.llmProvider != id {
		t.Fatalf("chose the pack's service: %+v, provider %q", got, a.prefs.llmProvider)
	}
	if settings.ProviderID(a.llmPresets(a.messages()), "https://llm.example.com/v1") != id {
		t.Error("the pack's endpoint is not its preset's")
	}

	a.OpenKeyPage()
	refresh(tt, a)
	alerts := tt.Alerts()
	if len(alerts) != 1 || alerts[0].Title != testlocale.Messages().SettingsAIKeyPageTitle("keys.example.com") {
		t.Fatalf("alerts %+v", alerts)
	}
	tt.Answer(0)

	*a.LLMKey() = "sk-gateway"
	a.ApplyLLM()
	a.SelectLLM("openai")
	if got := a.prefs.llmServices[id]; got.APIKey != "sk-gateway" {
		t.Fatalf("the pack's service was kept as %+v", got)
	}

	a.UninstallPlugin("acme.corp-gateway")
	waitUntil(t, tt, a, updates, func() bool { return !a.plugins.busy && len(a.plugins.packs) == 0 })
	if packs, _ := plugins.Scan(root); len(packs) != 0 {
		t.Fatal("still installed")
	}
	saved := a.settingsFile()
	saved.clean()
	if got := saved.LLMServices[id]; got.APIKey != "sk-gateway" {
		t.Fatalf("the key went with the pack: %+v", saved.LLMServices)
	}
	if slices.ContainsFunc(a.Presets(), func(p settings.Preset) bool { return p.ID == id }) {
		t.Error("the uninstalled pack's service is still offered")
	}
}

// pluginServiceOpenAI is an ai-service pack whose endpoint is the built-in OpenAI service's.
const pluginServiceOpenAI = `{
  "schema": 1,
  "id": "acme.openai-proxy",
  "kind": "ai-service",
  "version": "1.0.0",
  "license": "MIT",
  "name": {"en": "Proxy", "zh-Hans": "代理"},
  "service": {"baseURL": "https://api.openai.com/v1", "model": "gpt-4.1-mini", "group": "cloud"}
}`

// TestPluginAIServiceTurnedOff checks that turning off the pack whose service is in use keeps that service, and the key the user gave it, under the pack's id: choosing another service afterwards leaves the user's own custom endpoint and key as they were, rather than putting the pack's in their place, and turning the pack on again shows its service as its own.
func TestPluginAIServiceTurnedOff(t *testing.T) {
	llmEnv(t, "", "", "")
	a, tt, updates, _ := pluginsApp(t, pluginService)
	a.pipeline = media.Tools{Translator: media.ChatTranslator{BaseURL: "https://api.openai.com/v1", Model: "gpt-4.1-mini"}}
	a.syncLLM(a.pipeline.(media.Tools))
	const id = "pack.acme.corp-gateway"
	custom := savedLLMService{BaseURL: "https://mine.example.org/v1", Model: "mine", APIKey: "sk-mine"}
	a.prefs.llmServices = map[string]savedLLMService{"custom": custom}

	a.SelectLLM(id)
	*a.LLMKey() = "sk-gateway"
	a.ApplyLLM()
	a.SetPluginEnabled("acme.corp-gateway", false)
	waitUntil(t, tt, a, updates, func() bool { return !a.plugins.busy })
	if a.prefs.llmServices[id].APIKey != "sk-gateway" {
		t.Fatalf("turning the pack off kept its service as %+v", a.prefs.llmServices)
	}
	if saved := a.settingsFile(); saved.LLMProvider != id {
		t.Errorf("settings.json names the service %q, want the pack's", saved.LLMProvider)
	}
	a.SelectLLM("openai")
	if got := a.prefs.llmServices["custom"]; got != custom {
		t.Fatalf("the custom service became %+v", got)
	}
	if got := a.prefs.llmServices[id]; got.APIKey != "sk-gateway" || got.BaseURL != "https://llm.example.com/v1" {
		t.Fatalf("the pack's service is kept as %+v", got)
	}

	// Back on the pack's endpoint while the pack is off, then turned on: its service is its own again, with its key.
	a.SelectLLM(id)
	if a.prefs.llmProvider == id {
		t.Fatal("chose a service that is not offered")
	}
	a.SetPluginEnabled("acme.corp-gateway", true)
	waitUntil(t, tt, a, updates, func() bool { return !a.plugins.busy })
	a.SelectLLM(id)
	if got := translator(t, a); got.APIKey != "sk-gateway" || a.prefs.llmProvider != id {
		t.Fatalf("the pack's service came back as %+v, provider %q", got, a.prefs.llmProvider)
	}
}

// TestPluginAIServiceStartsAsSaved checks that the pack's service saved last time is shown as the pack's once the plugins folder is read, though the app took it for a custom endpoint at startup, before any pack was known, and that a key saved meanwhile goes under the pack's id.
func TestPluginAIServiceStartsAsSaved(t *testing.T) {
	llmEnv(t, "", "", "")
	root := filepath.Join(t.TempDir(), "plugins")
	pluginInstalled(t, root, pluginService)
	a, tt, updates := newTestApp(t, &fakePipeline{})
	const id = "pack.acme.corp-gateway"
	a.pipeline = media.Tools{Translator: media.ChatTranslator{BaseURL: "https://llm.example.com/v1", Model: "qwen3-32b", APIKey: "sk-gateway"}}
	a.syncLLM(a.pipeline.(media.Tools))
	var saved savedSettings
	saved.LLMProvider, saved.LLMBaseURL = id, "https://llm.example.com/v1"
	a.pluginsHooks().restore(saved)
	if a.prefs.llmProvider != "custom" {
		t.Fatalf("before the packs are read the service is %q", a.prefs.llmProvider)
	}
	a.llmRemember()
	if _, ok := a.prefs.llmServices["custom"]; ok {
		t.Fatalf("the pack's service was kept as the custom one: %+v", a.prefs.llmServices)
	}
	a.plugins.root = root
	a.pluginsScan()
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.scanned && !a.plugins.inboxBusy })
	if a.prefs.llmProvider != id {
		t.Fatalf("after the packs were read the service is %q", a.prefs.llmProvider)
	}
}

// TestPluginAIServiceKeepsTheChoice checks that reading the packs again does not swap the service the user chose for another of the same endpoint: a pack's service whose endpoint is OpenAI's stays the pack's, OpenAI stays OpenAI, and 自定义 stays 自定义.
func TestPluginAIServiceKeepsTheChoice(t *testing.T) {
	llmEnv(t, "", "", "")
	a, tt, updates, _ := pluginsApp(t, pluginServiceOpenAI, pluginGlossary)
	a.pipeline = media.Tools{Translator: media.ChatTranslator{BaseURL: "https://api.openai.com/v1", Model: "gpt-4.1-mini"}}
	a.syncLLM(a.pipeline.(media.Tools))
	const id = "pack.acme.openai-proxy"
	for _, choice := range []string{id, "openai"} {
		a.SelectLLM(choice)
		a.SetPluginEnabled("example.anime-ja-zh", false)
		waitUntil(t, tt, a, updates, func() bool { return !a.plugins.busy })
		a.pluginsScan()
		waitUntil(t, tt, a, updates, func() bool { return !a.plugins.inboxBusy && a.plugins.scanned })
		if a.prefs.llmProvider != choice {
			t.Errorf("chose %q, the packs changed it to %q", choice, a.prefs.llmProvider)
		}
		a.SetPluginEnabled("example.anime-ja-zh", true)
		waitUntil(t, tt, a, updates, func() bool { return !a.plugins.busy })
	}

	a.SelectLLM("custom")
	*a.LLMBase() = "https://llm.example.com/v1"
	a.ApplyLLM()
	pluginInstalled(t, a.plugins.root, pluginService)
	a.pluginsScan()
	waitUntil(t, tt, a, updates, func() bool { return !a.plugins.inboxBusy && len(a.plugins.packs) == 3 })
	if a.prefs.llmProvider != "custom" {
		t.Fatalf("a custom endpoint a new pack has became %q", a.prefs.llmProvider)
	}
}

// TestPluginsInstallFailureStaysOpen checks that an install that fails once confirmed says why in the confirmation, which stays open until 好, as it may show over the main window with the 插件 page nowhere in sight.
func TestPluginsInstallFailureStaysOpen(t *testing.T) {
	a, tt, updates, _ := pluginsApp(t, pluginGlossary)
	// An ai-service pack with the glossary pack's id, which another kind's pack has (plugins.ErrIDTaken).
	taken := strings.Replace(pluginService, `"acme.corp-gateway"`, `"example.anime-ja-zh"`, 1)
	a.InstallPlugins([]string{pluginFolder(t, taken)})
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.pending != nil })
	tt.Press("id:plugins-install-ok")
	waitUntil(t, tt, a, updates, func() bool { return !a.plugins.busy && a.plugins.pending != nil && a.plugins.pending.err != nil })
	if !tt.Exists("id:plugins-install-failed") || !hasText(tt, testlocale.Messages().PluginsErrIDTaken()) || !a.modalOpen() {
		t.Fatalf("the failed install; texts %q", tt.Texts())
	}
	tt.Press("id:plugins-install-done")
	if a.plugins.pending != nil || a.modalOpen() {
		t.Error("the failure stayed open after 好")
	}
}

// TestPluginsInstallRefusedOverANewerState checks that while state.json is a newer Kando's, which this one does not write over (plugins.ErrStateFormat), a confirmed install is refused before any pack is put in place, and the confirmation says why.
func TestPluginsInstallRefusedOverANewerState(t *testing.T) {
	a, tt, updates, root := pluginsApp(t)
	if err := os.MkdirAll(root, 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(root, "state.json"), []byte(`{"format": 99, "revision": 1, "plugins": []}`), 0o644); err != nil {
		t.Fatal(err)
	}
	a.plugins.scanned = false
	onEngine(a, a.pluginsScan)
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.scanned && !a.plugins.inboxBusy })
	a.InstallPlugins([]string{pluginFolder(t, pluginGlossary)})
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.pending != nil && !a.plugins.busy })
	tt.Press("id:plugins-install-ok")
	refresh(tt, a)
	if a.plugins.busy || a.plugins.pending == nil || a.plugins.pending.err == nil {
		t.Fatalf("the install over a newer state: busy %v, pending %+v", a.plugins.busy, a.plugins.pending)
	}
	if !tt.Exists("id:plugins-install-failed") || !hasText(tt, testlocale.Messages().SettingsPluginsStateNewer()) {
		t.Fatalf("the refusal; texts %q", tt.Texts())
	}
	if packs, _ := plugins.Scan(root); len(packs) != 0 {
		t.Fatalf("installed over a newer state: %+v", packs)
	}
}

// TestPluginsInstalledThoughTheStateWasNotSaved checks that a pack put in place is installed even when state.json cannot be saved after it: the confirmation closes, as the pack already took the place of the one before, and the page says the state was not saved.
func TestPluginsInstalledThoughTheStateWasNotSaved(t *testing.T) {
	a, tt, updates, root := pluginsApp(t)
	// A folder where state.json goes, which no file can be renamed over.
	if err := os.MkdirAll(filepath.Join(root, "state.json", "x"), 0o755); err != nil {
		t.Fatal(err)
	}
	a.InstallPlugins([]string{pluginFolder(t, pluginGlossary)})
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.pending != nil && !a.plugins.busy })
	tt.Press("id:plugins-install-ok")
	waitUntil(t, tt, a, updates, func() bool { return !a.plugins.busy && a.plugins.scanned && len(a.plugins.packs) == 1 })
	if a.plugins.pending != nil || a.modalOpen() {
		t.Fatalf("the confirmation stayed open after the pack was put in place: %+v", a.plugins.pending)
	}
	if a.plugins.err == nil || a.PluginsErr() == "" {
		t.Fatal("the page does not say state.json was not saved")
	}
	if packs, _ := plugins.Scan(root); len(packs) != 1 {
		t.Fatalf("installed %+v", packs)
	}
}

// TestPluginsInstallCannotBeCancelledWhileItRuns checks that once 安装 is pressed the confirmation cannot be cancelled until the install ends: 取消 is off, and Escape, Enter and CancelPluginInstall leave it open, so the staged copy is not thrown away while plugins.Swap moves it into place; the pack is then installed whole and turned on.
func TestPluginsInstallCannotBeCancelledWhileItRuns(t *testing.T) {
	a, tt, updates, root := pluginsApp(t)
	a.InstallPlugins([]string{pluginFolder(t, pluginGlossary)})
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.pending != nil && !a.plugins.busy })
	tt.Press("id:plugins-install-ok")
	refresh(tt, a)
	if !a.plugins.busy {
		t.Fatal("the install was not running")
	}
	if tt.Enabled("id:plugins-install-cancel") {
		t.Error("取消 is on while the install runs")
	}
	tt.Key("escape")
	tt.Key("enter")
	a.CancelPluginInstall()
	if a.plugins.pending == nil {
		t.Fatal("the confirmation was cancelled while its install ran")
	}
	waitUntil(t, tt, a, updates, func() bool { return !a.plugins.busy && a.plugins.pending == nil })
	if a.modalOpen() {
		t.Error("the confirmation held its modal flag after the install")
	}
	packs, issues := plugins.Scan(root)
	if len(packs) != 1 || len(issues) != 0 || !a.plugins.state.Enabled("example.anime-ja-zh") {
		t.Fatalf("after the install: packs %+v, issues %+v, state %+v", packs, issues, a.plugins.state)
	}
}

// TestPluginsInboxKeepsTheSwitches checks that looking at the inbox, as the app does each time it comes to the front, does not keep a pack from being turned off meanwhile.
func TestPluginsInboxKeepsTheSwitches(t *testing.T) {
	a, tt, updates, root := pluginsApp(t, pluginGlossary)
	a.plugins.inboxBusy = true
	a.SetPluginEnabled("example.anime-ja-zh", false)
	waitUntil(t, tt, a, updates, func() bool { return !a.plugins.busy })
	if state, err := plugins.LoadState(root); err != nil || state.Enabled("example.anime-ja-zh") {
		t.Fatalf("state.json after turning the pack off while the inbox was looked at: %+v, %v", state, err)
	}
	a.plugins.inboxBusy = false
}

// pluginsHeld runs fn on the engine's goroutine with the reports of the background work it starts held apart from the test's queue, and returns them as they come, so the test applies them in the order it chooses.
func pluginsHeld(a *app, fn func()) <-chan func() {
	held := make(chan func(), 4)
	onEngine(a, func() {
		update := a.update
		a.update = func(report func()) { held <- report }
		fn()
		a.update = update
	})
	return held
}

// TestPluginsInboxLooksAgainAfterASwitch checks that a look at the inbox that gave way to a switch running meanwhile looks again once the switch is saved, so the file the command line dropped is asked about without the app having to come to the front again.
func TestPluginsInboxLooksAgainAfterASwitch(t *testing.T) {
	a, tt, updates, root := pluginsApp(t, pluginGlossary)
	if _, err := plugins.Drop(root, pluginZip(t, pluginService)); err != nil {
		t.Fatal(err)
	}
	look := <-pluginsHeld(a, a.pluginsCheckInbox)
	onEngine(a, func() { a.SetPluginEnabled("example.anime-ja-zh", false) })
	onEngine(a, look)
	if a.plugins.pending != nil {
		t.Fatal("the inbox was asked about while the switch ran")
	}
	if files, _ := plugins.Inbox(root); len(files) != 1 {
		t.Fatalf("the look that gave way took the file: inbox %q", files)
	}
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.pending != nil })
	if p := a.plugins.pending; p.err != nil || p.pack.ID != "acme.corp-gateway" {
		t.Fatalf("the confirmation after the switch: %+v", p)
	}
}

// TestPluginsReadKeepsASwitch checks that a read of the plugins folder that read state.json before a switch was saved does not undo the switch, whichever report the engine takes first, and that one started while the switch is being saved does not either: the app writes state.json, so its own state is newer than such a read's (pluginsScan). The reports are taken from the test's queue in the order each case names.
func TestPluginsReadKeepsASwitch(t *testing.T) {
	const id = "example.anime-ja-zh"
	check := func(t *testing.T, a *app, tt *nsgotest.App, updates chan func(), root string) {
		t.Helper()
		waitUntil(t, tt, a, updates, func() bool { return a.plugins.scanned && !a.plugins.busy && !a.plugins.inboxBusy })
		if a.plugins.state.Enabled(id) || a.plugins.saving {
			t.Fatalf("the switch was undone: %+v", a.plugins.state)
		}
		if state, err := plugins.LoadState(root); err != nil || state.Enabled(id) {
			t.Fatalf("state.json %+v, %v", state, err)
		}
		// A read that starts once the switch is saved takes state.json as it is.
		a.plugins.scanned = false
		onEngine(a, a.pluginsScan)
		waitUntil(t, tt, a, updates, func() bool { return a.plugins.scanned && !a.plugins.inboxBusy })
		if a.plugins.state.Enabled(id) || a.plugins.stateErr != nil {
			t.Fatalf("the read after the save: %+v", a.plugins.state)
		}
	}
	for _, c := range []struct {
		name      string
		readFirst bool
	}{{"saved, then read", false}, {"read, then saved", true}} {
		t.Run(c.name, func(t *testing.T) {
			a, tt, updates, root := pluginsApp(t, pluginGlossary)
			// The read finds the pack on: its report is held until the switch's save reported too.
			read := <-pluginsHeld(a, a.pluginsScan)
			saved := <-pluginsHeld(a, func() { a.SetPluginEnabled(id, false) })
			first, second := saved, read
			if c.readFirst {
				first, second = read, saved
			}
			onEngine(a, first)
			onEngine(a, second)
			check(t, a, tt, updates, root)
		})
	}
	t.Run("read while saving", func(t *testing.T) {
		a, tt, updates, root := pluginsApp(t, pluginGlossary)
		held := pluginsHeld(a, func() {
			a.SetPluginEnabled(id, false)
			a.pluginsScan()
		})
		// Whichever ends first, the read may have found state.json before the save.
		first := <-held
		second := <-held
		onEngine(a, first)
		onEngine(a, second)
		check(t, a, tt, updates, root)
	})
}

// TestPluginTermsAreThePacks checks that a glossary pack's term in the open film's glossary is the pack's: the 术语 tab says so and offers no scope, lock or removal for it, removing it does nothing, and a term the user adds for the same words is the film's own, which comes first.
func TestPluginTermsAreThePacks(t *testing.T) {
	a, _, _, _ := pluginsApp(t, pluginGlossary)
	job := core.Job{ID: "j", Language: "简体中文", SpokenLanguage: "ja", VideoPath: filepath.Join(t.TempDir(), "film.mp4")}
	a.library.Jobs = []core.Job{job}
	a.gloss.video, a.gloss.filmRead = job.VideoPath, true
	pluginsFilmLines(a, job, "お兄ちゃん、待って！")
	// The pack's terms the film's lines do not hold are its translation's alone, not listed.
	if i := slices.IndexFunc(a.FilmTerms(), func(t core.Term) bool { return t.Source == "先輩" }); i >= 0 {
		t.Fatalf("a pack's term no line holds is listed: %+v", a.FilmTerms()[i])
	}
	pack := func() (core.Term, bool) {
		i := slices.IndexFunc(a.FilmTerms(), func(t core.Term) bool { return t.Source == "お兄ちゃん" })
		if i < 0 {
			return core.Term{}, false
		}
		return a.FilmTerms()[i], true
	}
	if term, ok := pack(); !ok || term.Origin != core.OriginPack {
		t.Fatalf("the pack's term %+v, %v", term, ok)
	}
	a.RemoveTerm("お兄ちゃん")
	a.ToggleTermLock("お兄ちゃん")
	a.CycleTermScope("お兄ちゃん")
	if term, ok := pack(); !ok || term.Origin != core.OriginPack || term.Locked || term.Scope != core.ScopeGlobal {
		t.Fatalf("after the tab's changes the pack's term is %+v, %v", term, ok)
	}
	if _, global := a.gloss.store.Terms(""); len(global) != 0 {
		t.Fatalf("the pack's term went into glossary.json: %+v", global)
	}
	*a.NewTermSource(), *a.NewTermTarget() = "お兄ちゃん", "老哥"
	a.AddTerm()
	if term, ok := pack(); !ok || term.Origin != core.OriginUser || term.Scope != core.ScopeFilm || term.Target != "老哥" {
		t.Fatalf("the user's term for the pack's words is %+v, %v", term, ok)
	}
}

// TestPluginsConfirmationFollowsTheLanguage checks that the install confirmation over the main window speaks the interface's language as it changes.
func TestPluginsConfirmationFollowsTheLanguage(t *testing.T) {
	a, tt, updates, _ := pluginsApp(t)
	a.InstallPlugins([]string{pluginFolder(t, pluginGlossary)})
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.pending != nil })
	if !hasText(tt, testlocale.Messages().SettingsPluginsInstallConfirm()) {
		t.Fatalf("the confirmation; texts %q", tt.Texts())
	}
	a.SetInterfaceLanguage("en")
	refresh(tt, a)
	if !hasText(tt, locales.In("en").SettingsPluginsInstallConfirm()) {
		t.Fatalf("the confirmation after the language changed; texts %q", tt.Texts())
	}
}

// pluginDictionary is a dictionary pack's manifest, of id and name, its words of from defined in to, in the file words.tsv.
func pluginDictionary(id, name, from, to string) string {
	return `{"schema": 1, "id": "` + id + `", "kind": "dictionary", "version": "1.0.0", "license": "CC-BY-4.0", "name": {"en": "` + name + `"},
  "dictionary": {"format": "tsv", "files": ["words.tsv"], "from": ["` + from + `"], "to": ["` + to + `"], "attribution": {"en": "` + name + ` © Example"}}}`
}

// pluginJMdict is a Yomitan dictionary pack of Japanese words defined in English, in jmdict.zip.
const pluginJMdict = `{"schema": 1, "id": "example.jmdict", "kind": "dictionary", "version": "1.0.0", "license": "CC-BY-SA-4.0", "name": {"en": "JMdict (English)"},
  "dictionary": {"format": "yomitan-zip", "files": ["jmdict.zip"], "from": ["ja"], "to": ["en"]}}`

// pluginsDictInstalled installs into the app's plugins folder, made at root when it has none yet, the dictionary pack of manifest with its files (name to content), reads the folder again and waits until every dictionary turned on is open.
func pluginsDictInstalled(t *testing.T, a *app, tt *nsgotest.App, updates chan func(), manifest string, files map[string][]byte) {
	t.Helper()
	if a.plugins.root == "" {
		a.plugins.root = filepath.Join(t.TempDir(), "plugins")
	}
	pluginInstalledWith(t, a.plugins.root, manifest, files)
	pluginsClosedAtEnd(t, a, updates)
	pluginsReadAgain(t, a, tt, updates)
}

// pluginsClosedAtEnd closes the app's dictionaries as the test ends, as the app's quit does (pluginsCloseDicts), once the opens still running have reported: a test app never quits, and Windows cannot remove an index file that is still open. It runs before the folders the test made before calling it are removed, as t.Cleanup runs the last registered first, so it is called once the plugins folder exists.
func pluginsClosedAtEnd(t *testing.T, a *app, updates <-chan func()) {
	t.Helper()
	t.Cleanup(func() {
		timeout := time.After(5 * time.Second)
		for {
			var opening int
			onEngine(a, func() { opening = len(a.plugins.opening) })
			if opening == 0 {
				break
			}
			select {
			case fn := <-updates:
				onEngine(a, fn)
				continue
			case <-timeout:
				t.Error("a dictionary was still opening as the test ended")
			}
			break
		}
		onEngine(a, a.pluginsCloseDicts)
	})
}

// pluginsReadAgain reads the plugins folder again and waits until every dictionary turned on is open.
func pluginsReadAgain(t *testing.T, a *app, tt *nsgotest.App, updates chan func()) {
	t.Helper()
	a.plugins.scanned = false
	onEngine(a, a.pluginsScan)
	waitUntil(t, tt, a, updates, func() bool {
		if !a.plugins.scanned || a.plugins.busy {
			return false
		}
		for _, pack := range a.pluginsOn(plugins.KindDictionary) {
			if open := a.plugins.dicts[pack.ID]; open == nil || (open.d == nil && open.err == nil) {
				return false
			}
		}
		return true
	})
}

// TestPluginDictionaryAnswersOffline checks a word looked up on the picture with no AI service set: a dictionary pack turned on answers it, the card saying which dictionary, with its credit, and offering to add the word; the AI service is not asked, and the answer is not kept among the words looked up, which keep the AI service's alone. A word no dictionary has says so, and that the words they lack need an AI service. Turned off, the dictionary answers nothing; uninstalled, its index goes too.
func TestPluginDictionaryAnswersOffline(t *testing.T) {
	_, seen := fakeLLM(t, `{"word":"x","meaning":"x"}`)
	a, tt, updates := vocabTalk(t, media.Tools{Translator: media.ChatTranslator{BaseURL: "https://api.openai.com/v1"}})
	msgs := testlocale.Messages()
	pluginsDictInstalled(t, a, tt, updates, pluginDictionary("example.words", "English words", "en", "zh-Hans"),
		map[string][]byte{"words.tsv": []byte("hello\t/həˈləʊ/\tint. 你好\n")})

	apply(tt, a, func() { a.LookUpWord(0, "Hello") })
	waitUntil(t, tt, a, updates, func() bool { return !a.vocab.lookup.busy })
	l := a.vocab.lookup
	if l.failed || l.entry.Word != "hello" || l.entry.Reading != "/həˈləʊ/" || l.entry.Meaning != "int. 你好" || l.source != "example.words" || len(*seen) != 0 {
		t.Fatalf("the lookup %+v, AI asked %d times", l, len(*seen))
	}
	refresh(tt, a)
	wantText(t, tt, "int. 你好")
	wantText(t, tt, msgs.VocabLookupSourceDictCredit("English words", "English words © Example"))
	if !tt.Exists("id:film-lookup-save") {
		t.Fatal("the card offers no Add to Vocabulary")
	}
	if _, ok := a.vocabCache().Get(vocabTalkQuery(a, "Hello", 0)); ok {
		t.Fatal("the dictionary's answer was kept among the words looked up")
	}

	apply(tt, a, func() { a.LookUpWord(1, "Bye") })
	waitUntil(t, tt, a, updates, func() bool { return !a.vocab.lookup.busy })
	if !a.vocab.lookup.failed || !a.vocab.lookup.needsAI {
		t.Fatalf("a word no dictionary has: %+v", a.vocab.lookup)
	}
	refresh(tt, a)
	wantText(t, tt, msgs.VocabLookupNotInDicts())

	a.SetPluginEnabled("example.words", false)
	waitUntil(t, tt, a, updates, func() bool { return !a.plugins.busy })
	if _, ok := a.plugins.dicts["example.words"]; ok {
		t.Fatal("the dictionary stayed open once turned off")
	}
	apply(tt, a, func() { a.LookUpWord(0, "Hello") })
	if !a.vocab.lookup.failed {
		t.Fatalf("a dictionary turned off answered: %+v", a.vocab.lookup)
	}

	a.SetPluginEnabled("example.words", true)
	pluginsReadAgain(t, a, tt, updates)
	index := a.pluginsIndexDir()
	if names, _ := filepath.Glob(filepath.Join(index, "example.words_*.idx")); len(names) != 1 {
		t.Fatalf("the index: %v", names)
	}
	a.UninstallPlugin("example.words")
	waitUntil(t, tt, a, updates, func() bool { return !a.plugins.busy && a.plugins.scanned && len(a.plugins.packs) == 0 })
	if names, _ := filepath.Glob(filepath.Join(index, "example.words_*.idx")); len(names) != 0 || len(a.plugins.dicts) != 0 {
		t.Fatalf("after the uninstall: indexes %v, open %v", names, a.plugins.dicts)
	}
}

// TestPluginDictionariesMissedAreNamed checks the card of a word the dictionaries asked first did not have, which the AI service then answered (docs/plugin-system.md 7.5): with one dictionary asked it names it, with several it says "your dictionaries"; a word the AI service answers with no dictionary asked says only that it did; and a dictionary whose lookup failed is not counted among those that did not have the word.
func TestPluginDictionariesMissedAreNamed(t *testing.T) {
	srv, _ := fakeLLM(t, `{"word":"bye","meaning":"int. 再见"}`)
	a, tt, updates := vocabTalk(t, media.Tools{Translator: media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m1"}})
	msgs := testlocale.Messages()
	// Each look asks about another word, as the cache keeps the AI service's answers and a word answered from it asks no dictionary.
	look := func(word string) string {
		apply(tt, a, func() { a.LookUpWord(1, word) })
		waitUntil(t, tt, a, updates, func() bool { return !a.vocab.lookup.busy })
		if l := a.vocab.lookup; l.failed || l.entry.Meaning != "int. 再见" || l.source != "" {
			t.Fatalf("the lookup %+v", l)
		}
		refresh(tt, a)
		return tt.Find("id:film-lookup-source").Text
	}
	if got := look("Bye"); got != msgs.VocabLookupSourceAI() {
		t.Fatalf("no dictionary asked: %q", got)
	}
	pluginsDictInstalled(t, a, tt, updates, pluginDictionary("example.words", "English words", "en", "zh-Hans"), map[string][]byte{"words.tsv": []byte("hello\t\t你好\n")})
	if got := look("Byes"); got != msgs.VocabLookupSourceAINotInDict("English words") {
		t.Fatalf("one dictionary asked: %q", got)
	}
	pluginsDictInstalled(t, a, tt, updates, pluginDictionary("example.more", "More words", "en", "zh-Hans"), map[string][]byte{"words.tsv": []byte("river\t\t河\n")})
	if got := look("Byebye"); got != msgs.VocabLookupSourceAINotInDicts() {
		t.Fatalf("two dictionaries asked: %q", got)
	}

	// The first dictionary's index is damaged where it keeps hello, so its lookup of hello fails: only the other is named as not having the word.
	var hash string
	onEngine(a, func() { hash = a.plugins.dicts["example.words"].hash })
	f, err := os.OpenFile(filepath.Join(a.pluginsIndexDir(), dict.IndexName("example.words", hash)), os.O_WRONLY, 0)
	if err == nil {
		_, err = f.WriteAt(bytes.Repeat([]byte{0xff}, 10), 32)
		err = errors.Join(err, f.Close())
	}
	if err != nil {
		t.Fatal(err)
	}
	if got := look("Hello"); got != msgs.VocabLookupSourceAINotInDict("More words") {
		t.Fatalf("one dictionary failed, the other missed: %q", got)
	}
}

// TestPluginDictionaryHintShowsOnce checks the hint to install an offline dictionary on the card of a word looked up (docs/plugin-system.md 7.4): a learner of Japanese with no Japanese dictionary pack sees it the first time, and never again, as settings.json keeps it shown; its button opens Settings › 插件. A word of another language, or a learner with a Japanese dictionary installed, sees none.
func TestPluginDictionaryHintShowsOnce(t *testing.T) {
	srv, _ := fakeLLM(t, `{"word":"hello","meaning":"int. 你好"}`)
	a, tt, updates, item := playerFilm(t, 12, media.Tools{Translator: media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m1"}})
	playerOpenFilm(t, a, tt, updates, item)
	playerLoadFilm(tt, a, 12*time.Second)
	// An English dictionary installed, which a Japanese word does not count as one of its language.
	pluginsDictInstalled(t, a, tt, updates, pluginDictionary("example.words", "English words", "en", "zh-Hans"), map[string][]byte{"words.tsv": []byte("hello\t\t你好\n")})

	apply(tt, a, func() { a.LookUpWord(0, "Hello") })
	if a.vocab.lookup.dictHint || a.plugins.dictHintShown {
		t.Fatal("the hint showed for an English word")
	}
	apply(tt, a, func() {
		a.CloseLookup()
		a.library.Jobs[0].SpokenLanguage = "ja"
		a.LookUpWord(0, "Hello")
	})
	if !a.vocab.lookup.dictHint || !tt.Exists("id:film-lookup-dict-hint") || !hasText(tt, testlocale.Messages().VocabLookupDictHint()) {
		t.Fatalf("no hint for a Japanese word: %+v; texts %q", a.vocab.lookup, tt.Texts())
	}
	var saved savedSettings
	if data, err := os.ReadFile(a.settingsPath); err != nil || json.Unmarshal(data, &saved) != nil || !saved.PluginDictHintShown {
		t.Fatalf("settings.json: %s, %v", data, err)
	}
	tt.Press("id:film-lookup-dict-hint-open")
	refresh(tt, a)
	wantSettings(t, a, settings.SectionPlugins)
	if a.vocab.lookup.open {
		t.Fatal("the card stayed open")
	}
	apply(tt, a, func() { a.prefs.open = false; a.LookUpWord(0, "Hello") })
	if a.vocab.lookup.dictHint {
		t.Fatal("the hint showed twice")
	}

	// A Japanese dictionary installed: no hint, though it was never shown.
	apply(tt, a, func() { a.plugins.dictHintShown = false; a.CloseLookup() })
	pluginsDictInstalled(t, a, tt, updates, pluginDictionary("example.ja-zh", "Japanese words", "ja", "zh-Hans"), map[string][]byte{"words.tsv": []byte("公園\tこうえん\t公园\n")})
	apply(tt, a, func() { a.LookUpWord(0, "Hello") })
	if a.vocab.lookup.dictHint || a.plugins.dictHintShown {
		t.Fatal("the hint showed with a Japanese dictionary installed")
	}
}

// TestPluginDictionaryOpenedAgainAfterAFailure checks a dictionary that fails: one whose index is found damaged by a lookup is closed, its index built again at once, and answers the next lookup; one that could not be opened for a reason that may pass, as a file it could not read, is listed among the page's problems and opened again at the next sync, while one Kando cannot read (dict.ErrEmpty) is not tried again.
func TestPluginDictionaryOpenedAgainAfterAFailure(t *testing.T) {
	a, tt, updates := vocabTalk(t, media.Tools{Translator: media.ChatTranslator{BaseURL: "https://api.openai.com/v1"}})
	const id = "example.words"
	pluginsDictInstalled(t, a, tt, updates, pluginDictionary(id, "English words", "en", "zh-Hans"), map[string][]byte{"words.tsv": []byte("hello\t\t你好\n")})
	var hash string
	onEngine(a, func() { hash = a.plugins.dicts[id].hash })
	// The entry after the header, the only one, is damaged; the header still says the index is whole, so it stays open until a lookup reads the entry.
	f, err := os.OpenFile(filepath.Join(a.pluginsIndexDir(), dict.IndexName(id, hash)), os.O_WRONLY, 0)
	if err == nil {
		_, err = f.WriteAt(bytes.Repeat([]byte{0xff}, 10), 32)
		err = errors.Join(err, f.Close())
	}
	if err != nil {
		t.Fatal(err)
	}
	apply(tt, a, func() { a.LookUpWord(0, "Hello") })
	waitUntil(t, tt, a, updates, func() bool { return !a.vocab.lookup.busy })
	if !a.vocab.lookup.failed {
		t.Fatalf("a damaged index answered: %+v", a.vocab.lookup)
	}
	waitUntil(t, tt, a, updates, func() bool {
		open := a.plugins.dicts[id]
		return open != nil && open.d != nil && len(a.plugins.opening) == 0
	})
	apply(tt, a, func() { a.LookUpWord(0, "Hello") })
	waitUntil(t, tt, a, updates, func() bool { return !a.vocab.lookup.busy })
	if l := a.vocab.lookup; l.failed || l.source != id || l.entry.Meaning != "你好" {
		t.Fatalf("the index built again: %+v", l)
	}

	failed := func(err error) {
		onEngine(a, func() {
			open := a.plugins.dicts[id]
			open.d.Close()
			open.d, open.err = nil, err
		})
	}
	failed(&fs.PathError{Op: "open", Path: "words.tsv", Err: fs.ErrPermission})
	if issues := a.pluginsDictIssues(); len(issues) != 1 {
		t.Fatalf("the dictionary that failed is not listed: %v", issues)
	}
	onEngine(a, a.pluginsSyncDicts)
	waitUntil(t, tt, a, updates, func() bool { open := a.plugins.dicts[id]; return open != nil && open.d != nil })
	if issues := a.pluginsDictIssues(); len(issues) != 0 {
		t.Fatalf("the dictionary opened again is still listed: %v", issues)
	}
	failed(dict.ErrEmpty)
	onEngine(a, a.pluginsSyncDicts)
	if open := a.plugins.dicts[id]; open.d != nil || open.err != dict.ErrEmpty || len(a.plugins.opening) != 0 {
		t.Fatalf("a dictionary Kando cannot read was opened again: %+v", open)
	}
}

// TestPluginDictionaryTurnedOnDuringARead checks that a dictionary turned on while a read of the plugins folder that found it off is under way opens, and answers, though that read reports after the switch was saved: the read keeps the app's state (pluginsScan), so the dictionary is neither closed again nor left without an open.
func TestPluginDictionaryTurnedOnDuringARead(t *testing.T) {
	a, tt, updates := vocabTalk(t, media.Tools{Translator: media.ChatTranslator{BaseURL: "https://api.openai.com/v1"}})
	const id = "example.words"
	pluginsDictInstalled(t, a, tt, updates, pluginDictionary(id, "English words", "en", "zh-Hans"), map[string][]byte{"words.tsv": []byte("hello\t\t你好\n")})
	onEngine(a, func() { a.SetPluginEnabled(id, false) })
	waitUntil(t, tt, a, updates, func() bool { return !a.plugins.busy })

	// The read finds the dictionary off; its report is held apart from the film's until the switch's save and the dictionary's open have reported.
	read := <-pluginsHeld(a, a.pluginsScan)
	onEngine(a, func() { a.SetPluginEnabled(id, true) })
	waitUntil(t, tt, a, updates, func() bool {
		open := a.plugins.dicts[id]
		return !a.plugins.busy && open != nil && open.d != nil
	})
	onEngine(a, read)
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.scanned && !a.plugins.inboxBusy })
	if !a.plugins.state.Enabled(id) || a.plugins.dicts[id] == nil || a.plugins.dicts[id].d == nil {
		t.Fatalf("the dictionary turned on: state %+v, open %+v", a.plugins.state, a.plugins.dicts[id])
	}
	apply(tt, a, func() { a.LookUpWord(0, "Hello") })
	waitUntil(t, tt, a, updates, func() bool { return !a.vocab.lookup.busy })
	if l := a.vocab.lookup; l.failed || l.source != id {
		t.Fatalf("the lookup %+v", l)
	}
}

// TestPluginDictionaryOpenedLateKeepsOnlyTheIndexWanted checks the indexes an open that finishes late leaves: one of a content replaced while it opened goes and the index of the content installed stays; turned off and on again while its content opens, the pack is not opened a second time but takes the dictionary of the open running; and an open that finishes after the pack's folder went, as when it is uninstalled while its index is built, leaves no index of it.
func TestPluginDictionaryOpenedLateKeepsOnlyTheIndexWanted(t *testing.T) {
	a, tt, updates := vocabTalk(t, media.Tools{})
	pluginsDictInstalled(t, a, tt, updates, pluginDictionary("example.words", "English words", "en", "zh-Hans"), map[string][]byte{"words.tsv": []byte("hello\t\t你好\n")})
	const id = "example.words"
	var pack plugins.Pack
	onEngine(a, func() {
		pack = a.plugins.packs[slices.IndexFunc(a.plugins.packs, func(p plugins.Pack) bool { return p.ID == id })]
	})
	index := a.pluginsIndexDir()
	current := filepath.Join(index, dict.IndexName(id, pack.Hash))
	indexes := func(want ...string) {
		t.Helper()
		deadline := time.Now().Add(5 * time.Second)
		for {
			names, _ := filepath.Glob(filepath.Join(index, id+"_*.idx"))
			if slices.Equal(names, want) {
				return
			}
			if time.Now().After(deadline) {
				t.Fatalf("indexes %v, want %v", names, want)
			}
			time.Sleep(10 * time.Millisecond)
		}
	}
	indexes(current)

	const old = "0000000000000000"
	if err := os.WriteFile(filepath.Join(index, dict.IndexName(id, old)), nil, 0o644); err != nil {
		t.Fatal(err)
	}
	onEngine(a, func() { a.pluginsDictOpened(id, old, nil, nil) })
	indexes(current)
	if open := a.plugins.dicts[id]; open == nil || open.d == nil {
		t.Fatalf("the dictionary of the content installed: %+v", open)
	}

	name := dict.IndexName(id, pack.Hash)
	onEngine(a, func() {
		a.pluginsCloseDict(id)
		a.plugins.opening[name] = true
		a.pluginsSyncDicts()
	})
	if open := a.plugins.dicts[id]; open == nil || open.d != nil {
		t.Fatalf("turned on while its content opens: %+v", open)
	}
	d, err := dict.Open(pack, index)
	if err != nil {
		t.Fatal(err)
	}
	onEngine(a, func() { a.pluginsDictOpened(id, pack.Hash, d, nil) })
	if open := a.plugins.dicts[id]; open == nil || open.d != d || a.plugins.opening[name] {
		t.Fatalf("the open running did not give its dictionary: %+v", open)
	}

	onEngine(a, a.pluginsCloseDicts)
	if err := os.RemoveAll(pack.Dir); err != nil {
		t.Fatal(err)
	}
	onEngine(a, func() { a.pluginsDictOpened(id, pack.Hash, nil, nil) })
	indexes()
}

// TestPluginDictionariesAnswerInTheirOrder checks that of two dictionaries that have the word the first in the order Settings › 插件 keeps answers, that the page's buttons move a dictionary up and down, the first unable to move up and the last down, its row moved rather than built again, so its button keeps the focus, and that the order moved is the order lookups follow.
func TestPluginDictionariesAnswerInTheirOrder(t *testing.T) {
	a, tt, updates := vocabTalk(t, media.Tools{Translator: media.ChatTranslator{BaseURL: "https://api.openai.com/v1"}})
	pluginsDictInstalled(t, a, tt, updates, pluginDictionary("example.first", "First words", "en", "zh-Hans"), map[string][]byte{"words.tsv": []byte("hello\t\t第一\n")})
	pluginsDictInstalled(t, a, tt, updates, pluginDictionary("example.second", "Second words", "en", "zh-Hans"), map[string][]byte{"words.tsv": []byte("hello\t\t第二\n")})
	meaning := func() string {
		apply(tt, a, func() { a.LookUpWord(0, "Hello") })
		waitUntil(t, tt, a, updates, func() bool { return !a.vocab.lookup.busy })
		return a.vocab.lookup.entry.Meaning
	}
	if got := meaning(); got != "第一" {
		t.Fatalf("the first dictionary did not answer: %q", got)
	}
	apply(tt, a, a.CloseLookup)

	tt = settingsOpenAt(a, tt, settings.SectionPlugins)
	for id, enabled := range map[string]bool{"settings-plugins-up-example.first": false, "settings-plugins-down-example.first": true, "settings-plugins-up-example.second": true, "settings-plugins-down-example.second": false} {
		if !tt.Exists("id:"+id) || tt.Enabled("id:"+id) != enabled {
			t.Fatalf("%s enabled %v, want %v; texts %q", id, tt.Enabled("id:"+id), enabled, tt.Texts())
		}
	}
	pressed := tt.Find("id:settings-plugins-up-example.second").ID
	tt.Press("id:settings-plugins-up-example.second")
	if i, j := slices.Index(a.plugins.order, "example.second"), slices.Index(a.plugins.order, "example.first"); i < 0 || i > j {
		t.Fatalf("the order after moving up: %q", a.plugins.order)
	}
	// The row moved, not built again, so the button pressed, which has the focus, is still there.
	if got := tt.Find("id:settings-plugins-up-example.second").ID; got != pressed {
		t.Fatalf("the button pressed was built again (view %v, was %v)", got, pressed)
	}
	if tt.Enabled("id:settings-plugins-up-example.second") {
		t.Fatal("the dictionary moved first can still move up")
	}
	if got := meaning(); got != "第二" {
		t.Fatalf("the dictionary moved first did not answer: %q", got)
	}
}

// TestPluginDictionaryAnswersAPhone checks a paired phone's lookup through the desktop (phoneStudy.LookUp) with dictionaries: offline and without an AI service, a Japanese word as it inflects (行った) is found by its dictionary form (行く) in a Yomitan dictionary, though its definitions are in English while the film is translated into Chinese; with an AI service set the AI service answers instead, as only a dictionary in the language the lookup answers in comes before it; a dictionary in that language answers before the AI service, which is not asked; and a read-only phone keeps nothing among the words looked up. Each answer says where it came from (psync.WordSource): the dictionary's name and credit, or the AI service.
func TestPluginDictionaryAnswersAPhone(t *testing.T) {
	a, tt, updates := newTestApp(t, &fakePipeline{})
	onEngine(a, func() { a.prefs.language = "简体中文" })
	bank := `[["行く","いく","v5k-s","v5",10,["to go","to move"],1,""],["公園","こうえん","n","",0,["park"],2,""]]`
	var zb bytes.Buffer
	zw := zip.NewWriter(&zb)
	for name, data := range map[string]string{"index.json": `{"title":"JMdict","revision":"1","format":3}`, "term_bank_1.json": bank} {
		w, err := zw.Create(name)
		if err == nil {
			_, err = w.Write([]byte(data))
		}
		if err != nil {
			t.Fatal(err)
		}
	}
	if err := zw.Close(); err != nil {
		t.Fatal(err)
	}
	pluginsDictInstalled(t, a, tt, updates, pluginJMdict, map[string][]byte{"jmdict.zip": zb.Bytes()})
	// The larger Japanese dictionary is read the first time a Japanese word is looked up, which under the race detector on one CPU takes about as long as a phone's request may: it is read here, outside the request's time.
	segment.Annotated("ja", "公園に行った")
	look := func(caller psync.Caller, word, line string) (psync.WordEntry, error) {
		return phoneCall(t, a, tt, updates, func(ctx context.Context) (psync.WordEntry, error) {
			return phoneStudy{a}.LookUp(ctx, caller, psync.WordLookup{Word: word, Line: line})
		})
	}
	e, err := look(psync.Caller{}, "行った", "公園に行った")
	if err != nil || e.Word != "行く" || e.Reading != "いく" || e.Meaning != "to go; to move" || e.Language != "简体中文" {
		t.Fatalf("offline: %+v, %v", e, err)
	}
	if e.Source != (psync.WordSource{Kind: psync.SourceDictionary, Name: "JMdict (English)", Desktop: phoneSyncName()}) {
		t.Fatalf("offline, the source: %+v", e.Source)
	}

	srv, seen := fakeLLM(t, `{"word":"行く","reading":"いく","meaning":"去"}`)
	onEngine(a, func() {
		a.pipeline = media.Tools{Translator: media.ChatTranslator{BaseURL: srv.URL + "/v1", APIKey: "k", Model: "m1"}}
	})
	if e, err := look(psync.Caller{ReadOnly: true}, "行った", "公園に行った"); err != nil || e.Meaning != "去" || len(*seen) == 0 || e.Source != (psync.WordSource{Kind: psync.SourceAI, Desktop: phoneSyncName()}) {
		t.Fatalf("with an AI service: %+v, %v, asked %d times", e, err, len(*seen))
	}
	if a.vocabCache().Len() != 0 {
		t.Fatal("a read-only phone's lookup was kept")
	}
	asked := len(*seen)

	pluginsDictInstalled(t, a, tt, updates, pluginDictionary("example.ja-zh", "Japanese words", "ja", "zh-Hans"), map[string][]byte{"words.tsv": []byte("公園\tこうえん\t公园\n")})
	if e, err := look(psync.Caller{}, "公園", "公園に行った"); err != nil || e.Meaning != "公园" || len(*seen) != asked || e.Source != (psync.WordSource{Kind: psync.SourceDictionary, Name: "Japanese words", Credit: "Japanese words © Example", Desktop: phoneSyncName()}) {
		t.Fatalf("a dictionary into Chinese: %+v, %v, the AI service asked again", e, err)
	}
	if a.vocabCache().Len() != 0 {
		t.Fatal("a dictionary's answer was kept among the words looked up")
	}
}

// pluginWordlist is a word list pack of words of lang, with the levels b1 (accent) and c1 (warn), in words.tsv.
func pluginWordlist(id, lang string) string {
	return `{"schema": 1, "id": "` + id + `", "kind": "wordlist", "version": "1.0.0", "license": "CC-BY-4.0", "name": {"en": "Levels", "zh-Hans": "等级"},
  "wordlist": {"language": "` + lang + `", "file": "words.tsv", "tiers": [{"id": "b1", "label": {"en": "B1"}, "tone": "accent"}, {"id": "c1", "label": {"en": "C1"}, "tone": "warn"}]}}`
}

// TestPluginWordlistMarksTheLines checks the marks of a film's lines from a word list pack: 字幕外观 offers the list turned on, and once it is chosen the lines are marked in the background, a word of the list in its level's tone, which the picture reads (CueMarks), and the choice is saved. The marks are computed once for the film's key: neither a report of the shelf (shelfChanged), which every job's progress makes, nor the player's progress computes them again. Turned off, the marks go.
func TestPluginWordlistMarksTheLines(t *testing.T) {
	a, tt, updates, item := playerFilm(t, 12, nil)
	a.library.Jobs[0].SpokenLanguage = "en"
	pluginsDictInstalled(t, a, tt, updates, pluginWordlist("example.cefr", "en"), map[string][]byte{"words.tsv": []byte("hello\tb1\nbye\tc1\n")})
	playerOpenFilm(t, a, tt, updates, item)
	playerLoadFilm(tt, a, 12*time.Second)

	apply(tt, a, func() { a.SetMarkWordlist("example.cefr") })
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.marks.lines != nil })
	lines := a.plugins.marks.lines
	want := [][]subtitle.Mark{{{Start: 0, End: 5, Tone: subtitle.ToneAccent}}, {{Start: 0, End: 3, Tone: subtitle.ToneWarn}}}
	if len(lines) != 2 || !slices.Equal(lines[0], want[0]) || !slices.Equal(lines[1], want[1]) {
		t.Fatalf("the marks: %+v", lines)
	}
	if got := a.CueMarks(0); !slices.Equal(got, want[0]) || a.MarkWordlist() != "example.cefr" {
		t.Fatalf("the picture reads %+v, chosen %q", got, a.MarkWordlist())
	}
	var saved savedSettings
	if data, err := os.ReadFile(a.settingsPath); err != nil || json.Unmarshal(data, &saved) != nil || saved.PluginWordlist != "example.cefr" {
		t.Fatalf("settings.json: %s, %v", data, err)
	}

	onEngine(a, a.areasShelfChanged)
	playerPlayTo(tt, a, 1500*time.Millisecond)
	playerPlayTo(tt, a, 4500*time.Millisecond)
	if a.plugins.marking != (pluginsMarkKey{}) || &a.plugins.marks.lines[0] != &lines[0] {
		t.Fatal("the marks were computed again for the same film")
	}

	apply(tt, a, func() { a.SetMarkWordlist("") })
	if a.plugins.marks.lines != nil || a.CueMarks(0) != nil || a.plugins.marking != (pluginsMarkKey{}) {
		t.Fatalf("the marks stayed once turned off: %+v", a.plugins.marks.lines)
	}

	tt = settingsOpenAt(a, tt, settings.SectionLook)
	tt.Press("id:settings-look-tiers")
	if !hasText(tt, "等级") {
		t.Fatalf("the drop-down does not offer the word list; texts %q", tt.Texts())
	}
}

// TestPluginMarksFollowTheVocabulary checks the underline under the words of the vocabulary: a word added is underlined at the next report, and a word swapped for another, which leaves the vocabulary as long as it was, moves the underline to the new word.
func TestPluginMarksFollowTheVocabulary(t *testing.T) {
	a, tt, updates, item := playerFilm(t, 12, nil)
	a.library.Jobs[0].SpokenLanguage = "en"
	playerOpenFilm(t, a, tt, updates, item)
	playerLoadFilm(tt, a, 12*time.Second)
	apply(tt, a, func() {
		a.saved.book.AddWord(saved.Word{Text: "hello", Spoken: "en"})
		a.SetMarkKnown(true)
	})
	if len(a.savedBook().Words) != 1 {
		t.Fatalf("the vocabulary: %+v", a.savedBook().Words)
	}
	hello := [][]subtitle.Mark{{{Start: 0, End: 5, Underline: true}}, nil}
	marked := func(want [][]subtitle.Mark) func() bool {
		return func() bool {
			l := a.plugins.marks.lines
			return len(l) == 2 && slices.Equal(l[0], want[0]) && slices.Equal(l[1], want[1])
		}
	}
	waitUntil(t, tt, a, updates, marked(hello))

	apply(tt, a, func() {
		a.saved.book.RemoveWord("hello", "en")
		a.saved.book.AddWord(saved.Word{Text: "bye", Spoken: "en"})
	})
	playerPlayTo(tt, a, 1500*time.Millisecond)
	waitUntil(t, tt, a, updates, marked([][]subtitle.Mark{nil, {{Start: 0, End: 3, Underline: true}}}))
}

// TestPluginMarksDropAStaleComputation checks that once the key is back to that of the marks shown, marks still being computed for another key are stopped and dropped, so they cannot replace the marks shown when they come; and that a computation started for another key stops the one before it, so only the latest goes on.
func TestPluginMarksDropAStaleComputation(t *testing.T) {
	a, tt, updates, item := playerFilm(t, 12, nil)
	a.library.Jobs[0].SpokenLanguage = "en"
	pluginsDictInstalled(t, a, tt, updates, pluginWordlist("example.cefr", "en"), map[string][]byte{"words.tsv": []byte("hello\tb1\nbye\tc1\n")})
	playerOpenFilm(t, a, tt, updates, item)
	playerLoadFilm(tt, a, 12*time.Second)
	apply(tt, a, func() { a.SetMarkWordlist("example.cefr") })
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.marks.lines != nil })

	stale := &atomic.Bool{}
	onEngine(a, func() {
		a.plugins.marking, a.plugins.stop = pluginsMarkKey{video: "another"}, stale
		a.pluginsMarksFollow()
	})
	if a.plugins.marking != (pluginsMarkKey{}) || a.plugins.stop != nil || !stale.Load() {
		t.Fatalf("the computation for another key is still awaited or was not stopped: %+v, stopped %v", a.plugins.marking, stale.Load())
	}

	stale = &atomic.Bool{}
	onEngine(a, func() {
		a.plugins.marking, a.plugins.stop = pluginsMarkKey{video: "another"}, stale
		a.plugins.prefs.known = true
		a.pluginsMarksFollow()
	})
	if !stale.Load() || a.plugins.stop == nil || a.plugins.stop == stale {
		t.Fatalf("a computation for a new key did not stop the one before it: stopped %v", stale.Load())
	}
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.marks.key.known })
}

// TestPluginMarksForgetTheListAsAChange checks that the marks' hook reports a change when it forgets the word list read, which the plugins page reads for its problems, as when the list chosen is turned off.
func TestPluginMarksForgetTheListAsAChange(t *testing.T) {
	a, tt, updates, item := playerFilm(t, 12, nil)
	a.library.Jobs[0].SpokenLanguage = "en"
	pluginsDictInstalled(t, a, tt, updates, pluginWordlist("example.cefr", "en"), map[string][]byte{"words.tsv": []byte("hello\tb1\n")})
	playerOpenFilm(t, a, tt, updates, item)
	playerLoadFilm(tt, a, 12*time.Second)
	apply(tt, a, func() { a.SetMarkWordlist("example.cefr") })
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.list != nil && a.plugins.marks.lines != nil })

	var changed bool
	onEngine(a, func() {
		a.plugins.prefs.list = ""
		a.plugins.prefs.known = true
		a.plugins.marks.lines = nil
		changed = a.pluginsMarksFollow()
	})
	if a.plugins.list != nil || !changed {
		t.Fatalf("the list read was not forgotten as a change: list %v, changed %v", a.plugins.list != nil, changed)
	}
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.marks.key.known })
}

// TestPluginMarksSaveFailureIsShown checks that a change of the word marks that cannot be saved says so, as the other settings do.
func TestPluginMarksSaveFailureIsShown(t *testing.T) {
	a, tt, _, _ := playerFilm(t, 12, nil)
	if err := os.MkdirAll(a.settingsPath+".tmp", 0o755); err != nil {
		t.Fatal(err)
	}
	apply(tt, a, func() { a.SetMarkReadings(true) })
	if prefix := testlocale.Messages().SettingsSaveFailed(""); !strings.HasPrefix(a.saveErr, prefix) {
		t.Fatalf("save error %q", a.saveErr)
	}
}

// TestPluginMarksSurviveNewLines checks that the marks follow only what they depend on: a translation landing, which rewrites the subtitle file and makes the player read it again into a new slice, keeps the marks shown while the file is read and after, without computing them again; a line whose original changed marks the film again, and while the new marks are computed that line shows none, its old marks not being drawn on the new text, and the other lines keep theirs.
func TestPluginMarksSurviveNewLines(t *testing.T) {
	a, tt, updates, item := playerFilm(t, 12, nil)
	a.library.Jobs[0].SpokenLanguage = "en"
	pluginsDictInstalled(t, a, tt, updates, pluginWordlist("example.cefr", "en"), map[string][]byte{"words.tsv": []byte("hello\tb1\nbye\tc1\n")})
	playerOpenFilm(t, a, tt, updates, item)
	playerLoadFilm(tt, a, 12*time.Second)
	apply(tt, a, func() { a.SetMarkWordlist("example.cefr") })
	waitUntil(t, tt, a, updates, func() bool { return a.plugins.marks.lines != nil })
	lines := a.plugins.marks.lines
	bye := []subtitle.Mark{{Start: 0, End: 3, Tone: subtitle.ToneWarn}}

	translated := strings.Replace(talkSubtitles, "\nBye.\n", "\n再见。\nBye.\n", 1)
	if err := os.WriteFile(a.player.subs.path, []byte(translated), 0o644); err != nil {
		t.Fatal(err)
	}
	apply(tt, a, a.reloadOpenSubtitles)
	onEngine(a, func() { a.pluginsMarksFollow() })
	if a.player.subs.ready || &a.plugins.marks.lines[0] != &lines[0] || a.plugins.marking != (pluginsMarkKey{}) {
		t.Fatalf("the marks went while the subtitles were read again: ready %v, marking %+v", a.player.subs.ready, a.plugins.marking)
	}
	waitUntil(t, tt, a, updates, func() bool { return a.player.subs.ready })
	onEngine(a, func() { a.pluginsMarksFollow() })
	if &a.plugins.marks.lines[0] != &lines[0] || a.plugins.marking != (pluginsMarkKey{}) || !slices.Equal(a.CueMarks(1), bye) {
		t.Fatalf("a translation marked the film again, or the picture lost its marks: marking %+v, cue 1 %+v", a.plugins.marking, a.CueMarks(1))
	}

	onEngine(a, func() {
		machine := slices.Clone(a.player.subs.machine)
		machine[0].Text = "你好。\nBye."
		a.player.subs.machine = machine
		a.playerCorrectSubs()
		a.pluginsMarksFollow()
	})
	if a.plugins.marking == (pluginsMarkKey{}) || a.CueMarks(0) != nil || !slices.Equal(a.CueMarks(1), bye) {
		t.Fatalf("a changed original was not marked again, its old marks stayed on the new text, or an unchanged line lost its marks meanwhile: marking %+v, cue 0 %+v, cue 1 %+v", a.plugins.marking, a.CueMarks(0), a.CueMarks(1))
	}
	waitUntil(t, tt, a, updates, func() bool { return slices.Equal(a.CueMarks(0), bye) })
}
