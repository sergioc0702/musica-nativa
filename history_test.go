package main

import (
	"context"
	"reflect"
	"testing"
	"time"

	"musica-nativa/internal/library"
)

func TestASongCountsAsListenedAfterThirtySeconds(t *testing.T) {
	var counter playCounter
	if counter.update("a.mp3", 29000, 200000, true) {
		t.Fatal("counted before 30 s")
	}
	if !counter.update("a.mp3", 30250, 200000, true) {
		t.Fatal("did not count at 30 s")
	}
	if counter.update("a.mp3", 60000, 200000, true) {
		t.Fatal("counted the same listen twice")
	}
}

func TestAShortSongCountsAtHalfway(t *testing.T) {
	var counter playCounter
	if !counter.update("corta.mp3", 20000, 40000, true) {
		t.Fatal("a 40 s song did not count at 20 s")
	}
}

func TestPausedSongsDoNotCount(t *testing.T) {
	var counter playCounter
	if counter.update("a.mp3", 45000, 200000, false) {
		t.Fatal("counted while paused")
	}
}

func TestStartingASongOverCountsAgain(t *testing.T) {
	var counter playCounter
	counter.update("a.mp3", 31000, 200000, true)
	counter.update("a.mp3", 199000, 200000, true)
	counter.update("a.mp3", 250, 200000, true) // repeat-one started it over
	if !counter.update("a.mp3", 30500, 200000, true) {
		t.Fatal("the second listen did not count")
	}
	if !counter.update("b.mp3", 31000, 200000, true) {
		t.Fatal("the next song did not count")
	}
}

func TestListeningHistoryOrdersRecentAndMostPlayed(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	start := time.Unix(1_700_000_000, 0)
	for i, id := range []string{"a", "b", "a", "c", "a", "b"} {
		if err := store.AddPlay(ctx, id, start.Add(time.Duration(i)*time.Minute)); err != nil {
			t.Fatal(err)
		}
	}

	history, err := store.ListeningHistory(ctx)
	if err != nil {
		t.Fatal(err)
	}

	if want := []string{"b", "a", "c"}; !reflect.DeepEqual(history.Recent, want) {
		t.Errorf("recent = %v; want %v (last listen first)", history.Recent, want)
	}
	if want := []TrackPlays{{TrackID: "a", Plays: 3}, {TrackID: "b", Plays: 2}, {TrackID: "c", Plays: 1}}; !reflect.DeepEqual(history.Top, want) {
		t.Errorf("top = %v; want %v", history.Top, want)
	}
	if history.LastPlayed["c"] != start.Add(3*time.Minute).Unix() {
		t.Errorf("last played c = %d", history.LastPlayed["c"])
	}
}

func TestListeningHistoryOffersTheSongToResume(t *testing.T) {
	store := openTestStore(t)
	service := NewLibraryService(store, newTestPlayer(&fakeOutputs{}))
	track := library.Track{ID: "amanecer", Path: "Amanecer.mp3", Title: "Amanecer", Artist: "A", Album: "B", AlbumArtist: "A", Format: "MP3"}
	if err := store.UpsertTracks(context.Background(), []library.Track{track}); err != nil {
		t.Fatal(err)
	}

	service.saveResumePoint("Amanecer.mp3", 83000, 220000)
	history, err := service.ListeningHistory()

	if err != nil || history.Resume == nil || *history.Resume != (ResumePoint{TrackID: "amanecer", PositionMs: 83000}) {
		t.Fatalf("history = %+v, %v; want to resume Amanecer at 1:23", history, err)
	}
	service.saveResumePoint("Amanecer.mp3", 218000, 220000) // a few seconds from the end
	if history, _ := service.ListeningHistory(); history.Resume == nil || history.Resume.PositionMs != 0 {
		t.Fatalf("resume = %+v; want the start of the song when it had nearly ended", history.Resume)
	}
}
