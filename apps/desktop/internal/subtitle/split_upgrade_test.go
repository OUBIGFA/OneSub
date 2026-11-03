package subtitle

import (
	"context"
	"reflect"
	"slices"
	"strings"
	"testing"
	"unicode"
	"unicode/utf8"
)

func assertSplitEnvelope(t *testing.T, cue Cue, parts []Cue) {
	t.Helper()
	if len(parts) < 2 {
		t.Fatalf("no split for %q", cue.Text)
	}
	if parts[0].StartMs != cue.StartMs || parts[len(parts)-1].EndMs != cue.EndMs {
		t.Fatal("changed outer envelope", parts)
	}
	text, source := "", ""
	for i, part := range parts {
		if !utf8.ValidString(part.Text) || part.EndMs-part.StartMs < minimumSplitDuration {
			t.Fatal("invalid text or flashing piece", part)
		}
		if i > 0 && part.StartMs != parts[i-1].EndMs {
			t.Fatal("introduced a gap or overlap", parts)
		}
		first, _ := utf8.DecodeRuneInString(strings.TrimSpace(part.Text))
		if i > 0 && lineStartPunctuation(first) {
			t.Fatal("stranded punctuation at line start", parts)
		}
		text += part.Text
		source += SourceText(part)
	}
	if text != cue.Text || source != SourceText(cue) {
		t.Fatalf("lost or inserted text: %q / %q", text, source)
	}
}

// Report scenarios are tested as observable invariants: semantic terms stay
// whole, no short punctuation islands are emitted, and all payload survives.
// The engine does not rewrite dialogue or fabricate quote continuations.
func TestSplitUpgradeChineseSemanticCases(t *testing.T) {
	for _, tc := range []struct {
		name, text string
		duration   int64
		terms      []string
	}{
		{"phrases", "我们今天一起来研究关于人工智能的发展前景和未来的商业落地应用", 4500, []string{"关于人工智能的发展前景", "人工智能", "发展前景", "商业落地"}},
		{"intro", "好，我们接下来继续看下一个核心参数的详细配置方法", 3800, []string{"接下来", "下一个", "核心参数", "配置方法"}},
		{"quotation", "大家请翻开“深入理解计算机系统”第四章查看流水线设计的详细架构", 4200, []string{"“深入理解计算机系统”", "流水线设计"}},
		{"confirmations", "对。是的。完全同意你的看法，这非常重要。", 4000, nil},
		{"balance", "通过这次长期的社会调查研究我们可以清晰地得出结论", 4000, []string{"通过", "社会调查", "研究", "结论"}},
		{"complex", "在大会上专家明确指出：“区块链技术是一项战略性创新”，但是我们必须清醒地认识到技术落地的潜在风险", 7000, []string{"在大会上", "区块链技术", "战略性创新", "但是", "认识到", "技术落地"}},
		{"dense_commas", "买笔，买纸，买墨，买砚，样样都要花钱", 5200, nil},
	} {
		t.Run(tc.name, func(t *testing.T) {
			cue := Cue{ID: tc.name, Text: tc.text, StartMs: 500, EndMs: 500 + tc.duration}
			report, err := SmartSplitCue(context.Background(), Document{Format: "srt", Cues: []Cue{cue}}, "upgrade", true, cue.ID, PunctuationOptions{})
			if err != nil || report.Changed != 1 {
				t.Fatal(report, err)
			}
			parts := report.Document.Cues
			assertSplitEnvelope(t, cue, parts)
			joined := []string{}
			for _, part := range parts {
				load := InspectReading(part.Text, part.EndMs-part.StartMs)
				if load.Units < 4 || load.Units > 16 {
					t.Fatalf("fragment or solvable overflow: %q (%g units)", part.Text, load.Units)
				}
				joined = append(joined, part.Text)
			}
			for _, term := range tc.terms {
				if !strings.Contains(strings.Join(joined, "|"), term) {
					t.Fatal("cut a semantic term", term, joined)
				}
			}
			t.Log(strings.Join(joined, " | "))
		})
	}
}

