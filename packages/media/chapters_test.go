package media

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strings"
	"testing"
	"time"

	"kando/packages/core"
)

func chapter(at float64, title, translation string) core.Chapter {
	return core.Chapter{Start: time.Duration(at * float64(time.Second)), Title: title, Translation: translation}
}

// The download reports the chapters yt-dlp read off the site, as a YouTube video with timestamps in its description prints them (captured from yt-dlp 2026.08.19, cut to its first three chapters); a video without chapters reports none.
func TestDownloadReportsTheSitesChapters(t *testing.T) {
	tools := Tools{YTDLP: fakeYTDLP(t, "testdata/ytdlp/download-chapters.stdout", "", 0), OutputDir: t.TempDir()}
	var got []core.Chapter
	ctx := WithTrace(context.Background(), &Trace{Chapters: func(list []core.Chapter) { got = list }})
	if _, err := tools.Download(ctx, "https://www.youtube.com/watch?v=rfscVS0vtbw"); err != nil {
		t.Fatal(err)
	}
	want := []core.Chapter{chapter(0, "Introduction", ""), chapter(105, "Installing Python & PyCharm", ""), chapter(400, "Setup & Hello World", "")}
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("chapters = %+v", got)
	}

	got = nil
	tools.YTDLP = fakeYTDLP(t, "testdata/ytdlp/download-youtube.stdout", "", 0)
	if _, err := tools.Download(ctx, "https://www.youtube.com/watch?v=jNQXAC9IVRw"); err != nil {
		t.Fatal(err)
	}
	if got != nil {
		t.Fatalf("a video without chapters reported %+v", got)
	}
}

