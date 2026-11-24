package main

import (
	"context"
	"errors"
	"log"
	"os"
	"time"

	"kando/apps/kando/saved"
)

// The sound of each saved line and each word of the vocabulary is cut out of its video as it is saved and kept in the library (saved.StoredSound), so the review and the export have it once the video is gone, as when the film was deleted to make room or streams from its site.

// savedKeepSound cuts the sound of l, a line just saved or the line of a word just added (saved.WordLine), with ffmpeg off the engine's goroutine, into the library, unless it is kept already, is being cut, or its video is neither on this computer nor on the media source it was indexed from (savedSoundSource) or ffmpeg is not at hand. The review, open by then, plays it once it is cut (savedSoundKept).
func (a *app) savedKeepSound(l saved.Line) {
	out := saved.StoredSound(a.saved.book, l.ID)
	ffmpeg, update := a.ffmpegPath(), a.update
	if out == "" || ffmpeg == "" || l.Video == "" || update == nil || a.saved.keeping[l.ID] || saved.HasSound(out) {
		return
	}
	video := a.saved.book.Abs(l)
	from := a.savedSoundSource(video)
	if from == nil {
		return
	}
	delay := a.player.delays[video]
	id := l.ID
	a.saved.keeping[id] = true
	go func() {
		err := savedKeepSoundFrom(context.Background(), ffmpeg, from, video, l, delay, out)
		update(func() {
			delete(a.saved.keeping, id)
			if err != nil {
				log.Printf("saved: the sound of %s was not kept: %v", id, err)
				out = ""
			}
			// Taken out of the study record while it was cut: the sound goes too, unless the library changed meanwhile, whose book this one is not.
			if out != "" && saved.StoredSound(a.saved.book, id) == out && a.savedFind(id) < 0 && a.saved.book.WordByID(id) < 0 {
				_ = os.Remove(out)
				out = ""
			}
			a.savedSoundKept(id, out)
		})
	}()
}

// savedSoundKept is told once the sound of the line or word id was cut as it was saved, out, "" when it could not be: the review open now plays it if it waits for it.
func (a *app) savedSoundKept(id, out string) {
	r := &a.saved.review
	if !r.open || r.cutting[id] {
		return
	}
	if _, ok := r.clips[id]; ok {
		return
	}
	for k, item := range r.queue {
		if k > 1 {
			break
		}
		if item.id == id {
			if out == "" {
				// It could not be kept as it was saved: the review cuts it itself.
				a.savedReviewCut()
				return
			}
			a.savedReviewSound(id, out)
			return
		}
	}
}

// savedSweepSounds takes away the sounds kept in the open library that no saved line or word has any more (saved.SweepSounds), as the library is read, leaving those being cut now. A sound follows its line or word, not its film: one whose film was deleted stays while its line does, so the review and the export still have it.
func (a *app) savedSweepSounds() {
	cutting := func(id string) bool { return a.saved.keeping[id] || a.saved.review.cutting[id] }
	if n := saved.SweepSounds(a.saved.book, cutting); n > 0 {
		log.Printf("saved: took away %d sounds no saved line or word has", n)
	}
}

// savedDropSound takes away the sound kept of the line or word id, as it is taken out of the study record.
func (a *app) savedDropSound(id string) {
	if path := saved.StoredSound(a.saved.book, id); path != "" {
		_ = os.Remove(path)
	}
}

// savedSoundSource is what ffmpeg cuts the sound of a line of the film whose video is at video from, off the engine's goroutine: the file, when it is on this computer; for a film indexed from a media source, the address the sources' pool serves it at until the context ends (linkedStreams), "" when the source cannot be read then; nil when there is neither, as for a video moved to a storage location or gone.
func (a *app) savedSoundSource(video string) func(ctx context.Context) string {
	if info, err := os.Stat(video); err == nil && !info.IsDir() {
		return func(context.Context) string { return video }
	}
	if !a.linkedHas(video) {
		return nil
	}
	streams := a.linkedStreams()
	return func(ctx context.Context) string { return streams(ctx, video) }
}

// errSoundUnread is savedKeepSoundFrom's error once its source gave nothing to cut from, as a media source that cannot be read now.
var errSoundUnread = errors.New("the video could not be read to cut its sound")

// savedKeepSoundFrom keeps the sound of l (saved.KeepSound) cut from what from gives, the video at video or the address it is served at, which is given back once the sound is cut.
func savedKeepSoundFrom(ctx context.Context, ffmpeg string, from func(ctx context.Context) string, video string, l saved.Line, delay time.Duration, out string) error {
	ctx, cancel := context.WithCancel(ctx)
	defer cancel()
	at := from(ctx)
	if at == "" {
		return errSoundUnread
	}
	return saved.KeepSound(ctx, ffmpeg, at, l, delay, out)
}
