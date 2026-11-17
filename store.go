package main

import (
	"context"
	"database/sql"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/google/uuid"
	_ "modernc.org/sqlite"
	"musica-nativa/internal/library"
)

type Theme struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	Background string `json:"background"`
	Accent     string `json:"accent"`
	Surface    string `json:"surface"`
	Text       string `json:"text"`
	Muted      string `json:"muted"`
	// BackgroundImage is an /artwork/ URL when read. When saving it may also be
	// a data: URL with a new picture, or empty to remove the picture.
	BackgroundImage string `json:"backgroundImage,omitempty"`
	BuiltIn         bool   `json:"builtIn"`
}

type Playlist struct {
	ID         string `json:"id"`
	Name       string `json:"name"`
	TrackCount int    `json:"trackCount"`
	CreatedAt  int64  `json:"createdAt"`
}

// Store keeps the catalog in SQLite. It uses a single connection, so a query's
// rows must be closed before the next statement runs.
type Store struct {
	db *sql.DB
}

func OpenStore(path string) (*Store, error) {
	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return nil, err
	}
	db, err := sql.Open("sqlite", path)
	if err != nil {
		return nil, err
	}
	db.SetMaxOpenConns(1)
	store := &Store{db: db}
	if err := store.migrate(context.Background()); err != nil {
		db.Close()
		return nil, err
	}
	return store, nil
}

func (s *Store) Close() error { return s.db.Close() }

