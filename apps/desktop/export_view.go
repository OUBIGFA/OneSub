package main

import (
	"context"
	"errors"
	"path/filepath"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func outputModes() []option {
	return []option{
		{"source", locales.OutputSource()},
		{"translation", locales.OutputTranslation()},
		{"bilingual", locales.OutputBilingual()},
	}
}

// outputFields are the subtitle content choices shared by the preview,
// video export and subtitle export. Language order and visual priority are
// separate, as in professional bilingual editors. In dialogs (keepRoom) the
// bilingual choices stay in place, disabled, so that choosing the content
// never resizes the dialog.
func (a *NativeApp) outputFields(c *ui.Context, keepRoom bool) {
	o := a.settings.SubtitleOutput.Normalized()
	modes := []string{"source", "translation", "bilingual"}
	mode := 0
	for i, m := range modes {
		if m == o.Mode {
			mode = i
		}
	}
	ui.Field(c, locales.OutputContent(), func() {
		if ui.Segmented(c, &mode, locales.TabSource(), locales.TabTranslation(), locales.TabBilingual()).Label(locales.OutputContent()).AlignSelf(ui.Start).Changed() {
			o.Mode = modes[mode]
			a.settings.SubtitleOutput = o
			a.saveSettings()
		}
	})
	bilingual := o.Mode == "bilingual"
	if !bilingual && !keepRoom {
		return
	}
	order := "translation-first"
	if o.SourceFirst {
		order = "source-first"
	}
	// In a dialog the choices span its form; in the style panel they keep
	// their own width, as its other controls do.
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

// exportView has one fixed size: every choice keeps its place, and the
// note below them has a fixed room.
func (a *NativeApp) exportView(c *ui.Context) {
	ui.Modal(c, &a.exportOpen, func() {
		ui.Column(c).Width(em(c, 560)).Height(em(c, 384)).Gap(em(c, 20)).Children(func() {
			dialogTitle(c, locales.MenuExportSubtitles())
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
			ui.Form(c, func() {
				a.outputFields(c, true)
				ui.Field(c, locales.ExportFormat(), func() {
					if ui.Segmented(c, &format, labels...).Label(locales.ExportFormat()).AlignSelf(ui.Start).Changed() {
						a.exportFormat = formats[format]
					}
				})
			})
			var err error
			if a.snapshot != nil {
				_, err = subtitle.SelectOutput(a.snapshot.Document, a.settings.SubtitleOutput)
			}
			note := ui.Column(c).Height(em(c, 48)).Shrink(0)
			note.Children(func() {
				switch {
				case err != nil:
					ui.Text(c, err.Error()).TextColor(c.Theme().Warning)
				case a.exportFormat == "srt" || a.exportFormat == "vtt":
					muted(c, locales.ExportPlainNote())
				}
			})
			ui.Spacer(c)
			ui.Row(c).Gap(em(c, 8)).Justify(ui.End).Children(func() {
				ui.Button(c, locales.ActionCancel()).OnClick(func() { a.exportOpen = false })
				ui.PrimaryButton(c, locales.ExportConfirm()).Disabled(err != nil || a.snapshot == nil || a.busy).OnClick(func() { a.exportOpen = false; a.exportSubtitle() })
			})
		})
	})
}

func exportFilename(name, format string, options subtitle.OutputOptions) string {
	suffix := map[string]string{"source": "source", "translation": "translated", "bilingual": "bi"}[options.Normalized().Mode]
	return name + "-" + suffix + "." + format
}

func (a *NativeApp) exportSubtitle() {
	if a.snapshot == nil {
		return
	}
	a.commitEdit(true)
	state, format := *a.snapshot, a.exportFormat
	options, style, secondary, aspect := a.settings.SubtitleOutput, a.currentStyle, a.secondaryStyle, a.frameAspect
	a.run(locales.MenuExportSubtitles(), func(ctx context.Context) (func(), error) {
		document, err := subtitle.SelectOutput(state.Document, options)
		if err != nil {
			return nil, err
		}
		path, err := a.saveDialog(mygo.SaveDialogOptions{Parent: a.win, Title: locales.MenuExportSubtitles(), DefaultPath: exportFilename(state.Manifest.Name, format, options), Filters: []mygo.FileFilter{{Name: strings.ToUpper(format), Extensions: []string{format}}}})
		if err != nil || path == "" {
			return nil, err
		}
		if !strings.EqualFold(filepath.Ext(path), "."+format) {
			path += "." + format
		}
		var data []byte
		if format == "ass" && state.Document.Format != "ass" && state.Document.Format != "ssa" {
			styled, buildErr := a.styledDocument(ctx, state, style, secondary, options, aspect)
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
