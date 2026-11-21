package track

// Heard is how far generation gen is heard, cell by cell: the cells the speech recognition heard, in any language, silent ones among them, and those it gave up on, tried Attempts times without being heard. Every other cell of a film being made is still to be heard, as the part of a film made from where it was opened that comes before it.
func (d Doc) Heard(gen string) (heard, givenUp map[int]bool) {
	heard, givenUp = map[int]bool{}, map[int]bool{}
	tried := map[Unit]int{}
	for _, e := range d.Events {
		if e.Gen != gen || Work(e.Source) != WorkHear {
			continue
		}
		switch e.Kind {
		case KindUnit:
			heard[unitOf(e).Cell] = true
		case KindAttempt:
			tried[unitOf(e)]++
		}
	}
	for u, n := range tried {
		if n >= Attempts && !heard[u.Cell] {
			givenUp[u.Cell] = true
		}
	}
	return heard, givenUp
}
