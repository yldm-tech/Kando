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
