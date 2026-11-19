package main

import (
	"cmp"
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"slices"
	"strings"
	"sync/atomic"

	"kando/apps/kando/live"
	"kando/apps/kando/locales"
	"kando/apps/kando/plugins"
	"kando/apps/kando/plugins/dict"
	"kando/apps/kando/plugins/script"
	"kando/apps/kando/settings"
	"kando/packages/core"
	"kando/packages/media"

	"github.com/nsgo-dev/nsgo/base"
	ns "github.com/nsgo-dev/nsgo/core"
)

// Plugins: the data packs the user installs from Settings › 插件 (docs/plugin-system.md). The packs are in the plugins folder beside settings.json (pluginsRoot), each in <kind>/<id>, and which are turned on in its state.json (plugins.State), never in settings.json. The app reads the folder off the UI thread once its window is ready (pluginsScan) and again after each install, removal or switch. A pack is installed only after the user confirmed what it is (the install confirmation), whether it came from 从文件安装…, a drop on the page, or the command line's inbox (`kando plugin install`), which the app looks at as it starts and each time it comes to the front. What the packs add: a glossary pack's terms join the global glossary of the films they match (pluginTerms), after the user's own; an ai-service pack's service joins the AI services the app offers (pluginPresets, through llmPresets); a dictionary pack answers lookups, offline, before the AI service (state_plugins_dict.go, vocabResolve); a word list pack draws a line in the color of their level under the words of a film's lines, with readings over them from the dictionaries (state_plugins_marks.go); a site pack's sites, those the user allowed, are known to the built-in browser's 下载视频 (state_plugins_sites.go).

// pluginsUI is the plugins' state. packs and issues are what the last read of the plugins folder found, state and stateErr what state.json held (plugins.LoadState); scanGen counts the reads, so only the last one's answer is kept. order is every pack id the app has seen, in the order it came (savedPlugins), which orders the list and decides which glossary pack's term wins. busy is set while an install, a removal, a switch or a copy into the staging folder runs, and err is why the last one failed; inboxBusy while the inbox is looked at, which keeps none of those from running. pending is the install confirmation, nil while it is closed. gone is the AI service of a pack that went while it was in use (pluginsGoneService). dicts are the dictionary packs opened for lookups, by id (pluginsSyncDicts), and opening the index names (dict.IndexName) of the contents being opened; engine's goroutine only. prefs is how the open film's lines are marked (savedPlugins), marks their marks, marking the key of the marks being computed, zero while none are, and stop what stops that computation (pluginsMarkStop), subs the lines whose hash was taken last and list the word list chosen as last read (state_plugins_marks.go). script is the script plugins' (state_plugins_script.go).
type pluginsUI struct {
	// root is the plugins folder when set, as the tests set it; pluginsRoot otherwise finds it beside settings.json.
	root      string
	packs     []plugins.Pack
	issues    []plugins.Issue
	state     plugins.State
	stateErr  error
	scanned   bool
	scanGen   int
	stateGen  int // the app's own changes of state.json, made or saved, counted; saving is set while one is being saved. A read of the plugins folder that may have read state.json before such a change keeps the app's state (pluginsScan).
	saving    bool
	order     []string
	busy      bool
	inboxBusy bool
	err       error
	pending   *pluginsPending
	gone      pluginsGone
	dicts     map[string]*pluginsDict
	opening   map[string]bool
	prefs     pluginsMarkPrefs
	marks     pluginsMarks
	marking   pluginsMarkKey
	stop      *atomic.Bool
	subs      pluginsSubs
	list      *pluginsList
	// phone is the marks computed for the phones' films (state_plugins_phonemarks.go).
	phone pluginsPhoneMarks
	// dictHintShown is set once the lookup card showed the hint to install an offline dictionary (pluginsDictHint), which shows once.
	dictHintShown bool
	// panels is the 扩展 tab's state (state_plugins_panels.go).
	panels pluginsPanels
	// sites are the site packs turned on with the domains the user allowed, which the browser's 下载视频 knows (state_plugins_sites.go).
	sites  []pluginsSite
	script pluginsScripts
	// task is the run of a task pack the side panel shows, taskGen counts the runs, so the reply of one left is dropped, and taskCancel stops the one on its way; usage is what the task packs spent today, read from usage.json with the first read of the folder (usageRead) and the app's own from then on (state_plugins_tasks.go).
	task       pluginsTask
	taskGen    int
	taskCancel context.CancelFunc
	usage      plugins.Usage
	usageRead  bool
	// export is the sheet that exports the user's glossary or vocabulary as a pack, nil while it is closed (state_plugins_export.go).
	export *pluginsExport
	// exportTarget is the export-template pack the saved lines are exported with, "" for Kando's own Anki notes (savedPlugins.PluginExportTarget).
	exportTarget string
	// index is the community plugin index 浏览插件 shows (state_plugins_index.go).
	index pluginsIndex
	// translate is what became of the translation source chosen, as the app last followed it (state_plugins_translate.go).
	translate pluginsTranslate
}

