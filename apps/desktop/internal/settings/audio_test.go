package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestAudioExportDefaultsAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	// Older installations migrate retired H.264 compression CRF to SVT-AV1 near-lossless
	// while keeping their container format and gaining MP3 export.
	if err := os.WriteFile(path, []byte(`{"version":6,"compressionFormat":"mkv","compressionQuality":"crf18"}`), 0600); err != nil {
		t.Fatal(err)
	}
	s, err := Load(path)
	if err != nil || s.AudioFormat != "mp3" || s.MediaProcessMode != "compress" || s.CompressionFormat != "mkv" || s.CompressionQuality != "near-lossless" {
		t.Fatal(s, err)
	}
	s.AudioFormat, s.MediaProcessMode, s.CompressionQuality = "flac", "audio", "high"
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	again, err := Load(path)
	if err != nil || again.AudioFormat != "flac" || again.MediaProcessMode != "audio" || again.CompressionFormat != "mkv" || again.CompressionQuality != "high" {
		t.Fatal(again, err)
	}
}
