package media

import (
	"context"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"kando/packages/core"
)

// captionPipeline is platformPipeline whose site also made captions of its own, counting the fetches by language; onCaptions, when set, runs as it fetches them.
type captionPipeline struct {
	*platformPipeline
	captions   []core.Segment
	asked      []string
	onCaptions func()
}

func (p *captionPipeline) AutoCaptions(_ context.Context, source, language string) ([]core.Segment, error) {
	p.asked = append(p.asked, language)
	if p.onCaptions != nil {
		p.onCaptions()
	}
	return p.captions, nil
}

// A link's job that keeps its site's captions as a reference layer hears the film alone, never takes the uploader's subtitles, and keeps the captions in the language heard beside the video, where the film's other files go when it moves.
func TestRunKeepsTheSitesCaptionsAsAReference(t *testing.T) {
	p := &captionPipeline{platformPipeline: newPlatformPipeline(t), captions: []core.Segment{seg(0, 2, "hello every one"), seg(2, 4, "welcome to the show")}}
	job, _ := core.NewJob("https://www.youtube.com/watch?v=jNQXAC9IVRw")
	job.SubtitleSource = core.SubtitlesAuto
	done := Run(context.Background(), p, job, "简体中文", func(core.Job) {})
	if done.Stage != core.StageComplete || done.PlatformSubtitles || len(p.fetched) != 0 {
		t.Fatalf("job %+v, fetched the uploader's %q", done, p.fetched)
	}
	if want := []string{"hello everyone", "welcome to the shoe", "um, so"}; !reflect.DeepEqual(texts(p.exported), want) {
		t.Fatalf("exported %q", texts(p.exported))
	}
	if !reflect.DeepEqual(p.asked, []string{"en"}) {
		t.Fatalf("asked for the captions in %q", p.asked)
	}
	if got := texts(ReadReference(p.video)); !reflect.DeepEqual(got, []string{"hello every one", "welcome to the show"}) {
		t.Fatalf("reference %q", got)
	}
	if !strings.HasPrefix(filepath.Base(ReferencePath(p.video)), ".") {
		t.Fatalf("the reference layer is not hidden: %s", ReferencePath(p.video))
	}
	found := false
	for _, path := range Companions(p.video) {
		found = found || path == ReferencePath(p.video)
	}
	if !found {
		t.Fatalf("companions %q lack the reference layer", Companions(p.video))
	}
}

// A job that does not keep the captions, a local video and a site without them leave the film without a reference layer; a job stopped while it fetches them stops.
func TestRunWithoutTheSitesCaptions(t *testing.T) {
	cases := map[string]func(*captionPipeline, *core.Job){
		"heard alone":   func(_ *captionPipeline, job *core.Job) { job.SubtitleSource = core.SubtitlesLocal },
		"uploader's":    func(_ *captionPipeline, job *core.Job) { job.SubtitleSource = core.SubtitlesPlatform },
		"none":          func(p *captionPipeline, _ *core.Job) { p.captions = nil },
		"a local video": func(_ *captionPipeline, job *core.Job) { job.Source = "file:///films/talk.mp4" },
	}
	for name, tweak := range cases {
		t.Run(name, func(t *testing.T) {
			p := &captionPipeline{platformPipeline: newPlatformPipeline(t), captions: []core.Segment{seg(0, 2, "hello")}}
			job, _ := core.NewJob("https://www.youtube.com/watch?v=jNQXAC9IVRw")
			job.SubtitleSource = core.SubtitlesAuto
			tweak(p, &job)
			done := Run(context.Background(), p, job, "简体中文", func(core.Job) {})
			if done.Stage != core.StageComplete {
				t.Fatalf("job %+v", done)
			}
			if _, err := os.Stat(ReferencePath(p.video)); err == nil {
				t.Fatal("kept a reference layer")
			}
		})
	}

	p := &captionPipeline{platformPipeline: newPlatformPipeline(t), captions: []core.Segment{seg(0, 2, "hello")}}
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	p.onCaptions = cancel
	job, _ := core.NewJob("https://www.youtube.com/watch?v=jNQXAC9IVRw")
	job.SubtitleSource = core.SubtitlesAuto
	if done := Run(ctx, p, job, "简体中文", func(core.Job) {}); done.Stage == core.StageComplete {
		t.Fatalf("a job stopped while fetching the captions completed: %+v", done)
	}
}

