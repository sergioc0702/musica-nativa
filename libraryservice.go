package main

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"github.com/wailsapp/wails/v3/pkg/application"
	"musica-nativa/internal/library"
)

type ScanProgress struct {
	Phase      string   `json:"phase"`
	Discovered int      `json:"discovered"`
	Total      int      `json:"total"`
	Message    string   `json:"message"`
	Issues     []string `json:"issues,omitempty"`
}

type AppState struct {
	Version       string          `json:"version"`
	Tracks        []library.Track `json:"tracks"`
	Folders       []string        `json:"folders"`
	Playlists     []Playlist      `json:"playlists"`
	Themes        []Theme         `json:"themes"`
	OutputDevices []AudioDevice   `json:"outputDevices"`
	ActiveTheme   string          `json:"activeTheme"`
	Playback      PlaybackState   `json:"playback"`
	Scanning      bool            `json:"scanning"`
}

type AudioDevicesChanged struct {
	Devices []AudioDevice `json:"devices"`
}

// ThemeSaveResult is the theme as saved and the updated list of themes.
type ThemeSaveResult struct {
	Theme  Theme   `json:"theme"`
	Themes []Theme `json:"themes"`
}

const (
	// scanBatchSize tracks are saved together while a scan runs, so stopping
	// a scan keeps what it already found.
	scanBatchSize = 200
	// scanProgressInterval limits progress events sent to the interface.
	scanProgressInterval = 150 * time.Millisecond
)

type LibraryService struct {
	app    *application.App
	store  *Store
	player *AudioPlayer
	// scanLibrary walks the music folders; it is library.ScanRoots.
	scanLibrary func(context.Context, []string, library.ScanOptions) ([]library.Track, []error)

	mu        sync.Mutex
	scanBusy  bool
	scanAgain bool
	scanStop  context.CancelFunc
	closing   bool
	scans     sync.WaitGroup
	queue     *playQueue
	updateMu  sync.Mutex
	// lrclib finds lyrics on lrclib.net when the song brings none.
	lrclib  *lrclibClient
	updates updateSource
}

func NewLibraryService(store *Store, player *AudioPlayer) *LibraryService {
	return &LibraryService{store: store, player: player, scanLibrary: library.ScanRoots, queue: newPlayQueue(), lrclib: newLRCLIBClient(lrclibURL)}
}

func (s *LibraryService) ServiceName() string { return "LibraryService" }

func (s *LibraryService) ServiceStartup(ctx context.Context, _ application.ServiceOptions) error {
	if volume, err := s.store.Preference(ctx, "volume", "72"); err == nil {
		var value int
		if _, scanErr := fmt.Sscanf(volume, "%d", &value); scanErr == nil {
			s.player.SetVolume(value)
		}
	}
	s.loadEqualizer(ctx)
	mode, _ := s.store.Preference(ctx, "outputMode", "shared")
	deviceID, _ := s.store.Preference(ctx, "outputDevice", "")
	if mode != "shared" || deviceID != "" {
		actualMode, actualDevice, _ := s.player.SetOutput(mode, deviceID)
		if actualMode != mode {
			_ = s.store.SetPreference(ctx, "outputMode", actualMode)
		}
		if actualDevice != deviceID {
			_ = s.store.SetPreference(ctx, "outputDevice", actualDevice)
		}
	}
	s.loadPlayMode(ctx)
	folders, err := s.store.Folders(ctx)
	if err != nil {
		return err
	}
	if len(folders) > 0 {
		s.startScan()
	}
	go s.playbackEvents(ctx)
	return nil
}

func (s *LibraryService) ServiceShutdown() error {
	s.stopScanning()
	s.player.Close()
	return s.store.Close()
}

func (s *LibraryService) AppState() (AppState, error) {
	ctx := context.Background()
	tracks, err := s.store.Tracks(ctx, "")
	if err != nil {
		return AppState{}, err
	}
	folders, err := s.store.Folders(ctx)
	if err != nil {
		return AppState{}, err
	}
	playlists, err := s.store.Playlists(ctx)
	if err != nil {
		return AppState{}, err
	}
	themes, err := s.store.Themes(ctx)
	if err != nil {
		return AppState{}, err
	}
	activeTheme, _ := s.store.Preference(ctx, "activeTheme", "nocturno")
	s.mu.Lock()
	scanning := s.scanBusy
	s.mu.Unlock()
	return AppState{Version: appVersion, Tracks: tracks, Folders: folders, Playlists: playlists, Themes: themes,
		OutputDevices: s.player.Devices(), ActiveTheme: activeTheme, Playback: s.player.State(), Scanning: scanning}, nil
}

