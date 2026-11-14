package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/importing"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
	"github.com/google/uuid"
)

type proofreadingImport struct {
	paths     []string
	subtitles []string
	roles     []int // 0 source, 1 translation; selected explicitly for each file
	open      bool
}

// Each language retains its own format template (notably ASS styles and VTT
// metadata), while edits and playback share the working document's cue IDs.
type proofreadingFile struct {
	document subtitle.Document
	ids      map[string]string
}

func (a *NativeApp) proofreadingButton(c *ui.Context, compact, iconOnly bool) {
	label := locales.ProofreadingEnter()
	if a.proofreading {
		label = locales.ProofreadingExit()
	}
	// Extremely narrow panes use the same labelled native icon target as the
	// other file actions, leaving room for the list and its navigation rail.
	var button ui.Element
	if iconOnly {
		button = toolIcon(c, label, "list-checks")
	} else {
		button = iconButton(c, label, "list-checks")
	}
	button.Shrink(0).Tooltip(label).Disabled(a.busy || a.translationLive != nil).OnClick(a.toggleProofreading)
	if compact && !iconOnly {
		button.PaddingX(em(c, 8))
	}
}

func (a *NativeApp) toggleProofreading() {
	if !a.commitEdit(true) {
		return
	}
	if a.proofreading {
		a.proofreading = false
		a.changeSubtitleTab(a.proofreadingPreviousTab)
		return
	}
	a.proofreadingPreviousTab = a.subtitleTab
	a.proofreading = true
	a.changeSubtitleTab(1)
	if a.settings.SubtitleOutput.Mode == "source" {
		a.settings.SubtitleOutput.SourceTrack = ""
		a.previewPending = true
	}
	// Track the editable working source separately from the untouched
	// original segmentation; exiting the mode must not lose its dirty state.
	if !a.proofreadingSourceTracked {
		a.proofreadingSourceTracked = true
		a.markSaved(2, "")
	}
}

// Both file selection and native drops reach this gate before reading files
// or displaying a save prompt. Cancelling therefore changes nothing.
func (a *NativeApp) requestProofreadingImport(paths []string) bool {
	pending := &proofreadingImport{paths: append([]string{}, paths...), open: true}
	for _, path := range paths {
		if importing.Kind(path) == "subtitle" {
			pending.subtitles = append(pending.subtitles, path)
			pending.roles = append(pending.roles, 0)
		}
	}
	if len(pending.subtitles) == 0 {
		// An automatically associated subtitle still needs a language role.
		selection, err := importing.Discover(paths)
		if err != nil || selection.Subtitle == "" {
			return false
		}
		pending.subtitles, pending.roles = []string{selection.Subtitle}, []int{0}
	}
	if a.proofreadingImport == nil || !a.proofreadingImport.open {
		a.proofreadingImport = pending
	}
	return true
}

func (a *NativeApp) proofreadingImportView(c *ui.Context) {
	pending := a.proofreadingImport
	if pending == nil || !pending.open {
		return
	}
	fixedDialog(c, &pending.open, locales.ProofreadingImportTitle(), 600, 0, func() {
		_, height := c.Size()
		ui.Scroll(c).MaxHeight(height * .55).Children(func() {
			ui.Column(c).Gap(em(c, 16)).Children(func() {
				for i, path := range pending.subtitles {
					ui.Column(c.Key(path)).Gap(em(c, 8)).Children(func() {
						ui.Text(c, filepath.Base(path)).SingleLine().Tooltip(path)
						segmentedFill(c, &pending.roles[i], locales.TabSource(), locales.TabTranslation())
					})
				}
			})
		})
	}, func() {
		lobeButton(c, locales.ActionCancel()).OnClick(func() { pending.open = false })
		lobePrimaryButton(c, locales.ListImport()).OnClick(func() {
			pending.open = false
			a.acceptProofreadingImport(pending)
		})
	})
}

func (a *NativeApp) acceptProofreadingImport(pending *proofreadingImport) {
	if a.guardUnsaved(func() { a.acceptProofreadingImport(pending) }) {
		return
	}
	a.run(locales.TaskImport(), func(ctx context.Context) (func(), error) { return a.importProofreadingFiles(ctx, pending) })
}

// Parse all inputs before mutating anything. Reuse the normal media loader
// when starting a workbench; subsequent subtitle imports retain its player.
func (a *NativeApp) importProofreadingFiles(ctx context.Context, pending *proofreadingImport) (func(), error) {
	docs := make([]subtitle.Document, len(pending.subtitles))
	for i, path := range pending.subtitles {
		doc, err := a.worker.Read(ctx, path)
		if err != nil {
			return nil, err
		}
		docs[i] = doc
	}
	store := a.store
	created := store == nil
	if created {
		root := filepath.Join(a.dataRoot, "projects")
		if err := os.MkdirAll(root, 0700); err != nil {
			return nil, err
		}
		name := strings.TrimSuffix(filepath.Base(pending.subtitles[0]), filepath.Ext(pending.subtitles[0]))
		baseline := docs[0]
		baseline.Cues = []subtitle.Cue{}
		next, err := project.Create(filepath.Join(root, name+"-"+uuid.NewString()+".onesub"), "", "", baseline, name)
		if err != nil {
			return nil, err
		}
		store = next
	}
	var revision int64
	if !created {
		revision = a.snapshot.Revision
	}
	completed := []int{}
	finish := func() func() {
		return func() {
			state, err := store.Snapshot()
			if err != nil {
				a.showError("import", err.Error())
				return
			}
			if created {
				a.replaceProject(store, state, "")
			}
			a.subtitleTab, a.proofreadingSourceTracked = 1, true
			a.setSnapshot(state)
			for _, i := range completed {
				track := 2
				if pending.roles[i] == 1 {
					track = 1
				}
				// A selected file is a save baseline only for its own language.
				file := &proofreadingFile{document: docs[i], ids: map[string]string{}}
				sameTiming := true
				for row, cue := range docs[i].Cues {
					if row >= len(state.Document.Cues) {
						sameTiming = false
						continue
					}
					working := state.Document.Cues[row]
					file.ids[working.ID] = cue.ID
					sameTiming = sameTiming && working.StartMs == cue.StartMs && working.EndMs == cue.EndMs
				}
				a.proofreadingFiles[track] = file
				a.markSaved(track, pending.subtitles[i])
				if !sameTiming {
					a.savedHash[track] = ""
				}
			}
			a.refreshUnsaved()
			a.settings.SubtitleOutput.SourceTrack = ""
			if a.trackHasContent(1) {
				a.settings.SubtitleOutput.Mode = "translation"
			} else {
				a.settings.SubtitleOutput.Mode = "source"
			}
			a.loadDraft()
			a.list.StopScroll()
			a.resumeFollow()
			// Explicitly paired media still uses the existing association path.
			for _, path := range pending.paths {
				if kind := importing.Kind(path); kind == "audio" || kind == "video" {
					a.run(locales.TaskImport(), func(ctx context.Context) (func(), error) { return a.importFiles(ctx, []string{path}) })
					break
				}
			}
		}
	}
	for i, doc := range docs {
		var err error
		revision, err = store.ImportProofreading(uuid.NewString(), revision, doc, pending.roles[i] == 0)
		if err != nil {
			if created && len(completed) == 0 {
				store.Close()
				return nil, err
			}
			return finish(), err
		}
		completed = append(completed, i)
	}
	return finish(), nil
}
