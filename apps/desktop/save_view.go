package main

import (
	"context"
	"crypto/sha256"
	"encoding/hex"
	"fmt"
	"path/filepath"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// Edits always reach the project; "saved" means written to a subtitle file.
// Each track (0 source, 1 translation) remembers the file it was last
// written to and a digest of what was written, so undoing back to that
// state is not reported as unsaved.

func trackHash(doc subtitle.Document, track int) string {
	h := sha256.New()
	for _, cue := range doc.Cues {
		text := subtitle.SourceText(cue)
		if track == 1 {
			text = "\x00"
			if cue.Translated {
				text = cue.Text
			}
		}
		fmt.Fprintf(h, "%s\x1f%d\x1f%d\x1f%s\x1e", cue.ID, cue.StartMs, cue.EndMs, text)
	}
	return hex.EncodeToString(h.Sum(nil))
}

// setSnapshot replaces the shown project state and re-evaluates which tracks
// differ from their files.
func (a *NativeApp) setSnapshot(state project.Snapshot) {
	a.snapshot = &state
	a.selected = min(a.selected, len(state.Document.Cues)-1)
	if a.selected < 0 && len(state.Document.Cues) > 0 {
		a.selected = 0
	}
	a.previewPending = true
	a.refreshUnsaved()
}
func (a *NativeApp) refreshUnsaved() {
	for track := range a.unsaved {
		a.unsaved[track] = a.snapshot != nil && trackHash(a.snapshot.Document, track) != a.savedHash[track]
	}
}
func (a *NativeApp) markSaved(track int, path string) {
	a.savedPath[track] = path
	a.savedHash[track] = ""
	if a.snapshot != nil {
		a.savedHash[track] = trackHash(a.snapshot.Document, track)
	}
	// Subtitles without a file of their own, as recognition results, stay
	// unsaved until written somewhere.
	if track == 0 && path == "" && a.snapshot != nil && len(a.snapshot.Document.Cues) > 0 {
		a.savedHash[track] = ""
	}
	a.refreshUnsaved()
}
func (a *NativeApp) trackUnsaved(track int) bool {
	return a.unsaved[track] || a.editPending && track == a.subtitleTab
}
func (a *NativeApp) trackHasContent(track int) bool {
	if a.snapshot == nil {
		return false
	}
	for _, cue := range a.snapshot.Document.Cues {
		if track == 0 || cue.Translated {
			return true
		}
	}
	return false
}

// originalSubtitlePath is the imported subtitle file the source track may
// overwrite; recognition output inside the app's data is not one.
func (a *NativeApp) originalSubtitlePath() string {
	if a.snapshot == nil || a.snapshot.Manifest.SubtitlePath == "" {
		return ""
	}
	path := a.snapshot.Manifest.SubtitlePath
	if rel, err := filepath.Rel(a.dataRoot, path); err == nil && !strings.HasPrefix(rel, "..") && !filepath.IsAbs(rel) {
		return ""
	}
	return path
}

// trackFormats lists the formats a track can be written in. ASS/SSA keeps
// its own format first; SRT and VTT are always available.
func (a *NativeApp) trackFormats() []string {
	formats := []string{"srt", "vtt"}
	if a.snapshot != nil && (a.snapshot.Document.Format == "ass" || a.snapshot.Document.Format == "ssa") {
		formats = append([]string{a.snapshot.Document.Format}, formats...)
	} else if a.snapshot != nil && a.snapshot.Document.Format == "vtt" {
		formats = []string{"vtt", "srt"}
	}
	return formats
}

// defaultTrackPath proposes a name next to the subtitle or the video:
// lesson.srt for the source, lesson.zh-CN.srt for the translation.
func (a *NativeApp) defaultTrackPath(track int) string {
	if a.savedPath[track] != "" {
		return a.savedPath[track]
	}
	base := ""
	if original := a.originalSubtitlePath(); original != "" {
		base = strings.TrimSuffix(original, filepath.Ext(original))
	} else if a.snapshot != nil && a.snapshot.Manifest.MediaPath != "" {
		base = strings.TrimSuffix(a.snapshot.Manifest.MediaPath, filepath.Ext(a.snapshot.Manifest.MediaPath))
	} else if a.snapshot != nil {
		base = a.snapshot.Manifest.Name
	}
	if track == 1 {
		base += "." + a.settings.TargetLanguage
	}
	return base + "." + a.trackFormats()[0]
}

func (a *NativeApp) saveTrack(track int, saveAs bool) {
	a.saveTracks([]int{track}, saveAs, nil)
}

// saveTracks writes tracks one after another. A track without a file asks
// where to save it; a cancelled dialog stops before next runs.
func (a *NativeApp) saveTracks(tracks []int, saveAs bool, next func()) {
	if a.busy || a.snapshot == nil || !a.commitEdit(true) {
		return
	}
	state := *a.snapshot
	formats := a.trackFormats()
	paths := [2]string{a.defaultTrackPath(0), a.defaultTrackPath(1)}
	known := a.savedPath
	a.run(locales.TaskSaveSubtitles(), func(ctx context.Context) (func(), error) {
		written := map[int]string{}
		finish := func(proceed bool) func() {
			return func() {
				for track, path := range written {
					a.savedPath[track] = path
					a.savedHash[track] = trackHash(state.Document, track)
				}
				a.refreshUnsaved()
				if len(written) > 0 {
					names := []string{}
					for _, track := range tracks {
						if path, ok := written[track]; ok {
							names = append(names, filepath.Base(path))
						}
					}
					a.status = locales.StatusSavedFiles(strings.Join(names, locales.ListSeparator()))
				}
				if proceed && next != nil {
					next()
				}
			}
		}
		for _, track := range tracks {
			path := known[track]
			if saveAs || path == "" {
				filters := []mygo.FileFilter{}
				for _, format := range formats {
					filters = append(filters, mygo.FileFilter{Name: strings.ToUpper(format), Extensions: []string{format}})
				}
				title := locales.SaveSourceTitle()
				if track == 1 {
					title = locales.SaveTranslationTitle()
				}
				chosen, err := a.saveDialog(mygo.SaveDialogOptions{Parent: a.win, Title: title, DefaultPath: paths[track], Filters: filters})
				if err != nil {
					return finish(false), err
				}
				if chosen == "" {
					return finish(false), nil
				}
				path = chosen
			}
			format := strings.TrimPrefix(strings.ToLower(filepath.Ext(path)), ".")
			allowed := false
			for _, candidate := range formats {
				allowed = allowed || candidate == format
			}
			if !allowed {
				format = formats[0]
				path += "." + format
			}
			data, err := a.encodeTrack(ctx, state.Document, track, format)
			if err != nil {
				return finish(false), err
			}
			// The file dialog confirmed any overwrite; the original is also
			// kept byte for byte inside the project.
			if err = subtitle.Replace(path, data); err != nil {
				return finish(false), err
			}
			written[track] = path
		}
		return finish(true), nil
	})
}

// encodeTrack writes one track in a subtitle format through libse. A
// translation with untranslated cues is refused, never filled with source.
func (a *NativeApp) encodeTrack(ctx context.Context, doc subtitle.Document, track int, format string) ([]byte, error) {
	mode := "source"
	if track == 1 {
		mode = "translation"
	}
	document, err := subtitle.SelectOutput(doc, subtitle.OutputOptions{Mode: mode})
	if err != nil {
		return nil, err
	}
	document.TargetFormat = format
	return a.worker.Write(ctx, document)
}

// guardUnsaved asks before an action that replaces or closes the workbench
// while a subtitle track differs from its file. next repeats the action.
func (a *NativeApp) guardUnsaved(next func()) bool {
	a.commitEdit(true)
	if a.skipGuard {
		a.skipGuard = false
		return false
	}
	if !a.unsaved[0] && !a.unsaved[1] {
		return false
	}
	if !a.savePromptOpen {
		a.afterSave = next
		a.savePromptOpen = true
	}
	return true
}

func (a *NativeApp) finishSavePrompt(choice string) {
	next := a.afterSave
	a.savePromptOpen, a.afterSave = false, nil
	switch choice {
	case "save":
		tracks := []int{}
		for track, unsaved := range a.unsaved {
			if unsaved {
				tracks = append(tracks, track)
			}
		}
		a.saveTracks(tracks, false, next)
	case "discard":
		a.skipGuard = true
		if next != nil {
			next()
		}
		a.skipGuard = false
	}
}

// unsavedText names each track with unsaved changes and what saving does
// to it, one line each.
func (a *NativeApp) unsavedText() string {
	lines := []string{}
	for track, unsaved := range a.unsaved {
		if !unsaved {
			continue
		}
		name := locales.TabSource()
		if track == 1 {
			name = locales.TabTranslation()
		}
		line := locales.SaveNewFile(name)
		if a.savedPath[track] != "" {
			line = locales.SaveOverwrite(name, filepath.Base(a.savedPath[track]))
		}
		lines = append(lines, line)
	}
	return strings.Join(lines, "\n")
}

// savePromptView asks in MyGo's alert, whose last button is the default and
// whose Escape cancels.
func (a *NativeApp) savePromptView(c *ui.Context) {
	if !a.savePromptOpen {
		return
	}
	open := true
	switch ui.AlertDialog(c, &open, locales.SavePromptTitle(), a.unsavedText(), locales.ActionCancel(), locales.SaveDiscard(), locales.SaveConfirm()) {
	case 1:
		a.finishSavePrompt("discard")
	case 2:
		a.finishSavePrompt("save")
	default:
		if !open {
			a.finishSavePrompt("cancel")
		}
	}
}

func (a *NativeApp) requestExport() {
	a.commitEdit(true)
	a.exportOpen = true
}

// Hiding to the tray keeps the project and its edits; nothing is lost.
func (a *NativeApp) hideToTray() {
	a.commitEdit(true)
	a.win.Hide()
}

func (a *NativeApp) requestClear() {
	a.commitEdit(true)
	a.clearOpen = true
}

// clearView is the second confirmation for clearing the workbench, with the
// save choice included when a track has unsaved changes.
func (a *NativeApp) clearView(c *ui.Context) {
	if !a.clearOpen {
		return
	}
	if !a.unsaved[0] && !a.unsaved[1] {
		if ui.AlertDialog(c, &a.clearOpen, locales.ClearTitle(), locales.ClearBody(), locales.ActionCancel(), locales.ClearConfirm()) == 1 {
			a.clearWorkbench()
		}
		return
	}
	switch ui.AlertDialog(c, &a.clearOpen, locales.ClearTitle(), locales.ClearBody()+"\n"+a.unsavedText(), locales.ActionCancel(), locales.ClearDiscard(), locales.ClearSave()) {
	case 1:
		a.clearWorkbench()
	case 2:
		tracks := []int{}
		for track, unsaved := range a.unsaved {
			if unsaved {
				tracks = append(tracks, track)
			}
		}
		a.saveTracks(tracks, false, a.clearWorkbench)
	}
}

// clearWorkbench closes the project and the video. No file is deleted.
func (a *NativeApp) clearWorkbench() {
	if a.busy {
		return
	}
	if a.store != nil {
		if err := a.store.Close(); err != nil {
			a.errorText = err.Error()
			return
		}
		a.store = nil
	}
	a.snapshot = nil
	a.selected = -1
	a.loadDraft()
	a.pending, a.candidatesOpen = nil, false
	a.translationCandidate, a.translationReview, a.translationConfirm = nil, false, false
	a.savedHash, a.savedPath, a.unsaved = [2]string{}, [2]string{}, [2]bool{}
	a.subtitleTab = 0
	a.query = ""
	a.mediaURL = ""
	a.position, a.duration, a.paused, a.activeCue, a.frameAspect = 0, 0, true, -1, 0
	a.loadVideo()
	a.errorText = ""
	a.status = locales.StatusCleared()
}
