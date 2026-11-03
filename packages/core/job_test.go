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

func TestNewJobRejectsInvalidSource(t *testing.T) {
	if _, err := NewJob("not-a-url"); err == nil {
		t.Fatal("expected invalid URL to be rejected")
	}
}
