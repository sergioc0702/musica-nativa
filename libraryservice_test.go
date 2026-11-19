package main

import (
	"context"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"musica-nativa/internal/library"
)

func receiveRoots(t *testing.T, scans <-chan []string) []string {
	t.Helper()
	select {
	case roots := <-scans:
		return roots
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for a library scan")
		return nil
	}
}

func TestFolderAddedDuringScanIsScannedToo(t *testing.T) {
	store := openTestStore(t)
	service := NewLibraryService(store, nil)
	ctx := context.Background()
	first, second := t.TempDir(), t.TempDir()
	if err := store.ReplaceFolders(ctx, []string{first}); err != nil {
		t.Fatal(err)
	}
	scans := make(chan []string, 4)
	release := make(chan struct{})
	service.scanLibrary = func(ctx context.Context, roots []string, _ library.ScanOptions) ([]library.Track, []error) {
		scans <- roots
		select {
		case <-release:
		case <-ctx.Done():
		}
		return nil, nil
	}
	defer service.stopScanning()

	if !service.RefreshLibrary() {
		t.Fatal("the first scan did not start")
	}
	receiveRoots(t, scans)
	if _, err := service.addFolders([]string{second}); err != nil {
		t.Fatal(err)
	}
	close(release)

	roots := receiveRoots(t, scans)
	found := false
	for _, root := range roots {
		found = found || strings.EqualFold(root, second)
	}
	if !found {
		t.Fatalf("scan after adding a folder covered %v; want it to include %s", roots, second)
	}
}

func TestStoppedScanKeepsTracksFoundSoFar(t *testing.T) {
	store := openTestStore(t)
	service := NewLibraryService(store, nil)
	ctx := context.Background()
	root := t.TempDir()
	if err := store.ReplaceFolders(ctx, []string{root}); err != nil {
		t.Fatal(err)
	}
	reported := make(chan struct{})
	service.scanLibrary = func(ctx context.Context, _ []string, options library.ScanOptions) ([]library.Track, []error) {
		for _, name := range []string{"a", "b"} {
			options.OnTrack(library.Track{ID: name, Path: filepath.Join(root, name+".mp3"), Title: name, Format: "MP3", Available: true})
		}
		close(reported)
		<-ctx.Done()
		return nil, nil
	}

	service.RefreshLibrary()
	select {
	case <-reported:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the scan to report tracks")
	}
	service.stopScanning()

	tracks, err := store.Tracks(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := trackIDs(tracks); got != "a,b" {
		t.Fatalf("tracks kept after stopping the scan = %q; want a,b", got)
	}
}

func TestScanSkipsFilesAlreadyInCatalog(t *testing.T) {
	store := openTestStore(t)
	service := NewLibraryService(store, nil)
	ctx := context.Background()
	root := t.TempDir()
	path := filepath.Join(root, "a.mp3")
	if err := store.ReplaceFolders(ctx, []string{root}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertTracks(ctx, []library.Track{{ID: "a", Path: path, Title: "A", Format: "MP3", Available: true}}); err != nil {
		t.Fatal(err)
	}
	var known map[string]library.Track
	done := make(chan struct{})
	service.scanLibrary = func(_ context.Context, _ []string, options library.ScanOptions) ([]library.Track, []error) {
		known = options.Known
		close(done)
		return nil, nil
	}
	defer service.stopScanning()

	service.RefreshLibrary()
	select {
	case <-done:
	case <-time.After(5 * time.Second):
		t.Fatal("timed out waiting for the scan")
	}
	if known[library.PathKey(path)].ID != "a" {
		t.Fatalf("scan was not given the catalog entry for %s: %v", path, known)
	}
}

func TestRemovingNestedFolderKeepsTracksCoveredByParentFolder(t *testing.T) {
	store := openTestStore(t)
	service := NewLibraryService(store, nil)
	ctx := context.Background()
	parent := t.TempDir()
	child := filepath.Join(parent, "Rock")
	if err := store.ReplaceFolders(ctx, []string{parent, child}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertTracks(ctx, []library.Track{
		{ID: "rock", Path: filepath.Join(child, "song.mp3"), Title: "Rock", Available: true},
		{ID: "pop", Path: filepath.Join(parent, "pop.mp3"), Title: "Pop", Available: true},
	}); err != nil {
		t.Fatal(err)
	}

	if err := service.RemoveMusicFolder(child); err != nil {
		t.Fatal(err)
	}
	tracks, err := store.Tracks(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := trackIDs(tracks); got != "pop,rock" {
		t.Fatalf("library after removing the nested folder = %q; want pop,rock", got)
	}
	for _, track := range tracks {
		if !track.Available {
			t.Errorf("%s was marked unavailable although its parent folder is still in the library", track.ID)
		}
	}
}

func TestRemovingFolderTakesItsTracksOutOfLibrary(t *testing.T) {
	store := openTestStore(t)
	service := NewLibraryService(store, nil)
	ctx := context.Background()
	kept, removed := t.TempDir(), t.TempDir()
	if err := store.ReplaceFolders(ctx, []string{kept, removed}); err != nil {
		t.Fatal(err)
	}
	if err := store.UpsertTracks(ctx, []library.Track{
		{ID: "stays", Path: filepath.Join(kept, "stays.mp3"), Title: "Se queda", Available: true},
		{ID: "goes", Path: filepath.Join(removed, "goes.mp3"), Title: "Se va", Available: true},
	}); err != nil {
		t.Fatal(err)
	}

	if err := service.RemoveMusicFolder(removed); err != nil {
		t.Fatal(err)
	}
	tracks, err := store.Tracks(ctx, "")
	if err != nil {
		t.Fatal(err)
	}
	if got := trackIDs(tracks); got != "stays" {
		t.Fatalf("library after removing a folder = %q; want stays", got)
	}
}

func TestSetAudioOptionsChangesDeviceAndModeTogether(t *testing.T) {
	store := openTestStore(t)
	outputs := &fakeOutputs{}
	player := newTestPlayer(outputs)
	if _, _, err := player.SetOutput("shared", ""); err != nil {
		t.Fatal(err)
	}
	service := NewLibraryService(store, player)

	state := service.SetAudioOptions("exclusive", "usb-dac")
	if state.OutputMode != "exclusive" || state.DeviceID != "usb-dac" || !state.OutputReady {
		t.Fatalf("output = %s on %q (ready %v); want exclusive on usb-dac", state.OutputMode, state.DeviceID, state.OutputReady)
	}
	ctx := context.Background()
	mode, _ := store.Preference(ctx, "outputMode", "")
	device, _ := store.Preference(ctx, "outputDevice", "")
	if mode != "exclusive" || device != "usb-dac" {
		t.Fatalf("saved output = %s on %q; want exclusive on usb-dac", mode, device)
	}
}
