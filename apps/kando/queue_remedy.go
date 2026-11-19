package main

import (
	"context"
	"errors"
	"path/filepath"
	"slices"
	"strings"
	"time"

	"kando/apps/kando/desktop"
	"kando/apps/kando/films"
	"kando/apps/kando/locales"
	"kando/apps/kando/settings"
	"kando/apps/kando/toolhealth"
	"kando/packages/core"
	"kando/packages/media"
)

// What a failed download's card offers, by why it failed (core.FailKind), in the prototype's words: the remedies a card can run, and the yt-dlp update 更新并重试 runs.

// ytdlpState is what the queue knows of the yt-dlp it downloads with: its version and how it was installed, and the browsers whose logins it can borrow, read off the UI thread when the window is ready and when the tools change, so a card only reads them; whether 更新并重试, or the daily update of Kando's own yt-dlp, is updating it now; latest, the version an update found to be the newest there is; and kitRetry, set by 下载 yt-dlp 并重试 until the yt-dlp Kando downloads for itself is in, when it is updated with -U, as the release it is pinned to may be too old for the site as well, and the downloads yt-dlp was too old for run again with it. freshened is when the daily update last brought Kando's own yt-dlp up to date, which settings.json keeps, and freshenTried when it last tried (queueFreshenYTDLP).
type ytdlpState struct {
	path         string
	version      string
	install      media.YTDLPInstall
	browsers     []media.Browser
	updating     bool
	latest       string
	kitRetry     bool
	freshened    time.Time
	freshenTried time.Time
}

// The remedies' action IDs (films.RemedyAction.ID). A borrowed login is remedyBorrow followed by the browser's --cookies-from-browser name.
const (
	remedyUpdate     = "update"
	remedyRetry      = "retry"
	remedyBorrow     = "borrow:"
	remedySwap       = "swap"
	remedyLocal      = "local"
	remedyDiskAccess = "disk-access"
	// remedyFindBrowsers looks for the browsers again, for a card that found none to borrow a login from.
	remedyFindBrowsers = "find-browsers"
	// remedySites opens the 支持的网站 sheet, for a card of a site yt-dlp does not know.
	remedySites = "sites"
	// remedyDownloadYTDLP downloads Kando's own yt-dlp (media.Kit) in place of one Kando cannot update, and then runs the download again with it.
	remedyDownloadYTDLP = "download-ytdlp"
	// remedyChooseBrowser opens Settings › 通用, whose 浏览器登录 chooses the browser every download borrows the login of, for a card whose browser's login could not be read.
	remedyChooseBrowser = "choose-browser"
	// remedyOwnTool is followed by a program's name: it runs Kando's own copy of the program, downloading it when needed (toolsUseOwn), for a card of a job that failed because the copy it ran does not start, and runs that job again, and every other one it stopped, once the program works. remedyToolSettings opens Settings › 通用, whose row of the program links to its page, where Kando downloads no copy of its own.
	remedyOwnTool      = "own-tool:"
	remedyToolSettings = "tool-settings"
)

// installedBrowsers looks on disk for the browsers whose logins the cards offer to borrow; it runs off the UI thread. Tests replace it.
var installedBrowsers = media.InstalledBrowsers

// openExternal opens a URL with the system under the scope cx, as the Full Disk Access pane of System Settings, and tells done whether it opened. Tests replace it.
var openExternal = desktop.OpenURL

// fullDiskAccess is the pane of System Settings that lets an app read Safari's cookies and the data folders of the other browsers.
const fullDiskAccess = "x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles"

