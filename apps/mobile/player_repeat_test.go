package main

import (
	"slices"
	"testing"
	"time"
)

// repeatOn turns 循环播放 on in the playback settings.
func (p *phone) repeatOn() {
	p.t.Helper()
	p.app.Press("id:player-more")
	p.app.Press("id:player-settings-repeat")
	p.app.Back()
	p.app.WaitFor("id:player-pill-repeat", wait)
}

// Repeat plays the film again from its start once it ends, playing on, rather than going on to the next film; its end is not kept as where the film was left.
func TestRepeatPlaysTheFilmAgainInPlaceOfTheNext(t *testing.T) {
	p, _, _ := openNextTalk(t)
	p.repeatOn()
	p.at(9 * sec)
	before := len(p.seeks())
	p.at(10 * sec)
	if s := p.seeks(); len(s) != before+1 || s[len(s)-1] != 0 {
		t.Fatalf("at its end the film went to %v, want its start", s)
	}
	if p.app.Find("id:player-video").Video.Paused {
		t.Fatal("the film held at its end")
	}
	p.app.Advance(10 * sec)
	if p.app.Exists("id:player-up-next") || !p.playing("f1") {
		t.Fatal("repeat went on to the next film")
	}
	p.at(500 * time.Millisecond)
	if got := p.texts("id:player-at"); got != "0:00" {
		t.Fatalf("played again the player is at %q", got)
	}
}

// A line looped plays again at the film's end before repeat does, and the sleep timer's End of Film holds the film there though repeat is on.
func TestALoopAndEndOfFilmComeBeforeRepeat(t *testing.T) {
	p, _ := openPlayer(t)
	p.repeatOn()
	p.at(7200 * time.Millisecond)
	p.app.Press("id:player-card-loop")
	p.at(10 * sec)
	if s := p.seeks(); s[len(s)-1] != 7*sec {
		t.Fatalf("with the last line looped the end went to %v, want 7s", s)
	}
	p.app.Press("id:player-card-loop")

	p.app.Press("id:player-more")
	p.app.Press("id:player-settings-sleep-film")
	p.app.Back()
	before := slices.Clone(p.seeks())
	p.at(10 * sec)
	if !p.app.Find("id:player-video").Video.Paused || len(p.seeks()) != len(before) {
		t.Fatalf("End of Film with repeat on: paused %v, seeks %v", p.app.Find("id:player-video").Video.Paused, p.seeks())
	}
	if p.app.Exists("id:player-pill-sleep") {
		t.Fatal("End of Film stayed on after it held the film")
	}
	// The timer spent, repeat plays the film again at its next end.
	p.app.Press("id:player-play")
	p.at(10 * sec)
	if s := p.seeks(); s[len(s)-1] != 0 || p.app.Find("id:player-video").Video.Paused {
		t.Fatalf("after End of Film repeat went to %v", s)
	}
}
