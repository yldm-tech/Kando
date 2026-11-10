//go:build unix

package main

import (
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"syscall"
	"testing"
	"time"
)

// TestAudioEndsWithTheApp: the sound is decoded by an ffmpeg of its own, and it must not run on once the app's process has ended, however it ended, playing or paused. The app here is a helper process running this test, killed outright, as a crash or a force quit ends it, without closing anything; ffmpeg is the real one, as what ends it is how ffmpeg takes the pipe breaking. The sound is silence, played at volume 0.
func TestAudioEndsWithTheApp(t *testing.T) {
	if mode := os.Getenv("KANDO_AUDIO_HELPER"); mode != "" {
		sound, err := openPCM(os.Getenv("KANDO_AUDIO_FFMPEG"), os.Getenv("KANDO_AUDIO_FILE"), 0, 0, 1)
		if err != nil {
			fmt.Println(-1)
			return
		}
		if mode == "playing" {
			sound.Start()
		}
		// Long enough for ffmpeg to fill the pipe and wait on it while the sound is held.
		time.Sleep(300 * time.Millisecond)
		fmt.Println(sound.decoder.proc.cmd.Process.Pid)
		time.Sleep(time.Minute)
		return
	}
	ffmpeg, err := exec.LookPath("ffmpeg")
	if err != nil {
		skipWithoutFFmpeg(t, "%v", err)
	}
	silence := filepath.Join(t.TempDir(), "silence.wav")
	if out, err := exec.Command(ffmpeg, "-nostdin", "-loglevel", "error", "-y", "-f", "lavfi", "-i", "anullsrc=r=48000:cl=stereo", "-t", "60", silence).CombinedOutput(); err != nil {
		skipWithoutFFmpeg(t, "ffmpeg could not make a sound: %v %s", err, out)
	}
	for _, mode := range []string{"playing", "paused"} {
		t.Run(mode, func(t *testing.T) {
			helper := exec.Command(os.Args[0], "-test.run=^TestAudioEndsWithTheApp$")
			helper.Env = append(os.Environ(), "KANDO_AUDIO_HELPER="+mode, "KANDO_AUDIO_FFMPEG="+ffmpeg, "KANDO_AUDIO_FILE="+silence)
			out, err := helper.StdoutPipe()
			if err != nil {
				t.Fatal(err)
			}
			if err := helper.Start(); err != nil {
				t.Fatal(err)
			}
			var pid int
			if _, err := fmt.Fscan(out, &pid); err != nil || pid <= 0 {
				_ = helper.Process.Kill()
				_ = helper.Wait()
				t.Fatalf("the helper did not open the sound: pid %d, %v", pid, err)
			}
			t.Cleanup(func() { _ = syscall.Kill(pid, syscall.SIGKILL) })
			if err := syscall.Kill(pid, 0); err != nil {
				t.Fatalf("ffmpeg was not decoding the sound: %v", err)
			}
			_ = helper.Process.Kill()
			_ = helper.Wait()
			deadline := time.Now().Add(5 * time.Second)
			for syscall.Kill(pid, 0) == nil {
				if time.Now().After(deadline) {
					t.Fatalf("ffmpeg (pid %d) still runs after the app ended", pid)
				}
				time.Sleep(20 * time.Millisecond)
			}
		})
	}
}
