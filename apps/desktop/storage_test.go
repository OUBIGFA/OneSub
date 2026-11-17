package main

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/models"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
)

func storageFixture(t *testing.T, root, name, text string) string {
	t.Helper()
	p := filepath.Join(root, filepath.FromSlash(name))
	if err := os.MkdirAll(filepath.Dir(p), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(p, []byte(text), 0600); err != nil {
		t.Fatal(err)
	}
	return p
}

func TestStorageInventoryProtectsUserFilesAndActiveProjects(t *testing.T) {
	root := t.TempDir()
	storageFixture(t, root, "cache/worker/job/input.wav", "audio")
	storageFixture(t, root, "cache/worker/fonts/font.ttf", "font")
	storageFixture(t, root, "cache/translation/result.json", "translated")
	storageFixture(t, root, "settings.json", "keep")
	storageFixture(t, root, "secrets/key", "keep")
	model, _ := models.ByID("Qwen3-ForcedAligner-Ultra")
	storageFixture(t, root, "models/"+model.ID+"/local_head/model.safetensors.partial", "partial")
	storageFixture(t, root, "models/"+model.ID+"/local_head/model.safetensors", "weights")
	storageFixture(t, root, "models/"+model.ID+"/config.json.partial/user-file", "keep")
	for _, name := range []string{"active", "old", "pending"} {
		storageFixture(t, root, "projects/"+name+".onesub/project.json", `{"schema":2,"id":"`+name+`"}`)
		storageFixture(t, root, "projects/"+name+".onesub/project.sqlite", "database")
	}
	storageFixture(t, root, "projects/not-a-project/notes.txt", "keep")
	groups, err := scanStorage(context.Background(), root, nil, []string{filepath.Join(root, "projects/active.onesub"), filepath.Join(root, "projects/pending.onesub")})
	if err == nil {
		t.Fatal("a directory posing as a download partial was accepted")
	}
	if groups["worker"].Bytes != 5 || groups["fonts"].Bytes != 4 || groups["translation"].Bytes != 10 || groups["downloads"].Bytes != 7 {
		t.Fatal(groups)
	}
	projects := groups["projects"].Items
	if len(projects) != 1 || filepath.Base(projects[0].Path) != "old.onesub" {
		t.Fatal(projects)
	}
	for _, group := range groups {
		for _, item := range group.Items {
			if filepath.Base(item.Path) == "settings.json" || filepath.Ext(item.Path) == "safetensors" {
				t.Fatal("functional data selected", item)
			}
		}
	}
}

func TestStorageRecycleStopsOnFailureAndKeepsUnrelatedFiles(t *testing.T) {
	root, bin := t.TempDir(), t.TempDir()
	storageFixture(t, root, "cache/translation/a", "first")
	storageFixture(t, root, "cache/translation/b", "second")
	storageFixture(t, root, "settings.json", "keep")
	groups, err := scanStorage(context.Background(), root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	count := 0
	result, err := recycleStorage(context.Background(), groups["translation"].Items, func(path string) error {
		count++
		if count == 2 {
			return errors.New("recycle unavailable")
		}
		return os.Rename(path, filepath.Join(bin, filepath.Base(path)))
	})
	if err == nil || result.Bytes != 5 || result.Count != 1 {
		t.Fatal(result, err)
	}
	if _, err = os.Stat(filepath.Join(root, "cache/translation/b")); err != nil {
		t.Fatal(err)
	}
	if _, err = os.Stat(filepath.Join(root, "settings.json")); err != nil {
		t.Fatal(err)
	}
}

func TestStorageRefusesChangedDirectoryLinks(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	storageFixture(t, root, "cache/translation/a", "cache")
	storageFixture(t, outside, "keep", "user file")
	groups, err := scanStorage(context.Background(), root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	item := groups["translation"].Items[0]
	// A changed candidate must be rejected at execution, even after a scan.
	if err = os.Rename(item.Path, item.Path+".saved"); err != nil {
		t.Fatal(err)
	}
	if err = os.Symlink(filepath.Join(outside, "keep"), item.Path); err != nil {
		t.Skip("symlink creation unavailable", err)
	}
	_, err = recycleStorage(context.Background(), []storageItem{item}, func(string) error { t.Fatal("link reached recycle"); return nil })
	if err == nil {
		t.Fatal("changed link accepted")
	}
}

func TestStorageInventoryExcludesProjectWithLiveWriter(t *testing.T) {
	root := t.TempDir()
	dir := filepath.Join(root, "projects")
	if err := os.MkdirAll(dir, 0700); err != nil {
		t.Fatal(err)
	}
	store, err := project.Create(filepath.Join(dir, "busy.onesub"), "", "", subtitle.Document{}, "busy")
	if err != nil {
		t.Fatal(err)
	}
	defer store.Close()
	groups, err := scanStorage(context.Background(), root, nil, nil)
	if err != nil {
		t.Fatal(err)
	}
	if len(groups["projects"].Items) != 0 {
		t.Fatal("live writer selected", groups["projects"])
	}
}

func TestStorageRejectsEscapingAndLinkedParentPaths(t *testing.T) {
	root, outside := t.TempDir(), t.TempDir()
	external := storageFixture(t, outside, "file", "keep")
	if _, err := storageSize(context.Background(), root, external); err == nil {
		t.Fatal("external file accepted")
	}
	if _, err := storageSize(context.Background(), root, root); err == nil {
		t.Fatal("data root accepted")
	}
	if err := os.MkdirAll(filepath.Join(root, "cache"), 0700); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink(outside, filepath.Join(root, "cache/translation")); err != nil {
		t.Skip("symlink creation unavailable", err)
	}
	groups, err := scanStorage(context.Background(), root, nil, nil)
	if err == nil || len(groups["translation"].Items) != 0 {
		t.Fatal("linked cache selected", groups, err)
	}
}