// AutoCaptions asks yt-dlp for YouTube's automatic captions alone, and for bilibili's AI subtitles, which yt-dlp lists among the uploader's.
func TestAutoCaptionsFetchesTheSitesOwn(t *testing.T) {
	dir := t.TempDir()
	args := filepath.Join(dir, "args")
	tool := writeFakeTool(t, dir, "yt-dlp", fakeTool{Args: args, Act: "subtitles", Files: map[string]string{
		"subs.en.srt": "1\n00:00:01,000 --> 00:00:02,500\nhello there\n\n",
	}})
	lines, err := (Tools{YTDLP: tool}).AutoCaptions(context.Background(), "https://www.youtube.com/watch?v=jNQXAC9IVRw", "en")
	if err != nil {
		t.Fatal(err)
	}
	if want := []core.Segment{{Start: time.Second, End: 2500 * time.Millisecond, Text: "hello there"}}; !reflect.DeepEqual(lines, want) {
		t.Fatalf("lines %+v", lines)
	}
	data, _ := os.ReadFile(args)
	if got := string(data); !strings.Contains(got, "--write-auto-subs\n--sub-langs\n^en(-.*)?$\n") || strings.Contains(got, "--write-subs\n") {
		t.Fatalf("args %s", got)
	}
	if got := autoCaptionArgs(core.PlatformBilibili, "zh"); !reflect.DeepEqual(got, []string{"--write-subs", "--sub-langs", "^ai-zh(-.*)?$"}) {
		t.Fatalf("bilibili %q", got)
	}
}

// Preview reads a video's title, uploader, length and which subtitles its site has from yt-dlp -J, without the playlist it is in; bilibili's AI subtitles count as the site's own, and its danmaku as none.
func TestPreviewReadsTheVideo(t *testing.T) {
	dir := t.TempDir()
	args := filepath.Join(dir, "args")
	tool := writeFakeTool(t, dir, "yt-dlp", fakeTool{Args: args, Stdout: `{"_type":"video","title":" Me at the zoo ","channel":"jawed","uploader":"jawed","duration":19.4,"subtitles":{},"automatic_captions":{"en":[{"ext":"vtt"}]}}`})
	got, err := (Tools{YTDLP: tool, Cookies: "chrome"}).Preview(context.Background(), "https://www.youtube.com/watch?v=jNQXAC9IVRw&list=PL1")
	if err != nil {
		t.Fatal(err)
	}
	if want := (Preview{Title: "Me at the zoo", Uploader: "jawed", Duration: 19 * time.Second, AutoCaptions: true}); got != want {
		t.Fatalf("preview %+v", got)
	}
	data, _ := os.ReadFile(args)
	for _, want := range []string{"-J\n", "--no-playlist\n", "--skip-download\n", "--cookies-from-browser\nchrome\n", "--\nhttps://www.youtube.com/watch?v=jNQXAC9IVRw&list=PL1\n"} {
		if !strings.Contains(string(data), want) {
			t.Fatalf("args lack %q: %s", want, data)
		}
	}

	bili, err := parsePreview([]byte(`{"title":"【中字】冰川","uploader":"地理君","duration":600,"subtitles":{"danmaku":[{}],"ai-zh":[{}],"zh-CN":[{}]}}`))
	if err != nil || bili != (Preview{Title: "【中字】冰川", Uploader: "地理君", Duration: 10 * time.Minute, Subtitles: true, AutoCaptions: true}) {
		t.Fatalf("bilibili %+v, %v", bili, err)
	}
	if only, _ := parsePreview([]byte(`{"title":"t","subtitles":{"danmaku":[{}]}}`)); only.Subtitles || only.AutoCaptions {
		t.Fatalf("danmaku counted as subtitles: %+v", only)
	}
	if _, err := parsePreview([]byte(`{"_type":"playlist","title":"list"}`)); err == nil {
		t.Fatal("a playlist read as a video")
	}
	if _, err := (Tools{}).Preview(context.Background(), "https://youtu.be/jNQXAC9IVRw"); err == nil {
		t.Fatal("previewed without yt-dlp")
	}
}
