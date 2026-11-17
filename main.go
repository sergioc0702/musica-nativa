package main

import (
	"embed"
	"log"
	"os"
	"path/filepath"

	"github.com/wailsapp/wails/v3/pkg/application"
)

//go:embed all:frontend/dist
var assets embed.FS

func main() {
	configDir, err := os.UserConfigDir()
	if err != nil {
		log.Fatal(err)
	}
	appDir := filepath.Join(configDir, "MusicaNativa")
	store, err := OpenStore(filepath.Join(appDir, "library.db"))
	if err != nil {
		log.Fatal(err)
	}
	player := NewAudioPlayer()
	service := NewLibraryService(store, player)

	application.RegisterEvent[ScanProgress]("library:scan-progress")
	application.RegisterEvent[PlaybackState]("player:state")
	application.RegisterEvent[AudioDevicesChanged]("audio:devices")
	app := application.New(application.Options{
		Name:        "Música Nativa",
		Description: "Tu colección de música, en casa.",
		Services:    []application.Service{application.NewService(service)},
		Assets: application.AssetOptions{
			Handler:    application.AssetFileServerFS(assets),
			Middleware: artworkMiddleware(store),
		},
	})
	if err := configureUpdates(app); err != nil {
		log.Fatal(err)
	}
	service.app = app

	app.Window.NewWithOptions(application.WebviewWindowOptions{
		Title:            "Música Nativa",
		Width:            1280,
		Height:           760,
		MinWidth:         900,
		MinHeight:        560,
		StartState:       application.WindowStateMaximised,
		BackgroundColour: application.NewRGB(17, 17, 15),
		URL:              "/",
	})

	if err := app.Run(); err != nil {
		log.Fatal(err)
	}
}