// pluginsGone is the AI service of a pack that is not offered now, though the endpoint in use is still its own: id is its preset id (pack.<id>) and base its endpoint. The service settings.json saved, when it was a pack's, starts as one, as the packs are read only once the window is ready.
type pluginsGone struct{ id, base string }

// pluginsPending is the install confirmation: the pack staged in staged (plugins.Stage), a script plugin verified and its code's word on what it is checked (pluginsStagedCheck), or why the file could not be staged (err). release gives back the modal flag it holds (holdModal). sites are the domains of a site pack the user ticked, which it is installed allowing (state_plugins_sites.go). listed is set for a pack downloaded from the community index (state_plugins_index.go), which the confirmation says it comes from.
type pluginsPending struct {
	pack    plugins.Pack
	staged  string
	err     error
	release func()
	sites   []string
	listed  bool
}

func newPluginsUI() pluginsUI { return pluginsUI{index: newPluginsIndex()} }

// savedPlugins is the plugins' part of settings.json: preferences alone, never what is installed or turned on, which the plugins folder and its state.json keep. PluginOrder is every pack id seen, in the order it came; an id whose pack is not in the plugins folder now is kept, and the page lists it as missing until the user forgets it. PluginReadings, PluginWordlist and PluginKnownWords are how 字幕外观's 逐词标注 marks the words of the originals (pluginsMarkPrefs): readings, the id of the word list whose levels draw a line under them, kept while that pack is missing, and a dashed line under the words in the vocabulary. PluginDictHintShown is set once the lookup card showed the hint to install an offline dictionary, which shows once (pluginsDictHint). PluginDeveloper is developer mode, which trusts the developer key of this computer, kept in the system's secure store, never here.
type savedPlugins struct {
	PluginOrder         []string `json:"pluginOrder,omitempty"`
	PluginReadings      bool     `json:"pluginReadings,omitempty"`
	PluginWordlist      string   `json:"pluginWordlist,omitempty"`
	PluginKnownWords    bool     `json:"pluginKnownWords,omitempty"`
	PluginDictHintShown bool     `json:"pluginDictHintShown,omitempty"`
	PluginDeveloper     bool     `json:"pluginDeveloper,omitempty"`
	// PluginExportTarget is the export-template pack the saved lines are exported with, "" for Kando's own Anki notes; kept while that pack is missing (state_plugins_templates.go).
	PluginExportTarget string `json:"pluginExportTarget,omitempty"`
	// PluginPanel is the panel of the 扩展 tab chosen last, whichever film is open, as plugin:<plugin id>:<panel id>; kept while its plugin is not running (state_plugins_panels.go).
	PluginPanel string `json:"pluginPanel,omitempty"`
}

// pluginOrderMax is the most ids PluginOrder keeps.
const pluginOrderMax = 1000

// clean trims the ids, drops empty and repeated ones and keeps at most pluginOrderMax, keeping the ids of packs that are not installed now, and trims the word list's id.
func (s *savedPlugins) clean() {
	var out []string
	for _, id := range s.PluginOrder {
		id = strings.TrimSpace(id)
		if id == "" || slices.Contains(out, id) {
			continue
		}
		out = append(out, id)
	}
	if len(out) > pluginOrderMax {
		out = out[:pluginOrderMax]
	}
	s.PluginOrder = out
	s.PluginWordlist = strings.TrimSpace(s.PluginWordlist)
	s.PluginExportTarget = strings.TrimSpace(s.PluginExportTarget)
	s.PluginPanel = strings.TrimSpace(s.PluginPanel)
}

