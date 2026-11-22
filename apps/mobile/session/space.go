package session

import (
	"github.com/nsgo-dev/nsgo/core"
)

// space is the room on the device, as the system last said it (core.Scope.DiskSpace): read is set once it has, and asked once Settings › Offline Films asked for it, from when it is read again as the films take more or less of the device.
type space struct {
	room  *core.Signal[core.DiskSpace]
	read  *core.Signal[bool]
	asked *core.Signal[bool]
}

func newSpace(cx *core.Scope) *space {
	return &space{room: core.NewSignal(cx, core.DiskSpace{}), read: core.NewSignal(cx, false), asked: core.NewSignal(cx, false)}
}

// followSpace reads the room on the device again as the films kept change what they take, once Settings › Offline Films asked for it (ReadSpace). Apple lets the app read it to show it (nsgo.yaml's apple.privacy, 85F4.1), which only that page does, so it is not read before.
func (s *Session) followSpace() {
	s.cx.Effect(func() {
		_ = s.used.Get()
		if s.space.asked.Get() {
			s.readSpace()
		}
	})
}

// ReadSpace has the system say the room on the device, for Space: Settings › Offline Films asks for it as it opens, and from then on it is read again whenever the films kept take more or less of the device.
func (s *Session) ReadSpace() {
	if s.space.asked.GetUntracked() {
		s.readSpace()
		return
	}
	s.space.asked.Set(true)
}

// readSpace asks the system for the room on the device. Where it cannot say, as on a platform without the service, Space stays false.
func (s *Session) readSpace() {
	s.cx.DiskSpace(func(room core.DiskSpace, err error) {
		if err != nil || room.Total <= 0 {
			return
		}
		s.space.room.Set(room)
		s.space.read.Set(true)
	})
}

// Space is the room on the device as the system last said it, and false until it has. Read it in a binding.
func (s *Session) Space() (core.DiskSpace, bool) {
	return s.space.room.Get(), s.space.read.Get()
}
