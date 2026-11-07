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

// What a failed download's card offers, by why it failed (core.FailKind), in the prototype's words: the remedies a card can run, the yt-dlp update 更新并重试 runs, and 换个链接's sheet.

// ytdlpState is what the queue knows of the yt-dlp it downloads with: its version and how it was installed, read off the UI thread when the window is ready and when the tools change, and whether 更新并重试 is updating it now.
type ytdlpState struct {
	path     string
	version  string
	install  media.YTDLPInstall
	updating bool
}

// swapState is 换个链接's sheet: the card whose link it replaces, the link typed so far and why the last one was refused, while open. release gives back the window's modal flag the sheet holds.
type swapState struct {
	open    bool
	id      string
	link    string
	refused string
	release func()
}

// The remedies' action IDs (films.RemedyAction.ID). A borrowed login is remedyBorrow followed by the browser's --cookies-from-browser name.
const (
	remedyUpdate     = "update"
	remedyRetry      = "retry"
	remedyBorrow     = "borrow:"
	remedySwap       = "swap"
	remedyLocal      = "local"
	remedyDiskAccess = "disk-access"
)

// installedBrowsers is the browsers whose logins the cards offer to borrow. Tests replace it.
var installedBrowsers = media.InstalledBrowsers

// openExternal opens a URL with the system, as the Full Disk Access pane of System Settings. Tests replace it.
var openExternal = mygo.Shell.OpenExternal

// fullDiskAccess is the pane of System Settings that lets an app read Safari's cookies.
const fullDiskAccess = "x-apple.systempreferences:com.apple.preference.security?Privacy_AllFiles"

// queueReadYTDLP reads the version of the yt-dlp the pipeline downloads with, and how it was installed, off the UI thread.
func (a *app) queueReadYTDLP() {
	tools, ok := a.pipeline.(media.Tools)
	if !ok || tools.YTDLP == "" || a.update == nil {
		return
	}
	path, update := tools.YTDLP, a.update
	go func() {
		version, _ := media.YTDLPVersion(context.Background(), path)
		install := media.FindYTDLPInstall(path)
		update(func() {
			if a.queue.ytdlp.updating {
				return
			}
			a.queue.ytdlp.path, a.queue.ytdlp.version, a.queue.ytdlp.install = path, version, install
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
	borrow := func(primary bool) films.RemedyAction {
		return films.RemedyAction{ID: remedyBorrow + browser.ID, Label: "借用 " + browser.Name + " 登录并重试", Primary: primary}
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
		r = films.Remedy{Headline: headline, Why: need + "借用已开通会员的浏览器登录即可。", Actions: []films.RemedyAction{borrow(true), swap(false)}}
	case core.FailAge:
		r = films.Remedy{Headline: "需要年龄验证", Why: "借用浏览器里已登录的 " + site + " 账号即可。", Actions: []films.RemedyAction{borrow(true)}}
	case core.FailLogin:
		r = films.Remedy{Headline: "要登录才能看", Why: hostOf(job.Source) + " 的这个视频需要账号。借用浏览器里已登录的账号即可，不保存密码。", Actions: []films.RemedyAction{borrow(true)}}
	case core.FailRegion:
		r = films.Remedy{Headline: "你所在的地区不能看", Why: "上传者限制了地区。别处有这个视频的话，换个链接。", Actions: []films.RemedyAction{swap(true)}}
	case core.FailPrivate:
		r = films.Remedy{Headline: "视频已设为私密或被删除", Why: "没法下载。别处有的话，换个链接。", Actions: []films.RemedyAction{swap(true)}}
	case core.FailDRM:
		r = films.Remedy{Headline: site + " 受 DRM 保护", Why: "视频是加密的，没法下载。有本地文件的话可以直接拖进来。", Actions: []films.RemedyAction{local(true)}}
	case core.FailUnsupported:
		r = films.Remedy{Headline: hostOf(job.Source) + " 暂不支持", Why: "没在页面里找到视频。换个链接，或先下载到本机再拖进来。", Actions: []films.RemedyAction{swap(true), local(false)}}
	case core.FailRateLimit:
		r = films.Remedy{Headline: site + " 暂时限制了下载", Why: "请求太频繁。等几分钟再重试，或借用浏览器里已登录的账号。", Actions: []films.RemedyAction{retry, borrow(false)}}
	case core.FailNetwork:
		r = films.Remedy{Headline: "连不上网络", Why: "检查网络连接后再重试。", Actions: []films.RemedyAction{retry}}
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

// remedyBrowsers is the browser a card offers to borrow a login from, the one the job borrows already or else the first installed, and another installed one to switch to; other is the zero Browser when there is none.
func (a *app) remedyBrowsers(job core.Job) (browser, other media.Browser) {
	installed := installedBrowsers()
	for _, b := range installed {
		if b.ID == job.Cookies {
			browser = b
		}
	}
	if browser.ID == "" && len(installed) > 0 {
		browser = installed[0]
	}
	if browser.ID == "" {
		browser = media.Browsers[len(media.Browsers)-1]
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
		a.queue.swap = swapState{open: true, id: job.ID, release: a.queue.swap.release}
		if a.queue.swap.release == nil {
			a.queue.swap.release = a.holdModal()
		}
	case action == remedyLocal:
		a.pickVideos()
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
			var outdated []core.Job
			for _, job := range a.library.Jobs {
				if job.Stage == core.StageFailed && job.FailKind == core.FailOutdated && films.InLibrary(a.snap().LibraryDir, job) {
					outdated = append(outdated, job)
				}
			}
			for i := len(outdated) - 1; i >= 0; i-- {
				a.queueRetry(outdated[i])
			}
			done := "下载组件已更新到 " + version
			if version == before {
				done = "下载组件 " + version + " 已是最新版本"
			}
			a.tell(fmt.Sprintf("%s，已重新开始下载 %d 个视频。", done, len(outdated)))
		})
	}()
}

func (a *app) Swapping() (*bool, films.Film, *string, string) {
	item, _ := a.JobFilm(a.queue.swap.id)
	return &a.queue.swap.open, item, &a.queue.swap.link, a.queue.swap.refused
}

// SwapLink runs the card 换个链接 is open on again from the link typed in its sheet, with what the old link told about the video forgotten. A link that is not one stays in the sheet with why.
func (a *app) SwapLink() {
	link := strings.TrimSpace(a.queue.swap.link)
	job, ok := a.library.Find(a.queue.swap.id)
	if !ok {
		a.queue.swap.open = false
		return
	}
	if msg, _ := a.precheck([]string{link}); msg != "" {
		a.queue.swap.refused = msg
		return
	}
	if _, err := core.NewJob(link); err != nil {
		a.queue.swap.refused = sourceError(err)
		return
	}
	job.Source = link
	job.Title, job.Thumbnail, job.VideoPath = "", "", ""
	a.queue.swap = swapState{release: a.queue.swap.release}
	a.queueRetry(job)
	a.tell("已换成新链接，开始下载。")
}

// queueModals gives back the window's modal flag 换个链接's sheet held once the sheet has closed, by its buttons or by Escape.
func (a *app) queueModals() {
	if !a.queue.swap.open && a.queue.swap.release != nil {
		a.queue.swap.release()
		a.queue.swap.release = nil
	}
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