// pluginsHooks: the order of the packs, how the lines are marked and developer mode are kept in settings.json, and the AI service of a pack that went is saved as its pack's; the plugins folder and the copy of the community index kept in it (pluginsIndexPeek) are read once the window is ready, links that ask to install a pack of the index are taken (pluginsIndexOpenLinks), and the inbox looked at again each time the app comes to the front; the open film's lines are marked as it plays and as the app's state changes; the dictionaries and the script plugins close as the app quits. The script plugins hear film.opened after the change that opened a film, job.completed as the shelf changes and llm.changed after the AI service changed; and those running whose grant changed with the app's state, as when Kando went offline, are loaded again.
func (a *app) pluginsHooks() areaHooks {
	return areaHooks{
		restore: func(saved savedSettings) {
			a.plugins.order = slices.Clone(saved.PluginOrder)
			a.plugins.prefs = pluginsMarkPrefs{readings: saved.PluginReadings, list: saved.PluginWordlist, known: saved.PluginKnownWords}
			a.plugins.dictHintShown = saved.PluginDictHintShown
			a.plugins.script.developer = saved.PluginDeveloper
			a.plugins.exportTarget = saved.PluginExportTarget
			a.plugins.panels.chosen = saved.PluginPanel
			if strings.HasPrefix(saved.LLMProvider, llmPackPrefix) {
				a.plugins.gone = pluginsGone{id: saved.LLMProvider, base: saved.LLMBaseURL}
			}
		},
		save: func(saved *savedSettings) {
			saved.PluginOrder = slices.Clone(a.plugins.order)
			// The service of a pack that is not offered now is saved as its pack's, so the next start knows it as the pack's (pluginsGoneService); the settings' save, which comes before (areas), saved it as the service shown.
			if tools, ok := a.pipeline.(media.Tools); ok {
				if id := a.pluginsGoneService(tools.Translator.BaseURL); id != "" {
					saved.LLMProvider = id
				}
			}
			saved.PluginReadings, saved.PluginWordlist, saved.PluginKnownWords = a.plugins.prefs.readings, a.plugins.prefs.list, a.plugins.prefs.known
			saved.PluginDictHintShown = a.plugins.dictHintShown
			saved.PluginDeveloper = a.plugins.script.developer
			saved.PluginExportTarget = a.plugins.exportTarget
			saved.PluginPanel = a.plugins.panels.chosen
		},
		ready: func() {
			a.pluginsScan()
			a.pluginsIndexPeek()
		},
		openLinks: a.pluginsIndexOpenLinks,
		becameActive: func() {
			if a.plugins.scanned {
				a.pluginsCheckInbox()
				a.pluginsCheckRevocations()
			}
		},
		quitStarting: func() {
			a.pluginsPhoneMarksStop()
			a.pluginsCloseDicts()
			a.pluginsCloseScripts()
		},
		shelfChanged: a.pluginsJobsCompleted,
		quitting:     a.pluginsTaskStop,
		// The open film's lines are marked again only when its key changed (pluginsMarksFollow), never on shelfChanged, which every report of a job's progress runs. afterChange catches what changes while the film is paused, as a line corrected or a word added to the vocabulary; playing what changes while it plays without a change of the app's state.
		playing: func() bool {
			a.pluginsPanelsLine()
			return a.pluginsMarksFollow()
		},
		afterChange: func() bool {
			changed := a.pluginsMarksFollow()
			changed = a.pluginsIndexFollow() || changed
			a.pluginsFilmOpened()
			a.pluginsPanelsLine()
			a.pluginsGrantsFollow()
			changed = a.pluginsTranslateFollow() || changed
			// A task's run goes once its film is not the one open (pluginsTaskFollow).
			return a.pluginsTaskFollow() || changed
		},
		// A service the user chose since, other than 自定义, which the gone pack's service shows as, is theirs: the gone pack's is forgotten.
		llmChanged: func() {
			if p := a.prefs.llmProvider; p != "custom" && p != a.plugins.gone.id {
				a.plugins.gone = pluginsGone{}
			}
			a.pluginsSend("llm.changed", func(*pluginsScript) map[string]any { return map[string]any{} })
		},
	}
}

// pluginsRoot is the plugins folder: the one the tests set, else plugins beside settings.json (plugins.DefaultRoot, where the command line looks too); "" without a settings file, as in tests that set none, where there are no plugins.
func (a *app) pluginsRoot() string {
	if a.plugins.root != "" {
		return a.plugins.root
	}
	if a.settingsPath == "" {
		return ""
	}
	return filepath.Join(filepath.Dir(a.settingsPath), "plugins")
}

// pluginsScan reads the plugins folder and its state.json off the UI thread (plugins.Scan, plugins.LoadState), and the first time what the task packs spent today (plugins.LoadUsage), and once they are read looks at the inbox. The app writes state.json itself, so when it changed the state since the read started, or is saving it as the read ends, the state.json read may be older than its own: the read's packs and issues are taken, and the app's state kept.
func (a *app) pluginsScan() {
	root := a.pluginsRoot()
	if root == "" {
		return
	}
	a.plugins.scanGen++
	gen, stateGen, update, day := a.plugins.scanGen, a.plugins.stateGen, a.update, a.pluginsDay()
	go func() {
		packs, issues := plugins.Scan(root)
		state, err := plugins.LoadState(root)
		usage := plugins.LoadUsage(root, day)
		update(func() {
			if a.plugins.scanGen != gen {
				return
			}
			// What the task packs spent is read once; the app counts it from then on and is the file's only writer.
			if !a.plugins.usageRead {
				a.plugins.usage, a.plugins.usageRead = usage, true
			}
			if a.plugins.stateGen != stateGen || a.plugins.saving {
				state, err = a.plugins.state, a.plugins.stateErr
			}
			a.pluginsScanned(packs, issues, state, err)
		})
	}()
}