func TestSplitUpgradeRoutesActualTextLanguages(t *testing.T) {
	for _, tc := range []struct {
		language splitLanguage
		text     string
	}{
		{splitChinese, "我們今天一起研究人工智慧的發展前景，然後檢查控制系統的各項配置方法。"},
		{splitJapanese, "本日は人工知能の仕組みについて説明します。その後、実際の使い方を詳しく確認します。"},
		{splitKorean, "오늘은 인공지능 시스템의 동작 방법을 자세하게 설명합니다. 그다음 모든 설정과 결과를 차례대로 확인합니다."},
		{splitWords, "Nous allons expliquer toutes les étapes du traitement, puis vérifier chaque résultat avant de terminer cette opération."},
		{splitWords, "Primero explicamos todos los pasos del proceso, pero después comprobamos cada resultado antes de terminar la operación."},
		{splitWords, "Сначала мы подробно объясним порядок работы системы, а затем проверим все параметры и полученные результаты."},
		{splitWords, "نشرح اليوم جميع خطوات العمل بالتفصيل، ثم نتحقق من كل النتائج قبل إكمال هذه العملية المهمة."},
		{splitThai, "วันนี้เราจะอธิบายวิธีการทำงานของระบบอย่างละเอียด จากนั้นเราจะตรวจสอบผลลัพธ์ทั้งหมดก่อนดำเนินการต่อไป"},
	} {
		t.Run(tc.text, func(t *testing.T) {
			lane := newSplitText(tc.text)
			if lane.language != tc.language {
				t.Fatal("routed to another script's algorithm", lane.language, tc.language)
			}
			cue := Cue{ID: "language", Text: tc.text, StartMs: 0, EndMs: 20000}
			report, err := SmartSplitCue(context.Background(), Document{Format: "srt", Cues: []Cue{cue}}, "language", true, cue.ID, PunctuationOptions{})
			if err != nil || report.Changed != 1 {
				t.Fatal(report, err)
			}
			assertSplitEnvelope(t, cue, report.Document.Cues)
			if tc.language == splitWords || tc.language == splitKorean {
				pieces := []string{}
				for _, part := range report.Document.Cues {
					pieces = append(pieces, part.Text)
				}
				for _, word := range strings.Fields(tc.text) {
					if !strings.Contains(strings.Join(pieces, "|"), word) {
						t.Fatal("split a whole Unicode word", word, pieces)
					}
				}
			}
		})
	}
}

func TestSplitUpgradeTranslationDrivesCountAndBoundaries(t *testing.T) {
	translation := "首先检查所有输入文件，然后完整说明当前操作的各项步骤。"
	sources := []string{
		"The first source sentence has enough detail. The second source sentence carries additional information. The final source sentence finishes every operation.",
		"This much longer source explains additional detailed information about every individual operation and configuration parameter before checking all remaining results together.",
	}
	var wanted []string
	for _, source := range sources {
		cue := Cue{ID: "paired", Text: translation, TranslationSource: source, Translated: true, StartMs: 1000, EndMs: 7500}
		before := cue
		report, err := SmartSplitCue(context.Background(), Document{Format: "srt", Cues: []Cue{cue}}, "translation", false, cue.ID, PunctuationOptions{})
		if err != nil || report.Changed != 1 || len(report.Document.Cues) != 2 {
			t.Fatal("source length dictated the translated split", report, err)
		}
		assertSplitEnvelope(t, cue, report.Document.Cues)
		got := []string{report.Document.Cues[0].Text, report.Document.Cues[1].Text}
		if wanted == nil {
			wanted = got
		} else if !slices.Equal(got, wanted) {
			t.Fatal("source punctuation changed the primary translation cuts", wanted, got)
		}
		if !reflect.DeepEqual(before, cue) {
			t.Fatal("mutated input")
		}
	}
	// Even with source word clocks, the translation still drives the count.
	pieces := strings.SplitAfter(sources[0], ". ")
	words := []Word{{pieces[0], 1000, 2400}, {pieces[1], 3000, 4700}, {pieces[2], 5400, 7100}}
	cue := Cue{ID: "aligned-paired", Text: translation, TranslationSource: sources[0], Translated: true, Words: words, StartMs: 1000, EndMs: 7500}
	report, err := SmartSplitCue(context.Background(), Document{Format: "srt", Cues: []Cue{cue}}, "aligned", false, cue.ID, PunctuationOptions{})
	if err != nil || report.Changed != 1 || len(report.Document.Cues) != 2 || report.Estimated != 0 {
		t.Fatal(report, err)
	}
	assertSplitEnvelope(t, cue, report.Document.Cues)
	if got := []string{report.Document.Cues[0].Text, report.Document.Cues[1].Text}; !slices.Equal(got, wanted) {
		t.Fatal("word clock changed translation-primary cuts", got, wanted)
	}
	cut := report.Document.Cues[1].StartMs
	if cut != words[1].StartMs && cut != words[2].StartMs {
		t.Fatal("estimated a cut despite exact source evidence", cut)
	}
	allWords := append(slices.Clone(report.Document.Cues[0].Words), report.Document.Cues[1].Words...)
	if !reflect.DeepEqual(allWords, words) {
		t.Fatal("changed or duplicated ASR words", allWords, words)
	}
}

