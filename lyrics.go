package main

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"regexp"
	"sort"
	"strconv"
	"strings"
	"time"
	"unicode/utf8"

	"musica-nativa/internal/library"
)

// LyricWord is a word of a line with its own start time, from enhanced LRC
// (<mm:ss.xx> tags inside the line).
type LyricWord struct {
	TimeMs int64  `json:"timeMs"`
	Text   string `json:"text"`
}

// LyricLine is one line of lyrics. In synced lyrics TimeMs is when it is sung;
// an empty Text marks an instrumental pause.
type LyricLine struct {
	TimeMs int64       `json:"timeMs"`
	Text   string      `json:"text"`
	Words  []LyricWord `json:"words,omitempty"`
}

// Lyrics is what the lyrics view shows for a song.
type Lyrics struct {
	Found bool `json:"found"`
	// Source is where the lyrics came from: "manual" (added in the app),
	// "lrc" (a .lrc file next to the song), "tags" (inside the song file) or
	// "lrclib" (downloaded from lrclib.net).
	Source       string      `json:"source"`
	Synced       bool        `json:"synced"`
	Instrumental bool        `json:"instrumental"`
	Lines        []LyricLine `json:"lines"`
	// Text is the lyrics as stored, for editing.
	Text string `json:"text"`
	// Message explains why nothing was found or what went wrong.
	Message string `json:"message,omitempty"`
}

var (
	lrcTimeTag   = regexp.MustCompile(`^\[(\d{1,3}):(\d{1,2})(?:[.:](\d{1,3}))?\]`)
	lrcInfoTag   = regexp.MustCompile(`^\[([a-zA-Z#]+):([^\]]*)\]\s*$`)
	lrcWordTag   = regexp.MustCompile(`<(\d{1,3}):(\d{1,2})(?:[.:](\d{1,3}))?>`)
	anyTimestamp = regexp.MustCompile(`(?m)^\s*\[\d{1,3}:\d{1,2}(?:[.:]\d{1,3})?\]`)
)

// lrcMillis turns the parts of an LRC timestamp into milliseconds; the
// fraction may be tenths, hundredths or thousandths of a second.
func lrcMillis(minutes, seconds, fraction string) int64 {
	m, _ := strconv.ParseInt(minutes, 10, 64)
	s, _ := strconv.ParseInt(seconds, 10, 64)
	total := (m*60 + s) * 1000
	if fraction != "" {
		f, _ := strconv.ParseInt(fraction, 10, 64)
		switch len(fraction) {
		case 1:
			f *= 100
		case 2:
			f *= 10
		}
		total += f
	}
	return total
}

// parseLyrics reads LRC (synced) or plain lyrics. Synced lines come back in
// time order; plain lyrics keep their lines, blank ones between verses too.
func parseLyrics(text string) ([]LyricLine, bool) {
	text = strings.ReplaceAll(strings.ReplaceAll(text, "\r\n", "\n"), "\r", "\n")
	if !anyTimestamp.MatchString(text) {
		return plainLines(text), false
	}
	var offset int64
	var lines []LyricLine
	for _, raw := range strings.Split(text, "\n") {
		raw = strings.TrimSpace(raw)
		if info := lrcInfoTag.FindStringSubmatch(raw); info != nil && !lrcTimeTag.MatchString(raw) {
			if strings.EqualFold(info[1], "offset") {
				offset, _ = strconv.ParseInt(strings.TrimSpace(info[2]), 10, 64)
			}
			continue
		}
		var times []int64
		for {
			match := lrcTimeTag.FindStringSubmatch(raw)
			if match == nil {
				break
			}
			times = append(times, lrcMillis(match[1], match[2], match[3]))
			raw = raw[len(match[0]):]
		}
		if len(times) == 0 {
			continue // a line without a time cannot be placed
		}
		body, words := lineWords(strings.TrimSpace(raw))
		for _, at := range times {
			lines = append(lines, LyricLine{TimeMs: at, Text: body, Words: words})
		}
	}
	sort.SliceStable(lines, func(i, j int) bool { return lines[i].TimeMs < lines[j].TimeMs })
	for i := range lines {
		lines[i].TimeMs = max(lines[i].TimeMs-offset, 0)
		for j := range lines[i].Words {
			lines[i].Words[j].TimeMs = max(lines[i].Words[j].TimeMs-offset, 0)
		}
	}
	return lines, true
}

