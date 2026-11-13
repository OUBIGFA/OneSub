package subtitle

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestExplicitSkipOmitsOnlyMissingTranslations(t *testing.T) {
	doc := Document{Format: "srt", Cues: []Cue{
		{ID: "done", Text: "译文", TranslationSource: "Source", Translated: true, StartMs: 100, EndMs: 2000},
		{ID: "missing", Text: "Keep this source", StartMs: 2100, EndMs: 4000},
		{ID: "empty", Text: "  ", TranslationSource: "Empty answer", Translated: true, StartMs: 4100, EndMs: 6000},
		{ID: "protected", Text: "Drawing", ReadOnly: true, StartMs: 6100, EndMs: 8000},
	}}
	before, _ := json.Marshal(doc)
	if got := MissingTranslationIndices(doc); !reflect.DeepEqual(got, []int{1, 2}) {
		t.Fatal("missing positions disagree with actual cue state", got)
	}
	if _, err := SelectOutput(doc, OutputOptions{Mode: "translation"}); err == nil {
		t.Fatal("output skipped missing translations without a choice")
	}
	for _, mode := range []string{"translation", "bilingual"} {
		output, err := SelectOutput(doc, OutputOptions{Mode: mode, SkipMissing: true})
		if err != nil || len(output.Cues) != 2 || output.Cues[0].ID != "done" || output.Cues[1].ID != "protected" || output.Cues[0].StartMs != 100 || output.Cues[1].Text != "Drawing" {
			t.Fatal("skip lost completed/protected content or its timing", mode, err, output.Cues)
		}
	}
	output, err := SelectOutput(doc, OutputOptions{Mode: "source", SkipMissing: true})
	if err != nil || len(output.Cues) != 4 || output.Cues[1].Text != "Keep this source" {
		t.Fatal("translation skip affected source output", err, output.Cues)
	}
	after, _ := json.Marshal(doc)
	if string(before) != string(after) {
		t.Fatal("output projection changed the workbench document")
	}
	data, _ := json.Marshal(OutputOptions{Mode: "translation", SkipMissing: true})
	var restored OutputOptions
	if err = json.Unmarshal(data, &restored); err != nil || restored.SkipMissing {
		t.Fatal("one output's skip choice became a persisted preference", err, string(data))
	}
}

func TestPartialTranslationRequiresExplicitChoiceAndUnchangedSource(t *testing.T) {
	base := Document{Format: "srt", Cues: []Cue{
		{ID: "a", Text: "First source", StartMs: 100, EndMs: 2000},
		{ID: "b", Text: "Keep this source", StartMs: 2100, EndMs: 4000},
	}}
	partial, _, err := Resegment(base, []SourceRange{{FirstAtom: "a", LastAtom: "a"}, {FirstAtom: "b", LastAtom: "b"}})
	if err != nil {
		t.Fatal(err)
	}
	partial.Cues[0].TranslationSource, partial.Cues[0].Text, partial.Cues[0].Translated = "First source", "译文", true
	if err = ValidateTranslation(base, partial); err == nil {
		t.Fatal("partial translation was accepted without an explicit skip")
	}
	if err = ValidateTranslation(base, partial, true); err != nil {
		t.Fatal("explicit skip refused an unchanged missing source", err)
	}
	partial.Cues[1].Text = "Changed source"
	if err = ValidateTranslation(base, partial, true); err == nil {
		t.Fatal("skip allowed a changed untranslated source")
	}
}
