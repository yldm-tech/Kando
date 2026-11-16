package media

import (
	"context"
	"os/exec"
)

// Command is exec.CommandContext for a program Kando runs in the background, as ffmpeg, yt-dlp and whisper-cli are. On Windows it starts with no console window (hideConsole): Kando is a GUI program, so each console program it starts would otherwise open a console of its own, which flashes or stays on screen while the program runs, and closing it kills the program and fails what it was doing. Every such program Kando runs is started through it.
func Command(ctx context.Context, name string, args ...string) *exec.Cmd {
	cmd := exec.CommandContext(ctx, name, args...)
	hideConsole(cmd)
	return cmd
}
