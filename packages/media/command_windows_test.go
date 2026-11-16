//go:build windows

package media

import (
	"context"
	"testing"
)

// TestCommandHidesConsole checks that a program Kando starts on Windows, a GUI program, gets no console window of its own.
func TestCommandHidesConsole(t *testing.T) {
	cmd := Command(context.Background(), "ffmpeg", "-version")
	if cmd.SysProcAttr == nil || !cmd.SysProcAttr.HideWindow || cmd.SysProcAttr.CreationFlags&createNoWindow == 0 {
		t.Fatalf("SysProcAttr = %+v, want HideWindow and CREATE_NO_WINDOW", cmd.SysProcAttr)
	}
}
