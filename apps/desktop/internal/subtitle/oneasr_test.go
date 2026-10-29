package subtitle

import (
	"fmt"
	"math/rand/v2"
	"reflect"
	"strings"
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

// A recognition with zero-length cues, as OneAsr wrote for a 28-minute
// English video, keeps all its text in cues with valid times.
func TestNormalizeRecognition(t *testing.T) {
	doc := Document{Format: "srt", Cues: []Cue{
		{ID: "a", StartMs: 951000, EndMs: 956360, Text: "and then I'm gonna delete this one."},
		{ID: "b", StartMs: 966435, EndMs: 966435, Text: "The first thing"},
		{ID: "c", StartMs: 966435, EndMs: 1009555, Text: "that you need to do"},
		{ID: "d", StartMs: 1118890, EndMs: 1118890, Text: "Okay."},
		{ID: "e", StartMs: 1153055, EndMs: 1154975, Text: "Yeah,"},
		{ID: "f", StartMs: 1160000, EndMs: 1160000, Text: "好的"},
		{ID: "g", StartMs: 1160000, EndMs: 1160000, Text: "谢谢"},
	}}
	out, changed := NormalizeRecognition(doc)
	if changed != 4 {
		t.Fatal("changed", changed)
	}
	want := []struct {
		text       string
		start, end int64
	}{
		{"and then I'm gonna delete this one.", 951000, 956360},
		{"The first thing that you need to do", 966435, 1009555},
		{"Okay.", 1118890, 1119890},
		{"Yeah,", 1153055, 1154975},
		{"好的谢谢", 1160000, 1161000},
	}
	if len(out.Cues) != len(want) {
		t.Fatalf("%+v", out.Cues)
	}
	for i, w := range want {
		c := out.Cues[i]
		if c.Text != w.text || c.StartMs != w.start || c.EndMs != w.end || c.Position != i {
			t.Errorf("cue %d = %+v, want %+v", i, c, w)
		}
	}
	if doc.Cues[2].Text != "that you need to do" {
		t.Fatal("the input was changed")
	}
}

// Whatever times and texts a recognition has, the result has text in
// every cue, starts at 0 or later, ends after it starts, and keeps every
// word in order.
func TestNormalizeRecognitionAlwaysWritable(t *testing.T) {
	rng := rand.New(rand.NewPCG(1, 2))
	for run := 0; run < 2000; run++ {
		doc := Document{Format: "srt"}
		words := []string{}
		at := int64(rng.IntN(5000)) - 2000
		for i := rng.IntN(12); i >= 0; i-- {
			text := ""
			switch rng.IntN(5) {
			case 0:
				text = "  "
			default:
				text = fmt.Sprint("w", run, "-", i)
				words = append(words, text)
			}
			start := at
			end := start + int64(rng.IntN(4000)) - 1500
			if rng.IntN(3) == 0 {
				end = start
			}
			doc.Cues = append(doc.Cues, Cue{ID: fmt.Sprint(i), StartMs: start, EndMs: end, Text: text, OriginalText: text})
			at += int64(rng.IntN(3000)) - 500
		}
		out, _ := NormalizeRecognition(doc)
		got := []string{}
		for i, c := range out.Cues {
			if c.StartMs < 0 || c.EndMs <= c.StartMs || strings.TrimSpace(c.Text) == "" || c.Position != i {
				t.Fatalf("run %d: unwritable cue %+v from %+v", run, c, doc.Cues)
			}
			got = append(got, strings.Fields(c.Text)...)
		}
		if strings.Join(got, " ") != strings.Join(words, " ") {
			t.Fatalf("run %d: words %v, want %v", run, got, words)
		}
	}
}

// Negative starts, ends before starts and blank cues are repaired too.
func TestNormalizeRecognitionOtherFaults(t *testing.T) {
	out, changed := NormalizeRecognition(Document{Cues: []Cue{
		{StartMs: -40, EndMs: 900, Text: "first"},
		{StartMs: 1000, EndMs: 800, Text: "second"},
		{StartMs: 1200, EndMs: 1300, Text: " "},
		{StartMs: 3000, EndMs: 4000, Text: "third"},
	}})
	if changed != 3 || len(out.Cues) != 3 || out.Cues[0].StartMs != 0 || out.Cues[1].EndMs != 1200 || out.Cues[2].Text != "third" {
		t.Fatalf("%d %+v", changed, out.Cues)
	}
}
