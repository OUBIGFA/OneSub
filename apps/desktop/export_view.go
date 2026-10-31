package main

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// Provenance comes from persisted cue metadata, never the next task's switch.
// SourceCueIDs survives source edits and retranslating an already optimized
// timeline even when that pass replaces SourceRange with an unchanged range.
func translationOutputLabel(doc subtitle.Document) string {
	for _, cue := range doc.Cues {
		if len(cue.SourceCueIDs) > 0 || cue.SourceRange != nil && cue.SourceRange.Reason != "" {
			return locales.OutputTranslation(locales.OutputOptimized())
		}
	}
	return locales.OutputTranslation(locales.OutputOriginal())
}

func (a *NativeApp) outputModes() []option {
	doc := subtitle.Document{}
	if a.snapshot != nil {
		doc = a.snapshot.Document
	}
	return []option{
		{"original", locales.TabOriginal()},
		{"source", locales.OutputRebuiltSource()},
		{"translation", translationOutputLabel(doc)},
		{"bilingual", locales.OutputBilingual()},
	}
}

// In dialogs, content spans the two-column grid; language order and priority
// occupy the second row. In the inspector each field keeps its natural width.
func (a *NativeApp) outputFields(c *ui.Context, keepRoom bool) {
	o := a.settings.SubtitleOutput.Normalized()
	mode := o.Mode
	if mode == "source" && o.SourceTrack == "original" {
		mode = "original"
	}
	content := ui.Field(c, locales.OutputContent(), func() {
		idSelect(c, &mode, a.outputModes()).Label(locales.OutputContent()).AlignSelf(ui.Start).OnChange(func() {
			o.Mode, o.SourceTrack = mode, ""
			if mode == "original" {
				o.Mode, o.SourceTrack = "source", "original"
			}
			a.settings.SubtitleOutput = o
			a.saveSettings()
		})
	})
	if keepRoom {
		content.ColumnSpan(2)
	}
	bilingual := o.Mode == "bilingual"
	if !bilingual && !keepRoom {
		return
	}
	order := "translation-first"
	if o.SourceFirst {
		order = "source-first"
	}
	// Bilingual options remain in the dialog grid when disabled; the style
	// inspector omits them for single-language output.
	fit := func(e ui.Element) {
		if !keepRoom {
			e.AlignSelf(ui.Start)
		}
	}
	ui.Field(c, locales.OutputOrder(), func() {
		fit(idSelect(c, &order, []option{{"translation-first", locales.OutputTranslationFirst()}, {"source-first", locales.OutputSourceFirst()}}).Label(locales.OutputOrder()).Disabled(!bilingual).OnChange(func() {
			o.SourceFirst = order == "source-first"
			a.settings.SubtitleOutput = o
			a.saveSettings()
		}))
	})
	ui.Field(c, locales.OutputPrimary(), func() {
		fit(idSelect(c, &o.Primary, []option{{"translation", locales.TabTranslation()}, {"source", locales.TabSource()}}).Label(locales.OutputPrimary()).Disabled(!bilingual).OnChange(func() {
			a.settings.SubtitleOutput = o
			a.saveSettings()
		}))
	})
}

