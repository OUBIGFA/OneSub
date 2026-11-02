package subtitle

import (
	"reflect"
	"testing"
)

func TestManualMergePreservesTracksTimesAndRepeatedBlockMap(t *testing.T) {
	doc := Document{Format: "srt", Cues: []Cue{
		{ID: "a", Text: "One", OriginalText: "One", StartMs: 1000, EndMs: 2000, Words: []Word{{Text: "One", StartMs: 1000, EndMs: 2000}}},
		{ID: "b", Text: "Two", OriginalText: "Two", StartMs: 500, EndMs: 3000},
		{ID: "c", Text: "Three", OriginalText: "Three", StartMs: 4000, EndMs: 5000},
	}}
	first, err := MergeAdjacent(doc, 0, "manual-first")
	if err != nil {
		t.Fatal(err)
	}
	second, err := MergeAdjacent(first, 0, "manual-second")
	if err != nil {
		t.Fatal(err)
	}
	cue := second.Cues[0]
	if cue.Text != "One, Two, Three" || cue.StartMs != 500 || cue.EndMs != 5000 || len(cue.Words) != 0 || !reflect.DeepEqual(cue.MergedCueIDs, []string{"a", "b", "c"}) || doc.Cues[0].Text != "One" || doc.Cues[1].Position != 0 {
		t.Fatal("merge lost content, anchors or changed input", cue, doc)
	}
}

func TestMergeAddsOneCommaAtTheVisibleBoundary(t *testing.T) {
	for _, tc := range []struct {
		first, second, want string
		ass                 bool
	}{
		{"First.", "Second.", "First, Second.", false},
		{"第一段。", "第二段。", "第一段，第二段。", false},
		{"第一段，", "第二段", "第一段，第二段", false},
		{"First,", "Second", "First, Second", false},
		{"First.", ", Second.", "First, Second.", false},
		{"第一段。", "，第二段。", "第一段，第二段。", false},
		{"First.", "<i>, Second.</i>", "First, <i>Second.</i>", false},
		{"第一段。", "“，第二段。”", "第一段，“第二段。”", false},
		{"First.", "第二段。", "First，第二段。", false},
		{"First\nline.", "Next\nline.", "First\nline, Next\nline.", false},
		{"<i>First.</i>", "Second.", "<i>First</i>, Second.", false},
		{"他说“好的。”", "继续", "他说“好的”，继续", false},
		{"3.14", "Next", "3.14, Next", false},
		{`{\i1}First.{\i0}`, "Second.", `{\i1}First{\i0}, {\r}Second.`, true},
	} {
		if got := joinMergeText(tc.first, tc.second, tc.ass); got != tc.want {
			t.Errorf("%q + %q = %q, want %q", tc.first, tc.second, got, tc.want)
		}
	}
}

func TestManualMergeJoinsSourceAndTranslationIndependently(t *testing.T) {
	doc := Document{Format: "srt", Cues: []Cue{
		{ID: "a", StartMs: 1000, EndMs: 2000, Text: "First.", OriginalText: "第一段。", TranslationSource: "第一段。", Translated: true},
		{ID: "b", StartMs: 2000, EndMs: 3000, Text: "Second.", OriginalText: "第二段。", TranslationSource: "第二段。", Translated: true},
	}}
	merged, err := MergeAdjacent(doc, 0, "manual-bilingual")
	if err != nil {
		t.Fatal(err)
	}
	cue := merged.Cues[0]
	if cue.Text != "First, Second." || cue.TranslationSource != "第一段，第二段。" || cue.OriginalText != "第一段，第二段。" {
		t.Fatal("merge used interface or translation typography for the other text", cue)
	}
}

func TestManualMergeRejectsPartialTranslationAndProtectedContent(t *testing.T) {
	for _, cues := range [][]Cue{
		{{ID: "a", ReadOnly: true}, {ID: "b"}},
		{{ID: "a"}, {ID: "b", ReadOnly: true}},
		{{ID: "a", Translated: true, Text: "Translated", TranslationSource: "Source"}, {ID: "b", Text: "Missing"}},
	} {
		if _, err := MergeAdjacent(Document{Format: "srt", Cues: cues}, 0, "merged"); err == nil {
			t.Fatal("merge would discard protected/partial content")
		}
	}
}
