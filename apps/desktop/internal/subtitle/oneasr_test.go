package subtitle

import (
	"reflect"
	"testing"
)

// Upstream timeline::place_whole/round_millis retain collapsed aligner tokens.
// The sentence remains usable; only word-level splitting loses its evidence.
func TestOneAsrZeroDurationRetainsSentenceAndOtherAnchors(t *testing.T) {
	doc := Document{Format: "srt", Cues: []Cue{
		{ID: "a", Text: "Hello!", OriginalText: "Hello!", StartMs: 0, EndMs: 1000},
		{ID: "b", Text: "Next", OriginalText: "Next", StartMs: 1000, EndMs: 2000},
	}}
	raw := []byte(`{"unit":"aligner_token","word_count":3,"words":[{"text":"Hello","start":0.1,"end":0.8},{"text":"!","start":0.8,"end":0.8},{"text":"Next","start":1.1,"end":1.9}]}`)
	out, warnings, err := AttachOneAsrWords(doc, raw)
	if err != nil || len(warnings) != 1 || out.Cues[0].Text != "Hello!" || len(out.Cues[0].Words) != 0 || len(out.Cues[1].Words) != 1 {
		t.Fatal(out, warnings, err)
	}
	if out.Cues[0].StartMs != 0 || out.Cues[0].EndMs != 1000 {
		t.Fatal("invented sentence time")
	}
	if !reflect.DeepEqual(doc.Cues[0].Words, []Word(nil)) {
		t.Fatal("mutated source")
	}
}
func TestOneAsrRejectsNegativeDurationAndMalformedEvidence(t *testing.T) {
	for _, raw := range []string{
		`{"unit":"aligner_token","word_count":1,"words":[{"text":"x","start":1,"end":0}]}`,
		`{"unit":"aligner_token","word_count":1,"words":[{"text":"x","start":-1,"end":0}]}`,
		`{"unit":"seconds","word_count":0,"words":[]}`,
		`{"unit":"aligner_token","word_count":1,"words":[]}`,
	} {
		if _, _, err := AttachOneAsrWords(Document{}, []byte(raw)); err == nil {
			t.Fatal(raw)
		}
	}
}
