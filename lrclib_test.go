package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"musica-nativa/internal/library"
)

func TestCleanTitleDropsEditionNotes(t *testing.T) {
	cases := map[string]string{
		"Bohemian Rhapsody (Remastered 2011)": "Bohemian Rhapsody",
		"Hey Jude - Remastered 2015":          "Hey Jude",
		"Despacito (feat. Daddy Yankee)":      "Despacito",
		"Corazón [En Vivo]":                   "Corazón",
		"La Flaca - Versión Acústica":         "La Flaca",
		"Song 2":                              "Song 2",
		"(Sittin' On) The Dock of the Bay":    "(Sittin' On) The Dock of the Bay",
	}
	for title, want := range cases {
		if got := cleanTitle(title); got != want {
			t.Errorf("cleanTitle(%q) = %q; want %q", title, got, want)
		}
	}
}

func TestMainArtistKeepsTheFirstName(t *testing.T) {
	cases := map[string]string{
		"Shakira, Bizarrap":         "Shakira",
		"Daft Punk feat. Pharrell":  "Daft Punk",
		"Bad Bunny x Jhay Cortez":   "Bad Bunny",
		"Simon & Garfunkel":         "Simon & Garfunkel",
		"Calle 13 ft. Rubén Blades": "Calle 13",
		"Artista desconocido":       "",
	}
	for artist, want := range cases {
		if got := mainArtist(artist); got != want {
			t.Errorf("mainArtist(%q) = %q; want %q", artist, got, want)
		}
	}
}

func TestNormalizeNameIgnoresAccentsCaseAndSigns(t *testing.T) {
	if got := normalizeName("  Canción  del Ñandú! "); got != "cancion del nandu" {
		t.Fatalf("normalizeName = %q", got)
	}
}

func TestBestMatchPicksTheSameSongPreferringSyncedLyrics(t *testing.T) {
	track := library.Track{Title: "Amanecer (Remastered)", Artist: "Luna Verde", DurationSeconds: 200}
	records := []lrclibRecord{
		{TrackName: "Amanecer", ArtistName: "Otra Banda", Duration: 200, SyncedLyrics: "[00:01.00]otra"},
		{TrackName: "Amanecer", ArtistName: "Luna Verde", Duration: 260, SyncedLyrics: "[00:01.00]larga"},
		{TrackName: "Amanecer", ArtistName: "Luna Verde", Duration: 201, PlainLyrics: "sin tiempos"},
		{TrackName: "AMANECER", ArtistName: "Luna Verde", Duration: 199, SyncedLyrics: "[00:01.00]esta"},
	}

	best, ok := bestMatch(records, track)

	if !ok || best.SyncedLyrics != "[00:01.00]esta" {
		t.Fatalf("best = %+v, %v; want the synced record of the same artist and length", best, ok)
	}
	if _, ok := bestMatch(records[:2], track); ok {
		t.Fatal("matched another artist's song or a much longer one")
	}
}

// fakeLRCLIB answers like lrclib.net: /api/get finds nothing for the exact
// file title, and /api/search knows the song.
type fakeLRCLIB struct {
	mu       sync.Mutex
	requests []string
	agents   []string
	searched []string // track_name of each search
	results  []lrclibRecord
}

func (f *fakeLRCLIB) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	f.mu.Lock()
	f.requests = append(f.requests, r.URL.Path+"?"+r.URL.RawQuery)
	f.agents = append(f.agents, r.UserAgent())
	if r.URL.Path == "/api/search" {
		f.searched = append(f.searched, r.URL.Query().Get("track_name"))
	}
	f.mu.Unlock()
	switch r.URL.Path {
	case "/api/get":
		http.NotFound(w, r)
	case "/api/search":
		_ = json.NewEncoder(w).Encode(f.results)
	default:
		http.Error(w, "?", http.StatusBadRequest)
	}
}

func (f *fakeLRCLIB) count() int {
	f.mu.Lock()
	defer f.mu.Unlock()
	return len(f.requests)
}

func TestLRCLIBFallsBackToSearchWithACleanTitle(t *testing.T) {
	fake := &fakeLRCLIB{results: []lrclibRecord{{TrackName: "Amanecer", ArtistName: "Luna Verde", Duration: 199, SyncedLyrics: "[00:01.00]Hola"}}}
	server := httptest.NewServer(fake)
	defer server.Close()
	track := library.Track{Title: "Amanecer (Remastered 2011)", Artist: "Luna Verde feat. Sol", Album: "Horizonte", DurationSeconds: 200}

	record, ok, err := newLRCLIBClient(server.URL).find(context.Background(), track)

	if err != nil || !ok || record.SyncedLyrics != "[00:01.00]Hola" {
		t.Fatalf("find = %+v, %v, %v; want the synced lyrics from the search", record, ok, err)
	}
	if !slicesContain(fake.searched, "Amanecer") {
		t.Fatalf("requests = %v; want a search for the clean title", fake.requests)
	}
	for _, agent := range fake.agents {
		if !strings.HasPrefix(agent, "Musica Nativa/") {
			t.Fatalf("User-Agent = %q; LRCLIB asks apps to name themselves", agent)
		}
	}
}

func TestLRCLIBSplitsArtistFromFileNameWhenTagsAreMissing(t *testing.T) {
	fake := &fakeLRCLIB{results: []lrclibRecord{{TrackName: "Volver", ArtistName: "Luna Verde", PlainLyrics: "letra"}}}
	server := httptest.NewServer(fake)
	defer server.Close()
	// Without tags, the library uses the file name as the title.
	track := library.Track{Title: "Luna Verde - Volver", Artist: "Artista desconocido"}

	if _, ok, err := newLRCLIBClient(server.URL).find(context.Background(), track); err != nil || !ok {
		t.Fatalf("find = %v, %v; want the song found from its file name", ok, err)
	}
}

func slicesContain(values []string, want string) bool {
	for _, value := range values {
		if value == want {
			return true
		}
	}
	return false
}
