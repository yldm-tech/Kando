package player

import (
	"errors"

	"kando/apps/mobile/locales"
	"kando/apps/mobile/look"
	"kando/apps/mobile/session"
	"kando/apps/mobile/sources"

	"github.com/nsgo-dev/nsgo/core"
)

// failedText is what the player says over a film it cannot play: for a film streamed from a source that could not be read, that it could not play from there, or that its file is no longer there (session.StreamProblem); else that the phone cannot play it. Read it in a binding.
func failedText(cx *core.Scope, s *session.Session, key string) string {
	err := s.StreamProblem(key)
	if err == nil {
		return locales.PlayerCantPlay(cx)
	}
	name := streamSourceName(s, key)
	if errors.Is(err, session.ErrStreamFileGone) {
		return locales.PlayerSourceFileGone(cx, name)
	}
	return locales.PlayerSourceFailed(cx, name)
}

// sourceFailed is why a film streamed from a source could not be read from it, as the source's folder page says it (sources.Failure), under what the player says over it (failedText); nothing for any other failure.
func sourceFailed(cx *core.Scope, s *session.Session, key string) core.Node {
	return core.Show{When: func() bool {
		err := s.StreamProblem(key)
		return err != nil && !errors.Is(err, session.ErrStreamFileGone)
	}, Then: func(cx *core.Scope) core.Node {
		return core.Text{TestID: "player-failed-why", ContentFunc: func() string {
			f, _ := s.Film(key)
			src, _ := s.Source(f.Source)
			return sources.Failure(cx, src, s.StreamProblem(key))
		}, Style: look.Subhead.Merge(core.Style{TextAlign: core.TextAlignCenter, Color: faint(0.7)})}
	}}
}

// streamSourceName is the name of the source the film kept under key streams from, as it is named now, else as it was when the film was kept, the name of its library.
func streamSourceName(s *session.Session, key string) string {
	f, _ := s.Film(key)
	if src, ok := s.Source(f.Source); ok {
		return src.Name
	}
	return f.Library
}
