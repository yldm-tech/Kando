package core

import "testing"

func TestNewJobTrimsSourceAndQueuesIt(t *testing.T) {
	job, err := NewJob(" https://example.com/video ")
	if err != nil {
		t.Fatal(err)
	}
	if job.ID == "" || job.Source != "https://example.com/video" || job.Stage != StageQueued {
		t.Fatalf("unexpected job: %#v", job)
	}
	other, err := NewJob("https://example.com/other")
	if err != nil {
		t.Fatal(err)
	}
	if other.ID == job.ID {
		t.Fatal("two jobs got the same id")
	}
}

func TestStageDone(t *testing.T) {
	for stage, want := range map[Stage]bool{
		StageQueued:       false,
		StageDownloading:  false,
		StageTranscribing: false,
		StageTranslating:  false,
		StageExporting:    false,
		StageComplete:     true,
		StageFailed:       true,
	} {
		if got := stage.Done(); got != want {
			t.Errorf("%s.Done() = %v, want %v", stage, got, want)
		}
	}
}

// A web address typed without a scheme makes a job whose source has https:// before it, the address yt-dlp is given and the library keeps.
// A job downloads a bilibili video from its canonical link, which yt-dlp's bilibili extractor reads, though the page it came from was under the reader's language.
func TestNewJobDownloadsTheCanonicalLink(t *testing.T) {
	job, err := NewJob("https://www.bilibili.com/zh-Hant/video/BV1nx4y1x7t1/?p=2")
	if err != nil {
		t.Fatal(err)
	}
	if job.Source != "https://www.bilibili.com/video/BV1nx4y1x7t1/?p=2" {
		t.Errorf("source %q", job.Source)
	}
}

func TestNewJobAddsTheScheme(t *testing.T) {
	job, err := NewJob("  vimeo.com/76979871 ")
	if err != nil {
		t.Fatal(err)
	}
	if job.Source != "https://vimeo.com/76979871" {
		t.Errorf("source %q, want https://vimeo.com/76979871", job.Source)
	}
}

func TestNewJobRejectsInvalidSource(t *testing.T) {
	if _, err := NewJob("not-a-url"); err == nil {
		t.Fatal("expected invalid URL to be rejected")
	}
}

func TestNewJobAcceptsLocalFileURL(t *testing.T) {
	job, err := NewJob("file:///Users/test/video.mp4")
	if err != nil {
		t.Fatal(err)
	}
	if job.Source != "file:///Users/test/video.mp4" || job.Stage != StageQueued {
		t.Fatalf("unexpected job: %#v", job)
	}
}

func TestNewJobAcceptsWindowsFileURL(t *testing.T) {
	const source = "file:///C:/Users/a%20b/clip.mp4"
	job, err := NewJob(source)
	if err != nil {
		t.Fatal(err)
	}
	if job.Source != source || job.Stage != StageQueued {
		t.Fatalf("unexpected job: %#v", job)
	}
}

func TestFractionClamps(t *testing.T) {
	for in, want := range map[float64]float64{-0.5: 0, 0: 0, 0.42: 0.42, 1: 1, 3: 1} {
		p := Fraction(in)
		if !p.Known || p.Fraction != want {
			t.Errorf("Fraction(%v) = %+v, want %v", in, p, want)
		}
	}
}