func (s *Store) migrate(ctx context.Context) error {
	statements := []string{
		`PRAGMA foreign_keys = ON`,
		`CREATE TABLE IF NOT EXISTS tracks (
			id TEXT PRIMARY KEY, path TEXT NOT NULL UNIQUE, title TEXT NOT NULL,
			artist TEXT NOT NULL, album TEXT NOT NULL, album_artist TEXT NOT NULL,
			genre TEXT NOT NULL DEFAULT '', year INTEGER NOT NULL DEFAULT 0,
			track_number INTEGER NOT NULL DEFAULT 0, duration_seconds INTEGER NOT NULL DEFAULT 0,
			format TEXT NOT NULL, size_bytes INTEGER NOT NULL DEFAULT 0, modified_unix INTEGER NOT NULL DEFAULT 0,
			cover_data_url TEXT NOT NULL DEFAULT '', available INTEGER NOT NULL DEFAULT 1,
			favorite INTEGER NOT NULL DEFAULT 0, cover_id TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE INDEX IF NOT EXISTS tracks_title_idx ON tracks(title COLLATE NOCASE)`,
		`CREATE INDEX IF NOT EXISTS tracks_artist_idx ON tracks(artist COLLATE NOCASE)`,
		`CREATE TABLE IF NOT EXISTS artwork (id TEXT PRIMARY KEY, mime TEXT NOT NULL, data BLOB NOT NULL)`,
		`CREATE TABLE IF NOT EXISTS folders (path TEXT PRIMARY KEY)`,
		`CREATE TABLE IF NOT EXISTS playlists (
			id TEXT PRIMARY KEY, name TEXT NOT NULL, created_at INTEGER NOT NULL
		)`,
		`CREATE TABLE IF NOT EXISTS playlist_tracks (
			playlist_id TEXT NOT NULL REFERENCES playlists(id) ON DELETE CASCADE,
			track_id TEXT NOT NULL REFERENCES tracks(id) ON DELETE CASCADE,
			position INTEGER NOT NULL,
			PRIMARY KEY (playlist_id, track_id)
		)`,
		`CREATE TABLE IF NOT EXISTS themes (
			id TEXT PRIMARY KEY, name TEXT NOT NULL, background TEXT NOT NULL, accent TEXT NOT NULL,
			surface TEXT NOT NULL, text_color TEXT NOT NULL, muted TEXT NOT NULL,
			background_image TEXT NOT NULL DEFAULT '', built_in INTEGER NOT NULL DEFAULT 0,
			background_image_id TEXT NOT NULL DEFAULT ''
		)`,
		`CREATE TABLE IF NOT EXISTS preferences (key TEXT PRIMARY KEY, value TEXT NOT NULL)`,
		`INSERT OR IGNORE INTO preferences(key, value) VALUES ('activeTheme', 'nocturno')`,
		`INSERT OR IGNORE INTO preferences(key, value) VALUES ('volume', '72')`,
		`INSERT OR IGNORE INTO preferences(key, value) VALUES ('outputMode', 'shared')`,
		`INSERT OR IGNORE INTO preferences(key, value) VALUES ('equalizerEnabled', 'false')`,
		`INSERT OR IGNORE INTO preferences(key, value) VALUES ('equalizerBands', '[0,0,0]')`,
	}
	for _, statement := range statements {
		if _, err := s.db.ExecContext(ctx, statement); err != nil {
			return fmt.Errorf("database migration: %w", err)
		}
	}
	// Databases created by version 1.0.0 lack the picture references.
	if err := s.ensureColumn(ctx, "tracks", "cover_id", `TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	if err := s.ensureColumn(ctx, "themes", "background_image_id", `TEXT NOT NULL DEFAULT ''`); err != nil {
		return err
	}
	for _, theme := range builtInThemes() {
		if err := s.saveBuiltInTheme(ctx, theme); err != nil {
			return err
		}
	}
	return s.moveInlineImages(ctx)
}

func (s *Store) ensureColumn(ctx context.Context, table, column, definition string) error {
	rows, err := s.db.QueryContext(ctx, `SELECT name FROM pragma_table_info(?)`, table)
	if err != nil {
		return err
	}
	found := false
	for rows.Next() {
		var name string
		if err := rows.Scan(&name); err != nil {
			rows.Close()
			return err
		}
		found = found || name == column
	}
	if err := rows.Close(); err != nil {
		return err
	}
	if found {
		return nil
	}
	if _, err := s.db.ExecContext(ctx, `ALTER TABLE `+table+` ADD COLUMN `+column+` `+definition); err != nil {
		return fmt.Errorf("database migration: %w", err)
	}
	return nil
}

// moveInlineImages takes the pictures that version 1.0.0 stored as data URLs
// inside track and theme rows out of those rows.
func (s *Store) moveInlineImages(ctx context.Context) error {
	// Track covers come from the music files: forgetting the modification time
	// makes the next scan read those files again and store their covers.
	result, err := s.db.ExecContext(ctx, `UPDATE tracks SET cover_data_url='', modified_unix=0 WHERE cover_data_url<>''`)
	if err != nil {
		return fmt.Errorf("database migration: %w", err)
	}
	moved, _ := result.RowsAffected()

	// Theme backgrounds were chosen by the listener and only exist here.
	themeIDs, err := s.stringColumn(ctx, `SELECT id FROM themes WHERE background_image<>''`)
	if err != nil {
		return err
	}
	for _, id := range themeIDs {
		var dataURL string
		if err := s.db.QueryRowContext(ctx, `SELECT background_image FROM themes WHERE id=?`, id).Scan(&dataURL); err != nil {
			return err
		}
		imageID := ""
		if art, err := artworkFromDataURL(dataURL); err == nil {
			if err := s.SaveArtwork(ctx, art); err != nil {
				return err
			}
			imageID = art.ID
		}
		if _, err := s.db.ExecContext(ctx, `UPDATE themes SET background_image='', background_image_id=? WHERE id=?`, imageID, id); err != nil {
			return err
		}
		moved++
	}
	if moved > 0 {
		// Give back the disk space the inline pictures used.
		if _, err := s.db.ExecContext(ctx, `VACUUM`); err != nil {
			return fmt.Errorf("database migration: %w", err)
		}
	}
	return nil
}

func builtInThemes() []Theme {
	return []Theme{
		{ID: "nocturno", Name: "Nocturno", Background: "#11110f", Accent: "#e7a878", Surface: "#1b1b18", Text: "#f4f0e8", Muted: "#969188", BuiltIn: true},
		{ID: "aurora", Name: "Aurora", Background: "#101611", Accent: "#9fc8a8", Surface: "#1a211b", Text: "#eef3ec", Muted: "#919d91", BuiltIn: true},
		{ID: "cobalto", Name: "Cobalto", Background: "#101319", Accent: "#90aadb", Surface: "#1a1e27", Text: "#f0f1f4", Muted: "#9399a5", BuiltIn: true},
	}
}

func (s *Store) saveBuiltInTheme(ctx context.Context, theme Theme) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO themes(id,name,background,accent,surface,text_color,muted,background_image,built_in)
		VALUES(?,?,?,?,?,?,?,'',1)`, theme.ID, theme.Name, theme.Background, theme.Accent, theme.Surface, theme.Text, theme.Muted)
	return err
}

