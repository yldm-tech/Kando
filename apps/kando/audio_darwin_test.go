//go:build darwin

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"

	"github.com/egoist/mygo/ui"
)

func TestAudioCommandPrefersFFplay(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		skipWithoutFFmpeg(t, "%v", err)
	}
	name, args, err := audioCommand(ffmpeg, "/tmp/video.mp4", 0, 1)
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

func TestAudioCommandSeeksWithThePicture(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		skipWithoutFFmpeg(t, "%v", err)
	}
	name, args, err := audioCommand(ffmpeg, "/tmp/video.mp4", 3*time.Second+250*time.Millisecond, 1)
	if err != nil {
		t.Fatal(err)
	}
	if filepath.Base(name) != "ffplay" {
		t.Skipf("%s cannot seek into a file", name)
	}
	for i := 0; i+1 < len(args); i++ {
		if args[i] == "-ss" {
			if args[i+1] != "3.250" {
				t.Fatalf("seek offset %q, args %v", args[i+1], args)
			}
			return
		}
	}
	t.Fatalf("missing seek offset in args %v", args)
}

// TestAudioEndsWithTheApp: the sound plays in a process of its own, and it must not play on once the app's process has ended, however it ended, playing or paused. The app here is a helper process running this test, killed outright, as a crash or a force quit ends it, without closing anything.
func TestAudioEndsWithTheApp(t *testing.T) {
	if mode := os.Getenv("KANDO_AUDIO_HELPER"); mode != "" {
		audio := &pcmAudio{name: "/bin/sleep", args: []string{"60"}, exited: make(chan struct{})}
		audio.Start()
		if mode == "paused" {
			audio.Pause()
		}
		fmt.Println(audio.cmd.Process.Pid)
		time.Sleep(time.Minute)
		return
	}
	for _, mode := range []string{"playing", "paused"} {
		t.Run(mode, func(t *testing.T) {
			helper := exec.Command(os.Args[0], "-test.run=^TestAudioEndsWithTheApp$")
			helper.Env = append(os.Environ(), "KANDO_AUDIO_HELPER="+mode)
			out, err := helper.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := helper.Start(); err != nil {
				t.Fatal(err)
			}
			var pid int
			if _, err := fmt.Fscan(out, &pid); err != nil {
				_ = helper.Process.Kill()
				t.Fatal(err)
			}
			t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
			if err := syscall.Kill(pid, 0); err != nil {
				t.Fatalf("the sound was not playing: %v", err)
			}
			_ = helper.Process.Kill()
			_ = helper.Wait()
			deadline := time.Now().Add(5 * time.Second)
			for syscall.Kill(pid, 0) == nil {
				if time.Now().After(deadline) {
					t.Fatalf("the sound (pid %d) still plays after the app ended", pid)
				}
				time.Sleep(20 * time.Millisecond)
			}
		})
	}
}

// TestAudioCommandSetsTheVolume: ffplay plays at the volume as a percentage, afplay as a fraction, and a volume out of range is held within 0 and 1.
func TestAudioCommandSetsTheVolume(t *testing.T) {
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		skipWithoutFFmpeg(t, "%v", err)
	}
	for _, tc := range []struct {
		volume         float64
		ffplay, afplay string
	}{{0.4, "40", "0.40"}, {0, "0", "0.00"}, {1.5, "100", "1.00"}, {-1, "0", "0.00"}} {
		name, args, err := audioCommand(ffmpeg, "/tmp/video.mp4", 0, tc.volume)
		if err != nil {
			t.Fatal(err)
		}
		flag, want := "-volume", tc.ffplay
		if filepath.Base(name) == "afplay" {
			flag, want = "-v", tc.afplay
		}
		if got := argAfter(args, flag); got != want {
			t.Fatalf("volume %v: %s %q, want %q; args %v", tc.volume, flag, got, want, args)
		}
	}
}

// TestHeldFilmStartsAtTheNewVolume: the volume changed while the film is held drops the sound it had ready, and play starts it at the new volume.
func TestHeldFilmStartsAtTheNewVolume(t *testing.T) {
	a, tt, updates := openClip(t, 2)
	if a.player.play.audio == nil {
		t.Skip("no sound player for the clip")
	}
	a.SetVolume(0.3)
	waitUntil(t, updates, func() bool { return a.player.play.volume == 0.3 })
	if a.player.play.audio != nil {
		t.Fatal("the held film kept its sound at the old volume")
	}
	tt.Key(0, ui.KeySpace)
	tt.Frame()
	audio := a.player.play.audio
	if audio == nil {
		t.Fatal("play did not start the sound")
	}
	flag, want := "-volume", "30"
	if filepath.Base(audio.name) == "afplay" {
		flag, want = "-v", "0.30"
	}
	if got := argAfter(audio.args, flag); got != want {
		t.Fatalf("sound started at %q, want %q; args %v", got, want, audio.args)
	}
}

func argAfter(args []string, flag string) string {
	for i := 0; i+1 < len(args); i++ {
		if args[i] == flag {
			return args[i+1]
		}
	}
	return ""
}
