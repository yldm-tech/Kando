package main

import (
	"archive/zip"
	"bytes"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	"kando/apps/kando/locales"
	"kando/apps/kando/locales/testlocale"
	"kando/apps/kando/plugins"
	"kando/apps/kando/settings"
	"kando/packages/core"
	"kando/packages/media"

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
	if !tt.Exists("id:plugins-install-failed") || !hasText(tt, testlocale.Messages().PluginsErrUnsupportedKind()) {
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
	korean := job
	korean.SpokenLanguage = "ko"
	if _, terms := a.glossaryStart(korean); target(terms, "お兄ちゃん") != "" {
		t.Errorf("a job heard in Korean got the pack's terms: %+v", terms)
	}

	a.library.Jobs = []core.Job{job}
	a.gloss.video = job.VideoPath
	if target(a.FilmTerms(), "お兄ちゃん") != "哥哥" {
		t.Errorf("the open film's glossary %+v", a.FilmTerms())
	}

	a.SetPluginEnabled("example.anime-ja-zh", false)
	waitUntil(t, tt, a, updates, func() bool { return !a.plugins.busy })
	if _, terms := a.glossaryStart(job); target(terms, "お兄ちゃん") != "" {
		t.Errorf("a pack turned off added %+v", terms)
	}
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

// TestPluginTermsAreThePacks checks that a glossary pack's term in the open film's glossary is the pack's: the 术语 tab says so and offers no scope, lock or removal for it, removing it does nothing, and a term the user adds for the same words is the film's own, which comes first.
func TestPluginTermsAreThePacks(t *testing.T) {
	a, _, _, _ := pluginsApp(t, pluginGlossary)
	job := core.Job{ID: "j", Language: "简体中文", SpokenLanguage: "ja", VideoPath: filepath.Join(t.TempDir(), "film.mp4")}
	a.library.Jobs = []core.Job{job}
	a.gloss.video, a.gloss.filmRead = job.VideoPath, true
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
