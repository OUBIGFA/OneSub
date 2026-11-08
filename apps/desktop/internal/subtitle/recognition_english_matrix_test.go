package subtitle

import (
	"context"
	"strings"
	"testing"
)

// Independent examples for the grammar categories in the source review.
// They exercise actual output, not a list of keywords found in the source.
func TestRecognitionEnglishClauseFamilies(t *testing.T) {
	for _, opening := range []string{
		"because", "although", "though", "while", "whereas", "unless", "if", "when",
		"before", "after", "since", "until", "once", "as", "whether",
		"as soon as", "as long as", "even if", "even though", "provided that",
		"as if", "as though", "now that", "in case", "so that", "in order that",
		"however", "therefore", "nevertheless", "otherwise", "meanwhile", "then",
	} {
		t.Run(opening, func(t *testing.T) {
			first := "We should check the complete material settings carefully"
			second := opening + " we can change the final appearance of the model."
			cue := punctuatedRecognitionCue(first + " " + second)
			out, err := SegmentRecognition(context.Background(), Document{Format: "srt", Cues: []Cue{cue}})
			if err != nil || len(out.Cues) != 2 || strings.TrimSpace(out.Cues[0].Text) != first || strings.TrimSpace(out.Cues[1].Text) != second {
				t.Fatal("did not keep the clause opener with its clause", out.Cues, err)
			}
		})
	}
}

func TestRecognitionEnglishProtectedFamilies(t *testing.T) {
	for _, tc := range [][2]string{
		{"black and white", "We use black and white materials for the complete collection of demonstration objects."},
		{"black or white", "We can choose black or white materials for the complete collection of demonstration objects."},
		{"either black or white", "We can choose either black or white materials for the complete collection of demonstration objects."},
		{"both black and white", "We can use both black and white materials for the complete collection of demonstration objects."},
		{"the complete material settings", "We should inspect the complete material settings for every object in this example project."},
		{"Dr. Jane Smith", "We should ask Dr. Jane Smith to review every material setting in the complete example project."},
		{"twenty five percent", "We should increase the brightness by twenty five percent in the complete example project."},
		{"a bit too bright", "The example scene looks a bit too bright for the final version of the complete project."},
		{"a little bit too bright", "The example scene looks a little bit too bright for the final version of the complete project."},
		{"a tiny bit too bright", "The example scene looks a tiny bit too bright for the final version of the complete project."},
		{"kind of bright", "The example scene looks kind of bright for the final version of the complete project."},
		{"sort of bright", "The example scene looks sort of bright for the final version of the complete project."},
		{"should not have been changed", "The material settings should not have been changed during the final review of the complete project."},
		{"will have been working", "The artists will have been working on this complete collection for several months by Friday."},
		{"wouldn't have worked", "The original material wouldn't have worked for all the objects in the complete example project."},
		{"look forward to", "We look forward to reviewing the complete collection of reusable materials with the artists."},
		{"take care of", "We should take care of every remaining material adjustment in the complete example project."},
		{"in front of", "We can place the camera in front of the complete collection of objects in the example project."},
		{"because of", "We must review the material because of the changes to the complete lighting setup in this project."},
		{"as well as", "We can inspect the models as well as the textures in the complete collection of reusable assets."},
		{"not only", "We should inspect not only the materials but also the lighting in the complete example project."},
		{"but also", "We should inspect not only the materials but also the lighting in the complete example project."},
	} {
		t.Run(tc[0], func(t *testing.T) {
			cue := punctuatedRecognitionCue(tc[1])
			out, err := SegmentRecognition(context.Background(), Document{Format: "srt", Cues: []Cue{cue}})
			if err != nil {
				t.Fatal(err)
			}
			found := false
			for _, c := range out.Cues {
				found = found || strings.Contains(c.Text, tc[0])
			}
			if !found {
				t.Fatal("separated protected phrase", out.Cues)
			}
		})
	}
}