func (s *LibraryService) ChooseMusicFolders() ([]string, error) {
	if s.app == nil {
		return nil, fmt.Errorf("la aplicación todavía no está lista")
	}
	selected, err := s.app.Dialog.OpenFile().
		SetTitle("Elige tus carpetas de música").
		CanChooseDirectories(true).
		CanChooseFiles(false).
		PromptForMultipleSelection()
	if err != nil {
		return nil, err
	}
	if len(selected) == 0 {
		return []string{}, nil
	}
	return s.addFolders(selected)
}

// addFolders adds music folders and makes sure they get scanned, even when a
// scan is already running.
func (s *LibraryService) addFolders(selected []string) ([]string, error) {
	folders, err := s.store.Folders(context.Background())
	if err != nil {
		return nil, err
	}
	merged := normalizeFolders(append(folders, selected...))
	if err := s.store.ReplaceFolders(context.Background(), merged); err != nil {
		return nil, err
	}
	s.scanFoldersAgain(true)
	return merged, nil
}

// RemoveMusicFolder takes a folder and the tracks only it covered out of the
// library. Tracks still inside another music folder stay.
func (s *LibraryService) RemoveMusicFolder(path string) error {
	ctx := context.Background()
	folders, err := s.store.Folders(ctx)
	if err != nil {
		return err
	}
	clean := filepath.Clean(path)
	filtered := make([]string, 0, len(folders))
	for _, folder := range folders {
		if !strings.EqualFold(filepath.Clean(folder), clean) {
			filtered = append(filtered, folder)
		}
	}
	if err := s.store.ReplaceFolders(ctx, filtered); err != nil {
		return err
	}
	if err := s.store.RemoveTracksOutsideFolders(ctx); err != nil {
		return err
	}
	// A scan that started with the old list must not finish with it.
	s.scanFoldersAgain(false)
	return nil
}

func normalizeFolders(folders []string) []string {
	seen := make(map[string]struct{})
	result := make([]string, 0, len(folders))
	for _, folder := range folders {
		folder = filepath.Clean(strings.TrimSpace(folder))
		if folder == "" || folder == "." {
			continue
		}
		key := strings.ToLower(folder)
		if _, ok := seen[key]; ok {
			continue
		}
		seen[key] = struct{}{}
		result = append(result, folder)
	}
	return result
}

func (s *LibraryService) RefreshLibrary() bool {
	folders, err := s.store.Folders(context.Background())
	if err != nil || len(folders) == 0 {
		s.emitScan(ScanProgress{Phase: "done", Message: "Añade una carpeta para empezar."})
		return false
	}
	return s.startScan()
}

// startScan scans the music folders in the background unless a scan is
// already running.
func (s *LibraryService) startScan() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	if s.scanBusy || s.closing {
		return false
	}
	ctx, cancel := context.WithCancel(context.Background())
	s.scanBusy, s.scanStop, s.scanAgain = true, cancel, false
	s.scans.Add(1)
	go s.scanLoop(ctx)
	return true
}

// scanFoldersAgain makes the current folder list get scanned: a running scan
// stops and starts over with the new list. With startIfIdle false, nothing
// happens while no scan is running.
func (s *LibraryService) scanFoldersAgain(startIfIdle bool) {
	s.mu.Lock()
	if s.scanBusy {
		s.scanAgain = true
		s.scanStop()
		s.mu.Unlock()
		return
	}
	s.mu.Unlock()
	if startIfIdle {
		s.startScan()
	}
}

// stopScanning stops a running scan, waits until it has saved what it found
// and refuses new scans. It runs when the app closes.
func (s *LibraryService) stopScanning() {
	s.mu.Lock()
	s.closing = true
	if s.scanStop != nil {
		s.scanStop()
	}
	s.mu.Unlock()
	s.scans.Wait()
}

