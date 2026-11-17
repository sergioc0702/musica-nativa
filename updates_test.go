package main

import (
	"context"
	"errors"
	"path/filepath"
	"strings"
	"testing"

	"github.com/wailsapp/wails/v3/pkg/updater"
)

// fakeUpdates stands in for the Wails updater and records what the service
// asks of it, moving through the same states.
type fakeUpdates struct {
	state            updater.State
	release          *updater.Release
	downloadErr      error
	calls            []string
	player           *AudioPlayer
	playingAtRestart bool
}

func (f *fakeUpdates) State() updater.State { return f.state }

func (f *fakeUpdates) Check(context.Context) (*updater.Release, error) {
	f.calls = append(f.calls, "check")
	if f.release == nil {
		f.state = updater.StateUpToDate
		return nil, nil
	}
	f.state = updater.StateAvailable
	return f.release, nil
}

func (f *fakeUpdates) DownloadAndInstall(context.Context) error {
	f.calls = append(f.calls, "download")
	if f.downloadErr != nil {
		f.state = updater.StateError
		return f.downloadErr
	}
	f.state = updater.StateReady
	return nil
}

func (f *fakeUpdates) Restart(context.Context) error {
	f.calls = append(f.calls, "restart")
	if f.player != nil {
		f.playingAtRestart = f.player.State().IsPlaying
	}
	return nil
}

func newUpdateTestService(t *testing.T, updates *fakeUpdates) *LibraryService {
	t.Helper()
	player := newTestPlayer(&fakeOutputs{})
	if _, _, err := player.SetOutput("shared", ""); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(player.Close)
	service := NewLibraryService(openTestStore(t), player)
	service.updates = updates
	updates.player = player
	return service
}

func TestUpdateNowInstallsNewerVersionAndRestarts(t *testing.T) {
	updates := &fakeUpdates{state: updater.StateIdle, release: &updater.Release{Version: "9.9.9", Name: "Música Nativa 9.9.9"}}
	service := newUpdateTestService(t, updates)
	// A repository fixture: the decoder keeps it open, which on Windows would
	// stop a temporary copy from being cleaned up.
	if err := service.player.Play(filepath.Join("audio-testdata", "sample.mp3"), 0); err != nil {
		t.Fatal(err)
	}
	if !service.player.State().IsPlaying {
		t.Fatal("the test song did not start")
	}

	result, err := service.UpdateNow()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(updates.calls, ","); got != "check,download,restart" {
		t.Fatalf("updater calls = %s; want check,download,restart", got)
	}
	if !result.Available || result.Version != "9.9.9" {
		t.Fatalf("result = %+v; want version 9.9.9 available", result)
	}
	if updates.playingAtRestart {
		t.Fatal("the song was still playing when the app restarted")
	}
}

func TestUpdateNowLeavesUpToDateAppRunning(t *testing.T) {
	updates := &fakeUpdates{state: updater.StateIdle}
	service := newUpdateTestService(t, updates)

	result, err := service.UpdateNow()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(updates.calls, ","); got != "check" {
		t.Fatalf("updater calls = %s; want only check", got)
	}
	if result.Available {
		t.Fatalf("result = %+v; want no update", result)
	}
}

func TestUpdateNowKeepsAppRunningWhenDownloadFails(t *testing.T) {
	updates := &fakeUpdates{state: updater.StateIdle, release: &updater.Release{Version: "9.9.9"}, downloadErr: errors.New("sin conexión")}
	service := newUpdateTestService(t, updates)

	if _, err := service.UpdateNow(); err == nil {
		t.Fatal("expected the download error")
	}
	if got := strings.Join(updates.calls, ","); got != "check,download" {
		t.Fatalf("updater calls = %s; want check,download and no restart", got)
	}
}

func TestUpdateNowFinishesAnUpdateAlreadyDownloaded(t *testing.T) {
	updates := &fakeUpdates{state: updater.StateReady}
	service := newUpdateTestService(t, updates)

	result, err := service.UpdateNow()
	if err != nil {
		t.Fatal(err)
	}
	if got := strings.Join(updates.calls, ","); got != "restart" {
		t.Fatalf("updater calls = %s; want only restart", got)
	}
	if !result.Available {
		t.Fatalf("result = %+v; want the pending update reported", result)
	}
}

func TestUpdateNowWaitsForUpdateInProgress(t *testing.T) {
	updates := &fakeUpdates{state: updater.StateDownloading}
	service := newUpdateTestService(t, updates)

	if _, err := service.UpdateNow(); err == nil {
		t.Fatal("expected an error while another update downloads")
	}
	if len(updates.calls) != 0 {
		t.Fatalf("updater calls = %v; want none", updates.calls)
	}
}

func TestCheckForUpdatesOnlyLooks(t *testing.T) {
	updates := &fakeUpdates{state: updater.StateIdle, release: &updater.Release{Version: "9.9.9"}}
	service := newUpdateTestService(t, updates)

	result, err := service.CheckForUpdates()
	if err != nil {
		t.Fatal(err)
	}
	if !result.Available || result.Version != "9.9.9" {
		t.Fatalf("result = %+v; want version 9.9.9 available", result)
	}
	if got := strings.Join(updates.calls, ","); got != "check" {
		t.Fatalf("updater calls = %s; want only check, nothing downloaded", got)
	}
}