// lineWords splits a line with <mm:ss.xx> word tags into its words. Without
// tags the line has no word timings.
func lineWords(body string) (string, []LyricWord) {
	tags := lrcWordTag.FindAllStringSubmatchIndex(body, -1)
	if len(tags) == 0 {
		return body, nil
	}
	var words []LyricWord
	var text strings.Builder
	text.WriteString(body[:tags[0][0]])
	for i, tag := range tags {
		end := len(body)
		if i+1 < len(tags) {
			end = tags[i+1][0]
		}
		word := body[tag[1]:end]
		if word == "" {
			continue
		}
		at := lrcMillis(body[tag[2]:tag[3]], body[tag[4]:tag[5]], optionalGroup(body, tag[6], tag[7]))
		words = append(words, LyricWord{TimeMs: at, Text: word})
		text.WriteString(word)
	}
	return strings.TrimSpace(text.String()), trimLastWord(words)
}

func optionalGroup(text string, start, end int) string {
	if start < 0 {
		return ""
	}
	return text[start:end]
}

// trimLastWord drops the trailing space of the last word, so the words add
// up to the line's text.
func trimLastWord(words []LyricWord) []LyricWord {
	if len(words) > 0 {
		words[len(words)-1].Text = strings.TrimRight(words[len(words)-1].Text, " \t")
	}
	return words
}

func plainLines(text string) []LyricLine {
	raw := strings.Split(strings.Trim(text, "\n \t"), "\n")
	lines := make([]LyricLine, 0, len(raw))
	for _, line := range raw {
		lines = append(lines, LyricLine{Text: strings.TrimSpace(line)})
	}
	if len(lines) == 1 && lines[0].Text == "" {
		return nil
	}
	return lines
}

// lyricsRetryAfter is how long LRCLIB is not asked again for a song it had
// no lyrics for. "Buscar de nuevo" asks right away.
const lyricsRetryAfter = 7 * 24 * time.Hour

const (
	lyricsNotFound = "No encontramos la letra de esta canción."
	lyricsOffline  = "La búsqueda de letras en internet está apagada en Ajustes."
	lyricsNoLink   = "No se pudo buscar la letra en internet: revisa tu conexión."
)

func lyricsFrom(source, text string, instrumental bool) Lyrics {
	lines, synced := parseLyrics(text)
	if lines == nil {
		lines = []LyricLine{} // the interface reads it as a list, never as null
	}
	return Lyrics{Found: len(lines) > 0 || instrumental, Source: source, Synced: synced, Instrumental: instrumental, Lines: lines, Text: text}
}

func notFound(local Lyrics, message string) Lyrics {
	if local.Found {
		return local
	}
	return Lyrics{Lines: []LyricLine{}, Message: message}
}

// readSidecarLyrics reads the .lrc file next to a song (same name), in
// UTF-8 or, as older files often are, in Latin-1.
func readSidecarLyrics(songPath string) string {
	base := strings.TrimSuffix(songPath, filepath.Ext(songPath))
	for _, path := range []string{base + ".lrc", base + ".LRC"} {
		data, err := os.ReadFile(path)
		if err != nil {
			continue
		}
		data = bytes.TrimPrefix(data, []byte("\xef\xbb\xbf"))
		if utf8.Valid(data) {
			return string(data)
		}
		runes := make([]rune, len(data))
		for i, b := range data {
			runes[i] = rune(b)
		}
		return string(runes)
	}
	return ""
}

