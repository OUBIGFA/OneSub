package translator

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"os"
	"path/filepath"
	"time"
)

// cache keeps each validated model result, keyed by everything that shaped
// it: service, model, rules, style, glossary, phase and the exact input.
// A translation that stops (cancelled, a request limit, a failed batch)
// resumes from it, as VideoCaptioner's translate cache does: finished
// batches are not sent, nor paid for, again. It holds results only, never
// keys or credentials.
type cache struct{ dir string }

// cacheAge is how long an unused result is kept.
const cacheAge = 30 * 24 * time.Hour

func cacheKey(parts ...any) string {
	data, _ := json.Marshal(parts)
	sum := sha256.Sum256(data)
	return hex.EncodeToString(sum[:])
}

func (c cache) get(key string, value any) bool {
	if c.dir == "" {
		return false
	}
	path := filepath.Join(c.dir, key+".json")
	data, err := os.ReadFile(path)
	if err != nil || json.Unmarshal(data, value) != nil {
		return false
	}
	// A result in use is not one to prune.
	now := time.Now()
	os.Chtimes(path, now, now)
	return true
}

// put writes through a temporary file, so a crash never leaves half a
// result to be read back.
func (c cache) put(key string, value any) error {
	if c.dir == "" {
		return nil
	}
	data, err := json.Marshal(value)
	if err != nil {
		return err
	}
	if err = os.MkdirAll(c.dir, 0700); err != nil {
		return err
	}
	file, err := os.CreateTemp(c.dir, key+".*.tmp")
	if err != nil {
		return err
	}
	_, err = file.Write(data)
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err == nil {
		err = os.Rename(file.Name(), filepath.Join(c.dir, key+".json"))
	}
	if err != nil {
		os.Remove(file.Name())
	}
	return err
}

// prune removes results unused for cacheAge. It is housekeeping: a file it
// cannot remove is simply tried again next time.
func (c cache) prune() {
	if c.dir == "" {
		return
	}
	entries, err := os.ReadDir(c.dir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if info, err := entry.Info(); err == nil && time.Since(info.ModTime()) > cacheAge {
			os.Remove(filepath.Join(c.dir, entry.Name()))
		}
	}
}
