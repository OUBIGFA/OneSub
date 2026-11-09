package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAudioExportDefaultsAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	// Older installations keep their compression choices and gain MP3 export.
	if err := os.WriteFile(path, []byte(`{"version":6,"compressionFormat":"mkv","compressionQuality":"crf18"}`), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil || s.AudioFormat != "mp3" || s.MediaProcessMode != "compress" || s.CompressionFormat != "mkv" || s.CompressionQuality != "crf18" {
		t.Fatal(s, err)
	}
	s.AudioFormat, s.MediaProcessMode = "flac", "audio"
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	again, err := Load(path)
	if err != nil || again.AudioFormat != "flac" || again.MediaProcessMode != "audio" || again.CompressionFormat != "mkv" {
		t.Fatal(again, err)
	}
}
