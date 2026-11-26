package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
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
	"kando/apps/kando/mediasource"
	"kando/apps/kando/vault"
	"kando/packages/media"

	ns "github.com/nsgo-dev/nsgo/core"
)

// The library cards' posters: each video's own cover, the picture its site gives it or the cover art its file carries, and one frame grabbed from the video only when it has neither.

// posterOf is the cover of video as the cards show it, the zero ImageSource until one was found.
func (a *app) posterOf(video string) ns.ImageSource {
	if video == "" {
		return ns.ImageSource{}
	}
	return a.lib.posters[video]
}

// linkedPosterTimeout bounds fetching a cover or probing the stream of a film from a media source.
const linkedPosterTimeout = mediasource.PoolOpenTimeout

// ensurePoster finds a film's cover the first time the shelf has it (findCover), and reads how long the video plays. A miss is remembered so the video is not tried again. loadExtras calls it; the page only shows what it found.
func (a *app) ensurePoster(item films.Film) {
	video := item.VideoPath
	if video == "" || a.update == nil || libraryHasImage(a.lib.posters[video]) || a.lib.posterMiss[video] || a.lib.posterBusy[video] {
		return
	}
	dest := posterFor(a.lib.posterDir, video, item.Home)
	if dest == "" {
		a.markPosterMiss(video)
		return
	}
	// A video moved to a storage location is not here for ffmpeg to read: its cover is the one kept before it moved, or its site's picture (state_stored.go).
	_, stored := a.storedOf(video)
	l, linked := a.linkedOf(video)
	if _, err := os.Stat(video); err != nil && !stored && !linked {
		a.markPosterMiss(video)
		return
	}
	tools, _ := a.pipeline.(media.Tools)
	cover := coverSources{ffmpeg: tools.FFmpeg, video: video, dest: dest}
	if stored {
		cover.ffmpeg = ""
	}
	if dir := posterFolder(a.lib.posterDir, video); item.Thumbnail != "" && dir != "" {
		// Offline mode fetches no site's picture: only one fetched before is used.
		if cache := siteThumbnailPath(dir, item.Thumbnail); !a.privacy.offline || fileExists(cache) {
			cover.site, cover.siteCache = item.Thumbnail, cache
		}
	}
	switch {
	case linked && fileExists(dest):
		// An indexed video whose poster is already cached on disk reads it there without reaching its source.
		cover.ffmpeg = ""
	case linked:
		i := a.mediaSourceIndex(l.Source)
		if i < 0 {
			a.markPosterMiss(video)
			return
		}
		if _, inPool := a.sources.pool.Source(l.Source); !inPool {
			// The source's pool or plugin kind is not ready yet (e.g. startup before mediaSourceHooks' ready or plugin load): try again once it is, without marking a miss.
			return
		}
		if k, ok := a.sources.kinds.Kind(a.sources.list[i].Kind); cover.ffmpeg == "" && (!ok || k.Origin.Builtin()) {
			a.markPosterMiss(video)
			return
		}
		cover.source = mediasource.Ref{Source: l.Source, Path: l.Path}.String()
		cover.copier = a.mediaSourceCopier()
		cover.linked = true
	case !stored && !fileExists(dest) && media.IsSourceRef(item.Source):
		// A video copied from a media server (such as Plex) takes the server's artwork when its poster has not been written yet, falling back to its local file.
		if ref, err := mediasource.ParseRef(item.Source); err == nil {
			if _, inPool := a.sources.pool.Source(ref.Source); inPool {
				if i := a.mediaSourceIndex(ref.Source); i >= 0 {
					if k, ok := a.sources.kinds.Kind(a.sources.list[i].Kind); ok && !k.Origin.Builtin() {
						cover.source = item.Source
						cover.copier = a.mediaSourceCopier()
					}
				}
			}
		}
	}
	if cover.ffmpeg == "" && cover.site == "" && cover.source == "" && !fileExists(dest) {
		a.markPosterMiss(video)
		return
	}
	if a.lib.posterBusy == nil {
		a.lib.posterBusy = map[string]bool{}
	}
	a.lib.posterBusy[video] = true
	if a.lib.posterSlots == nil {
		a.lib.posterSlots = make(chan struct{}, posterLimit)
	}
	slots, update := a.lib.posterSlots, a.update
	go func() {
		slots <- struct{}{}
		defer func() { <-slots }()
		ctx := context.Background()
		var cancel context.CancelFunc
		if cover.source != "" {
			ctx, cancel = context.WithTimeout(ctx, linkedPosterTimeout)
			defer cancel()
		}
		var length time.Duration
		var offline bool
		var data []byte
		var err error
		if cover.linked {
			data, length, offline, err = linkedCover(ctx, cover)
		} else {
			if cover.ffmpeg != "" {
				header := media.ReadHeaderContext(ctx, cover.ffmpeg, video)
				length, cover.sound = header.Length, header.Read && !header.Picture
				cover.length = length
			}
			// The cover is kept as the JPEG findCover returns, which the cards show as it is, once it reads as a picture.
			data, err = findCover(ctx, cover)
		}
		if err == nil {
			_, _, err = image.DecodeConfig(bytes.NewReader(data))
		}
		update(func() {
			delete(a.lib.posterBusy, video)
			if length > 0 {
				a.libraryLearnDuration(video, length)
			}
			if offline {
				// Offline mode kept an internet media source closed; try again once it is off.
				return
			}
			if err != nil || len(data) == 0 {
				a.markPosterMiss(video)
				return
			}
			if a.lib.posters == nil {
				a.lib.posters = map[string]ns.ImageSource{}
			}
			a.lib.posters[video] = ns.ImageSource{Data: data}
			if a.player.play != nil && a.player.play.path == video {
				a.playerNowPlayingArtwork(video)
			}
		})
	}()
}

