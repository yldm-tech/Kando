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

// DropGoneSounds takes away the sounds kept of the lines and words of before that after, the same library's study record later, has no more, as a merge with another device's removals takes them out (study.Book.Merge, MergeWords): the sound of a line or word follows its record, not its film's video, so the review and the export still have it after the video is gone.
func DropGoneSounds(before, after Book) {
	for _, l := range before.Lines {
		if after.Index(l.ID) < 0 {
			removeSound(StoredSound(after, l.ID))
		}
	}
	for _, w := range before.Words {
		if after.WordByID(w.ID) < 0 {
			removeSound(StoredSound(after, w.ID))
		}
	}
}

// SweepSounds takes away the sounds kept in b's library (SoundDir) that no line or word of b has any more, and what cuts stopped halfway left beside them, as when a film's lines were taken out while Kando was not running or by a version that left their sounds behind. A sound of an id cutting reports is being cut now stays, with its half. It does nothing for a book whose study record could not be read, as it does not know its lines then, and it returns how many files it took away.
func SweepSounds(b Book, cutting func(id string) bool) int {
	if b.Dir() == "" || b.Unread() != nil {
		return 0
	}
	entries, err := os.ReadDir(filepath.Join(b.Dir(), SoundDir))
	if err != nil {
		return 0
	}
	n := 0
	for _, e := range entries {
		name := e.Name()
		if e.IsDir() || !strings.HasSuffix(name, ".m4a") {
			continue
		}
		id, half := strings.CutSuffix(strings.TrimSuffix(name, ".m4a"), ".cutting")
		if cutting != nil && cutting(id) {
			continue
		}
		if !half && (b.Index(id) >= 0 || b.WordByID(id) >= 0) {
			continue
		}
		if os.Remove(filepath.Join(b.Dir(), SoundDir, name)) == nil {
			n++
		}
	}
	return n
}

// removeSound takes away the sound kept at path, "" for none.
func removeSound(path string) {
	if path != "" {
		_ = os.Remove(path)
	}
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
	// A film whose video is not on this computer, as one indexed from a media source, is cut from where the app serves it (WithStreams); the delay still goes by its path in the library.
	at := video
	if !cut && ffmpeg != "" && l.Video != "" {
		if stream := streamOf(ctx, video); stream != "" {
			at, cut = stream, true
		}
	}
	if !cut && !HasSound(kept) {
		return "", nil
	}
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return "", nil
	}
	if cut {
		from, to := ClipSpan(l, delay(video))
		err := CutClip(ctx, ffmpeg, at, from, to, out)
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

// SoundFor writes the sound of l into dir as an export does (SoundName): cut out of its video with ffmpeg, else copied from the sound kept when it was saved, and returns its path, "" when it has none; an error only when ctx stopped. A plugin reading a line's sound (kando.sound.read) takes it this way, as the Anki export does.
func SoundFor(ctx context.Context, ffmpeg string, b Book, l Line, delay time.Duration, dir string) (string, error) {
	name, err := soundFor(ctx, ffmpeg, b, l, func(string) time.Duration { return delay }, dir)
	if name == "" || err != nil {
		return "", err
	}
	return filepath.Join(dir, name), nil
}

// streamsKey is the key of WithStreams' function in a context.
type streamsKey struct{}

// WithStreams is ctx carrying stream: for a film's video that is not on this computer, by its path in the library, the address ffmpeg reads it at until ctx ends, "" for none, so the export and a plugin reading a line's sound cut it from there, as from a film indexed from a media source.
func WithStreams(ctx context.Context, stream func(ctx context.Context, video string) string) context.Context {
	return context.WithValue(ctx, streamsKey{}, stream)
}

// streamOf is the address WithStreams gives video at under ctx, "" for none.
func streamOf(ctx context.Context, video string) string {
	if stream, ok := ctx.Value(streamsKey{}).(func(context.Context, string) string); ok && stream != nil {
		return stream(ctx, video)
	}
	return ""
}
