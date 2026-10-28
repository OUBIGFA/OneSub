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

// These choices are shared by preview, compose and subtitle export. Language
// order and visual priority are separate, as in professional bilingual editors.
func (a *NativeApp) outputFields(c *ui.Context) {
	o := a.settings.SubtitleOutput.Normalized()
	ui.Field(c, locales.OutputContent(), func() {
		idSelect(c, &o.Mode, outputModes()).Label(locales.OutputContent()).OnChange(func() {
			a.settings.SubtitleOutput = o
			a.saveSettings()
		})
	})
	if o.Mode != "bilingual" {
		return
	}
	ui.Row(c).Wrap().Gap(20).Children(func() {
		order := "translation-first"
		if o.SourceFirst {
			order = "source-first"
		}
		ui.Field(c, locales.OutputOrder(), func() {
			idSelect(c, &order, []option{{"translation-first", locales.OutputTranslationFirst()}, {"source-first", locales.OutputSourceFirst()}}).Label(locales.OutputOrder()).OnChange(func() {
				o.SourceFirst = order == "source-first"
				a.settings.SubtitleOutput = o
				a.saveSettings()
			})
		})
		ui.Field(c, locales.OutputPrimary(), func() {
			idSelect(c, &o.Primary, []option{{"translation", locales.TabTranslation()}, {"source", locales.TabSource()}}).Label(locales.OutputPrimary()).OnChange(func() {
				a.settings.SubtitleOutput = o
				a.saveSettings()
			})
		})
	})
}

func (a *NativeApp) exportView(c *ui.Context) {
	ui.Modal(c, &a.exportOpen, func() {
		ui.Column(c).Width(620).Gap(24).Children(func() {
			ui.Text(c, locales.MenuExportSubtitles()).FontSize(18).FontWeight(600)
			ui.Form(c, func() {
				a.outputFields(c)
				formats := []string{"srt", "vtt", "ass"}
				if a.snapshot != nil && a.snapshot.Document.Format == "ssa" {
					formats = append(formats, "ssa")
				}
				ui.Field(c, locales.ExportFormat(), func() { ui.Select(c, &a.exportFormat, formats).Label(locales.ExportFormat()) })
			})
			if a.exportFormat == "srt" || a.exportFormat == "vtt" {
				ui.Text(c, locales.ExportPlainNote()).TextColor(c.Theme().TextMuted)
			}
			var err error
			if a.snapshot != nil {
				_, err = subtitle.SelectOutput(a.snapshot.Document, a.settings.SubtitleOutput)
			}
			if err != nil {
				ui.Text(c, err.Error()).TextColor(c.Theme().Warning)
			}
			ui.Row(c).Gap(12).Justify(ui.End).Children(func() {
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
