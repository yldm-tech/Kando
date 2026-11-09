//go:build darwin

package main

import (
	"math"
	"os"
	"os/exec"
	"path/filepath"
	"strconv"
	"sync"
	"syscall"
	"time"
)

// pcmAudio plays the video's audio. YouTube downloads are often Opus, which afplay reports as a zero-length track, so ffplay is used when it sits beside ffmpeg.
type pcmAudio struct {
	name   string
	args   []string
	cmd    *exec.Cmd
	mu     sync.Mutex
	once   sync.Once
	exited chan struct{}
}

// audioGuardScript ends the process $1 once its stdin closes, which happens when the app's process ends, however it ends. It ignores the signals a terminal sends its whole process group, so a Ctrl-C to the app still finds it waiting.
const audioGuardScript = `trap '' INT HUP TERM QUIT; read -r _; kill -CONT "$1" 2>/dev/null; kill -KILL "$1" 2>/dev/null`

// guardAudio starts a small sh that ends the sound's process pid when the app's process ends, even when the app crashes or is killed, since macOS cannot signal a child when its parent dies (Linux's PR_SET_PDEATHSIG). The sh waits on a pipe whose write end only this process holds; the system closes it when the process exits. Ending the returned guard, once pid has exited by itself, keeps it from ever signaling a reused pid.
func guardAudio(pid int) (*exec.Cmd, *os.File, error) {
	r, w, err := os.Pipe()
	if err != nil {
		return nil, nil, err
	}
	defer r.Close()
	guard := exec.Command("/bin/sh", "-c", audioGuardScript, "kando-audio-guard", strconv.Itoa(pid))
	guard.Stdin = r
	if err := guard.Start(); err != nil {
		w.Close()
		return nil, nil, err
	}
	return guard, w, nil
}

// openPCM prepares the sound of path from at, at volume from 0 (silent) to 1 (as recorded).
func openPCM(ffmpeg, path string, at time.Duration, volume float64) (*pcmAudio, error) {
	name, args, err := audioCommand(ffmpeg, path, at, volume)
	if err != nil {
		return nil, err
	}
	return &pcmAudio{name: name, args: args, exited: make(chan struct{})}, nil
}

func audioCommand(ffmpeg, path string, at time.Duration, volume float64) (string, []string, error) {
	if path == "" {
		return "", nil, exec.ErrNotFound
	}
	volume = min(max(volume, 0), 1)
	// ffplay takes the volume as a percentage; it cannot change it while it plays without its window, so a new volume starts the sound again.
	args := []string{"-nodisp", "-autoexit", "-vn", "-loglevel", "error", "-volume", strconv.Itoa(int(math.Round(volume * 100)))}
	if at > 0 {
		args = append(args, "-ss", strconv.FormatFloat(at.Seconds(), 'f', 3, 64))
	}
	args = append(args, path)
	if ffplay := ffplayPath(ffmpeg); ffplay != "" {
		return ffplay, args, nil
	}
	// afplay cannot start partway into a file, so it plays only from the start: no sound is better than sound from the wrong place, after a seek or a new volume.
	if at > 0 {
		return "", nil, exec.ErrNotFound
	}
	if _, err := exec.LookPath("afplay"); err != nil {
		return "", nil, err
	}
	return "afplay", []string{"-v", strconv.FormatFloat(volume, 'f', 2, 64), path}, nil
}

func ffplayPath(ffmpeg string) string {
	if ffmpeg != "" {
		candidate := filepath.Join(filepath.Dir(ffmpeg), "ffplay")
		st, err := os.Stat(candidate)
		if err == nil && !st.IsDir() && st.Mode()&0o111 != 0 {
			return candidate
		}
	}
	if p, err := exec.LookPath("ffplay"); err == nil {
		return p
	}
	return ""
}

func (a *pcmAudio) Start() {
	if a == nil {
		return
	}
	a.mu.Lock()
	defer a.mu.Unlock()
	if a.cmd != nil {
		return
	}
	cmd := exec.Command(a.name, a.args...)
	if err := cmd.Start(); err != nil {
		return
	}
	guard, hold, err := guardAudio(cmd.Process.Pid)
	if err != nil {
		// Without its guard the sound would play on after a crash; better no sound than sound nobody can stop.
		_ = cmd.Process.Kill()
		_ = cmd.Wait()
		return
	}
	a.cmd = cmd
	go func() {
		cmd.Wait()
		_ = guard.Process.Kill()
		_ = guard.Wait()
		hold.Close()
		close(a.exited)
	}()
}

func (a *pcmAudio) Pause() {
	a.signal(syscall.SIGSTOP)
}

func (a *pcmAudio) Play() {
	if a == nil {
		return
	}
	a.mu.Lock()
	started := a.cmd != nil
	a.mu.Unlock()
	if !started {
		a.Start()
		return
	}
	a.signal(syscall.SIGCONT)
}

func (a *pcmAudio) signal(sig syscall.Signal) {
	if a == nil {
		return
	}
	a.mu.Lock()
	cmd := a.cmd
	a.mu.Unlock()
	if cmd == nil || cmd.Process == nil {
		return
	}
	_ = cmd.Process.Signal(sig)
}

func (a *pcmAudio) Close() {
	if a == nil {
		return
	}
	a.once.Do(func() {
		a.mu.Lock()
		cmd := a.cmd
		a.mu.Unlock()
		if cmd == nil || cmd.Process == nil {
			return
		}
		_ = cmd.Process.Signal(syscall.SIGCONT)
		_ = cmd.Process.Kill()
		<-a.exited
	})
}
