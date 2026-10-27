// Package project owns durable edits. The frontend never writes the database.
package project

import (
	"crypto/sha256"
	"database/sql"
	"encoding/hex"
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"sync"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/google/uuid"
	_ "modernc.org/sqlite"
)

const schemaVersion = 1
const maxSafeTime = int64(9007199254740991)

type Manifest struct {
	Schema       int    `json:"schema"`
	ID           string `json:"id"`
	Name         string `json:"name"`
	MediaPath    string `json:"mediaPath"`
	SubtitlePath string `json:"subtitlePath"`
	SourceSHA256 string `json:"sourceSha256"`
}
type Snapshot struct {
	Manifest Manifest          `json:"manifest"`
	Path     string            `json:"path"`
	Revision int64             `json:"revision"`
	Document subtitle.Document `json:"document"`
	CanUndo  bool              `json:"canUndo"`
	CanRedo  bool              `json:"canRedo"`
}

type Store struct {
	mu       sync.Mutex
	db       *sql.DB
	lock     *os.File
	path     string
	manifest Manifest
}

// Create reserves a new directory; existing projects and their sources are never
// overwritten. The original subtitle is copied byte for byte before edits exist.
func Create(path, mediaPath, sourcePath string, doc subtitle.Document, displayName ...string) (*Store, error) {
	path, err := localPath(path)
	if err != nil {
		return nil, err
	}
	if !strings.EqualFold(filepath.Ext(path), ".onesub") {
		return nil, errors.New("工程目录须以 .onesub 结尾")
	}
	if err := os.Mkdir(path, 0700); err != nil {
		return nil, fmt.Errorf("请使用新的工程名称：%w", err)
	}
	manifest := Manifest{Schema: schemaVersion, ID: uuid.NewString(), Name: strings.TrimSuffix(filepath.Base(path), filepath.Ext(path)), MediaPath: mediaPath, SubtitlePath: sourcePath}
	if len(displayName) > 0 && strings.TrimSpace(displayName[0]) != "" {
		manifest.Name = displayName[0]
	}
	if err = os.Mkdir(filepath.Join(path, "originals"), 0700); err != nil {
		return nil, err
	}
	if sourcePath != "" {
		raw, err := os.ReadFile(sourcePath)
		if err != nil {
			return nil, err
		}
		if len(raw) > 64*1024*1024 {
			return nil, errors.New("字幕来源超过 64 MiB")
		}
		hash := sha256.Sum256(raw)
		manifest.SourceSHA256 = hex.EncodeToString(hash[:])
		if err = os.WriteFile(filepath.Join(path, "originals", "source"+filepath.Ext(sourcePath)), raw, 0600); err != nil {
			return nil, err
		}
	}
	raw, err := json.MarshalIndent(manifest, "", "  ")
	if err != nil {
		return nil, err
	}
	if err = os.WriteFile(filepath.Join(path, "project.json"), raw, 0600); err != nil {
		return nil, err
	}
	s, err := connect(path, manifest, true)
	if err != nil {
		return nil, err
	}
	if err = s.initialize(doc); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

func localPath(path string) (string, error) {
	abs, err := filepath.Abs(path)
	if err != nil {
		return "", err
	}
	// UNC paths are not reliable WAL/lock locations. Sync-provider detection is a
	// separate UI warning; this does not claim to detect every remote filesystem.
	if strings.HasPrefix(abs, `\\`) {
		return "", errors.New("活动工程须位于本地磁盘，不能使用网络共享")
	}
	return abs, nil
}

func Open(path string) (*Store, error) {
	path, err := localPath(path)
	if err != nil {
		return nil, err
	}
	raw, err := os.ReadFile(filepath.Join(path, "project.json"))
	if err != nil {
		return nil, err
	}
	var m Manifest
	if err = json.Unmarshal(raw, &m); err != nil {
		return nil, err
	}
	// No migration is attempted on an unknown schema; the existing files stay intact.
	if m.Schema != schemaVersion || m.ID == "" {
		return nil, errors.New("不支持此工程版本；未修改原工程")
	}
	s, err := connect(path, m, false)
	if err != nil {
		return nil, err
	}
	if _, err = s.Snapshot(); err != nil {
		s.Close()
		return nil, err
	}
	return s, nil
}

func connect(path string, m Manifest, create bool) (*Store, error) {
	lock, err := acquireLock(filepath.Join(path, "writer.lock"))
	if err != nil {
		return nil, fmt.Errorf("工程正在使用或无法加锁；未抢占其他写入者：%w", err)
	}
	dbPath := filepath.Join(path, "project.sqlite")
	if !create {
		if _, err = os.Stat(dbPath); err != nil {
			lock.Close()
			return nil, err
		}
	}
	db, err := sql.Open("sqlite", filepath.ToSlash(dbPath))
	if err != nil {
		lock.Close()
		return nil, err
	}
	db.SetMaxOpenConns(1)
	s := &Store{db: db, lock: lock, path: path, manifest: m}
	for _, p := range []string{"PRAGMA journal_mode=WAL", "PRAGMA synchronous=FULL", "PRAGMA foreign_keys=ON", "PRAGMA busy_timeout=5000"} {
		if _, err = db.Exec(p); err != nil {
			s.Close()
			return nil, err
		}
	}
	return s, nil
}

func (s *Store) initialize(doc subtitle.Document) error {
	tx, err := s.db.Begin()
	if err != nil {
		return err
	}
	defer tx.Rollback()
	_, err = tx.Exec(`CREATE TABLE state (singleton INTEGER PRIMARY KEY CHECK(singleton=1), revision INTEGER NOT NULL, cursor INTEGER NOT NULL, format TEXT NOT NULL, original TEXT NOT NULL);
 CREATE TABLE cues (id TEXT PRIMARY KEY, position INTEGER UNIQUE NOT NULL, body TEXT NOT NULL);
 CREATE TABLE history (seq INTEGER PRIMARY KEY, before_json TEXT NOT NULL, after_json TEXT NOT NULL);
 CREATE TABLE commands (id TEXT PRIMARY KEY, revision INTEGER NOT NULL);
 PRAGMA user_version=1;`)
	if err != nil {
		return err
	}
	if _, err = tx.Exec("INSERT INTO state VALUES(1,0,0,?,?)", doc.Format, doc.Original); err != nil {
		return err
	}
	for i, cue := range doc.Cues {
		cue.Position = i
		if cue.ID == "" {
			return errors.New("字幕 ID 不能为空")
		}
		raw, err := json.Marshal(cue)
		if err != nil {
			return err
		}
		if _, err = tx.Exec("INSERT INTO cues VALUES(?,?,?)", cue.ID, cue.Position, string(raw)); err != nil {
			return err
		}
	}
	return tx.Commit()
}

func (s *Store) Snapshot() (Snapshot, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	result := Snapshot{Manifest: s.manifest, Path: s.path, Document: subtitle.Document{Cues: []subtitle.Cue{}}}
	var cursor int64
	err := s.db.QueryRow("SELECT revision,cursor,format,original FROM state WHERE singleton=1").Scan(&result.Revision, &cursor, &result.Document.Format, &result.Document.Original)
	if err != nil {
		return result, err
	}
	rows, err := s.db.Query("SELECT body FROM cues ORDER BY position")
	if err != nil {
		return result, err
	}
	for rows.Next() {
		var raw string
		var cue subtitle.Cue
		if err = rows.Scan(&raw); err == nil {
			err = json.Unmarshal([]byte(raw), &cue)
		}
		if err != nil {
			rows.Close()
			return result, err
		}
		result.Document.Cues = append(result.Document.Cues, cue)
	}
	err = rows.Err()
	rows.Close()
	if err != nil {
		return result, err
	}
	err = s.db.QueryRow("SELECT EXISTS(SELECT 1 FROM history WHERE seq<=?),EXISTS(SELECT 1 FROM history WHERE seq>?)", cursor, cursor).Scan(&result.CanUndo, &result.CanRedo)
	return result, err
}

// Edit contains only editable fields. Original text, identity, order and locks
// are owned by the transaction, not supplied by the page.
type Edit struct {
	ID      string `json:"id"`
	Text    string `json:"text"`
	StartMs int64  `json:"startMs"`
	EndMs   int64  `json:"endMs"`
	Track   string `json:"track,omitempty"` // manual source/translation editing only
}

// Apply persists one gesture as a diff. Repeating commandID returns its original
// receipt; optimistic revision checks prevent late results overwriting edits.
func (s *Store) Apply(commandID string, expected int64, edits []Edit) (int64, error) {
	return s.apply(commandID, expected, edits, true)
}

// ApplyTranslation accepts only reviewed, version-matched candidates. Unlike a
// manual gesture it does not create locks, and it cannot replace a locked cue.
func (s *Store) ApplyTranslation(commandID string, expected int64, edits []Edit) (int64, error) {
	return s.apply(commandID, expected, edits, false)
}

func (s *Store) apply(commandID string, expected int64, edits []Edit, manual bool) (int64, error) {
	return s.mutate(commandID, expected, func(tx *sql.Tx, cursor int64) (int64, error) {
		if len(edits) == 0 {
			return cursor, errors.New("没有修改")
		}
		before, after := []subtitle.Cue{}, []subtitle.Cue{}
		seen := map[string]bool{}
		for _, e := range edits {
			if seen[e.ID] {
				return cursor, errors.New("修改包含重复字幕 ID")
			}
			seen[e.ID] = true
			if e.StartMs < 0 || e.EndMs <= e.StartMs || e.EndMs > maxSafeTime {
				return cursor, errors.New("结束时间须大于开始时间，且时间不能为负")
			}
			if strings.TrimSpace(e.Text) == "" || len(e.Text) > 1024*1024 {
				return cursor, errors.New("字幕不能为空或超过 1 MiB")
			}
			var raw string
			if err := tx.QueryRow("SELECT body FROM cues WHERE id=?", e.ID).Scan(&raw); err != nil {
				return cursor, err
			}
			var cue subtitle.Cue
			if err := json.Unmarshal([]byte(raw), &cue); err != nil {
				return cursor, err
			}
			if cue.ReadOnly {
				return cursor, errors.New("此事件受格式保护，不能编辑")
			}
			if !manual && (cue.TextLocked || cue.StartMs != e.StartMs || cue.EndMs != e.EndMs) {
				return cursor, errors.New("候选不能覆盖手改锁或修改时间轴")
			}
			if e.Track != "" && e.Track != "source" && e.Track != "translation" {
				return cursor, errors.New("未知字幕语言轨")
			}
			if !manual && e.Track != "" {
				return cursor, errors.New("翻译结果不能更改原文轨")
			}
			before = append(before, cue)
			if manual {
				previous := cue.Text
				if e.Track == "source" && cue.Translated {
					previous = cue.TranslationSource
				}
				cue.TextLocked = cue.TextLocked || previous != e.Text
				cue.TimeLocked = cue.TimeLocked || cue.StartMs != e.StartMs || cue.EndMs != e.EndMs
			}
			if !manual || e.Track == "translation" {
				if !cue.Translated {
					cue.TranslationSource = cue.Text
				}
				cue.Translated = true
			}
			if manual && e.Track == "source" && cue.Translated {
				cue.TranslationSource = e.Text
			} else {
				cue.Text = e.Text
			}
			cue.StartMs, cue.EndMs = e.StartMs, e.EndMs
			after = append(after, cue)
		}
		return recordHistory(tx, cursor, before, after)
	})
}

// SetTextLock is an explicit reversible user action; automatic results never
// unlock text as a side effect of rerunning a task.
func (s *Store) SetTextLock(commandID string, expected int64, id string, locked bool) (int64, error) {
	return s.mutate(commandID, expected, func(tx *sql.Tx, cursor int64) (int64, error) {
		var raw string
		if err := tx.QueryRow("SELECT body FROM cues WHERE id=?", id).Scan(&raw); err != nil {
			return cursor, err
		}
		var cue subtitle.Cue
		if err := json.Unmarshal([]byte(raw), &cue); err != nil {
			return cursor, err
		}
		if cue.ReadOnly {
			return cursor, errors.New("此事件受格式保护，不能解锁")
		}
		before := []subtitle.Cue{cue}
		cue.TextLocked = locked
		return recordHistory(tx, cursor, before, []subtitle.Cue{cue})
	})
}

func recordHistory(tx *sql.Tx, cursor int64, before, after []subtitle.Cue) (int64, error) {
	if err := putCues(tx, after); err != nil {
		return cursor, err
	}
	if _, err := tx.Exec("DELETE FROM history WHERE seq>?", cursor); err != nil {
		return cursor, err
	}
	b, err := json.Marshal(before)
	if err != nil {
		return cursor, err
	}
	a, err := json.Marshal(after)
	if err != nil {
		return cursor, err
	}
	_, err = tx.Exec("INSERT INTO history VALUES(?,?,?)", cursor+1, string(b), string(a))
	return cursor + 1, err
}

func (s *Store) Undo(commandID string, expected int64, redo bool) (int64, error) {
	return s.mutate(commandID, expected, func(tx *sql.Tx, cursor int64) (int64, error) {
		seq, column, next := cursor, "before_json", cursor-1
		if redo {
			seq, column, next = cursor+1, "after_json", cursor+1
		}
		var raw string
		if err := tx.QueryRow("SELECT "+column+" FROM history WHERE seq=?", seq).Scan(&raw); err != nil {
			return cursor, errors.New("没有可撤销或重做的修改")
		}
		var cues []subtitle.Cue
		if err := json.Unmarshal([]byte(raw), &cues); err != nil {
			return cursor, err
		}
		return next, putCues(tx, cues)
	})
}

func putCues(tx *sql.Tx, cues []subtitle.Cue) error {
	for _, cue := range cues {
		raw, err := json.Marshal(cue)
		if err != nil {
			return err
		}
		if _, err = tx.Exec("UPDATE cues SET body=? WHERE id=?", string(raw), cue.ID); err != nil {
			return err
		}
	}
	return nil
}

func (s *Store) mutate(id string, expected int64, change func(*sql.Tx, int64) (int64, error)) (int64, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	if id == "" || len(id) > 128 {
		return 0, errors.New("缺少有效命令 ID")
	}
	tx, err := s.db.Begin()
	if err != nil {
		return 0, err
	}
	defer tx.Rollback()
	var prior int64
	err = tx.QueryRow("SELECT revision FROM commands WHERE id=?", id).Scan(&prior)
	if err == nil {
		return prior, nil
	}
	if !errors.Is(err, sql.ErrNoRows) {
		return 0, err
	}
	var revision, cursor int64
	if err = tx.QueryRow("SELECT revision,cursor FROM state WHERE singleton=1").Scan(&revision, &cursor); err != nil {
		return 0, err
	}
	if revision != expected {
		return revision, errors.New("REVISION_CONFLICT：工程已有新修改，请重新载入后核对")
	}
	cursor, err = change(tx, cursor)
	if err != nil {
		return revision, err
	}
	revision++
	if _, err = tx.Exec("UPDATE state SET revision=?,cursor=? WHERE singleton=1", revision, cursor); err != nil {
		return 0, err
	}
	if _, err = tx.Exec("INSERT INTO commands VALUES(?,?)", id, revision); err != nil {
		return 0, err
	}
	return revision, tx.Commit()
}

func (s *Store) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()
	err := s.db.Close()
	lockErr := s.lock.Close()
	if err != nil {
		return err
	}
	return lockErr
}