// pluginsScanned takes what a read of the plugins folder found: the packs, which join the order the first time they are seen, the folders that are not valid packs and state.json. The AI service in use may be a pack's that came or went, so it is kept under its pack's id first and its preset found again after; the dictionaries turned on are opened; then the inbox is looked at.
func (a *app) pluginsScanned(packs []plugins.Pack, issues []plugins.Issue, state plugins.State, err error) {
	a.pluginsKeepLLM()
	p := &a.plugins
	p.packs, p.issues, p.state, p.stateErr, p.scanned = packs, issues, state, err, true
	for _, pack := range packs {
		if !slices.Contains(p.order, pack.ID) {
			p.order = append(p.order, pack.ID)
		}
	}
	a.pluginsFollowLLM()
	a.pluginsSyncDicts()
	a.pluginsSyncSites()
	a.pluginsScriptsScanned()
	a.pluginsCheckInbox()
}

// pluginsFollowLLM finds the preset of the AI service in use again, after a pack that adds services came, went, or was turned on or off. The service chosen stays while it is still offered at the endpoint in use, and 自定义 stays 自定义 (llmProviderFor), so neither a built-in service nor the user's custom endpoint is swapped for a pack's of the same endpoint behind the user's back. A pack's service that is no longer offered shows as the preset of its endpoint, 自定义 for most, and is kept as gone, so llmRemember keeps it under its pack's id rather than as 自定义's; once its pack is offered again it is shown as its own. The app found the service at startup before the plugins folder was read, so the first read brings back the pack's service settings.json saved (pluginsGone).
func (a *app) pluginsFollowLLM() {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return
	}
	base, old := tools.Translator.BaseURL, a.prefs.llmProvider
	if g := a.plugins.gone; g.id != "" && old != g.id && settings.SameEndpoint(base, g.base) {
		if preset, ok := settings.PresetByID(a.llmPresets(a.messages()), g.id); ok && settings.SameEndpoint(preset.BaseURL, base) {
			a.prefs.llmProvider, a.plugins.gone = g.id, pluginsGone{}
			return
		}
	}
	next := a.llmProviderFor(old, base)
	if strings.HasPrefix(old, llmPackPrefix) && next != old {
		a.plugins.gone = pluginsGone{id: old, base: base}
	}
	a.prefs.llmProvider = next
}

// pluginsGoneService is the id the service at base is kept under while it is the service of a pack that is not offered now (pluginsGone), shown as 自定义; "" otherwise.
func (a *app) pluginsGoneService(base string) string {
	g := a.plugins.gone
	if g.id == "" || a.prefs.llmProvider != "custom" || !settings.SameEndpoint(base, g.base) {
		return ""
	}
	return g.id
}

// pluginsKeepLLM keeps the AI service in use under its pack's id (llmRemember) before the packs change, while its preset is still offered, so the key the user gave it is there when the pack comes back.
func (a *app) pluginsKeepLLM() {
	if strings.HasPrefix(a.prefs.llmProvider, llmPackPrefix) {
		a.llmRemember()
	}
}

// pluginsOn are the packs of kind that are turned on, in the order the settings keep.
func (a *app) pluginsOn(kind plugins.Kind) []plugins.Pack {
	var out []plugins.Pack
	for _, id := range a.plugins.order {
		i := slices.IndexFunc(a.plugins.packs, func(p plugins.Pack) bool { return p.ID == id })
		if i < 0 {
			continue
		}
		if pack := a.plugins.packs[i]; pack.Kind == kind && a.plugins.state.EnabledPack(pack) {
			out = append(out, pack)
		}
	}
	return out
}

// pluginTerms are the terms of the glossary packs turned on that are for job: packs into the language job translates into (its Language, or RedoLanguage while it translates into another, or the settings' language for a job that names none, as queueRun gives it once its glossary is found) and, once its speech was heard or named, from the language it is in; a job whose target Kando does not know gets none. They are every film's terms and the pack's (core.OriginPack), so a term the user has for the same words, and a locked one, comes first (plugins.AddTerms).
func (a *app) pluginTerms(job core.Job) []core.Term {
	target := core.TargetTag(cmp.Or(strings.TrimSpace(job.RedoLanguage), strings.TrimSpace(job.Language), strings.TrimSpace(a.prefs.language)))
	if target == "" {
		return nil
	}
	spoken := cmp.Or(job.SpokenLanguage, job.SpeechLanguage)
	var out []core.Term
	for _, pack := range a.pluginsOn(plugins.KindGlossary) {
		g := pack.Glossary
		if !plugins.SameLanguage(g.To, target) || (spoken != "" && core.BaseLanguage(g.From) != core.BaseLanguage(spoken)) {
			continue
		}
		for _, t := range g.Terms {
			out = append(out, core.Term{Source: t.Source, Target: t.Target, Scope: core.ScopeGlobal, Origin: core.OriginPack})
		}
	}
	return out
}