func TestSplitUpgradePairedAndLiteralProtection(t *testing.T) {
	for _, text := range []string{
		"大家请翻开（《深入理解计算机系统》）第四章，然后仔细查看所有控制系统的实际运行参数。",
		"He said \"check every complete command\" before explaining all the remaining steps in this long example.",
		"We don't remove the user's data because it's important to keep O'Connor and students' projects intact during every operation.",
		"我们在Kubernetes集群上部署了vLLM推理服务，然后使用GPT-4o和OneSub-Desktop检查所有任务。",
		`Keep "first.last"@bbc.co.uk and first.last@[127.0.0.1] intact before we explain every remaining operation in this example.`,
		"Keep AsiaPacific@bbc.co.uk, 3.14159, 12,500, 12:30, 16:9 and Ctrl+Shift+S intact before we finish every remaining operation.",
		"需要沿着道路继续向前走50米，然后检查其余所有控制系统和字幕中的配置参数。",
		"他说：“我们需要完整保留所有控制系统的配置参数，然后继续检查人工智能的发展前景。后面的正常字幕仍须继续拆分。",
	} {
		cue := Cue{ID: "protected-content", Text: text, StartMs: 0, EndMs: 20000}
		report, err := SmartSplitCue(context.Background(), Document{Format: "srt", Cues: []Cue{cue}}, "protect", true, cue.ID, PunctuationOptions{})
		if err != nil || report.Changed != 1 {
			t.Fatal(report, err)
		}
		assertSplitEnvelope(t, cue, report.Document.Cues)
		parts := []string{}
		for _, part := range report.Document.Cues {
			parts = append(parts, part.Text)
		}
		for _, term := range []string{"（《深入理解计算机系统》）", `"check every complete command"`, "don't", "user's", "it's", "O'Connor", "students'", "Kubernetes", "vLLM", "GPT-4o", "OneSub-Desktop", "AsiaPacific@bbc.co.uk", "3.14159", "12,500", "12:30", "16:9", "Ctrl+Shift+S", "50米", `"first.last"@bbc.co.uk`, "first.last@[127.0.0.1]"} {
			if strings.Contains(text, term) && !strings.Contains(strings.Join(parts, "|"), term) {
				t.Fatal("tore a protected term", term, parts)
			}
		}
	}
}

func TestSplitUpgradeLongContainersContinueWithoutInventedMarks(t *testing.T) {
	for _, pair := range [][2]string{{"“", "”"}, {"'", "'"}, {"《", "》"}, {"（", "）"}, {"【", "】"}, {"「", "」"}, {"〔", "〕"}, {"{", "}"}} {
		text := pair[0] + strings.Repeat("人工智能的发展前景和未来的商业落地应用，", 3) + pair[1]
		cue := Cue{ID: "long-quote", Text: text, StartMs: 0, EndMs: 16000}
		report, err := SmartSplitCue(context.Background(), Document{Format: "srt", Cues: []Cue{cue}}, "long-pair", true, cue.ID, PunctuationOptions{})
		if err != nil || report.Changed != 1 {
			t.Fatal(pair, report, err)
		}
		assertSplitEnvelope(t, cue, report.Document.Cues)
		parts := report.Document.Cues
		if !strings.HasPrefix(parts[0].Text, pair[0]) || !strings.HasSuffix(parts[len(parts)-1].Text, pair[1]) {
			t.Fatal("moved continuation marks", parts)
		}
		for i, part := range parts {
			if i > 0 && strings.Contains(part.Text, pair[0]) && pair[0] != pair[1] || i < len(parts)-1 && strings.Contains(part.Text, pair[1]) && pair[0] != pair[1] {
				t.Fatal("invented a continuation delimiter", parts)
			}
		}
	}
}

func TestSplitUpgradeRejectsFragmentsAndPreservesUnsplittableCues(t *testing.T) {
	for _, cue := range []Cue{
		{ID: "tiny", Text: "早上好晚上好", StartMs: 0, EndMs: 6000},
		{ID: "few-words", Text: "Hello world.", StartMs: 0, EndMs: 6000},
		{ID: "too-fast", Text: strings.Repeat("完整内容", 12), StartMs: 0, EndMs: 2000},
		{ID: "one-word", Text: strings.Repeat("Kubernetes", 8), StartMs: 0, EndMs: 10000},
		{ID: "too-many-boundaries", Text: strings.Repeat("完整内容，", 1100), StartMs: 0, EndMs: 30000},
	} {
		report, err := SmartSplitCue(context.Background(), Document{Format: "srt", Cues: []Cue{cue}}, "unsplit", true, cue.ID, PunctuationOptions{ReplacePeriods: true})
		if err != nil || report.Changed != 0 || !reflect.DeepEqual(report.Document.Cues, []Cue{cue}) || report.Skipped[cue.ID] == "" {
			t.Fatal("failed split changed a cue or claimed success", report, err)
		}
	}
	cue := Cue{ID: "slow", Text: "我们今天继续说明", StartMs: 0, EndMs: 6000}
	report, err := SmartSplitCue(context.Background(), Document{Format: "srt", Cues: []Cue{cue}}, "slow", true, cue.ID, PunctuationOptions{})
	if err != nil || len(report.Document.Cues) > 2 {
		t.Fatal("slow speech was over-fragmented", report, err)
	}
}

