package subtitle

import (
	"os"
	"path/filepath"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/apperr"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// writeTemporary leaves a complete, synced copy of data next to path.
func writeTemporary(path string, data []byte) (string, error) {
	file, err := os.CreateTemp(filepath.Dir(path), ".onesub-write-*")
	if err != nil {
		return "", err
	}
	temporary := file.Name()
	if _, err = file.Write(data); err == nil {
		err = file.Sync()
	}
	if closeErr := file.Close(); err == nil {
		err = closeErr
	}
	if err != nil {
		os.Remove(temporary)
		return "", err
	}
	return temporary, nil
}

// Publish commits validated bytes without replacing any existing filesystem
// entity. Linking a same-volume completed temporary file is an atomic no-clobber
// operation; unsupported filesystems fail instead of falling back to unsafe copy.
func Publish(path string, data []byte) error {
	temporary, err := writeTemporary(path, data)
	if err != nil {
		return err
	}
	defer os.Remove(temporary)
	if err = os.Link(temporary, path); err != nil {
		return apperr.Wrap(locales.ErrPublishExists(err.Error()), err)
	}
	return nil
}

// Replace writes a subtitle the user chose to overwrite, such as the file a
// track was imported from. The complete file is renamed over the target, so
// a failure leaves the old file intact and never a partial one. Renaming
// replaces the directory entry: a hard-linked alias keeps the old content.
func Replace(path string, data []byte) error {
	temporary, err := writeTemporary(path, data)
	if err != nil {
		return apperr.Wrap(locales.ErrSaveFile(err.Error()), err)
	}
	if err = os.Rename(temporary, path); err != nil {
		os.Remove(temporary)
		return apperr.Wrap(locales.ErrSaveFile(err.Error()), err)
	}
	return nil
}
