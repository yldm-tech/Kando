// Package download fetches a film from a paired desktop into its folder in the store: its subtitles and poster first, then its video, by ranges, so a download that stops, paused or cut off, goes on from where it stopped.
package download

import (
	"context"
	"errors"
	"io"
	"mime"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"kando/apps/mobile/store"
	"kando/packages/sync"
)

// Progress hears how far the video is: done bytes of total.
type Progress func(done, total int64)

// Source is where a film's files come from: its desktop on the local network (*sync.Client), or, away from it, the user's storage by the links the desktop lists in its state.json (sync.RelayFilms), which answer alike.
type Source interface {
	Subtitles(ctx context.Context, id string) ([]sync.Cue, error)
	Poster(ctx context.Context, id string) ([]byte, string, error)
	Video(ctx context.Context, id string, from, to int64, ifRange string) (*sync.Part, error)
}

// Film downloads f from src into its folder in st and returns its record as it then stands, which it also keeps: whole, or as far as it got when ctx ended or the download failed. The subtitles and the poster come first, once (a film the desktop changes later takes its new subtitles when it is played, Follow); then the video goes on from the part already kept, if the desktop's file has not changed since (its ETag), else from the start.
func Film(ctx context.Context, c Source, st *store.Store, f store.Film, progress Progress) (store.Film, error) {
	if err := st.SaveFilm(f); err != nil {
		return f, err
	}
	if _, ok, err := st.Cues(f.Desktop, f.Film.ID); err != nil {
		return f, err
	} else if !ok {
		cues, err := c.Subtitles(ctx, f.Film.ID)
		if err != nil {
			return f, err
		}
		if err := st.SaveCues(f.Desktop, f.Film.ID, cues); err != nil {
			return f, err
		}
	}
	dir := st.FilmDir(f.Desktop, f.Film.ID)
	if f.Poster == "" && f.Film.Poster != "" {
		name, err := poster(ctx, c, f.Film.ID, dir)
		var e *sync.Error
		switch {
		case errors.As(err, &e) && e.Status == http.StatusNotFound:
			// A poster the desktop lost since it listed the film is no reason to stop.
		case err != nil:
			return f, err
		default:
			f.Poster = name
			if err := st.SaveFilm(f); err != nil {
				return f, err
			}
		}
	}
	if f.Done() {
		return f, nil
	}
	name, err := video(ctx, c, f.Film.ID, dir, f.ETag, func(etag string) error {
		if etag == f.ETag {
			return nil
		}
		f.ETag = etag
		return st.SaveFilm(f)
	}, progress)
	if err != nil {
		return f, err
	}
	f.Video = name
	return f, st.SaveFilm(f)
}

// Follow asks f's desktop whether it changed f since the phone has it (sync.Film.Updated), as by translating it again or proofreading its transcript, and returns the desktop's record of it and its subtitles, and true, when it did. It keeps nothing: the caller merges the record into the phone's own, whose video and poster stay as they were downloaded, and keeps them. A film the desktop lists in none of its libraries, or has not changed, gives false; so does one whose desktop cannot be reached, with the error.
func Follow(ctx context.Context, c *sync.Client, f store.Film) (sync.Film, []sync.Cue, bool, error) {
	listed, ok, err := listedFilm(ctx, c, f.Film.ID)
	if err != nil || !ok || !listed.Updated.After(f.Film.Updated) {
		return sync.Film{}, nil, false, err
	}
	cues, err := c.Subtitles(ctx, f.Film.ID)
	if err != nil {
		return sync.Film{}, nil, false, err
	}
	return listed, cues, true, nil
}

// listedFilm is film id as its desktop lists it, looked for in each of its libraries, as the phone keeps a film's library by the name it had, which may have changed since; false when no library lists it.
func listedFilm(ctx context.Context, c *sync.Client, id string) (sync.Film, bool, error) {
	libs, err := c.Libraries(ctx)
	if err != nil {
		return sync.Film{}, false, err
	}
	for _, lib := range libs {
		films, err := c.Films(ctx, lib.ID)
		if err != nil {
			return sync.Film{}, false, err
		}
		for _, f := range films {
			if f.ID == id {
				return f, true, nil
			}
		}
	}
	return sync.Film{}, false, nil
}

// poster keeps the film's poster in dir and returns the file's name.
func poster(ctx context.Context, c Source, id, dir string) (string, error) {
	data, kind, err := c.Poster(ctx, id)
	if err != nil {
		return "", err
	}
	name := "poster" + extension(kind, ".jpg")
	return name, os.WriteFile(filepath.Join(dir, name), data, 0o600)
}

// video downloads the film's video into dir's part file, from as far as it goes when its ETag is etag, and once whole renames it after its type, returning the name. saveETag keeps the ETag of the answer before any of its bytes are written.
func video(ctx context.Context, c Source, id, dir, etag string, saveETag func(string) error, progress Progress) (string, error) {
	partPath := filepath.Join(dir, store.PartFile)
	var from int64
	if info, err := os.Stat(partPath); err == nil && etag != "" {
		from = info.Size()
	}
	ifRange := ""
	if from > 0 {
		ifRange = etag
	}
	part, err := c.Video(ctx, id, from, -1, ifRange)
	var e *sync.Error
	if errors.As(err, &e) && e.Status == http.StatusRequestedRangeNotSatisfiable && from > 0 {
		// The part is as long as the file or longer: ask for it whole.
		from = 0
		part, err = c.Video(ctx, id, 0, -1, "")
	}
	if err != nil {
		return "", err
	}
	defer part.Body.Close()
	if err := saveETag(part.ETag); err != nil {
		return "", err
	}
	flags := os.O_WRONLY | os.O_CREATE
	if part.Start == 0 {
		// The answer starts the file again: the part kept was of a file that changed, or there was none.
		flags |= os.O_TRUNC
	} else {
		flags |= os.O_APPEND
	}
	out, err := os.OpenFile(partPath, flags, 0o600)
	if err != nil {
		return "", err
	}
	done := part.Start
	progress(done, part.Total)
	buf := make([]byte, 256<<10)
	for {
		n, rerr := part.Body.Read(buf)
		if n > 0 {
			if _, err := out.Write(buf[:n]); err != nil {
				out.Close()
				return "", err
			}
			done += int64(n)
			progress(done, part.Total)
		}
		if rerr == io.EOF {
			break
		}
		if rerr != nil {
			out.Close()
			if ctx.Err() != nil {
				return "", ctx.Err()
			}
			return "", rerr
		}
	}
	if err := out.Close(); err != nil {
		return "", err
	}
	if part.Total >= 0 && done != part.Total {
		return "", io.ErrUnexpectedEOF
	}
	name := "video" + extension(part.ContentType, ".mp4")
	return name, os.Rename(partPath, filepath.Join(dir, name))
}

// fileTypes are the extensions of the types of the videos and posters a desktop serves: the phone's player tells a video's container by its file's extension, and mime's own table lacks some of them on some systems.
var fileTypes = map[string]string{
	"video/mp4":        ".mp4",
	"video/x-m4v":      ".m4v",
	"video/quicktime":  ".mov",
	"video/webm":       ".webm",
	"video/x-matroska": ".mkv",
	"image/jpeg":       ".jpg",
	"image/png":        ".png",
	"image/webp":       ".webp",
}

// extension is the file extension of content type kind, else fallback.
func extension(kind, fallback string) string {
	t, _, err := mime.ParseMediaType(kind)
	if err != nil {
		return fallback
	}
	if ext, ok := fileTypes[strings.ToLower(t)]; ok {
		return ext
	}
	return fallback
}
