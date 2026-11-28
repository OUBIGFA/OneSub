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
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	"github.com/google/uuid"
	lobeLayout "github.com/lobehub/lobe-mygo/pkg/layout"
)

func (a *NativeApp) pickFiles() {
	a.run(locales.TaskOpenFiles(), func(ctx context.Context) (func(), error) {
		paths, err := a.openDialog(mygo.OpenDialogOptions{Parent: a.win, Title: locales.ActionAddFiles(), Multiple: true, Filters: []mygo.FileFilter{{Name: locales.FilterMediaAndSubtitles(), Extensions: importing.Extensions}}})
		if err != nil || len(paths) == 0 {
			return nil, err
		}
		// Decide the target after selection. Adding to a queue does not
		// replace subtitles, so only the actual import needs a save prompt.
		return func() { a.acceptFiles(paths) }, nil
	})
}

// importPaths is the shared entry of drops, launches and associations.
func (a *NativeApp) importPaths(paths []string) {
	if a.proofreading && a.requestProofreadingImport(paths) {
		return
	}
	if a.guardUnsaved(func() { a.importPaths(paths) }) {
		return
	}
	a.run(locales.TaskImport(), func(ctx context.Context) (func(), error) { return a.importFiles(ctx, paths) })
}
func (a *NativeApp) importFiles(ctx context.Context, paths []string, wordEvidence ...[]byte) (func(), error) {
	selection, err := importing.Discover(paths)
	if err != nil {
		return nil, err
	}
	if a.proofreading && selection.Media != "" && selection.Automatic == "subtitle" {
		// The proofreading entry already asked about this sidecar. Attaching
		// its media must not reread it and replace the paired working track.
		selection.Subtitle, selection.Automatic, selection.Match = "", "", nil
	}
	// A ranked match or ambiguous candidates take precedence. Only a
	// standalone subtitle without either uses the current workbench's media.
	if selection.Subtitle != "" && selection.Media == "" && len(selection.Candidates) == 0 && a.snapshot != nil {
		selection.Media = a.snapshot.Manifest.MediaPath
	}
	currentSubtitle := false
	if selection.Automatic == "subtitle" && a.snapshot != nil {
		current := a.snapshot.Manifest.SubtitlePath
		currentSubtitle = selection.Subtitle == current
		if !currentSubtitle && current != "" {
			chosen, chosenErr := os.Stat(selection.Subtitle)
			existing, existingErr := os.Stat(current)
			currentSubtitle = chosenErr == nil && existingErr == nil && os.SameFile(chosen, existing)
		}
	}
	keepProject := selection.Media != "" && len(selection.Candidates) == 0 && (selection.Subtitle == "" || currentSubtitle) && a.store != nil && a.snapshot != nil && (len(a.snapshot.TrackDocument(0).Cues) > 0 || len(a.snapshot.Document.Cues) > 0)
	toLoad := selection
	if keepProject {
		// An automatic name match may point back to the already edited source.
		// Probe only the incoming media; rereading that file would lose edits.
		toLoad.Subtitle, toLoad.Automatic, toLoad.Match = "", "", nil
	}
	loaded, err := importing.Load(ctx, toLoad, a.worker, a.probe)
	if keepProject {
		loaded.Selection = selection
	}
	if err != nil {
		return nil, err
	}
	if len(wordEvidence) > 0 {
		// Faulty word timings cost only finer splits later, never the
		// recognized subtitles: they are left out, with a notice.
		if attached, wordWarnings, attachErr := subtitle.AttachOneAsrWords(loaded.Document, wordEvidence[0]); attachErr != nil {
			loaded.Warnings = append(loaded.Warnings, locales.WarnWordsSkipped(attachErr.Error()))
		} else {
			loaded.Document = attached
			loaded.Warnings = append(loaded.Warnings, wordWarnings...)
		}
	}
	if keepProject {
		// The complementary video changes only the association. Rereading
		// the original subtitle here would discard edits and translations.
		if err = a.store.SetMedia(loaded.Selection.Media); err != nil {
			return nil, err
		}
		state, err := a.store.Snapshot()
		if err != nil {
			return nil, err
		}
		url, previewErr := a.preview.Register(loaded.Selection.Media)
		return func() {
			a.setSnapshot(state)
			a.loadDraft()
			a.pending, a.candidatesOpen = &loaded, false
			a.mediaURL = url
			a.duration, a.frameAspect = float64(loaded.Media.DurationMs())/1000, loaded.Media.FrameAspect()
			a.position, a.activeCue, a.paused = 0, -1, true
			a.pendingClick, a.rowClick = nil, nil
			a.browseHeld = false
			a.resumeFollow()
			a.loadVideo()
			a.showImportResult(loaded)
			if previewErr != nil {
				a.showError("player", previewErr.Error())
			}
		}, nil
	}
	name := selection.Subtitle
	if name == "" {
		name = selection.Media
	}
	name = strings.TrimSuffix(filepath.Base(name), filepath.Ext(name))
	root := filepath.Join(a.dataRoot, "projects")
	if err = os.MkdirAll(root, 0700); err != nil {
		return nil, err
	}
	next, err := project.Create(filepath.Join(root, name+"-"+uuid.NewString()+".onesub"), loaded.Selection.Media, loaded.Selection.Subtitle, loaded.Document, name)
	if err != nil {
		return nil, err
	}
	state, err := next.Snapshot()
	if err != nil {
		next.Close()
		return nil, err
	}
	if len(wordEvidence) > 0 {
		// Retain the unmodified aligner artifact independently of the safe
		// split anchors attached to cues; zero-width tokens are not discarded.
		if err = os.WriteFile(filepath.Join(state.Path, "originals", "oneasr-words.json"), wordEvidence[0], 0600); err != nil {
			next.Close()
			return nil, err
		}
	}
	url, previewErr := a.preview.Register(loaded.Selection.Media)
	return func() {
		a.replaceProject(next, state, url)
		a.pending = &loaded
		a.candidatesOpen = len(selection.Candidates) > 0
		a.duration = float64(loaded.Media.DurationMs()) / 1000
		a.frameAspect = loaded.Media.FrameAspect()
		a.showImportResult(loaded)
		if previewErr != nil {
			a.showError("player", previewErr.Error())
		}
	}, nil
}

