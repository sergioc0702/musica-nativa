package library

import (
	"bytes"
	"context"
	"encoding/binary"
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestSupportedAudioFormat(t *testing.T) {
	for _, path := range []string{"song.mp3", "song.WAV", "folder/track.FlAc"} {
		if !Supported(path) {
			t.Errorf("Supported(%q) = false, want true", path)
		}
	}
	for _, path := range []string{"cover.jpg", "song.m4a", "song.flac.txt", "song"} {
		if Supported(path) {
			t.Errorf("Supported(%q) = true, want false", path)
		}
	}
}

func TestScanRootsFindsSupportedFilesAndUsesFallbackMetadata(t *testing.T) {
	root := t.TempDir()
	paths := []string{
		filepath.Join(root, "album", "song.mp3"),
		filepath.Join(root, "album", "second.FLAC"),
		filepath.Join(root, "cover.png"),
	}
	for _, path := range paths {
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(path, []byte("not a real audio file"), 0o600); err != nil {
			t.Fatal(err)
		}
	}

	var progress []Track
	tracks, issues := ScanRoots(context.Background(), []string{root}, ScanOptions{OnTrack: func(track Track) {
		progress = append(progress, track)
	}})
	if len(issues) != 0 {
		t.Fatalf("ScanRoots returned issues: %v", issues)
	}
	if len(tracks) != 2 || len(progress) != 2 {
		t.Fatalf("found %d tracks and %d progress updates, want 2", len(tracks), len(progress))
	}
	for _, track := range tracks {
		if track.Title == "" || track.Artist != "Artista desconocido" || track.Album != "Sencillos" {
			t.Errorf("unexpected fallback metadata: %+v", track)
		}
		if track.ID == "" || track.DurationSeconds != 0 {
			t.Errorf("unexpected identity or duration defaults: %+v", track)
		}
	}
}

func TestScanRootsReportsMissingFolderAndContinues(t *testing.T) {
	root := t.TempDir()
	trackPath := filepath.Join(root, "keep.wav")
	if err := os.WriteFile(trackPath, []byte("x"), 0o600); err != nil {
		t.Fatal(err)
	}
	tracks, issues := ScanRoots(context.Background(), []string{filepath.Join(root, "gone"), root}, ScanOptions{})
	if len(tracks) != 1 {
		t.Fatalf("found %d tracks, want 1", len(tracks))
	}
	if len(issues) == 0 {
		t.Fatal("expected a missing folder issue")
	}
}

func TestScanRootsHandsOverEachDistinctCoverOnce(t *testing.T) {
	root := t.TempDir()
	albumArt := fakePNG("album art")
	singleArt := fakePNG("single art")
	writeTaggedMP3(t, filepath.Join(root, "album", "01.mp3"), "Uno", albumArt)
	writeTaggedMP3(t, filepath.Join(root, "album", "02.mp3"), "Dos", albumArt)
	writeTaggedMP3(t, filepath.Join(root, "single.mp3"), "Tres", singleArt)
	writeTaggedMP3(t, filepath.Join(root, "plain.mp3"), "Cuatro", nil)

	var saved []Artwork
	tracks, issues := ScanRoots(context.Background(), []string{root}, ScanOptions{
		OnArtwork: func(art Artwork) error {
			saved = append(saved, art)
			return nil
		},
	})
	if len(issues) != 0 {
		t.Fatalf("ScanRoots returned issues: %v", issues)
	}
	if len(saved) != 2 {
		t.Fatalf("handed over %d covers, want 2 distinct covers", len(saved))
	}
	for _, art := range saved {
		if art.ID == "" || art.MIME != "image/png" {
			t.Errorf("cover = id %q, mime %q; want an id and image/png", art.ID, art.MIME)
		}
		if !bytes.Equal(art.Data, albumArt) && !bytes.Equal(art.Data, singleArt) {
			t.Errorf("cover bytes do not match either embedded picture")
		}
	}

	byTitle := make(map[string]Track)
	for _, track := range tracks {
		byTitle[track.Title] = track
	}
	if byTitle["Uno"].CoverID == "" || byTitle["Uno"].CoverID != byTitle["Dos"].CoverID {
		t.Errorf("album tracks should share one cover: %q and %q", byTitle["Uno"].CoverID, byTitle["Dos"].CoverID)
	}
	if byTitle["Tres"].CoverID == "" || byTitle["Tres"].CoverID == byTitle["Uno"].CoverID {
		t.Errorf("single should have its own cover, got %q", byTitle["Tres"].CoverID)
	}
	if byTitle["Cuatro"].CoverID != "" {
		t.Errorf("track without artwork got cover %q", byTitle["Cuatro"].CoverID)
	}
}

func TestScanRootsReusesKnownTrackUntilFileChanges(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "song.mp3")
	writeTaggedMP3(t, path, "Uno", nil)
	modified := time.Date(2026, 3, 4, 10, 0, 0, 0, time.UTC)
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatal(err)
	}
	first, issues := ScanRoots(context.Background(), []string{root}, ScanOptions{})
	if len(issues) != 0 || len(first) != 1 || first[0].Title != "Uno" {
		t.Fatalf("first scan = %+v, issues %v", first, issues)
	}
	known := map[string]Track{PathKey(path): first[0]}

	// Same size and modification time: the catalog entry is trusted.
	writeTaggedMP3(t, path, "Dos", nil)
	if err := os.Chtimes(path, modified, modified); err != nil {
		t.Fatal(err)
	}
	unchanged, _ := ScanRoots(context.Background(), []string{root}, ScanOptions{Known: known})
	if len(unchanged) != 1 || unchanged[0].Title != "Uno" {
		t.Fatalf("unchanged file was read again: %+v", unchanged)
	}

	// A new modification time means the tags must be read again.
	later := modified.Add(time.Hour)
	if err := os.Chtimes(path, later, later); err != nil {
		t.Fatal(err)
	}
	changed, _ := ScanRoots(context.Background(), []string{root}, ScanOptions{Known: known})
	if len(changed) != 1 || changed[0].Title != "Dos" {
		t.Fatalf("changed file kept stale metadata: %+v", changed)
	}
}

