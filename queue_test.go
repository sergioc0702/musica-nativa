package main

import (
	"context"
	"reflect"
	"testing"
)

// reverseShuffle stands in for rand.Shuffle so the tests know the order.
func reverseShuffle(n int, swap func(i, j int)) {
	for i, j := 0, n-1; i < j; i, j = i+1, j-1 {
		swap(i, j)
	}
}

func testQueue(ids []string, at int) *playQueue {
	queue := newPlayQueue()
	queue.shuffle = reverseShuffle
	queue.set(ids, at)
	return queue
}

func TestShuffleKeepsCurrentSongFirstAndEverySong(t *testing.T) {
	queue := testQueue([]string{"a", "b", "c", "d"}, 1)

	queue.setShuffle(true)

	if want := []string{"b", "d", "c", "a"}; !reflect.DeepEqual(queue.ids, want) || queue.at != 0 {
		t.Fatalf("shuffled = %v at %d; want %v at 0", queue.ids, queue.at, want)
	}
}

func TestShuffleOffRestoresOrderAtCurrentSong(t *testing.T) {
	queue := testQueue([]string{"a", "b", "c", "d"}, 1)
	queue.setShuffle(true)
	queue.at = 2 // now on "c"

	queue.setShuffle(false)

	if want := []string{"a", "b", "c", "d"}; !reflect.DeepEqual(queue.ids, want) || queue.at != 2 {
		t.Fatalf("restored = %v at %d; want %v at 2", queue.ids, queue.at, want)
	}
}

func TestNewQueueIsShuffledWhileShuffleIsOn(t *testing.T) {
	queue := testQueue(nil, 0)
	queue.setShuffle(true)

	queue.set([]string{"a", "b", "c"}, 2)

	if want := []string{"c", "b", "a"}; !reflect.DeepEqual(queue.ids, want) || queue.at != 0 {
		t.Fatalf("queue = %v at %d; want %v at 0", queue.ids, queue.at, want)
	}
}

func TestSongAddedWhileShuffledStaysAfterShuffleOff(t *testing.T) {
	queue := testQueue([]string{"a", "b"}, 0)
	queue.setShuffle(true)
	queue.add("c")

	queue.setShuffle(false)

	if want := []string{"a", "b", "c"}; !reflect.DeepEqual(queue.ids, want) {
		t.Fatalf("queue = %v; want %v", queue.ids, want)
	}
}

func TestRepeatOffStopsAtTheEnds(t *testing.T) {
	queue := testQueue([]string{"a", "b", "c"}, 2)
	if got := queue.order(1, true); len(got) != 0 {
		t.Fatalf("after the last song = %v; want nothing", got)
	}
	queue.at = 0
	if got := queue.order(-1, false); len(got) != 0 {
		t.Fatalf("before the first song = %v; want nothing", got)
	}
	queue.at = 1
	if got, want := queue.order(1, false), []int{2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("next = %v; want %v", got, want)
	}
}

func TestRepeatAllWrapsAround(t *testing.T) {
	queue := testQueue([]string{"a", "b", "c"}, 2)
	queue.mode.Repeat = RepeatAll

	if got, want := queue.order(1, true), []int{0, 1, 2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("after the last song = %v; want %v", got, want)
	}
	queue.at = 0
	if got, want := queue.order(-1, false), []int{2, 1, 0}; !reflect.DeepEqual(got, want) {
		t.Fatalf("before the first song = %v; want %v", got, want)
	}
}

func TestRepeatOneReplaysOnlyWhenTheSongEnds(t *testing.T) {
	queue := testQueue([]string{"a", "b", "c"}, 1)
	queue.mode.Repeat = RepeatOne

	if got, want := queue.order(1, true), []int{1}; !reflect.DeepEqual(got, want) {
		t.Fatalf("song ended = %v; want %v", got, want)
	}
	if got, want := queue.order(1, false), []int{2}; !reflect.DeepEqual(got, want) {
		t.Fatalf("next pressed = %v; want %v", got, want)
	}
}

func TestPlayModeIsRemembered(t *testing.T) {
	store := openTestStore(t)
	service := NewLibraryService(store, newTestPlayer(&fakeOutputs{}))
	service.SetShuffle(true)
	if _, err := service.SetRepeat(RepeatOne); err != nil {
		t.Fatal(err)
	}
	if _, err := service.SetRepeat("sometimes"); err == nil {
		t.Fatal("SetRepeat accepted an unknown mode")
	}

	reopened := NewLibraryService(store, newTestPlayer(&fakeOutputs{}))
	reopened.loadPlayMode(context.Background())

	if got, want := reopened.PlayMode(), (PlayMode{Shuffle: true, Repeat: RepeatOne}); got != want {
		t.Fatalf("play mode = %+v; want %+v", got, want)
	}
}
