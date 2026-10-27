package styles

import (
	"os"
	"path/filepath"
	"testing"
)

func TestFontSignatureDetectsInstallChangeAndUninstall(t *testing.T) {
	root := filepath.Join(t.TempDir(), "Fonts")
	before := fontSignature([]string{root})
	if err := os.Mkdir(root, 0700); err != nil {
		t.Fatal(err)
	}
	font := filepath.Join(root, "test.ttf")
	if err := os.WriteFile(font, []byte("font metadata fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	installed := fontSignature([]string{root})
	if installed == before {
		t.Fatal("installation not detected")
	}
	if err := os.WriteFile(font, []byte("changed font metadata fixture"), 0600); err != nil {
		t.Fatal(err)
	}
	if changed := fontSignature([]string{root}); changed == installed {
		t.Fatal("change not detected")
	}
	os.Remove(font)
	if removed := fontSignature([]string{root}); removed != before {
		t.Fatal("uninstall not detected")
	}
}
