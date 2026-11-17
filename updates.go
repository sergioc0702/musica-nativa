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

func (s *LibraryService) CheckForUpdates() (UpdateCheckResult, error) {
	app, err := s.updateApp()
	if err != nil {
		return UpdateCheckResult{}, err
	}
	if !s.updateMu.TryLock() {
		return UpdateCheckResult{}, fmt.Errorf("ya hay una actualización en curso")
	}
	defer s.updateMu.Unlock()

	switch app.Updater.State() {
	case updater.StateChecking, updater.StateDownloading, updater.StateVerifying, updater.StateInstalling, updater.StateReady:
		return UpdateCheckResult{}, fmt.Errorf("termina la actualización actual antes de buscar otra")
	}

	release, err := app.Updater.Check(context.Background())
	if err != nil {
		return UpdateCheckResult{}, err
	}
	if release == nil {
		return UpdateCheckResult{CurrentVersion: appVersion, Version: appVersion}, nil
	}
	return UpdateCheckResult{
		CurrentVersion: appVersion,
		Available:      true,
		Version:        release.Version,
		Name:           release.Name,
		Notes:          release.Notes,
	}, nil
}

func (s *LibraryService) DownloadUpdate() error {
	app, err := s.updateApp()
	if err != nil {
		return err
	}
	if !s.updateMu.TryLock() {
		return fmt.Errorf("la descarga de una actualización ya está en curso")
	}
	defer s.updateMu.Unlock()
	if app.Updater.State() != updater.StateAvailable {
		return fmt.Errorf("primero busca una actualización disponible")
	}
	return app.Updater.DownloadAndInstall(context.Background())
}

func (s *LibraryService) RestartUpdate() error {
	app, err := s.updateApp()
	if err != nil {
		return err
	}
	if app.Updater.State() != updater.StateReady {
		return fmt.Errorf("todavía no hay una actualización lista para aplicar")
	}
	return app.Updater.Restart(context.Background())
}

func (s *LibraryService) updateApp() (*application.App, error) {
	if s.app == nil || s.app.Updater == nil {
		return nil, fmt.Errorf("el servicio de actualizaciones no está disponible")
	}
	return s.app, nil
}
