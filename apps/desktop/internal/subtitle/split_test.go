package subtitle

import (
	"context"
	"reflect"
	"strings"
	"testing"
	"unicode"
)

func withoutSpace(s string) string {
	return strings.Map(func(r rune) rune {
		if unicode.IsSpace(r) {
			return -1
		}
		return r
	}, s)
}

func TestReadingLoadSeparatesLengthAndSpeed(t *testing.T) {
	for _, tt := range []struct {
		text       string
		duration   int64
		long, fast bool
	}{
		{"这是一条很短的字幕", 500, false, true},
		{strings.Repeat("文", 26), 10000, true, false},
		{strings.Repeat("a", 43), 10000, true, false},
		{"<i>你好</i>", 1000, false, false},
		{"你好（Hello）", 1000, false, false},
	} {
		got := InspectReading(tt.text, tt.duration)
		if got.TooLong != tt.long || got.TooFast != tt.fast {
			t.Fatalf("%q: %+v", tt.text, got)
		}
	}
	for _, text := range []string{"e\u0301", "👨‍👩‍👧‍👦"} {
		if got := InspectReading(text, 1000).Units; got != 1 {
			t.Fatalf("grapheme counted more than once: %q %v", text, got)
		}
	}
	if got := InspectReading("汉字한글한글", 1000).MaxRate; got != 12 {
		t.Fatal(got)
	}
	if got := InspectReading("日本語かな", 1000).MaxRate; got != 4 {
		t.Fatal(got)
	}
}

func TestSmartSplitPreservesContentEnvelopeAndNumbering(t *testing.T) {
	for _, text := range []string{
		"这是第一段完整的说明文字，我们接下来继续说明操作步骤，然后检查所有字幕的阅读体验。",
		"First we explain the complete workflow. Then we inspect every generated subtitle, preserving every word and number.",
		"Dr. Smith checks version 3.14 and https://example.com/a.b carefully, then explains the complete setup and all remaining steps.",
	} {
		cue := Cue{ID: "a", Text: text, OriginalText: text, StartMs: 1000, EndMs: 25000}
		doc := Document{Format: "srt", Cues: []Cue{cue, {ID: "b", Text: "Done", StartMs: 26000, EndMs: 27000}}}
		before := append([]Cue{}, doc.Cues...)
		r, err := SmartSplit(context.Background(), doc, "test", true, PunctuationOptions{})
		if err != nil || r.Changed != 1 || r.Added < 1 {
			t.Fatal(r, err)
		}
		if !reflect.DeepEqual(before, doc.Cues) {
			t.Fatal("mutated input")
		}
		var combined strings.Builder
		for i, part := range r.Document.Cues[:len(r.Document.Cues)-1] {
			combined.WriteString(part.Text)
			if part.Position != i || part.EndMs-part.StartMs < 1000 || part.TimingQuality != "estimated" || len(part.SourceCueIDs) != 0 {
				t.Fatal(part)
			}
			if i > 0 && part.StartMs != r.Document.Cues[i-1].EndMs {
				t.Fatal("new gap")
			}
		}
		if withoutSpace(combined.String()) != withoutSpace(text) {
			t.Fatal("text changed", combined.String())
		}
		if r.Document.Cues[0].StartMs != 1000 || r.Document.Cues[len(r.Document.Cues)-2].EndMs != 25000 {
			t.Fatal("envelope changed")
		}
		if r.Document.Cues[len(r.Document.Cues)-1].Position != len(r.Document.Cues)-1 {
			t.Fatal("tail not renumbered")
		}
	}
}

func TestSmartSplitCleansPunctuationAfterWordTimingAndKeepsSkippedCues(t *testing.T) {
	words := []Word{{"Version 3.14 is ready. ", 1000, 4000}, {"Are we done with this operation?", 5000, 8000}}
	text := words[0].Text + words[1].Text
	cue := Cue{ID: "aligned", Text: text, Words: words, StartMs: 1000, EndMs: 9000}
	skipped := Cue{ID: "short", Position: 1, Text: "This sentence has too little time. It must remain exactly unchanged.", StartMs: 10000, EndMs: 11000}
	doc := Document{Format: "srt", Cues: []Cue{cue, skipped}}
	report, err := SmartSplit(context.Background(), doc, "clean", true, PunctuationOptions{StripTrailing: true, ReplacePeriods: true, Keep: "?"})
	if err != nil || report.Changed != 1 || report.Estimated != 0 || len(report.Document.Cues) != 3 {
		t.Fatal(report, err)
	}
	first, second := report.Document.Cues[0], report.Document.Cues[1]
	if first.Text != "Version 3.14 is ready" || second.Text != "Are we done with this operation?" {
		t.Fatal("cleanup order or kept marks changed", first.Text, second.Text)
	}
	if first.OriginalText != words[0].Text || first.StartMs != 1000 || first.EndMs != 5000 || second.StartMs != 5000 || second.EndMs != 9000 || first.TimingQuality != "aligned" || second.TimingQuality != "aligned" {
		t.Fatal("cleanup replaced the original evidence or word timing", first, second)
	}
	last := report.Document.Cues[2]
	last.Position = skipped.Position
	if !reflect.DeepEqual(last, skipped) || doc.Cues[0].Text != text || doc.Cues[0].Words[0].Text != words[0].Text {
		t.Fatal("cleanup touched skipped cues or mutated the input")
	}
}

