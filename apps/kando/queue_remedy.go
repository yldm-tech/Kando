package main

import (
	"context"
	"fmt"
	"strings"

	"kando/apps/kando/films"
	"kando/packages/core"
	"kando/packages/media"

	"github.com/egoist/mygo"
)

// What a failed download's card offers, by why it failed (core.FailKind), in the prototype's words: the remedies a card can run, and the yt-dlp update 更新并重试 runs.

// ytdlpState is what the queue knows of the yt-dlp it downloads with: its version and how it was installed, and the browsers whose logins it can borrow, read off the UI thread when the window is ready and when the tools change, so a card only reads them; whether 更新并重试 is updating it now; and latest, the version an update found to be the newest there is.
type ytdlpState struct {
	path     string
	version  string
	install  media.YTDLPInstall
	browsers []media.Browser
	updating bool
	latest   string
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
)

// installedBrowsers looks on disk for the browsers whose logins the cards offer to borrow; it runs off the UI thread. Tests replace it.
var installedBrowsers = media.InstalledBrowsers

// noBrowser is what a card that wants a login says when this machine has no browser yt-dlp can borrow one from, as Windows or Linux without Chrome, Firefox, Edge, Brave or Chromium.
const noBrowser = "这台电脑上没找到能借用登录的浏览器。在 Chrome、Firefox、Edge、Brave 或 Chromium 里登录后，重新查找。"

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
				a.tell("还是没找到能借用登录的浏览器。")
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
	// borrow is 借用 … 登录并重试, or 重新查找浏览器 when there is no browser to borrow from; why is then followed by noBrowser in place of withBrowser.
	borrow := func(primary bool) films.RemedyAction {
		if browser.ID == "" {
			return films.RemedyAction{ID: remedyFindBrowsers, Label: "重新查找浏览器", Primary: primary}
		}
		return films.RemedyAction{ID: remedyBorrow + browser.ID, Label: "借用 " + browser.Name + " 登录并重试", Primary: primary}
	}
	why := func(need, withBrowser string) string {
		if browser.ID == "" {
			return need + noBrowser
		}
		return need + withBrowser
	}
	swap := func(primary bool) films.RemedyAction {
		return films.RemedyAction{ID: remedySwap, Label: "换个链接", Primary: primary}
	}
	retry := films.RemedyAction{ID: remedyRetry, Label: "重试", Primary: true}
	local := func(primary bool) films.RemedyAction {
		return films.RemedyAction{ID: remedyLocal, Label: "选择本地文件", Primary: primary}
	}
	var r films.Remedy
	switch job.FailKind {
	case core.FailOutdated:
		r = a.outdatedRemedy(site)
	case core.FailLoginExpired:
		name := media.BrowserName(job.Cookies)
		r = films.Remedy{Headline: site + "登录已过期", Why: "先在 " + name + " 里重新登录" + siteShort(site) + "，再回来重试。",
			Actions: []films.RemedyAction{{ID: remedyBorrow + job.Cookies, Label: "已重新登录，重试", Primary: true}}}
		if other.ID != "" {
			r.Actions = append(r.Actions, films.RemedyAction{ID: remedyBorrow + other.ID, Label: "改用 " + other.Name})
		}
	case core.FailMembers:
		headline, need := "频道会员专享视频", "需要频道会员。"
		if site == "哔哩哔哩" {
			headline, need = "大会员专享视频", "需要大会员。"
		}
		r = films.Remedy{Headline: headline, Why: why(need, "借用已开通会员的浏览器登录即可。"), Actions: []films.RemedyAction{borrow(true), swap(false)}}
	case core.FailAge:
		r = films.Remedy{Headline: "需要年龄验证", Why: "借用浏览器里已登录的 " + site + " 账号即可。", Actions: []films.RemedyAction{borrow(true)}}
		if browser.ID == "" {
			r.Why = "要用已登录的 " + site + " 账号。" + noBrowser
		}
	case core.FailLogin:
		r = films.Remedy{Headline: "要登录才能看", Why: why(hostOf(job.Source)+" 的这个视频需要账号。", "借用浏览器里已登录的账号即可，不保存密码。"), Actions: []films.RemedyAction{borrow(true)}}
	case core.FailRegion:
		r = films.Remedy{Headline: "你所在的地区不能看", Why: "上传者限制了地区。别处有这个视频的话，换个链接。", Actions: []films.RemedyAction{swap(true)}}
	case core.FailPrivate:
		r = films.Remedy{Headline: "视频已设为私密或被删除", Why: "没法下载。别处有的话，换个链接。", Actions: []films.RemedyAction{swap(true)}}
	case core.FailDRM:
		r = films.Remedy{Headline: site + " 受 DRM 保护", Why: "视频是加密的，没法下载。有本地文件的话可以直接拖进来。", Actions: []films.RemedyAction{local(true)}}
	case core.FailUnsupported:
		r = films.Remedy{Headline: hostOf(job.Source) + " 暂不支持", Why: "没在页面里找到视频。换个链接，或先下载到本机再拖进来。", Actions: []films.RemedyAction{swap(true), local(false), {ID: remedySites, Label: "查看支持的网站"}}}
	case core.FailRateLimit:
		r = films.Remedy{Headline: site + " 暂时限制了下载", Why: "请求太频繁。等几分钟再重试，或借用浏览器里已登录的账号。", Actions: []films.RemedyAction{retry, borrow(false)}}
		if browser.ID == "" {
			r.Why, r.Actions = "请求太频繁。等几分钟再重试。", []films.RemedyAction{retry}
		}
	case core.FailNetwork:
		r = films.Remedy{Headline: "连不上网络", Why: "检查网络连接后再重试。", Actions: []films.RemedyAction{retry}}
	case core.FailBadLink:
		r = films.Remedy{Headline: "链接不完整", Why: "链接好像少了一截，重试也下载不了。复制完整的链接，换个链接再试。", Actions: []films.RemedyAction{swap(true)}}
	case core.FailHost:
		r = films.Remedy{Headline: "找不到 " + hostOf(job.Source), Why: "网络正常的话，多半是链接里的网址写错了，或者这个网站已经关闭。换个链接，或检查网络后再重试。", Actions: []films.RemedyAction{swap(true), {ID: remedyRetry, Label: "重试"}}}
	case core.FailCookies:
		r = a.cookiesRemedy(job, other)
	default:
		return films.Remedy{}, false
	}
	return r, true
}

