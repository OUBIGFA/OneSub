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
	for _, encoder := range []string{"", "invalid", "nvenc", "cpu", "gpu"} {
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
		if encoder == "gpu" {
			want = "gpu"
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