// SaveTheme creates or updates a listener theme and returns it as stored. A new
// theme gets its own identifier, so two themes may share a name.
func (s *Store) SaveTheme(ctx context.Context, theme Theme) (Theme, error) {
	theme.Name = strings.TrimSpace(theme.Name)
	if theme.Name == "" {
		return Theme{}, fmt.Errorf("el tema necesita un nombre")
	}
	if theme.ID == "" {
		theme.ID = "tema-" + uuid.NewString()
	}
	var newImage *library.Artwork
	imageID := ""
	switch image := strings.TrimSpace(theme.BackgroundImage); {
	case image == "":
	case strings.HasPrefix(image, "data:"):
		art, err := artworkFromDataURL(image)
		if err != nil {
			return Theme{}, err
		}
		newImage, imageID = &art, art.ID
	case strings.HasPrefix(image, artworkPathPrefix):
		imageID = strings.TrimPrefix(image, artworkPathPrefix)
	default:
		return Theme{}, fmt.Errorf("la imagen de fondo no es válida")
	}

	// One transaction, so the unused-artwork cleanup never sees the picture
	// without the theme that uses it.
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return Theme{}, err
	}
	defer tx.Rollback()
	var builtIn int
	err = tx.QueryRowContext(ctx, `SELECT built_in FROM themes WHERE id=?`, theme.ID).Scan(&builtIn)
	if err != nil && err != sql.ErrNoRows {
		return Theme{}, err
	}
	if builtIn == 1 {
		return Theme{}, fmt.Errorf("los temas incluidos no se pueden sobrescribir")
	}
	if newImage != nil {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO artwork(id,mime,data) VALUES(?,?,?)`, newImage.ID, newImage.MIME, newImage.Data); err != nil {
			return Theme{}, err
		}
	} else if imageID != "" {
		var exists int
		if err := tx.QueryRowContext(ctx, `SELECT 1 FROM artwork WHERE id=?`, imageID).Scan(&exists); err != nil {
			if err == sql.ErrNoRows {
				return Theme{}, fmt.Errorf("la imagen de fondo ya no está disponible; elige otra")
			}
			return Theme{}, err
		}
	}
	_, err = tx.ExecContext(ctx, `INSERT INTO themes(id,name,background,accent,surface,text_color,muted,background_image,background_image_id,built_in)
		VALUES(?,?,?,?,?,?,?,'',?,0) ON CONFLICT(id) DO UPDATE SET name=excluded.name,background=excluded.background,
		accent=excluded.accent,surface=excluded.surface,text_color=excluded.text_color,muted=excluded.muted,
		background_image='',background_image_id=excluded.background_image_id`, theme.ID, theme.Name, theme.Background,
		theme.Accent, theme.Surface, theme.Text, theme.Muted, imageID)
	if err != nil {
		return Theme{}, err
	}
	if err := tx.Commit(); err != nil {
		return Theme{}, err
	}
	theme.BackgroundImage = artworkURL(imageID)
	theme.BuiltIn = false
	return theme, nil
}

func (s *Store) Themes(ctx context.Context) ([]Theme, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT id,name,background,accent,surface,text_color,muted,background_image_id,built_in FROM themes ORDER BY built_in DESC,name COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Theme
	for rows.Next() {
		var theme Theme
		var imageID string
		var builtIn int
		if err := rows.Scan(&theme.ID, &theme.Name, &theme.Background, &theme.Accent, &theme.Surface, &theme.Text, &theme.Muted, &imageID, &builtIn); err != nil {
			return nil, err
		}
		theme.BackgroundImage = artworkURL(imageID)
		theme.BuiltIn = builtIn == 1
		result = append(result, theme)
	}
	return result, rows.Err()
}

// SaveArtwork stores a picture once; saving the same picture again does nothing.
func (s *Store) SaveArtwork(ctx context.Context, art library.Artwork) error {
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO artwork(id,mime,data) VALUES(?,?,?)`, art.ID, art.MIME, art.Data)
	return err
}

