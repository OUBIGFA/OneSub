package main

import (
	"context"
	"errors"
	"fmt"
	"maps"
	"path/filepath"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

type storageState struct {
	groups            map[string]storageGroup
	reading, cleaning bool
	confirm           string
	items             []storageItem
	message           string
}

func (a *NativeApp) protectedStorageProjects() []string {
	paths := []string{a.settings.TranslationRecovery}
	if a.snapshot != nil {
		paths = append(paths, a.snapshot.Path)
	}
	if a.settings.Processing != nil {
		paths = append(paths, a.settings.Processing.ProjectPath)
	}
	return paths
}

func (a *NativeApp) refreshStorage() {
	if a.storage.reading || a.storage.cleaning || a.win == nil {
		return
	}
	a.storage.reading = true
	root, paths, protected := a.dataRoot, maps.Clone(a.settings.ModelPaths), a.protectedStorageProjects()
	go func() {
		groups, err := scanStorage(context.Background(), root, paths, protected)
		a.win.Update(func() {
			a.storage.groups, a.storage.reading = groups, false
			if err != nil {
				a.showError(locales.SettingsStorage(), err.Error())
			} else {
				a.clearError(locales.SettingsStorage())
			}
		})
	}()
}

func storageBytes(value int64) string {
	for _, unit := range []struct {
		size  int64
		label string
	}{{1 << 30, "GiB"}, {1 << 20, "MiB"}, {1 << 10, "KiB"}} {
		if value >= unit.size {
			return fmt.Sprintf("%.1f %s", float64(value)/float64(unit.size), unit.label)
		}
	}
	return fmt.Sprintf("%d B", value)
}

func (a *NativeApp) storageUnavailable() bool {
	return a.busy || a.storage.reading || a.storage.cleaning || a.previewRunning || a.fontCatalogLoading || a.updates.phase == updateInstalling
}

// Keep the controls in the existing settings grid. Sizes are data, never input
// to layout; loading and completed scans therefore use the same native rows.
func (a *NativeApp) storageSettingsView(c *ui.Context) {
	settingsGroup(c, locales.SettingsStorage(), func() {
		ui.Row(c).Gap(em(c, 12)).AlignItems(ui.Center).Children(func() {
			muted(c, locales.StoragePreserved()).Grow(1)
			lobeButton(c, locales.StorageRefresh()).Disabled(a.storageUnavailable()).OnClick(a.refreshStorage)
		})
		for _, row := range []struct{ kind, title string }{
			{"worker", locales.StorageWorker()}, {"fonts", locales.StorageFonts()}, {"translation", locales.StorageTranslation()},
			{"downloads", locales.StorageDownloads()}, {"logs", locales.StorageLogs()}, {"projects", locales.StorageProjects()},
		} {
			group := a.storage.groups[row.kind]
			settingsRow(c, row.title, func() ui.Element {
				return ui.Row(c).Gap(em(c, 12)).AlignItems(ui.Center).Children(func() {
					value := "—"
					if a.storage.groups != nil {
						value = storageBytes(group.Bytes)
					}
					ui.Text(c, value).Grow(1)
					lobeButton(c, locales.StorageClean()).Label(row.title + " / " + locales.StorageClean()).Disabled(a.storageUnavailable() || len(group.Items) == 0).OnClick(func() {
						a.storage.confirm = row.kind
						a.storage.items = append([]storageItem(nil), group.Items...)
					})
				})
			})
		}
		settingsRowLast(c, locales.StoragePlayer(), func() ui.Element {
			return ui.Row(c).Gap(em(c, 12)).AlignItems(ui.Center).Children(func() {
				muted(c, locales.StorageManaged()).Grow(1)
				lobeButton(c, locales.StorageClean()).Label(locales.StoragePlayer() + " / " + locales.StorageClean()).Disabled(a.storageUnavailable()).OnClick(func() { a.storage.confirm = "player"; a.storage.items = nil })
			})
		})
		muted(c, locales.StorageProjectProtection())
		muted(c, locales.StorageRecycleNote())
		stableDialogNote(c, a.storage.message, false, locales.StorageMoved("999", "999.9 GiB"), locales.StoragePlayerCleaned())
	})
}

func (a *NativeApp) storageConfirmView(c *ui.Context) {
	if a.storage.confirm == "" {
		return
	}
	kind := a.storage.confirm
	note := locales.StorageConfirmFiles()
	if kind == "projects" {
		note = locales.StorageConfirmProjects()
	}
	if kind == "translation" {
		note = locales.StorageConfirmTranslation()
	}
	if kind == "player" {
		note = locales.StorageConfirmPlayer()
	} else {
		var size int64
		for _, item := range a.storage.items {
			size += item.Bytes
		}
		note += "\n\n" + locales.StorageSelection(fmt.Sprint(len(a.storage.items)), storageBytes(size))
	}
	open := true
	choice := confirmationDialog(c, &open, locales.StorageClean(), note, locales.ActionCancel(), locales.StorageClean())
	if !open {
		a.storage.confirm = ""
	}
	if choice == 1 && !a.storageUnavailable() {
		a.storage.confirm = ""
		a.cleanStorage(kind, append([]storageItem(nil), a.storage.items...))
	}
}

func (a *NativeApp) cleanStorage(kind string, items []storageItem) {
	if a.storageUnavailable() || !a.commitEdit(true) {
		return
	}
	// An import can finish after a scan. Recheck the live protection list at
	// the action boundary instead of trusting the previously displayed total.
	for _, item := range items {
		for _, active := range a.protectedStorageProjects() {
			if active != "" && strings.EqualFold(filepath.Clean(item.Path), filepath.Clean(active)) {
				a.showError(locales.SettingsStorage(), locales.StorageProjectProtection())
				a.refreshStorage()
				return
			}
		}
	}
	a.storage.cleaning = true
	a.run(locales.StorageClean(), func(ctx context.Context) (func(), error) {
		var result storageResult
		var err error
		if kind == "player" {
			err = mygo.DefaultSession().ClearData(mygo.SiteDataCaches)
		} else {
			var other bool
			other, err = storageOtherInstance()
			if err == nil && other {
				err = errors.New(locales.StorageOtherInstance())
			}
			if err == nil {
				result, err = recycleStorage(ctx, items, mygo.Shell.TrashItem)
			}
		}
		return func() {
			a.storage.cleaning = false
			if kind == "player" && err == nil {
				a.storage.message = locales.StoragePlayerCleaned()
			} else if kind != "player" {
				a.storage.message = locales.StorageMoved(fmt.Sprint(result.Count), storageBytes(result.Bytes))
			}
			if kind == "fonts" {
				a.previewRevision++
				a.previewPending = true
				a.reloadFonts()
			}
			a.refreshStorage()
		}, err
	})
}
