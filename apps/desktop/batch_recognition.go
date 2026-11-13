package main

import (
	"context"
	"os"
	"path/filepath"
	"slices"
	"strconv"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/importing"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/styles"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// A queue item owns only its source and result. Recognition options belong to
// the shared settings, never to a second batch-only configuration.
type batchStatus string

const (
	batchWaiting   batchStatus = "waiting"
	batchRunning   batchStatus = "running"
	batchDone      batchStatus = "done"
	batchFailed    batchStatus = "failed"
	batchCancelled batchStatus = "cancelled"
)

type batchItem struct {
	Source string
	Status batchStatus
	Stage  string
	Output string
	Error  string
}

// The queue is an input target inside speech recognition. Its list is also a
// native drop target, so one or many files can be added without opening a
// separate page or replacing the current workbench's media and subtitles.
func (a *NativeApp) batchRecognitionView(c *ui.Context) {
	ui.Column(c).Grow(1).MinHeight(0).Padding(0, em(c, panelPad), em(c, 24), em(c, panelPad)).Gap(em(c, 8)).Children(func() {
		ui.Row(c).Gap(em(c, 8)).Wrap().AlignItems(ui.Center).Shrink(0).Children(func() {
			iconButton(c, locales.BatchAddFiles(), "folder-open").Disabled(a.busy).OnClick(a.addBatchFiles)
			lobeButton(c, locales.BatchRemove()).Disabled(a.busy || len(a.batchItems) == 0).OnClick(a.clearBatchFiles)
			ui.Spacer(c)
			done, failed := a.batchCounts()
			muted(c, locales.BatchProgress(strconv.Itoa(done+failed), strconv.Itoa(len(a.batchItems)))).FontFeatures("tnum").SingleLine()
		})
		area := ui.Column(c).Grow(1).MinHeight(0).Background(studioColors(c).panel).Border(1, studioColors(c).hairline).Radius(c.Theme().Radius).Clip().Label(locales.BatchDropArea())
		if area.FileDragOver() && !a.busy {
			area.BorderColor(c.Theme().Accent).Background(c.Theme().SurfaceHover)
		}
		area.Children(func() {
			if len(a.batchItems) == 0 {
				ui.Column(c).Fill().Center().Padding(em(c, 12)).Children(func() {
					muted(c, locales.BatchEmpty()).TextAlign(ui.Center)
				})
				return
			}
			a.batchList.Key = func(i int) any { return a.batchItems[i].Source }
			a.batchList.Label = func(i int) string {
				return filepath.Base(a.batchItems[i].Source) + " " + batchStateLabel(a.batchItems[i])
			}
			ui.List(c, &a.batchList, len(a.batchItems), func(i int) {
				item := a.batchItems[i]
				ui.Column(c).Gap(em(c, 4)).Padding(em(c, 6), em(c, 10)).Children(func() {
					ui.Row(c).Gap(em(c, 8)).AlignItems(ui.Center).Children(func() {
						ui.Text(c, filepath.Base(item.Source)).SingleLine().Tooltip(item.Source).Grow(1).MinWidth(0)
						toolIcon(c, locales.BatchRemoveFile(filepath.Base(item.Source)), "eraser").Disabled(a.busy).OnClick(func() { a.removeBatchFile(item.Source) })
					})
					ui.Row(c).Gap(em(c, 8)).Wrap().AlignItems(ui.Center).Children(func() {
						state := muted(c, batchStateLabel(item)).SingleLine().Tooltip(item.Error)
						switch item.Status {
						case batchFailed:
							state.TextColor(c.Theme().Danger)
						case batchDone:
							state.TextColor(c.Theme().Success).Tooltip(item.Output)
						case batchRunning:
							state.TextColor(c.Theme().Accent)
						}
						if item.Stage != "" {
							muted(c, item.Stage).FontSize(em(c, captionSize))
						}
					})
				})
			}).Fill().Dividers(1, c.Theme().Border).Label(locales.A11yBatchList())
		})
		if paths := area.DroppedFiles(); len(paths) > 0 && !a.busy {
			a.queueBatchFiles(paths)
		}
	})
}

func batchStateLabel(item batchItem) string {
	switch item.Status {
	case batchRunning:
		return locales.BatchRunning()
	case batchDone:
		return locales.BatchDone()
	case batchFailed:
		return locales.BatchFailed()
	case batchCancelled:
		return locales.StatusCancelled()
	default:
		return locales.BatchWaiting()
	}
}

// acceptFiles is shared by native drops, file selection and launches. An
// explicit media/subtitle pair retains the normal import/association flow.
// Multiple media files instead become one queue, never successive overwrites.
func (a *NativeApp) acceptFiles(paths []string) {
	if a.busy || len(paths) == 0 {
		return
	}
	if a.filesTargetQueue(paths) {
		a.workflowTab, a.recognitionMode = 0, 1
		a.workflowScroll.Y = 0
		a.queueBatchFiles(paths)
		return
	}
	a.importPaths(paths)
}

func (a *NativeApp) filesTargetQueue(paths []string) bool {
	media := map[string]bool{}
	for _, path := range paths {
		kind := importing.Kind(path)
		if kind != "video" && kind != "audio" {
			return false
		}
		absolute, err := filepath.Abs(path)
		if err != nil {
			return false
		}
		media[strings.ToLower(absolute)] = true
	}
	return len(media) > 1 || (len(media) > 0 && a.workflowTab == 0 && a.recognitionMode == 1)
}

func (a *NativeApp) addBatchFiles() {
	if a.busy {
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

// The same extension registry serves imports, drops and the media picker.
func mediaExtensions() []string {
	extensions := []string{}
	for _, kind := range importing.Extensions {
		if kind != "srt" && kind != "vtt" && kind != "ass" && kind != "ssa" {
			extensions = append(extensions, kind)
		}
	}
	return extensions
}

func (a *NativeApp) queueBatchFiles(paths []string) {
	if a.busy {
		return
	}
	added, rejected := 0, 0
	for _, path := range paths {
		absolute, err := filepath.Abs(path)
		if err != nil || (importing.Kind(absolute) != "video" && importing.Kind(absolute) != "audio") {
			rejected++
			continue
		}
		if info, err := os.Stat(absolute); err != nil || !info.Mode().IsRegular() {
			rejected++
			continue
		}
		if slices.ContainsFunc(a.batchItems, func(item batchItem) bool { return strings.EqualFold(item.Source, absolute) }) {
			continue
		}
		a.batchItems = append(a.batchItems, batchItem{Source: absolute, Status: batchWaiting})
		added++
	}
	a.saveBatchSources()
	a.status = locales.BatchAdded(added, len(a.batchItems))
	if rejected > 0 {
		a.showError("batch-input", locales.BatchFilesRejected(rejected))
	} else {
		a.clearError("batch-input")
	}
}

func (a *NativeApp) saveBatchSources() {
	sources := make([]string, 0, len(a.batchItems))
	for _, item := range a.batchItems {
		sources = append(sources, item.Source)
	}
	a.settings.BatchSources = sources
	a.saveSettings()
}

// Restore only regular media files and use the same deduplication rules as a
// drop. Restoring a queue never starts recognition.
func (a *NativeApp) restoreBatchSources() {
	if len(a.settings.BatchSources) == 0 {
		return
	}
	a.queueBatchFiles(a.settings.BatchSources)
	if len(a.batchItems) > 0 {
		a.recognitionMode = 1
	}
}

func (a *NativeApp) clearBatchFiles() {
	if a.busy {
		return
	}
	a.batchItems = nil
	a.saveBatchSources()
}

func (a *NativeApp) removeBatchFile(source string) {
	if a.busy {
		return
	}
	a.batchItems = slices.DeleteFunc(a.batchItems, func(item batchItem) bool { return item.Source == source })
	a.saveBatchSources()
}

func (a *NativeApp) pendingBatchCount() int {
	done, _ := a.batchCounts()
	return len(a.batchItems) - done
}

func (a *NativeApp) batchCounts() (done, failed int) {
	for _, item := range a.batchItems {
		switch item.Status {
		case batchDone:
			done++
		case batchFailed:
			failed++
		}
	}
	return
}

// A batch uses the normal task owner, including cancellation and "quit when
// finished". The shared settings are frozen once for the entire queue. Copy
// ModelPaths too: a shallow settings copy would let a later folder edit change
// the models used by the remaining files.
func (a *NativeApp) startBatchRecognition() {
	if a.busy || a.pendingBatchCount() == 0 {
		return
	}
	prefs, style, aspect := a.recognitionPreferences(), a.currentStyle, a.frameAspect
	t := a.toolset()
	if _, _, err := t.recognitionModels(prefs); err != nil {
		if err.Error() == locales.ErrModelsNeeded() {
			a.settingsOpen, a.settingsTab = true, 1
		}
		a.showError(locales.BatchRecognition(), err.Error())
		return
	}
	continuing := false
	for i := range a.batchItems {
		if a.batchItems[i].Status != batchDone {
			continuing = continuing || a.batchItems[i].Status == batchCancelled || a.batchItems[i].Status == batchFailed
			a.batchItems[i].Status = batchWaiting
			a.batchItems[i].Stage, a.batchItems[i].Error, a.batchItems[i].Output = "", "", ""
		}
	}
	items := slices.Clone(a.batchItems)
	a.runTimed(timingRecognition, continuing, locales.BatchRecognition(), func(ctx context.Context) (func(), error) {
		for i, item := range items {
			if ctx.Err() != nil {
				break
			}
			// A retry processes only unfinished files, not successful outputs.
			if item.Status == batchDone {
				continue
			}
			a.updateBatchItem(i, func(item *batchItem) { item.Status = batchRunning })
			path, err := a.transcribeBatchFile(ctx, t, item.Source, prefs, style, aspect)
			// run releases its context before queued UI updates execute. Freeze
			// cancellation now so cleanup cannot relabel a published file.
			cancelled := ctx.Err() != nil
			a.updateBatchItem(i, func(item *batchItem) {
				item.Stage = ""
				switch {
				case cancelled:
					item.Status = batchCancelled
				case err != nil:
					item.Status, item.Error = batchFailed, err.Error()
				default:
					item.Status, item.Output = batchDone, path
					a.lastOutput = path
				}
			})
		}
		return func() {
			done, failed := a.batchCounts()
			if failed > 0 {
				a.status = locales.BatchDoneWithFailures(strconv.Itoa(done), strconv.Itoa(failed), strconv.Itoa(len(a.batchItems)))
			} else {
				a.status = locales.BatchDoneAll(strconv.Itoa(done))
			}
		}, ctx.Err()
	})
}

func (a *NativeApp) updateBatchItem(index int, change func(*batchItem)) {
	a.batchUpdate(func() {
		if index >= 0 && index < len(a.batchItems) {
			change(&a.batchItems[index])
		}
	})
}

func (a *NativeApp) batchUpdate(change func()) {
	if a.win == nil {
		change()
		return
	}
	a.win.Update(change)
}

// The queue calls the same recognition and format/punctuation/style pipeline
// as the current file. The only published artifact is the requested subtitle;
// an empty RecognitionDir means beside each source, otherwise the chosen folder.
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
	return publishRecognition(source, prefs.RecognitionDir, prefs.RecognitionFormat, data)
}
