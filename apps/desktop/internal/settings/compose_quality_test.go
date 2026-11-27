package settings

import (
	"encoding/json"
	"os"
	"path/filepath"
	"testing"
)

func TestComposeQualityDefaultsAndRetiredChoice(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	fresh, err := Load(path)
	if err != nil || fresh.ComposeQuality != "near-lossless" {
		t.Fatal("new install must start at near-lossless", fresh.ComposeQuality, err)
	}
	for _, quality := range []string{"lossless", "crf18", "crf20", "crf23", "near-lossless", "high", "compact"} {
		s := Default()
		s.ComposeQuality, s.EditorWidth, s.SubtitleHeight = quality, 63, 29
		raw, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		loaded, err := Load(path)
		want := quality
		if quality != "near-lossless" && quality != "high" && quality != "compact" {
			want = "near-lossless"
		}
		if err != nil || loaded.ComposeQuality != want || loaded.EditorWidth != 63 || loaded.SubtitleHeight != 29 {
			t.Fatal("quality migration changed unrelated preferences", quality, loaded, err)
		}
		if err = Save(path, loaded); err != nil {
			t.Fatal(err)
		}
		again, err := Load(path)
		if err != nil || again.ComposeQuality != want {
			t.Fatal("migration did not persist", err)
		}
	}
	if err = os.WriteFile(path, []byte(`{"version":5}`), 0600); err != nil {
		t.Fatal(err)
	}
	missing, err := Load(path)
	if err != nil || missing.ComposeQuality != "near-lossless" {
		t.Fatal("omitted quality did not use the install default", err)
	}
}

func TestEncoderDefaultsAndPersistence(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	fresh, err := Load(path)
	if err != nil || fresh.ComposeEncoder != "cpu" || fresh.CompressionEncoder != "cpu" {
		t.Fatal("new install must start on cpu encoder", fresh.ComposeEncoder, fresh.CompressionEncoder, err)
	}
	for _, encoder := range []string{"", "invalid", "nvenc", "cpu", "cpu-h264", "gpu"} {
		s := Default()
		s.ComposeEncoder, s.CompressionEncoder = encoder, encoder
		raw, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		loaded, err := Load(path)
		want := "cpu"
		if encoder == "gpu" || encoder == "cpu-h264" {
			want = encoder
		}
		if err != nil || loaded.ComposeEncoder != want || loaded.CompressionEncoder != want {
			t.Fatal("encoder normalization failed", encoder, loaded.ComposeEncoder, loaded.CompressionEncoder, err)
		}
		if err = Save(path, loaded); err != nil {
			t.Fatal(err)
		}
		again, err := Load(path)
		if err != nil || again.ComposeEncoder != want || again.CompressionEncoder != want {
			t.Fatal("encoder preference did not persist", err)
		}
	}
}

func TestH264QualityIsRememberedSeparately(t *testing.T) {
	path := filepath.Join(t.TempDir(), "settings.json")
	s := Default()
	s.ComposeQuality, s.CompressionQuality = "high", "compact"
	s.ComposeEncoder, s.CompressionEncoder = "cpu-h264", "cpu-h264"
	if *s.VideoQuality(false) != "crf20" || *s.VideoQuality(true) != "crf23" {
		t.Fatal("H.264 must use the original composition/compression defaults")
	}
	*s.VideoQuality(false), *s.VideoQuality(true) = "crf18", "crf20"
	if err := Save(path, s); err != nil {
		t.Fatal(err)
	}
	loaded, err := Load(path)
	if err != nil || *loaded.VideoQuality(false) != "crf18" || *loaded.VideoQuality(true) != "crf20" {
		t.Fatal("H.264 quality was not saved", loaded, err)
	}
	loaded.ComposeEncoder, loaded.CompressionEncoder = "cpu", "gpu"
	if *loaded.VideoQuality(false) != "high" || *loaded.VideoQuality(true) != "compact" {
		t.Fatal("switching encoders replaced the perceptual quality choices")
	}
	loaded.ComposeH264Quality, loaded.CompressionH264Quality = "lossless", "high"
	if err := Save(path, loaded); err != nil {
		t.Fatal(err)
	}
	loaded, err = Load(path)
	if err != nil || loaded.ComposeH264Quality != "crf20" || loaded.CompressionH264Quality != "crf23" {
		t.Fatal("invalid fixed qualities did not restore separate defaults", err)
	}
	if err := os.WriteFile(path, []byte(`{"version":7,"composeEncoder":"cpu","composeQuality":"compact","compressionEncoder":"gpu","compressionQuality":"high"}`), 0600); err != nil {
		t.Fatal(err)
	}
	loaded, err = Load(path)
	if err != nil || loaded.ComposeEncoder != "cpu" || loaded.CompressionEncoder != "gpu" || loaded.ComposeQuality != "compact" || loaded.CompressionQuality != "high" || loaded.ComposeH264Quality != "crf20" || loaded.CompressionH264Quality != "crf23" {
		t.Fatal("existing settings changed or omitted fixed defaults", err)
	}
}
