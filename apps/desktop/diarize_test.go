package main

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
)

func TestSpeakerMCPArtifactRejectsStaleSubtitles(t *testing.T) {
	root := t.TempDir()
	tools := toolset{dataRoot: root}
	doc := subtitle.Document{Format: "srt", Speakers: &subtitle.SpeakerState{AnalysisID: "test", Names: map[string]string{"a": "A"}}, Cues: []subtitle.Cue{{ID: "a", Text: "Words", StartMs: 0, EndMs: 2000, Speaker: &subtitle.SpeakerAssignment{Status: "single", IDs: []string{"a"}}}}}
	path, err := tools.keepSpeakerArtifact(doc)
	if err != nil {
		t.Fatal(err)
	}
	plain := doc
	plain.Speakers = nil
	plain.Cues = append([]subtitle.Cue{}, doc.Cues...)
	plain.Cues[0].Speaker = nil
	restored, err := attachSpeakerArtifact(plain, path)
	if err != nil || restored.Cues[0].Speaker == nil {
		t.Fatal(restored, err)
	}
	plain.Cues[0].EndMs++
	if _, err = attachSpeakerArtifact(plain, path); err == nil {
		t.Fatal("stale timing accepted")
	}
	plain.Cues[0].EndMs--
	plain.Cues[0].Text = "Changed"
	if _, err = attachSpeakerArtifact(plain, path); err == nil {
		t.Fatal("stale text accepted")
	}
	raw, err := os.ReadFile(path)
	if err != nil {
		t.Fatal(err)
	}
	var saved subtitle.Document
	if err = json.Unmarshal(raw, &saved); err != nil || saved.Cues[0].Text != "Words" {
		t.Fatal("artifact mutated")
	}
}

func TestSpeakerInferenceCacheAndCancellation(t *testing.T) {
	wav := os.Getenv("ONESUB_DIARIZE_TEST_WAV")
	modelRoot := os.Getenv("ONESUB_DIARIZE_TEST_MODELS")
	if wav == "" || modelRoot == "" || os.Getenv("ONESUB_DIARIZE_WORKER") == "" {
		t.Skip("explicit local speaker fixtures required")
	}
	root := t.TempDir()
	scratch := filepath.Join(root, "worker")
	if err := os.MkdirAll(scratch, 0700); err != nil {
		t.Fatal(err)
	}
	tools := toolset{resources: os.Getenv("ONESUB_DIARIZE_TEST_RUNTIME"), dataRoot: root, worker: subtitle.Client{Scratch: scratch}}
	prefs := settings.Settings{SpeakerCount: 2, ModelPaths: map[string]string{"speaker-segmentation": filepath.Join(modelRoot, "speaker-segmentation"), "speaker-embedding": filepath.Join(modelRoot, "speaker-embedding")}}
	state, err := tools.analyzeSpeakers(context.Background(), prefs, wav, wav, func(recognitionStep) {})
	if err != nil {
		t.Fatal(err)
	}
	if len(state.Names) != 2 || len(state.Turns) == 0 {
		t.Fatal(state)
	}
	reused, err := tools.analyzeSpeakers(context.Background(), prefs, wav, wav, func(recognitionStep) { t.Error("cache reran inference") })
	if err != nil || reused.AnalysisID != state.AnalysisID {
		t.Fatal(reused, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = tools.analyzeSpeakers(ctx, prefs, wav, wav, func(recognitionStep) {}); err == nil {
		t.Fatal("cancelled task succeeded")
	}
	prefs.SpeakerCount = 0
	ctx, cancel = context.WithCancel(context.Background())
	defer cancel()
	started := false
	_, err = tools.analyzeSpeakers(ctx, prefs, wav, wav, func(p recognitionStep) {
		if p.Stage == "diarize" && p.Total > 0 {
			started = true
			cancel()
		}
	})
	if !started || err == nil || ctx.Err() == nil {
		t.Fatal("running child cancellation was not exercised", started, err)
	}
}