// Import warnings are scoped to the import operation. A successful
// association stays out of the persistent footer.
func (a *NativeApp) showImportResult(loaded importing.Loaded) {
	a.showError("import", strings.Join(loaded.Warnings, locales.ListSeparator()))
}

// replaceProject switches the workbench to another project. The subtitle
// file it came from is the saved baseline of the source track.
func (a *NativeApp) replaceProject(next *project.Store, state project.Snapshot, url string) {
	a.setPlayerFullscreen(false)
	if a.store != nil {
		a.store.Close()
	}
	a.store = next
	a.subtitleTab = 0
	a.proofreadingSourceTracked = false
	a.proofreadingFiles = [3]*proofreadingFile{}
	a.savedHash[2], a.savedPath[2], a.unsaved[2] = "", "", false
	if a.proofreading {
		a.subtitleTab = 1
	}
	// A newly imported subtitle is directly usable, including a standalone
	// translated file. Do not inherit a missing bilingual/translation track.
	a.settings.SubtitleOutput.Mode, a.settings.SubtitleOutput.SourceTrack = "source", "original"
	a.selected = 0
	a.translationCandidate = nil
	// The footer action opens the last output. A previous project's artifact
	// is not part of this workbench, so drop its path on a new import.
	a.lastOutput = ""
	a.translationLive, a.translationTask = nil, nil
	a.settings.TranslationRecovery = ""
	a.settings.Processing = nil
	a.saveSettings()
	a.setSnapshot(state)
	a.markSaved(0, a.originalSubtitlePath())
	a.markSaved(1, "")
	a.loadDraft()
	a.mediaURL = url
	a.position = 0
	a.activeCue = -1
	a.paused = true
	a.pendingClick, a.rowClick = nil, nil
	a.browseHeld = false
	a.resumeFollow()
	a.loadVideo()
}
func (a *NativeApp) candidatesView(c *ui.Context) {
	fixedDialog(c, &a.candidatesOpen, locales.CandidatesTitle(), 600, 0, func() {
		_, height := c.Size()
		// Candidate rows size to both text lines; only a long list scrolls.
		// Do not estimate the viewport from a fixed single-line button height.
		ui.Scroll(c).MaxHeight(height * .55).Children(func() {
			ui.Column(c).Gap(em(c, 8)).Children(func() {
				if a.pending != nil {
					for _, candidate := range a.pending.Selection.Candidates {
						path := candidate.Path
						rule := locales.CandidatesSimilar()
						switch candidate.Reason {
						case "exact":
							rule = locales.CandidatesExact()
						case "normalized":
							rule = locales.CandidatesNormalized()
						case "language":
							rule = locales.CandidatesLanguage()
						}
						score := locales.CandidatesScore(candidate.Score)
						lobeLayout.Block(c.Key(path), lobeLayout.BlockProps{Variant: lobeLayout.BlockOutlined, Clickable: true}).
							Row().Gap(em(c, 16)).Padding(em(c, 12), em(c, 16)).AlignItems(ui.Center).
							Label(filepath.Base(path)).Tooltip(path + "\n" + score + " · " + rule).Disabled(a.busy).
							Children(func() {
								ui.Column(c).Grow(1).MinWidth(0).Gap(em(c, 4)).Children(func() {
									ui.Text(c, filepath.Base(path)).SingleLine()
									muted(c, rule).FontSize(em(c, captionSize))
								})
								muted(c, score).SingleLine().FontFeatures("tnum").Shrink(0)
							}).OnClick(func() {
							a.candidatesOpen = false
							paths := []string{path}
							if a.pending.Selection.CandidateKind == "subtitle" {
								paths = append(paths, a.pending.Selection.Media)
							} else {
								paths = append(paths, a.pending.Selection.Subtitle)
							}
							a.importPaths(paths)
						})
					}
				}
			})
		})
	}, func() {
		lobeButton(c, locales.CandidatesLater()).OnClick(func() { a.candidatesOpen = false })
	})
}