func TestSmartSplitUsesWordEvidenceAndRejectsStaleWords(t *testing.T) {
	words := []Word{{"First sentence has enough words. ", 1000, 5000}, {"Second sentence is also quite long. ", 7000, 11000}, {"Finally this sentence finishes the explanation.", 13000, 19000}}
	text := ""
	for _, w := range words {
		text += w.Text
	}
	cue := Cue{ID: "a", Text: text, Words: words, StartMs: 1000, EndMs: 20000}
	for _, stale := range []bool{false, true} {
		if stale {
			cue.Text = "Changed " + text
		}
		r, err := SmartSplit(context.Background(), Document{Format: "srt", Cues: []Cue{cue}}, "words", true, PunctuationOptions{})
		if err != nil || r.Changed != 1 {
			t.Fatal(r, err)
		}
		for _, part := range r.Document.Cues {
			if stale {
				if part.TimingQuality != "estimated" || len(part.Words) != 0 {
					t.Fatal(part)
				}
				continue
			}
			if part.TimingQuality != "aligned" || len(part.Words) == 0 {
				t.Fatal(part)
			}
			if part.StartMs != 1000 && part.StartMs != 7000 && part.StartMs != 13000 {
				t.Fatal("invented word cut", part)
			}
		}
	}
}

func TestSmartSplitKeepsUnfixableCuesAndFastCuesRed(t *testing.T) {
	for _, cue := range []Cue{
		{ID: "short", Text: strings.Repeat("word ", 30), StartMs: 0, EndMs: 1000},
		{ID: "url", Text: "https://example.com/" + strings.Repeat("a", 80), StartMs: 0, EndMs: 10000},
		{ID: "tags", Text: "<i>" + strings.Repeat("word ", 30) + "</i>", StartMs: 0, EndMs: 20000},
	} {
		r, err := SmartSplit(context.Background(), Document{Format: "srt", Cues: []Cue{cue}}, "keep", true, PunctuationOptions{})
		if err != nil || r.Changed != 0 || len(r.Skipped) != 1 || r.Remaining != 1 {
			t.Fatal(r, err)
		}
	}
	cue := Cue{ID: "fast", Text: strings.Repeat("内容", 30), StartMs: 0, EndMs: 3000}
	r, _ := SmartSplit(context.Background(), Document{Format: "srt", Cues: []Cue{cue}}, "fast", true, PunctuationOptions{})
	if r.Changed != 1 || r.Remaining == 0 {
		t.Fatal("split pretended to cure speed", r)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := SmartSplit(ctx, Document{Cues: []Cue{cue}}, "cancel", true, PunctuationOptions{}); err == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestSmartSplitPairsBilingualAndPreservesMultilineLanes(t *testing.T) {
	source := "This first explanation describes the complete workflow. This second explanation describes the next operation."
	translation := "首先完整地说明当前工作流程，然后接着仔细说明下一步操作的各项注意事项。"
	for _, multiline := range []bool{false, true} {
		cue := Cue{ID: "a", Text: translation, TranslationSource: source, OriginalText: source, Translated: true, StartMs: 0, EndMs: 24000}
		if multiline {
			cue.Text = source + "\n" + translation
			cue.Translated = false
		}
		r, err := SmartSplit(context.Background(), Document{Format: "srt", Cues: []Cue{cue}}, "pair", false, PunctuationOptions{})
		if err != nil || r.Changed != 1 {
			t.Fatal(r, err)
		}
		src, tr := "", ""
		for _, part := range r.Document.Cues {
			if !part.SplitReview {
				t.Fatal("pairing was not marked for review")
			}
			if multiline {
				lines := strings.Split(part.Text, "\n")
				if len(lines) != 2 {
					t.Fatal(part)
				}
				src += lines[0]
				tr += lines[1]
			} else {
				src += SourceText(part)
				tr += part.Text
				if !part.Translated || !part.TextLocked {
					t.Fatal(part)
				}
			}
		}
		if withoutSpace(src) != withoutSpace(source) || withoutSpace(tr) != withoutSpace(translation) {
			t.Fatal("bilingual content changed", src, tr)
		}
	}
}

func TestSmartSplitResolvesLengthInOneBatchAndAvoidsShortPieces(t *testing.T) {
	// At 84 characters, a preferred sentence boundary at 43 would leave a
	// still-long first cue. A single batch must look for a finer safe split.
	text := strings.Repeat("word ", 8) + "ab. " + strings.Repeat("word ", 7) + "abcd."
	for _, duration := range []int64{16000, 2400} {
		cue := Cue{ID: "balanced", Text: text, StartMs: 0, EndMs: duration}
		report, err := SmartSplit(context.Background(), Document{Format: "srt", Cues: []Cue{cue}}, "balanced", true, PunctuationOptions{})
		if err != nil || report.Changed != 1 {
			t.Fatal("available time was rejected", report, err)
		}
		for _, piece := range report.Document.Cues {
			if piece.EndMs-piece.StartMs < 1000 {
				t.Fatal("piece flashes by", piece)
			}
			if duration == 16000 && InspectCueReading(piece).Long() {
				t.Fatal("one batch left a solvable long cue", piece)
			}
		}
	}
	// Imported bilingual lines are simultaneous. Their combined visible load
	// still has to fit after each lane has been partitioned.
	cue := Cue{ID: "multi", Text: strings.Repeat("word ", 16) + "\n" + strings.Repeat("内容", 24), StartMs: 0, EndMs: 24000}
	report, err := SmartSplit(context.Background(), Document{Format: "srt", Cues: []Cue{cue}}, "multi", true, PunctuationOptions{})
	if err != nil || report.Changed != 1 {
		t.Fatal(report, err)
	}
	for _, piece := range report.Document.Cues {
		if InspectCueReading(piece).Long() {
			t.Fatal("combined multiline load was not checked", piece)
		}
	}
}

func TestSmartSplitSingleCueAlsoHandlesReadableCues(t *testing.T) {
	for _, text := range []string{"Hello wonderful world today.", "早上好大家好晚上好各位好"} {
		cue := Cue{ID: "short", Text: text, StartMs: 0, EndMs: 4000}
		long := Cue{ID: "long", Position: 1, Text: strings.Repeat("word ", 30), StartMs: 5000, EndMs: 25000}
		if InspectCueReading(cue).Problem() {
			t.Fatal("fixture is not readable", text)
		}
		doc := Document{Format: "srt", Cues: []Cue{cue, long}}
		report, err := SmartSplitCue(context.Background(), doc, "single", true, "short", PunctuationOptions{})
		if err != nil || report.Changed != 1 || len(report.Document.Cues) != 3 {
			t.Fatal(report, err)
		}
		last := report.Document.Cues[2]
		last.Position = long.Position
		if !reflect.DeepEqual(last, long) {
			t.Fatal("single-cue command changed another long cue", last)
		}
		if withoutSpace(report.Document.Cues[0].Text+report.Document.Cues[1].Text) != withoutSpace(text) {
			t.Fatal("single split changed text")
		}
		if report.Document.Cues[0].StartMs != 0 || report.Document.Cues[1].EndMs != 4000 {
			t.Fatal("single split changed envelope")
		}
		if _, err := SmartSplitCue(context.Background(), doc, "missing", true, "missing", PunctuationOptions{}); err == nil {
			t.Fatal("missing cue silently selected another target")
		}
	}
}

func TestSplitDoesNotBreakGraphemesNumbersOrParentheses(t *testing.T) {
	text := "Check 👨‍👩‍👧‍👦 and e\u0301 carefully (including all these words), then keep 3.14159, 12,500 and Ctrl+Shift+S intact in the remaining text."
	s := newSplitText(text)
	parts, _ := s.divide(3, nil)
	for _, part := range parts {
		if strings.Contains(part, "(") != strings.Contains(part, ")") {
			t.Fatal("split parentheses", parts)
		}
	}
	joined := strings.Join(parts, "|")
	for _, token := range []string{"👨‍👩‍👧‍👦", "e\u0301", "3.14159", "12,500", "Ctrl+Shift+S"} {
		if !strings.Contains(joined, token) {
			t.Fatal("broken token", token, joined)
		}
	}
}
