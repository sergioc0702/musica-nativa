package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"musica-nativa/internal/library"
)

func lineTimes(lines []LyricLine) []int64 {
	times := make([]int64, len(lines))
	for i, line := range lines {
		times[i] = line.TimeMs
	}
	return times
}

func lineTexts(lines []LyricLine) []string {
	texts := make([]string, len(lines))
	for i, line := range lines {
		texts[i] = line.Text
	}
	return texts
}

func TestParseLyricsReadsTimedLines(t *testing.T) {
	lines, synced := parseLyrics("[ar:Luna Verde]\n[00:12.50]Amanecer\r\n[00:05.00][01:02.3]Coro\n[00:20]   \n")

	if !synced {
		t.Fatal("LRC with timestamps was read as unsynced")
	}
	if want := []int64{5000, 12500, 20000, 62300}; !reflect.DeepEqual(lineTimes(lines), want) {
		t.Fatalf("times = %v; want %v", lineTimes(lines), want)
	}
	if want := []string{"Coro", "Amanecer", "", "Coro"}; !reflect.DeepEqual(lineTexts(lines), want) {
		t.Fatalf("texts = %q; want %q (a timed empty line marks a pause)", lineTexts(lines), want)
	}
}

func TestParseLyricsAppliesOffset(t *testing.T) {
	// A positive offset shows the lyrics sooner.
	lines, _ := parseLyrics("[offset:+250]\n[00:10.00]Uno\n")

	if len(lines) != 1 || lines[0].TimeMs != 9750 {
		t.Fatalf("lines = %+v; want one line at 9750 ms", lines)
	}
}

func TestParseLyricsReadsWordTimings(t *testing.T) {
	lines, _ := parseLyrics("[00:01.00]<00:01.00>Hola <00:01.80>mundo <00:02.40>azul\n")

	if len(lines) != 1 || lines[0].Text != "Hola mundo azul" {
		t.Fatalf("lines = %+v; want the words joined into one line", lines)
	}
	want := []LyricWord{{TimeMs: 1000, Text: "Hola "}, {TimeMs: 1800, Text: "mundo "}, {TimeMs: 2400, Text: "azul"}}
	if !reflect.DeepEqual(lines[0].Words, want) {
		t.Fatalf("words = %+v; want %+v", lines[0].Words, want)
	}
}

func TestParseLyricsKeepsPlainTextAsIs(t *testing.T) {
	lines, synced := parseLyrics("\n\nPrimera línea\nSegunda\n\nOtra estrofa\n\n")

	if synced {
		t.Fatal("plain lyrics were read as synced")
	}
	if want := []string{"Primera línea", "Segunda", "", "Otra estrofa"}; !reflect.DeepEqual(lineTexts(lines), want) {
		t.Fatalf("texts = %q; want %q", lineTexts(lines), want)
	}
}

// lyricsTestService has one song, "Amanecer" by Luna Verde, and looks lyrics
// up on fake instead of lrclib.net.
func lyricsTestService(t *testing.T, fake *fakeLRCLIB) (*LibraryService, library.Track) {
	t.Helper()
	store := openTestStore(t)
	service := NewLibraryService(store, newTestPlayer(&fakeOutputs{}))
	server := httptest.NewServer(fake)
	t.Cleanup(server.Close)
	service.lrclib = newLRCLIBClient(server.URL)
	path := filepath.Join(t.TempDir(), "Amanecer.mp3")
	writeTestTaggedMP3(t, path, "Amanecer", nil)
	track := library.Track{ID: "amanecer", Path: path, Title: "Amanecer", Artist: "Luna Verde", Album: "Horizonte",
		AlbumArtist: "Luna Verde", Format: "MP3", DurationSeconds: 200}
	if err := store.UpsertTracks(context.Background(), []library.Track{track}); err != nil {
		t.Fatal(err)
	}
	return service, track
}

func onlineSong(lyrics string) *fakeLRCLIB {
	return &fakeLRCLIB{results: []lrclibRecord{{TrackName: "Amanecer", ArtistName: "Luna Verde", Duration: 200, SyncedLyrics: lyrics}}}
}

func writeSidecar(t *testing.T, track library.Track, text string) {
	t.Helper()
	if err := os.WriteFile(strings.TrimSuffix(track.Path, ".mp3")+".lrc", []byte(text), 0o600); err != nil {
		t.Fatal(err)
	}
}

func mustLyrics(t *testing.T, service *LibraryService, id string) Lyrics {
	t.Helper()
	lyrics, err := service.Lyrics(id)
	if err != nil {
		t.Fatal(err)
	}
	return lyrics
}

func TestLyricsPreferASyncedLRCFileNextToTheSong(t *testing.T) {
	fake := onlineSong("[00:01.00]De internet")
	service, track := lyricsTestService(t, fake)
	writeSidecar(t, track, "[00:02.00]Del archivo")

	lyrics := mustLyrics(t, service, track.ID)

	if lyrics.Source != "lrc" || !lyrics.Synced || lyrics.Lines[0].Text != "Del archivo" {
		t.Fatalf("lyrics = %+v; want the synced .lrc next to the song", lyrics)
	}
	if fake.count() != 0 {
		t.Fatalf("asked LRCLIB %d times although the song has its own lyrics", fake.count())
	}
}