func (s *LibraryService) scanLoop(ctx context.Context) {
	defer s.scans.Done()
	for {
		completed := s.scanOnce(ctx)
		s.mu.Lock()
		s.scanStop()
		if !s.scanAgain || s.closing {
			s.scanBusy, s.scanStop, s.scanAgain = false, nil, false
			s.mu.Unlock()
			if !completed {
				s.emitScan(ScanProgress{Phase: "cancelled", Message: "Escaneo detenido."})
			}
			return
		}
		s.scanAgain = false
		ctx, s.scanStop = context.WithCancel(context.Background())
		s.mu.Unlock()
	}
}

// scanOnce scans the music folders, saving tracks as it finds them, and reports
// whether it ran to the end. Unchanged files keep their catalog entry.
func (s *LibraryService) scanOnce(ctx context.Context) bool {
	// Saving uses its own context: a stopped scan still keeps what it found.
	save := context.Background()
	folders, err := s.store.Folders(save)
	if err == nil {
		var known map[string]library.Track
		if known, err = s.store.KnownTracks(save); err == nil {
			return s.scanFolders(ctx, folders, known)
		}
	}
	s.emitScan(ScanProgress{Phase: "done", Message: "No se pudo leer el catálogo.", Issues: []string{err.Error()}})
	return true
}

func (s *LibraryService) scanFolders(ctx context.Context, folders []string, known map[string]library.Track) bool {
	save := context.Background()
	s.emitScan(ScanProgress{Phase: "scanning", Message: "Buscando música en tus carpetas…"})
	var issues []error
	batch := make([]library.Track, 0, scanBatchSize)
	flush := func() {
		if len(batch) == 0 {
			return
		}
		if err := s.store.UpsertTracks(save, batch); err != nil {
			issues = append(issues, err)
		}
		batch = batch[:0]
	}
	present := make(map[string]struct{})
	var lastProgress time.Time
	_, scanIssues := s.scanLibrary(ctx, folders, library.ScanOptions{
		Known: known,
		OnTrack: func(track library.Track) {
			present[track.ID] = struct{}{}
			batch = append(batch, track)
			if len(batch) >= scanBatchSize {
				flush()
			}
			if time.Since(lastProgress) >= scanProgressInterval {
				lastProgress = time.Now()
				s.emitScan(ScanProgress{Phase: "scanning", Discovered: len(present), Message: fmt.Sprintf("%d canciones encontradas", len(present))})
			}
		},
		OnArtwork: func(art library.Artwork) error { return s.store.SaveArtwork(save, art) },
	})
	flush()
	issues = append(issues, scanIssues...)
	if ctx.Err() != nil {
		return false
	}

	validRoots := make([]string, 0, len(folders))
	for _, folder := range folders {
		if info, err := os.Stat(folder); err == nil && info.IsDir() {
			validRoots = append(validRoots, folder)
		}
	}
	if err := s.store.MarkMissingUnderRoots(save, validRoots, present); err != nil {
		issues = append(issues, err)
	}
	if err := s.store.RemoveTracksOutsideFolders(save); err != nil {
		issues = append(issues, err)
	}
	if err := s.store.RemoveUnusedArtwork(save); err != nil {
		issues = append(issues, err)
	}
	messages := make([]string, 0, len(issues))
	for _, issue := range issues {
		messages = append(messages, issue.Error())
	}
	message := fmt.Sprintf("Catálogo actualizado: %d canciones.", len(present))
	if len(issues) > 0 {
		message = fmt.Sprintf("Escaneo terminado con %d aviso(s).", len(issues))
	}
	s.emitScan(ScanProgress{Phase: "done", Discovered: len(present), Total: len(present), Message: message, Issues: messages})
	s.emitPlayback()
	return true
}

func (s *LibraryService) emitScan(progress ScanProgress) {
	if s.app != nil {
		s.app.Event.Emit("library:scan-progress", progress)
	}
}

func (s *LibraryService) SearchTracks(query string) ([]library.Track, error) {
	return s.store.Tracks(context.Background(), query)
}