// pluginPresets are the AI services of the ai-service packs turned on, as presets whose id is pack.<id> (llmPackPrefix), their names and blurbs in the language of m. A pack fills in the endpoint, model and key page; the key is the user's, kept with the service as any other's (savedLLM).
func (a *app) pluginPresets(m locales.Messages) []settings.Preset {
	var out []settings.Preset
	for _, pack := range a.pluginsOn(plugins.KindAIService) {
		s := pack.Service
		group := settings.GroupCloud
		if s.Group == plugins.GroupLocal {
			group = settings.GroupLocal
		}
		out = append(out, settings.Preset{ID: llmPackPrefix + pack.ID, Name: pack.Name.In(m.Language()), Blurb: pack.Description.In(m.Language()), Group: group,
			BaseURL: s.BaseURL, Model: s.Model, KeyURL: s.KeyURL, Plugin: pack.ID})
	}
	return out
}

// pluginsOpenKeyPage opens the key page of the service preset, which a pack added, once the user agreed to go there: a system alert names the page's host and the pack first, as the address is the pack's, not Kando's.
func (a *app) pluginsOpenKeyPage(preset settings.Preset) {
	if a.cx == nil {
		return
	}
	m := a.messages()
	open := m.SettingsAIKeyPageOpen()
	release := a.holdModal()
	base.ShowAlertDialog(a.cx, base.AlertDialog{
		Title:       m.SettingsAIKeyPageTitle(settings.Host(preset.KeyURL)),
		Description: m.SettingsAIKeyPageMessage(preset.Name),
		Actions:     []ns.AlertAction{{Label: m.SettingsPluginsCancel(), Role: ns.AlertCancel}, {Label: open}},
	}, func(label string) {
		release()
		if label == open {
			a.menuOpenLink(preset.KeyURL)
		}
		a.Changed()
	})
}

// pluginsCheckInbox stages the oldest file the command line dropped into the inbox (plugins.Inbox), off the UI thread, and opens the install confirmation for it; the file leaves the inbox once it is asked about, whether it can be installed or not, so one that cannot is said once. It waits while a confirmation is open or another install runs, which look at the inbox again as they end; one that started while it looked goes first, and the file stays in the inbox for the next look. Looking sets inboxBusy, not busy, so the page's switches and buttons work meanwhile.
func (a *app) pluginsCheckInbox() {
	root := a.pluginsRoot()
	if root == "" || a.plugins.busy || a.plugins.inboxBusy || a.plugins.pending != nil {
		return
	}
	a.plugins.inboxBusy = true
	update, verify := a.update, a.pluginsStagedVerifier()
	go func() {
		files, err := plugins.Inbox(root)
		if err != nil || len(files) == 0 {
			update(func() { a.plugins.inboxBusy = false })
			return
		}
		file := files[0]
		staged, pack, err := plugins.Stage(root, file)
		v, err := pluginsVerifyStaged(verify, pack, err)
		update(func() {
			a.plugins.inboxBusy = false
			if a.plugins.busy || a.plugins.pending != nil {
				if staged != "" {
					go os.RemoveAll(staged)
				}
				return
			}
			// The file leaves the inbox here, once it is asked about, so a look that yielded leaves it for the next; removing a file is quick enough for the engine's goroutine, and the inbox is empty as the confirmation opens.
			os.Remove(file)
			a.pluginsAskStaged(pack, staged, v, err)
		})
	}()
}

// pluginsStage stages the pack at src, a zip or a folder, off the UI thread, and opens the install confirmation for it, or says why it cannot be installed.
func (a *app) pluginsStage(src string) { a.pluginsStageThen(src, nil) }

// pluginsStageThen is pluginsStage, running after, when set, once src was copied, as to remove the folder a developer plugin was packed into. A script plugin's package is verified off the UI thread too, and its code's word on what it is checked as the confirmation opens (pluginsAskStaged).
func (a *app) pluginsStageThen(src string, after func()) {
	root := a.pluginsRoot()
	if root == "" || a.plugins.busy || a.plugins.pending != nil {
		if after != nil {
			go after()
		}
		return
	}
	a.plugins.busy, a.plugins.err = true, nil
	update, verify := a.update, a.pluginsStagedVerifier()
	go func() {
		staged, pack, err := plugins.Stage(root, src)
		if after != nil {
			after()
		}
		v, err := pluginsVerifyStaged(verify, pack, err)
		update(func() {
			a.plugins.busy = false
			a.pluginsAskStaged(pack, staged, v, err)
		})
	}()
}

// pluginsVerifyStaged verifies the pack staged, unless staging failed with err, off the UI thread.
func pluginsVerifyStaged(verify func(plugins.Pack) (script.Verified, error), pack plugins.Pack, err error) (script.Verified, error) {
	if err != nil {
		return script.Verified{}, err
	}
	return verify(pack)
}

