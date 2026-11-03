package core

import "testing"

func TestNewJobTrimsSourceAndQueuesIt(t *testing.T) {
	job, err := NewJob(" https://example.com/video ")
	if err != nil {
		t.Fatal(err)
	}
	if job.Source != "https://example.com/video" || job.Stage != StageQueued {
		t.Fatalf("unexpected job: %#v", job)
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
