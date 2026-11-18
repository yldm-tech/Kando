package media

import (
	"context"
	"encoding/json"
	"errors"
	"math"
	"os"
	"strings"
	"sync"
	"time"

	"kando/packages/core"
)

// FilmChapters is what Kando keeps of a link's film's chapters, as its site marks them (core.Chapter), in a hidden file beside its video (ChaptersPath): the chapters, the language their titles are translated into, and whether the user chose to cut this film at them or evenly by time (Use).
type FilmChapters struct {
	Chapters []core.Chapter
	// Language is the language of the chapters' Translation, as the job's target language names it (简体中文); empty until a job translated them. A film heard in that language keeps no translation.
	Language string
	Use      core.ChapterUse
}

// storedChapters is the file ChaptersPath names. Starts are in seconds, to the millisecond.
type storedChapters struct {
	Chapters []storedChapter `json:"chapters"`
	Language string          `json:"language,omitempty"`
	Use      core.ChapterUse `json:"use,omitempty"`
}

type storedChapter struct {
	Start       float64 `json:"start"`
	Title       string  `json:"title"`
	Translation string  `json:"translation,omitempty"`
}

// ChaptersPath is the hidden file beside the video at videoPath that keeps its chapters (FilmChapters).
func ChaptersPath(videoPath string) string {
	return hiddenBeside(videoPath, ".chapters.json")
}

// ReadChapters is the chapters kept beside the video at videoPath, ok false when there are none, or the file cannot be read or holds fewer than two (core.CleanChapters). A use Kando does not know follows the setting.
func ReadChapters(videoPath string) (FilmChapters, bool) {
	if videoPath == "" {
		return FilmChapters{}, false
	}
	data, err := os.ReadFile(ChaptersPath(videoPath))
	if err != nil {
		return FilmChapters{}, false
	}
	var stored storedChapters
	if json.Unmarshal(data, &stored) != nil {
		return FilmChapters{}, false
	}
	list := make([]core.Chapter, 0, len(stored.Chapters))
	for _, c := range stored.Chapters {
		if math.IsNaN(c.Start) || math.IsInf(c.Start, 0) {
			continue
		}
		list = append(list, core.Chapter{Start: seconds(c.Start), Title: c.Title, Translation: c.Translation})
	}
	out := FilmChapters{Chapters: core.CleanChapters(list), Language: stored.Language, Use: stored.Use}
	if !out.Use.Known() {
		out.Use = ""
	}
	if len(out.Chapters) == 0 {
		return FilmChapters{}, false
	}
	return out, true
}

// SaveChapters writes chapters beside the video at videoPath (ChaptersPath), whole or not at all (writeFileAtomic).
func SaveChapters(videoPath string, chapters FilmChapters) error {
	if videoPath == "" {
		return errors.New("video path is empty")
	}
	stored := storedChapters{Chapters: make([]storedChapter, 0, len(chapters.Chapters)), Language: chapters.Language, Use: chapters.Use}
	for _, c := range chapters.Chapters {
		stored.Chapters = append(stored.Chapters, storedChapter{Start: math.Round(c.Start.Seconds()*1000) / 1000, Title: c.Title, Translation: c.Translation})
	}
	data, err := json.MarshalIndent(stored, "", "  ")
	if err != nil {
		return err
	}
	return writeFileAtomic(ChaptersPath(videoPath), append(data, '\n'))
}

// chaptersMu serialises the changes to chapters files (changeChapters): a job translating a film's titles and the user choosing how the open film is cut both rewrite its file, from different goroutines.
var chaptersMu sync.Mutex

// changeChapters reads the chapters beside the video at videoPath, has change edit them, and writes them back when it returns true, with no other change to a chapters file in between. ok tells change whether there were any.
func changeChapters(videoPath string, change func(chapters *FilmChapters, ok bool) bool) error {
	chaptersMu.Lock()
	defer chaptersMu.Unlock()
	chapters, ok := ReadChapters(videoPath)
	if !change(&chapters, ok) {
		return nil
	}
	return SaveChapters(videoPath, chapters)
}

// SetChapterUse keeps use, whether the film whose video is at videoPath is cut at its site's chapters or evenly by time, "" to follow the setting, with its chapters. A film without chapters has nothing to keep it with.
func SetChapterUse(videoPath string, use core.ChapterUse) error {
	found := true
	err := changeChapters(videoPath, func(chapters *FilmChapters, ok bool) bool {
		found = ok
		chapters.Use = use
		return ok
	})
	if !found {
		return errors.New("the film has no chapters")
	}
	return err
}

