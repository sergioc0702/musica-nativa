package main

import (
	"context"
	"database/sql"
	"errors"
	"strconv"
	"time"
)

// listenedAfterMs is how long a song has to play to count as listened;
// shorter songs count at half their length.
const listenedAfterMs = 30_000

// playCounter decides when the song that is playing counts as listened.
// Starting the same song over (repeat, or going back to the start) counts again.
type playCounter struct {
	path    string
	counted bool
	lastMs  int64
}

// update takes the player's state, reported four times a second, and says
// whether this is the moment the song counts.
func (c *playCounter) update(path string, positionMs, durationMs int64, playing bool) bool {
	if path != c.path {
		c.path, c.counted = path, false
	}
	if c.counted && positionMs < 5000 && positionMs+1000 < c.lastMs {
		c.counted = false // it started over
	}
	c.lastMs = positionMs
	if c.counted || !playing || path == "" {
		return false
	}
	threshold := int64(listenedAfterMs)
	if durationMs > 0 && durationMs/2 < threshold {
		threshold = durationMs / 2
	}
	if positionMs < threshold {
		return false
	}
	c.counted = true
	return true
}

// TrackPlays is how many times a song was listened to.
type TrackPlays struct {
	TrackID string `json:"trackId"`
	Plays   int    `json:"plays"`
}

// ResumePoint is the song that was playing last and where it was.
type ResumePoint struct {
	TrackID    string `json:"trackId"`
	PositionMs int64  `json:"positionMs"`
}

// ListeningHistory is what the home page builds on.
type ListeningHistory struct {
	// Recent holds track IDs, the most recently listened first.
	Recent []string `json:"recent"`
	// Top holds the most listened songs, most plays first.
	Top []TrackPlays `json:"top"`
	// LastPlayed is when each listened song was last heard (Unix seconds).
	LastPlayed map[string]int64 `json:"lastPlayed"`
	Resume     *ResumePoint     `json:"resume,omitempty"`
}

// AddPlay records that a track was listened to at at.
func (s *Store) AddPlay(ctx context.Context, trackID string, at time.Time) error {
	_, err := s.db.ExecContext(ctx, `INSERT INTO plays(track_id, played_unix) VALUES(?, ?)`, trackID, at.Unix())
	return err
}

// ListeningHistory returns recent and most listened songs, without the resume point.
func (s *Store) ListeningHistory(ctx context.Context) (ListeningHistory, error) {
	history := ListeningHistory{Recent: []string{}, Top: []TrackPlays{}, LastPlayed: map[string]int64{}}
	rows, err := s.db.QueryContext(ctx, `SELECT track_id, MAX(played_unix) AS last FROM plays GROUP BY track_id ORDER BY last DESC, MAX(rowid) DESC`)
	if err != nil {
		return history, err
	}
	for rows.Next() {
		var id string
		var last int64
		if err := rows.Scan(&id, &last); err != nil {
			rows.Close()
			return history, err
		}
		history.LastPlayed[id] = last
		if len(history.Recent) < 60 {
			history.Recent = append(history.Recent, id)
		}
	}
	if err := rows.Close(); err != nil {
		return history, err
	}
	rows, err = s.db.QueryContext(ctx, `SELECT track_id, COUNT(*) AS plays FROM plays GROUP BY track_id ORDER BY plays DESC, MAX(played_unix) DESC LIMIT 20`)
	if err != nil {
		return history, err
	}
	defer rows.Close()
	for rows.Next() {
		var top TrackPlays
		if err := rows.Scan(&top.TrackID, &top.Plays); err != nil {
			return history, err
		}
		history.Top = append(history.Top, top)
	}
	return history, rows.Err()
}

// TrackIDByPath finds the catalog entry of a file.
func (s *Store) TrackIDByPath(ctx context.Context, path string) (string, error) {
	var id string
	err := s.db.QueryRowContext(ctx, `SELECT id FROM tracks WHERE path=?`, path).Scan(&id)
	return id, err
}

// trackListening runs with each playback report: it counts listened songs
// and, every few seconds, remembers where the song is to resume it later.
func (s *LibraryService) trackListening() {
	state := s.player.State()
	if state.Path == "" {
		return
	}
	ctx := context.Background()
	if s.plays.update(state.Path, state.PositionMs, state.DurationMs, state.IsPlaying) {
		if id, err := s.store.TrackIDByPath(ctx, state.Path); err == nil && s.store.AddPlay(ctx, id, time.Now()) == nil {
			s.emitHistory()
		}
	}
	if state.IsPlaying && time.Since(s.resumeSaved) >= 5*time.Second {
		s.resumeSaved = time.Now()
		s.saveResumePoint(state.Path, state.PositionMs, state.DurationMs)
	}
}

// saveResumePoint remembers the song at path and its position; a song that
// had nearly ended is resumed from the start.
func (s *LibraryService) saveResumePoint(path string, positionMs, durationMs int64) {
	ctx := context.Background()
	id, err := s.store.TrackIDByPath(ctx, path)
	if err != nil {
		return
	}
	if durationMs > 0 && positionMs > durationMs-10_000 {
		positionMs = 0
	}
	_ = s.store.SetPreference(ctx, "resumeTrack", id)
	_ = s.store.SetPreference(ctx, "resumePosition", strconv.FormatInt(positionMs, 10))
}

func (s *LibraryService) resumePoint(ctx context.Context) *ResumePoint {
	id, _ := s.store.Preference(ctx, "resumeTrack", "")
	if id == "" {
		return nil
	}
	if _, err := s.store.Track(ctx, id); errors.Is(err, sql.ErrNoRows) {
		return nil
	}
	raw, _ := s.store.Preference(ctx, "resumePosition", "0")
	position, _ := strconv.ParseInt(raw, 10, 64)
	return &ResumePoint{TrackID: id, PositionMs: position}
}

// ListeningHistory returns what was listened to, for the home page.
func (s *LibraryService) ListeningHistory() (ListeningHistory, error) {
	ctx := context.Background()
	history, err := s.store.ListeningHistory(ctx)
	if err != nil {
		return history, err
	}
	history.Resume = s.resumePoint(ctx)
	return history, nil
}

// ResumeListening plays the last song from where it was left.
func (s *LibraryService) ResumeListening() error {
	point := s.resumePoint(context.Background())
	if point == nil {
		return errors.New("no hay ninguna canción para seguir escuchando")
	}
	if err := s.PlayTrack(point.TrackID); err != nil {
		return err
	}
	if point.PositionMs > 0 {
		s.player.SeekTo(point.PositionMs)
		s.emitPlayback()
	}
	return nil
}

func (s *LibraryService) emitHistory() {
	if s.app == nil {
		return
	}
	if history, err := s.ListeningHistory(); err == nil {
		s.app.Event.Emit("history:changed", history)
	}
}