// Artwork returns a stored picture, or sql.ErrNoRows.
func (s *Store) Artwork(ctx context.Context, id string) (library.Artwork, error) {
	art := library.Artwork{ID: id}
	err := s.db.QueryRowContext(ctx, `SELECT mime,data FROM artwork WHERE id=?`, id).Scan(&art.MIME, &art.Data)
	return art, err
}

// RemoveUnusedArtwork deletes pictures that no track or theme references.
func (s *Store) RemoveUnusedArtwork(ctx context.Context) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM artwork
		WHERE id NOT IN (SELECT cover_id FROM tracks WHERE cover_id<>'')
		AND id NOT IN (SELECT background_image_id FROM themes WHERE background_image_id<>'')`)
	return err
}

func (s *Store) SetPreference(ctx context.Context, key, value string) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO preferences(key,value) VALUES(?,?) ON CONFLICT(key) DO UPDATE SET value=excluded.value`, key, value)
	return err
}

func (s *Store) Preference(ctx context.Context, key, fallback string) (string, error) {
	var value string
	err := s.db.QueryRowContext(ctx, `SELECT value FROM preferences WHERE key=?`, key).Scan(&value)
	if err == sql.ErrNoRows {
		return fallback, nil
	}
	return value, err
}

func (s *Store) ReplaceFolders(ctx context.Context, folders []string) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	if _, err := tx.ExecContext(ctx, `DELETE FROM folders`); err != nil {
		tx.Rollback()
		return err
	}
	for _, folder := range folders {
		if _, err := tx.ExecContext(ctx, `INSERT OR IGNORE INTO folders(path) VALUES(?)`, folder); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Folders(ctx context.Context) ([]string, error) {
	return s.stringColumn(ctx, `SELECT path FROM folders ORDER BY path COLLATE NOCASE`)
}

func (s *Store) stringColumn(ctx context.Context, query string, args ...any) ([]string, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []string
	for rows.Next() {
		var value string
		if err := rows.Scan(&value); err != nil {
			return nil, err
		}
		result = append(result, value)
	}
	return result, rows.Err()
}

func (s *Store) UpsertTracks(ctx context.Context, tracks []library.Track) error {
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	statement := `INSERT INTO tracks(id,path,title,artist,album,album_artist,genre,year,track_number,duration_seconds,format,size_bytes,modified_unix,cover_id,available)
		VALUES(?,?,?,?,?,?,?,?,?,?,?,?,?,?,1) ON CONFLICT(id) DO UPDATE SET path=excluded.path,title=excluded.title,
		artist=excluded.artist,album=excluded.album,album_artist=excluded.album_artist,genre=excluded.genre,year=excluded.year,
		track_number=excluded.track_number,duration_seconds=excluded.duration_seconds,format=excluded.format,size_bytes=excluded.size_bytes,
		modified_unix=excluded.modified_unix,cover_id=excluded.cover_id,available=1`
	for _, track := range tracks {
		_, err := tx.ExecContext(ctx, statement, track.ID, track.Path, track.Title, track.Artist, track.Album, track.AlbumArtist,
			track.Genre, track.Year, track.TrackNumber, track.DurationSeconds, track.Format, track.SizeBytes, track.ModifiedUnix,
			track.CoverID)
		if err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

const trackColumns = `id,path,title,artist,album,album_artist,genre,year,track_number,duration_seconds,format,size_bytes,modified_unix,cover_id,available,favorite`

type rowScanner interface{ Scan(dest ...any) error }

func scanTrack(row rowScanner) (library.Track, error) {
	var track library.Track
	var available, favorite int
	err := row.Scan(&track.ID, &track.Path, &track.Title, &track.Artist, &track.Album, &track.AlbumArtist, &track.Genre,
		&track.Year, &track.TrackNumber, &track.DurationSeconds, &track.Format, &track.SizeBytes, &track.ModifiedUnix,
		&track.CoverID, &available, &favorite)
	track.Available = available == 1
	track.Favorite = favorite == 1
	track.CoverURL = artworkURL(track.CoverID)
	return track, err
}

func (s *Store) queryTracks(ctx context.Context, query string, args ...any) ([]library.Track, error) {
	rows, err := s.db.QueryContext(ctx, query, args...)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []library.Track
	for rows.Next() {
		track, err := scanTrack(rows)
		if err != nil {
			return nil, err
		}
		result = append(result, track)
	}
	return result, rows.Err()
}

// Tracks lists the library: the tracks inside the music folders, optionally
// filtered by a search over title, artist and album.
func (s *Store) Tracks(ctx context.Context, query string) ([]library.Track, error) {
	folders, err := s.Folders(ctx)
	if err != nil {
		return nil, err
	}
	all, err := s.queryTracks(ctx, `SELECT `+trackColumns+` FROM tracks ORDER BY title COLLATE NOCASE`)
	if err != nil {
		return nil, err
	}
	roots := folderKeys(folders)
	needle := strings.ToLower(strings.TrimSpace(query))
	result := make([]library.Track, 0, len(all))
	for _, track := range all {
		if !insideAnyFolder(track.Path, roots) {
			continue
		}
		if needle != "" && !strings.Contains(strings.ToLower(track.Title+" "+track.Artist+" "+track.Album), needle) {
			continue
		}
		result = append(result, track)
	}
	return result, nil
}

// Track returns one catalog entry, inside the library or not, or sql.ErrNoRows.
func (s *Store) Track(ctx context.Context, id string) (library.Track, error) {
	return scanTrack(s.db.QueryRowContext(ctx, `SELECT `+trackColumns+` FROM tracks WHERE id=?`, id))
}

// TracksByIDs returns the catalog entries for ids, in the same order, skipping
// unknown ids.
func (s *Store) TracksByIDs(ctx context.Context, ids []string) ([]library.Track, error) {
	all, err := s.queryTracks(ctx, `SELECT `+trackColumns+` FROM tracks`)
	if err != nil {
		return nil, err
	}
	byID := make(map[string]library.Track, len(all))
	for _, track := range all {
		byID[track.ID] = track
	}
	result := make([]library.Track, 0, len(ids))
	for _, id := range ids {
		if track, ok := byID[id]; ok {
			result = append(result, track)
		}
	}
	return result, nil
}

// KnownTracks returns every catalog entry keyed by library.PathKey, so a scan
// can skip files that have not changed.
func (s *Store) KnownTracks(ctx context.Context) (map[string]library.Track, error) {
	all, err := s.queryTracks(ctx, `SELECT `+trackColumns+` FROM tracks`)
	if err != nil {
		return nil, err
	}
	known := make(map[string]library.Track, len(all))
	for _, track := range all {
		known[library.PathKey(track.Path)] = track
	}
	return known, nil
}

// RemoveTracksOutsideFolders deletes catalog entries that no music folder
// covers any more. Favorites and playlist entries are kept out of sight, so they
// come back if their folder is added again.
func (s *Store) RemoveTracksOutsideFolders(ctx context.Context) error {
	folders, err := s.Folders(ctx)
	if err != nil {
		return err
	}
	candidates, err := s.queryTracks(ctx, `SELECT `+trackColumns+` FROM tracks
		WHERE favorite=0 AND id NOT IN (SELECT track_id FROM playlist_tracks)`)
	if err != nil {
		return err
	}
	roots := folderKeys(folders)
	var outside []string
	for _, track := range candidates {
		if !insideAnyFolder(track.Path, roots) {
			outside = append(outside, track.ID)
		}
	}
	return s.updateTracks(ctx, `DELETE FROM tracks WHERE id=?`, outside)
}

func (s *Store) updateTracks(ctx context.Context, statement string, ids []string) error {
	if len(ids) == 0 {
		return nil
	}
	tx, err := s.db.BeginTx(ctx, nil)
	if err != nil {
		return err
	}
	for _, id := range ids {
		if _, err := tx.ExecContext(ctx, statement, id); err != nil {
			tx.Rollback()
			return err
		}
	}
	return tx.Commit()
}

// folderKeys turns music folders into comparable path prefixes.
func folderKeys(folders []string) []string {
	keys := make([]string, 0, len(folders))
	for _, folder := range folders {
		key := library.PathKey(folder)
		if !strings.HasSuffix(key, string(filepath.Separator)) {
			key += string(filepath.Separator)
		}
		keys = append(keys, key)
	}
	return keys
}

func insideAnyFolder(path string, roots []string) bool {
	key := library.PathKey(path)
	for _, root := range roots {
		if strings.HasPrefix(key, root) {
			return true
		}
	}
	return false
}

func (s *Store) ToggleFavorite(ctx context.Context, trackID string) error {
	result, err := s.db.ExecContext(ctx, `UPDATE tracks SET favorite=CASE favorite WHEN 1 THEN 0 ELSE 1 END WHERE id=?`, trackID)
	if err != nil {
		return err
	}
	count, _ := result.RowsAffected()
	if count == 0 {
		return sql.ErrNoRows
	}
	return nil
}

func (s *Store) CreatePlaylist(ctx context.Context, name string) (Playlist, error) {
	name = strings.TrimSpace(name)
	if name == "" {
		return Playlist{}, fmt.Errorf("el nombre de la playlist está vacío")
	}
	playlist := Playlist{ID: uuid.NewString(), Name: name, CreatedAt: time.Now().Unix()}
	_, err := s.db.ExecContext(ctx, `INSERT INTO playlists(id,name,created_at) VALUES(?,?,?)`, playlist.ID, playlist.Name, playlist.CreatedAt)
	return playlist, err
}

func (s *Store) Playlists(ctx context.Context) ([]Playlist, error) {
	rows, err := s.db.QueryContext(ctx, `SELECT p.id,p.name,p.created_at,COUNT(pt.track_id) FROM playlists p LEFT JOIN playlist_tracks pt ON pt.playlist_id=p.id GROUP BY p.id ORDER BY p.created_at DESC`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var result []Playlist
	for rows.Next() {
		var playlist Playlist
		if err := rows.Scan(&playlist.ID, &playlist.Name, &playlist.CreatedAt, &playlist.TrackCount); err != nil {
			return nil, err
		}
		result = append(result, playlist)
	}
	return result, rows.Err()
}

func (s *Store) AddTrackToPlaylist(ctx context.Context, playlistID, trackID string) error {
	var position int
	if err := s.db.QueryRowContext(ctx, `SELECT COALESCE(MAX(position),-1)+1 FROM playlist_tracks WHERE playlist_id=?`, playlistID).Scan(&position); err != nil {
		return err
	}
	_, err := s.db.ExecContext(ctx, `INSERT OR IGNORE INTO playlist_tracks(playlist_id,track_id,position) VALUES(?,?,?)`, playlistID, trackID, position)
	return err
}

func (s *Store) RemoveTrackFromPlaylist(ctx context.Context, playlistID, trackID string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM playlist_tracks WHERE playlist_id=? AND track_id=?`, playlistID, trackID)
	return err
}

func (s *Store) PlaylistTracks(ctx context.Context, playlistID string) ([]library.Track, error) {
	return s.queryTracks(ctx, `SELECT `+prefixColumns("t.", trackColumns)+` FROM playlist_tracks pt JOIN tracks t ON t.id=pt.track_id WHERE pt.playlist_id=? ORDER BY pt.position`, playlistID)
}

func prefixColumns(prefix, columns string) string {
	return prefix + strings.ReplaceAll(columns, ",", ","+prefix)
}

// MarkMissingUnderRoots flags tracks inside roots that a complete scan did not
// find. Tracks under unreachable folders are left alone.
func (s *Store) MarkMissingUnderRoots(ctx context.Context, roots []string, present map[string]struct{}) error {
	if len(roots) == 0 {
		return nil
	}
	candidates, err := s.queryTracks(ctx, `SELECT `+trackColumns+` FROM tracks WHERE available=1`)
	if err != nil {
		return err
	}
	keys := folderKeys(roots)
	var missing []string
	for _, track := range candidates {
		if _, ok := present[track.ID]; ok {
			continue
		}
		if insideAnyFolder(track.Path, keys) {
			missing = append(missing, track.ID)
		}
	}
	return s.updateTracks(ctx, `UPDATE tracks SET available=0 WHERE id=?`, missing)
}

func (s *Store) DeletePlaylist(ctx context.Context, id string) error {
	_, err := s.db.ExecContext(ctx, `DELETE FROM playlists WHERE id=?`, id)
	return err
}
