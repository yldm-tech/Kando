//go:build darwin

package main

import (
	"os/exec"
	"sync"
	"syscall"
)

// pcmAudio plays the video file's own audio with afplay. Pause stops the process and resume continues it, so the sound stays with the picture.
type pcmAudio struct {
	path   string
	cmd    *exec.Cmd
	mu     sync.Mutex
	once   sync.Once
	exited chan struct{}
}

func openPCM(_, path string) (*pcmAudio, error) {
	if path == "" {
		return nil, exec.ErrNotFound
	}
	if _, err := exec.LookPath("afplay"); err != nil {
		return nil, err
	}
	return &pcmAudio{path: path, exited: make(chan struct{})}, nil
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
	cmd := exec.Command("afplay", a.path)
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