// queueReadYTDLP reads, off the UI thread, the browsers whose logins yt-dlp can borrow, and the version of the yt-dlp the pipeline downloads with and how it was installed.
func (a *app) queueReadYTDLP() {
	if a.update == nil {
		return
	}
	tools, _ := a.pipeline.(media.Tools)
	path, update := tools.YTDLP, a.update
	go func() {
		browsers := installedBrowsers()
		var version string
		var install media.YTDLPInstall
		if path != "" {
			version, _ = media.YTDLPVersion(context.Background(), path)
			install = media.FindYTDLPInstall(path)
		}
		update(func() {
			a.queue.ytdlp.browsers = browsers
			// A read of a yt-dlp the pipeline no longer downloads with is stale: the tools changed while it ran, as when Kando's own yt-dlp came in and was updated, and that newer state stays.
			current, _ := a.pipeline.(media.Tools)
			if path == "" || a.queue.ytdlp.updating || current.YTDLP != path {
				return
			}
			a.queue.ytdlp.path, a.queue.ytdlp.version, a.queue.ytdlp.install = path, version, install
		})
	}()
}

// queueFindBrowsers looks for the browsers again off the UI thread, for 重新查找浏览器; the message bar says when there is still none.
func (a *app) queueFindBrowsers() {
	if a.update == nil {
		return
	}
	update := a.update
	go func() {
		browsers := installedBrowsers()
		update(func() {
			a.queue.ytdlp.browsers = browsers
			if len(browsers) == 0 {
				a.tell(a.messages().QueueNoBrowserAgain())
			}
		})
	}()
}