// pluginsAskStaged opens the install confirmation of the pack staged in staged, after a script plugin passed the checks of the engine's goroutine (pluginsStagedCheck), or says why it cannot be installed, throwing the staged copy away.
func (a *app) pluginsAskStaged(pack plugins.Pack, staged string, v script.Verified, err error) {
	if err == nil {
		err = a.pluginsStagedCheck(&pack, v)
	}
	if err != nil && staged != "" {
		go os.RemoveAll(staged)
		staged = ""
	}
	a.pluginsAsk(&pluginsPending{pack: pack, staged: staged, err: err})
}

// pluginsAsk opens the install confirmation of p, holding the window's modal flag until it closes (pluginsClose). A site pack's domains start ticked only where the user allowed them for the pack of its id installed now (pluginsTickedBefore).
func (a *app) pluginsAsk(p *pluginsPending) {
	if p.err == nil {
		p.sites = a.pluginsTickedBefore(p.pack)
	}
	p.release = a.holdModal()
	a.plugins.pending = p
}

// pluginsClose closes the install confirmation, gives back its modal flag and throws its staged copy away unless keep is set, as when it was installed; then looks at the inbox for the next file. It cannot be cancelled while busy, as while its install runs: plugins.Swap is moving the staged copy into place then, and throwing it away would leave a pack the user cancelled installed, or a broken one in place of the version it replaced.
func (a *app) pluginsClose(keep bool) {
	p := a.plugins.pending
	if p == nil || (!keep && a.plugins.busy) {
		return
	}
	a.plugins.pending = nil
	p.release()
	if p.staged != "" && !keep {
		go os.RemoveAll(p.staged)
	}
	a.pluginsCheckInbox()
}

// pluginsInstall installs the pack of the open confirmation off the UI thread (plugins.Swap), turned on for its content (state.json), a site pack allowing the domains ticked in it and no other, a task pack off until the user turns it on, and reads the plugins folder again. A pack of the same id is replaced; one of another kind with the id is not (plugins.ErrIDTaken). When the install fails the confirmation stays open and says why, as it may show over the main window with the 插件 page nowhere in sight. A state.json a newer Kando wrote, which this one does not write over (plugins.ErrStateFormat), refuses the install before anything is replaced; once the pack is in place it is installed, and a state.json that could not be saved after is said on the page, apart from the install.
func (a *app) pluginsInstall() {
	p := a.plugins.pending
	root := a.pluginsRoot()
	if p == nil || p.err != nil || root == "" || a.plugins.busy {
		return
	}
	if errors.Is(a.plugins.stateErr, plugins.ErrStateFormat) {
		p.err = a.plugins.stateErr
		return
	}
	if i := slices.IndexFunc(a.plugins.packs, func(o plugins.Pack) bool { return o.ID == p.pack.ID }); i >= 0 && a.plugins.packs[i].Kind == plugins.KindAIService {
		a.pluginsKeepLLM()
	}
	pack, staged := p.pack, p.staged
	// The dictionary or the script of the pack it replaces is closed first; the read of the folder after opens it again.
	a.pluginsCloseDict(pack.ID)
	a.pluginsCloseScript(pack.ID)
	// A task pack spends the user's tokens, so it is installed off, a new content of it too, until the user turns it on (plugins.State.EnabledPack); its budget is kept.
	prev, _ := a.plugins.state.Entry(pack.ID)
	entry := plugins.PluginState{ID: pack.ID, Hash: pack.Hash, Enabled: pack.Kind != plugins.KindTask, Sites: pluginsInstallSites(p), Budget: prev.Budget}
	var old *plugins.Pack
	if i := slices.IndexFunc(a.plugins.packs, func(o plugins.Pack) bool { return o.ID == pack.ID }); i >= 0 {
		old = &a.plugins.packs[i]
	}
	if pack.Kind == plugins.KindScript {
		e, had := a.plugins.state.Entry(pack.ID)
		entry = plugins.InstallEntry(pack, old, e, had)
		// Another key signed it: what the one before kept is not this one's (docs/plugin-system.md 6.2). The signer is the one state.json recorded, as the pack replaced may not verify now.
		if old != nil {
			signer := old.Signer
			if had && e.Signer != "" {
				signer = e.Signer
			}
			if signer != "" && signer != pack.Signer {
				a.pluginsForgetPartitions(*old, signer)
			}
		}
		a.plugins.script.health = plugins.SetHealth(a.plugins.script.health, plugins.Health{ID: pack.ID})
		a.pluginsSaveHealth()
		a.pluginsAudit(plugins.AuditInstall, pack.ID, pack.Version+" "+string(pack.Source)+" "+pack.Signer)
	}
	state := a.pluginsNextState(func(s *plugins.State) { s.Set(entry) })
	a.plugins.busy, a.plugins.err, a.plugins.saving = true, nil, true
	update := a.update
	go func() {
		err := plugins.Swap(root, staged, &pack)
		var saveErr error
		if err == nil {
			saveErr = state.Save(root)
		}
		update(func() {
			a.plugins.busy, a.plugins.saving = false, false
			a.plugins.stateGen++
			switch {
			case err != nil && a.plugins.pending == p:
				p.err = err
			case err != nil:
				a.plugins.err = err
			default:
				if saveErr != nil {
					a.plugins.err = plugins.StateUnsaved{Err: saveErr}
				}
				if !slices.Contains(a.plugins.order, pack.ID) {
					a.plugins.order = append(a.plugins.order, pack.ID)
				}
				if a.plugins.pending == p {
					a.pluginsClose(true)
				}
			}
			a.pluginsScan()
		})
	}()
}

