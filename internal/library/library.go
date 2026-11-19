package library

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"errors"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
	"strings"

	"github.com/dhowden/tag"
	"github.com/gopxl/beep"
	"github.com/gopxl/beep/flac"
	"github.com/gopxl/beep/mp3"
	"github.com/gopxl/beep/wav"
)

// MaxArtworkBytes caps the size of a single stored picture.
const MaxArtworkBytes = 10 * 1024 * 1024

// Track describes a local file. Paths remain references to the user's originals.
type Track struct {
	ID              string `json:"id"`
	Path            string `json:"path"`
	Title           string `json:"title"`
	Artist          string `json:"artist"`
	Album           string `json:"album"`
	AlbumArtist     string `json:"albumArtist"`
	Genre           string `json:"genre"`
	Year            int    `json:"year"`
	TrackNumber     int    `json:"trackNumber"`
	DiscNumber      int    `json:"discNumber"`
	DurationSeconds int    `json:"durationSeconds"`
	Format          string `json:"format"`
	SizeBytes       int64  `json:"sizeBytes"`
	ModifiedUnix    int64  `json:"modifiedUnix"`
	// CoverID references stored Artwork; CoverURL is where the interface loads it.
	CoverID   string `json:"-"`
	CoverURL  string `json:"coverUrl,omitempty"`
	Available bool   `json:"available"`
	Favorite  bool   `json:"favorite"`
}

// Artwork is a picture embedded in a music file or chosen for a theme. Its ID
// is derived from its bytes, so identical album art is stored only once.
type Artwork struct {
	ID   string
	MIME string
	Data []byte
}

// NewArtwork identifies picture data. It reports false when the bytes are
// empty, too large, or not a recognised image format.
func NewArtwork(data []byte) (Artwork, bool) {
	if len(data) == 0 || len(data) > MaxArtworkBytes {
		return Artwork{}, false
	}
	mime := http.DetectContentType(data)
	if !strings.HasPrefix(mime, "image/") {
		return Artwork{}, false
	}
	sum := sha256.Sum256(data)
	return Artwork{ID: hex.EncodeToString(sum[:]), MIME: mime, Data: data}, true
}

// ScanOptions configures ScanRoots.
type ScanOptions struct {
	// Known holds catalog entries keyed by PathKey. A file whose size and
	// modification time still match its entry is not read again.
	Known map[string]Track
	// OnTrack receives every track found, including reused known tracks.
	OnTrack func(Track)
	// OnArtwork stores each distinct picture read during the scan. Tracks only
	// reference artwork that OnArtwork accepted.
	OnArtwork func(Artwork) error
}

// PathKey identifies a file path independently of letter case, as Windows does.
func PathKey(path string) string {
	if !filepath.IsAbs(path) {
		if absolute, err := filepath.Abs(path); err == nil {
			path = absolute
		}
	}
	return strings.ToLower(filepath.Clean(path))
}

// Supported reports whether a path uses one of the formats in the first release.
func Supported(path string) bool {
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp3", ".wav", ".flac":
		return true
	default:
		return false
	}
}

// ScanRoots reads tags from supported files and reports each discovered track.
// A bad or unavailable root is returned as an issue while other roots continue.
func ScanRoots(ctx context.Context, roots []string, options ScanOptions) ([]Track, []error) {
	tracks := make([]Track, 0)
	issues := make([]error, 0)
	seen := make(map[string]struct{})
	storedArtwork := make(map[string]struct{})

	for _, root := range roots {
		root = filepath.Clean(strings.TrimSpace(root))
		if root == "." || root == "" {
			continue
		}
		rootInfo, err := os.Stat(root)
		if err != nil {
			issues = append(issues, fmt.Errorf("%s: %w", root, err))
			continue
		}
		if !rootInfo.IsDir() {
			issues = append(issues, fmt.Errorf("%s: no es una carpeta", root))
			continue
		}

		err = filepath.WalkDir(root, func(path string, entry fs.DirEntry, walkErr error) error {
			if ctx.Err() != nil {
				return ctx.Err()
			}
			if walkErr != nil {
				issues = append(issues, fmt.Errorf("%s: %w", path, walkErr))
				if entry != nil && entry.IsDir() {
					return filepath.SkipDir
				}
				return nil
			}
			if entry.IsDir() || !Supported(path) {
				return nil
			}
			key := PathKey(path)
			if _, ok := seen[key]; ok {
				return nil
			}
			info, err := os.Stat(path)
			if err != nil {
				issues = append(issues, fmt.Errorf("%s: %w", path, err))
				return nil
			}
			track, known := options.Known[key]
			if !known || track.SizeBytes != info.Size() || track.ModifiedUnix != info.ModTime().Unix() {
				var artwork *Artwork
				track, artwork, err = readTrack(path, info)
				if err != nil {
					issues = append(issues, fmt.Errorf("%s: %w", path, err))
					return nil
				}
				if artwork != nil {
					if _, ok := storedArtwork[artwork.ID]; !ok && options.OnArtwork != nil {
						if err := options.OnArtwork(*artwork); err != nil {
							issues = append(issues, fmt.Errorf("%s: no se pudo guardar la portada: %w", path, err))
						} else {
							storedArtwork[artwork.ID] = struct{}{}
						}
					}
					if _, ok := storedArtwork[artwork.ID]; ok {
						track.CoverID = artwork.ID
					}
				}
			}
			seen[key] = struct{}{}
			tracks = append(tracks, track)
			if options.OnTrack != nil {
				options.OnTrack(track)
			}
			return nil
		})
		if err != nil && !errors.Is(err, context.Canceled) && !errors.Is(err, context.DeadlineExceeded) {
			issues = append(issues, fmt.Errorf("%s: %w", root, err))
		}
	}

	return tracks, issues
}

