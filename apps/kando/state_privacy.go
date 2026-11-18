package main

import (
	"os"
	"path/filepath"

	"kando/apps/kando/cache"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings"
	"kando/apps/kando/settings/sectionid"
	"kando/apps/kando/sidebar"
	"kando/packages/core"
	"kando/packages/media"
)

// privacyUI is Settings › 隐私: offline mode, which stops every step that would use the network, and 清除缓存.
//
// Offline mode is one switch, offline, which the pipeline carries as media.Tools.Offline, so a download from a link, a list of a playlist's videos and a request to an AI service off this computer are refused where they would start (media.ErrOffline). The app refuses the rest before it starts them: adding a link (queueTake, precheck), a queued download's turn (queueCanRun, which holds it until offline mode is off), an upload, a sign-in, the update check the user starts, a download of a program or a speech model, yt-dlp's update, a site's thumbnail and a web page. Turning it on stops what is under way over the network (privacyStopNetwork). An AI service on this computer, as Ollama or LM Studio, goes on (media.ChatTranslator.Local). Phone Sync stays: it serves the local network only.
type privacyUI struct {
	offline bool
	// clear is where 清除缓存 stands; clearGen counts the clears, so only the last one's answer is kept.
	clear    settings.CacheClear
	clearGen int
}

func newPrivacyUI() privacyUI { return privacyUI{} }

// savedPrivacy is offline mode's part of settings.json.
type savedPrivacy struct {
	Offline bool `json:"offline,omitempty"`
}

func (s *savedPrivacy) clean() {}

// privacyHooks: offline mode is saved with the settings and given back to the pipeline at startup, before any job runs.
func (a *app) privacyHooks() areaHooks {
	return areaHooks{
		save: func(saved *savedSettings) { saved.savedPrivacy = savedPrivacy{Offline: a.privacy.offline} },
		restore: func(saved savedSettings) {
			a.privacy.offline = saved.Offline
			a.privacyApply()
		},
	}
}

// privacyApply gives the pipeline offline mode as it is. Every change of the tools starts from the pipeline's, so they keep it.
func (a *app) privacyApply() {
	tools, ok := a.pipeline.(media.Tools)
	if !ok {
		return
	}
	tools.Offline = a.privacy.offline
	tools.Translator.Offline = a.privacy.offline
	a.pipeline = tools
}

// privacySetOffline turns offline mode on or off and saves it. Turned on, it stops what is under way over the network; turned off, the downloads it held go on.
func (a *app) privacySetOffline(on bool) {
	if a.privacy.offline == on {
		return
	}
	a.privacy.offline = on
	a.privacyApply()
	if on {
		a.privacyStopNetwork()
	}
	a.checkReadiness(false)
	if err := a.saveSettings(); err != nil {
		a.saveErr = a.messages().SettingsPrivacySaveFailed(err.Error())
	}
	if !on {
		a.pump()
	}
}

// privacyStopNetwork stops what uses the network as offline mode comes on: the running job while it downloads, or while it translates or proofreads through a service off this computer (it is marked 已取消, and 重试 picks up from where it stopped), an upload, the downloads of a speech model or a program, and the browser's page.
func (a *app) privacyStopNetwork() {
	if id := a.queue.active; id != "" {
		if job, ok := a.library.Find(id); ok && a.privacyNeedsNetwork(job.Stage) {
			a.Cancel(id)
		}
	}
	a.CancelUpload()
	if a.prefs.downloading != "" {
		a.CancelDownload()
	}
	for _, job := range a.kits.running {
		job.cancel()
	}
	// The browser leaves the page it shows for its start, whose sites offline mode does not open (browseLoad).
	if a.browse.source != "" && a.browse.url != browseBlank {
		a.browseHome()
	}
}

// privacyNeedsNetwork reports whether a job at stage uses the network now: it downloads, or it translates or proofreads through an AI service that is not on this computer.
func (a *app) privacyNeedsNetwork(stage core.Stage) bool {
	switch stage {
	case core.StageDownloading:
		return true
	case core.StageTranslating:
		return !a.privacyLocalAI()
	}
	return false
}

// privacyLocalAI reports whether the AI service in use runs on this computer, which offline mode lets through.
func (a *app) privacyLocalAI() bool {
	tools, ok := a.pipeline.(media.Tools)
	return ok && tools.Translator.Local()
}

// privacyHolds reports whether offline mode holds job in the queue: a job that would download its video first, which waits until offline mode is off rather than failing. A local video, a video already downloaded and a done film translated again need no download.
func (a *app) privacyHolds(job core.Job) bool {
	if !a.privacy.offline || job.Redo.TranslatesOnly() || queueLocalPath(job.Source) != "" {
		return false
	}
	return job.VideoPath == "" || !fileExists(job.VideoPath)
}

// privacyRefusesLink is why source cannot be added while offline mode is on, in the interface's language: a link would be downloaded. nil for a local video, or while offline mode is off.
func (a *app) privacyRefusesLink(source string) error {
	if !a.privacy.offline || queueLocalPath(source) != "" {
		return nil
	}
	return errOffline(a.messages().PrivacyNoDownload())
}

// offlineError is a step offline mode refused, with what the app says of it in the interface's language. It is media.ErrOffline, so errors.Is tells it from other failures.
type offlineError struct{ msg string }

func errOffline(msg string) error           { return offlineError{msg: msg} }
func (e offlineError) Error() string        { return e.msg }
func (e offlineError) Is(target error) bool { return target == media.ErrOffline }

// privacyReadiness is the sidebar's status line while offline mode is on, in place of ready: it says Kando uses only what runs on this computer, or, when the AI service is in the cloud, that it will not translate until another service is chosen or offline mode is off.
func (a *app) privacyReadiness(status sidebar.Readiness) sidebar.Readiness {
	if !a.privacy.offline || status.State != sidebar.Ready {
		return status
	}
	if _, ok := a.pipeline.(media.Tools); ok && !a.privacyLocalAI() {
		return sidebar.Readiness{State: sidebar.Missing, Message: locales.PrivacyNoCloudAI, Section: settings.SectionAI}
	}
	return sidebar.Readiness{State: sidebar.Ready, Message: locales.ReadinessOffline, Section: sectionid.Privacy}
}

// privacyClearCache removes the caches Kando makes again when it needs them, off the UI thread: the posters folder's site thumbnails and frames, and the copies of videos made for playback, except the one playing.
func (a *app) privacyClearCache() {
	if a.privacy.clear.Busy || a.update == nil {
		return
	}
	var dirs []string
	if a.lib.posterDir != "" {
		dirs = append(dirs, a.lib.posterDir)
	}
	playback := a.player.remuxDir
	if playback == "" {
		if dir, err := os.UserCacheDir(); err == nil {
			playback = filepath.Join(dir, "Kando", "playback")
		}
	}
	dirs = append(dirs, playback)
	var playing string
	if p := a.player.play; p != nil && p.remux != 0 {
		playing = p.source
	}
	a.privacy.clearGen++
	gen := a.privacy.clearGen
	a.privacy.clear = settings.CacheClear{Busy: true}
	update := a.update
	go func() {
		freed, err := cache.Clear(dirs, playing)
		update(func() {
			if gen != a.privacy.clearGen {
				return
			}
			a.privacy.clear = settings.CacheClear{Done: true, Freed: freed}
			if err != nil {
				a.privacy.clear.Err = err.Error()
			}
		})
	}()
}
