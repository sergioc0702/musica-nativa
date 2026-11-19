package main

import (
	"context"
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/url"
	"regexp"
	"strings"
	"time"
	"unicode"

	"musica-nativa/internal/library"
)

// lrclibURL is the public, free lyrics database at lrclib.net.
const lrclibURL = "https://lrclib.net"

// LRCLIB asks apps to name themselves in the User-Agent.
var lrclibUserAgent = "Musica Nativa/" + appVersion + " (https://github.com/sergioc0702/musica-nativa)"

// lrclibRecord is a song as LRCLIB returns it; missing lyrics arrive as "".
type lrclibRecord struct {
	TrackName    string  `json:"trackName"`
	ArtistName   string  `json:"artistName"`
	AlbumName    string  `json:"albumName"`
	Duration     float64 `json:"duration"`
	Instrumental bool    `json:"instrumental"`
	PlainLyrics  string  `json:"plainLyrics"`
	SyncedLyrics string  `json:"syncedLyrics"`
}

func (r lrclibRecord) synced() bool { return strings.TrimSpace(r.SyncedLyrics) != "" }

func (r lrclibRecord) hasLyrics() bool {
	return r.Instrumental || r.synced() || strings.TrimSpace(r.PlainLyrics) != ""
}

type lrclibClient struct {
	baseURL string
	client  *http.Client
}

func newLRCLIBClient(baseURL string) *lrclibClient {
	return &lrclibClient{baseURL: strings.TrimRight(baseURL, "/"), client: &http.Client{Timeout: 8 * time.Second}}
}

var (
	editionWords   = `(?i)(\bremaster(ed)?\b|\blive\b|\ben vivo\b|\bao vivo\b|\bversion\b|\bversi[oó]n\b|\bedit\b|\bradio\b|\bmono\b|\bstereo\b|\bdeluxe\b|\bbonus\b|\bfeat\b|\bft\b|\bfeaturing\b|\bcon\b|\bwith\b|\bacoustic\b|\bac[uú]stic[oa]\b)`
	editionNote    = regexp.MustCompile(editionWords)
	bracketed      = regexp.MustCompile(`\s*[\(\[]([^\)\]]*)[\)\]]`)
	dashSuffix     = regexp.MustCompile(`\s+-\s+(.+)$`)
	featuringTail  = regexp.MustCompile(`(?i)\s+(feat\.?|ft\.|featuring)\s+.*$`)
	artistJoins    = regexp.MustCompile(`(?i)\s*(,|;|\s+feat\.?\s+|\s+ft\.?\s+|\s+featuring\s+|\s+x\s+)`)
	accentReplacer = strings.NewReplacer(
		"á", "a", "à", "a", "â", "a", "ä", "a", "ã", "a", "å", "a",
		"é", "e", "è", "e", "ê", "e", "ë", "e",
		"í", "i", "ì", "i", "î", "i", "ï", "i",
		"ó", "o", "ò", "o", "ô", "o", "ö", "o", "õ", "o",
		"ú", "u", "ù", "u", "û", "u", "ü", "u",
		"ñ", "n", "ç", "c",
	)
)

// cleanTitle drops what a file title adds to the song's name: editions such
// as "(Remastered 2011)" or "- Live", and featured artists.
func cleanTitle(title string) string {
	cleaned := bracketed.ReplaceAllStringFunc(title, func(group string) string {
		if editionNote.MatchString(group) {
			return ""
		}
		return group
	})
	if match := dashSuffix.FindStringSubmatch(cleaned); match != nil && editionNote.MatchString(match[1]) {
		cleaned = strings.TrimSuffix(cleaned, match[0])
	}
	cleaned = featuringTail.ReplaceAllString(cleaned, "")
	if cleaned = strings.TrimSpace(cleaned); cleaned == "" {
		return strings.TrimSpace(title)
	}
	return cleaned
}

// mainArtist is the first of several credited artists. "Artista desconocido",
// what the library shows without tags, is no artist at all.
func mainArtist(artist string) string {
	artist = strings.TrimSpace(artist)
	if strings.EqualFold(artist, "Artista desconocido") {
		return ""
	}
	return strings.TrimSpace(artistJoins.Split(artist, 2)[0])
}

// normalizeName compares names without accents, case or punctuation.
func normalizeName(name string) string {
	name = accentReplacer.Replace(strings.ToLower(name))
	var out strings.Builder
	space := false
	for _, r := range name {
		if unicode.IsLetter(r) || unicode.IsDigit(r) {
			if space && out.Len() > 0 {
				out.WriteByte(' ')
			}
			out.WriteRune(r)
			space = false
		} else {
			space = true
		}
	}
	return out.String()
}