// outdatedRemedy is the card of a site yt-dlp no longer understands: the version it has, and 更新并重试, or how to update it when Kando cannot.
func (a *app) outdatedRemedy(site string) films.Remedy {
	y := a.queue.ytdlp
	if y.updating {
		return films.Remedy{Headline: "正在更新下载组件…", Why: "更新完会自动重新下载。", Working: true,
			Actions: []films.RemedyAction{{ID: remedyUpdate, Label: "更新中…", Primary: true, Disabled: true}}}
	}
	if y.latest != "" && y.latest == y.version {
		return films.Remedy{Headline: site + " 改版了，下载组件还没跟上", Why: "本机的 yt-dlp 已是最新的 " + y.version + " 版，网站的修复还没发布，过一两天再重试。",
			Actions: []films.RemedyAction{{ID: remedyRetry, Label: "重试", Primary: true}, {ID: remedyUpdate, Label: "检查更新"}}}
	}
	r := films.Remedy{Headline: site + " 改版了，下载组件要更新", Why: "本机的 yt-dlp 太旧了，更新几秒就好。"}
	if y.version != "" {
		r.Why = "本机的 yt-dlp 是 " + y.version + " 版，更新几秒就好。"
	}
	if y.path != "" && y.install.Update == nil {
		r.Why = "本机的 yt-dlp 是 " + y.version + " 版，看懂没法替它更新：" + y.install.Hint + "。更新后再重试。"
		r.Actions = []films.RemedyAction{{ID: remedyRetry, Label: "已更新，重试", Primary: true}}
		return r
	}
	r.Actions = []films.RemedyAction{{ID: remedyUpdate, Label: "更新并重试", Primary: true}}
	return r
}

