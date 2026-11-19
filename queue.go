package main

import "math/rand/v2"

// Repeat modes for PlayMode.Repeat.
const (
	RepeatOff = "off"
	RepeatAll = "all"
	RepeatOne = "one"
)

// PlayMode is how the queue goes on: shuffled or in order, and whether it
// starts again or repeats the song when it gets to the end.
type PlayMode struct {
	Shuffle bool   `json:"shuffle"`
	Repeat  string `json:"repeat"`
}

// playQueue is the order songs play in. ids is that order, shuffled when
// shuffle is on; ordered keeps the order the songs were chosen in, so turning
// shuffle off can put it back. at is the song playing now, an index into ids.
type playQueue struct {
	ids     []string
	ordered []string
	at      int
	mode    PlayMode
	shuffle func(n int, swap func(i, j int))
}

func newPlayQueue() *playQueue {
	return &playQueue{mode: PlayMode{Repeat: RepeatOff}, shuffle: rand.Shuffle}
}

// set replaces the queue, starting at startAt.
func (q *playQueue) set(ids []string, startAt int) {
	if startAt < 0 || startAt >= len(ids) {
		startAt = 0
	}
	q.ordered = append([]string(nil), ids...)
	q.ids = append([]string(nil), ids...)
	q.at = startAt
	if q.mode.Shuffle {
		q.mix()
	}
}

// add puts a song at the end of the queue.
func (q *playQueue) add(id string) {
	q.ids = append(q.ids, id)
	q.ordered = append(q.ordered, id)
}

// index is where id is in the play order, or -1.
func (q *playQueue) index(id string) int { return queueIndex(q.ids, id) }

func (q *playQueue) setShuffle(on bool) {
	if on == q.mode.Shuffle {
		return
	}
	q.mode.Shuffle = on
	if on {
		q.ordered = append([]string(nil), q.ids...)
		q.mix()
		return
	}
	current := q.current()
	q.ids = append([]string(nil), q.ordered...)
	q.at = max(q.index(current), 0)
}

func (q *playQueue) current() string {
	if q.at < 0 || q.at >= len(q.ids) {
		return ""
	}
	return q.ids[q.at]
}

// mix shuffles the songs and puts the one playing now first, so every other
// song is still ahead of it.
func (q *playQueue) mix() {
	if len(q.ids) == 0 {
		return
	}
	current := q.ids[q.at]
	rest := make([]string, 0, len(q.ids)-1)
	rest = append(rest, q.ids[:q.at]...)
	rest = append(rest, q.ids[q.at+1:]...)
	q.shuffle(len(rest), func(i, j int) { rest[i], rest[j] = rest[j], rest[i] })
	q.ids = append([]string{current}, rest...)
	q.at = 0
}

// order lists the queue positions to try, one after another, to move a song
// in direction (1 or -1); the caller skips songs it cannot play. ended means
// the song finished on its own, the only time repeat-one plays it again.
func (q *playQueue) order(direction int, ended bool) []int {
	n := len(q.ids)
	if n == 0 {
		return nil
	}
	if ended && q.mode.Repeat == RepeatOne {
		return []int{q.at}
	}
	positions := make([]int, 0, n)
	for step := 1; step <= n; step++ {
		index := q.at + direction*step
		if q.mode.Repeat == RepeatAll {
			index = ((index % n) + n) % n
		} else if index < 0 || index >= n {
			break
		}
		positions = append(positions, index)
	}
	return positions
}
