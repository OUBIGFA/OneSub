package project

import (
	"crypto/sha256"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"slices"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/google/uuid"
)

// Speaker history includes both tracks and the dictionary in one transaction.
// Ordinary text history retains its existing array representation.
type speakerSnapshot struct {
	State   *subtitle.SpeakerState `json:"state"`
	Source  []subtitle.Cue         `json:"source"`
	Working []subtitle.Cue         `json:"working"`
}

// Store large immutable acoustic evidence once. Renaming or correcting one
// label must not copy an hour of turns into every undo record.
func speakerReference(state *subtitle.SpeakerState) *subtitle.SpeakerState {
	if state == nil {
		return nil
	}
	copy := *state
	if copy.Artifact != "" {
		copy.Turns = nil
	}
	return &copy
}
func speakerHistory(value speakerSnapshot) ([]byte, error) {
	value.State = speakerReference(value.State)
	return json.Marshal(value)
}
func (s *Store) persistSpeakerArtifact(state *subtitle.SpeakerState) error {
	if state == nil || len(state.Turns) == 0 {
		return nil
	}
	evidence := struct {
		AnalysisID string                 `json:"analysisId"`
		Turns      []subtitle.SpeakerTurn `json:"turns"`
	}{state.AnalysisID, state.Turns}
	raw, err := json.Marshal(evidence)
	if err != nil {
		return err
	}
	name := fmt.Sprintf("speaker-%x.json", sha256.Sum256(raw))
	dir := filepath.Join(s.path, "artifacts")
	if err = os.MkdirAll(dir, 0700); err != nil {
		return err
	}
	if err = subtitle.Replace(filepath.Join(dir, name), raw); err != nil {
		return err
	}
	state.Artifact = name
	return nil
}
func (s *Store) hydrateSpeakers(state *subtitle.SpeakerState) error {
	if state == nil || state.Artifact == "" || len(state.Turns) > 0 {
		return nil
	}
	if filepath.Base(state.Artifact) != state.Artifact || len(state.Artifact) != 77 || !strings.HasPrefix(state.Artifact, "speaker-") {
		return errors.New("invalid speaker artifact path")
	}
	file, err := os.Open(filepath.Join(s.path, "artifacts", state.Artifact))
	if err != nil {
		return err
	}
	raw, err := io.ReadAll(io.LimitReader(file, 64*1024*1024+1))
	file.Close()
	if err != nil {
		return err
	}
	if len(raw) > 64*1024*1024 || fmt.Sprintf("speaker-%x.json", sha256.Sum256(raw)) != state.Artifact {
		return errors.New("speaker evidence checksum mismatch")
	}
	var evidence struct {
		AnalysisID string                 `json:"analysisId"`
		Turns      []subtitle.SpeakerTurn `json:"turns"`
	}
	if err = json.Unmarshal(raw, &evidence); err != nil {
		return err
	}
	if evidence.AnalysisID != state.AnalysisID {
		return errors.New("speaker evidence identity mismatch")
	}
	state.Turns = evidence.Turns
	return nil
}

func (store *Store) loadSpeakers(tx *sql.Tx) (speakerSnapshot, error) {
	var s speakerSnapshot
	var raw string
	if err := tx.QueryRow("SELECT body FROM speaker_state WHERE singleton=1").Scan(&raw); err != nil {
		return s, err
	}
	if err := json.Unmarshal([]byte(raw), &s.State); err != nil {
		return s, err
	}
	if err := store.hydrateSpeakers(s.State); err != nil {
		return s, err
	}
	var err error
	s.Source, err = orderedCues(tx, "source_cues")
	if err != nil {
		return s, err
	}
	s.Working, err = orderedCues(tx, "cues")
	return s, err
}
func writeSpeakers(tx *sql.Tx, s speakerSnapshot) error {
	if err := subtitle.ValidateSpeakerState(s.State); err != nil {
		return err
	}
	raw, err := json.Marshal(speakerReference(s.State))
	if err != nil {
		return err
	}
	if _, err = tx.Exec("UPDATE speaker_state SET body=? WHERE singleton=1", string(raw)); err != nil {
		return err
	}
	for _, table := range []string{"source_cues", "cues"} {
		if _, err = tx.Exec("DELETE FROM " + table); err != nil {
			return err
		}
		cues := s.Source
		if table == "cues" {
			cues = s.Working
		}
		if err = replaceTableCues(tx, nil, cues, table); err != nil {
			return err
		}
	}
	return nil
}
func (s *Store) changeSpeakers(id string, expected int64, change func(*speakerSnapshot) error) (int64, error) {
	return s.mutate(id, expected, func(tx *sql.Tx, cursor int64) (int64, error) {
		value, err := s.loadSpeakers(tx)
		if err != nil {
			return cursor, err
		}
		before, err := speakerHistory(value)
		if err != nil {
			return cursor, err
		}
		if err = change(&value); err != nil {
			return cursor, err
		}
		if err = writeSpeakers(tx, value); err != nil {
			return cursor, err
		}
		after, err := speakerHistory(value)
		if err != nil {
			return cursor, err
		}
		if _, err = tx.Exec("DELETE FROM history WHERE seq>?", cursor); err != nil {
			return cursor, err
		}
		_, err = tx.Exec("INSERT INTO history(seq,before_json,after_json,track) VALUES(?,?,?,'speakers')", cursor+1, string(before), string(after))
		return cursor + 1, err
	})
}

