package importing

import (
	"os"
	"path/filepath"
	"testing"
)

func files(t *testing.T, names ...string) []string {
	t.Helper()
	dir := t.TempDir()
	var paths []string
	for _, name := range names {
		path := filepath.Join(dir, name)
		if err := os.WriteFile(path, []byte("fixture"), 0600); err != nil {
			t.Fatal(err)
		}
		paths = append(paths, path)
	}
	return paths
}

// Removing either direction or truncating the stem must break these import contracts.
func TestUniquePairFromEitherEntry(t *testing.T) {
	p := files(t, "课程 01.mp4", "课程 01.srt", "课程 010.vtt", "课程.srt")
	for _, entry := range p[:2] {
		got, err := Discover([]string{entry})
		if err != nil || got.Media != p[0] || got.Subtitle != p[1] || got.Automatic == "" {
			t.Fatalf("entry %s: %+v, %v", entry, got, err)
		}
	}
}

func TestAmbiguousCandidatesRequireChoice(t *testing.T) {
	p := files(t, "lesson.mp4", "lesson.srt", "lesson.vtt")
	got, err := Discover(p[:1])
	if err != nil || got.Media != p[0] || got.Subtitle != "" || len(got.Candidates) != 2 || got.CandidateKind != "subtitle" {
		t.Fatalf("ambiguous import: %+v, %v", got, err)
	}
}

func TestExplicitPairWinsAndDuplicatesAreIgnored(t *testing.T) {
	p := files(t, "lesson.mp4", "lesson.srt", "lesson.vtt", "edited.srt")
	got, err := Discover([]string{p[0], p[3], p[0]})
	if err != nil || got.Media != p[0] || got.Subtitle != p[3] || len(got.Candidates) != 0 || got.Automatic != "" {
		t.Fatalf("explicit pair: %+v, %v", got, err)
	}
}

func TestNoCrossDirectoryOrLanguageSuffixAutoPair(t *testing.T) {
	p := files(t, "lesson.mp4", "lesson.zh-CN.srt", "less.srt")
	got, err := Discover(p[:1])
	if err != nil || got.Media != p[0] || got.Subtitle != "" {
		t.Fatalf("suffix: %+v, %v", got, err)
	}
	q := files(t, "lesson.srt")
	got, err = Discover(q)
	if err != nil || got.Media != "" || got.Subtitle != q[0] {
		t.Fatalf("directory: %+v, %v", got, err)
	}
}

func TestCaseInsensitiveStemOnWindows(t *testing.T) {
	p := files(t, "LESSON.MP4", "lesson.SRT")
	got, err := Discover(p[:1])
	if err != nil || got.Subtitle != p[1] {
		t.Fatalf("case: %+v, %v", got, err)
	}
}

func TestInvalidAndMultipleGroupsDoNotSilentlyReplace(t *testing.T) {
	p := files(t, "one.mp4", "two.mp4", "notes.txt")
	for _, paths := range [][]string{nil, p[:2], p[2:], {filepath.Join(t.TempDir(), "gone.srt")}, {t.TempDir()}} {
		if _, err := Discover(paths); err == nil {
			t.Fatalf("accepted %v", paths)
		}
	}
}
