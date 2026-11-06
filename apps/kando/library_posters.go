package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"os"
	"path/filepath"
	"strings"
	"time"

	"kando/packages/media"

	"github.com/egoist/mygo/ui"
)

// The library cards' posters, one frame grabbed from each video.

func (a *app) posterOf(video string) ui.ImageSource {
	if video == "" {
		return nil
	}
	return a.lib.posters[video]
}

// ensurePoster grabs one frame of a video the first time the shelf has it, and reads how long the video plays with it. A miss is remembered so the video is not tried again. loadExtras calls it; the page only shows what it found.
func (a *app) ensurePoster(video string) {
	if video == "" || a.update == nil || a.lib.posters[video] != nil || a.lib.posterMiss[video] || a.lib.posterBusy[video] {
		return
	}
	dest := posterDest(a.lib.posterDir, video)
	if dest == "" {
		a.markPosterMiss(video)
		return
	}
	if _, err := os.Stat(video); err != nil {
		a.markPosterMiss(video)
		return
	}
	tools, ok := a.pipeline.(media.Tools)
	probe := tools.FFmpeg
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
	if a.lib.posterBusy == nil {
		a.lib.posterBusy = map[string]bool{}
	}
	a.lib.posterBusy[video] = true
	update, ffmpeg := a.update, tools.FFmpeg
	go func() {
		var length time.Duration
		if probe != "" {
			length = media.VideoDuration(probe, video)
		}
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
			delete(a.lib.posterBusy, video)
			if length > 0 {
				if a.lib.durations == nil {
					a.lib.durations = map[string]time.Duration{}
				}
				a.lib.durations[video] = length
			}
			if err != nil || img == nil {
				a.markPosterMiss(video)
				return
			}
			if a.lib.posters == nil {
				a.lib.posters = map[string]ui.ImageSource{}
			}
			a.lib.posters[video] = img
		})
	}()
}

func (a *app) markPosterMiss(video string) {
	if a.lib.posterMiss == nil {
		a.lib.posterMiss = map[string]bool{}
	}
	a.lib.posterMiss[video] = true
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
