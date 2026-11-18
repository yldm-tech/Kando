package main

import (
	"context"
	"log"
	"os"

	"kando/apps/kando/saved"
)

// The sound of each saved line and each word of the vocabulary is cut out of its video as it is saved and kept in the library (saved.StoredSound), so the review and the export have it once the video is gone, as when the film was deleted to make room or streams from its site.

// savedKeepSound cuts the sound of l, a line just saved or the line of a word just added (saved.WordLine), with ffmpeg off the engine's goroutine, into the library, unless it is kept already, is being cut, or its video is not on this computer or ffmpeg is not at hand. The review, open by then, plays it once it is cut (savedSoundKept).
func (a *app) savedKeepSound(l saved.Line) {
	out := saved.StoredSound(a.saved.book, l.ID)
	ffmpeg, update := a.ffmpegPath(), a.update
	if out == "" || ffmpeg == "" || l.Video == "" || update == nil || a.saved.keeping[l.ID] || saved.HasSound(out) {
		return
	}
	video := a.saved.book.Abs(l)
	if info, err := os.Stat(video); err != nil || info.IsDir() {
		return
	}
	delay := a.player.delays[video]
	id := l.ID
	a.saved.keeping[id] = true
	go func() {
		err := saved.KeepSound(context.Background(), ffmpeg, video, l, delay, out)
		update(func() {
			delete(a.saved.keeping, id)
			if err != nil {
				log.Printf("saved: the sound of %s was not kept: %v", id, err)
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

// savedDropSound takes away the sound kept of the line or word id, as it is taken out of the study record.
func (a *app) savedDropSound(id string) {
	if path := saved.StoredSound(a.saved.book, id); path != "" {
		_ = os.Remove(path)
	}
}