// pluginsSetEnabled turns the pack id on or off, saving state.json off the UI thread (pluginsApplyState); a site pack keeps the domains the user allowed.
func (a *app) pluginsSetEnabled(id string, on bool) {
	i := slices.IndexFunc(a.plugins.packs, func(p plugins.Pack) bool { return p.ID == id })
	if a.pluginsRoot() == "" || i < 0 || a.plugins.busy {
		return
	}
	if a.plugins.packs[i].Kind == plugins.KindScript {
		a.pluginsScriptSetEnabled(a.plugins.packs[i], on)
		return
	}
	hash := a.plugins.packs[i].Hash
	a.pluginsKeepLLM()
	next := a.pluginsNextState(func(s *plugins.State) {
		e, _ := s.Entry(id)
		e.ID, e.Hash, e.Enabled = id, hash, on
		s.Set(e)
	})
	a.pluginsApplyState(next)
}

// pluginsApplyState takes next as state.json, a pack turned on or off or a site allowed, saving it off the UI thread; what follows the packs turned on (the AI services offered, the dictionaries open) follows at once, and goes back when the file could not be saved; the sites the browser knows follow once it is saved, as a change of them makes the browser's web view again (browseSitesChanged), which a failed save would do twice. Once the save ends, the inbox is looked at again, as a look at it gives way while the save runs (pluginsCheckInbox).
func (a *app) pluginsApplyState(next plugins.State) {
	root := a.pluginsRoot()
	before := a.plugins.state
	a.plugins.state, a.plugins.busy, a.plugins.err, a.plugins.saving = next, true, nil, true
	a.plugins.stateGen++
	a.pluginsFollowLLM()
	a.pluginsSyncDicts()
	update := a.update
	go func() {
		err := next.Save(root)
		update(func() {
			a.plugins.busy, a.plugins.saving = false, false
			a.plugins.stateGen++
			defer a.pluginsCheckInbox()
			if err != nil {
				a.plugins.state, a.plugins.err = before, err
				a.pluginsFollowLLM()
				a.pluginsSyncDicts()
				return
			}
			a.plugins.state, a.plugins.stateErr = next, nil
			a.pluginsSyncSites()
		})
	}()
}

// pluginsUninstall removes the pack id from the plugins folder and from state.json off the UI thread (plugins.Remove), with a dictionary's index, closed first, and reads the folder again. The service an ai-service pack added keeps its key in settings.json under the pack's id (pluginsKeepLLM, savedLLM.clean), so installing it again brings it back. As with an install, a state.json a newer Kando wrote, which this one does not write over (plugins.ErrStateFormat), refuses the removal before anything is removed; once the folder is gone the pack is uninstalled, and a state.json that could not be saved after is said on the page apart from it (plugins.StateUnsaved).
func (a *app) pluginsUninstall(id string) {
	root := a.pluginsRoot()
	if root == "" || a.plugins.busy {
		return
	}
	if errors.Is(a.plugins.stateErr, plugins.ErrStateFormat) {
		a.plugins.err = fmt.Errorf("%w: %w", plugins.ErrUninstallRefused, a.plugins.stateErr)
		return
	}
	a.pluginsKeepLLM()
	a.pluginsCloseDict(id)
	if i := slices.IndexFunc(a.plugins.packs, func(p plugins.Pack) bool { return p.ID == id }); i >= 0 && a.plugins.packs[i].Kind == plugins.KindScript {
		pack := a.plugins.packs[i]
		a.pluginsCloseScript(id)
		// The signer state.json recorded, as the plugin may not verify now; its own when it never had an entry.
		signer := pack.Signer
		if e, ok := a.plugins.state.Entry(id); ok && e.Signer != "" {
			signer = e.Signer
		}
		a.pluginsForgetPartitions(pack, signer)
		delete(a.plugins.script.values, id)
		a.plugins.script.health = plugins.SetHealth(a.plugins.script.health, plugins.Health{ID: id})
		a.pluginsSaveHealth()
		pluginsWriteValues(id, func() { plugins.RemoveValues(root, id) })
		a.pluginsAudit(plugins.AuditUninstall, id, "")
	}
	state := a.pluginsNextState(func(s *plugins.State) {
		s.Plugins = slices.DeleteFunc(s.Plugins, func(e plugins.PluginState) bool { return e.ID == id })
	})
	a.plugins.saving = true
	a.plugins.busy, a.plugins.err = true, nil
	update, index := a.update, a.pluginsIndexDir()
	go func() {
		err := plugins.Remove(root, id)
		if err == nil {
			dict.Forget(index, id, "")
			if saveErr := state.Save(root); saveErr != nil {
				err = plugins.StateUnsaved{Err: saveErr}
			}
		}
		update(func() {
			a.plugins.busy, a.plugins.saving = false, false
			a.plugins.stateGen++
			if err != nil {
				a.plugins.err = err
			}
			// What a task pack spent goes with it once its folder is gone, so installing it again starts from its budget.
			var unsaved plugins.StateUnsaved
			if _, ok := a.plugins.usage.Tokens[id]; ok && (err == nil || errors.As(err, &unsaved)) {
				a.plugins.usage.Forget(id)
				if err := a.plugins.usage.Save(root); err != nil {
					a.plugins.err = err
				}
			}
			a.pluginsScan()
		})
	}()
}