// FailureRemedy is what the card of a job whose download failed says and offers, by why it failed, and of a job of any stage that failed because the copy of a program it ran does not start (toolRemedy). A failure of another stage, or one yt-dlp did not say enough about, has none, and the card says FailureHint's reason instead.
func (a *app) FailureRemedy(item films.Film) (films.Remedy, bool) {
	job, ok := a.library.Find(item.JobID)
	if !ok {
		return films.Remedy{}, false
	}
	if r, broken := a.toolRemedy(job); broken {
		return r, true
	}
	if job.Stage != core.StageFailed || job.FailKind == "" {
		return films.Remedy{}, false
	}
	m := a.messages()
	site := films.SourceLabel(m, item)
	browser, other := a.remedyBrowsers(job)
	// borrow is 借用 … 登录并重试, or 重新查找浏览器 when there is no browser to borrow from; why is then followed by the sentence that says there is none in place of withBrowser.
	borrow := func(primary bool) films.RemedyAction {
		if browser.ID == "" {
			return films.RemedyAction{ID: remedyFindBrowsers, Label: m.QueueRemedyFindBrowsers(), Primary: primary}
		}
		return films.RemedyAction{ID: remedyBorrow + browser.ID, Label: m.QueueRemedyBorrow(browser.Name), Primary: primary}
	}
	why := func(need, withBrowser string) string {
		if browser.ID == "" {
			return m.QueueRemedySentences(need, m.QueueRemedyNoBrowser())
		}
		return m.QueueRemedySentences(need, withBrowser)
	}
	swap := func(primary bool) films.RemedyAction {
		return films.RemedyAction{ID: remedySwap, Label: m.QueueRemedySwap(), Primary: primary}
	}
	retry := films.RemedyAction{ID: remedyRetry, Label: m.QueueRemedyRetry(), Primary: true}
	local := func(primary bool) films.RemedyAction {
		return films.RemedyAction{ID: remedyLocal, Label: m.QueueRemedyLocal(), Primary: primary}
	}
	var r films.Remedy
	switch job.FailKind {
	case core.FailOutdated:
		r = a.outdatedRemedy(site)
	case core.FailLoginExpired:
		name := media.BrowserName(job.Cookies)
		r = films.Remedy{Headline: m.QueueRemedyLoginExpired(site), Why: m.QueueRemedyLoginExpiredWhy(name, siteShort(m, job.Source, site)),
			Actions: []films.RemedyAction{{ID: remedyBorrow + job.Cookies, Label: m.QueueRemedyRelogged(), Primary: true}}}
		if other.ID != "" {
			r.Actions = append(r.Actions, films.RemedyAction{ID: remedyBorrow + other.ID, Label: m.QueueRemedySwitchBrowser(other.Name)})
		}
	case core.FailMembers:
		headline, need := m.QueueRemedyMembers(), m.QueueRemedyMembersNeed()
		if queueBilibili(job.Source) {
			headline, need = m.QueueRemedyPremium(), m.QueueRemedyPremiumNeed()
		}
		r = films.Remedy{Headline: headline, Why: why(need, m.QueueRemedyMembersBorrow()), Actions: []films.RemedyAction{borrow(true), swap(false)}}
	case core.FailAge:
		r = films.Remedy{Headline: m.QueueRemedyAge(), Why: m.QueueRemedyAgeWhy(site), Actions: []films.RemedyAction{borrow(true)}}
		if browser.ID == "" {
			r.Why = m.QueueRemedySentences(m.QueueRemedyAgeNeed(site), m.QueueRemedyNoBrowser())
		}
	case core.FailLogin:
		r = films.Remedy{Headline: m.QueueRemedyLogin(), Why: why(m.QueueRemedyLoginNeed(hostOf(job.Source)), m.QueueRemedyLoginBorrow()), Actions: []films.RemedyAction{borrow(true)}}
	case core.FailRegion:
		r = films.Remedy{Headline: m.QueueRemedyRegion(), Why: m.QueueRemedyRegionWhy(), Actions: []films.RemedyAction{swap(true)}}
	case core.FailPrivate:
		r = films.Remedy{Headline: m.QueueRemedyPrivate(), Why: m.QueueRemedyPrivateWhy(), Actions: []films.RemedyAction{swap(true)}}
	case core.FailDRM:
		r = films.Remedy{Headline: m.QueueRemedyDrm(site), Why: m.QueueRemedyDrmWhy(), Actions: []films.RemedyAction{local(true)}}
	case core.FailUnsupported:
		r = films.Remedy{Headline: m.QueueRemedyUnsupported(hostOf(job.Source)), Why: m.QueueRemedyUnsupportedWhy(), Actions: []films.RemedyAction{swap(true), local(false), {ID: remedySites, Label: m.QueueRemedySites()}}}
	case core.FailRateLimit:
		r = films.Remedy{Headline: m.QueueRemedyRateLimit(site), Why: m.QueueRemedyRateLimitWhy(), Actions: []films.RemedyAction{retry, borrow(false)}}
		if browser.ID == "" {
			r.Why, r.Actions = m.QueueRemedyRateLimitWait(), []films.RemedyAction{retry}
		}
	case core.FailNetwork:
		r = films.Remedy{Headline: m.QueueRemedyNetwork(), Why: m.QueueRemedyNetworkWhy(), Actions: []films.RemedyAction{retry}}
	case core.FailBadLink:
		r = films.Remedy{Headline: m.QueueRemedyBadLink(), Why: m.QueueRemedyBadLinkWhy(), Actions: []films.RemedyAction{swap(true)}}
	case core.FailHost:
		r = films.Remedy{Headline: m.QueueRemedyHost(hostOf(job.Source)), Why: m.QueueRemedyHostWhy(), Actions: []films.RemedyAction{swap(true), {ID: remedyRetry, Label: m.QueueRemedyRetry()}}}
	case core.FailCookies:
		r = a.cookiesRemedy(job, other)
	case core.FailHiddenVideo:
		r = a.hiddenVideoRemedy(job, other)
	default:
		return films.Remedy{}, false
	}
	// A download from a site pack's site never borrows a sign-in (queuePipeline), so its card offers none.
	if job.PluginSite != "" {
		r = pluginsSiteRemedy(m, r)
	}
	return r, true
}

