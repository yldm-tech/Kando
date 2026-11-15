// Package nowplaying is what Kando tells the system of the film it plays, where the system shows what plays: Now Playing in Control Center and the menu bar on macOS, the media controls of Windows and the MPRIS clients of Linux, and the commands the media keys and those controls send back. The player speaks only these types. System is where they reach the system under NSGo, which has no Now Playing API yet (system.go).
package nowplaying

import "time"

// Center is where the player shows the open film and hears the media keys and the system's controls. System is the system's; tests connect a recorder in its place.
type Center interface {
	Set(info Info)
	SetPlayback(state State, position time.Duration)
	Clear()
	OnCommand(fn func(cmd Command, position time.Duration)) (off func())
}

// Command is a command of the media keys and the system's controls, or a set of them (Info.Commands). A command the system sends that Kando does not offer, as stop or next track, comes as 0, which is none of these.
type Command uint32

const (
	Play Command = 1 << iota
	Pause
	// Seek moves to the position OnCommand gets: the system's scrubber.
	Seek
)

// State is whether the film shown plays. Playing is the zero value, as the system's.
type State int

const (
	Playing State = iota
	Paused
)

// Info is what the system shows of the film.
type Info struct {
	Title string
	// Artwork is the film's poster, PNG or JPEG, or nil for none.
	Artwork []byte
	// Duration is how long the film is; Position is where it is as the info is set, which the system advances while State is Playing.
	Duration time.Duration
	Position time.Duration
	State    State
	// Commands are the commands the system's controls offer for the film.
	Commands Command
}
