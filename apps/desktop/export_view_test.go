package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/styles"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
)

func TestOutputArtifactsUseSelectedContentAndSmartSubLayers(t *testing.T) {
	a := withStyleCatalog(t)
	a.worker = subtitle.Client{Executable: os.Getenv("ONESUB_SUBTITLE_WORKER"), Scratch: t.TempDir()}
	a.styleClient = styles.Client{Executable: os.Getenv("ONESUB_STYLE_WORKER"), Scratch: t.TempDir()}
	secondary := a.styleCatalog.DefaultStyle
	secondary.FontSize = 16
	secondary.Italic = true
	doc := subtitle.Document{Format: "srt", Cues: []subtitle.Cue{{ID: "c1", Text: "译文", TranslationSource: "Edited source", OriginalText: "raw source", Translated: true, StartMs: 1000, EndMs: 3000}}}
	for _, mode := range []string{"source", "translation", "bilingual"} {
		for _, format := range []string{"srt", "vtt"} {
			selected, err := subtitle.SelectOutput(doc, subtitle.OutputOptions{Mode: mode, SourceFirst: true})
			if err != nil {
				t.Fatal(err)
			}
			selected.TargetFormat = format
			raw, err := a.worker.Write(context.Background(), selected)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(t.TempDir(), "output."+format)
			os.WriteFile(path, raw, 0600)
			read, err := a.worker.Read(context.Background(), path)
			if err != nil || read.Cues[0].Text != selected.Cues[0].Text {
				t.Fatal(read, err)
			}
			if strings.Contains(string(raw), "raw source") {
				t.Fatal("immutable original replaced current source")
			}
		}
	}
	for _, primary := range []string{"source", "translation"} {
		for _, sourceFirst := range []bool{false, true} {
			opts := subtitle.OutputOptions{Mode: "bilingual", Primary: primary, SourceFirst: sourceFirst}
			styled, err := a.styledDocument(context.Background(), project.Snapshot{Document: doc}, a.currentStyle, secondary, opts, 16.0/9)
			if err != nil || len(styled.Report.MissingGlyphs) > 0 || len(styled.Fonts) == 0 {
				t.Fatal(styled.Report, err)
			}
			main, second := "译文", "Edited source"
			if primary == "source" {
				main, second = second, main
			}
			want := "{\\rDefault}" + main + "\\N{\\rSecondary}" + second
			if sourceFirst != (primary == "source") {
				want = "{\\rSecondary}" + second + "\\N{\\rDefault}" + main
			}
			if !strings.Contains(styled.ASS, want) || !strings.Contains(styled.ASS, ",16,") {
				t.Fatal(styled.ASS, want)
			}
		}
	}
}
