package subtitle

import (
	"context"
	"reflect"
	"strings"
	"testing"
)

func TestSegmentRecognitionPreservesWordsPunctuationAndClock(t *testing.T) {
	for _, text := range []string{
		"首先检查模型中的布线结构，然后调整边缘的位置，最后确认整个模型没有多余的面。",
		"We first inspect the complete structure of the model, then carefully adjust the edges and check the final result.",
	} {
		var words []Word
		pieces := strings.Fields(text)
		if len(pieces) == 1 {
			pieces = strings.Split(text, "")
		} else {
			for i := 1; i < len(pieces); i++ {
				pieces[i] = " " + pieces[i]
			}
		}
		for i, piece := range pieces {
			words = append(words, Word{piece, int64(i) * 350, int64(i+1) * 350})
		}
		cue := Cue{ID: "asr", Text: text, OriginalText: text, StartMs: 0, EndMs: int64(len(words)) * 350, Words: words}
		doc := Document{Format: "srt", Cues: []Cue{cue}}
		out, err := SegmentRecognition(context.Background(), doc)
		if err != nil || len(out.Cues) < 2 {
			t.Fatal(out, err)
		}
		joined := ""
		var recovered []Word
		for _, part := range out.Cues {
			joined += part.Text
			recovered = append(recovered, part.Words...)
			if part.TimingQuality != "aligned" || part.EndMs-part.StartMs < minimumSplitDuration {
				t.Fatal(part)
			}
		}
		if joined != text || !reflect.DeepEqual(recovered, words) {
			t.Fatal("text or word evidence changed", out)
		}
		if !reflect.DeepEqual(doc.Cues, []Cue{cue}) {
			t.Fatal("input changed")
		}
		assertSplitEnvelope(t, cue, out.Cues)
	}
}

func TestSegmentRecognitionKeepsReadableAndUnalignedCues(t *testing.T) {
	long := "We first inspect the complete structure of the model, then carefully adjust the edges and check the final result."
	cues := []Cue{
		{ID: "short", Text: "A complete short sentence.", StartMs: 0, EndMs: 3000, Words: []Word{{"A complete short sentence.", 0, 3000}}},
		{ID: "missing", Text: long, StartMs: 10000, EndMs: 20000},
		{ID: "mismatch", Text: long, StartMs: 30000, EndMs: 40000, Words: []Word{{"different", 30000, 34000}, {"text", 34000, 40000}}},
		{ID: "multiline", Text: long + "\n" + long, StartMs: 50000, EndMs: 70000, Words: []Word{{long, 50000, 59000}, {"\n" + long, 60000, 70000}}},
	}
	for i := range cues {
		cues[i].Position = i
	}
	out, err := SegmentRecognition(context.Background(), Document{Format: "srt", Cues: cues})
	if err != nil || !reflect.DeepEqual(out.Cues, cues) {
		t.Fatal(out, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := SegmentRecognition(ctx, Document{Cues: cues}); err == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestAttachOneAsrGroupsCollapsedAndOverlappingTokens(t *testing.T) {
	for _, words := range [][]Word{
		{{"I'", 0, 300}, {"m", 300, 300}, {" ready.", 500, 1000}},
		{{"I", 0, 0}, {" am", 100, 300}, {" ready.", 500, 1000}},
		{{"I", 0, 200}, {" am", 100, 300}, {" ready.", 500, 1000}},
	} {
		text := ""
		for _, word := range words {
			text += word.Text
		}
		raw, _ := MarshalOneAsrWords(words)
		out, warnings, err := AttachOneAsrWords(Document{Cues: []Cue{{Text: text, StartMs: 0, EndMs: 1100}}}, raw)
		if err != nil || len(warnings) != 0 || len(out.Cues[0].Words) != 2 {
			t.Fatal(out, warnings, err)
		}
		group := out.Cues[0].Words[0]
		if group.Text != words[0].Text+words[1].Text || group.StartMs != 0 || group.EndMs != 300 {
			t.Fatal("invented time or lost text", group)
		}
	}
	// Different collapsed timestamps must not manufacture a timed group.
	raw, _ := MarshalOneAsrWords([]Word{{"Unknown", 0, 0}, {" time", 500, 500}})
	out, warnings, err := AttachOneAsrWords(Document{Cues: []Cue{{Text: "Unknown time", StartMs: 0, EndMs: 1000}}}, raw)
	if err != nil || len(warnings) != 1 || len(out.Cues[0].Words) != 0 {
		t.Fatal(out, warnings, err)
	}
}
