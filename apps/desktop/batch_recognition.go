package main

import (
	"context"
	"os"
	"path/filepath"
	"strconv"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/importing"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/styles"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// batchStatus is where a file in the recognition queue is: waiting, at
// work, done, or failed.
type batchStatus string

const (
	batchWaiting   batchStatus = "waiting"
	batchRunning   batchStatus = "running"
	batchDone      batchStatus = "done"
	batchFailed    batchStatus = "failed"
	batchCancelled batchStatus = "cancelled"
)

// batchItem is one file in the recognition queue: the media to transcribe,
// where it is, and what came of it.
type batchItem struct {
	Source string
	Status batchStatus
	Stage  string
	Output string
	Error  string
}

// batchRecognitionView is the recognition queue: the files to transcribe,
// each with its state, and the controls that start or stop the work. It is
// one of the workbench's own pages, beside the single-file recognition, not
// a separate window.
func (a *NativeApp) batchRecognitionView(c *ui.Context) {
	parameterGroup(c, locales.BatchQueue(), nil, func() {
		ui.Row(c).Gap(em(c, 8)).Wrap().AlignItems(ui.Center).Children(func() {
			ui.Button(c, locales.BatchAddFiles()).Disabled(a.batchRunning).OnClick(a.addBatchFiles)
			ui.Button(c, locales.BatchRemove()).Disabled(a.batchRunning || len(a.batchItems) == 0).OnClick(a.clearBatchFiles)
			ui.Button(c, locales.BatchStart()).Disabled(a.busy || a.batchRunning || len(a.batchItems) == 0).OnClick(a.startBatchRecognition)
			if a.batchRunning {
				ui.Button(c, locales.ActionCancelTask()).OnClick(func() {
					if a.batchCancel != nil {
						a.batchCancel()
					}
				})
			}
			if len(a.batchItems) > 0 {
				muted(c, locales.BatchProgress(strconv.Itoa(a.batchDone), strconv.Itoa(len(a.batchItems)))).FontFeatures("tnum")
			}
		})
		if len(a.batchItems) == 0 {
			muted(c, locales.BatchEmpty()).FontSize(em(c, captionSize))
			return
		}
		a.batchList.Key = func(i int) any { return a.batchItems[i].Source }
		ui.List(c, &a.batchList, len(a.batchItems), func(i int) {
			item := a.batchItems[i]
			ui.Row(c).Gap(em(c, 12)).AlignItems(ui.Center).Padding(em(c, 4), em(c, 6)).Children(func() {
				ui.Text(c, filepath.Base(item.Source)).SingleLine().Tooltip(item.Source).Grow(1)
				state := muted(c, batchStateLabel(item)).SingleLine().Tooltip(item.Error)
				switch item.Status {
				case batchFailed:
					state.TextColor(c.Theme().Danger)
				case batchDone:
					state.TextColor(c.Theme().Success)
				case batchRunning:
					state.TextColor(c.Theme().Accent)
				}
				if item.Stage != "" {
					muted(c, item.Stage).SingleLine().FontSize(em(c, captionSize))
				}
			})
		}).Grow(1).MaxHeight(em(c, 320)).Label(locales.A11yBatchList())
	})
}

// batchStateLabel says where a file in the queue is, in the app's language.
func batchStateLabel(item batchItem) string {
	switch item.Status {
	case batchWaiting:
		return locales.BatchWaiting()
	case batchRunning:
		return locales.BatchRunning()
	case batchDone:
		return locales.BatchDone()
	case batchFailed:
		return locales.BatchFailed()
	case batchCancelled:
		return locales.StatusCancelled()
	}
	return locales.BatchWaiting()
}

// addBatchFiles adds media files to the queue. The workbench's own files are
// untouched: a batch writes its subtitles beside the media it transcribed.
func (a *NativeApp) addBatchFiles() {
	if a.busy || a.batchRunning {
		return
	}
	a.run(locales.BatchAddFiles(), func(ctx context.Context) (func(), error) {
		paths, err := a.openDialog(mygo.OpenDialogOptions{Parent: a.win, Title: locales.BatchAddFiles(), Multiple: true, Filters: []mygo.FileFilter{{Name: locales.BatchMediaFilter(), Extensions: mediaExtensions()}}})
		if err != nil || len(paths) == 0 {
			return nil, err
		}
		return func() { a.queueBatchFiles(paths) }, nil
	})
}

// mediaExtensions are the files a batch can transcribe: the video and audio
// formats the importer knows.
func mediaExtensions() []string {
	extensions := []string{}
	for _, kind := range importing.Extensions {
		if kind == "srt" || kind == "vtt" || kind == "ass" || kind == "ssa" {
			continue
		}
		extensions = append(extensions, kind)
	}
	return extensions
}

// queueBatchFiles adds the media files not already in the queue. A file
// already queued is not added twice.
func (a *NativeApp) queueBatchFiles(paths []string) {
	for _, path := range paths {
		absolute, err := filepath.Abs(path)
		if err != nil {
			continue
		}
		if importing.Kind(absolute) != "video" && importing.Kind(absolute) != "audio" {
			continue
		}
		if info, err := os.Stat(absolute); err != nil || !info.Mode().IsRegular() {
			continue
		}
		known := false
		for _, item := range a.batchItems {
			if strings.EqualFold(item.Source, absolute) {
				known = true
				break
			}
		}
		if known {
			continue
		}
		a.batchItems = append(a.batchItems, batchItem{Source: absolute, Status: batchWaiting})
	}
	a.batchDone = 0
	a.saveBatchSources()
}

// saveBatchSources keeps the queue in the settings, so a long list of files
// survives a restart of the app.
func (a *NativeApp) saveBatchSources() {
	sources := make([]string, 0, len(a.batchItems))
	for _, item := range a.batchItems {
		sources = append(sources, item.Source)
	}
	a.settings.BatchSources = sources
	a.saveSettings()
}

// restoreBatchSources brings the queue back after a restart. The files that
// are gone are dropped; the rest wait again.
func (a *NativeApp) restoreBatchSources() {
	for _, source := range a.settings.BatchSources {
		if _, err := os.Stat(source); err != nil {
			continue
		}
		a.batchItems = append(a.batchItems, batchItem{Source: source, Status: batchWaiting})
	}
}

// clearBatchFiles empties the queue. The subtitles already written stay.
func (a *NativeApp) clearBatchFiles() {
	if a.batchRunning {
		return
	}
	a.batchItems = nil
	a.batchDone = 0
	a.saveBatchSources()
}

// startBatchRecognition transcribes every file in the queue, one after
// another. Each file's subtitles are written beside the media; a file that
// fails does not stop the ones after it.
func (a *NativeApp) startBatchRecognition() {
	if a.busy || a.batchRunning || len(a.batchItems) == 0 {
		return
	}
	prefs, style, aspect := a.settings, a.currentStyle, a.frameAspect
	t := a.toolset()
	if _, _, err := t.recognitionModels(prefs); err != nil {
		a.showError(locales.BatchRecognition(), err.Error())
		a.status = locales.StatusFailed()
		return
	}
	ctx, cancel := context.WithCancel(context.Background())
	a.batchCancel = cancel
	a.batchRunning = true
	// The queue is a task of its own: the workbench's other tasks wait for
	// it, as they wait for any other running task.
	a.busy = true
	a.status = locales.BatchRecognition()
	items := append([]batchItem{}, a.batchItems...)
	go func() {
		done := 0
		for i := range items {
			if ctx.Err() != nil {
				break
			}
			path, err := a.transcribeBatchFile(ctx, t, items[i].Source, prefs, style, aspect)
			switch {
			case ctx.Err() != nil:
				a.updateBatchItem(i, func(item *batchItem) {
					item.Status = batchCancelled
					item.Stage = ""
				})
			case err != nil:
				a.updateBatchItem(i, func(item *batchItem) {
					item.Status = batchFailed
					item.Error = err.Error()
					item.Stage = ""
				})
			default:
				done++
				a.updateBatchItem(i, func(item *batchItem) {
					item.Status = batchDone
					item.Output = path
					item.Stage = ""
				})
			}
			a.batchUpdate(func() { a.batchDone = done })
		}
		a.batchUpdate(func() {
			a.batchRunning = false
			a.batchCancel = nil
			a.batchDone = done
			a.busy = false
			if failed := a.batchFailures(); failed > 0 {
				a.status = locales.BatchDoneWithFailures(strconv.Itoa(done), strconv.Itoa(len(a.batchItems)), strconv.Itoa(failed))
				return
			}
			if ctx.Err() != nil {
				a.status = locales.StatusCancelled()
				return
			}
			a.status = locales.BatchDoneAll(strconv.Itoa(done))
		})
	}()
}

// batchFailures is how many files in the queue did not finish.
func (a *NativeApp) batchFailures() int {
	failed := 0
	for _, item := range a.batchItems {
		if item.Status == batchFailed {
			failed++
		}
	}
	return failed
}

// updateBatchItem changes one file's state in the queue, on the UI thread.
func (a *NativeApp) updateBatchItem(index int, change func(*batchItem)) {
	a.batchUpdate(func() {
		if index < 0 || index >= len(a.batchItems) {
			return
		}
		change(&a.batchItems[index])
	})
}

// batchUpdate runs a change to the queue on the UI thread. Without a window
// there is no UI thread, and the change is applied as it comes.
func (a *NativeApp) batchUpdate(change func()) {
	if a.win == nil {
		change()
		return
	}
	a.win.Update(change)
}

// transcribeBatchFile transcribes one file and publishes its subtitles
// beside it, the same way the single-file recognition does. The workbench's
// own subtitles are not touched.
func (a *NativeApp) transcribeBatchFile(ctx context.Context, t toolset, source string, prefs settings.Settings, style styles.Style, aspect float64) (string, error) {
	step := func(s recognitionStep) {
		label := recognitionStepLabel(s)
		a.batchUpdate(func() {
			for i := range a.batchItems {
				if strings.EqualFold(a.batchItems[i].Source, source) {
					a.batchItems[i].Status = batchRunning
					a.batchItems[i].Stage = label
					break
				}
			}
		})
	}
	srt, _, cleanup, err := t.recognize(ctx, prefs, source, step)
	if err != nil {
		return "", err
	}
	defer cleanup()
	data, _, err := t.recognitionFile(ctx, srt, prefs, style, aspect)
	if err != nil {
		return "", err
	}
	if err = ctx.Err(); err != nil {
		return "", err
	}
	// Only the subtitle file is published beside the media. Nothing else is
	// written to the target folder.
	path, err := publishRecognition(source, prefs.RecognitionDir, prefs.RecognitionFormat, data)
	if err != nil {
		return "", err
	}
	return path, nil
}