func TestScanRootsReadsTrackAndDiscNumbers(t *testing.T) {
	root := t.TempDir()
	frames := textFrame("TIT2", "Segunda parte")
	frames = append(frames, textFrame("TRCK", "3/12")...)
	frames = append(frames, textFrame("TPOS", "2/2")...)
	writeID3File(t, filepath.Join(root, "song.mp3"), frames)

	tracks, issues := ScanRoots(context.Background(), []string{root}, ScanOptions{})
	if len(issues) != 0 {
		t.Fatalf("ScanRoots returned issues: %v", issues)
	}
	if len(tracks) != 1 || tracks[0].TrackNumber != 3 || tracks[0].DiscNumber != 2 {
		t.Fatalf("tracks = %+v; want track 3 on disc 2", tracks)
	}
}

func TestReadLyricsReturnsLyricsStoredInTheFile(t *testing.T) {
	path := filepath.Join(t.TempDir(), "song.mp3")
	uslt := append([]byte{3}, "spa"...) // UTF-8 text, Spanish
	uslt = append(uslt, 0)              // empty description
	uslt = append(uslt, "Primera línea\nSegunda"...)
	writeID3File(t, path, append(textFrame("TIT2", "Con letra"), id3Frame("USLT", uslt)...))

	if got := ReadLyrics(path); got != "Primera línea\nSegunda" {
		t.Fatalf("lyrics = %q", got)
	}
	writeTaggedMP3(t, path, "Sin letra", nil)
	if got := ReadLyrics(path); got != "" {
		t.Fatalf("lyrics of a file without them = %q; want none", got)
	}
}

// fakePNG returns bytes that start with the PNG signature, which is all the
// content sniffing needs to recognise them as an image.
func fakePNG(body string) []byte {
	return append([]byte("\x89PNG\r\n\x1a\n"), body...)
}

// writeTaggedMP3 writes an ID3v2.3 tag with a title and an optional front cover.
// The audio payload is not valid MPEG data, so the duration stays unknown.
func writeTaggedMP3(t *testing.T, path, title string, picture []byte) {
	t.Helper()
	frames := id3Frame("TIT2", append([]byte{0}, title...))
	if picture != nil {
		apic := append([]byte{0}, "image/png"...)
		apic = append(apic, 0, 3, 0) // MIME terminator, front cover, empty description
		apic = append(apic, picture...)
		frames = append(frames, id3Frame("APIC", apic)...)
	}
	writeID3File(t, path, frames)
}

// textFrame is an ID3v2.3 text frame such as TRCK or TPOS, in ISO-8859-1.
func textFrame(id, value string) []byte {
	return id3Frame(id, append([]byte{0}, value...))
}

// writeID3File writes an ID3v2.3 tag holding frames, followed by bytes that
// are not valid MPEG audio.
func writeID3File(t *testing.T, path string, frames []byte) {
	t.Helper()
	size := len(frames)
	data := []byte{'I', 'D', '3', 3, 0, 0, byte(size >> 21 & 0x7f), byte(size >> 14 & 0x7f), byte(size >> 7 & 0x7f), byte(size & 0x7f)}
	data = append(data, frames...)
	data = append(data, "not mpeg audio"...)
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(path, data, 0o600); err != nil {
		t.Fatal(err)
	}
}

func id3Frame(id string, payload []byte) []byte {
	frame := make([]byte, 10, 10+len(payload))
	copy(frame, id)
	binary.BigEndian.PutUint32(frame[4:8], uint32(len(payload)))
	return append(frame, payload...)
}
