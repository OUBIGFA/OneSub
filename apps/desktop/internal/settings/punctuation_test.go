package settings

import (
	"os"
	"path/filepath"
	"testing"
)

func TestReplacePeriodsDefaultsOnWithoutOverridingSavedChoice(t *testing.T) {
	if !Default().ReplacePeriods {
		t.Fatal("not enabled by default")
	}
	for _, tt := range []struct {
		json string
		want bool
	}{{`{"version":5}`, true}, {`{"version":5,"replacePeriods":false}`, false}, {`{"version":5,"replacePeriods":true}`, true}} {
		path := filepath.Join(t.TempDir(), "settings.json")
		if err := os.WriteFile(path, []byte(tt.json), 0600); err != nil {
			t.Fatal(err)
		}
		settings, err := Load(path)
		if err != nil || settings.ReplacePeriods != tt.want {
			t.Fatal(settings.ReplacePeriods, err)
		}
		if err = Save(path, settings); err != nil {
			t.Fatal(err)
		}
		loaded, err := Load(path)
		if err != nil || loaded.ReplacePeriods != tt.want {
			t.Fatal("saved choice lost", err)
		}
	}
}
