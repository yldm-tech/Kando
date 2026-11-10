package main

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"kando/apps/kando/films"
	"kando/apps/kando/locales"
	"kando/packages/core"
	"kando/packages/media"

	"github.com/egoist/mygo"
)

// What a failed download's card offers, by why it failed (core.FailKind), in the prototype's words: the remedies a card can run, and the yt-dlp update 更新并重试 runs.

// ytdlpState is what the queue knows of the yt-dlp it downloads with: its version and how it was installed, and the browsers whose logins it can borrow, read off the UI thread when the window is ready and when the tools change, so a card only reads them; whether 更新并重试 is updating it now; latest, the version an update found to be the newest there is; and kitRetry, set by 下载 yt-dlp 并重试 until the yt-dlp Kando downloads for itself is in, when it is updated with -U, as the release it is pinned to may be too old for the site as well, and the downloads yt-dlp was too old for run again with it.
type ytdlpState struct {
	path     string
	version  string
	install  media.YTDLPInstall
	browsers []media.Browser
	updating bool
	latest   string
	kitRetry bool
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
)

// installedBrowsers looks on disk for the browsers whose logins the cards offer to borrow; it runs off the UI thread. Tests replace it.
var installedBrowsers = media.InstalledBrowsers

// openExternal opens a URL with the system, as the Full Disk Access pane of System Settings. Tests replace it.
var openExternal = mygo.Shell.OpenExternal

// fullDiskAccess is the pane of System Settings that lets an app read Safari's cookies.
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
			if path == "" || a.queue.ytdlp.updating {
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
				a.tell(locales.QueueNoBrowserAgain())
			}
		})
	}()
}

// FailureRemedy is what the card of a job whose download failed says and offers, by why it failed. A failure of another stage, or one yt-dlp did not say enough about, has none, and the card says FailureHint's reason instead.
func (a *app) FailureRemedy(item films.Film) (films.Remedy, bool) {
	job, ok := a.library.Find(item.JobID)
	if !ok || job.Stage != core.StageFailed || job.FailKind == "" {
		return films.Remedy{}, false
	}
	site := films.SourceLabel(item)
	browser, other := a.remedyBrowsers(job)
	// borrow is 借用 … 登录并重试, or 重新查找浏览器 when there is no browser to borrow from; why is then followed by the sentence that says there is none in place of withBrowser.
	borrow := func(primary bool) films.RemedyAction {
		if browser.ID == "" {
			return films.RemedyAction{ID: remedyFindBrowsers, Label: locales.QueueRemedyFindBrowsers(), Primary: primary}
		}
		return films.RemedyAction{ID: remedyBorrow + browser.ID, Label: locales.QueueRemedyBorrow(browser.Name), Primary: primary}
	}
	why := func(need, withBrowser string) string {
		if browser.ID == "" {
			return locales.QueueRemedySentences(need, locales.QueueRemedyNoBrowser())
		}
		return locales.QueueRemedySentences(need, withBrowser)
	}
	swap := func(primary bool) films.RemedyAction {
		return films.RemedyAction{ID: remedySwap, Label: locales.QueueRemedySwap(), Primary: primary}
	}
	retry := films.RemedyAction{ID: remedyRetry, Label: locales.QueueRemedyRetry(), Primary: true}
	local := func(primary bool) films.RemedyAction {
		return films.RemedyAction{ID: remedyLocal, Label: locales.QueueRemedyLocal(), Primary: primary}
	}
	var r films.Remedy
	switch job.FailKind {
	case core.FailOutdated:
		r = a.outdatedRemedy(site)
	case core.FailLoginExpired:
		name := media.BrowserName(job.Cookies)
		r = films.Remedy{Headline: locales.QueueRemedyLoginExpired(site), Why: locales.QueueRemedyLoginExpiredWhy(name, siteShort(job.Source, site)),
			Actions: []films.RemedyAction{{ID: remedyBorrow + job.Cookies, Label: locales.QueueRemedyRelogged(), Primary: true}}}
		if other.ID != "" {
			r.Actions = append(r.Actions, films.RemedyAction{ID: remedyBorrow + other.ID, Label: locales.QueueRemedySwitchBrowser(other.Name)})
		}
	case core.FailMembers:
		headline, need := locales.QueueRemedyMembers(), locales.QueueRemedyMembersNeed()
		if queueBilibili(job.Source) {
			headline, need = locales.QueueRemedyPremium(), locales.QueueRemedyPremiumNeed()
		}
		r = films.Remedy{Headline: headline, Why: why(need, locales.QueueRemedyMembersBorrow()), Actions: []films.RemedyAction{borrow(true), swap(false)}}
	case core.FailAge:
		r = films.Remedy{Headline: locales.QueueRemedyAge(), Why: locales.QueueRemedyAgeWhy(site), Actions: []films.RemedyAction{borrow(true)}}
		if browser.ID == "" {
			r.Why = locales.QueueRemedySentences(locales.QueueRemedyAgeNeed(site), locales.QueueRemedyNoBrowser())
		}
	case core.FailLogin:
		r = films.Remedy{Headline: locales.QueueRemedyLogin(), Why: why(locales.QueueRemedyLoginNeed(hostOf(job.Source)), locales.QueueRemedyLoginBorrow()), Actions: []films.RemedyAction{borrow(true)}}
	case core.FailRegion:
		r = films.Remedy{Headline: locales.QueueRemedyRegion(), Why: locales.QueueRemedyRegionWhy(), Actions: []films.RemedyAction{swap(true)}}
	case core.FailPrivate:
		r = films.Remedy{Headline: locales.QueueRemedyPrivate(), Why: locales.QueueRemedyPrivateWhy(), Actions: []films.RemedyAction{swap(true)}}
	case core.FailDRM:
		r = films.Remedy{Headline: locales.QueueRemedyDrm(site), Why: locales.QueueRemedyDrmWhy(), Actions: []films.RemedyAction{local(true)}}
	case core.FailUnsupported:
		r = films.Remedy{Headline: locales.QueueRemedyUnsupported(hostOf(job.Source)), Why: locales.QueueRemedyUnsupportedWhy(), Actions: []films.RemedyAction{swap(true), local(false), {ID: remedySites, Label: locales.QueueRemedySites()}}}
	case core.FailRateLimit:
		r = films.Remedy{Headline: locales.QueueRemedyRateLimit(site), Why: locales.QueueRemedyRateLimitWhy(), Actions: []films.RemedyAction{retry, borrow(false)}}
		if browser.ID == "" {
			r.Why, r.Actions = locales.QueueRemedyRateLimitWait(), []films.RemedyAction{retry}
		}
	case core.FailNetwork:
		r = films.Remedy{Headline: locales.QueueRemedyNetwork(), Why: locales.QueueRemedyNetworkWhy(), Actions: []films.RemedyAction{retry}}
	case core.FailBadLink:
		r = films.Remedy{Headline: locales.QueueRemedyBadLink(), Why: locales.QueueRemedyBadLinkWhy(), Actions: []films.RemedyAction{swap(true)}}
	case core.FailHost:
		r = films.Remedy{Headline: locales.QueueRemedyHost(hostOf(job.Source)), Why: locales.QueueRemedyHostWhy(), Actions: []films.RemedyAction{swap(true), {ID: remedyRetry, Label: locales.QueueRemedyRetry()}}}
	case core.FailCookies:
		r = a.cookiesRemedy(job, other)
	default:
		return films.Remedy{}, false
	}
	return r, true
}

