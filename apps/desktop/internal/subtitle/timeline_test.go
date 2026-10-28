package subtitle

import (
	"reflect"
	"testing"
)

func timelineFixture() Document {
	return Document{Format: "srt", Cues: []Cue{
		{ID: "c1", Position: 0, StartMs: 100, EndMs: 2000, Text: "Hello world", OriginalText: "Hello world", Words: []Word{{Text: "Hello", StartMs: 200, EndMs: 800}, {Text: " world", StartMs: 1100, EndMs: 1800}}},
		{ID: "c2", Position: 1, StartMs: 2300, EndMs: 4000, Text: "again", OriginalText: "again"},
	}}
}
func TestResegmentWordCutsKeepOuterBoundariesAndCoverage(t *testing.T) {
	doc := timelineFixture()
	ranges := []SourceRange{{"c1:w0", "c1:w0", "分开完整语义单元"}, {"c1:w1", "c2", "修复跨句悬空词尾"}}
	out, warnings, err := Resegment(doc, ranges)
	if err != nil {
		t.Fatal(err)
	}
	if out.Cues[0].StartMs != 100 || out.Cues[0].EndMs != 1100 || out.Cues[1].StartMs != 1100 || out.Cues[1].EndMs != 4000 || out.Cues[1].Text != " world again" || len(warnings) != 1 {
		t.Fatal(out, warnings)
	}
	for i := range out.Cues {
		out.Cues[i].Translated = true
		out.Cues[i].TranslationSource = out.Cues[i].Text
		out.Cues[i].Text = "译文"
	}
	if err := ValidateTranslation(doc, out); err != nil {
		t.Fatal(err)
	}
	out.Cues[0].EndMs++
	if err := ValidateTranslation(doc, out); err == nil {
		t.Fatal("tampered time accepted")
	}
	if doc.Cues[0].Text != "Hello world" || doc.Cues[0].EndMs != 2000 {
		t.Fatal("source mutated")
	}
}
func TestResegmentRejectsUnsafeRanges(t *testing.T) {
	for name, ranges := range map[string][]SourceRange{
		"missing":       {{"c1:w0", "c1:w1", ""}},
		"duplicate":     {{"c1:w0", "c1:w1", ""}, {"c1:w1", "c2", "bad"}},
		"out-of-bounds": {{"c1:w0", "absent", "bad"}},
		"no-reason":     {{"c1:w0", "c2", ""}},
		"invented-word": {{"c1:w0", "c1:w1", ""}, {"c2:w0", "c2:w1", "bad"}},
	} {
		t.Run(name, func(t *testing.T) {
			if _, _, err := Resegment(timelineFixture(), ranges); err == nil {
				t.Fatal("accepted")
			}
		})
	}
	for _, format := range []string{"ass", "ssa", "vtt"} {
		doc := timelineFixture()
		doc.Format = format
		if _, _, err := Resegment(doc, []SourceRange{{"c1", "c2", "merge"}}); err == nil {
			t.Fatal(format)
		}
	}
	for _, lock := range []string{"text", "time", "drawing", "overlap"} {
		doc := timelineFixture()
		doc.Cues[0].Words = nil
		switch lock {
		case "text":
			doc.Cues[0].TextLocked = true
		case "time":
			doc.Cues[0].TimeLocked = true
		case "drawing":
			doc.Cues[0].ReadOnly = true
		case "overlap":
			doc.Cues[1].StartMs = 1500
		}
		if _, _, err := Resegment(doc, []SourceRange{{"c1", "c2", "merge"}}); err == nil {
			t.Fatal(lock)
		}
	}
}
func TestOneAsrWordsExactMappingAndFallback(t *testing.T) {
	doc := timelineFixture()
	doc.Cues[0].Words = nil
	raw := []byte(`{"unit":"aligner_token","word_count":3,"words":[{"text":"Hello","start":0.2,"end":0.8},{"text":"world","start":1.1,"end":1.8},{"text":"different","start":2.4,"end":3.8}]}`)
	out, warnings, err := AttachOneAsrWords(doc, raw)
	if err != nil || len(warnings) != 1 || !reflect.DeepEqual(out.Cues[0].Words, timelineFixture().Cues[0].Words) || len(out.Cues[1].Words) != 0 {
		t.Fatal(out, warnings, err)
	}
}
func TestSelectOutputPreservesEditedSourceAndOrder(t *testing.T) {
	doc := Document{Format: "srt", Cues: []Cue{{ID: "c1", Text: "译文", TranslationSource: "corrected source", OriginalText: "raw", Translated: true}}}
	for _, primary := range []string{"source", "translation"} {
		for _, first := range []bool{true, false} {
			out, err := SelectOutput(doc, OutputOptions{Mode: "bilingual", Primary: primary, SourceFirst: first})
			want := "译文\ncorrected source"
			if first {
				want = "corrected source\n译文"
			}
			if err != nil || out.Cues[0].Text != want {
				t.Fatal(out, err)
			}
		}
	}
	out, err := SelectOutput(doc, OutputOptions{Mode: "source"})
	if err != nil || out.Cues[0].Text != "corrected source" {
		t.Fatal(out, err)
	}
	doc.Cues[0].Translated = false
	if _, err := SelectOutput(doc, OutputOptions{Mode: "translation"}); err == nil {
		t.Fatal("missing translation silently filled")
	}
}
