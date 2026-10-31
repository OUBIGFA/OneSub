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
	if err != nil || fresh.ComposeQuality != "crf20" {
		t.Fatal("new install must start at CRF 20", fresh.ComposeQuality, err)
	}
	for _, quality := range []string{"lossless", "crf18", "crf20", "crf23"} {
		s := Default()
		s.ComposeQuality, s.EditorWidth, s.StyleWidth = quality, 63, 29
		raw, err := json.Marshal(s)
		if err != nil {
			t.Fatal(err)
		}
		if err = os.WriteFile(path, raw, 0600); err != nil {
			t.Fatal(err)
		}
		loaded, err := Load(path)
		want := quality
		if quality == "lossless" {
			want = "crf20"
		}
		if err != nil || loaded.ComposeQuality != want || loaded.EditorWidth != 63 || loaded.StyleWidth != 29 {
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
	if err != nil || missing.ComposeQuality != "crf20" {
		t.Fatal("omitted quality did not use the install default", err)
	}
}
