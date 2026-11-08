package subtitle

import (
	"context"
	"strings"
	"testing"
	"unicode"
)

func assertWordClockAfterPadding(t *testing.T, got, want []Word) {
	t.Helper()
	if len(got) != len(want) {
		t.Fatal("changed word count")
	}
	for i, w := range got {
		if w.StartMs != want[i].StartMs || w.EndMs != want[i].EndMs || w.Text != want[i].Text && w.Text != strings.TrimLeftFunc(want[i].Text, unicode.IsSpace) {
			t.Fatal("changed word or clock", w, want[i])
		}
	}
}

func TestRecognitionSoIntroducesFollowingClause(t *testing.T) {
	for _, pair := range [][2]string{
		{"It helps us understand a little more about the", "so because the environment light uses the image as its source,"},
		{"Perhaps this looks a bit too bright", "so we can lower the gamma setting a little bit before saving."},
		{"We need to inspect the complete material carefully", "so the artist can adjust the final appearance of the model."},
	} {
		cue := punctuatedRecognitionCue(strings.Join(pair[:], " "))
		out, err := SegmentRecognition(context.Background(), Document{Format: "srt", Cues: []Cue{cue}})
		if err != nil || len(out.Cues) != 2 {
			t.Fatal(out, err)
		}
		for i, want := range pair {
			if strings.TrimSpace(out.Cues[i].Text) != want {
				t.Fatalf("wrong so boundary: %q / %q", out.Cues[0].Text, out.Cues[1].Text)
			}
		}
	}
}

func TestRecognitionSoRecoversDegreePhraseAcrossChunks(t *testing.T) {
	first := punctuatedRecognitionCue("Perhaps this looks a bit")
	second := punctuatedRecognitionCue("too bright so we can lower the gamma setting a little bit before saving")
	second.ID = "next"
	offset := first.EndMs + 435
	second.StartMs += offset
	second.EndMs += offset
	for i := range second.Words {
		second.Words[i].StartMs += offset
		second.Words[i].EndMs += offset
	}
	out, err := SegmentRecognition(context.Background(), Document{Format: "srt", Cues: []Cue{first, second}})
	if err != nil || len(out.Cues) != 2 {
		t.Fatal(out, err)
	}
	if strings.TrimSpace(out.Cues[0].Text) != "Perhaps this looks a bit too bright" || strings.TrimSpace(out.Cues[1].Text) != "so we can lower the gamma setting a little bit before saving" {
		t.Fatal(out.Cues)
	}
}

func TestRecognitionLongSentencePreservesEachSafeClause(t *testing.T) {
	parts := []string{"Perhaps this looks a bit too bright", "so we can lower the gamma setting a little bit", "and then we can inspect the final materials in the completed project."}
	cue := punctuatedRecognitionCue(strings.Join(parts, " "))
	out, err := SegmentRecognition(context.Background(), Document{Format: "srt", Cues: []Cue{cue}})
	if err != nil || len(out.Cues) != len(parts) {
		t.Fatal(out, err)
	}
	for i, want := range parts {
		if out.Cues[i].Text != want {
			t.Fatal("length balancing swallowed a clause boundary", out.Cues)
		}
	}
}

func TestRecognitionKeepsDegreeSoWithAdjective(t *testing.T) {
	for _, phrase := range []string{"so bright", "so much brighter", "a bit too bright", "a little bit too bright"} {
		cue := punctuatedRecognitionCue("The preview is " + phrase + " for the current scene and we should adjust the settings before saving the complete project.")
		out, err := SegmentRecognition(context.Background(), Document{Format: "srt", Cues: []Cue{cue}})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, c := range out.Cues {
			found = found || strings.Contains(c.Text, phrase)
		}
		if !found {
			t.Fatalf("broke degree phrase %q: %+v", phrase, out.Cues)
		}
	}
}

func TestRecognitionAndManualSplitPublishWithoutLeadingWhitespace(t *testing.T) {
	text := "We first inspect the complete structure of the model, then carefully adjust the edges and check the final result."
	for _, prefix := range []string{"", " \t\u00a0\u3000"} {
		cue := punctuatedRecognitionCue(text)
		cue.Text = prefix + cue.Text
		cue.OriginalText = cue.Text
		cue.Words[0].Text = prefix + cue.Words[0].Text
		input := Document{Format: "srt", Cues: []Cue{cue}}
		out, err := SegmentRecognition(context.Background(), input)
		if err != nil {
			t.Fatal(err)
		}
		report, err := SmartSplit(context.Background(), input, "spaces", true, PunctuationOptions{})
		if err != nil {
			t.Fatal(err)
		}
		for _, doc := range []Document{out, report.Document} {
			if len(doc.Cues) < 2 {
				t.Fatal("did not split")
			}
			for _, part := range doc.Cues {
				if part.Text != strings.TrimLeftFunc(part.Text, unicode.IsSpace) {
					t.Fatalf("leading whitespace: %q", part.Text)
				}
				if times, _ := wordBoundaries(part, part.Text); times == nil {
					t.Fatalf("trim invalidated word evidence: %+v", part)
				}
			}
		}
		if input.Cues[0].Text != prefix+text || input.Cues[0].Words[0].Text != prefix+"We" {
			t.Fatal("mutated input")
		}
	}
}
