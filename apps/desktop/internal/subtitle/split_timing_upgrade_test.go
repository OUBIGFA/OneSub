package subtitle

import (
	"context"
	"slices"
	"strings"
	"testing"
)

func TestSplitCalloutExemptionRequiresRealSilenceOnBothSides(t *testing.T) {
	for _, measured := range []bool{false, true} {
		words := []Word{
			{"This complete narration explains all the initial steps. ", 500, 5500},
			{"Stop! ", 6200, 7500},
			{"The following narration explains every remaining operation.", 8200, 20500},
		}
		if !measured {
			words[0].EndMs, words[1].EndMs = 6000, 8000
		}
		text := words[0].Text + words[1].Text + words[2].Text
		cue := Cue{ID: "callout", Text: text, Words: words, StartMs: 0, EndMs: 22000}
		report, err := SmartSplitCue(context.Background(), Document{Format: "srt", Cues: []Cue{cue}}, "callout", true, cue.ID, PunctuationOptions{})
		if err != nil || report.Changed != 1 {
			t.Fatal(report, err)
		}
		assertSplitEnvelope(t, cue, report.Document.Cues)
		callout := slices.ContainsFunc(report.Document.Cues, func(c Cue) bool { return strings.TrimSpace(c.Text) == "Stop!" })
		if callout != measured {
			t.Fatal("callout exemption did not require two measured gaps", measured, report.Document.Cues)
		}
		if measured {
			for _, part := range report.Document.Cues {
				if strings.TrimSpace(part.Text) == "Stop!" && (part.StartMs != 6200 || part.EndMs != 8200 || part.TimingQuality != "aligned") {
					t.Fatal("moved the callout away from word clocks", part)
				}
			}
		}
	}
}

// Min-duration repair is allowed only for estimates. A valid five-character
// opening phrase must not be rejected solely because its raw ratio is 700 ms.
func TestSplitEstimatedTimeRedistributesWithoutChangingAlignedTimes(t *testing.T) {
	text := "大家请翻开“深入理解计算机操作系统”第四章查看流水线设计的详细架构"
	cue := Cue{ID: "estimated", Text: text, StartMs: 0, EndMs: 4200}
	report, err := SmartSplitCue(context.Background(), Document{Format: "srt", Cues: []Cue{cue}}, "redistribute", true, cue.ID, PunctuationOptions{})
	if err != nil || len(report.Document.Cues) != 3 || report.Document.Cues[0].Text != "大家请翻开" || report.Document.Cues[0].EndMs < minimumSplitDuration {
		t.Fatal("rejected a short valid phrase instead of redistributing estimates", report, err)
	}
	assertSplitEnvelope(t, cue, report.Document.Cues)
	// With exact phrase clocks the 700 ms prefix cannot be enlarged. Keep it
	// with the next phrase rather than fabricate an aligned 833 ms boundary.
	cue.ID = "aligned"
	cue.Words = []Word{{"大家请翻开", 0, 600}, {"“深入理解计算机操作系统”第四章", 700, 2200}, {"查看流水线设计的详细架构", 2400, 4000}}
	report, err = SmartSplitCue(context.Background(), Document{Format: "srt", Cues: []Cue{cue}}, "aligned", true, cue.ID, PunctuationOptions{})
	if err != nil || report.Changed != 1 || len(report.Document.Cues) != 2 || report.Estimated != 0 || report.Document.Cues[1].StartMs != 2400 {
		t.Fatal("redistributed genuine word times", report, err)
	}
	assertSplitEnvelope(t, cue, report.Document.Cues)
}
