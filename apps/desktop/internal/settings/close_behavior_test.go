package settings

import (
	"os"
	"path/filepath"
	"testing"
)

// Defaults apply to a new install or an omitted field, not an explicit saved
// exit preference. No migration can infer whether an old false was intentional.
func TestCloseDefaultsToTrayWithoutOverwritingChoices(t *testing.T) {
	if !Default().CloseToTray {
		t.Fatal("new installs must close to tray")
	}
	path := filepath.Join(t.TempDir(), "settings.json")
	prefs, err := Load(path)
	if err != nil || !prefs.CloseToTray {
		t.Fatal("missing settings did not use tray", err)
	}
	for _, test := range []struct {
		json string
		tray bool
	}{
		{`{"version":5}`, true},
		{`{"version":1,"closeToTray":false}`, false},
		{`{"version":5,"closeToTray":false}`, false},
		{`{"version":5,"closeToTray":true}`, true},
	} {
		if err = os.WriteFile(path, []byte(test.json), 0600); err != nil {
			t.Fatal(err)
		}
		prefs, err = Load(path)
		if err != nil || prefs.CloseToTray != test.tray {
			t.Fatal(test, prefs.CloseToTray, err)
		}
		if err = Save(path, prefs); err != nil {
			t.Fatal(err)
		}
		prefs, err = Load(path)
		if err != nil || prefs.CloseToTray != test.tray {
			t.Fatal("choice changed after save", test, err)
		}
	}
}