// toolRemedy is the card of a job that failed at a stage a program runs whose copy does not start now (toolBrokenFor): why, and 下载 … 并重试, or 重新下载 … 并重试 when Kando's own copy is the one that does not start, which runs Kando's own copy and the job again once it works; where Kando downloads no copy of its own, 打开设置, whose row links to the program's page, and 重试. While Kando's own copy downloads, the card says so. A job whose programs all start now has none, as the program was fixed since.
func (a *app) toolRemedy(job core.Job) (films.Remedy, bool) {
	program, broken := a.toolBrokenFor(job)
	if !broken {
		return films.Remedy{}, false
	}
	m := a.messages()
	path, health := a.toolHealth(program)
	reason := toolhealth.Why(m, health)
	if _, downloading := a.kitsProgress(program); downloading {
		return films.Remedy{Headline: m.QueueRemedyToolDownloading(program), Why: m.QueueRemedyToolDownloadingWhy(), Working: true,
			Actions: []films.RemedyAction{{ID: remedyOwnTool + program, Label: m.QueueRemedyToolDownloadingAction(), Primary: true, Disabled: true}}}, true
	}
	r := films.Remedy{Headline: m.QueueRemedyToolBroken(program)}
	switch {
	case !a.kitsCanInstall(program):
		r.Why = m.QueueRemedyToolManualWhy(reason, program)
		r.Actions = []films.RemedyAction{{ID: remedyToolSettings, Label: m.QueueRemedyToolSettings(), Primary: true}, {ID: remedyRetry, Label: m.QueueRemedyRetry()}}
	case path == media.OwnTool(program):
		r.Why = m.QueueRemedyToolRedownloadWhy(reason, program)
		r.Actions = []films.RemedyAction{{ID: remedyOwnTool + program, Label: m.QueueRemedyToolRedownload(program), Primary: true}}
	default:
		r.Why = m.QueueRemedyToolOwnWhy(reason, program)
		r.Actions = []films.RemedyAction{{ID: remedyOwnTool + program, Label: m.QueueRemedyToolDownload(program), Primary: true}}
	}
	return r, true
}

// queueOwnToolAndRetry is 下载 … 并重试 on the card of a job a program's copy stopped: every failed job of the open library it stopped runs again once Kando's own copy works (toolsRetryWhenFixed), which it downloads when it has none or its copy does not start (toolsUseOwn).
func (a *app) queueOwnToolAndRetry(program string) {
	for _, job := range a.library.Jobs {
		if broken, ok := a.toolBrokenFor(job); ok && broken == program && films.InLibrary(a.snap().LibraryDir, job) && !slices.Contains(a.tools.retry[program], job.ID) {
			a.tools.retry[program] = append(a.tools.retry[program], job.ID)
		}
	}
	a.toolsUseOwn(program)
}

// outdatedRemedy is the card of a site yt-dlp no longer understands: the version it has, and 更新并重试, or how to update it when Kando cannot, with 下载 yt-dlp 并重试 first when Kando can download its own yt-dlp here.
func (a *app) outdatedRemedy(site string) films.Remedy {
	m := a.messages()
	y := a.queue.ytdlp
	if _, downloading := a.kitsProgress("yt-dlp"); downloading && y.kitRetry {
		return films.Remedy{Headline: m.QueueRemedyDownloadingYtdlp(), Why: m.QueueRemedyDownloadingYtdlpWhy(), Working: true,
			Actions: []films.RemedyAction{{ID: remedyDownloadYTDLP, Label: m.QueueRemedyDownloadingYtdlpAction(), Primary: true, Disabled: true}}}
	}
	if y.updating {
		return films.Remedy{Headline: m.QueueRemedyUpdating(), Why: m.QueueRemedyUpdatingWhy(), Working: true,
			Actions: []films.RemedyAction{{ID: remedyUpdate, Label: m.QueueRemedyUpdatingAction(), Primary: true, Disabled: true}}}
	}
	if y.latest != "" && y.latest == y.version {
		return films.Remedy{Headline: m.QueueRemedyOutdatedLatest(site), Why: m.QueueRemedyOutdatedLatestWhy(y.version),
			Actions: []films.RemedyAction{{ID: remedyRetry, Label: m.QueueRemedyRetry(), Primary: true}, {ID: remedyUpdate, Label: m.QueueRemedyCheckUpdate()}}}
	}
	r := films.Remedy{Headline: m.QueueRemedyOutdated(site), Why: m.QueueRemedyOutdatedWhy()}
	if y.version != "" {
		r.Why = m.QueueRemedyOutdatedVersionWhy(y.version)
	}
	if y.path != "" && y.install.Update == nil {
		hint := queueYTDLPHint(m, y.install)
		if !a.kitsCanInstall("yt-dlp") {
			r.Why = m.QueueRemedyOutdatedManualWhy(y.version, hint)
			r.Actions = []films.RemedyAction{{ID: remedyRetry, Label: m.QueueRemedyUpdated(), Primary: true}}
			return r
		}
		r.Why = m.QueueRemedyOutdatedDownloadWhy(y.version, hint)
		if err := a.kits.errs["yt-dlp"]; err != nil {
			r.Why = m.QueueRemedyDownloadYtdlpFailed(settings.KitsError(m, err))
		}
		r.Actions = []films.RemedyAction{{ID: remedyDownloadYTDLP, Label: m.QueueRemedyDownloadYtdlp(), Primary: true}, {ID: remedyRetry, Label: m.QueueRemedyUpdated()}}
		return r
	}
	r.Actions = []films.RemedyAction{{ID: remedyUpdate, Label: m.QueueRemedyUpdate(), Primary: true}}
	return r
}

