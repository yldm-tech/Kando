package nowplaying

import "time"

// System is the system's Center. NSGo cannot show what plays or hear the media keys yet, so this one drops what the player tells it and sends no commands: nothing shows in the system's controls, and the media keys do not reach the player.
func System() Center {
	// TODO(nsgo#1084): show info in Now Playing on macOS, the media controls on Windows and MPRIS on Linux, and pass the media keys' and those controls' play, pause and seek on to OnCommand, and their next and previous track, Kando's next and previous film (app.PlayNext, PlayPrevious); NSGo still has no Now Playing or media session service https://github.com/nsgo-dev/nsgo/issues/1084
	return system{}
}

// system is System's Center.
type system struct{}

func (system) Set(Info) {}

func (system) SetPlayback(State, time.Duration) {}

func (system) Clear() {}

func (system) OnCommand(func(cmd Command, position time.Duration)) (off func()) { return func() {} }
