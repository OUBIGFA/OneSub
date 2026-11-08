package subtitle

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// Synthetic speech clocks exercise segmentation without importing private media.
func punctuatedRecognitionCue(text string) Cue {
	pieces := strings.Fields(text)
	if len(pieces) == 1 {
		pieces = strings.Split(text, "")
	} else {
		for i := 1; i < len(pieces); i++ {
			pieces[i] = " " + pieces[i]
		}
	}
	cue := Cue{ID: "speech", Text: text, OriginalText: text, EndMs: int64(len(pieces)) * 350}
	for i, piece := range pieces {
		cue.Words = append(cue.Words, Word{piece, int64(i) * 350, int64(i+1) * 350})
	}
	return cue
}

func TestRecognitionKeepsWholeSentencesBeforeBalancingLength(t *testing.T) {
	for _, sentences := range [][]string{
		{"Welcome back, everyone, to our weekly workshop.", "Today we inspect the collection of reusable assets.", "First of all, we will look at the downloadable examples."},
		{"我们先检查这个模型的整体结构。", "接下来调整所有边缘的位置。", "最后再确认模型没有多余的面。"},
	} {
		text := strings.Join(sentences, " ")
		cue := punctuatedRecognitionCue(text)
		if strings.Contains(text, "我们") {
			cue = punctuatedRecognitionCue(strings.Join(sentences, ""))
		}
		out, err := SegmentRecognition(context.Background(), Document{Format: "srt", Cues: []Cue{cue}})
		if err != nil {
			t.Fatal(err)
		}
		if len(out.Cues) != len(sentences) {
			t.Fatalf("wanted complete sentences, got %+v", out.Cues)
		}
		for i, want := range sentences {
			if strings.TrimSpace(out.Cues[i].Text) != want {
				t.Fatalf("sentence %d split or joined to next: %q", i, out.Cues[i].Text)
			}
		}
		assertSplitEnvelope(t, cue, out.Cues)
	}
}

func TestRecognitionNeverMixesSentenceTailWithNextOpening(t *testing.T) {
	sentences := []string{
		"Hello everyone, and welcome back to our series H.",
		"Today we're exploring a few fantastic places where you can download useful object files that you can work with.",
		"First off, we're going to visit the team at Studio Four, as they've prepared a sample library of several thousand items, which are freely available and built from reusable code.",
		"And for those who haven't heard about it Studio Four is the work of an independent team.",
	}
	cue := punctuatedRecognitionCue(strings.Join(sentences, " "))
	for i := range cue.Words {
		cue.Words[i].StartMs = int64(i) * 160
		cue.Words[i].EndMs = int64(i+1) * 160
	}
	cue.EndMs = int64(len(cue.Words)) * 160
	out, err := SegmentRecognition(context.Background(), Document{Format: "srt", Cues: []Cue{cue}})
	if err != nil {
		t.Fatal(err)
	}
	if strings.TrimSpace(out.Cues[0].Text) != sentences[0] {
		t.Fatalf("broke the complete opening sentence: %q", out.Cues[0].Text)
	}
	for _, part := range out.Cues {
		if strings.Contains(strings.TrimSuffix(strings.TrimSpace(part.Text), "."), ".") {
			t.Fatalf("mixed sentence tail and following opening: %q", part.Text)
		}
	}
}

func TestRecognitionLongSentenceUsesCommasBeforeLexicalCuts(t *testing.T) {
	clauses := []string{"Before you begin, carefully inspect every model,", "then adjust the edges using the existing tools,", "and save the result in the project folder."}
	cue := punctuatedRecognitionCue(strings.Join(clauses, " "))
	out, err := SegmentRecognition(context.Background(), Document{Format: "srt", Cues: []Cue{cue}})
	if err != nil {
		t.Fatal(err)
	}
	if len(out.Cues) != len(clauses) {
		t.Fatal(out.Cues)
	}
	for i, want := range clauses {
		if strings.TrimSpace(out.Cues[i].Text) != want {
			t.Fatalf("ignored comma: %q", out.Cues[i].Text)
		}
	}
}

func TestRecognitionPunctuationPreservesLiteralsAndShortIntroductions(t *testing.T) {
	sentences := []string{"We use version 3.14 from example.com with Dr. Smith.", "First off, we check the complete collection of models.", "Next we open the example file and save our changes."}
	cue := punctuatedRecognitionCue(strings.Join(sentences, " "))
	out, err := SegmentRecognition(context.Background(), Document{Format: "srt", Cues: []Cue{cue}})
	if err != nil || len(out.Cues) != len(sentences) {
		t.Fatal(out, err)
	}
	for i, want := range sentences {
		if strings.TrimSpace(out.Cues[i].Text) != want {
			t.Fatal(out.Cues)
		}
	}
}

