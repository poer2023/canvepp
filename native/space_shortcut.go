package main

import "time"

// Counts complete taps, never autorepeat or a held Space used to pan. Other
// keys, modifiers and mouse presses break the sequence. No typed text is kept.
type tripleSpace struct {
	count         int
	first, downAt time.Duration
	down          bool
}

func (s *tripleSpace) feed(kind int, at time.Duration) bool {
	if kind == 0 {
		*s = tripleSpace{}
		return false
	}
	if kind == 1 {
		if s.down {
			*s = tripleSpace{}
			return false
		}
		if s.count > 0 && (at < s.first || at-s.first > 700*time.Millisecond) {
			*s = tripleSpace{}
		}
		if s.count == 0 {
			s.first = at
		}
		s.down = true
		s.downAt = at
		return false
	}
	if !s.down {
		return false
	}
	s.down = false
	if at < s.downAt || at-s.downAt > 220*time.Millisecond || at-s.first > 700*time.Millisecond {
		*s = tripleSpace{}
		return false
	}
	s.count++
	if s.count == 3 {
		*s = tripleSpace{}
		return true
	}
	return false
}