// outdatedRemedy is the card of a site yt-dlp no longer understands: the version it has, and 更新并重试, or how to update it when Kando cannot, with 下载 yt-dlp 并重试 first when Kando can download its own yt-dlp here.
func (a *app) outdatedRemedy(site string) films.Remedy {
	y := a.queue.ytdlp
	if _, downloading := a.kitsProgress("yt-dlp"); downloading && y.kitRetry {
		return films.Remedy{Headline: locales.QueueRemedyDownloadingYtdlp(), Why: locales.QueueRemedyDownloadingYtdlpWhy(), Working: true,
			Actions: []films.RemedyAction{{ID: remedyDownloadYTDLP, Label: locales.QueueRemedyDownloadingYtdlpAction(), Primary: true, Disabled: true}}}
	}
	if y.updating {
		return films.Remedy{Headline: locales.QueueRemedyUpdating(), Why: locales.QueueRemedyUpdatingWhy(), Working: true,
			Actions: []films.RemedyAction{{ID: remedyUpdate, Label: locales.QueueRemedyUpdatingAction(), Primary: true, Disabled: true}}}
	}
	if y.latest != "" && y.latest == y.version {
		return films.Remedy{Headline: locales.QueueRemedyOutdatedLatest(site), Why: locales.QueueRemedyOutdatedLatestWhy(y.version),
			Actions: []films.RemedyAction{{ID: remedyRetry, Label: locales.QueueRemedyRetry(), Primary: true}, {ID: remedyUpdate, Label: locales.QueueRemedyCheckUpdate()}}}
	}
	r := films.Remedy{Headline: locales.QueueRemedyOutdated(site), Why: locales.QueueRemedyOutdatedWhy()}
	if y.version != "" {
		r.Why = locales.QueueRemedyOutdatedVersionWhy(y.version)
	}
	if y.path != "" && y.install.Update == nil {
		hint := queueYTDLPHint(y.install)
		if !a.kitsCanInstall("yt-dlp") {
			r.Why = locales.QueueRemedyOutdatedManualWhy(y.version, hint)
			r.Actions = []films.RemedyAction{{ID: remedyRetry, Label: locales.QueueRemedyUpdated(), Primary: true}}
			return r
		}
		r.Why = locales.QueueRemedyOutdatedDownloadWhy(y.version, hint)
		if err := a.kits.errs["yt-dlp"]; err != nil {
			r.Why = locales.QueueRemedyDownloadYtdlpFailed(kitsError(err))
		}
		r.Actions = []films.RemedyAction{{ID: remedyDownloadYTDLP, Label: locales.QueueRemedyDownloadYtdlp(), Primary: true}, {ID: remedyRetry, Label: locales.QueueRemedyUpdated()}}
		return r
	}
	r.Actions = []films.RemedyAction{{ID: remedyUpdate, Label: locales.QueueRemedyUpdate(), Primary: true}}
	return r
}

