package project

import (
	"context"
	"database/sql"
	"errors"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// SplitCues uses the same deterministic plan as the format preflight, but
// resolves the current track inside the revision-checked transaction. One
// command renumbers every affected cue and makes exactly one undo step.
func (s *Store) SplitCues(ctx context.Context, command string, expected int64, original bool) (int64, error) {
	return s.splitCues(ctx, command, expected, original, "")
}

// SplitCue deliberately operates on the chosen cue, even when it is not long.
// The other track and all other cue content/timing remain untouched.
func (s *Store) SplitCue(ctx context.Context, command string, expected int64, original bool, id string) (int64, error) {
	if id == "" {
		return 0, errors.New(locales.ErrCuePosition())
	}
	return s.splitCues(ctx, command, expected, original, id)
}

func (s *Store) splitCues(ctx context.Context, command string, expected int64, original bool, id string) (int64, error) {
	return s.mutate(command, expected, func(tx *sql.Tx, cursor int64) (int64, error) {
		table, track, err := manualTrack(tx, original)
		if err != nil {
			return cursor, err
		}
		cues, err := orderedCues(tx, table)
		if err != nil {
			return cursor, err
		}
		var format string
		if err = tx.QueryRow("SELECT format FROM state WHERE singleton=1").Scan(&format); err != nil {
			return cursor, err
		}
		document := subtitle.Document{Format: format, Cues: cues}
		var report subtitle.SplitReport
		if id == "" {
			report, err = subtitle.SmartSplit(ctx, document, command, original)
		} else {
			report, err = subtitle.SmartSplitCue(ctx, document, command, original, id)
		}
		if err != nil {
			return cursor, err
		}
		if report.Changed == 0 {
			return cursor, errors.New(locales.ErrNoEdits())
		}
		if err = ctx.Err(); err != nil {
			return cursor, err
		}
		return recordTrackHistory(tx, cursor, cues[report.First:], report.Document.Cues[report.First:], track)
	})
}
