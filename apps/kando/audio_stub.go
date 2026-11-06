//go:build !darwin

package main

import "time"

// pcmAudio is the sound of a film. This build has no afplay, so the picture plays alone.
type pcmAudio struct{}

func openPCM(string, string, time.Duration) (*pcmAudio, error) { return nil, nil }

func (a *pcmAudio) Start() {}

func (a *pcmAudio) Pause() {}

func (a *pcmAudio) Play() {}

func (a *pcmAudio) Close() {}
