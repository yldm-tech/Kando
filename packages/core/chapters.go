package core

import (
	"slices"
	"strings"
	"time"
)

// Chapter is a part of a link's film as its site marks it: the timestamps the uploader wrote in a YouTube video's description, or the 视频看点 of a bilibili video, as yt-dlp reads them. Start is where it starts in the video; Title is what the site calls it and Translation that title in the language the film's subtitles are translated into, "" until it is translated and for a film heard in that language.
type Chapter struct {
	Start       time.Duration
	Title       string
	Translation string
}

// ChapterUse is whether a film's player cuts it at its site's chapters or evenly by time, as the user chose for that film. Empty follows the setting 导入平台章节 (Settings › 通用 › 在线视频).
type ChapterUse string

const (
	// ChaptersPlatform cuts the film at its site's chapters.
	ChaptersPlatform ChapterUse = "platform"
	// ChaptersEven cuts the film evenly by time, at its lines, as a film without chapters is.
	ChaptersEven ChapterUse = "even"
)

// Known reports whether u is one of the uses Kando knows; empty is not.
func (u ChapterUse) Known() bool {
	return u == ChaptersPlatform || u == ChaptersEven
}

// CleanChapters is chapters in the order of their starts, without the ones that have no title or start before the video or at the same time as another, which keeps the first. A film needs two chapters to be cut at them, so fewer than two is none.
func CleanChapters(chapters []Chapter) []Chapter {
	out := make([]Chapter, 0, len(chapters))
	for _, c := range chapters {
		c.Title = strings.TrimSpace(c.Title)
		c.Translation = strings.TrimSpace(c.Translation)
		if c.Title == "" || c.Start < 0 {
			continue
		}
		out = append(out, c)
	}
	slices.SortStableFunc(out, func(a, b Chapter) int {
		switch {
		case a.Start < b.Start:
			return -1
		case a.Start > b.Start:
			return 1
		}
		return 0
	})
	out = slices.CompactFunc(out, func(a, b Chapter) bool { return a.Start == b.Start })
	if len(out) < 2 {
		return nil
	}
	return out
}
