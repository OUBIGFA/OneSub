package project

import (
	"database/sql"
	"encoding/json"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
)

// ApplyTranslationDocument commits a reviewed timeline and translations as one
// reversible gesture. The transaction rechecks the live revision and provenance;
// rejected or stale candidates never touch cues/history.
func (s *Store) ApplyTranslationDocument(commandID string, expected int64, candidate subtitle.Document) (int64, error) {
	return s.mutate(commandID, expected, func(tx *sql.Tx, cursor int64) (int64, error) {
		var base subtitle.Document
		if err := tx.QueryRow("SELECT format,original FROM state WHERE singleton=1").Scan(&base.Format, &base.Original); err != nil {
			return cursor, err
		}
		rows, err := tx.Query("SELECT body FROM cues ORDER BY position")
		if err != nil {
			return cursor, err
		}
		for rows.Next() {
			var raw string
			var cue subtitle.Cue
			if err = rows.Scan(&raw); err == nil {
				err = json.Unmarshal([]byte(raw), &cue)
			}
			if err != nil {
				rows.Close()
				return cursor, err
			}
			base.Cues = append(base.Cues, cue)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return cursor, err
		}
		if err = subtitle.ValidateTranslation(base, candidate); err != nil {
			return cursor, err
		}
		return recordHistory(tx, cursor, base.Cues, candidate.Cues)
	})
}
