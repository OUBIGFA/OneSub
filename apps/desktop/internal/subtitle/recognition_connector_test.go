package subtitle

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestRecognitionPrefersConnectorBeforeLengthBalance(t *testing.T) {
	for _, pair := range [][2]string{
		{"We can inspect these examples", "and you can download the complete collection from the project website."},
		{"We can inspect these examples", "or you can download the complete collection from the project website."},
		{"We can inspect these examples", "because the complete collection is available on the project website."},
		{"We can inspect these examples", "but you should check the download instructions on the project website."},
		{"We can inspect these examples", "and then download the complete collection from the project website."},
		{"We can inspect these examples", "and the project website provides the complete collection for download."},
	} {
		t.Run(strings.Fields(pair[1])[0]+"/"+pair[1], func(t *testing.T) {
			cue := punctuatedRecognitionCue(strings.Join(pair[:], " "))
			out, err := SegmentRecognition(context.Background(), Document{Format: "srt", Cues: []Cue{cue}})
			if err != nil || len(out.Cues) != 2 {
				t.Fatal(out, err)
			}
			for i, want := range pair {
				if strings.TrimSpace(out.Cues[i].Text) != want {
					t.Fatalf("connector must start the next subtitle, got %q / %q", out.Cues[0].Text, out.Cues[1].Text)
				}
			}
			assertSplitEnvelope(t, cue, out.Cues)
			var words []Word
			for _, part := range out.Cues {
				words = append(words, part.Words...)
			}
			if !reflect.DeepEqual(words, cue.Words) {
				t.Fatal("changed word clocks")
			}
		})
	}
}

func TestRecognitionConnectorPriorityPreservesShortSentencesAndPhrases(t *testing.T) {
	for _, text := range []string{
		"We inspect the models and then save the complete project.",
		"We inspect the models or download new examples from the website.",
		"We use these models because they are free.",
	} {
		cue := punctuatedRecognitionCue(text)
		out, err := SegmentRecognition(context.Background(), Document{Format: "srt", Cues: []Cue{cue}})
		if err != nil || len(out.Cues) != 1 || out.Cues[0].Text != text {
			t.Fatal(out, err)
		}
	}
	for _, phrase := range []string{"black and white", "black or white", "models and textures", "models or textures"} {
		cue := punctuatedRecognitionCue("We inspect all the " + phrase + " in this collection and you can download the complete collection from the project website.")
		out, err := SegmentRecognition(context.Background(), Document{Format: "srt", Cues: []Cue{cue}})
		if err != nil {
			t.Fatal(err)
		}
		found := false
		for _, part := range out.Cues {
			found = found || strings.Contains(part.Text, phrase)
		}
		if !found {
			t.Fatalf("split a coordinated phrase %q: %+v", phrase, out.Cues)
		}
		if len(out.Cues) != 2 || !strings.HasPrefix(strings.TrimSpace(out.Cues[1].Text), "and you") {
			t.Fatal("ignored later clause connector", out.Cues)
		}
	}
}

func TestRecognitionPunctuationRemainsAheadOfConnectors(t *testing.T) {
	parts := []string{"We inspect the models and save all the results,", "because the client needs the complete collection for the next project."}
	cue := punctuatedRecognitionCue(strings.Join(parts, " "))
	out, err := SegmentRecognition(context.Background(), Document{Format: "srt", Cues: []Cue{cue}})
	if err != nil || len(out.Cues) != 2 {
		t.Fatal(out, err)
	}
	for i, want := range parts {
		if strings.TrimSpace(out.Cues[i].Text) != want {
			t.Fatal(out.Cues)
		}
	}
}

func TestRecognitionKeepsSentenceEndAfterAlphanumericTerm(t *testing.T) {
	parts := []string{"You can inspect the model and see it in 3D.", "You can open the material panel to inspect all the available materials."}
	cue := punctuatedRecognitionCue(strings.Join(parts, " "))
	out, err := SegmentRecognition(context.Background(), Document{Format: "srt", Cues: []Cue{cue}})
	if err != nil || len(out.Cues) != 2 {
		t.Fatal(out, err)
	}
	for i, want := range parts {
		if strings.TrimSpace(out.Cues[i].Text) != want {
			t.Fatal("a term suffix hid the sentence period", out.Cues)
		}
	}
}

func TestRecognitionKeepsCoordinatedPhraseDuringLexicalFallback(t *testing.T) {
	for _, phrase := range []string{"black and white", "black or white", "models and textures", "models or textures"} {
		cue := punctuatedRecognitionCue("We inspect every available " + phrase + " carefully to make sure the complete collection will work correctly in the next project.")
		out, err := SegmentRecognition(context.Background(), Document{Format: "srt", Cues: []Cue{cue}})
		if err != nil || len(out.Cues) < 2 {
			t.Fatal(out, err)
		}
		found := false
		for _, part := range out.Cues {
			found = found || strings.Contains(part.Text, phrase)
		}
		if !found {
			t.Fatalf("ordinary fallback tore %q: %+v", phrase, out.Cues)
		}
		assertSplitEnvelope(t, cue, out.Cues)
	}
}

func TestRecognitionConnectorCannotStrandShortHeadOrTail(t *testing.T) {
	for _, text := range []string{
		"Try this and you can inspect the complete collection of reusable assets from the project website.",
		"We can inspect the complete collection of reusable assets from the project website and save.",
	} {
		cue := punctuatedRecognitionCue(text)
		out, err := SegmentRecognition(context.Background(), Document{Format: "srt", Cues: []Cue{cue}})
		if err != nil {
			t.Fatal(err)
		}
		for _, part := range out.Cues {
			if len(strings.Fields(part.Text)) < 3 || part.EndMs-part.StartMs < minimumSplitDuration {
				t.Fatal("created a short connector fragment", out.Cues)
			}
		}
		assertSplitEnvelope(t, cue, out.Cues)
	}
}