func TestLyricsFromTheInternetAreSavedForNextTime(t *testing.T) {
	fake := onlineSong("[00:01.00]Hola")
	service, track := lyricsTestService(t, fake)

	first := mustLyrics(t, service, track.ID)
	asked := fake.count()
	second := mustLyrics(t, service, track.ID)

	if first.Source != "lrclib" || !first.Synced || !reflect.DeepEqual(first, second) {
		t.Fatalf("first = %+v, second = %+v; want the same synced lyrics from LRCLIB", first, second)
	}
	if fake.count() != asked {
		t.Fatalf("asked LRCLIB again for lyrics it already sent")
	}
}

func TestLyricsKeepLookingWhenTheFileHasNoTimes(t *testing.T) {
	service, track := lyricsTestService(t, onlineSong("[00:01.00]Con tiempos"))
	writeSidecar(t, track, "Sin tiempos")
	if lyrics := mustLyrics(t, service, track.ID); lyrics.Source != "lrclib" || !lyrics.Synced {
		t.Fatalf("lyrics = %+v; want LRCLIB's synced lyrics over the plain file", lyrics)
	}

	service, track = lyricsTestService(t, &fakeLRCLIB{})
	writeSidecar(t, track, "Sin tiempos")
	if lyrics := mustLyrics(t, service, track.ID); lyrics.Source != "lrc" || lyrics.Synced || !lyrics.Found {
		t.Fatalf("lyrics = %+v; want the plain file when nothing better exists", lyrics)
	}
}

func TestMissingLyricsAreNotAskedForAgainUntilSearchingAgain(t *testing.T) {
	fake := &fakeLRCLIB{}
	service, track := lyricsTestService(t, fake)

	if lyrics := mustLyrics(t, service, track.ID); lyrics.Found || lyrics.Message == "" {
		t.Fatalf("lyrics = %+v; want not found, with a message", lyrics)
	}
	asked := fake.count()
	mustLyrics(t, service, track.ID)
	if fake.count() != asked {
		t.Fatal("asked LRCLIB again right after it had nothing")
	}

	fake.mu.Lock()
	fake.results = onlineSong("[00:01.00]Ya está").results
	fake.mu.Unlock()
	lyrics, err := service.SearchLyricsAgain(track.ID)
	if err != nil || !lyrics.Found || lyrics.Source != "lrclib" {
		t.Fatalf("search again = %+v, %v; want the lyrics LRCLIB has now", lyrics, err)
	}
}

func TestOwnLyricsWinAndCanBeRemoved(t *testing.T) {
	service, track := lyricsTestService(t, onlineSong("[00:01.00]De internet"))

	saved, err := service.SaveLyrics(track.ID, "[00:03.00]Mía")
	if err != nil || saved.Source != "manual" || !saved.Synced || saved.Text != "[00:03.00]Mía" {
		t.Fatalf("saved = %+v, %v; want my synced lyrics", saved, err)
	}
	if lyrics := mustLyrics(t, service, track.ID); lyrics.Source != "manual" {
		t.Fatalf("lyrics = %+v; want mine", lyrics)
	}
	removed, err := service.SaveLyrics(track.ID, "  ")
	if err != nil || removed.Source != "lrclib" {
		t.Fatalf("after removing mine = %+v, %v; want LRCLIB's again", removed, err)
	}
}

func TestLyricsStayOfflineWhenInternetSearchIsOff(t *testing.T) {
	fake := onlineSong("[00:01.00]Hola")
	service, track := lyricsTestService(t, fake)

	service.SetLyricsOnline(false)

	if service.LyricsOnline() {
		t.Fatal("internet search still on")
	}
	if lyrics := mustLyrics(t, service, track.ID); lyrics.Found || !strings.Contains(lyrics.Message, "Ajustes") {
		t.Fatalf("lyrics = %+v; want not found, pointing to Ajustes", lyrics)
	}
	if fake.count() != 0 {
		t.Fatalf("asked LRCLIB %d times with internet search off", fake.count())
	}
}

func TestLyricsNetworkErrorIsNotRemembered(t *testing.T) {
	service, track := lyricsTestService(t, onlineSong("[00:01.00]Hola"))
	working := service.lrclib
	broken := httptest.NewServer(http.NotFoundHandler())
	broken.Close() // nothing listens there any more
	service.lrclib = newLRCLIBClient(broken.URL)

	if lyrics := mustLyrics(t, service, track.ID); lyrics.Found || !strings.Contains(lyrics.Message, "conexión") {
		t.Fatalf("lyrics = %+v; want a connection message", lyrics)
	}
	service.lrclib = working
	if lyrics := mustLyrics(t, service, track.ID); !lyrics.Found {
		t.Fatalf("lyrics = %+v; want them once the connection works", lyrics)
	}
}