// cookiesRemedy is the card of a job whose borrowed browser login yt-dlp could not read: Safari's needs Full Disk Access, which yt-dlp reports as an operation not permitted, and a browser may have no login data on this machine.
func (a *app) cookiesRemedy(job core.Job, other media.Browser) films.Remedy {
	name := media.BrowserName(job.Cookies)
	r := films.Remedy{Headline: "读不到 " + name + " 的登录", Why: "这台电脑上找不到 " + name + " 的登录数据。换一个已登录的浏览器再试。"}
	if job.Cookies == "safari" && job.Err != nil && strings.Contains(job.Err.Error(), "Operation not permitted") {
		r.Why = "macOS 只让有「完全磁盘访问权限」的 App 读 Safari 的登录。在系统设置里允许看懂，回来再重试，或改用别的浏览器。"
		r.Actions = append(r.Actions, films.RemedyAction{ID: remedyDiskAccess, Label: "打开系统设置", Primary: true}, films.RemedyAction{ID: remedyBorrow + "safari", Label: "已允许，重试"})
	}
	if other.ID != "" {
		r.Actions = append(r.Actions, films.RemedyAction{ID: remedyBorrow + other.ID, Label: "改用 " + other.Name, Primary: len(r.Actions) == 0})
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
	case action == remedyRetry:
		a.Retry(job.ID)
	case strings.HasPrefix(action, remedyBorrow):
		browser := strings.TrimPrefix(action, remedyBorrow)
		job.Cookies = browser
		a.queueRetry(job)
		a.tell("已借用 " + media.BrowserName(browser) + " 的登录，开始下载。")
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
			a.warn("没能打开系统设置："+err.Error(), "")
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
		update(func() {
			a.queue.ytdlp.updating = false
			if err != nil {
				a.warn("没能更新下载组件 yt-dlp："+err.Error(), "")
				return
			}
			before := a.queue.ytdlp.version
			a.queue.ytdlp.path, a.queue.ytdlp.version = path, version
			// The newest yt-dlp there is fails the same way until the site's fix is out: nothing runs again, and the cards say so.
			if version == before {
				a.queue.ytdlp.latest = version
				a.tell("下载组件 yt-dlp 已是最新的 " + version + " 版，网站的修复还没发布，过一两天再试。")
				return
			}
			a.queue.ytdlp.latest = ""
			var outdated []core.Job
			for _, job := range a.library.Jobs {
				if job.Stage == core.StageFailed && job.FailKind == core.FailOutdated && films.InLibrary(a.snap().LibraryDir, job) {
					outdated = append(outdated, job)
				}
			}
			for i := len(outdated) - 1; i >= 0; i-- {
				a.queueRetry(outdated[i])
			}
			a.tell(fmt.Sprintf("下载组件已更新到 %s，已重新开始下载 %d 个视频。", version, len(outdated)))
		})
	}()
}

// queueSwapLink is 换个链接, as the prototype's swapLink: the card goes, the link field above is emptied and takes the focus, and the message bar says to paste the link from elsewhere there. The link pasted starts a job of its own, so nothing of the old one, such as the login it borrowed for its site, carries over.
func (a *app) queueSwapLink(item films.Film) {
	name := item.Name
	if name == "" {
		name = item.Title
	}
	a.remove(item.JobID)
	a.queue.source, a.queue.linkFocus = "", true
	a.tell("已移除「" + name + "」。如果别处有这个视频，把链接粘贴到右上角。")
}

// hostOf is the site a link points at, without www.
func hostOf(source string) string {
	if link, ok := core.ParseLink(source); ok && link.Host != "" {
		return strings.TrimPrefix(link.Host, "www.")
	}
	return source
}

// siteShort is how a sentence names a site after 登录: B 站 for 哔哩哔哩.
func siteShort(site string) string {
	if site == "哔哩哔哩" {
		return " B 站"
	}
	return " " + site
}
