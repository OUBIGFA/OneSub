package main

import (
	"math"
	"path/filepath"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func (a *NativeApp) view(c *ui.Context) {
	look := applyStudioTheme(c)
	x, y, subtitles, editor := a.paneSizes(c)
	width, height := c.Size()
	// The split percentage and viewport are known in this frame; the list
	// can choose its compact controls without measuring an earlier layout.
	insets := em(c, 2*workbenchInset)
	subtitleWidth := ((width-1)*(100-x)/100-1)*editor/100 - insets
	paneHeight := height - em(c, toolbarSize+32) - 3
	surface := ui.Column(c).Fill().Background(look.chrome)
	surface.Children(func() {
		if a.playerFullscreen {
			// Reuse the native WebView and subtitle renderer at the new size.
			// None of the four saved dividers participates in fullscreen layout.
			a.playerView(c)
			return
		}
		a.topBar(c, look)
		ui.Box(c).Height(1).Shrink(0)
		// Three independent columns: preview/style, subtitles/editor, workflow.
		// Native splitters retain pointer, keyboard and same-frame layout.
		panelSplit(c, false, &x, func() {
			panelSplit(c, true, &y, func() {
				workbenchPanel(c, look, func() { a.playerViewAtWidth(c, (width-1)*x/100-insets) })
			}, func() {
				workbenchPanel(c, look, func() { a.stylePanelAtSize(c, (width-1)*x/100-insets, paneHeight*(100-y)/100-insets) })
			}, func() {
				previous := a.splitY
				a.settings.StylePanelCollapsed = false
				a.splitY = y
				_, a.splitY, _, _ = a.paneSizes(c)
				if paneHeight*(100-a.splitY)/100-insets <= em(c, headerSize)+1.01 {
					// A drag to the title folds without discarding the height
					// that the disclosure must restore on its next click.
					a.settings.StylePanelCollapsed, a.splitY = true, previous
				}
				a.prefsSaveDue = c.Now().Add(400 * time.Millisecond)
			}).Fill()
		}, func() {
			panelSplit(c, false, &editor, func() {
				panelSplit(c, true, &subtitles, func() {
					workbenchPanel(c, look, func() { a.subtitlesViewAtWidth(c, subtitleWidth) })
				}, func() {
					workbenchPanel(c, look, func() { a.editorViewAtHeight(c, paneHeight*(100-subtitles)/100-insets) })
				}, func() {
					previous := a.subtitleSplit
					a.settings.EditorPanelCollapsed = false
					a.subtitleSplit = subtitles
					_, _, a.subtitleSplit, _ = a.paneSizes(c)
					if paneHeight*(100-a.subtitleSplit)/100-insets <= em(c, headerSize)+1.01 {
						a.settings.EditorPanelCollapsed, a.subtitleSplit = true, previous
					}
					a.prefsSaveDue = c.Now().Add(400 * time.Millisecond)
				}).Fill()
			}, func() {
				workbenchPanel(c, look, func() { a.workflowColumn(c) })
			}, func() {
				a.editorSplit = editor
				_, _, _, a.editorSplit = a.paneSizes(c)
			}).Fill()
		}, func() {
			a.splitX = x
			a.splitX, _, _, _ = a.paneSizes(c)
		}).Grow(1)
		ui.Box(c).Height(1).Shrink(0)
		a.statusBar(c, look)
	})
	if paths := surface.DroppedFiles(); len(paths) > 0 {
		a.acceptFiles(paths)
	}
	a.settingsView(c)
	a.modelManagerView(c)
	a.modelRecycleView(c)
	a.storageConfirmView(c)
	a.candidatesView(c)
	a.proofreadingImportView(c)
	a.exportView(c)
	a.composeView(c)
	a.compressView(c)
	a.translationView(c)
	a.missingSaveView(c)
	a.retranslationView(c)
	a.providerDeleteView(c)
	a.savePromptView(c)
	a.clearView(c)
	a.presetDialogView(c)
	a.insertSubtitleView(c)
	a.findReplaceView(c)
	a.observeStyle(c.Now())
	// The panes keep the sizes the user drags them to, saved once the drag
	// stops, as the other preferences.
	if w, h, e, s := float64(a.splitX), float64(a.splitY), float64(a.editorSplit), float64(a.subtitleSplit); w != a.settings.PlayerWidth || h != a.settings.TopHeight || e != a.settings.EditorWidth || s != a.settings.SubtitleHeight {
		a.settings.PlayerWidth, a.settings.TopHeight, a.settings.EditorWidth, a.settings.SubtitleHeight = w, h, e, s
		a.prefsSaveDue = c.Now().Add(400 * time.Millisecond)
	}
	if !a.prefsSaveDue.IsZero() {
		if !c.Now().Before(a.prefsSaveDue) {
			a.saveSettings()
			a.prefsSaveDue = time.Time{}
		} else {
			c.After(a.prefsSaveDue.Sub(c.Now()))
		}
	}
	// Edits reach the project after a pause in typing, never in the middle of
	// an input-method composition.
	if a.editPending && !a.composing && !a.editDue.IsZero() {
		if !c.Now().Before(a.editDue) {
			a.commitEdit(false)
		} else {
			c.After(a.editDue.Sub(c.Now()))
		}
	}
	// Style previews use their own worker and revision checks. Unrelated
	// background work must not delay a visible style change until it finishes.
	if a.previewPending && !a.previewRunning && !a.storage.cleaning && !a.editPending && a.playerReady && a.mediaURL != "" && len(a.styleCatalog.Presets) > 0 {
		if !c.Now().Before(a.previewDue) {
			a.refreshSubtitles()
		} else {
			c.After(a.previewDue.Sub(c.Now()))
		}
	}
	if c.Shortcut(0, ui.KeyF11) && !c.ShortcutRepeated() {
		a.setPlayerFullscreen(!a.playerFullscreen)
	} else if a.playerFullscreen && c.Shortcut(0, ui.KeyEscape) {
		a.setPlayerFullscreen(false)
	}
	if !a.playerFullscreen && c.Shortcut(ui.Cmd, ui.KeyH) && !a.busy {
		a.openFindReplace()
	}
	if c.Shortcut(ui.Cmd, ui.KeyS) && !a.busy {
		a.saveTrack(a.subtitleTab, false)
	}
	if a.playbackShortcut(c) {
		a.togglePlayback()
	}
	if !a.playerFullscreen && !a.busy && a.list.Shortcut(c, 0, ui.KeyDelete) {
		if cues := a.activeDocument().Cues; a.selected >= 0 && a.selected < len(cues) {
			a.deleteSubtitle(cues[a.selected].ID)
		}
	}
}

// paneSizes clamps only the displayed shares, so startup or a narrow window
// cannot overwrite the user's proportions. Split OnChange callbacks also use
// these bounds to persist actual divider gestures, never temporary resizing.
func (a *NativeApp) paneSizes(c *ui.Context) (player, top, subtitles, editor float32) {
	player, top, subtitles, editor = a.splitX, a.splitY, a.subtitleSplit, a.editorSplit
	width, height := c.Size()
	toolbar := em(c, toolbarSize)
	// Two horizontal rules and the split's own one-DIP line are not part
	// of either pane. Include them instead of leaving a phantom scrollbar.
	height -= toolbar + em(c, 32) + 3
	keep := func(percent *float32, room, first, second float32) {
		// Nested percentage conversion can lose a fraction of a pixel at
		// an exact minimum. Keep the bound instead of reverting to the ratio.
		if room+.01 >= first+second {
			*percent = min(max(*percent, first/room*100), 100-second/room*100)
		}
	}
	// Clamp display sizes only when the requested ratios cannot fit the
	// inspector's fixed action and title. User proportions stay unchanged.
	workflowMinimum := em(c, 250)
	titleWidth, _ := c.MeasureText(0, ui.Span{Text: locales.TranslationOptimizeTimeline(), Size: em(c, bodySize), Weight: 500})
	// Include both panel and toggle padding plus its two one-DIP borders.
	// Round up so SingleLine never ellipsizes a subpixel overflow.
	workflowMinimum = max(workflowMinimum, float32(math.Ceil(float64(titleWidth+em(c, 2*panelPad+24)+2))))
	for _, label := range []string{locales.WorkflowStartRecognize(), locales.WorkflowStartTranslate()} {
		w, _ := c.MeasureText(0, ui.Span{Text: label, Weight: 500})
		workflowMinimum = max(workflowMinimum, float32(math.Ceil(float64(w+em(c, 2*panelPad+32+iconSize+8)+2))))
	}
	// Reserve the style split's labels at the supported minimum. If the OS work
	// area is smaller, retain the compact bounds for the other two columns.
	insets := em(c, 2*workbenchInset)
	workflowMinimum += insets
	playerFloor := em(c, 160) + insets
	listMinimum := min(em(c, 356)+insets, max(em(c, 160)+insets, width-2-playerFloor-workflowMinimum))
	otherMinimum := listMinimum + workflowMinimum + 1
	playerMinimum := min(em(c, 700)+insets, max(playerFloor, width-1-otherMinimum))
	keep(&player, width-1, playerMinimum, otherMinimum)
	// Content must scroll inside a deliberately shortened pane, never
	// override a user's drag with its measured natural height.
	// Inspectors can fold to their native title bar and its separator.
	// Content visibility uses this same frame's allocation, never old bounds.
	keep(&top, height, em(c, 240)+insets, em(c, headerSize)+1+insets)
	keep(&subtitles, height, em(c, 240)+insets, em(c, headerSize)+1+insets)
	// Folding is independent of the saved split: expanding restores the
	// previous working height even after a resize or application restart.
	foldedTop := max(0, 100-(em(c, headerSize)+1+insets)/max(height, 1)*100)
	if a.settings.StylePanelCollapsed {
		top = foldedTop
	}
	if a.settings.EditorPanelCollapsed {
		subtitles = foldedTop
	}
	keep(&editor, (width-1)*(100-player)/100-1, listMinimum, workflowMinimum)
	return
}

// The flexible filename gives way to commands as the viewport narrows.
// Subtitle-only projects use their subtitle filename in the same slot.
func (a *NativeApp) topBar(c *ui.Context, look studio) {
	width, _ := c.Size()
	command := func(label, name string) ui.Element {
		if width < em(c, 1100) {
			return toolIcon(c, label, name).Size(em(c, 36), em(c, 36))
		}
		return quietIconButton(c, label, name).MinHeight(em(c, 36))
	}
	hasMedia := a.snapshot != nil && a.snapshot.Manifest.MediaPath != ""
	hasCues := a.snapshot != nil && len(a.snapshot.Document.Cues) > 0
	ui.Row(c).Height(em(c, toolbarSize)).Shrink(0).AlignItems(ui.Center).Gap(em(c, 8)).Padding(0, em(c, 16)).Background(look.chrome).Label(locales.A11yToolbar()).Children(func() {
		ui.Row(c).Gap(em(c, 4)).AlignItems(ui.Center).Shrink(0).Children(func() {
			iconButton(c, locales.ActionAddFiles(), "folder-open").MinHeight(em(c, 36)).Disabled(a.busy).OnClick(a.pickFiles)
			toolIcon(c, locales.ActionClearWorkbench(), "eraser").Size(em(c, 36), em(c, 36)).Disabled(a.busy || a.snapshot == nil).OnClick(a.requestClear)
		})
		path, docIcon := "", "scroll-text"
		if a.snapshot != nil {
			path = a.snapshot.Manifest.MediaPath
			if path != "" {
				docIcon = "file-video"
			} else {
				path = a.snapshot.Manifest.SubtitlePath
			}
		}
		if path != "" {
			separator(c).Height(em(c, 18)).AlignSelf(ui.Center).Shrink(0)
		}
		ui.Row(c).Grow(1).MinWidth(0).Gap(em(c, 6)).AlignItems(ui.Center).Children(func() {
			if path != "" {
				ui.Icon(c, icons[docIcon]).Size(em(c, 15), em(c, 15)).TextColor(c.Theme().TextMuted).Shrink(0)
				elidedText(c, filepath.Base(path)).MinWidth(0).Shrink(1).FontWeight(500)
			}
		})
		ui.Row(c).AlignItems(ui.Center).Gap(em(c, 4)).Shrink(0).Children(func() {
			toolIcon(c, locales.ActionSettings(), "settings").Size(em(c, 36), em(c, 36)).OnClick(func() { a.reloadTranslationProfiles(); a.settingsOpen = true })
			command(locales.MenuExportSubtitles(), "file-down").Disabled(a.busy || !hasCues).OnClick(a.requestExport)
			command(locales.ActionCompress(), "file-video").Disabled(a.busy || !hasMedia).OnClick(func() { a.compressOpen = true })
		})
		separator(c).Height(em(c, 20)).AlignSelf(ui.Center).Shrink(0)
		ui.Row(c).Gap(em(c, 8)).AlignItems(ui.Center).Shrink(0).Children(func() {
			label, disabled, run := locales.ActionCompose(), a.busy || !hasMedia || !hasCues, a.compose
			// Reopen a background export from the same place it started.
			if a.composeRunning && a.composeInBackground && !a.compressRunning && !a.audioExtractRunning {
				label, disabled, run = locales.ComposeShowProgress(), false, a.showComposeProgress
			}
			primaryIconButton(c, label, "clapperboard").MinHeight(em(c, 36)).Disabled(disabled).OnClick(run)
		})
	})
}

// Both split sizing and the toolbar use this height in the first frame.
const toolbarSize = 56

// errorDuration is how long the status bar shows an error before it goes
// back to the last status.
const errorDuration = 8 * time.Second

// statusBar reports the last result in caption text. An error shows in the
// danger color, with its full text in the tooltip, for errorDuration.
func (a *NativeApp) statusBar(c *ui.Context, look studio) {
	if a.errorText != a.errorShown {
		a.errorShown, a.errorSince = a.errorText, c.Now()
	}
	if a.errorText != "" {
		if left := errorDuration - c.Now().Sub(a.errorSince); left > 0 {
			c.After(left)
		} else {
			a.errorText, a.errorShown, a.errorScope = "", "", ""
		}
	}
	width, _ := c.Size()
	compact := width < em(c, 680) || a.lastOutput != "" && width < em(c, 1600)
	original, optimized := a.subtitleCounts()
	counts := []string{}
	if original > 0 {
		counts = append(counts, locales.StatusOriginalCount(original))
	}
	if optimized > 0 {
		counts = append(counts, locales.StatusOptimizedCount(optimized))
	}
	// The 24-DIP actions fit inside the 32-DIP bar with four DIP above/below.
	// Keep the same inset for icon-only and labelled actions at every text scale.
	ui.Row(c).Height(em(c, 32)).Shrink(0).Padding(em(c, 4), em(c, 16)).Gap(em(c, 12)).AlignItems(ui.Center).Background(look.chrome).FontSize(em(c, captionSize)).Label(locales.A11yStatusBar()).Children(func() {
		// Reading warnings and transient results stay on the left. Track counts
		// have a separate stable slot immediately before the output action.
		readingHint := false
		if !a.showsTranslationProgress() {
			if load := a.cueReading(a.selected); load.Problem() {
				label, detail := a.readingLabel(a.activeDocument().Cues[a.selected], load)
				ui.Row(c).Gap(em(c, 4)).AlignItems(ui.Center).Shrink(0).Children(func() {
					icon(c, "triangle-alert").TextColor(c.Theme().Danger).Label(locales.A11yWarningIcon()).Tooltip(locales.StatusOwnerSubtitle() + detail)
					if !compact {
						muted(c, locales.StatusOwnerSubtitle()+label).TextColor(c.Theme().Danger).SingleLine().Shrink(0).Tooltip(locales.StatusOwnerSubtitle() + detail)
					}
				})
				readingHint = true
			}
		}

		hasStatus := a.errorText != "" || a.status != ""
		if hasStatus {
			if readingHint {
				separator(c).Height(em(c, 16)).AlignSelf(ui.Center)
			}
			if a.errorText != "" {
				ui.Row(c).Grow(1).MinWidth(0).Gap(em(c, 4)).AlignItems(ui.Center).Children(func() {
					icon(c, "triangle-alert").TextColor(c.Theme().Danger).Label(locales.A11yWarningIcon())
					elidedText(c, locales.StatusOwnerError()+a.errorText).TextColor(c.Theme().Danger).Grow(1)
				})
			} else {
				elidedText(c, locales.StatusOwnerStatus()+a.status).TextColor(c.Theme().TextMuted).Grow(1)
			}
		} else {
			ui.Text(c, "").Grow(1)
		}
		a.moduleTimesView(c)

		backgroundCompose := a.busy && a.composeRunning && a.composeInBackground && !a.compressRunning && !a.audioExtractRunning
		canCancel := a.busy && a.cancel != nil && !a.choosingFile && (!a.composeRunning || a.composeInBackground) && !a.recognitionRunning && !a.translationRunning && !backgroundCompose
		canOpenOutput := a.lastOutput != "" && !a.busy
		if (readingHint || hasStatus) && (backgroundCompose || canCancel || canOpenOutput || len(counts) > 0) {
			separator(c).Height(em(c, 16)).AlignSelf(ui.Center)
		}
		if backgroundCompose {
			// The same worker fraction drives both progress surfaces. Negative
			// fractions retain the native indeterminate bar during preparation/checks.
			ui.Row(c).Gap(em(c, 6)).AlignItems(ui.Center).Shrink(0).Children(func() {
				muted(c, locales.StatusOwnerTask()).SingleLine().Shrink(0)
				lobeProgress(c, a.composeProgress.Fraction).Width(em(c, 120)).Shrink(0).Label(locales.A11yComposeProgress()).Tooltip(composeStageLabel(a.composeProgress.Stage))
				if a.composeProgress.Fraction >= 0 {
					muted(c, locales.Percent(100*a.composeProgress.Fraction)).SingleLine().FontFeatures("tnum").Shrink(0)
				}
			})
		} else if canCancel {
			lobeButton(c, locales.StatusOwnerTask()+locales.ActionCancelTask()).MinHeight(em(c, 24)).Height(em(c, 24)).FontSize(em(c, captionSize)).LineHeight(1.1).Padding(em(c, 2), em(c, 10)).Shrink(0).OnClick(func() { a.cancel() })
		}
		if (backgroundCompose || canCancel) && len(counts) > 0 {
			separator(c).Height(em(c, 16)).AlignSelf(ui.Center)
		}
		if len(counts) > 0 {
			ui.Row(c).Gap(em(c, 12)).AlignItems(ui.Center).Shrink(0).Children(func() {
				for _, label := range counts {
					muted(c, label).FontFeatures("tnum").LineHeight(1.1).SingleLine().Shrink(0)
				}
			})
		}
		if len(counts) > 0 && canOpenOutput {
			separator(c).Height(em(c, 16)).AlignSelf(ui.Center)
		}
		if canOpenOutput {
			label := locales.StatusOwnerOutput() + locales.ResultOpenFolder()
			open := func() {
				if err := mygo.Shell.OpenPath(filepath.Dir(a.lastOutput)); err != nil {
					a.showError("output-folder", err.Error())
				} else {
					a.clearError("output-folder")
				}
			}
			if compact {
				toolIcon(c, label, "folder-open").Size(em(c, 24), em(c, 24)).Shrink(0).OnClick(open)
			} else {
				lobeButton(c, label).MinHeight(em(c, 24)).Height(em(c, 24)).FontSize(em(c, captionSize)).LineHeight(1.1).Padding(em(c, 2), em(c, 10)).Shrink(0).OnClick(open)
			}
		}
	})
}

// run owns one cancellable operation. Closing to tray does not end this context.
// Callers that replace the workbench ask about unsaved subtitle files first.