// exportView separates content, format and line selection, with an anchored
// action row shared with video export. Disabled bilingual fields keep their room.
func (a *NativeApp) exportView(c *ui.Context) {
	var outputErr error
	if a.exportOpen && a.snapshot != nil {
		_, outputErr = subtitle.SelectOutput(a.snapshot.OutputDocument(a.settings.SubtitleOutput), a.settings.SubtitleOutput)
	}
	fixedDialog(c, &a.exportOpen, locales.MenuExportSubtitles(), 680, 496, func() {
		ui.Grid(c).Columns(2).Gap(em(c, 12)).Children(func() { a.outputFields(c, true) })
		ui.Form(c, func() {
			ui.Field(c, locales.ExportFormat(), func() {
				formats := []string{"srt", "vtt", "ass"}
				if a.snapshot != nil && a.snapshot.Document.Format == "ssa" {
					formats = append(formats, "ssa")
				}
				format, labels := 0, []string{}
				for i, f := range formats {
					labels = append(labels, strings.ToUpper(f))
					if f == a.exportFormat {
						format = i
					}
				}
				if ui.Segmented(c, &format, labels...).Label(locales.ExportFormat()).AlignSelf(ui.Start).Changed() {
					a.exportFormat = formats[format]
				}
			})
			ui.Field(c, locales.ExportLines(), func() {
				lines, selected := []string{"all", "upper", "lower"}, 0
				for i, line := range lines {
					if line == a.exportLines {
						selected = i
					}
				}
				if ui.Segmented(c, &selected, locales.ExportAllLines(), locales.ExportUpperLine(), locales.ExportLowerLine()).Label(locales.ExportLines()).AlignSelf(ui.Start).Changed() {
					a.exportLines = lines[selected]
				}
			})
		})
		ui.Column(c).MinHeight(em(c, 48)).Children(func() {
			switch {
			case outputErr != nil:
				ui.Text(c, outputErr.Error()).TextColor(c.Theme().Warning)
			case a.exportFormat == "srt" || a.exportFormat == "vtt":
				muted(c, locales.ExportPlainNote())
			}
		})
	}, func() {
		ui.Button(c, locales.ActionCancel()).OnClick(func() { a.exportOpen = false })
		ui.PrimaryButton(c, locales.ExportConfirm()).Disabled(outputErr != nil || a.snapshot == nil || a.busy).OnClick(func() { a.exportOpen = false; a.exportSubtitle() })
	})
}

func exportFilename(name, format string, options subtitle.OutputOptions, line string) string {
	suffix := map[string]string{"source": "rebuilt-source", "translation": "translated", "bilingual": "bilingual"}[options.Normalized().Mode]
	if options.Normalized().Mode == "source" && options.SourceTrack == "original" {
		suffix = "original"
	}
	if line == "upper" || line == "lower" {
		suffix += "-" + line
	}
	return name + "-" + suffix + "." + format
}

func (a *NativeApp) exportSubtitle() {
	if a.snapshot == nil {
		return
	}
	a.commitEdit(true)
	state, format, line := *a.snapshot, a.exportFormat, a.exportLines
	options, style, secondary, aspect := a.settings.SubtitleOutput, a.currentStyle, a.secondaryStyle, a.frameAspect
	a.run(locales.MenuExportSubtitles(), func(ctx context.Context) (func(), error) {
		document, err := subtitle.SelectOutput(state.OutputDocument(options), options)
		if err != nil {
			return nil, err
		}
		document, err = a.worker.SelectLines(ctx, document, line)
		if err != nil {
			return nil, err
		}
		path, err := a.saveDialog(mygo.SaveDialogOptions{Parent: a.win, Title: locales.MenuExportSubtitles(), DefaultPath: exportFilename(state.Manifest.Name, format, options, line), Filters: []mygo.FileFilter{{Name: strings.ToUpper(format), Extensions: []string{format}}}})
		if err != nil || path == "" {
			return nil, err
		}
		if !strings.EqualFold(filepath.Ext(path), "."+format) {
			path += "." + format
		}
		var data []byte
		if format == "ass" && document.Format != "ass" && document.Format != "ssa" {
			// A line projection is already single-language text. Render that
			// exact result rather than rebuilding a second bilingual layer.
			renderState, renderOptions := state, options
			if line == "upper" || line == "lower" {
				renderState = project.Snapshot{Document: document}
				renderOptions = subtitle.OutputOptions{Mode: "source"}
			}
			styled, buildErr := a.styledDocument(ctx, renderState, style, secondary, renderOptions, aspect)
			if buildErr != nil {
				return nil, buildErr
			}
			if len(styled.Report.MissingGlyphs) > 0 {
				return nil, errors.New(locales.ErrMissingGlyphs(strings.Join(styled.Report.MissingGlyphs, " ")))
			}
			data = []byte(styled.ASS)
		} else {
			document.TargetFormat = format
			data, err = a.worker.Write(ctx, document)
			if err != nil {
				return nil, err
			}
		}
		// The native dialog confirmed any overwrite of an existing file.
		if err = subtitle.Replace(path, data); err != nil {
			return nil, err
		}
		return func() { a.lastOutput = path; a.status = locales.StatusExported(filepath.Base(path)) }, nil
	})
}
