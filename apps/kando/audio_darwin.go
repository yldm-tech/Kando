//go:build darwin

package main

import (
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

func openPCM(ffmpeg, path string, at time.Duration) (*pcmAudio, error) {
	name, args, err := audioCommand(ffmpeg, path, at)
	if err != nil {
		return nil, err
	}
	return &pcmAudio{name: name, args: args, exited: make(chan struct{})}, nil
}

func audioCommand(ffmpeg, path string, at time.Duration) (string, []string, error) {
	if path == "" {
		return "", nil, exec.ErrNotFound
	}
	args := []string{"-nodisp", "-autoexit", "-vn", "-loglevel", "error"}
	if at > 0 {
		args = append(args, "-ss", strconv.FormatFloat(at.Seconds(), 'f', 3, 64))
	}
	args = append(args, path)
	if ffplay := ffplayPath(ffmpeg); ffplay != "" {
		return ffplay, args, nil
	}
	if _, err := exec.LookPath("afplay"); err != nil {
		return "", nil, err
	}
	return "afplay", []string{path}, nil
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
	a.cmd = cmd
	go func() {
		cmd.Wait()
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
