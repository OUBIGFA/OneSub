package main

import (
	"context"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
)

// Exercise the actual shared post-ASR file boundary and libse round trip.
// Reattaching the unchanged evidence must still cover every displayed cue.
func TestRecognitionLocalSegmentationFile(t *testing.T) {
	worker := os.Getenv("ONESUB_SUBTITLE_WORKER")
	if worker == "" {
		t.Skip("run build/test.ps1")
	}
	ctx := context.Background()
	root := t.TempDir()
	tools := toolset{worker: subtitle.Client{Executable: worker, Scratch: root}}
	text := "We first inspect the complete structure of the model, then carefully adjust the edges and check the final result."
	var words []subtitle.Word
	for i, word := range strings.Fields(text) {
		words = append(words, subtitle.Word{Text: word, StartMs: int64(i) * 350, EndMs: int64(i+1) * 350})
	}
	doc := subtitle.Document{Format: "srt", TargetFormat: "srt", Cues: []subtitle.Cue{{ID: "native", Text: text, StartMs: 0, EndMs: int64(len(words)) * 350}}}
	data, err := tools.worker.Write(ctx, doc)
	if err != nil {
		t.Fatal(err)
	}
	path := filepath.Join(root, "native.srt")
	if err := os.WriteFile(path, data, 0600); err != nil {
		t.Fatal(err)
	}
	evidence, _ := subtitle.MarshalOneAsrWords(words)
	if err := tools.segmentRecognition(ctx, path, evidence); err != nil {
		t.Fatal(err)
	}
	out, err := tools.worker.Read(ctx, path)
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Cues) < 2 {
		t.Fatal("long engine output was not segmented")
	}
	out, warnings, err := subtitle.AttachOneAsrWords(out, evidence)
	if err != nil || len(warnings) != 0 {
		t.Fatal(warnings, err)
	}
	joined := ""
	for _, cue := range out.Cues {
		joined += " " + cue.Text
		if len(cue.Words) == 0 || cue.TimingQuality != "aligned" {
			t.Fatal(cue)
		}
	}
	if strings.Join(strings.Fields(joined), " ") != text {
		t.Fatal("changed original wording", joined)
	}
	before, _ := os.ReadFile(path)
	if err := tools.segmentRecognition(ctx, path, []byte("invalid")); err == nil {
		t.Fatal("ignored malformed evidence")
	}
	after, _ := os.ReadFile(path)
	if string(before) != string(after) {
		t.Fatal("overwrote source on failure")
	}
}
