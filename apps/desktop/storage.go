package main

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"io/fs"
	"os"
	"path/filepath"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/models"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
)

type storageItem struct {
	Root, Path string
	Bytes      int64
	Project    bool
}
type storageGroup struct {
	Items   []storageItem
	Bytes   int64
	Skipped int
}
type storageResult struct {
	Count int
	Bytes int64
}

// storageBoundary rejects escaping paths and parent links before enumeration.
func storageBoundary(root, path string) error {
	root, err := filepath.Abs(root)
	if err != nil {
		return err
	}
	path, err = filepath.Abs(path)
	if err != nil {
		return err
	}
	rel, err := filepath.Rel(root, path)
	if err != nil || rel == "." || rel == ".." || strings.HasPrefix(rel, ".."+string(filepath.Separator)) || filepath.IsAbs(rel) {
		return errors.New("cleanup path is outside its data directory")
	}
	for parent := path; ; parent = filepath.Dir(parent) {
		info, err := os.Lstat(parent)
		if err != nil {
			return err
		}
		if info.Mode()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			return errors.New("cleanup refuses a linked path: " + parent)
		}
		if strings.EqualFold(parent, root) {
			break
		}
	}
	return nil
}

// storageSize measures each candidate once and never follows descendant links.
func storageSize(ctx context.Context, root, path string) (int64, error) {
	if err := storageBoundary(root, path); err != nil {
		return 0, err
	}
	var size int64
	err := filepath.WalkDir(path, func(path string, entry fs.DirEntry, walkErr error) error {
		if walkErr != nil {
			return walkErr
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if entry.Type()&(os.ModeSymlink|os.ModeIrregular) != 0 {
			return errors.New("cleanup refuses a linked path: " + path)
		}
		if entry.IsDir() {
			return nil
		}
		info, err := entry.Info()
		if err != nil {
			return err
		}
		if !info.Mode().IsRegular() {
			return errors.New("cleanup refuses a special file: " + path)
		}
		size += info.Size()
		return nil
	})
	return size, err
}

// A writer in another OneSub/MCP process holds writer.lock exclusively.
// Do not open the database or migrate an old project just to inspect storage.
func storageProjectAvailable(path string) bool {
	lock, err := os.OpenFile(filepath.Join(path, "writer.lock"), os.O_RDWR, 0)
	if errors.Is(err, os.ErrNotExist) {
		return true
	}
	if err != nil {
		return false
	}
	return lock.Close() == nil
}

func scanStorage(ctx context.Context, root string, modelPaths map[string]string, protected []string) (map[string]storageGroup, error) {
	groups := map[string]storageGroup{}
	var failures []error
	add := func(kind, owner, path string, isProject bool) {
		group := groups[kind]
		size, err := storageSize(ctx, owner, path)
		if err != nil {
			group.Skipped++
			failures = append(failures, err)
		} else {
			group.Items = append(group.Items, storageItem{owner, path, size, isProject})
			group.Bytes += size
		}
		groups[kind] = group
	}
	for _, folder := range []struct{ kind, relative string }{{"worker", "cache/worker"}, {"translation", "cache/translation"}, {"logs", "logs"}, {"projects", "projects"}} {
		dir := filepath.Join(root, filepath.FromSlash(folder.relative))
		if _, err := os.Lstat(dir); errors.Is(err, os.ErrNotExist) {
			continue
		}
		// Validate ancestors before ReadDir so a junction cannot enumerate
		// someone else's files. The walk itself never follows descendant links.
		if err := storageBoundary(root, dir); err != nil {
			failures = append(failures, err)
			continue
		}
		entries, err := os.ReadDir(dir)
		if err != nil {
			return groups, err
		}
		for _, entry := range entries {
			path := filepath.Join(dir, entry.Name())
			kind := folder.kind
			if kind == "worker" && entry.Name() == "fonts" {
				kind = "fonts"
			}
			if kind == "projects" {
				keep := false
				for _, active := range protected {
					if active != "" && strings.EqualFold(filepath.Clean(active), filepath.Clean(path)) {
						keep = true
						break
					}
				}
				if keep || !storageProjectAvailable(path) {
					group := groups[kind]
					group.Skipped++
					groups[kind] = group
					continue
				}
				if !entry.IsDir() || !strings.EqualFold(filepath.Ext(path), ".onesub") {
					continue
				}
				raw, err := os.ReadFile(filepath.Join(path, "project.json"))
				var manifest project.Manifest
				if err != nil || json.Unmarshal(raw, &manifest) != nil || manifest.ID == "" || (manifest.Schema != 1 && manifest.Schema != 2) {
					continue
				}
				if info, err := os.Stat(filepath.Join(path, "project.sqlite")); err != nil || !info.Mode().IsRegular() {
					continue
				}
			}
			add(kind, root, path, kind == "projects")
		}
	}
	seen := map[string]bool{}
	for _, model := range models.Catalog() {
		dir := modelPaths[model.ID]
		if dir == "" {
			dir = filepath.Join(root, "models", model.ID)
		}
		for _, file := range model.Files {
			path := filepath.Join(dir, filepath.FromSlash(file.Name)+".partial")
			if seen[strings.ToLower(path)] {
				continue
			}
			seen[strings.ToLower(path)] = true
			info, err := os.Lstat(path)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				failures = append(failures, err)
				continue
			}
			if !info.Mode().IsRegular() {
				failures = append(failures, fmt.Errorf("download partial is not a regular file: %s", path))
				continue
			}
			add("downloads", dir, path, false)
		}
	}
	return groups, errors.Join(failures...)
}

// A failure stops the operation without a permanent-delete fallback. Return
// only successful items so the interface never reports partial work as done.
func recycleStorage(ctx context.Context, items []storageItem, trash func(string) error) (storageResult, error) {
	var result storageResult
	for _, item := range items {
		if err := ctx.Err(); err != nil {
			return result, err
		}
		size, err := storageSize(ctx, item.Root, item.Path)
		if err != nil {
			return result, err
		}
		if item.Project && !storageProjectAvailable(item.Path) {
			return result, fmt.Errorf("project is in use: %s", filepath.Base(item.Path))
		}
		if err = trash(item.Path); err != nil {
			return result, err
		}
		result.Count++
		result.Bytes += size
	}
	return result, nil
}
