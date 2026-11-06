package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"

	"lingocut/packages/media"

	"github.com/egoist/mygo/ui"
)

// The library cards' posters, one frame grabbed from each video.

func (a *app) posterOf(video string) ui.ImageSource {
	if video == "" {
		return nil
	}
	return a.posters[video]
}

// ensurePoster grabs one frame the first time a card needs it. A miss is remembered so the card does not retry every frame.
func (a *app) ensurePoster(video string) {
	if video == "" || a.update == nil || a.posters[video] != nil || a.posterMiss[video] || a.posterBusy[video] {
		return
	}
	dest := posterDest(a.posterDir, video)
	if dest == "" {
		a.markPosterMiss(video)
		return
	}
	if _, err := os.Stat(video); err != nil {
		a.markPosterMiss(video)
		return
	}
	tools, ok := a.pipeline.(media.Tools)
	replace := false
	if info, err := os.Stat(dest); err == nil && info.Size() > 0 {
		replace = media.PosterTooDark(dest)
		if !replace {
			ok = true
			tools.FFmpeg = ""
		}
	}
	if !ok || (tools.FFmpeg == "" && !fileExists(dest)) {
		a.markPosterMiss(video)
		return
	}
	if a.posterBusy == nil {
		a.posterBusy = map[string]bool{}
	}
	a.posterBusy[video] = true
	update, ffmpeg := a.update, tools.FFmpeg
	go func() {
		var err error
		if _, statErr := os.Stat(dest); statErr != nil || replace {
			err = media.GrabPoster(context.Background(), ffmpeg, video, dest)
		}
		var img ui.ImageSource
		if err == nil {
			data, readErr := os.ReadFile(dest)
			if readErr != nil {
				err = readErr
			} else {
				img, err = ui.DecodeImage(data)
			}
		}
		update(func() {
			delete(a.posterBusy, video)
			if err != nil || img == nil {
				a.markPosterMiss(video)
				return
			}
			if a.posters == nil {
				a.posters = map[string]ui.ImageSource{}
			}
			a.posters[video] = img
		})
	}()
}

func (a *app) markPosterMiss(video string) {
	if a.posterMiss == nil {
		a.posterMiss = map[string]bool{}
	}
	a.posterMiss[video] = true
}

func posterKey(video string) string {
	sum := sha256.Sum256([]byte(video))
	return hex.EncodeToString(sum[:8])
}

func posterDest(posterDir, video string) string {
	if strings.TrimSuffix(filepath.Base(video), filepath.Ext(video)) == "video" {
		return filepath.Join(filepath.Dir(video), "cover.jpg")
	}
	if posterDir == "" {
		return ""
	}
	return filepath.Join(posterDir, posterKey(video)+".jpg")
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() > 0
}