// libraryLearnDuration keeps length as how long the video at path plays when the poster's probe has not told it, as before the probe ends or where it cannot run (no ffmpeg, a cover that was not found), once the player has opened the video and knows it: a film's length is what the check over its poster and 标记为已看完 go by (films.WatchState). For an indexed video whose length library.json has not kept yet, it notes it there too so the next launch knows it without probing the source again.
func (a *app) libraryLearnDuration(path string, length time.Duration) {
	if path == "" || length <= 0 {
		return
	}
	if a.lib.durations[path] <= 0 {
		if a.lib.durations == nil {
			a.lib.durations = map[string]time.Duration{}
		}
		a.lib.durations[path] = length
	}
	if l, ok := a.linkedOf(path); ok && l.Duration <= 0 {
		l.Duration = length
		a.library.Linked[path] = l
		if err := a.library.Save(); err != nil {
			a.saveErr = a.shelfSaveErr(err, a.messages().LibraryShelfSaveFailed)
		}
	}
}

// coverSources is where a film's cover can come from: the address of the picture its site gives it and where that picture is kept once fetched ("" for a local video), the media source ref and copier when the film came from a media source (source, copier, with linked set when the video itself is only on the source), the ffmpeg that reads the video file, its already-probed length, and dest, the cover.jpg the cover is kept in. sound is true for a film of sound alone, whose cover the player shows in place of a picture (media.WideCover).
type coverSources struct {
	site, siteCache string
	source          string
	copier          mediasource.Copier
	linked          bool
	ffmpeg, video   string
	length          time.Duration
	dest            string
	sound           bool
}

// fetchSiteThumbnail downloads a site's picture of a video, and embeddedCover reads the cover art a video file carries. Tests replace them.
var (
	fetchSiteThumbnail = media.FetchThumbnail
	embeddedCover      = media.EmbeddedCover
)

// linkedCover finds the cover of a film indexed from a media source: the server's own artwork when its driver provides one (CoverDriver), else the embedded cover or a frame grabbed by ffmpeg from the source's loopback stream URL, which is revoked as ctx ends. offline reports that offline mode kept an internet source closed.
func linkedCover(ctx context.Context, from coverSources) (data []byte, length time.Duration, offline bool, err error) {
	if own, cerr := from.copier.Cover(ctx, from.source); cerr == nil && len(own) > 0 {
		if cover, perr := prepareSourceCover(ctx, from.ffmpeg, own); perr == nil {
			if kept, _ := os.ReadFile(from.dest); !bytes.Equal(kept, cover) {
				_ = writeCover(from.dest, cover)
			}
			return cover, 0, false, nil
		}
	} else if errors.Is(cerr, media.ErrOffline) {
		return nil, 0, true, cerr
	}
	if from.ffmpeg == "" {
		return nil, 0, false, errors.New("poster source is missing")
	}
	stream, err := from.copier.URL(ctx, from.source)
	if err != nil {
		return nil, 0, errors.Is(err, media.ErrOffline), err
	}
	header := media.ReadHeaderContext(ctx, from.ffmpeg, stream)
	from.video = stream
	from.length = header.Length
	from.sound = header.Read && !header.Picture
	from.source = ""
	data, err = findCover(ctx, from)
	return data, header.Length, false, err
}