// songNames returns the title and artist to look for. A file without tags
// is often named "Artist - Title".
func songNames(track library.Track) (title, artist string) {
	title, artist = cleanTitle(track.Title), mainArtist(track.Artist)
	if artist == "" {
		if parts := strings.SplitN(track.Title, " - ", 2); len(parts) == 2 {
			return cleanTitle(parts[1]), mainArtist(parts[0])
		}
	}
	return title, artist
}

// bestMatch picks the record that is the same song as track: same title and
// artist, and about as long. Synced lyrics win, then the closest length.
func bestMatch(records []lrclibRecord, track library.Track) (lrclibRecord, bool) {
	title, artist := songNames(track)
	wantTitle, wantArtist := normalizeName(title), normalizeName(artist)
	best, found := lrclibRecord{}, false
	bestDiff := math.Inf(1)
	for _, record := range records {
		if !record.hasLyrics() || normalizeName(cleanTitle(record.TrackName)) != wantTitle {
			continue
		}
		if have := normalizeName(record.ArtistName); wantArtist != "" && !strings.Contains(have, wantArtist) && !strings.Contains(wantArtist, have) {
			continue
		}
		diff := 0.0
		if track.DurationSeconds > 0 && record.Duration > 0 {
			if diff = math.Abs(record.Duration - float64(track.DurationSeconds)); diff > 5 {
				continue
			}
		}
		better := !found || (record.synced() && !best.synced()) || (record.synced() == best.synced() && diff < bestDiff)
		if better {
			best, bestDiff, found = record, diff, true
		}
	}
	return best, found
}

// find looks track up: first exactly (as LRCLIB matches by name and length),
// then by searching with a cleaned title. ok is false when LRCLIB has nothing.
// Synced lyrics are preferred; plain lyrics are kept in case nothing better turns up.
func (c *lrclibClient) find(ctx context.Context, track library.Track) (lrclibRecord, bool, error) {
	title, artist := songNames(track)
	var fallback lrclibRecord
	consider := func(record lrclibRecord) bool {
		if record.synced() || record.Instrumental {
			return true
		}
		if !fallback.hasLyrics() && record.hasLyrics() {
			fallback = record
		}
		return false
	}
	if track.DurationSeconds > 0 && artist != "" {
		tried := map[string]bool{}
		for _, names := range [][2]string{{track.Title, track.Artist}, {title, artist}} {
			if key := names[0] + "\x00" + names[1]; tried[key] {
				continue
			} else {
				tried[key] = true
			}
			var record lrclibRecord
			ok, err := c.request(ctx, "/api/get", url.Values{
				"track_name": {names[0]}, "artist_name": {names[1]},
				"album_name": {track.Album}, "duration": {fmt.Sprint(track.DurationSeconds)},
			}, &record)
			if err != nil {
				return lrclibRecord{}, false, err
			}
			if ok && consider(record) {
				return record, true, nil
			}
		}
	}
	searches := []url.Values{{"q": {strings.TrimSpace(title + " " + artist)}}}
	if artist != "" {
		searches = append([]url.Values{{"track_name": {title}, "artist_name": {artist}}}, searches...)
	}
	for _, query := range searches {
		var records []lrclibRecord
		ok, err := c.request(ctx, "/api/search", query, &records)
		if err != nil {
			return lrclibRecord{}, false, err
		}
		if !ok {
			continue
		}
		if record, found := bestMatch(records, track); found && consider(record) {
			return record, true, nil
		}
	}
	return fallback, fallback.hasLyrics(), nil
}

// request gets path from LRCLIB into into. It returns false for "not found".
func (c *lrclibClient) request(ctx context.Context, path string, query url.Values, into any) (bool, error) {
	request, err := http.NewRequestWithContext(ctx, http.MethodGet, c.baseURL+path+"?"+query.Encode(), nil)
	if err != nil {
		return false, err
	}
	request.Header.Set("User-Agent", lrclibUserAgent)
	response, err := c.client.Do(request)
	if err != nil {
		return false, err
	}
	defer response.Body.Close()
	switch response.StatusCode {
	case http.StatusOK:
	case http.StatusNotFound:
		return false, nil
	default:
		return false, fmt.Errorf("LRCLIB respondió %s", response.Status)
	}
	if err := json.NewDecoder(io.LimitReader(response.Body, 4<<20)).Decode(into); err != nil {
		return false, fmt.Errorf("LRCLIB envió una respuesta que no se entiende: %w", err)
	}
	return true, nil
}