func (s *LibraryService) GetPlaylistTracks(playlistID string) ([]library.Track, error) {
	return s.store.PlaylistTracks(context.Background(), playlistID)
}

func (s *LibraryService) CreatePlaylist(name string) (Playlist, error) {
	return s.store.CreatePlaylist(context.Background(), name)
}

func (s *LibraryService) DeletePlaylist(id string) error {
	return s.store.DeletePlaylist(context.Background(), id)
}

func (s *LibraryService) AddTrackToPlaylist(playlistID, trackID string) error {
	return s.store.AddTrackToPlaylist(context.Background(), playlistID, trackID)
}

func (s *LibraryService) RemoveTrackFromPlaylist(playlistID, trackID string) error {
	return s.store.RemoveTrackFromPlaylist(context.Background(), playlistID, trackID)
}

func (s *LibraryService) ToggleFavorite(trackID string) error {
	if err := s.store.ToggleFavorite(context.Background(), trackID); err != nil {
		return err
	}
	s.emitPlayback()
	return nil
}

func (s *LibraryService) SetQueue(trackIDs []string, startAt int) {
	s.mu.Lock()
	s.queue.set(trackIDs, startAt)
	s.mu.Unlock()
}

func (s *LibraryService) AddToQueue(trackID string) error {
	if _, err := s.trackByID(trackID); err != nil {
		return err
	}
	s.mu.Lock()
	s.queue.add(trackID)
	s.mu.Unlock()
	return nil
}

func (s *LibraryService) Queue() ([]library.Track, error) {
	s.mu.Lock()
	ids := append([]string(nil), s.queue.ids...)
	s.mu.Unlock()
	return s.store.TracksByIDs(context.Background(), ids)
}

func (s *LibraryService) PlayTrack(trackID string) error {
	track, err := s.trackByID(trackID)
	if err != nil {
		return err
	}
	if !track.Available {
		return fmt.Errorf("no se encuentra el archivo: %s", track.Path)
	}
	s.mu.Lock()
	queued := s.queue.index(trackID) >= 0
	s.mu.Unlock()
	if !queued {
		// Without a queue from the interface, play through the whole library.
		tracks, _ := s.store.Tracks(context.Background(), "")
		ids := make([]string, 0, len(tracks))
		for _, item := range tracks {
			if item.Available {
				ids = append(ids, item.ID)
			}
		}
		s.mu.Lock()
		s.queue.set(ids, queueIndex(ids, trackID))
		s.mu.Unlock()
	}
	s.mu.Lock()
	if index := s.queue.index(trackID); index >= 0 {
		s.queue.at = index
	}
	s.mu.Unlock()
	if err := s.player.Play(track.Path, 0); err != nil {
		return err
	}
	s.emitPlayback()
	return nil
}

func queueIndex(queue []string, trackID string) int {
	for index, id := range queue {
		if id == trackID {
			return index
		}
	}
	return -1
}

func (s *LibraryService) TogglePlayback() PlaybackState {
	s.player.Toggle()
	s.emitPlayback()
	return s.player.State()
}

func (s *LibraryService) NextTrack() error { return s.stepQueue(1, false) }

func (s *LibraryService) PreviousTrack() error {
	state := s.player.State()
	if state.PositionMs > 3000 {
		s.player.SeekTo(0)
		s.emitPlayback()
		return nil
	}
	return s.stepQueue(-1, false)
}

// stepQueue plays the next playable song in direction (1 or -1). ended means
// the song finished on its own, which is when repeat-one plays it again.
func (s *LibraryService) stepQueue(direction int, ended bool) error {
	s.mu.Lock()
	if len(s.queue.ids) == 0 {
		s.mu.Unlock()
		return fmt.Errorf("la cola está vacía")
	}
	start := s.queue.at
	for _, index := range s.queue.order(direction, ended) {
		if index >= len(s.queue.ids) {
			break // the queue was replaced while a song was starting
		}
		s.queue.at = index
		id := s.queue.ids[index]
		s.mu.Unlock()
		err := s.playTrackWithoutQueueChange(id)
		if err == nil {
			return nil
		}
		s.mu.Lock()
	}
	if start < len(s.queue.ids) {
		s.queue.at = start
	}
	s.mu.Unlock()
	return fmt.Errorf("no hay otra canción disponible en esta dirección de la cola")
}

