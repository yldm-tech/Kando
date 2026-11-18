package saved

import (
	"context"
	"io"
	"os"
	"path/filepath"
	"strings"
	"time"
)

// SoundDir is the hidden folder at the top of a library where the sound of each saved line and each word of the vocabulary is kept, cut out of its video as it is saved, so the review and the export still have it once the video is gone. The library lists only videos and passes over hidden folders, so it is not taken for a film, and in an encrypted library it is inside the encrypted image with the films.
const SoundDir = ".kando-sounds"

// StoredSound is the file the sound of the line or word id is kept in, in the library of b (SoundDir); "" for a book without a folder or a record without an id.
func StoredSound(b Book, id string) string {
	if b.Dir() == "" || strings.TrimSpace(id) == "" || strings.ContainsAny(id, `/\.`) {
		return ""
	}
	return filepath.Join(b.Dir(), SoundDir, id+".m4a")
}

// HasSound reports whether path is a sound kept already: a file that is not empty.
func HasSound(path string) bool {
	if path == "" {
		return false
	}
	info, err := os.Stat(path)
	return err == nil && !info.IsDir() && info.Size() > 0
}

// WordLine is the line w was looked up in, as a Line whose sound is cut for the word (ClipSpan, SoundName): its id is the word's, so its sound is kept under the word's id (StoredSound).
func WordLine(w Word) Line {
	return Line{ID: w.ID, Video: w.Video, Start: w.Start, End: w.End, Original: w.Context, Translation: w.ContextTranslation, Film: w.Film}
}

// KeepSound cuts the sound of l out of the video at video, with its subtitles moved by delay (ClipSpan), into out, as StoredSound names it: into a file beside it first, which takes its name once it is whole, so a cut stopped halfway leaves no sound that would pass for one.
func KeepSound(ctx context.Context, ffmpeg, video string, l Line, delay time.Duration, out string) error {
	if err := os.MkdirAll(filepath.Dir(out), 0o755); err != nil {
		return err
	}
	tmp := strings.TrimSuffix(out, ".m4a") + ".cutting.m4a"
	from, to := ClipSpan(l, delay)
	if err := CutClip(ctx, ffmpeg, video, from, to, tmp); err != nil {
		_ = os.Remove(tmp)
		return err
	}
	return os.Rename(tmp, out)
}

// MoveSound moves the sound kept of the line id in from's library to to's, kept there under next, as the line moves with its film to another library and may take another id there; nothing is done when from keeps none. Across disks, as into or out of an encrypted library, it is copied and then taken away from from.
func MoveSound(from Book, id string, to Book, next string) error {
	src, dst := StoredSound(from, id), StoredSound(to, next)
	if src == "" || dst == "" || src == dst || !HasSound(src) {
		return nil
	}
	if err := os.MkdirAll(filepath.Dir(dst), 0o755); err != nil {
		return err
	}
	if os.Rename(src, dst) == nil {
		return nil
	}
	if err := copySound(src, dst); err != nil {
		return err
	}
	return os.Remove(src)
}

// copySound copies the sound kept at from to to.
func copySound(from, to string) error {
	in, err := os.Open(from)
	if err != nil {
		return err
	}
	defer in.Close()
	out, err := os.Create(to)
	if err != nil {
		return err
	}
	if _, err := io.Copy(out, in); err != nil {
		_ = out.Close()
		_ = os.Remove(to)
		return err
	}
	return out.Close()
}

// soundFor writes the sound of l into dir, as SoundName names it, for an export: cut out of its video, which follows the subtitle delay set now, or where the video is gone or ffmpeg is not at hand or could not cut it, copied from the sound kept when it was saved (StoredSound) in b's library. It returns the file's name, "" when it has no sound, and an error only when ctx stopped.
func soundFor(ctx context.Context, ffmpeg string, b Book, l Line, delay func(video string) time.Duration, dir string) (string, error) {
	name := SoundName(l)
	out := filepath.Join(dir, name)
	video := b.Abs(l)
	kept := StoredSound(b, l.ID)
	_, err := os.Stat(video)
	cut := ffmpeg != "" && l.Video != "" && err == nil
	if !cut && !HasSound(kept) {
		return "", nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", nil
	}
	if cut {
		from, to := ClipSpan(l, delay(video))
		err := CutClip(ctx, ffmpeg, video, from, to, out)
		if err == nil {
			return name, nil
		}
		if ctx.Err() != nil {
			return "", ctx.Err()
		}
	}
	if HasSound(kept) {
		if err := copySound(kept, out); err == nil {
			return name, nil
		}
	}
	return "", nil
}
