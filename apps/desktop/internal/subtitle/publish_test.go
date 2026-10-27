package subtitle

import (
	"os"
	"path/filepath"
	"testing"
)

func TestPublishNeverReplacesSourceOrAlias(t *testing.T) {
	root := t.TempDir()
	source := filepath.Join(root, "source.srt")
	alias := filepath.Join(root, "alias.srt")
	if err := os.WriteFile(source, []byte("original"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(source, alias); err != nil {
		t.Fatal(err)
	}
	for _, path := range []string{source, alias} {
		if err := Publish(path, []byte("replacement")); err == nil {
			t.Fatal("replaced", path)
		}
	}
	raw, err := os.ReadFile(source)
	if err != nil || string(raw) != "original" {
		t.Fatal("original changed")
	}
	output := filepath.Join(root, "export.srt")
	if err = Publish(output, []byte("valid")); err != nil {
		t.Fatal(err)
	}
	raw, err = os.ReadFile(output)
	if err != nil || string(raw) != "valid" {
		t.Fatal("export invalid")
	}
}
