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
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
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

func TestExportSeparatesOriginalRebuiltAndTranslation(t *testing.T) {
	a := storeApp(t, subtitle.Cue{ID: "a", Position: 0, Text: "Hello", OriginalText: "Hello", StartMs: 0, EndMs: 1000}, subtitle.Cue{ID: "b", Position: 1, Text: "world", OriginalText: "world", StartMs: 1000, EndMs: 2000})
	a.worker = subtitle.Client{Executable: os.Getenv("ONESUB_SUBTITLE_WORKER"), Scratch: t.TempDir()}
	rebuilt, _, err := subtitle.Resegment(a.snapshot.Document, []subtitle.SourceRange{{FirstAtom: "a", LastAtom: "b", Reason: "complete thought"}})
	if err != nil {
		t.Fatal(err)
	}
	rebuilt.Cues[0].TranslationSource = rebuilt.Cues[0].Text
	rebuilt.Cues[0].Text, rebuilt.Cues[0].Translated = "你好世界", true
	if _, err = a.store.ApplyTranslationDocument("rebuild", 0, rebuilt); err != nil {
		t.Fatal(err)
	}
	state, err := a.store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	a.setSnapshot(state)
	for _, test := range []struct {
		options      subtitle.OutputOptions
		count        int
		text, suffix string
	}{
		{subtitle.OutputOptions{Mode: "source", SourceTrack: "original"}, 2, "Hello", "original"},
		{subtitle.OutputOptions{Mode: "source"}, 1, "Hello world", "rebuilt-source"},
		{subtitle.OutputOptions{Mode: "translation"}, 1, "你好世界", "translated"},
		{subtitle.OutputOptions{Mode: "bilingual"}, 1, "你好世界\nHello world", "bilingual"},
	} {
		for _, format := range []string{"srt", "vtt"} {
			selected, err := subtitle.SelectOutput(state.OutputDocument(test.options), test.options)
			if err != nil {
				t.Fatal(err)
			}
			selected.TargetFormat = format
			data, err := a.worker.Write(context.Background(), selected)
			if err != nil {
				t.Fatal(err)
			}
			name := exportFilename("lesson", format, test.options)
			if name != "lesson-"+test.suffix+"."+format {
				t.Fatal("ambiguous export filename", name)
			}
			path := filepath.Join(t.TempDir(), name)
			if err = os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			read, err := a.worker.Read(context.Background(), path)
			if err != nil || len(read.Cues) != test.count || read.Cues[0].Text != test.text {
				t.Fatal("wrong export content", test.options, err, read)
			}
		}
	}
	a.exportOpen = true
	a.settings.SubtitleOutput = subtitle.OutputOptions{Mode: "source"}
	tt := settled(a.exportView, 800, 700)
	if err = tt.Click(locales.OutputRebuiltSource()); err != nil {
		t.Fatal(err)
	}
	for _, label := range []string{locales.TabOriginal(), locales.OutputRebuiltSource(), locales.OutputTranslation(), locales.OutputBilingual()} {
		if !tt.HasText(label) {
			t.Fatal("export choice missing", label, tt.Texts())
		}
	}
	if err = tt.Click(locales.TabOriginal()); err != nil {
		t.Fatal(err)
	}
	if a.settings.SubtitleOutput.SourceTrack != "original" || a.settings.SubtitleOutput.Mode != "source" {
		t.Fatal("export selected a label but not its data")
	}
}
