package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"image"
	_ "image/gif" // coverJPEG
	"image/jpeg"
	_ "image/png" // coverJPEG
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	_ "golang.org/x/image/webp" // coverJPEG: YouTube's thumbnails are often WebP

	"kando/apps/kando/films"
	"kando/packages/media"

	"github.com/egoist/mygo/ui"
)

// The library cards' posters: each video's own cover, the picture its site gives it or the cover art its file carries, and one frame grabbed from the video only when it has neither.

func (a *app) posterOf(video string) ui.ImageSource {
	if video == "" {
		return nil
	}
	return a.lib.posters[video]
}

// ensurePoster finds a film's cover the first time the shelf has it (findCover), and reads how long the video plays. A miss is remembered so the video is not tried again. loadExtras calls it; the page only shows what it found.
func (a *app) ensurePoster(item films.Film) {
	video := item.VideoPath
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
	tools, _ := a.pipeline.(media.Tools)
	cover := coverSources{ffmpeg: tools.FFmpeg, video: video, dest: dest}
	if item.Thumbnail != "" && a.lib.posterDir != "" {
		cover.site, cover.siteCache = item.Thumbnail, siteThumbnailPath(a.lib.posterDir, item.Thumbnail)
	}
	if cover.ffmpeg == "" && cover.site == "" && !fileExists(dest) {
		a.markPosterMiss(video)
		return
	}
	if a.lib.posterBusy == nil {
		a.lib.posterBusy = map[string]bool{}
	}
	a.lib.posterBusy[video] = true
	update := a.update
	go func() {
		var length time.Duration
		if cover.ffmpeg != "" {
			length = media.VideoDuration(cover.ffmpeg, video)
		}
		var img ui.ImageSource
		data, err := findCover(context.Background(), cover)
		if err == nil {
			img, err = ui.DecodeImage(data)
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

// coverSources is where a film's cover can come from: the address of the picture its site gives it and where that picture is kept once fetched ("" for a local video), the ffmpeg that reads the video file, and dest, the cover.jpg the cover is kept in.
type coverSources struct {
	site, siteCache string
	ffmpeg, video   string
	dest            string
}

// fetchSiteThumbnail downloads a site's picture of a video, and embeddedCover reads the cover art a video file carries. Tests replace them.
var (
	fetchSiteThumbnail = media.FetchThumbnail
	embeddedCover      = media.EmbeddedCover
)

// findCover returns a film's cover as JPEG and keeps it in dest, off the UI thread. The cover is the video's own: the picture its site gives it, else the cover art its file carries; dest is rewritten whenever it holds something else, so a frame an earlier Kando grabbed gives way to the video's own cover. Only a video with neither keeps the cover dest already has, unless that is a black frame, or has a frame grabbed from it.
func findCover(ctx context.Context, from coverSources) ([]byte, error) {
	var own []byte
	if from.site != "" {
		own = siteThumbnail(ctx, from.site, from.siteCache)
	}
	if own == nil && from.ffmpeg != "" {
		own, _ = embeddedCover(ctx, from.ffmpeg, from.video)
	}
	if own != nil {
		if cover, err := coverJPEG(own); err == nil {
			if kept, _ := os.ReadFile(from.dest); !bytes.Equal(kept, cover) {
				// The cover shows even when it cannot be kept, as in a library on a read-only volume.
				_ = writeCover(from.dest, cover)
			}
			return cover, nil
		}
	}
	if fileExists(from.dest) && (from.ffmpeg == "" || !media.PosterTooDark(from.dest)) {
		return os.ReadFile(from.dest)
	}
	if err := media.GrabPoster(ctx, from.ffmpeg, from.video, from.dest); err != nil {
		return nil, err
	}
	return os.ReadFile(from.dest)
}

// siteThumbnail is the picture at address, read from cache, where the processing card's thumbnail keeps it too, or fetched there first; nil when it cannot be had, as when the site no longer serves it.
func siteThumbnail(ctx context.Context, address, cache string) []byte {
	if !fileExists(cache) {
		if err := fetchSiteThumbnail(ctx, address, cache); err != nil {
			return nil
		}
	}
	data, err := os.ReadFile(cache)
	if err != nil || len(data) == 0 {
		return nil
	}
	return data
}

// siteThumbnailPath is where the picture at a thumbnail address is kept in the posters folder.
func siteThumbnailPath(posterDir, address string) string {
	return filepath.Join(posterDir, "site-"+posterKey(address))
}

// coverJPEG is a cover picture as JPEG, so cover.jpg is what its name says: a JPEG as it is, and a PNG, GIF or WebP, as sites and cover art often are, encoded again.
func coverJPEG(data []byte) ([]byte, error) {
	if http.DetectContentType(data) == "image/jpeg" {
		return data, nil
	}
	img, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, err
	}
	var out bytes.Buffer
	if err := jpeg.Encode(&out, img, &jpeg.Options{Quality: 90}); err != nil {
		return nil, err
	}
	return out.Bytes(), nil
}

// writeCover replaces dest with data whole, so a reader never sees half a cover.
func writeCover(dest string, data []byte) error {
	if err := os.MkdirAll(filepath.Dir(dest), 0o755); err != nil {
		return err
	}
	tmp := dest + ".part"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, dest)
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

// The processing cards' thumbnails: the picture a video's site gives it, shown until the video is downloaded and has a poster of its own, and on a card whose download failed.

func (a *app) thumbnailOf(item films.Film) ui.ImageSource {
	if item.Thumbnail == "" {
		return nil
	}
	return a.lib.thumbs[item.Thumbnail]
}

// ensureThumbnail reads the picture at a film's thumbnail address from the posters folder, where it is kept by its address, or fetches it there first, off the UI thread. A miss is remembered so it is not tried again while the app runs. loadExtras calls it; the page only shows what it found.
func (a *app) ensureThumbnail(item films.Film) {
	address := item.Thumbnail
	if address == "" || a.update == nil || a.lib.posterDir == "" || a.lib.thumbs[address] != nil || a.lib.thumbMiss[address] || a.lib.thumbBusy[address] {
		return
	}
	if a.lib.thumbBusy == nil {
		a.lib.thumbBusy = map[string]bool{}
	}
	a.lib.thumbBusy[address] = true
	dest := siteThumbnailPath(a.lib.posterDir, address)
	update := a.update
	go func() {
		var err error
		if !fileExists(dest) {
			err = fetchSiteThumbnail(context.Background(), address, dest)
		}
		var img ui.ImageSource
		if err == nil {
			var data []byte
			if data, err = os.ReadFile(dest); err == nil {
				img, err = ui.DecodeImage(data)
			}
		}
		update(func() {
			delete(a.lib.thumbBusy, address)
			if err != nil || img == nil {
				if a.lib.thumbMiss == nil {
					a.lib.thumbMiss = map[string]bool{}
				}
				a.lib.thumbMiss[address] = true
				return
			}
			if a.lib.thumbs == nil {
				a.lib.thumbs = map[string]ui.ImageSource{}
			}
			a.lib.thumbs[address] = img
		})
	}()
}
