package project

import (
	"encoding/json"
	"errors"
	"path/filepath"
	"sort"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/google/uuid"
)

// upgradeOriginalTrack takes a consistent SQLite backup before changing the
// schema. Reversing history to before the first translation recovers the real
// source segmentation, including source corrections made before that run.
func (s *Store) upgradeOriginalTrack() error {
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version != 1 && version != schemaVersion {
		return errors.New(locales.ErrProjectVersion())
	}
	if version == 1 {
		backup := filepath.Join(s.path, "originals", "project-v1-"+uuid.NewString()+".sqlite")
		if _, err := s.db.Exec("VACUUM INTO ?", backup); err != nil {
			return err
		}
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		var cursor int64
		if err = tx.QueryRow("SELECT cursor FROM state WHERE singleton=1").Scan(&cursor); err != nil {
			return err
		}
		cues := map[string]subtitle.Cue{}
		rows, err := tx.Query("SELECT body FROM cues")
		if err != nil {
			return err
		}
		for rows.Next() {
			var raw string
			var cue subtitle.Cue
			if err = rows.Scan(&raw); err == nil {
				err = json.Unmarshal([]byte(raw), &cue)
			}
			if err != nil {
				rows.Close()
				return err
			}
			cues[cue.ID] = cue
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		type change struct{ before, after []subtitle.Cue }
		changes := []change{}
		first := -1
		rows, err = tx.Query("SELECT before_json,after_json FROM history WHERE seq<=? ORDER BY seq", cursor)
		if err != nil {
			return err
		}
		for rows.Next() {
			var b, a string
			var c change
			if err = rows.Scan(&b, &a); err == nil {
				err = json.Unmarshal([]byte(b), &c.before)
			}
			if err == nil {
				err = json.Unmarshal([]byte(a), &c.after)
			}
			if err != nil {
				rows.Close()
				return err
			}
			if first < 0 {
				for _, cue := range c.after {
					if cue.Translated || cue.SourceRange != nil && cue.SourceRange.Reason != "" {
						first = len(changes)
						break
					}
				}
			}
			changes = append(changes, c)
		}
		err = rows.Err()
		rows.Close()
		if err != nil {
			return err
		}
		if first >= 0 {
			for i := len(changes) - 1; i >= first; i-- {
				for _, cue := range changes[i].after {
					delete(cues, cue.ID)
				}
				for _, cue := range changes[i].before {
					cues[cue.ID] = cue
				}
			}
		}
		if _, err = tx.Exec("CREATE TABLE source_cues (id TEXT PRIMARY KEY, position INTEGER UNIQUE NOT NULL, body TEXT NOT NULL); ALTER TABLE history ADD COLUMN track TEXT NOT NULL DEFAULT 'working'; PRAGMA user_version=2;"); err != nil {
			return err
		}
		original := make([]subtitle.Cue, 0, len(cues))
		for _, cue := range cues {
			original = append(original, cue)
		}
		sort.Slice(original, func(i, j int) bool { return original[i].Position < original[j].Position })
		if err = replaceTableCues(tx, nil, original, "source_cues"); err != nil {
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	// A crash after the transaction but before the manifest is safe to resume.
	s.manifest.Schema = schemaVersion
	raw, err := json.MarshalIndent(s.manifest, "", "  ")
	if err != nil {
		return err
	}
	return subtitle.Replace(filepath.Join(s.path, "project.json"), raw)
}