// PlayMode says whether the queue is shuffled and how it repeats.
func (s *LibraryService) PlayMode() PlayMode {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.queue.mode
}

// SetShuffle mixes the songs after the current one, or puts them back in
// the order they were chosen in.
func (s *LibraryService) SetShuffle(on bool) PlayMode {
	s.mu.Lock()
	s.queue.setShuffle(on)
	mode := s.queue.mode
	s.mu.Unlock()
	_ = s.store.SetPreference(context.Background(), "shuffle", fmt.Sprint(on))
	return mode
}

// SetRepeat sets the repeat mode: "off", "all" or "one".
func (s *LibraryService) SetRepeat(repeat string) (PlayMode, error) {
	if repeat != RepeatOff && repeat != RepeatAll && repeat != RepeatOne {
		return s.PlayMode(), fmt.Errorf("modo de repetición desconocido: %q", repeat)
	}
	s.mu.Lock()
	s.queue.mode.Repeat = repeat
	mode := s.queue.mode
	s.mu.Unlock()
	_ = s.store.SetPreference(context.Background(), "repeat", repeat)
	return mode, nil
}

// loadPlayMode restores shuffle and repeat from the last session.
func (s *LibraryService) loadPlayMode(ctx context.Context) {
	shuffle, _ := s.store.Preference(ctx, "shuffle", "false")
	repeat, _ := s.store.Preference(ctx, "repeat", RepeatOff)
	s.mu.Lock()
	defer s.mu.Unlock()
	s.queue.setShuffle(shuffle == "true")
	if repeat == RepeatAll || repeat == RepeatOne {
		s.queue.mode.Repeat = repeat
	}
}

func (s *LibraryService) playTrackWithoutQueueChange(id string) error {
	track, err := s.trackByID(id)
	if err != nil {
		return err
	}
	if !track.Available {
		return fmt.Errorf("no se encuentra el archivo")
	}
	if err := s.player.Play(track.Path, 0); err != nil {
		return err
	}
	s.emitPlayback()
	return nil
}

// SeekTo moves playback to positionMs; the interface calls it when the
// listener releases the position slider.
func (s *LibraryService) SeekTo(positionMs int64) PlaybackState {
	s.player.SeekTo(positionMs)
	s.emitPlayback()
	return s.player.State()
}

func (s *LibraryService) SetVolume(value int) PlaybackState {
	s.player.SetVolume(value)
	_ = s.store.SetPreference(context.Background(), "volume", fmt.Sprint(value))
	s.emitPlayback()
	return s.player.State()
}

// SetAudioOptions applies the output mode and output device. A new
// mode and device are opened together; the player explains any fallback in
// the returned state's Error.
func (s *LibraryService) SetAudioOptions(mode, deviceID string) PlaybackState {
	before := s.player.State()
	if mode != before.OutputMode || deviceID != before.DeviceID {
		_, _, _ = s.player.SetOutput(mode, deviceID)
	}
	after := s.player.State()
	ctx := context.Background()
	_ = s.store.SetPreference(ctx, "outputMode", after.OutputMode)
	_ = s.store.SetPreference(ctx, "outputDevice", after.DeviceID)
	s.emitPlayback()
	return after
}

// SetEqualizer turns the equalizer on or off and sets its ten bands (dB).
// The interface calls it while a slider moves, and the change is heard at once.
func (s *LibraryService) SetEqualizer(enabled bool, bands []float64) PlaybackState {
	s.player.SetEqualizer(enabled, bands)
	state := s.player.State()
	s.saveEqualizer(context.Background(), state.EqualizerOn, state.Equalizer)
	s.emitPlayback()
	return state
}

func (s *LibraryService) saveEqualizer(ctx context.Context, enabled bool, bands []float64) {
	_ = s.store.SetPreference(ctx, "equalizerEnabled", fmt.Sprint(enabled))
	if encoded, err := json.Marshal(bands); err == nil {
		_ = s.store.SetPreference(ctx, "equalizerBands", string(encoded))
	}
}