func TestRecognitionAllowsTwoLineSentenceBeforeSplitting(t *testing.T) {
	text := "The reusable assets in this collection are ready for your next personal project."
	for _, text := range []string{text, "    " + text + "    "} {
		cue := punctuatedRecognitionCue(text)
		// Preserve the padded ASR text in its exact word evidence too.
		cue.Words[0].Text = strings.Repeat(" ", len(text)-len(strings.TrimLeft(text, " "))) + cue.Words[0].Text
		cue.Words[len(cue.Words)-1].Text += strings.Repeat(" ", len(text)-len(strings.TrimRight(text, " ")))
		out, err := SegmentRecognition(context.Background(), Document{Format: "srt", Cues: []Cue{cue}})
		if err != nil || len(out.Cues) != 1 || out.Cues[0].Text != text {
			t.Fatal("split readable complete sentence", out, err)
		}
	}
}

func TestRecognitionHonorsShortCompleteSentence(t *testing.T) {
	cue := punctuatedRecognitionCue("So this is it. We can now inspect the complete collection of downloadable models.")
	out, err := SegmentRecognition(context.Background(), Document{Format: "srt", Cues: []Cue{cue}})
	if err != nil || len(out.Cues) != 2 || strings.TrimSpace(out.Cues[0].Text) != "So this is it." {
		t.Fatal("treated a short complete sentence as a fragment", out, err)
	}
}

func TestRecognitionJoinsOnlyUnfinishedAdjacentSpeech(t *testing.T) {
	for _, gap := range []int64{100, 535, 750, 751, 2000} {
		first := punctuatedRecognitionCue("We can use all these models")
		second := punctuatedRecognitionCue("in our next project. Next we inspect the materials.")
		second.ID = "next"
		offset := first.EndMs + gap
		second.StartMs += offset
		second.EndMs += offset
		for i := range second.Words {
			second.Words[i].StartMs += offset
			second.Words[i].EndMs += offset
		}
		inputWords := append(slices.Clone(first.Words), second.Words...)
		out, err := SegmentRecognition(context.Background(), Document{Format: "srt", Cues: []Cue{first, second}})
		if err != nil {
			t.Fatal(err)
		}
		want := "We can use all these models in our next project."
		if gap <= 750 && strings.TrimSpace(out.Cues[0].Text) != want {
			t.Fatal("kept the audio chunk boundary inside a sentence", out.Cues)
		}
		if gap > 750 && out.Cues[0].EndMs != first.EndMs {
			t.Fatal("joined across silence", out.Cues)
		}
		joined := ""
		for _, c := range out.Cues {
			joined += c.Text
		}
		if strings.Join(strings.Fields(joined), "") != strings.Join(strings.Fields(first.Text+second.Text), "") {
			t.Fatal("changed words")
		}
		var recovered []Word
		for _, cue := range out.Cues {
			recovered = append(recovered, cue.Words...)
		}
		if len(recovered) != len(inputWords) {
			t.Fatal("lost word evidence")
		}
		for i, w := range recovered {
			if w.StartMs != inputWords[i].StartMs || w.EndMs != inputWords[i].EndMs || strings.TrimSpace(w.Text) != strings.TrimSpace(inputWords[i].Text) {
				t.Fatal("changed a word or its clock")
			}
		}
		if !reflect.DeepEqual(append(slices.Clone(first.Words), second.Words...), inputWords) {
			t.Fatal("mutated input word evidence")
		}
	}
}

func TestRecognitionDoesNotJoinProtectedOrUntimedSpeech(t *testing.T) {
	for _, kind := range []string{"complete", "readOnly", "translated", "missing", "mismatch", "overlap", "wordPause"} {
		t.Run(kind, func(t *testing.T) {
			first := punctuatedRecognitionCue("We can use all these models")
			second := punctuatedRecognitionCue("in our next project.")
			second.ID = "next"
			offset := first.EndMs + 100
			second.StartMs += offset
			second.EndMs += offset
			for i := range second.Words {
				second.Words[i].StartMs += offset
				second.Words[i].EndMs += offset
			}
			switch kind {
			case "complete":
				first.Text += "."
				first.OriginalText = first.Text
				first.Words[len(first.Words)-1].Text += "."
			case "readOnly":
				second.ReadOnly = true
			case "translated":
				second.Translated = true
			case "missing":
				second.Words = nil
			case "mismatch":
				second.Words[0].Text = "different"
			case "overlap":
				second.StartMs = first.EndMs - 100
			case "wordPause":
				first.EndMs += 1000
				second.StartMs += 1000
				second.EndMs += 1000
				for i := range second.Words {
					second.Words[i].StartMs += 1000
					second.Words[i].EndMs += 1000
				}
			}
			out, err := SegmentRecognition(context.Background(), Document{Format: "srt", Cues: []Cue{first, second}})
			if err != nil || len(out.Cues) != 2 || out.Cues[0].Text != first.Text || out.Cues[0].EndMs != first.EndMs {
				t.Fatal("joined across protected boundary", out, err)
			}
		})
	}
}