func (s *Store) ApplySpeakers(id string, expected int64, state *subtitle.SpeakerState) (int64, error) {
	if err := subtitle.ValidateSpeakerState(state); err != nil {
		return 0, err
	}
	if state == nil {
		return 0, errors.New("missing speaker analysis")
	}
	// Copy user-owned maps before retaining manual names from an older analysis.
	raw, _ := json.Marshal(state)
	var frozen subtitle.SpeakerState
	if err := json.Unmarshal(raw, &frozen); err != nil {
		return 0, err
	}
	if frozen.Names == nil {
		frozen.Names = map[string]string{}
	}
	if err := s.persistSpeakerArtifact(&frozen); err != nil {
		return 0, err
	}
	return s.changeSpeakers(id, expected, func(v *speakerSnapshot) error {
		if frozen.MediaPath != "" && !strings.EqualFold(frozen.MediaPath, s.manifest.MediaPath) {
			return errors.New("media changed since speaker analysis")
		}
		if v.State != nil {
			for key, name := range v.State.Names {
				// A cached/repeated analysis has stable IDs. Preserve its user
				// aliases; different analyses do not guess an identity mapping.
				if strings.HasPrefix(key, "manual/") || v.State.AnalysisID == frozen.AnalysisID && frozen.Names[key] != "" {
					frozen.Names[key] = name
				}
			}
		}
		for _, track := range [][]subtitle.Cue{v.Source, v.Working} {
			for _, c := range track {
				if c.Speaker != nil && c.Speaker.Manual && v.State != nil {
					for _, old := range c.Speaker.IDs {
						if name := v.State.Names[old]; name != "" {
							frozen.Names[old] = name
						}
					}
				}
			}
		}
		v.State = &frozen
		v.Source = subtitle.AttachSpeakers(subtitle.Document{Cues: v.Source}, v.State).Cues
		v.Working = subtitle.AttachSpeakers(subtitle.Document{Cues: v.Working}, v.State).Cues
		return nil
	})
}