// queueYTDLPHint says how the user updates a yt-dlp Kando cannot update itself, by how it was installed.
func queueYTDLPHint(install media.YTDLPInstall) string {
	switch install.Kind {
	case "homebrew":
		return locales.QueueYtdlpHintBrew()
	case "pipx":
		return locales.QueueYtdlpHintPipx()
	case "system":
		return locales.QueueYtdlpHintSystem(install.Dir)
	}
	return locales.QueueYtdlpHintStandalone(install.Dir)
}

// cookiesRemedy is the card of a job whose borrowed browser login yt-dlp could not read: Safari's needs Full Disk Access, which yt-dlp reports as an operation not permitted, and a browser may have no login data on this machine.
func (a *app) cookiesRemedy(job core.Job, other media.Browser) films.Remedy {
	name := media.BrowserName(job.Cookies)
	r := films.Remedy{Headline: locales.QueueRemedyCookies(name), Why: locales.QueueRemedyCookiesWhy(name)}
	if job.Cookies == "safari" && job.Err != nil && strings.Contains(job.Err.Error(), "Operation not permitted") {
		r.Why = locales.QueueRemedySafariWhy()
		r.Actions = append(r.Actions, films.RemedyAction{ID: remedyDiskAccess, Label: locales.QueueRemedyOpenSystemSettings(), Primary: true}, films.RemedyAction{ID: remedyBorrow + "safari", Label: locales.QueueRemedyAllowed()})
	}
	if other.ID != "" {
		r.Actions = append(r.Actions, films.RemedyAction{ID: remedyBorrow + other.ID, Label: locales.QueueRemedySwitchBrowser(other.Name), Primary: len(r.Actions) == 0})
	}
	return r
}

// remedyBrowsers is the browser a card offers to borrow a login from, the one the job borrows already or else the first installed, and another installed one to switch to, of those queueReadYTDLP found; either is the zero Browser when there is none.
func (a *app) remedyBrowsers(job core.Job) (browser, other media.Browser) {
	installed := a.queue.ytdlp.browsers
	for _, b := range installed {
		if b.ID == job.Cookies {
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
		a.Retry(job.ID)
	case strings.HasPrefix(action, remedyBorrow):
		browser := strings.TrimPrefix(action, remedyBorrow)
		job.Cookies = browser
		a.queueRetry(job)
		a.tell(locales.QueueBorrowed(media.BrowserName(browser)))
	case action == remedySwap:
		a.queueSwapLink(item)
	case action == remedyFindBrowsers:
		a.queueFindBrowsers()
	case action == remedyLocal:
		a.pickVideos()
	case action == remedySites:
		a.OpenSites()
	case action == remedyDiskAccess:
		if err := openExternal(fullDiskAccess); err != nil {
			a.warn(locales.QueueOpenSettingsFailed(err.Error()), "")
		}
	}
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
				a.warn(locales.QueueUpdateFailed(queueYTDLPHint(install)), "")
				return
			}
			before := a.queue.ytdlp.version
			a.queue.ytdlp.path, a.queue.ytdlp.version, a.queue.ytdlp.install = path, version, install
			if err != nil {
				a.warn(locales.QueueUpdateFailed(err.Error()), "")
				return
			}
			// The newest yt-dlp there is fails the same way until the site's fix is out: nothing runs again, and the cards say so.
			if version == before {
				a.queue.ytdlp.latest = version
				a.tell(locales.QueueYtdlpLatest(version))
				return
			}
			a.queue.ytdlp.latest = ""
			a.tell(locales.QueueYtdlpUpdated(version, a.queueRetryOutdated()))
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
	a.tell(locales.QueueSwapRemoved(name))
}

// hostOf is the site a link points at, without www.
func hostOf(source string) string {
	if link, ok := core.ParseLink(source); ok && link.Host != "" {
		return strings.TrimPrefix(link.Host, "www.")
	}
	return source
}

// siteShort is how a sentence names the site of source after 登录: B 站 for 哔哩哔哩, else site.
func siteShort(source, site string) string {
	if queueBilibili(source) {
		return locales.QueueRemedyBilibiliShort()
	}
	return site
}

// queueBilibili reports whether source is a 哔哩哔哩 link, whose cards say 大会员 and B 站.
func queueBilibili(source string) bool {
	link, ok := core.ParseLink(source)
	return ok && link.Platform == core.PlatformBilibili
}
