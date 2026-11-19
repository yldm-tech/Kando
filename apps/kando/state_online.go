package main

import (
	"net/url"

	"kando/packages/core"
)

// Settings › 通用 › 在线视频: whether a link's film takes the subtitles the uploader put on the site as its original lines where they match the speech (core.SubtitlesPlatform), and the picture its download asks for (core.Quality). The queue gives them to each link's job that has none of its own as it first runs (queueRun, onlineFor), and the job keeps them. 导入平台章节 is whether films whose site marks chapters are cut at them, which the player reads as it shows them, unless the user chose otherwise for a film.

// onlineUI is what 在线视频 chose. The zero value is the defaults: the site's subtitles, at the best picture.
type onlineUI struct {
	// heardAlone is true once the user turned off 优先用平台自带的字幕: links are heard with the speech recognition alone.
	heardAlone bool
	// quality is the picture downloads ask for, "" for the best.
	quality core.Quality
	// evenChapters is true once the user turned off 导入平台章节: films whose site marks chapters are cut evenly by time unless the user chose their chapters for one (film.CutsAtSite).
	evenChapters bool
}

// savedOnline is 在线视频's part of settings.json: the source of the original lines, "" for the site's subtitles, and the quality, "" for the best.
type savedOnline struct {
	SubtitleSource  core.SubtitleSource `json:"subtitleSource,omitempty"`
	DownloadQuality core.Quality        `json:"downloadQuality,omitempty"`
	// EvenChapters is 导入平台章节 turned off.
	EvenChapters bool `json:"evenChapters,omitempty"`
}

// clean keeps only a source and a quality Kando knows, as another version may have saved one it does not.
func (s *savedOnline) clean() {
	if !s.SubtitleSource.Known() {
		s.SubtitleSource = ""
	}
	if !s.DownloadQuality.Known() {
		s.DownloadQuality = ""
	}
}

// onlineHooks: the choices are saved and taken back at startup.
func (a *app) onlineHooks() areaHooks {
	return areaHooks{
		save: func(saved *savedSettings) {
			part := savedOnline{}
			if a.online.heardAlone {
				part.SubtitleSource = core.SubtitlesLocal
			}
			if a.online.quality != core.QualityBest {
				part.DownloadQuality = a.online.quality
			}
			part.EvenChapters = a.online.evenChapters
			saved.savedOnline = part
		},
		restore: func(saved savedSettings) {
			a.online = onlineUI{heardAlone: saved.SubtitleSource == core.SubtitlesLocal, quality: saved.DownloadQuality, evenChapters: saved.EvenChapters}
		},
	}
}

// onlineSource is where a link's original lines come from unless its job says.
func (a *app) onlineSource() core.SubtitleSource {
	if a.online.heardAlone {
		return core.SubtitlesLocal
	}
	return core.SubtitlesPlatform
}

// onlineQuality is the picture a link's download asks for unless its job says.
func (a *app) onlineQuality() core.Quality {
	if a.online.quality.Known() {
		return a.online.quality
	}
	return core.QualityBest
}

// onlineFor is job with the source of the original lines and the quality 在线视频 names, where job is a link's that has none of its own, a link a paired phone sent too, as the phone studies a film that keeps no video by its lines and asks the computer for its video when it is to be watched (psync.OpFilmVideo); a video on disk is neither downloaded nor has a site, and keeps none.
func (a *app) onlineFor(job core.Job) core.Job {
	if u, err := url.Parse(job.Source); err != nil || (u.Scheme != "http" && u.Scheme != "https") {
		return job
	}
	if !job.SubtitleSource.Known() {
		job.SubtitleSource = a.onlineSource()
	}
	if !job.Quality.Known() {
		job.Quality = a.onlineQuality()
	}
	return job
}