// SetSpeakers changes the selected cues and exact provenance counterparts,
// never unrelated rows at the same table position.
func (s *Store) SetSpeakers(id string, expected int64, ids []string, speaker string, original ...bool) (int64, error) {
	return s.changeSpeakers(id, expected, func(v *speakerSnapshot) error {
		if len(ids) == 0 || v.State == nil {
			return errors.New("no speaker selection")
		}
		if speaker != "" && v.State.Names[speaker] == "" {
			return errors.New("unknown speaker")
		}
		primary, other := v.Working, v.Source
		if len(original) > 0 && original[0] {
			primary, other = v.Source, v.Working
		}
		selected := map[string]bool{}
		for _, id := range ids {
			selected[id] = true
		}
		lineage := func(c subtitle.Cue) []string {
			keys := []string{c.ID}
			keys = append(keys, c.SourceCueIDs...)
			keys = append(keys, c.SplitCueIDs...)
			keys = append(keys, c.MergedCueIDs...)
			return keys
		}
		chosen := map[string][]subtitle.Cue{}
		found := 0
		assign := func(c *subtitle.Cue) {
			a := &subtitle.SpeakerAssignment{Status: "unknown", Manual: true}
			if speaker != "" {
				a.Status = "single"
				a.IDs = []string{speaker}
			}
			c.Speaker = a
		}
		for i := range primary {
			c := &primary[i]
			if !selected[c.ID] {
				continue
			}
			if c.ReadOnly {
				return errors.New("protected subtitle")
			}
			found++
			for _, key := range lineage(*c) {
				chosen[key] = append(chosen[key], *c)
			}
			assign(c)
		}
		if found != len(selected) {
			return errors.New("subtitle no longer exists")
		}
		for i := range other {
			c := &other[i]
			if c.ReadOnly {
				continue
			}
			var spans [][2]int64
			seen := map[string]bool{}
			for _, key := range lineage(*c) {
				for _, source := range chosen[key] {
					if !seen[source.ID] {
						seen[source.ID] = true
						start, end := max(c.StartMs, source.StartMs), min(c.EndMs, source.EndMs)
						if end > start {
							spans = append(spans, [2]int64{start, end})
						}
					}
				}
			}
			if len(spans) == 0 {
				continue
			}
			slices.SortFunc(spans, func(a, b [2]int64) int {
				if a[0] < b[0] {
					return -1
				}
				if a[0] > b[0] {
					return 1
				}
				return 0
			})
			end := c.StartMs
			for _, span := range spans {
				if span[0] > end {
					break
				}
				end = max(end, span[1])
			}
			if end >= c.EndMs {
				assign(c)
			} else {
				subtitle.MarkSpeakerReview(c)
			}
		}
		return nil
	})
}
func (s *Store) RenameSpeaker(id string, expected int64, speaker, name string) (int64, error) {
	name = strings.TrimSpace(name)
	return s.changeSpeakers(id, expected, func(v *speakerSnapshot) error {
		if v.State == nil || v.State.Names[speaker] == "" {
			return errors.New("unknown speaker")
		}
		v.State.Names[speaker] = name
		return subtitle.ValidateSpeakerState(v.State)
	})
}

func (s *Store) AddSpeaker(id string, expected int64, name string) (int64, error) {
	return s.changeSpeakers(id, expected, func(v *speakerSnapshot) error {
		if v.State == nil {
			v.State = &subtitle.SpeakerState{Names: map[string]string{}}
		}
		v.State.Names["manual/"+uuid.NewString()] = strings.TrimSpace(name)
		return subtitle.ValidateSpeakerState(v.State)
	})
}

func (s *Store) upgradeSpeakers() error {
	var version int
	if err := s.db.QueryRow("PRAGMA user_version").Scan(&version); err != nil {
		return err
	}
	if version != 2 && version != 3 {
		return errors.New("unsupported speaker schema")
	}
	if version == 2 {
		backup := filepath.Join(s.path, "originals", "project-v2-"+uuid.NewString()+".sqlite")
		if _, err := s.db.Exec("VACUUM INTO ?", backup); err != nil {
			return err
		}
		tx, err := s.db.Begin()
		if err != nil {
			return err
		}
		defer tx.Rollback()
		if _, err = tx.Exec("CREATE TABLE speaker_state(singleton INTEGER PRIMARY KEY CHECK(singleton=1),body TEXT NOT NULL); INSERT INTO speaker_state VALUES(1,'null'); PRAGMA user_version=3;"); err != nil {
			return err
		}
		if err = tx.Commit(); err != nil {
			return err
		}
	}
	s.manifest.Schema = 3
	raw, err := json.MarshalIndent(s.manifest, "", "  ")
	if err != nil {
		return err
	}
	return subtitle.Replace(filepath.Join(s.path, "project.json"), raw)
}

// Media changes invalidate automatic labels, but retain names and evidence for
// review. Snapshot applies this projection even to old history after Undo.
func invalidateSpeakerMedia(doc subtitle.Document, media string) subtitle.Document {
	if doc.Speakers == nil || doc.Speakers.MediaPath == "" {
		return doc
	}
	valid := strings.EqualFold(doc.Speakers.MediaPath, media)
	if valid && doc.Speakers.MediaModifiedNs != 0 {
		info, err := os.Stat(media)
		valid = err == nil && info.Size() == doc.Speakers.MediaBytes && info.ModTime().UnixNano() == doc.Speakers.MediaModifiedNs
	}
	if valid {
		return doc
	}
	doc.Cues = slices.Clone(doc.Cues)
	for i := range doc.Cues {
		subtitle.MarkSpeakerReview(&doc.Cues[i])
	}
	return doc
}
