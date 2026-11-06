//go:build darwin

package main

import (
	"os/exec"
	"path/filepath"
	"testing"
)

func TestAudioCommandPrefersFFplay(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		skipWithoutFFmpeg(t, "%v", err)
	}
	name, args, err := audioCommand(ffmpeg, "/tmp/video.mp4", 0)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(name) != "ffplay" {
		t.Fatalf("player %s", name)
	}
	if len(args) == 0 || args[len(args)-1] != "/tmp/video.mp4" {
		t.Fatalf("args %v", args)
	}
	got := map[string]bool{}
	for _, arg := range args {
		got[arg] = true
	}
	if !got["-nodisp"] || !got["-vn"] {
		t.Fatalf("args %v", args)
	}
}
