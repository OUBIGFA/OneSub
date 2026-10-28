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

// Replace is the confirmed overwrite of save and save-as: the complete file
// takes the name, and a hard-linked alias keeps the previous content.
func TestReplaceWritesWholeFileAndKeepsAliases(t *testing.T) {
	root := t.TempDir()
	path := filepath.Join(root, "lesson.srt")
	alias := filepath.Join(root, "alias.srt")
	if err := os.WriteFile(path, []byte("old"), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.Link(path, alias); err != nil {
		t.Fatal(err)
	}
	if err := Replace(path, []byte("new content")); err != nil {
		t.Fatal(err)
	}
	if raw, _ := os.ReadFile(path); string(raw) != "new content" {
		t.Fatal(string(raw))
	}
	if raw, _ := os.ReadFile(alias); string(raw) != "old" {
		t.Fatal("alias changed", string(raw))
	}
	entries, _ := os.ReadDir(root)
	if len(entries) != 2 {
		t.Fatal("temporary file left behind", entries)
	}
	if err := Replace(filepath.Join(root, "missing", "x.srt"), []byte("x")); err == nil {
		t.Fatal("missing folder accepted")
	}
}
