package main

import (
	"context"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo"
)

// The list's import is explicitly a subtitle replacement, never a new media
// selection. Passing the current media as an explicit companion bypasses
// same-name discovery (for example, lesson.mp4 with lesson-zh.srt).
func (a *NativeApp) pickSubtitle() {
	a.run(locales.ListImport(), func(context.Context) (func(), error) {
		paths, err := a.openDialog(mygo.OpenDialogOptions{Parent: a.win, Title: locales.ListImport(), Filters: []mygo.FileFilter{{Name: locales.ListImport(), Extensions: []string{"srt", "vtt", "ass", "ssa"}}}})
		if err != nil || len(paths) == 0 {
			return nil, err
		}
		return func() { a.importSubtitle(paths[0]) }, nil
	})
}

func (a *NativeApp) importSubtitle(path string) {
	paths := []string{path}
	if a.snapshot != nil && a.snapshot.Manifest.MediaPath != "" {
		paths = append(paths, a.snapshot.Manifest.MediaPath)
	}
	// The shared entry validates both files and offers save/discard/cancel
	// before replacing the recovery project. Source files are never overwritten.
	a.importPaths(paths)
}