// queueYTDLPHint says how the user updates a yt-dlp Kando cannot update itself, by how it was installed, in the language of m.
func queueYTDLPHint(m locales.Messages, install media.YTDLPInstall) string {
	switch install.Kind {
	case "homebrew":
		return m.QueueYtdlpHintBrew()
	case "pipx":
		return m.QueueYtdlpHintPipx()
	case "system":
		return m.QueueYtdlpHintSystem(install.Dir)
	}
	return m.QueueYtdlpHintStandalone(install.Dir)
}

// cookiesRemedy is the card of a job whose borrowed browser login yt-dlp could not read, saying why as Settings › 通用 › 浏览器登录 does (settings.SignInWhy): on macOS Safari's, and the data folder of another browser, need Full Disk Access (settings.NeedsDiskAccess), which yt-dlp reports as an operation not permitted or, for the folder, as no login data; on Windows an open Chromium browser locks its login, which closing it fixes, and Chrome's own encryption keeps it from yt-dlp; on macOS the Keychain may not have given its key; and a browser may have no login data on this machine. 选择浏览器… opens that setting, where the browser every download borrows from is chosen, while the computer has one to choose.
func (a *app) cookiesRemedy(job core.Job, other media.Browser) films.Remedy {
	m := a.messages()
	name := media.BrowserName(job.Cookies)
	var trouble media.LoginTrouble
	if job.Err != nil {
		trouble = media.ReadLoginTrouble(job.Err.Error())
	}
	r := films.Remedy{Headline: m.QueueRemedyCookies(name), Why: settings.SignInWhy(m, signinPlatform(), job.Cookies, trouble)}
	switch {
	// macOS hides Safari's cookies, and the data folder of the other browsers, from an app without Full Disk Access, and yt-dlp then says it found none, or that it may not open them.
	case settings.NeedsDiskAccess(signinPlatform(), job.Cookies, trouble):
		r.Actions = append(r.Actions, films.RemedyAction{ID: remedyDiskAccess, Label: m.QueueRemedyOpenSystemSettings(), Primary: true}, films.RemedyAction{ID: remedyBorrow + job.Cookies, Label: m.QueueRemedyAllowed()})
	case trouble == media.LoginInUse || trouble == media.LoginNoKey:
		r.Actions = append(r.Actions, films.RemedyAction{ID: remedyBorrow + job.Cookies, Label: m.QueueRemedyRetry(), Primary: true})
	}
	if other.ID != "" {
		r.Actions = append(r.Actions, films.RemedyAction{ID: remedyBorrow + other.ID, Label: m.QueueRemedySwitchBrowser(other.Name), Primary: len(r.Actions) == 0})
	}
	// The setting offers the browsers the computer has; with none, there is nothing to choose.
	if len(a.queue.ytdlp.browsers) > 0 {
		r.Actions = append(r.Actions, films.RemedyAction{ID: remedyChooseBrowser, Label: m.QueueRemedyChooseBrowser()})
	}
	return r
}