// keepChapters keeps the chapters a download found beside the video at videoPath. A film that already had chapters with the same titles at the same times keeps their translation and the use the user chose; one whose chapters changed keeps the use alone, and its titles are translated again. Fewer than two chapters keep nothing, and leave the film's as they were.
func keepChapters(videoPath string, found []core.Chapter) {
	found = core.CleanChapters(found)
	if len(found) == 0 {
		return
	}
	_ = changeChapters(videoPath, func(chapters *FilmChapters, ok bool) bool {
		next := FilmChapters{Chapters: found}
		if ok {
			next.Use = chapters.Use
			if sameChapters(chapters.Chapters, found) {
				next.Chapters, next.Language = chapters.Chapters, chapters.Language
			}
		}
		*chapters = next
		return true
	})
}

// sameChapters reports whether a and b are the same chapters, by start and title.
func sameChapters(a, b []core.Chapter) bool {
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i].Start != b[i].Start || a[i].Title != b[i].Title {
			return false
		}
	}
	return true
}

// translateChapters translates the titles of the chapters beside the video at videoPath into language with p, as a job translates its lines: all of them when they were translated into another language, otherwise those that have no translation yet. A film heard in language (same) keeps none, its titles being in it already. A translation that fails leaves the titles as they were, untranslated ones shown as the site wrote them; it reports no progress, as the stage's is the lines'. The file is read again when the translation is back and only the titles' translations and language are written into it, so a cut the user chose meanwhile (SetChapterUse) stays; chapters a download replaced meanwhile keep none of it. It returns ctx's error only when the translation failed because ctx was done, so the job stops as it would while translating its lines; nil otherwise.
func translateChapters(ctx context.Context, p Pipeline, videoPath, language string, same bool) error {
	chapters, ok := ReadChapters(videoPath)
	if !ok {
		return nil
	}
	if same {
		if chapters.Language == language && !anyTranslated(chapters.Chapters) {
			return nil
		}
		_ = changeChapters(videoPath, func(now *FilmChapters, ok bool) bool {
			for i := range now.Chapters {
				now.Chapters[i].Translation = ""
			}
			now.Language = language
			return ok
		})
		return nil
	}
	if chapters.Language != language {
		for i := range chapters.Chapters {
			chapters.Chapters[i].Translation = ""
		}
	}
	var titles []core.Segment
	var at []int
	for i, c := range chapters.Chapters {
		if c.Translation != "" {
			continue
		}
		end := c.Start + time.Second
		if i+1 < len(chapters.Chapters) {
			end = chapters.Chapters[i+1].Start
		}
		titles = append(titles, core.Segment{Start: c.Start, End: end, Text: c.Title})
		at = append(at, i)
	}
	if len(titles) == 0 {
		if chapters.Language != language {
			_ = changeChapters(videoPath, func(now *FilmChapters, ok bool) bool {
				now.Language = language
				return ok && sameChapters(now.Chapters, chapters.Chapters)
			})
		}
		return nil
	}
	got, err := p.Translate(WithTrace(ctx, &Trace{}), titles, language)
	var partial *UntranslatedError
	if errors.As(err, &partial) {
		err = nil
	}
	if err != nil {
		return ctx.Err()
	}
	if len(got) != len(titles) {
		return nil
	}
	for k, i := range at {
		chapters.Chapters[i].Translation = strings.TrimSpace(got[k].Text)
	}
	_ = changeChapters(videoPath, func(now *FilmChapters, ok bool) bool {
		if !ok || !sameChapters(now.Chapters, chapters.Chapters) {
			return false
		}
		now.Chapters, now.Language = chapters.Chapters, language
		return true
	})
	return nil
}

func anyTranslated(chapters []core.Chapter) bool {
	for _, c := range chapters {
		if c.Translation != "" {
			return true
		}
	}
	return false
}

// ytdlpChapter is a chapter as yt-dlp prints it in a video's info: its start in seconds and its title.
type ytdlpChapter struct {
	Start *float64 `json:"start_time"`
	Title string   `json:"title"`
}

// chaptersOf is the chapters yt-dlp printed, without the ones it gave no start.
func chaptersOf(raw []ytdlpChapter) []core.Chapter {
	var out []core.Chapter
	for _, c := range raw {
		if c.Start == nil || math.IsNaN(*c.Start) || math.IsInf(*c.Start, 0) {
			continue
		}
		out = append(out, core.Chapter{Start: seconds(*c.Start), Title: c.Title})
	}
	return out
}

// seconds is s seconds, to the millisecond.
func seconds(s float64) time.Duration {
	return time.Duration(math.Round(s*1000)) * time.Millisecond
}
