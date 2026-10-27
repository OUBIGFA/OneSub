package subtitle

import (
	"fmt"
	"os"
	"path/filepath"
)

// Publish commits validated bytes without replacing any existing filesystem
// entity. Linking a same-volume completed temporary file is an atomic no-clobber
// operation; unsupported filesystems fail instead of falling back to unsafe copy.
func Publish(path string, data []byte) error {
	file, err := os.CreateTemp(filepath.Dir(path), ".onesub-export-*")
	if err != nil {
		return err
	}
	temporary := file.Name()
	defer os.Remove(temporary)
	if _, err = file.Write(data); err != nil {
		file.Close()
		return err
	}
	if err = file.Sync(); err != nil {
		file.Close()
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	if err = os.Link(temporary, path); err != nil {
		return fmt.Errorf("无法发布；请使用本地磁盘上的新文件名，已有文件未被覆盖：%w", err)
	}
	return nil
}
