package subtitle

import (
	"context"
	"slices"
	"strings"
	"sync"
	"testing"
)

// These hand-reviewed examples exercise the public editing operation. They
// distinguish a complete phrase from cuts that merely balance printed width.
func TestSmartSplitSemanticPhrases(t *testing.T) {
	for _, tc := range []struct {
		name    string
		text    string
		phrases []string
	}{
		{"english-noun-phrase", "The new artificial intelligence system can process every incoming request, and the engineering team will review all the results tomorrow.", []string{"artificial intelligence system", "engineering team", "will review"}},
		{"english-negation", "We should not have ignored the early warning signs, because the entire production system was already running out of available memory.", []string{"should not have ignored", "early warning signs", "entire production system", "was already running out of"}},
		{"english-relative", "The engineer who designed the new control system explained the problem, and everyone agreed to check the remaining safety requirements.", []string{"new control system", "remaining safety requirements"}},
		{"english-unpunctuated", "we need to review the complete financial report before we can make a final decision about the proposed construction project", []string{"complete financial report", "can make", "final decision", "proposed construction project"}},
		{"chinese-phrase", "今天我们将讨论人工智能技术在医疗领域的实际应用，然后分析这些系统在真实环境中面临的主要挑战。", []string{"医疗领域", "实际应用", "真实环境中", "主要挑战", "然后分析这些系统"}},
		{"chinese-unpunctuated", "这项研究为理解人类大脑的工作机制提供了重要线索也为未来的临床治疗开辟了新的方向", []string{"工作机制", "重要线索", "临床治疗", "新的方向"}},
		{"traditional-phrase", "這項研究為理解人類大腦的工作機制提供了重要線索，也為未來的臨床治療開闢了新的方向。", []string{"工作機制", "重要線索", "臨床治療", "新的方向"}},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cue := Cue{ID: tc.name, Text: tc.text, StartMs: 0, EndMs: 18000}
			report, err := SmartSplitCue(context.Background(), Document{Format: "srt", Cues: []Cue{cue}}, "semantic", true, cue.ID, PunctuationOptions{})
			if err != nil || report.Changed != 1 {
				t.Fatalf("split failed: %#v %v", report, err)
			}
			assertSplitEnvelope(t, cue, report.Document.Cues)
			var parts []string
			for _, part := range report.Document.Cues {
				parts = append(parts, part.Text)
			}
			joined := strings.Join(parts, "|")
			for _, phrase := range tc.phrases {
				if !strings.Contains(joined, phrase) {
					t.Errorf("broke phrase %q: %s", phrase, joined)
				}
			}
			// A feasible full clause must end a subtitle, rather than leaving
			// the start of the next clause stranded on the previous screen.
			if tc.name == "english-noun-phrase" || tc.name == "chinese-phrase" {
				separator := ","
				if tc.name == "chinese-phrase" {
					separator = "，"
				}
				for _, part := range parts {
					if strings.Contains(strings.TrimSpace(part), separator) && !strings.HasSuffix(strings.TrimSpace(part), separator) {
						t.Errorf("carried next clause across its boundary: %s", joined)
					}
				}
			}
			t.Log(joined)
		})
	}
}

func TestSmartSplitEnglishVariantsKeepWordsAndSharedTiming(t *testing.T) {
	for _, phrase := range []string{"the complete financial report", "the\tcomplete\tfinancial\treport", "the  complete  financial  report", "the complete financial reports"} {
		text := "We carefully reviewed " + phrase + ", and the directors approved the new construction project."
		cue := Cue{ID: "variants", Text: text, StartMs: 0, EndMs: 12000}
		report, err := SmartSplitCue(context.Background(), Document{Format: "srt", Cues: []Cue{cue}}, "variants", true, cue.ID, PunctuationOptions{})
		if err != nil || report.Changed != 1 {
			t.Fatalf("split failed: %v %v", report, err)
		}
		assertSplitEnvelope(t, cue, report.Document.Cues)
		var parts []string
		for _, part := range report.Document.Cues {
			parts = append(parts, part.Text)
		}
		if !strings.Contains(strings.Join(parts, "|"), phrase) {
			t.Errorf("split a noun phrase across whitespace: %q", parts)
		}
	}
	first, second := "We shouldn't have ignored the original warning signs, ", "because the complete system was already failing."
	cue := Cue{ID: "aligned-pos", Text: first + second, Words: []Word{{first, 1000, 3900}, {second, 4500, 7900}}, StartMs: 1000, EndMs: 8500}
	report, err := SmartSplitCue(context.Background(), Document{Format: "srt", Cues: []Cue{cue}}, "aligned-pos", true, cue.ID, PunctuationOptions{})
	if err != nil || report.Changed != 1 || len(report.Document.Cues) != 2 || report.Estimated != 0 || report.Document.Cues[1].StartMs != 4500 {
		t.Fatalf("changed real word timing: %#v %v", report, err)
	}
	assertSplitEnvelope(t, cue, report.Document.Cues)
}