// hiddenVideoRemedy is the card of an X post that showed yt-dlp no video (core.FailHiddenVideo). Signed out, X leaves out the video of a sensitive post or a protected account's, so the card says the sign-in shows it and offers 选择浏览器…, which opens Settings › 通用 › 浏览器登录, and 重试, which then borrows the browser chosen there (queueRun); with no browser on the computer to choose, 重新查找浏览器 instead. Signed in through a browser already, the account is one X does not show the video to: the card names X's setting for sensitive media and offers 重试 with that browser, or another one, rather than the sign-in again.
func (a *app) hiddenVideoRemedy(job core.Job, other media.Browser) films.Remedy {
	m := a.messages()
	retry := films.RemedyAction{ID: remedyRetry, Label: m.QueueRemedyRetry()}
	if job.Browser() != "" {
		retry.Primary = true
		r := films.Remedy{Headline: m.QueueRemedyXSignedIn(), Why: m.QueueRemedyXSignedInWhy(media.BrowserName(job.Browser())), Actions: []films.RemedyAction{retry}}
		if other.ID != "" {
			r.Actions = append(r.Actions, films.RemedyAction{ID: remedyBorrow + other.ID, Label: m.QueueRemedySwitchBrowser(other.Name)})
		}
		return r
	}
	r := films.Remedy{Headline: m.QueueRemedyXSignedOut(), Why: m.QueueRemedySentences(m.QueueRemedyXSignedOutNeed(), m.QueueRemedyXSignedOutChoose()),
		Actions: []films.RemedyAction{{ID: remedyChooseBrowser, Label: m.QueueRemedyChooseBrowser(), Primary: true}, retry}}
	// The setting offers the browsers the computer has; with none, there is nothing to choose.
	if len(a.queue.ytdlp.browsers) == 0 {
		r.Why = m.QueueRemedySentences(m.QueueRemedyXSignedOutNeed(), m.QueueRemedyNoBrowser())
		r.Actions[0] = films.RemedyAction{ID: remedyFindBrowsers, Label: m.QueueRemedyFindBrowsers(), Primary: true}
	}
	return r
}

// remedyBrowsers is the browser a card offers to borrow a login from, the one the job borrows already or else the first installed, and another installed one to switch to, of those queueReadYTDLP found; either is the zero Browser when there is none.
func (a *app) remedyBrowsers(job core.Job) (browser, other media.Browser) {
	installed := a.queue.ytdlp.browsers
	for _, b := range installed {
		if b.ID == job.Browser() {
			browser = b
		}
	}
	if browser.ID == "" && len(installed) > 0 {
		browser = installed[0]
	}
	for _, b := range installed {
		if b.ID != browser.ID && b.ID != job.Cookies {
			other = b
			break
		}
	}
	return browser, other
}

// RunRemedy does what the remedy button the user clicked on a failed card says.
func (a *app) RunRemedy(item films.Film, action string) {
	job, ok := a.library.Find(item.JobID)
	if !ok {
		return
	}
	switch {
	case action == remedyUpdate:
		a.queueUpdateYTDLP()
	case action == remedyDownloadYTDLP:
		a.queue.ytdlp.kitRetry = true
		a.kitsInstall("yt-dlp")
	case action == remedyRetry:
		// An X post that showed no video to a download the Add sheet sent out signed out (core.CookiesNone) is tried again with the browser Settings › 通用 › 浏览器登录 names, which its card asked to choose.
		if job.FailKind == core.FailHiddenVideo && job.Cookies == core.CookiesNone {
			job.Cookies = ""
			a.library.Upsert(job)
		}
		a.Retry(job.ID)
	case strings.HasPrefix(action, remedyBorrow):
		browser := strings.TrimPrefix(action, remedyBorrow)
		job.Cookies = browser
		a.queueRetry(job)
		a.tell(a.messages().QueueBorrowed(media.BrowserName(browser)))
	case action == remedySwap:
		a.queueSwapLink(item)
	case action == remedyFindBrowsers:
		a.queueFindBrowsers()
	case action == remedyLocal:
		a.pickVideos()
	case action == remedySites:
		a.OpenSites()
	case action == remedyDiskAccess:
		a.queueOpenDiskAccess()
	case action == remedyChooseBrowser, action == remedyToolSettings:
		a.OpenSettings(settings.SectionGeneral)
	case strings.HasPrefix(action, remedyOwnTool):
		a.queueOwnToolAndRetry(strings.TrimPrefix(action, remedyOwnTool))
	}
}

