package main

import "kando/packages/core"

// Settings › 通用 › 在线视频's reads and edits (settings.App), over the state in state_online.go. The page saves after each edit.

func (a *app) PlatformSubtitles() bool { return !a.online.heardAlone }

func (a *app) SetPlatformSubtitles(on bool) { a.online.heardAlone = !on }

func (a *app) DownloadQuality() core.Quality { return a.onlineQuality() }

func (a *app) SetDownloadQuality(q core.Quality) {
	if q.Known() {
		a.online.quality = q
	}
}
