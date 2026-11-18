package main

import "kando/packages/core"

// Settings › 通用 › 在线视频's reads and edits (settings.App), over the state in state_online.go. The page saves after each edit.

func (a *app) PlatformSubtitles() bool { return !a.online.heardAlone }

func (a *app) SetPlatformSubtitles(on bool) { a.online.heardAlone = !on }

func (a *app) DownloadQuality() core.Quality { return a.onlineQuality() }

// ImportChapters is 导入平台章节: whether a film whose site marks chapters is cut at them unless the user chose otherwise for it (film.CutsAtSite). The setter's change shows in the open film's 片段 at once.
func (a *app) ImportChapters() bool { return !a.online.evenChapters }

func (a *app) SetImportChapters(on bool) {
	a.online.evenChapters = !on
	a.loadClipFrames()
}

func (a *app) SetDownloadQuality(q core.Quality) {
	if q.Known() {
		a.online.quality = q
	}
}
