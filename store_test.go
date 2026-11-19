package main

import (
	"bytes"
	"context"
	"crypto/sha256"
	"database/sql"
	"encoding/base64"
	"encoding/hex"
	"errors"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"sort"
	"strconv"
	"strings"
	"testing"

	"musica-nativa/internal/library"
)

func openTestStore(t *testing.T) *Store {
	t.Helper()
	store, err := OpenStore(filepath.Join(t.TempDir(), "library.db"))
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { store.Close() })
	return store
}

func TestStorePersistsTracksFavoritesPlaylistsAndThemes(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	if err := store.ReplaceFolders(ctx, []string{`C:\Music`}); err != nil {
		t.Fatal(err)
	}
	tracks := []library.Track{{ID: "track-a", Path: `C:\Music\a.flac`, Title: "Luz", Artist: "Norte", Album: "Azul", Available: true}}
	if err := store.UpsertTracks(ctx, tracks); err != nil {
		t.Fatal(err)
	}
	if err := store.ToggleFavorite(ctx, "track-a"); err != nil {
		t.Fatal(err)
	}
	playlist, err := store.CreatePlaylist(ctx, "Viaje")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddTrackToPlaylist(ctx, playlist.ID, "track-a"); err != nil {
		t.Fatal(err)
	}
	custom := Theme{ID: "custom-night", Name: "Noche propia", Background: "#111318", Accent: "#d39b72", Surface: "#1c1d22", Text: "#f8f5ef", Muted: "#a8a49e"}
	if _, err := store.SaveTheme(ctx, custom); err != nil {
		t.Fatal(err)
	}
	if err := store.SetPreference(ctx, "activeTheme", custom.ID); err != nil {
		t.Fatal(err)
	}

	gotTracks, err := store.Tracks(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(gotTracks) != 1 || !gotTracks[0].Favorite {
		t.Fatalf("favorite did not persist: %+v", gotTracks)
	}
	gotPlaylistTracks, err := store.PlaylistTracks(ctx, playlist.ID)
	if err != nil {
		t.Fatal(err)
	}
	if len(gotPlaylistTracks) != 1 || gotPlaylistTracks[0].ID != "track-a" {
		t.Fatalf("playlist did not persist: %+v", gotPlaylistTracks)
	}
	themes, err := store.Themes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if len(themes) != 4 || themes[3].ID != custom.ID || themes[3].Accent != custom.Accent {
		t.Fatalf("theme did not persist: %+v", themes)
	}
	active, err := store.Preference(ctx, "activeTheme", "")
	if err != nil || active != custom.ID {
		t.Fatalf("active theme = %q, %v; want %q", active, err, custom.ID)
	}
}

func TestOpenStoreRefreshesBuiltInThemeColors(t *testing.T) {
	path := filepath.Join(t.TempDir(), "library.db")
	store, err := OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	// Colors saved by an earlier version of the app.
	if _, err := store.db.Exec(`UPDATE themes SET accent='#e7a878', background='#11110f' WHERE id='nocturno'`); err != nil {
		t.Fatal(err)
	}
	store.Close()

	store, err = OpenStore(path)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	themes, err := store.Themes(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	for _, theme := range themes {
		if theme.ID == "nocturno" && (theme.Accent != "#ff7a59" || theme.Background != "#121214") {
			t.Fatalf("nocturno kept old colors: %+v", theme)
		}
	}
}

func TestStoreSearchesTracksAndMarksMissingRoots(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	if err := store.ReplaceFolders(ctx, []string{`C:\collection`, `C:\other`}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertTracks(ctx, []library.Track{
		{ID: "one", Path: `C:\collection\one.mp3`, Title: "Luz roja", Artist: "Norte", Available: true},
		{ID: "two", Path: `C:\other\two.wav`, Title: "Azul", Artist: "Sur", Available: true},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.MarkMissingUnderRoots(ctx, []string{`C:\collection`}, map[string]struct{}{}); err != nil {
		t.Fatal(err)
	}
	tracks, err := store.Tracks(ctx, "nOrTe")
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 1 || tracks[0].Available {
		t.Fatalf("search/availability result = %+v", tracks)
	}
}

func TestStoreListsOnlyTracksInsideMusicFolders(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	if err := store.UpsertTracks(ctx, []library.Track{
		{ID: "root", Path: `C:\Music\root.mp3`, Title: "Raíz", Available: true},
		{ID: "nested", Path: `C:\Music\Rock\nested.mp3`, Title: "Anidada", Available: true},
		{ID: "sibling", Path: `C:\MusicExtra\sibling.mp3`, Title: "Vecina", Available: true},
		{ID: "elsewhere", Path: `D:\Otra\elsewhere.mp3`, Title: "Lejos", Available: true},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceFolders(ctx, []string{`c:\music`}); err != nil {
		t.Fatal(err)
	}
	tracks, err := store.Tracks(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := trackIDs(tracks); got != "nested,root" {
		t.Fatalf("library tracks = %s; want nested,root", got)
	}
}

func TestStoreRemovesTracksOutsideFoldersButKeepsFavoritesAndPlaylistEntries(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	if err := store.UpsertTracks(ctx, []library.Track{
		{ID: "kept", Path: `C:\Music\kept.mp3`, Title: "Sigue", Available: true},
		{ID: "plain", Path: `D:\Removed\plain.mp3`, Title: "Simple", Available: true},
		{ID: "loved", Path: `D:\Removed\loved.mp3`, Title: "Querida", Available: true},
		{ID: "listed", Path: `D:\Removed\listed.mp3`, Title: "En lista", Available: true},
	}); err != nil {
		t.Fatal(err)
	}
	if err := store.ToggleFavorite(ctx, "loved"); err != nil {
		t.Fatal(err)
	}
	playlist, err := store.CreatePlaylist(ctx, "Viaje")
	if err != nil {
		t.Fatal(err)
	}
	if err := store.AddTrackToPlaylist(ctx, playlist.ID, "listed"); err != nil {
		t.Fatal(err)
	}
	if err := store.ReplaceFolders(ctx, []string{`C:\Music`}); err != nil {
		t.Fatal(err)
	}

	if err := store.RemoveTracksOutsideFolders(ctx); err != nil {
		t.Fatal(err)
	}
	if _, err := store.Track(ctx, "plain"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("track outside every folder still stored, err = %v", err)
	}
	inPlaylist, err := store.PlaylistTracks(ctx, playlist.ID)
	if err != nil || len(inPlaylist) != 1 || inPlaylist[0].ID != "listed" {
		t.Fatalf("playlist lost its entry: %+v, %v", inPlaylist, err)
	}

	// Adding the folder back restores the favorite that was kept aside.
	if err := store.ReplaceFolders(ctx, []string{`C:\Music`, `D:\Removed`}); err != nil {
		t.Fatal(err)
	}
	tracks, err := store.Tracks(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := trackIDs(tracks); got != "kept,listed,loved" {
		t.Fatalf("library tracks = %s; want kept,listed,loved", got)
	}
	for _, track := range tracks {
		if track.ID == "loved" && !track.Favorite {
			t.Fatal("favorite flag was lost while the folder was removed")
		}
	}
}

func TestStoreFindsTrackByIDOutsideMusicFolders(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	if err := store.UpsertTracks(ctx, []library.Track{{ID: "solo", Path: `D:\Suelta\solo.mp3`, Title: "Solo", Available: true}}); err != nil {
		t.Fatal(err)
	}
	track, err := store.Track(ctx, "solo")
	if err != nil || track.Title != "Solo" || track.Path != `D:\Suelta\solo.mp3` {
		t.Fatalf("Track(solo) = %+v, %v", track, err)
	}
	if _, err := store.Track(ctx, "nadie"); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("unknown track error = %v, want sql.ErrNoRows", err)
	}
}

func TestStoreKeepsCoverArtOutsideTrackRows(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	used, _ := library.NewArtwork(testPNG("portada usada"))
	unused, _ := library.NewArtwork(testPNG("portada sin uso"))
	for _, art := range []library.Artwork{used, unused} {
		if err := store.SaveArtwork(ctx, art); err != nil {
			t.Fatal(err)
		}
	}
	if err := store.ReplaceFolders(ctx, []string{`C:\Music`}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertTracks(ctx, []library.Track{{ID: "a", Path: `C:\Music\a.mp3`, Title: "A", CoverID: used.ID, Available: true}}); err != nil {
		t.Fatal(err)
	}

	tracks, err := store.Tracks(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 1 || tracks[0].CoverURL != "/artwork/"+used.ID {
		t.Fatalf("track cover URL = %+v", tracks)
	}

	if err := store.RemoveUnusedArtwork(ctx); err != nil {
		t.Fatal(err)
	}
	got, err := store.Artwork(ctx, used.ID)
	if err != nil || got.MIME != "image/png" || !bytes.Equal(got.Data, testPNG("portada usada")) {
		t.Fatalf("used artwork = %+v, %v", got, err)
	}
	if _, err := store.Artwork(ctx, unused.ID); !errors.Is(err, sql.ErrNoRows) {
		t.Fatalf("unused artwork still stored, err = %v", err)
	}
}

func TestArtworkRouteServesStoredPictures(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	art, _ := library.NewArtwork(testPNG("ruta"))
	if err := store.SaveArtwork(ctx, art); err != nil {
		t.Fatal(err)
	}
	fallback := http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) { w.WriteHeader(http.StatusTeapot) })
	handler := artworkMiddleware(store)(fallback)

	response := httptest.NewRecorder()
	handler.ServeHTTP(response, httptest.NewRequest(http.MethodGet, "/artwork/"+art.ID, nil))
	if response.Code != http.StatusOK || response.Header().Get("Content-Type") != "image/png" || !bytes.Equal(response.Body.Bytes(), testPNG("ruta")) {
		t.Fatalf("artwork response = %d %q %q", response.Code, response.Header().Get("Content-Type"), response.Body.Bytes())
	}

	missing := httptest.NewRecorder()
	handler.ServeHTTP(missing, httptest.NewRequest(http.MethodGet, "/artwork/0000", nil))
	if missing.Code != http.StatusNotFound {
		t.Fatalf("unknown artwork status = %d, want 404", missing.Code)
	}

	other := httptest.NewRecorder()
	handler.ServeHTTP(other, httptest.NewRequest(http.MethodGet, "/index.html", nil))
	if other.Code != http.StatusTeapot {
		t.Fatalf("other paths should reach the app assets, got %d", other.Code)
	}
}

func TestThemeBackgroundIsServedAsArtworkAndKeptOnResave(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	picture := testPNG("fondo")
	theme := Theme{Name: "Bosque", Background: "#101611", Accent: "#9fc8a8", Surface: "#1a211b", Text: "#eef3ec", Muted: "#919d91",
		BackgroundImage: "data:image/png;base64," + base64.StdEncoding.EncodeToString(picture)}

	saved, err := store.SaveTheme(ctx, theme)
	if err != nil {
		t.Fatal(err)
	}
	if saved.ID == "" || !strings.HasPrefix(saved.BackgroundImage, "/artwork/") {
		t.Fatalf("saved theme = %+v; want an id and an artwork URL", saved)
	}
	art, err := store.Artwork(ctx, strings.TrimPrefix(saved.BackgroundImage, "/artwork/"))
	if err != nil || !bytes.Equal(art.Data, picture) {
		t.Fatalf("background artwork = %+v, %v", art, err)
	}

	// Saving the edited theme again sends back the URL it received.
	saved.Accent = "#ffffff"
	resaved, err := store.SaveTheme(ctx, saved)
	if err != nil {
		t.Fatal(err)
	}
	if resaved.ID != saved.ID || resaved.BackgroundImage != saved.BackgroundImage {
		t.Fatalf("resaved theme = %+v; want same id and background as %+v", resaved, saved)
	}

	// A second theme with the same name is a new theme, not an overwrite.
	twin, err := store.SaveTheme(ctx, Theme{Name: "Bosque", Background: "#000000", Accent: "#111111", Surface: "#222222", Text: "#ffffff", Muted: "#999999"})
	if err != nil {
		t.Fatal(err)
	}
	themes, err := store.Themes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	if twin.ID == saved.ID || len(themes) != 5 {
		t.Fatalf("same-name theme replaced the first one: twin %q, first %q, %d themes", twin.ID, saved.ID, len(themes))
	}

	if _, err := store.SaveTheme(ctx, Theme{Name: "Roto", Background: "#000000", BackgroundImage: "data:text/html;base64," + base64.StdEncoding.EncodeToString([]byte("<script>"))}); err == nil {
		t.Fatal("expected a non-image background to be rejected")
	}
}

func TestOpenStoreUpgradesImagesSavedByEarlierVersions(t *testing.T) {
	dir := t.TempDir()
	songPath := filepath.Join(dir, "music", "song.mp3")
	cover := testPNG("portada antigua")
	writeTestTaggedMP3(t, songPath, "Antigua", cover)
	info, err := os.Stat(songPath)
	if err != nil {
		t.Fatal(err)
	}
	background := testPNG("fondo antiguo")
	// Version 1.0.0 identified tracks by the SHA-256 of the lower-case path.
	legacyID := sha256.Sum256([]byte(strings.ToLower(songPath)))
	dbPath := filepath.Join(dir, "library.db")
	createLegacyDatabase(t, dbPath,
		`INSERT INTO folders(path) VALUES ('`+filepath.Dir(songPath)+`')`,
		`INSERT INTO tracks(id,path,title,artist,album,album_artist,format,size_bytes,modified_unix,cover_data_url,available)
			VALUES ('`+hex.EncodeToString(legacyID[:])+`','`+songPath+`','Antigua','A','B','A','MP3',`+itoa(info.Size())+`,`+itoa(info.ModTime().Unix())+`,
			'data:image/png;base64,`+base64.StdEncoding.EncodeToString(cover)+`',1)`,
		`INSERT INTO themes(id,name,background,accent,surface,text_color,muted,background_image,built_in)
			VALUES ('mine','Mío','#000000','#111111','#222222','#ffffff','#999999','data:image/png;base64,`+base64.StdEncoding.EncodeToString(background)+`',0)`,
	)

	store, err := OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	ctx := context.Background()

	var leftover int
	if err := store.db.QueryRowContext(ctx, `SELECT COALESCE(SUM(length(cover_data_url)),0) + COALESCE((SELECT SUM(length(background_image)) FROM themes),0) FROM tracks`).Scan(&leftover); err != nil {
		t.Fatal(err)
	}
	if leftover != 0 {
		t.Fatalf("%d bytes of embedded images are still stored inline", leftover)
	}

	themes, err := store.Themes(ctx)
	if err != nil {
		t.Fatal(err)
	}
	var mine Theme
	for _, theme := range themes {
		if theme.ID == "mine" {
			mine = theme
		}
	}
	art, err := store.Artwork(ctx, strings.TrimPrefix(mine.BackgroundImage, "/artwork/"))
	if err != nil || !bytes.Equal(art.Data, background) {
		t.Fatalf("theme background after upgrade = %q, %+v, %v", mine.BackgroundImage, art, err)
	}

	// The next library scan reads the cover again from the music file.
	known, err := store.KnownTracks(ctx)
	if err != nil {
		t.Fatal(err)
	}
	scanned, issues := library.ScanRoots(ctx, []string{filepath.Dir(songPath)}, library.ScanOptions{
		Known:     known,
		OnArtwork: func(art library.Artwork) error { return store.SaveArtwork(ctx, art) },
	})
	if len(issues) != 0 {
		t.Fatalf("scan issues: %v", issues)
	}
	if err := store.UpsertTracks(ctx, scanned); err != nil {
		t.Fatal(err)
	}
	tracks, err := store.Tracks(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if len(tracks) != 1 || tracks[0].CoverURL == "" {
		t.Fatalf("cover not restored after upgrade: %+v", tracks)
	}
}

func TestPlaylistsShowTheCoversOfTheirFirstSongs(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	var tracks []library.Track
	for i, cover := range []string{"c1", "", "c2", "c1", "c3", "c4", "c5"} {
		tracks = append(tracks, library.Track{ID: "t" + strconv.Itoa(i), Path: "t" + strconv.Itoa(i) + ".mp3", Title: "T", Artist: "A", Album: "B", AlbumArtist: "A", Format: "MP3", CoverID: cover})
	}
	if err := store.UpsertTracks(ctx, tracks); err != nil {
		t.Fatal(err)
	}
	playlist, err := store.CreatePlaylist(ctx, "Mezcla")
	if err != nil {
		t.Fatal(err)
	}
	for _, track := range tracks {
		if err := store.AddTrackToPlaylist(ctx, playlist.ID, track.ID); err != nil {
			t.Fatal(err)
		}
	}
	if _, err := store.CreatePlaylist(ctx, "Vacía"); err != nil {
		t.Fatal(err)
	}

	playlists, err := store.Playlists(ctx)
	if err != nil {
		t.Fatal(err)
	}

	covers := map[string][]string{}
	for _, item := range playlists {
		covers[item.Name] = item.Covers
	}
	want := []string{artworkURL("c1"), artworkURL("c2"), artworkURL("c3"), artworkURL("c4")}
	if !reflect.DeepEqual(covers["Mezcla"], want) {
		t.Fatalf("covers = %v; want the first four different ones in playlist order", covers["Mezcla"])
	}
	if covers["Vacía"] == nil || len(covers["Vacía"]) != 0 {
		t.Fatalf("covers of an empty playlist = %#v; want an empty list", covers["Vacía"])
	}
}

func TestStoreKeepsDiscNumbers(t *testing.T) {
	store := openTestStore(t)
	ctx := context.Background()
	saved := library.Track{ID: "doble", Path: filepath.Join(t.TempDir(), "doble.mp3"), Title: "Doble", Artist: "A", Album: "B",
		AlbumArtist: "A", Format: "MP3", TrackNumber: 4, DiscNumber: 2}
	if err := store.UpsertTracks(ctx, []library.Track{saved}); err != nil {
		t.Fatal(err)
	}

	track, err := store.Track(ctx, "doble")
	if err != nil {
		t.Fatal(err)
	}
	if track.TrackNumber != 4 || track.DiscNumber != 2 {
		t.Fatalf("stored track %d on disc %d; want track 4 on disc 2", track.TrackNumber, track.DiscNumber)
	}
}

func TestOpenStoreRereadsTracksSavedWithoutDiscNumbers(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "library.db")
	createLegacyDatabase(t, dbPath,
		`INSERT INTO tracks(id,path,title,artist,album,album_artist,format,size_bytes,modified_unix,available)
			VALUES ('vieja','vieja.mp3','Vieja','A','B','A','MP3',10,1700000000,1)`)
	ctx := context.Background()

	store, err := OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	closed := false
	t.Cleanup(func() {
		if !closed {
			store.Close()
		}
	})
	old, err := store.Track(ctx, "vieja")
	if err != nil {
		t.Fatal(err)
	}
	if old.ModifiedUnix != 0 {
		t.Fatalf("modified time after upgrade = %d; want 0 so the next scan reads the disc number", old.ModifiedUnix)
	}

	// Once upgraded, opening the library again keeps what the scan saved.
	old.ModifiedUnix, old.DiscNumber = 1700000000, 1
	if err := store.UpsertTracks(ctx, []library.Track{old}); err != nil {
		t.Fatal(err)
	}
	store.Close()
	closed = true
	reopened, err := OpenStore(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer reopened.Close()
	again, err := reopened.Track(ctx, "vieja")
	if err != nil {
		t.Fatal(err)
	}
	if again.ModifiedUnix != 1700000000 || again.DiscNumber != 1 {
		t.Fatalf("after reopening: modified %d, disc %d; want 1700000000 and 1", again.ModifiedUnix, again.DiscNumber)
	}
}

func trackIDs(tracks []library.Track) string {
	ids := make([]string, 0, len(tracks))
	for _, track := range tracks {
		ids = append(ids, track.ID)
	}
	sort.Strings(ids)
	return strings.Join(ids, ",")
}

func itoa(value int64) string { return strconv.FormatInt(value, 10) }

// createLegacyDatabase builds the schema shipped in version 1.0.0, before
// pictures moved out of the track and theme rows.
func createLegacyDatabase(t *testing.T, path string, inserts ...string) {
	t.Helper()
	db, err := sql.Open("sqlite", path)
	if err != nil {
		t.Fatal(err)
	}
	defer db.Close()
	statements := append([]string{
		`CREATE TABLE tracks (
			id TEXT PRIMARY KEY, path TEXT NOT NULL UNIQUE, title TEXT NOT NULL,
			artist TEXT NOT NULL, album TEXT NOT NULL, album_artist TEXT NOT NULL,
			genre TEXT NOT NULL DEFAULT '', year INTEGER NOT NULL DEFAULT 0,
			track_number INTEGER NOT NULL DEFAULT 0, duration_seconds INTEGER NOT NULL DEFAULT 0,
			format TEXT NOT NULL, size_bytes INTEGER NOT NULL DEFAULT 0, modified_unix INTEGER NOT NULL DEFAULT 0,
			cover_data_url TEXT NOT NULL DEFAULT '', available INTEGER NOT NULL DEFAULT 1,
			favorite INTEGER NOT NULL DEFAULT 0
		)`,
		`CREATE TABLE folders (path TEXT PRIMARY KEY)`,
		`CREATE TABLE themes (
			id TEXT PRIMARY KEY, name TEXT NOT NULL, background TEXT NOT NULL, accent TEXT NOT NULL,
			surface TEXT NOT NULL, text_color TEXT NOT NULL, muted TEXT NOT NULL,
			background_image TEXT NOT NULL DEFAULT '', built_in INTEGER NOT NULL DEFAULT 0
		)`,
	}, inserts...)
	for _, statement := range statements {
		if _, err := db.Exec(statement); err != nil {
			t.Fatalf("%v\n%s", err, statement)
		}
	}
}
