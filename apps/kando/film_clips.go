package main

import (
	"context"
	"time"

	"kando/apps/kando/film"
	"kando/packages/media"
)

// loadClipFrames fills the 片段 tab with real frames, one per chapter, read with ffmpeg off the engine's goroutine. It runs once the film's subtitles are read and once the system's player knows the video's length (playerLoad), which set the chapters, so the view builder stays pure.
func (a *app) loadClipFrames() {
	path := a.player.playPath
	if path == "" || a.update == nil || !a.player.subs.ready || len(a.player.subs.cues) == 0 {
		return
	}
	ffmpeg := a.ffmpegPath()
	if ffmpeg == "" {
		return
	}
	chapters := film.Chapters(a.player.subs.cues, a.PlayLength())
	if len(chapters) == 0 {
		return
	}
	if a.player.film.clipFrames == nil {
		a.player.film.clipFrames = map[time.Duration][]byte{}
	}
	if a.player.film.clipBusy == nil {
		a.player.film.clipBusy = map[time.Duration]bool{}
	}
	for _, chapter := range chapters {
		start := chapter.Start
		if _, ok := a.player.film.clipFrames[start]; ok || a.player.film.clipBusy[start] {
			continue
		}
		a.player.film.clipBusy[start] = true
		update := a.update
		go func() {
			data, err := media.GrabFrame(context.Background(), ffmpeg, path, start)
			update(func() {
				delete(a.player.film.clipBusy, start)
				if err == nil && len(data) > 0 && a.player.playPath == path {
					a.player.film.clipFrames[start] = data
				}
			})
		}()
	}
}