// The file beside the video keeps the chapters in order, without the ones that have no title or share a start; fewer than two are none, and a use Kando does not know follows the setting.
func TestChaptersFileKeepsWhatAPlayerCanCutAt(t *testing.T) {
	video := filepath.Join(t.TempDir(), "video.mp4")
	if _, ok := ReadChapters(video); ok {
		t.Fatal("a film without the file has chapters")
	}
	err := SaveChapters(video, FilmChapters{Chapters: []core.Chapter{
		chapter(58, "Carving Valleys", "切出山谷"), chapter(0, "Intro", "开场"), chapter(24, "  ", ""), chapter(58, "Again", ""), chapter(24.5, "Where Rivers Begin", ""),
	}, Language: "简体中文", Use: core.ChaptersEven})
	if err != nil {
		t.Fatal(err)
	}
	got, ok := ReadChapters(video)
	want := FilmChapters{Chapters: []core.Chapter{chapter(0, "Intro", "开场"), chapter(24.5, "Where Rivers Begin", ""), chapter(58, "Carving Valleys", "切出山谷")}, Language: "简体中文", Use: core.ChaptersEven}
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("read %+v, %v", got, ok)
	}
	if err := os.WriteFile(ChaptersPath(video), []byte(`{"chapters":[{"start":0,"title":"A"},{"start":3,"title":"B"}],"use":"sometimes"}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if got, ok := ReadChapters(video); !ok || got.Use != "" || len(got.Chapters) != 2 {
		t.Fatalf("unknown use read %+v, %v", got, ok)
	}
	if err := os.WriteFile(ChaptersPath(video), []byte(`{"chapters":[{"start":0,"title":"Only"}]}`), 0o644); err != nil {
		t.Fatal(err)
	}
	if _, ok := ReadChapters(video); ok {
		t.Fatal("one chapter cuts the film")
	}
	if err := SetChapterUse(video, core.ChaptersPlatform); err == nil {
		t.Fatal("a film without chapters kept a use")
	}
}

// chapterPipeline is a link's pipeline whose download finds chapters: Download writes the video into dir and reports chapters, Transcribe hears two lines, and Translate prefixes each text with the language and remembers it, or fails with translateErr.
type chapterPipeline struct {
	dir          string
	chapters     []core.Chapter
	sent         []string
	translateErr error
	// stopAt, when set, stops the job (stop) as the translation is sent a text that starts with it.
	stopAt string
	stop   context.CancelFunc
}

func (p *chapterPipeline) Check() error { return nil }

func (p *chapterPipeline) Download(ctx context.Context, _ string) (string, error) {
	video := filepath.Join(p.dir, "River [abc]", "video.mp4")
	if err := os.MkdirAll(filepath.Dir(video), 0o755); err != nil {
		return "", err
	}
	reportChapters(ctx, p.chapters)
	return video, os.WriteFile(video, []byte("v"), 0o644)
}

func (p *chapterPipeline) Transcribe(context.Context, string) ([]core.Segment, error) {
	return []core.Segment{{Start: 0, End: time.Second, Text: "Rivers start small."}, {Start: 30 * time.Second, End: 31 * time.Second, Text: "They carve valleys."}}, nil
}

func (p *chapterPipeline) Translate(_ context.Context, segments []core.Segment, language string) ([]core.Segment, error) {
	if p.translateErr != nil {
		return nil, p.translateErr
	}
	if p.stopAt != "" && strings.HasPrefix(segments[0].Text, p.stopAt) {
		p.stop()
		return nil, context.Canceled
	}
	out := slices.Clone(segments)
	for i := range out {
		p.sent = append(p.sent, out[i].Text)
		out[i].Text = language + ":" + out[i].Text
	}
	return out, nil
}

func (p *chapterPipeline) Export(ctx context.Context, video string, original, translated []core.Segment) (string, error) {
	return exportSubtitles(ctx, video, original, translated)
}

func runChapters(t *testing.T, p Pipeline, job core.Job, language string) core.Job {
	t.Helper()
	done := Run(context.Background(), p, job, language, func(core.Job) {})
	if done.Stage != core.StageComplete {
		t.Fatalf("job %s: %v", done.Stage, done.Err)
	}
	return done
}

// A link's job keeps the chapters its download found beside the video and translates their titles after the lines, apart from them; translating the film into another language translates them again, a film heard in its target language keeps none, and a translation that fails as a whole leaves them as the site wrote them.
func TestRunKeepsAndTranslatesTheChapters(t *testing.T) {
	p := &chapterPipeline{dir: t.TempDir(), chapters: []core.Chapter{chapter(0, "Intro", ""), chapter(24, "Where Rivers Begin", "")}}
	job, _ := core.NewJob("https://www.youtube.com/watch?v=a1B2c3D4e5F")
	job = runChapters(t, p, job, "简体中文")
	got, ok := ReadChapters(job.VideoPath)
	want := FilmChapters{Chapters: []core.Chapter{chapter(0, "Intro", "简体中文:Intro"), chapter(24, "Where Rivers Begin", "简体中文:Where Rivers Begin")}, Language: "简体中文"}
	if !ok || !reflect.DeepEqual(got, want) {
		t.Fatalf("chapters %+v, %v", got, ok)
	}
	if want := []string{"Rivers start small.", "They carve valleys.", "Intro", "Where Rivers Begin"}; !slices.Equal(p.sent, want) {
		t.Fatalf("sent %q", p.sent)
	}

	// The user's choice for the film stays; translating only the missing lines sends no title already translated.
	if err := SetChapterUse(job.VideoPath, core.ChaptersEven); err != nil {
		t.Fatal(err)
	}
	p.sent = nil
	job.Redo = core.RedoMissing
	job = runChapters(t, p, job, "简体中文")
	if len(p.sent) != 0 {
		t.Fatalf("translating the missing lines sent %q", p.sent)
	}

	job.Redo, job.RedoLanguage = core.RedoTranslation, "日本語"
	job = runChapters(t, p, job, "日本語")
	got, _ = ReadChapters(job.VideoPath)
	if got.Language != "日本語" || got.Chapters[1].Translation != "日本語:Where Rivers Begin" || got.Use != core.ChaptersEven {
		t.Fatalf("translated again %+v", got)
	}

	// A film heard in English for English subtitles is not translated: its chapters keep no translation.
	job.Redo, job.RedoLanguage = core.RedoTranslation, "English"
	job.SpokenLanguage = "en"
	job = runChapters(t, p, job, "English")
	got, _ = ReadChapters(job.VideoPath)
	if got.Language != "English" || got.Chapters[0].Translation != "" || got.Chapters[1].Translation != "" {
		t.Fatalf("same language %+v", got)
	}

	q := &chapterPipeline{dir: t.TempDir(), chapters: p.chapters, translateErr: errors.New("no service")}
	fresh, _ := core.NewJob("https://www.youtube.com/watch?v=a1B2c3D4e5F")
	fresh = runChapters(t, q, fresh, "简体中文")
	got, ok = ReadChapters(fresh.VideoPath)
	if fresh.TranslationErr == "" || !ok || got.Language != "" || got.Chapters[0].Translation != "" {
		t.Fatalf("failed translation %+v, chapters %+v", fresh, got)
	}
}

// A download that finds the same chapters again keeps their translation; chapters the site changed are translated anew, keeping the user's choice; and one that finds none, or one, leaves the film's as they were.
func TestDownloadAgainKeepsTheChaptersChosen(t *testing.T) {
	video := filepath.Join(t.TempDir(), "video.mp4")
	kept := FilmChapters{Chapters: []core.Chapter{chapter(0, "Intro", "开场"), chapter(24, "Where Rivers Begin", "河流的源头")}, Language: "简体中文", Use: core.ChaptersPlatform}
	if err := SaveChapters(video, kept); err != nil {
		t.Fatal(err)
	}
	keepChapters(video, []core.Chapter{chapter(0, "Intro", ""), chapter(24, "Where Rivers Begin", "")}, core.ChaptersEven)
	if got, _ := ReadChapters(video); !reflect.DeepEqual(got, kept) {
		t.Fatalf("same chapters %+v", got)
	}
	keepChapters(video, []core.Chapter{chapter(0, "Only one", "")}, "")
	if got, _ := ReadChapters(video); !reflect.DeepEqual(got, kept) {
		t.Fatalf("one chapter replaced them: %+v", got)
	}
	keepChapters(video, []core.Chapter{chapter(0, "Intro", ""), chapter(30, "Sources", "")}, "")
	want := FilmChapters{Chapters: []core.Chapter{chapter(0, "Intro", ""), chapter(30, "Sources", "")}, Use: core.ChaptersPlatform}
	if got, _ := ReadChapters(video); !reflect.DeepEqual(got, want) {
		t.Fatalf("changed chapters %+v", got)
	}
	if !slices.Contains(Companions(video), ChaptersPath(video)) {
		t.Fatal("the chapters do not move with the film")
	}
}

// The first download of a film keeps the chapter use its job carries, as the Add sheet chose it, with the chapters it finds.
func TestDownloadKeepsTheJobsChapterUse(t *testing.T) {
	video := filepath.Join(t.TempDir(), "video.mp4")
	keepChapters(video, []core.Chapter{chapter(0, "Intro", ""), chapter(24, "Where Rivers Begin", "")}, core.ChaptersEven)
	want := FilmChapters{Chapters: []core.Chapter{chapter(0, "Intro", ""), chapter(24, "Where Rivers Begin", "")}, Use: core.ChaptersEven}
	if got, _ := ReadChapters(video); !reflect.DeepEqual(got, want) {
		t.Fatalf("chapters %+v", got)
	}
}

// A job stopped while it translates the chapters' titles stops, as one stopped while it translates its lines does; one whose pipeline completes although it was stopped completes.
func TestRunStoppedWhileTranslatingTheChapters(t *testing.T) {
	ctx, stop := context.WithCancel(context.Background())
	defer stop()
	p := &chapterPipeline{dir: t.TempDir(), chapters: []core.Chapter{chapter(0, "Intro", ""), chapter(24, "Where Rivers Begin", "")}, stopAt: "Intro", stop: stop}
	job, _ := core.NewJob("https://www.youtube.com/watch?v=a1B2c3D4e5F")
	job = Run(ctx, p, job, "简体中文", func(core.Job) {})
	if job.Stage != core.StageFailed || !errors.Is(job.Err, context.Canceled) {
		t.Fatalf("job %s: %v", job.Stage, job.Err)
	}
}

// cutChoosingPipeline translates as chapterPipeline does, the user choosing meanwhile how the film whose video is at video is cut (SetChapterUse), as one pressing 改按时间切分 while a job translates its titles does.
type cutChoosingPipeline struct {
	*chapterPipeline
	video string
	use   core.ChapterUse
}

func (p *cutChoosingPipeline) Translate(ctx context.Context, segments []core.Segment, language string) ([]core.Segment, error) {
	if err := SetChapterUse(p.video, p.use); err != nil {
		return nil, err
	}
	return p.chapterPipeline.Translate(ctx, segments, language)
}

// The cut the user chooses while a job translates the chapters' titles stays when the translation is written; chapters a download replaced meanwhile are not overwritten with the old ones' translation.
func TestTranslatingTheChaptersKeepsTheCutChosenMeanwhile(t *testing.T) {
	video := filepath.Join(t.TempDir(), "video.mp4")
	if err := SaveChapters(video, FilmChapters{Chapters: []core.Chapter{chapter(0, "Intro", ""), chapter(24, "Where Rivers Begin", "")}}); err != nil {
		t.Fatal(err)
	}
	p := &cutChoosingPipeline{chapterPipeline: &chapterPipeline{}, video: video, use: core.ChaptersEven}
	if err := translateChapters(context.Background(), p, video, "简体中文", false); err != nil {
		t.Fatal(err)
	}
	want := FilmChapters{Chapters: []core.Chapter{chapter(0, "Intro", "简体中文:Intro"), chapter(24, "Where Rivers Begin", "简体中文:Where Rivers Begin")}, Language: "简体中文", Use: core.ChaptersEven}
	if got, _ := ReadChapters(video); !reflect.DeepEqual(got, want) {
		t.Fatalf("chapters %+v", got)
	}

	replaced := FilmChapters{Chapters: []core.Chapter{chapter(0, "Intro", ""), chapter(30, "Sources", "")}}
	q := &replacingPipeline{chapterPipeline: &chapterPipeline{}, video: video, with: replaced}
	if err := translateChapters(context.Background(), q, video, "日本語", false); err != nil {
		t.Fatal(err)
	}
	if got, _ := ReadChapters(video); !reflect.DeepEqual(got, replaced) {
		t.Fatalf("replaced chapters %+v", got)
	}
}

// replacingPipeline translates as chapterPipeline does, the film's chapters being replaced with with meanwhile, as a download finding other chapters does.
type replacingPipeline struct {
	*chapterPipeline
	video string
	with  FilmChapters
}

func (p *replacingPipeline) Translate(ctx context.Context, segments []core.Segment, language string) ([]core.Segment, error) {
	if err := SaveChapters(p.video, p.with); err != nil {
		return nil, err
	}
	return p.chapterPipeline.Translate(ctx, segments, language)
}