func TestEnglishPhraseRoutingDoesNotTreatSharedLatinWordsAsEnglish(t *testing.T) {
	for _, text := range []string{
		"La casa de la familia está cerca del centro de la ciudad y todos pueden caminar hasta el parque.",
		"Nous avons un rapport de recherche et une liste de tous les résultats à vérifier.",
		"Мы проверяем все результаты, а затем сохраняем окончательный вариант документа.",
		"我们检查 the complete report 然后保存所有结果。",
	} {
		if englishSplitText(text) {
			t.Errorf("applied English model to %q", text)
		}
	}
}

func TestSmartSplitModelsAreDeterministicAcrossConcurrentCalls(t *testing.T) {
	text := "We should not have ignored the early warning signs, because the entire production system was already running out of available memory."
	cue := Cue{ID: "concurrent", Text: text, StartMs: 0, EndMs: 18000}
	doc := Document{Format: "srt", Cues: []Cue{cue}}
	want, err := SmartSplit(context.Background(), doc, "parallel", true, PunctuationOptions{})
	if err != nil || want.Changed != 1 {
		t.Fatal(want, err)
	}
	var wg sync.WaitGroup
	for range 12 {
		wg.Add(1)
		go func() {
			defer wg.Done()
			got, err := SmartSplit(context.Background(), doc, "parallel", true, PunctuationOptions{})
			if err != nil || len(got.Document.Cues) != len(want.Document.Cues) {
				t.Errorf("concurrent split failed: %v", err)
				return
			}
			for i, part := range got.Document.Cues {
				if part.Text != want.Document.Cues[i].Text || part.StartMs != want.Document.Cues[i].StartMs {
					t.Errorf("non-deterministic split at %d", i)
				}
			}
		}()
	}
	wg.Wait()
}

func BenchmarkSmartSplitEnglishPhrases(b *testing.B) {
	text := "The new artificial intelligence system can process every incoming request, and the engineering team will review all the results tomorrow."
	doc := Document{Format: "srt", Cues: []Cue{{ID: "benchmark", Text: text, StartMs: 0, EndMs: 18000}}}
	SmartSplit(context.Background(), doc, "warm", true, PunctuationOptions{})
	b.ReportAllocs()
	b.ResetTimer()
	for range b.N {
		report, err := SmartSplit(context.Background(), doc, "bench", true, PunctuationOptions{})
		if err != nil || report.Changed != 1 {
			b.Fatal(report, err)
		}
	}
}

func TestSmartSplitKeepsUnevenCompleteClauses(t *testing.T) {
	for _, want := range [][]string{
		{"请先仔细检查所有设备的连接状态，", "然后启动系统。"},
		{"The entire engineering team reviewed the final report, ", "and we approved the plan."},
		{"The new system is ready. ", "We can start the final checks."},
	} {
		cue := Cue{ID: "clauses", Text: strings.Join(want, ""), StartMs: 0, EndMs: 12000}
		report, err := SmartSplitCue(context.Background(), Document{Format: "srt", Cues: []Cue{cue}}, "clauses", true, cue.ID, PunctuationOptions{})
		if err != nil {
			t.Fatal(err)
		}
		var got []string
		for _, part := range report.Document.Cues {
			got = append(got, strings.TrimSpace(part.Text))
		}
		for i := range want {
			want[i] = strings.TrimSpace(want[i])
		}
		assertSplitEnvelope(t, cue, report.Document.Cues)
		if !slices.Equal(got, want) {
			t.Errorf("split complete clauses for width: got %q, want %q", got, want)
		}
	}
}
