package subtitle

import (
	"context"
	"testing"
)

func TestSpeakerEvidenceDoesNotGuessAcrossGaps(t *testing.T) {
	turns := []SpeakerTurn{{StartMs: 0, EndMs: 2000, ID: "a"}, {StartMs: 2000, EndMs: 4000, ID: "b"}}
	for _, c := range []struct {
		start, end int64
		status     string
	}{{0, 1500, "single"}, {0, 4000, "mixed"}, {8000, 9000, "unknown"}} {
		a := AssignSpeaker(Cue{StartMs: c.start, EndMs: c.end}, turns, "run")
		if a.Status != c.status {
			t.Fatalf("%+v: %+v", c, a)
		}
	}
	overlap := AssignSpeaker(Cue{StartMs: 0, EndMs: 2000}, []SpeakerTurn{{StartMs: 0, EndMs: 2000, ID: "a"}, {StartMs: 500, EndMs: 1500, ID: "b"}}, "run")
	if overlap.Status != "overlap" {
		t.Fatal(overlap)
	}
}

func TestSpeakerBoundarySurvivesAutomaticAndManualMerge(t *testing.T) {
	doc := Document{Format: "srt", Cues: []Cue{
		{ID: "a", Text: "Hello", StartMs: 0, EndMs: 1000, Words: []Word{{Text: "Hello", StartMs: 0, EndMs: 1000}}, Speaker: &SpeakerAssignment{Status: "single", IDs: []string{"a"}}},
		{ID: "b", Text: "there", StartMs: 1000, EndMs: 2000, Words: []Word{{Text: "there", StartMs: 1000, EndMs: 2000}}, Speaker: &SpeakerAssignment{Status: "single", IDs: []string{"b"}}},
	}}
	got, err := SegmentRecognition(context.Background(), doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(got.Cues) != 2 {
		t.Fatal("joined different people", got)
	}
	atoms, _ := TimelineAtoms(doc)
	if _, _, err := Resegment(doc, []SourceRange{{FirstAtom: atoms[0].ID, LastAtom: atoms[len(atoms)-1].ID, Reason: "join"}}); err == nil {
		t.Fatal("AI merged speakers")
	}
	merged, err := MergeAdjacent(doc, 0, "joined")
	if err != nil {
		t.Fatal(err)
	}
	if merged.Cues[0].Speaker == nil || merged.Cues[0].Speaker.Status != "mixed" {
		t.Fatal("manual merge lost people")
	}
}

func TestSpeakerOutputIsOnlyAProjection(t *testing.T) {
	doc := Document{Format: "srt", Speakers: &SpeakerState{Names: map[string]string{"a": "主持人"}}, Cues: []Cue{{ID: "1", Text: "内容", Speaker: &SpeakerAssignment{IDs: []string{"a"}, Status: "single"}}}}
	out := WithSpeakerNames(doc)
	if out.Cues[0].Text != "主持人：内容" || doc.Cues[0].Text != "内容" {
		t.Fatal(out, doc)
	}
}

func TestSpeakerInvalidClockStaysUnknown(t *testing.T) {
	a := AssignSpeaker(Cue{StartMs: 9000, EndMs: 1000}, []SpeakerTurn{{StartMs: 2000, EndMs: 3000, ID: "a"}}, "run")
	if a.Status != "unknown" {
		t.Fatal(a)
	}
}

func TestSpeakerOutputBilingualNameOnce(t *testing.T) {
	doc := Document{Format: "srt", Speakers: &SpeakerState{Names: map[string]string{"a": "A"}}, Cues: []Cue{{ID: "1", Text: "Translation", TranslationSource: "Source", Translated: true, Speaker: &SpeakerAssignment{Status: "single", IDs: []string{"a"}}}}}
	out, err := SelectOutput(doc, OutputOptions{Mode: "bilingual", SourceFirst: true, ShowSpeakers: true})
	if err != nil {
		t.Fatal(err)
	}
	if out.Cues[0].Text != "A：Source\nTranslation" || doc.Cues[0].Text != "Translation" {
		t.Fatal(out, doc)
	}
}

func TestSpeakerManualCorrectionSurvivesRebuild(t *testing.T) {
	state := &SpeakerState{AnalysisID: "run", Names: map[string]string{"a": "A", "b": "B"}, Turns: []SpeakerTurn{{StartMs: 0, EndMs: 3000, ID: "a"}}}
	doc := Document{Format: "srt", Speakers: state, Cues: []Cue{{ID: "1", Text: "Hello", StartMs: 0, EndMs: 1000, Speaker: &SpeakerAssignment{Status: "single", IDs: []string{"b"}, Manual: true}}, {ID: "2", Text: "world", StartMs: 1000, EndMs: 2000, Speaker: &SpeakerAssignment{Status: "single", IDs: []string{"b"}, Manual: true}}}}
	got, _, err := Resegment(doc, []SourceRange{{FirstAtom: "1", LastAtom: "2", Reason: "join"}})
	if err != nil {
		t.Fatal(err)
	}
	if got.Cues[0].Speaker == nil || !got.Cues[0].Speaker.Manual || got.Cues[0].Speaker.IDs[0] != "b" {
		t.Fatal("rebuild restored wrong automatic speaker", got.Cues[0].Speaker)
	}
}