// Lyrics finds the lyrics of a track. Lyrics added in the app come first,
// then the song's own (.lrc next to it, then its tags), then LRCLIB. Lyrics
// without times are only used when nothing synced turns up.
func (s *LibraryService) Lyrics(trackID string) (Lyrics, error) {
	ctx := context.Background()
	track, err := s.trackByID(trackID)
	if err != nil {
		return Lyrics{}, err
	}
	stored, hasStored, err := s.store.StoredLyrics(ctx, trackID)
	if err != nil {
		return Lyrics{}, err
	}
	if hasStored && stored.Source == "manual" {
		return lyricsFrom("manual", stored.Text, stored.Instrumental), nil
	}
	var local Lyrics
	for _, own := range []struct{ source, text string }{
		{"lrc", readSidecarLyrics(track.Path)},
		{"tags", library.ReadLyrics(track.Path)},
	} {
		if strings.TrimSpace(own.text) == "" {
			continue
		}
		lyrics := lyricsFrom(own.source, own.text, false)
		if lyrics.Synced {
			return lyrics, nil
		}
		if !local.Found {
			local = lyrics
		}
	}
	if hasStored && stored.Source == "lrclib" {
		lyrics := lyricsFrom("lrclib", stored.Text, stored.Instrumental)
		if lyrics.Synced || lyrics.Instrumental || !local.Found {
			return lyrics, nil
		}
		return local, nil
	}
	if hasStored && stored.Source == "none" && time.Since(stored.Checked) < lyricsRetryAfter {
		return notFound(local, lyricsNotFound), nil
	}
	if !s.LyricsOnline() {
		return notFound(local, lyricsOffline), nil
	}
	record, ok, err := s.lrclib.find(ctx, track)
	if err != nil {
		return notFound(local, lyricsNoLink), nil
	}
	if !ok {
		_ = s.store.SaveStoredLyrics(ctx, trackID, storedLyrics{Source: "none", Checked: time.Now()})
		return notFound(local, lyricsNotFound), nil
	}
	text := record.SyncedLyrics
	if strings.TrimSpace(text) == "" {
		text = record.PlainLyrics
	}
	_ = s.store.SaveStoredLyrics(ctx, trackID, storedLyrics{Source: "lrclib", Text: text, Instrumental: record.Instrumental, Checked: time.Now()})
	lyrics := lyricsFrom("lrclib", text, record.Instrumental)
	if !lyrics.Synced && !lyrics.Instrumental && local.Found {
		return local, nil
	}
	return lyrics, nil
}

// SaveLyrics keeps lyrics typed or loaded in the app for a track, plain or
// LRC. Empty text removes them, and the track's other lyrics come back.
func (s *LibraryService) SaveLyrics(trackID, text string) (Lyrics, error) {
	ctx := context.Background()
	if strings.TrimSpace(text) == "" {
		if err := s.store.DeleteStoredLyrics(ctx, trackID, "manual"); err != nil {
			return Lyrics{}, err
		}
		return s.Lyrics(trackID)
	}
	if _, err := s.trackByID(trackID); err != nil {
		return Lyrics{}, err
	}
	if err := s.store.SaveStoredLyrics(ctx, trackID, storedLyrics{Source: "manual", Text: text, Checked: time.Now()}); err != nil {
		return Lyrics{}, err
	}
	return lyricsFrom("manual", text, false), nil
}

// SearchLyricsAgain forgets what LRCLIB answered before and asks again.
func (s *LibraryService) SearchLyricsAgain(trackID string) (Lyrics, error) {
	if err := s.store.DeleteStoredLyrics(context.Background(), trackID, "lrclib", "none"); err != nil {
		return Lyrics{}, err
	}
	return s.Lyrics(trackID)
}

// LyricsOnline says whether lyrics are looked up on LRCLIB; it is on unless
// turned off in Ajustes.
func (s *LibraryService) LyricsOnline() bool {
	value, _ := s.store.Preference(context.Background(), "lyricsOnline", "true")
	return value != "false"
}

func (s *LibraryService) SetLyricsOnline(on bool) {
	_ = s.store.SetPreference(context.Background(), "lyricsOnline", strconv.FormatBool(on))
}
