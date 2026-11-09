package main

import (
	"context"
	"time"

	"kando/apps/kando/film"
	"kando/packages/media"

	"github.com/egoist/mygo/ui"
)

// loadClipFrames fills the 片段 tab with real frames, one per chapter. Work starts from the hook so the view builder stays pure.
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
		a.player.film.clipFrames = map[time.Duration]ui.ImageSource{}
	}
	if a.player.film.clipBusy == nil {
		a.player.film.clipBusy = map[time.Duration]bool{}
	}
	for _, chapter := range chapters {
		start := chapter.Start
		if a.player.film.clipFrames[start] != nil || a.player.film.clipBusy[start] {
			continue
		}
		a.player.film.clipBusy[start] = true
		update := a.update
		go func() {
			data, err := media.GrabFrame(context.Background(), ffmpeg, path, start)
			var img ui.ImageSource
			if err == nil {
				img, err = ui.DecodeImage(data)
			}
			update(func() {
				delete(a.player.film.clipBusy, start)
				if err == nil && img != nil && a.player.playPath == path {
					a.player.film.clipFrames[start] = img
				}
			})
		}()
	}
}
