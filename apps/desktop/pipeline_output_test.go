package main

import (
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/styles"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
)

// The moved controls must not keep mutating the shared desktop/MCP
// recognition output through an old, hidden preferences path.
func TestRecognitionOutputPreservesPunctuationAfterControlsMove(t *testing.T) {
	root := t.TempDir()
	tools := toolset{worker: subtitle.Client{Executable: os.Getenv("ONESUB_SUBTITLE_WORKER"), Scratch: root}}
	doc := subtitle.Document{Format: "srt", Cues: []subtitle.Cue{{ID: "a", Text: "First. 第二句。", StartMs: 100, EndMs: 3000}}}
	for _, format := range []string{"srt", "vtt"} {
		t.Run(format, func(t *testing.T) {
			prefs := settings.Default()
			prefs.RecognitionFormat = format
			prefs.StripPunctuation, prefs.ReplacePeriods = true, true
			data, err := tools.renderRecognition(context.Background(), doc, prefs, styles.Style{}, 16.0/9)
			if err != nil {
				t.Fatal(err)
			}
			path := filepath.Join(root, "recognized."+format)
			if err = os.WriteFile(path, data, 0600); err != nil {
				t.Fatal(err)
			}
			back, err := tools.worker.Read(context.Background(), path)
			if err != nil || len(back.Cues) != 1 || back.Cues[0].Text != doc.Cues[0].Text {
				t.Fatal("translation cleanup still changed recognized subtitles", err, back.Cues)
			}
		})
	}
}