func TestSplitUpgradeEmbeddedLanguageResources(t *testing.T) {
	trie := splitLexicon()
	if len(trie.data) > 2*1024*1024 {
		t.Fatal("Chinese DAT exceeds the memory budget", len(trie.data))
	}
	for _, word := range []string{"人工智能", "人工智慧", "控制系统", "控制系統", "关于", "關於", "认识到"} {
		state := 1
		for _, b := range []byte(word) {
			state = trie.advance(state, b)
			if state == 0 {
				t.Fatal("missing reviewed dictionary word", word)
			}
		}
		if trie.cost(state) == 0 {
			t.Fatal("missing dictionary terminal", word)
		}
	}
	for _, tc := range []struct {
		model phraseModel
		text  string
		want  []string
	}{
		{japanesePhrases(), "今日は天気です。", []string{"今日は", "天気です。"}},
		{thaiPhrases(), "วันนี้อากาศดี", []string{"วัน", "นี้", "อากาศ", "ดี"}},
	} {
		bytes := []int{}
		for offset := range tc.text {
			bytes = append(bytes, offset)
		}
		bytes = append(bytes, len(tc.text))
		chunks, previous := []string{}, 0
		for i := 1; i < len(bytes)-1; i++ {
			if tc.model.boundary(tc.text, bytes, i) {
				chunks, previous = append(chunks, tc.text[previous:bytes[i]]), bytes[i]
			}
		}
		chunks = append(chunks, tc.text[previous:])
		if !slices.Equal(chunks, tc.want) {
			t.Fatal("BudouX port differs from official example", chunks, tc.want)
		}
	}
}

func FuzzIntelligentSplitPreservesText(f *testing.F) {
	for _, text := range []string{
		"好，我们接下来继续说明完整内容，然后检查人工智能的发展前景。",
		"Keep 👨‍👩‍👧‍👦 and e\u0301 intact. Do not damage Dr. Smith's email test@example.org.",
		"これは長い字幕の例です。すべての文字と記号を保存してください。",
		"'Quote' (parentheses) 《书名》 3.14 https://example.test/a.b",
	} {
		f.Add(text)
	}
	f.Fuzz(func(t *testing.T, text string) {
		if !utf8.ValidString(text) || len(text) > 3000 || strings.ContainsAny(text, "\r\n") || strings.TrimSpace(text) == "" {
			t.Skip()
		}
		cue := Cue{ID: "fuzz", Text: text, StartMs: 1000, EndMs: 21000}
		report, err := SmartSplitCue(context.Background(), Document{Format: "srt", Cues: []Cue{cue}}, "fuzz", true, cue.ID, PunctuationOptions{})
		if err != nil {
			t.Fatal(err)
		}
		if report.Changed == 0 {
			if !reflect.DeepEqual(report.Document.Cues, []Cue{cue}) {
				t.Fatal("unsplittable input changed")
			}
			return
		}
		parts := report.Document.Cues
		if parts[0].StartMs != cue.StartMs || parts[len(parts)-1].EndMs != cue.EndMs {
			t.Fatal("outer times changed")
		}
		combined := ""
		for i, part := range parts {
			if !utf8.ValidString(part.Text) || part.EndMs-part.StartMs < minimumSplitDuration || i > 0 && part.StartMs != parts[i-1].EndMs {
				t.Fatal("invalid split", part)
			}
			combined += part.Text
			// Existing illegal punctuation at the very beginning is retained;
			// a newly introduced boundary must not strand it on the next piece.
			first, _ := utf8.DecodeRuneInString(strings.TrimLeftFunc(part.Text, unicode.IsSpace))
			if i > 0 && lineStartPunctuation(first) {
				t.Fatal("introduced line-start punctuation", parts)
			}
		}
		if combined != text {
			t.Fatal("split lost text", combined, text)
		}
	})
}

func BenchmarkIntelligentSplit(b *testing.B) {
	text := "在大会上专家明确指出人工智能的发展前景，但是我们必须清醒地认识到技术落地的潜在风险"
	newSplitText(text).divide(3, nil) // initialize immutable resources and pools
	b.ReportAllocs()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		parts, _ := newSplitText(text).divide(3, nil)
		if len(parts) != 3 {
			b.Fatal("benchmark did not exercise a split")
		}
	}
}
