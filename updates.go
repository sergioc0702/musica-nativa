package main

import (
	"context"
	"fmt"

	"github.com/wailsapp/wails/v3/pkg/application"
	"github.com/wailsapp/wails/v3/pkg/updater"
	"github.com/wailsapp/wails/v3/pkg/updater/providers/github"
)

type UpdateCheckResult struct {
	CurrentVersion string `json:"currentVersion"`
	Available      bool   `json:"available"`
	Version        string `json:"version"`
	Name           string `json:"name,omitempty"`
	Notes          string `json:"notes,omitempty"`
}

// updateSource is the part of the Wails updater (*updater.Updater) that the
// service drives.
type updateSource interface {
	State() updater.State
	Check(ctx context.Context) (*updater.Release, error)
	DownloadAndInstall(ctx context.Context) error
	Restart(ctx context.Context) error
}

func configureUpdates(app *application.App) error {
	provider, err := github.New(github.Config{
		Repository:    updateRepository,
		ChecksumAsset: "SHA256SUMS",
	})
	if err != nil {
		return err
	}
	return app.Updater.Init(updater.Config{
		CurrentVersion: appVersion,
		Providers:      []updater.Provider{provider},
		Window:         updater.WindowNone,
	})
}

// CheckForUpdates only looks for a newer version; it never downloads. The
// interface runs it when the app opens to show that an update is available.
func (s *LibraryService) CheckForUpdates() (UpdateCheckResult, error) {
	updates, err := s.updateSource()
	if err != nil {
		return UpdateCheckResult{}, err
	}
	if !s.updateMu.TryLock() {
		return UpdateCheckResult{}, fmt.Errorf("ya hay una actualización en curso")
	}
	defer s.updateMu.Unlock()
	if state := updates.State(); updateInProgress(state) || state == updater.StateReady {
		return UpdateCheckResult{}, fmt.Errorf("termina la actualización actual antes de buscar otra")
	}
	release, err := updates.Check(context.Background())
	if err != nil {
		return UpdateCheckResult{}, err
	}
	return checkResult(release), nil
}

// UpdateNow is the one-click update: when a newer version exists it downloads
// and verifies it, pauses playback and restarts the app into it. An update
// that was already downloaded is applied straight away. When the app is up to
// date nothing else happens.
func (s *LibraryService) UpdateNow() (UpdateCheckResult, error) {
	updates, err := s.updateSource()
	if err != nil {
		return UpdateCheckResult{}, err
	}
	if !s.updateMu.TryLock() {
		return UpdateCheckResult{}, fmt.Errorf("ya hay una actualización en curso")
	}
	defer s.updateMu.Unlock()

	ctx := context.Background()
	result := checkResult(nil)
	switch state := updates.State(); {
	case state == updater.StateReady:
		result.Available = true
	case updateInProgress(state):
		return result, fmt.Errorf("termina la actualización actual antes de empezar otra")
	default:
		release, err := updates.Check(ctx)
		if err != nil || release == nil {
			return result, err
		}
		result = checkResult(release)
		if err := updates.DownloadAndInstall(ctx); err != nil {
			return result, err
		}
	}
	// Stop the music cleanly instead of cutting it off when the app quits.
	s.player.Pause()
	return result, updates.Restart(ctx)
}

func checkResult(release *updater.Release) UpdateCheckResult {
	if release == nil {
		return UpdateCheckResult{CurrentVersion: appVersion, Version: appVersion}
	}
	return UpdateCheckResult{
		CurrentVersion: appVersion,
		Available:      true,
		Version:        release.Version,
		Name:           release.Name,
		Notes:          release.Notes,
	}
}

func updateInProgress(state updater.State) bool {
	switch state {
	case updater.StateChecking, updater.StateDownloading, updater.StateVerifying, updater.StateInstalling:
		return true
	}
	return false
}

func (s *LibraryService) updateSource() (updateSource, error) {
	if s.updates == nil {
		return nil, fmt.Errorf("el servicio de actualizaciones no está disponible")
	}
	return s.updates, nil
}
