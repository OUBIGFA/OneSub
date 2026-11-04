package main

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/models"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// Only catalog assets and their own download partials belong to a model.
// Never recycle an arbitrary user-selected directory or unrelated files in it.
func modelAssetPaths(model models.Model, dir string) ([]string, error) {
	paths := []string{}
	for _, file := range model.Files {
		for _, name := range []string{file.Name, file.Name + ".partial"} {
			path := filepath.Join(dir, name)
			info, err := os.Lstat(path)
			if errors.Is(err, os.ErrNotExist) {
				continue
			}
			if err != nil {
				return nil, err
			}
			if info.IsDir() {
				return nil, fmt.Errorf("model asset is a directory: %s", name)
			}
			paths = append(paths, path)
		}
	}
	return paths, nil
}

// MyGo's native TrashItem refuses a permanent delete if the drive or recycle
// bin cannot accept the item. No Remove/RemoveAll fallback is permitted here.
func recycleModelAssets(ctx context.Context, model models.Model, dir string) error {
	paths, err := modelAssetPaths(model, dir)
	if err != nil {
		return err
	}
	if len(paths) == 0 {
		return os.ErrNotExist
	}
	for _, path := range paths {
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := mygo.Shell.TrashItem(path); err != nil {
			return err
		}
	}
	return nil
}

func (a *NativeApp) modelRecycleView(c *ui.Context) {
	if a.modelRecycle == "" {
		return
	}
	model, err := models.ByID(a.modelRecycle)
	if err != nil {
		a.modelRecycle = ""
		return
	}
	open := true
	choice := ui.AlertDialog(c, &open, locales.ModelRecycleTitle(), locales.ModelRecycleNote(), locales.ActionCancel(), locales.ModelDelete())
	if !open {
		a.modelRecycle = ""
	}
	if choice != 1 || a.busy {
		return
	}
	dir := a.modelPath(model.ID)
	a.run(locales.TaskRecycleModel(), func(ctx context.Context) (func(), error) {
		err := recycleModelAssets(ctx, model, dir)
		// Even a failed/cancelled multi-file operation may have moved some
		// files. Refresh availability on every outcome, and keep the paths.
		a.win.Update(func() {
			delete(a.modelStates, model.ID)
			a.refreshDownloadedAsrModels()
		})
		if err != nil && !errors.Is(err, context.Canceled) {
			err = fmt.Errorf("%s: %w", locales.ErrModelRecycle(), err)
		}
		return func() { a.status = locales.ModelRecycled() }, err
	})
}