// loadEqualizer restores the equalizer of the last session. Versions up to
// 1.0.2 saved three bands; those become the ten bands with the same curve.
func (s *LibraryService) loadEqualizer(ctx context.Context) {
	raw, _ := s.store.Preference(ctx, "equalizerBands", "[]")
	enabledRaw, _ := s.store.Preference(ctx, "equalizerEnabled", "false")
	var bands []float64
	if json.Unmarshal([]byte(raw), &bands) != nil {
		bands = nil
	}
	upgraded := len(bands) == 3
	if upgraded {
		converted := legacyEqualizerBands([3]float64{bands[0], bands[1], bands[2]})
		bands = converted[:]
	}
	s.player.SetEqualizer(enabledRaw == "true", bands)
	if upgraded {
		state := s.player.State()
		s.saveEqualizer(ctx, state.EqualizerOn, state.Equalizer)
	}
}

func (s *LibraryService) SaveTheme(theme Theme) (ThemeSaveResult, error) {
	saved, err := s.store.SaveTheme(context.Background(), theme)
	if err != nil {
		return ThemeSaveResult{}, err
	}
	themes, err := s.store.Themes(context.Background())
	if err != nil {
		return ThemeSaveResult{}, err
	}
	return ThemeSaveResult{Theme: saved, Themes: themes}, nil
}

func (s *LibraryService) ApplyTheme(themeID string) error {
	themes, err := s.store.Themes(context.Background())
	if err != nil {
		return err
	}
	for _, theme := range themes {
		if theme.ID == themeID {
			return s.store.SetPreference(context.Background(), "activeTheme", themeID)
		}
	}
	return fmt.Errorf("no se encontró el tema")
}

func (s *LibraryService) trackByID(trackID string) (library.Track, error) {
	track, err := s.store.Track(context.Background(), trackID)
	if errors.Is(err, sql.ErrNoRows) {
		return library.Track{}, fmt.Errorf("canción no encontrada")
	}
	if err != nil {
		return library.Track{}, err
	}
	if _, err := os.Stat(track.Path); err != nil {
		track.Available = false
	}
	return track, nil
}

func (s *LibraryService) emitPlayback() {
	if s.app != nil {
		s.app.Event.Emit("player:state", s.player.State())
	}
}

func (s *LibraryService) playbackEvents(ctx context.Context) {
	ticker := time.NewTicker(250 * time.Millisecond)
	defer ticker.Stop()
	deviceTicker := time.NewTicker(3 * time.Second)
	defer deviceTicker.Stop()
	lastDeviceSignature := audioDeviceSignature(s.player.Devices())
	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			if s.player.TakeEnded() {
				if err := s.stepQueue(1, true); err != nil {
					s.emitPlayback()
				}
			}
			if s.player.State().IsPlaying {
				s.emitPlayback()
			}
		case <-deviceTicker.C:
			devices := s.player.Devices()
			signature := audioDeviceSignature(devices)
			refreshDefault := false
			if signature != lastDeviceSignature {
				lastDeviceSignature = signature
				if s.app != nil {
					s.app.Event.Emit("audio:devices", AudioDevicesChanged{Devices: devices})
				}
				state := s.player.State()
				if state.DeviceID != "" {
					found := false
					for _, device := range devices {
						if device.ID == state.DeviceID {
							found = true
							break
						}
					}
					if !found {
						_, _, _ = s.player.SetOutput(state.OutputMode, "")
						_ = s.store.SetPreference(context.Background(), "outputDevice", "")
						s.emitPlayback()
					}
				}
				refreshDefault = state.DeviceID == ""
			}
			state := s.player.State()
			if refreshDefault || !state.OutputReady {
				actualMode, _, err := s.player.SetOutput(state.OutputMode, state.DeviceID)
				if err == nil {
					if actualMode != state.OutputMode {
						_ = s.store.SetPreference(context.Background(), "outputMode", actualMode)
					}
					s.emitPlayback()
				} else if nextState := s.player.State(); nextState.Error != state.Error {
					s.emitPlayback()
				}
			}
		}
	}
}

func audioDeviceSignature(devices []AudioDevice) string {
	var builder strings.Builder
	for _, device := range devices {
		fmt.Fprintf(&builder, "%s:%s:%t|", device.ID, device.Name, device.Default)
	}
	return builder.String()
}