// queueOpenDiskAccess opens the Full Disk Access pane of System Settings, and says in the message bar when it could not. The answer comes later, on the engine's goroutine; without the app's scope, as in a test that starts no app, nothing opens.
func (a *app) queueOpenDiskAccess() {
	if a.cx == nil {
		return
	}
	// The answer belongs to the app, not to the card that asked, which may be gone by then (core.Scope.Run).
	a.cx.Run(func() {
		openExternal(a.cx, fullDiskAccess, func(opened bool) {
			// desktop.OpenURL says only whether an app opened it, so the bar says why in a sentence of its own.
			if !opened {
				a.warn(a.messages().QueueOpenSettingsNoApp(), "")
			}
			a.Changed()
		})
	})
}

// queueRetry queues a failed job again as it is now, with why it failed forgotten; the message bar says why when it cannot start.
func (a *app) queueRetry(job core.Job) {
	job.FailKind = ""
	a.library.Upsert(job)
	a.Retry(job.ID)
}

// queueUpdateYTDLP updates yt-dlp the way it was installed, off the UI thread, and then queues again every download of the open library that failed because yt-dlp was too old. The message bar says what came of it.
func (a *app) queueUpdateYTDLP() {
	tools, ok := a.pipeline.(media.Tools)
	if !ok || tools.YTDLP == "" || a.queue.ytdlp.updating || a.update == nil {
		return
	}
	if a.privacy.offline {
		a.warn(a.messages().PrivacyNoYtdlpUpdate(), settings.SectionPrivacy)
		return
	}
	a.queue.ytdlp.updating = true
	path, update := tools.YTDLP, a.update
	go func() {
		version, err := media.UpdateYTDLP(context.Background(), path)
		// How it was installed is read again whatever came of it: a yt-dlp Kando cannot update is named by it, from which the message bar says how to update it, and after 下载 yt-dlp 并重试 the pipeline downloads with Kando's own yt-dlp, which queueReadYTDLP did not record while this ran. A failed update leaves the version it has.
		install := media.FindYTDLPInstall(path)
		if err != nil && !errors.Is(err, media.ErrCannotUpdate) {
			version, _ = media.YTDLPVersion(context.Background(), path)
		}
		update(func() {
			a.queue.ytdlp.updating = false
			if errors.Is(err, media.ErrCannotUpdate) {
				a.warn(a.messages().QueueUpdateFailed(queueYTDLPHint(a.messages(), install)), "")
				return
			}
			before := a.queue.ytdlp.version
			a.queue.ytdlp.path, a.queue.ytdlp.version, a.queue.ytdlp.install = path, version, install
			if err != nil {
				a.warn(a.messages().QueueUpdateFailed(err.Error()), "")
				return
			}
			// Kando's own yt-dlp is up to date for the day (queueFreshenYTDLP).
			if path == media.OwnTool("yt-dlp") {
				a.queue.ytdlp.freshened = time.Now()
			}
			// The newest yt-dlp there is fails the same way until the site's fix is out: nothing runs again, and the cards say so.
			if version == before {
				a.queue.ytdlp.latest = version
				a.tell(a.messages().QueueYtdlpLatest(version))
				return
			}
			a.queue.ytdlp.latest = ""
			a.tell(a.messages().QueueYtdlpUpdated(version, a.queueRetryOutdated()))
		})
	}()
}

