package project

import (
	"database/sql"
	"encoding/json"
	"errors"
	"slices"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/google/uuid"
)

// Insertion contains only user-editable fields. The store assigns identity,
// evidence and locks; positions are continuous within each subtitle set.
type Insertion struct {
	Position            int
	Source, Translation string
	StartMs, EndMs      int64
}

func manualTrack(tx *sql.Tx, original bool) (table, track string, err error) {
	if !original {
		return "cues", "working", nil
	}
	var same bool
	err = tx.QueryRow("SELECT NOT EXISTS(SELECT id,body FROM cues EXCEPT SELECT id,body FROM source_cues) AND NOT EXISTS(SELECT id,body FROM source_cues EXCEPT SELECT id,body FROM cues)").Scan(&same)
	track = "original"
	if same {
		track = "both"
	}
	return "source_cues", track, err
}

func orderedCues(tx *sql.Tx, table string) ([]subtitle.Cue, error) {
	rows, err := tx.Query("SELECT body FROM " + table + " ORDER BY position")
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	cues := []subtitle.Cue{}
	for rows.Next() {
		var raw string
		var cue subtitle.Cue
		if err = rows.Scan(&raw); err != nil {
			return nil, err
		}
		if err = json.Unmarshal([]byte(raw), &cue); err != nil {
			return nil, err
		}
		cues = append(cues, cue)
	}
	return cues, rows.Err()
}

func (s *Store) InsertCue(commandID string, expected int64, original bool, insertion Insertion) (int64, error) {
	return s.mutate(commandID, expected, func(tx *sql.Tx, cursor int64) (int64, error) {
		if strings.TrimSpace(insertion.Source) == "" || len(insertion.Source) > 1024*1024 || len(insertion.Translation) > 1024*1024 {
			return cursor, errors.New(locales.ErrEditText())
		}
		if insertion.StartMs < 0 || insertion.EndMs <= insertion.StartMs || insertion.EndMs > maxSafeTime {
			return cursor, errors.New(locales.ErrEditTiming())
		}
		table, track, err := manualTrack(tx, original)
		if err != nil {
			return cursor, err
		}
		cues, err := orderedCues(tx, table)
		if err != nil {
			return cursor, err
		}
		at := insertion.Position
		if at < 0 || at > len(cues) {
			return cursor, errors.New(locales.ErrCuePosition())
		}
		cue := subtitle.Cue{ID: "manual-" + uuid.NewString(), Position: at, StartMs: insertion.StartMs, EndMs: insertion.EndMs, Text: insertion.Source, OriginalText: insertion.Source, TimeLocked: true, TimingQuality: "manual"}
		if !original && strings.TrimSpace(insertion.Translation) != "" {
			cue.TranslationSource, cue.Text, cue.Translated, cue.TextLocked = insertion.Source, insertion.Translation, true, true
		}
		before := append([]subtitle.Cue{}, cues[at:]...)
		after := append([]subtitle.Cue{cue}, before...)
		for i := range after {
			after[i].Position = at + i
		}
		return recordTrackHistory(tx, cursor, before, after, track)
	})
}

func (s *Store) DeleteCue(commandID string, expected int64, original bool, id string) (int64, error) {
	return s.mutate(commandID, expected, func(tx *sql.Tx, cursor int64) (int64, error) {
		table, track, err := manualTrack(tx, original)
		if err != nil {
			return cursor, err
		}
		cues, err := orderedCues(tx, table)
		if err != nil {
			return cursor, err
		}
		at := slices.IndexFunc(cues, func(cue subtitle.Cue) bool { return cue.ID == id })
		if at < 0 {
			return cursor, errors.New(locales.ErrCuePosition())
		}
		if cues[at].ReadOnly {
			return cursor, errors.New(locales.ErrEditProtected())
		}
		before := cues[at:]
		after := append([]subtitle.Cue{}, cues[at+1:]...)
		for i := range after {
			after[i].Position = at + i
		}
		return recordTrackHistory(tx, cursor, before, after, track)
	})
}