// pluginsForget forgets the missing pack id, which leaves the list.
func (a *app) pluginsForget(id string) {
	if slices.ContainsFunc(a.plugins.packs, func(p plugins.Pack) bool { return p.ID == id }) {
		return
	}
	a.plugins.order = slices.DeleteFunc(a.plugins.order, func(o string) bool { return o == id })
}

// pluginsNextState is state.json with change made to a copy of it. While the file read as every pack off, as when it was damaged (plugins.ErrBadState), each pack the folder holds is first given an entry turned off, as the file's first save would otherwise turn on every pack it has no entry for (plugins.State.Save).
func (a *app) pluginsNextState(change func(s *plugins.State)) plugins.State {
	s := a.plugins.state
	s.Plugins = slices.Clone(s.Plugins)
	if errors.Is(a.plugins.stateErr, plugins.ErrBadState) {
		for _, pack := range a.plugins.packs {
			if !slices.ContainsFunc(s.Plugins, func(e plugins.PluginState) bool { return e.ID == pack.ID }) {
				s.Plugins = append(s.Plugins, plugins.PluginState{ID: pack.ID, Hash: pack.Hash, Enabled: false})
			}
		}
	}
	change(&s)
	return s
}

// pluginsShowFolder shows the plugins folder in the file manager, making it first when no pack was ever installed, and says on the page when no file manager showed it.
func (a *app) pluginsShowFolder() {
	root := a.pluginsRoot()
	if root == "" || a.cx == nil {
		return
	}
	a.plugins.err = nil
	update := a.update
	go func() {
		err := os.MkdirAll(root, 0o755)
		update(func() {
			if err != nil {
				a.plugins.err = err
				return
			}
			revealInFolder(a.cx, root, live.On(a, func(err error) {
				if err != nil {
					a.plugins.err = errPluginsFolderNotShown
				}
			}))
		})
	}()
}

// errPluginsFolderNotShown is why the plugins folder did not show: no file manager opened it.
var errPluginsFolderNotShown = errors.New("no file manager showed the plugins folder")

// pluginsPickFile asks for a pack file with the system's file panel and stages the one picked (pluginsStage).
func (a *app) pluginsPickFile() {
	if a.cx == nil || a.plugins.busy || a.plugins.pending != nil {
		return
	}
	release := a.holdModal()
	m := a.messages()
	// The answer belongs to the app, not to the button that asked, which may be gone by then (core.Scope.Run).
	a.cx.Run(func() {
		a.cx.PickFiles(ns.PickFilesOptions{Types: []string{".zip", ".kandopack", ".kandoplugin"}, TypesName: m.SettingsPluginsFileTypes(), Title: m.SettingsPluginsPickTitle(), Button: m.SettingsPluginsPickButton()}, func(files []ns.File, err error) {
			release()
			defer a.Changed()
			if err != nil {
				a.plugins.err = err
				return
			}
			if len(files) > 0 && files[0].Path != "" {
				a.pluginsStage(files[0].Path)
			}
		})
	})
}

// pluginsOverlay is the script plugins' toasts, the sheet that exports the glossary or the vocabulary as a pack (state_plugins_export.go) and the install confirmation over the main window (overlays), the confirmation while the 插件 page in a settings window of its own does not draw it (settings.PluginsApp.PluginInstallHere): a pack from the command line's inbox, or one picked while the settings show over the main window.
func (a *app) pluginsOverlay(cx *ns.Scope) ns.Node {
	return ns.Group{
		settings.PluginInstallDialog{App: a, Open: live.Func(a, func() bool { return a.plugins.pending != nil && !a.PluginInstallHere() })},
		settings.PluginExportDialog{App: a, Open: live.Func(a, func() bool { return a.plugins.export != nil })},
		a.pluginsToasts(cx),
	}
}
