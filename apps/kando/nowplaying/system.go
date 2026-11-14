package nowplaying

import (
	"time"

	"github.com/egoist/mygo"
)

// System is the system's Center, mygo.NowPlaying in Kando's types.
func System() Center { return system{} }

// system passes each call on to mygo.NowPlaying, turning Kando's types into MyGo's and the commands back.
type system struct{}

func (system) Set(info Info) { mygo.NowPlaying.Set(mygoInfo(info)) }

func (system) SetPlayback(state State, position time.Duration) {
	mygo.NowPlaying.SetPlayback(mygoState(state), position)
}

func (system) Clear() { mygo.NowPlaying.Clear() }

func (system) OnCommand(fn func(cmd Command, position time.Duration)) (off func()) {
	return mygo.NowPlaying.OnCommand(func(cmd mygo.MediaCommand, position time.Duration) {
		fn(command(cmd), position)
	})
}

// mygoInfo is info as mygo.NowPlaying takes it.
func mygoInfo(info Info) mygo.NowPlayingInfo {
	return mygo.NowPlayingInfo{
		Title:    info.Title,
		Artwork:  info.Artwork,
		Duration: info.Duration,
		Position: info.Position,
		State:    mygoState(info.State),
		Commands: mygoCommands(info.Commands),
	}
}

// mygoState is state as mygo.NowPlaying takes it.
func mygoState(state State) mygo.PlaybackState {
	if state == Paused {
		return mygo.PlaybackPaused
	}
	return mygo.PlaybackPlaying
}

// commandPairs pairs each Command with MyGo's.
var commandPairs = [...]struct {
	kando Command
	mygo  mygo.MediaCommand
}{
	{Play, mygo.MediaPlay},
	{Pause, mygo.MediaPause},
	{Seek, mygo.MediaSeek},
}

// mygoCommands is the set cmds as mygo.NowPlaying takes it.
func mygoCommands(cmds Command) mygo.MediaCommand {
	var out mygo.MediaCommand
	for _, p := range commandPairs {
		if cmds&p.kando != 0 {
			out |= p.mygo
		}
	}
	return out
}

// command is the command mygo.NowPlaying sent, in Kando's types: 0 for one Kando does not offer.
func command(cmd mygo.MediaCommand) Command {
	var out Command
	for _, p := range commandPairs {
		if cmd&p.mygo != 0 {
			out |= p.kando
		}
	}
	return out
}