func TestRecognitionPrepositionAndDegreeAreNotClauseOpeners(t *testing.T) {
	for _, text := range []string{
		"We inspect the models after lunch in the studio for the complete project that belongs to the client.",
		"We have worked here since Monday on the detailed materials for the complete collection of models.",
		"The display is so much brighter than the original material that we should carefully inspect the settings.",
	} {
		_, preferred := recognitionConnectorLanes(recognitionSplitText(text))
		if len(preferred.boundaries) > 2 {
			t.Fatal("treated a preposition or degree modifier as a clause", text, preferred.boundaries)
		}
	}
}

func TestRecognitionLargeChunksRejoinOnlyUnfinishedSentence(t *testing.T) {
	first := punctuatedRecognitionCue(strings.Repeat("We inspect every item in the collection. ", 14) + "Perhaps this looks a bit")
	second := punctuatedRecognitionCue("too bright so we can adjust the gamma setting before saving. " + strings.TrimSpace(strings.Repeat("We inspect every item in the collection. ", 14)))
	second.ID = "next"
	offset := first.EndMs + 435
	second.StartMs += offset
	second.EndMs += offset
	for i := range second.Words {
		second.Words[i].StartMs += offset
		second.Words[i].EndMs += offset
	}
	out, err := SegmentRecognition(context.Background(), Document{Format: "srt", Cues: []Cue{first, second}})
	if err != nil {
		t.Fatal(err)
	}
	found := false
	for _, c := range out.Cues {
		found = found || strings.Contains(c.Text, "a bit too bright")
	}
	if !found {
		t.Fatal("large chunks prevented continuation repair")
	}
}

func TestRecognitionCoordinatedPhrasesWithModifiers(t *testing.T) {
	for _, pair := range [][2]string{
		{"We use black or very dark materials in the example", "black or very dark"},
		{"We can inspect the models and the textures in the example", "models and the textures"},
		{"The client assigned this complete project to you and me", "you and me"},
	} {
		cue := punctuatedRecognitionCue(pair[0] + " because we need a complete collection of reusable assets for the client.")
		out, err := SegmentRecognition(context.Background(), Document{Format: "srt", Cues: []Cue{cue}})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, c := range out.Cues {
			found = found || strings.Contains(c.Text, pair[1])
		}
		if !found {
			t.Fatal("split modified coordination", out.Cues)
		}
	}
	first := "We should inspect every material in the model"
	second := "and Alice can review all the changes before the final delivery."
	cue := punctuatedRecognitionCue(first + " " + second)
	out, err := SegmentRecognition(context.Background(), Document{Format: "srt", Cues: []Cue{cue}})
	if err != nil || len(out.Cues) != 2 || out.Cues[0].Text != first || out.Cues[1].Text != second {
		t.Fatal("mistook clause subject for a coordinated noun", out.Cues, err)
	}
}

func TestManualSplitTrimsBilingualLinesWithoutChangingPayload(t *testing.T) {
	first := "First we inspect every material in the collection. Then we review the complete lighting setup."
	second := "首先我们检查集合中的每一个材质。然后再检查整个场景的完整灯光设置。"
	for _, format := range []string{"srt", "ass"} {
		separator := "\n"
		if format == "ass" {
			separator = `\N`
		}
		cue := Cue{ID: "lanes", Text: "  " + second, TranslationSource: "\t" + first, OriginalText: first, Translated: true, EndMs: 15000}
		report, err := SmartSplitCueForced(context.Background(), Document{Format: format, Cues: []Cue{cue}}, "padding", false, cue.ID, PunctuationOptions{})
		if err != nil || report.Changed != 1 {
			t.Fatal(report, err)
		}
		var source, translation string
		for _, c := range report.Document.Cues {
			for _, text := range []string{c.Text, c.TranslationSource} {
				for _, line := range strings.Split(text, separator) {
					if line != strings.TrimLeft(line, " \t\u00a0\u3000") {
						t.Fatal("indented output", c)
					}
				}
			}
			source += c.TranslationSource
			translation += c.Text
		}
		if withoutSpace(source) != withoutSpace(first) || withoutSpace(translation) != withoutSpace(second) {
			t.Fatal("changed bilingual payload")
		}
	}
}
