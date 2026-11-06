package main

import (
	"path/filepath"
	"testing"
	"time"
)

// TestResumePositionsSurviveARestart checks that where a video stopped is saved with the shelf and read back when the shelf opens again, and that a video watched to the end is forgotten on disk too.
func TestResumePositionsSurviveARestart(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.json")
	a, _, _ := newTestApp(&fakePipeline{})
	a.openLibrary(path)
	a.rememberPosition(&filmPlay{path: "/v.mp4", duration: 10 * time.Second, frames: &frameDecoder{shown: 3 * time.Second}})
	a.rememberPosition(&filmPlay{path: "/w.mp4", duration: 10 * time.Second, frames: &frameDecoder{shown: 7 * time.Second}})

	again, _, _ := newTestApp(&fakePipeline{})
	again.openLibrary(path)
	if at := again.ResumeAt("/v.mp4"); at != 3*time.Second {
		t.Fatalf("ResumeAt(/v.mp4) after a restart = %v", at)
	}
	if at := again.ResumeAt("/w.mp4"); at != 7*time.Second {
		t.Fatalf("ResumeAt(/w.mp4) after a restart = %v", at)
	}

	again.rememberPosition(&filmPlay{path: "/v.mp4", duration: 10 * time.Second, finished: true})
	third, _, _ := newTestApp(&fakePipeline{})
	third.openLibrary(path)
	if at := third.ResumeAt("/v.mp4"); at != 0 {
		t.Fatalf("a finished video resumes at %v", at)
	}
	if at := third.ResumeAt("/w.mp4"); at != 7*time.Second {
		t.Fatalf("ResumeAt(/w.mp4) = %v", at)
	}
}