// prepareSourceCover encodes a media source's artwork as JPEG, widening portrait artwork (such as a movie's poster when the server has no 16:9 fanart) into 16:9 over a blurred backdrop so cards do not crop off titles or faces.
func prepareSourceCover(ctx context.Context, ffmpeg string, own []byte) ([]byte, error) {
	cover, err := coverJPEG(own)
	if err != nil {
		return nil, err
	}
	if ffmpeg != "" {
		if cfg, _, err := image.DecodeConfig(bytes.NewReader(cover)); err == nil && cfg.Height > 0 && cfg.Width < cfg.Height {
			if wide, err := media.WideCover(ctx, ffmpeg, cover); err == nil {
				if widened, err := coverJPEG(wide); err == nil {
					return widened, nil
				}
			}
		}
	}
	return cover, nil
}

// findCover returns a film's cover as JPEG and keeps it in dest, off the UI thread. The cover is the video's own: the picture its site or media source gives it, else the cover art its file carries, made 16:9 for a film of sound alone, whose art is mostly square (media.WideCover); dest is rewritten whenever it holds something else, so a frame an earlier Kando grabbed gives way to the video's own cover. Only a video with neither keeps the cover dest already has, unless that is a black frame, or has a frame grabbed from it, for sound alone a picture of its sound (media.GrabPoster).
func findCover(ctx context.Context, from coverSources) ([]byte, error) {
	var own []byte
	if from.site != "" {
		own = siteThumbnail(ctx, from.site, from.siteCache)
	}
	if own == nil && from.source != "" {
		if pic, err := from.copier.Cover(ctx, from.source); err == nil && len(pic) > 0 {
			if prepared, err := prepareSourceCover(ctx, from.ffmpeg, pic); err == nil {
				own = prepared
			}
		}
	}
	if own == nil && from.ffmpeg != "" {
		own, _ = embeddedCover(ctx, from.ffmpeg, from.video)
	}
	if own != nil && from.sound {
		if wide, err := media.WideCover(ctx, from.ffmpeg, own); err == nil {
			own = wide
		}
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
	if err := media.GrabPosterWithDuration(ctx, from.ffmpeg, from.video, from.dest, from.length); err != nil {
		return nil, err
	}
	return os.ReadFile(from.dest)
}

// forgetLinkedPosterMiss drops posterMiss for films indexed from sourceID (or all indexed films when sourceID is ""), so ensurePoster tries them again once their source is reachable or changed.
func (a *app) forgetLinkedPosterMiss(sourceID string) {
	for video, l := range a.library.Linked {
		if sourceID == "" || l.Source == sourceID {
			delete(a.lib.posterMiss, video)
		}
	}
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

// posterLimit is how many posters are looked for at once.
const posterLimit = 4

// posterDest is where the poster of the video at video is kept: cover.jpg beside a video.<ext> of a film's folder in a library bundle of Kando's own, which Kando laid out, else a file named after the video in posterDir. A video.<ext> anywhere else is the user's, as in a folder kept for another player, whose cover.jpg is the user's own and is never written.
func posterDest(posterDir, video string) string {
	if strings.TrimSuffix(filepath.Base(video), filepath.Ext(video)) == "video" && inBundle(video) {
		return filepath.Join(filepath.Dir(video), "cover.jpg")
	}
	posterDir = posterFolder(posterDir, video)
	if posterDir == "" {
		return ""
	}
	return filepath.Join(posterDir, posterKey(video)+".jpg")
}

// posterFor is where the poster of the film whose video is at video is kept, home being the folder of its files in its library for a video kept outside it (core.Job.Home): cover.jpg there, as for a film the bundle holds, once that folder is there, else where posterDest keeps it. Finding a poster never makes the folder, nor the library it is in.
func posterFor(posterDir, video, home string) string {
	if home != "" {
		if info, err := os.Stat(home); err == nil && info.IsDir() {
			return filepath.Join(home, "cover.jpg")
		}
	}
	return posterDest(posterDir, video)
}

// inBundle reports whether path is inside a library bundle of Kando's own (media.LibraryExt).
func inBundle(path string) bool {
	for dir := filepath.Dir(filepath.Clean(path)); ; dir = filepath.Dir(dir) {
		if shelfBundle(dir) {
			return true
		}
		if parent := filepath.Dir(dir); parent == dir {
			return false
		}
	}
}

// posterVaultDir is the hidden folder inside an encrypted library (package vault) where its posters and site thumbnails are kept in place of the app's cache, which is not encrypted: a frame of a film would show there what the library hides.
const posterVaultDir = ".kando-posters"

// posterFolder is where the poster or site thumbnail of the video at video is kept: inside its library when that library is encrypted and open (vault.RootOf), else posterDir, the app's cache.
func posterFolder(posterDir, video string) string {
	if root, ok := vault.RootOf(video); ok {
		return filepath.Join(root, posterVaultDir)
	}
	return posterDir
}

func fileExists(path string) bool {
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() > 0
}

// The processing cards' thumbnails: the picture a video's site gives it, shown until the video is downloaded and has a poster of its own, and on a card whose download failed.

// thumbnailOf is the picture a film's site gives it, the zero ImageSource until it was fetched.
func (a *app) thumbnailOf(item films.Film) ns.ImageSource {
	if item.Thumbnail == "" {
		return ns.ImageSource{}
	}
	return a.lib.thumbs[item.Thumbnail]
}

// thumbnailFolder is where the site thumbnail of item is kept: inside the library its job saves into when that library is encrypted and open, as its posters are (posterFolder), else the app's cache. A film still downloading has no video to tell its library by.
func (a *app) thumbnailFolder(item films.Film) string {
	lib := a.snap().LibraryDir
	if job, ok := a.library.Find(item.JobID); ok && job.LibraryDir != "" {
		lib = job.LibraryDir
	}
	if vault.Mounted(lib) {
		return filepath.Join(lib, posterVaultDir)
	}
	return a.lib.posterDir
}

// libraryHasImage reports whether source names a picture.
func libraryHasImage(source ns.ImageSource) bool { return source.URI != "" || len(source.Data) > 0 }

// ensureThumbnail reads the picture at a film's thumbnail address from the posters folder, where it is kept by its address, or fetches it there first, off the UI thread. A miss is remembered so it is not tried again while the app runs. loadExtras calls it; the page only shows what it found.
func (a *app) ensureThumbnail(item films.Film) {
	address := item.Thumbnail
	if address == "" || a.update == nil || a.lib.posterDir == "" || libraryHasImage(a.lib.thumbs[address]) || a.lib.thumbMiss[address] || a.lib.thumbBusy[address] {
		return
	}
	if a.lib.thumbBusy == nil {
		a.lib.thumbBusy = map[string]bool{}
	}
	dest := siteThumbnailPath(a.thumbnailFolder(item), address)
	// Offline mode fetches no site's picture; one not fetched yet is tried again once it is off.
	if a.privacy.offline && !fileExists(dest) {
		return
	}
	a.lib.thumbBusy[address] = true
	update := a.update
	go func() {
		var err error
		if !fileExists(dest) {
			err = fetchSiteThumbnail(context.Background(), address, dest)
		}
		// The picture is kept as a JPEG, whatever the site served (coverJPEG), which also tells a picture from a page the site answered with instead.
		var img []byte
		if err == nil {
			var data []byte
			if data, err = os.ReadFile(dest); err == nil {
				img, err = coverJPEG(data)
			}
		}
		update(func() {
			delete(a.lib.thumbBusy, address)
			if err != nil || len(img) == 0 {
				if a.lib.thumbMiss == nil {
					a.lib.thumbMiss = map[string]bool{}
				}
				a.lib.thumbMiss[address] = true
				return
			}
			if a.lib.thumbs == nil {
				a.lib.thumbs = map[string]ns.ImageSource{}
			}
			a.lib.thumbs[address] = ns.ImageSource{Data: img}
		})
	}()
}