// queueFreshenYTDLP updates Kando's own yt-dlp with -U in the background, quietly, at most once a day: the sites change what yt-dlp must understand every few weeks, and Kando's own is the release this Kando pins until updated, which only the card of a download it was too old for did before, and some of what it says when too old reads as another reason (classify.go); a yt-dlp the computer installed is its package manager's to update. It runs when the window is ready, when the app comes to the front and when the tools changed, as when Kando's own yt-dlp has just landed; not in offline mode, not while an update runs, and not within an hour of one that failed.
func (a *app) queueFreshenYTDLP() {
	tools, ok := a.pipeline.(media.Tools)
	own := media.OwnTool("yt-dlp")
	if !ok || own == "" || tools.YTDLP != own || a.queue.ytdlp.updating || a.privacy.offline || a.update == nil {
		return
	}
	now := time.Now()
	if now.Sub(a.queue.ytdlp.freshened) < 24*time.Hour || now.Sub(a.queue.ytdlp.freshenTried) < time.Hour {
		return
	}
	a.queue.ytdlp.freshenTried, a.queue.ytdlp.updating = now, true
	update := a.update
	go func() {
		version, err := media.UpdateYTDLP(context.Background(), own)
		update(func() {
			a.queue.ytdlp.updating = false
			if err != nil {
				return
			}
			a.queue.ytdlp.freshened = time.Now()
			a.Save()
			if a.queue.ytdlp.path == own && version != a.queue.ytdlp.version {
				a.queue.ytdlp.version, a.queue.ytdlp.latest = version, ""
			}
		})
	}()
}

// queueRetryOutdated queues again, oldest first, every download of the open library that failed because yt-dlp was too old, and returns how many.
func (a *app) queueRetryOutdated() int {
	var outdated []core.Job
	for _, job := range a.library.Jobs {
		if job.Stage == core.StageFailed && job.FailKind == core.FailOutdated && films.InLibrary(a.snap().LibraryDir, job) {
			outdated = append(outdated, job)
		}
	}
	for i := len(outdated) - 1; i >= 0; i-- {
		a.queueRetry(outdated[i])
	}
	return len(outdated)
}

// queueRetryWithKit runs, when the tools were looked up again, 更新并重试 on the yt-dlp 下载 yt-dlp 并重试 asked Kando to download for itself, once it is in and the pipeline downloads with it. The kit is the release this Kando was built with, which may be older than the site needs and than the yt-dlp it replaces, so it is updated with -U first (it is a standalone build Kando may write), and the downloads yt-dlp was too old for run again only when that gave a version other than the one they failed with. A download that failed leaves kitRetry set, so the card offers it again and the retry waits for the next one that lands.
func (a *app) queueRetryWithKit() {
	if !a.queue.ytdlp.kitRetry {
		return
	}
	if _, downloading := a.kitsProgress("yt-dlp"); downloading {
		return
	}
	kit, ok := media.KitFor("yt-dlp")
	tools, isTools := a.pipeline.(media.Tools)
	if !ok || !isTools || media.OwnToolsDir == "" || tools.YTDLP != filepath.Join(media.OwnToolsDir, kit.ID, kit.Program) {
		return
	}
	a.queue.ytdlp.kitRetry = false
	a.queueUpdateYTDLP()
}

// queueSwapLink is 换个链接, as the prototype's swapLink: the card goes, the link field above is emptied and takes the focus, and the message bar says to paste the link from elsewhere there. The link pasted starts a job of its own, so nothing of the old one, such as the login it borrowed for its site, carries over.
func (a *app) queueSwapLink(item films.Film) {
	name := item.Name
	if name == "" {
		name = item.Title
	}
	a.remove(item.JobID)
	a.queue.source, a.queue.linkFocus = "", true
	a.tell(a.messages().QueueSwapRemoved(name))
}

// hostOf is the site a link points at, without www.
func hostOf(source string) string {
	if link, ok := core.ParseLink(source); ok && link.Host != "" {
		return strings.TrimPrefix(link.Host, "www.")
	}
	return source
}

// siteShort is how a sentence names the site of source after 登录, in the language of m: B 站 for 哔哩哔哩, else site.
func siteShort(m locales.Messages, source, site string) string {
	if queueBilibili(source) {
		return m.QueueRemedyBilibiliShort()
	}
	return site
}

// queueBilibili reports whether source is a 哔哩哔哩 link, whose cards say 大会员 and B 站.
func queueBilibili(source string) bool {
	link, ok := core.ParseLink(source)
	return ok && link.Platform == core.PlatformBilibili
}