func readTrack(path string, info os.FileInfo) (Track, *Artwork, error) {
	resolved, err := filepath.Abs(path)
	if err != nil {
		return Track{}, nil, err
	}
	resolved = filepath.Clean(resolved)
	pathHash := sha256.Sum256([]byte(strings.ToLower(resolved)))
	base := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	track := Track{
		ID:           hex.EncodeToString(pathHash[:]),
		Path:         resolved,
		Title:        base,
		Artist:       "Artista desconocido",
		Album:        "Sencillos",
		AlbumArtist:  "Artista desconocido",
		Format:       strings.TrimPrefix(strings.ToUpper(filepath.Ext(path)), "."),
		SizeBytes:    info.Size(),
		ModifiedUnix: info.ModTime().Unix(),
		Available:    true,
	}

	file, err := os.Open(path)
	if err != nil {
		return Track{}, nil, err
	}
	metadata, tagErr := tag.ReadFrom(file)
	_ = file.Close()
	var artwork *Artwork
	if tagErr == nil && metadata != nil {
		if value := strings.TrimSpace(metadata.Title()); value != "" {
			track.Title = value
		}
		if value := strings.TrimSpace(metadata.Artist()); value != "" {
			track.Artist = value
		}
		if value := strings.TrimSpace(metadata.Album()); value != "" {
			track.Album = value
		}
		if value := strings.TrimSpace(metadata.AlbumArtist()); value != "" {
			track.AlbumArtist = value
		} else {
			track.AlbumArtist = track.Artist
		}
		track.Genre = strings.TrimSpace(metadata.Genre())
		track.Year = metadata.Year()
		track.TrackNumber, _ = metadata.Track()
		track.DiscNumber, _ = metadata.Disc()
		if picture := metadata.Picture(); picture != nil {
			if art, ok := NewArtwork(picture.Data); ok {
				artwork = &art
			}
		}
	}
	track.DurationSeconds = probeDuration(resolved)
	return track, artwork, nil
}

func probeDuration(path string) int {
	file, err := os.Open(path)
	if err != nil {
		return 0
	}
	defer file.Close()

	var stream beep.StreamSeekCloser
	var format beep.Format
	switch strings.ToLower(filepath.Ext(path)) {
	case ".mp3":
		stream, format, err = mp3.Decode(file)
	case ".wav":
		stream, format, err = wav.Decode(file)
	case ".flac":
		stream, format, err = flac.Decode(file)
	default:
		return 0
	}
	if err != nil || stream == nil || format.SampleRate <= 0 {
		return 0
	}
	defer stream.Close()
	seconds := stream.Len() / int(format.SampleRate)
	if stream.Len()%int(format.SampleRate) > 0 {
		seconds++
	}
	return seconds
}

// ReadLyrics returns the lyrics stored in the tags of the file at path, or ""
// when it has none or cannot be read.
func ReadLyrics(path string) string {
	file, err := os.Open(path)
	if err != nil {
		return ""
	}
	defer file.Close()
	metadata, err := tag.ReadFrom(file)
	if err != nil {
		return ""
	}
	return strings.TrimSpace(metadata.Lyrics())
}
