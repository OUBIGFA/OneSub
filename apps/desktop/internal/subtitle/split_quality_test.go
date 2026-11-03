package subtitle

import (
	"context"
	"fmt"
	"reflect"
	"slices"
	"strings"
	"testing"
)

// A manual command makes a readable cue eligible, not obliged, to split.
// Short predicates must not be divided just to produce two balanced pieces.
func TestSmartSplitKeepsCompactUtterances(t *testing.T) {
	for _, text := range []string{
		"为什么公司几乎立即就道歉？",
		"你能不能具体讲讲发生了什么？",
		"Kylian Mbappé 是个极具统治力的球员",
		"我们现在就检查所有输入文件",
		"为什么公司几乎立即就向公众道歉？",
		"Hello wonderful world today.",
	} {
		for _, duration := range []int64{4000, 20000, 60000} {
			t.Run(text+"/"+fmt.Sprint(duration), func(t *testing.T) {
				cue := Cue{ID: "compact", Text: text, OriginalText: text, StartMs: 1000, EndMs: 1000 + duration}
				doc := Document{Format: "srt", Cues: []Cue{cue}}
				for _, punctuation := range []PunctuationOptions{{}, {StripTrailing: true, ReplacePeriods: true}} {
					report, err := SmartSplitCue(context.Background(), doc, "compact", true, cue.ID, punctuation)
					if err != nil || report.Changed != 0 || !reflect.DeepEqual(report.Document.Cues, doc.Cues) {
						t.Fatalf("forced a readable utterance apart: %#v, %v", report.Document.Cues, err)
					}
				}
			})
		}
	}
}

// These are closed-class grammar connections, not a dictionary of the sample
// sentences. Check every emitted candidate so an alternative DP cut cannot
// conceal an unsafe connection that would fail on another sentence length.
func TestSplitChineseGrammarConnectionsAreNotCandidates(t *testing.T) {
	for _, connection := range [][2]string{
		{"是", "个极具统治力的球员"},
		{"是 ", "个极具统治力的球员"},
		{"是个", "极具统治力的球员"},
		{"就", "检查所有输入文件"},
		{"就 ", "检查所有输入文件"},
		{"就", "Kubernetes集群部署"},
		{"几乎", "立即就道歉"},
		{"立即", "就道歉"},
		{"具体", "讲讲发生了什么"},
		{"能不能", "具体讲讲发生了什么"},
		{"不", "应该忽略这些问题"},
		{"应该", "完整保留所有结果"},
		{"把", "所有输入文件保存"},
		{"被", "错误地删除所有记录"},
		{"对", "所有客户公开说明"},
		{"在", "会议结束之前完成"},
		{"一", "个完整的字幕文件"},
		{"一个", "完整的字幕文件"},
		{"三", "本详细的参考手册"},
		{"三本", "详细的参考手册"},
		{"這個", "完整的字幕檔案"},
		{"並沒有", "立即刪除所有記錄"},
		{"並不", "應該忽略這些問題"},
		{"统治", "力强大的球员"},
		{"讲", "讲发生了什么"},
		{"会不会", "立即公开说明"},
		{"能不能 ", "具体讲讲发生了什么"},
		{"Kylian ", "Mbappé是位球员"},
		{"Alice ", "Smith是位专家"},
		{"然后我们", "继续分析比赛表现"},
		{"继续", "分析比赛表现"},
		{"我们", "检查所有输入文件"},
		{"你", "能不能具体讲讲发生了什么"},
		{"人工智能", " 的发展前景"},
		{"研究", " 了全部资料"},
		{"具体的 ", "讲解内容"},
		{"“人工智能”", " 的发展前景"},
		{"人工智能的 ", "“发展前景”"},
		{"3", "本详细的参考手册"},
		{"3本", "详细的参考手册"},
		{"３ ", "本詳細的參考手冊"},
	} {
		t.Run(connection[0]+"/"+connection[1], func(t *testing.T) {
			prefix, suffix := "这段详细说明指出", "然后我们继续核对其余所有结果"
			text := prefix + connection[0] + connection[1] + suffix
			cut := len(prefix + connection[0])
			lane := newSplitText(text)
			if slices.ContainsFunc(lane.boundaries, func(b textBoundary) bool { return b.offset == cut }) {
				t.Fatalf("emitted a cut inside a grammatical connection: %q | %q", text[:cut], text[cut:])
			}
		})
	}
}

func TestSplitChineseWholeWordsAndPunctuationRemainAvailable(t *testing.T) {
	for _, word := range []string{"就职", "对方", "地理", "目的", "是非", "调和"} {
		text := "今天详细解释" + word + " 我们继续核对所有结果"
		cut := len("今天详细解释" + word + " ")
		lane := newSplitText(text)
		if !slices.ContainsFunc(lane.boundaries, func(b textBoundary) bool { return b.offset == cut }) {
			t.Fatal("applied a single-character restriction to a different whole word", word, lane.boundaries)
		}
	}
	text := "问题的详细解释就是，我们继续核对所有结果"
	cut := len("问题的详细解释就是，")
	if !slices.ContainsFunc(newSplitText(text).boundaries, func(b textBoundary) bool { return b.offset == cut }) {
		t.Fatal("grammar connection crossed actual punctuation")
	}
}

func TestSmartSplitLongChineseDialogueKeepsPredicatesAndMixedNames(t *testing.T) {
	for _, tc := range []struct {
		text  string
		terms []string
	}{
		{"今天我们详细介绍Kylian Mbappé 是个极具统治力的球员，然后我们继续分析他的技术特点和比赛表现。", []string{"Kylian Mbappé", "是个极具统治力的球员", "然后我们继续分析"}},
		{"在这份详细说明中你能不能具体讲讲发生了什么，然后我们就检查所有输入文件并且核对所有处理结果。", []string{"能不能具体讲讲", "就检查"}},
		{"这份完整报告说明为什么公司几乎立即就道歉，然后我们继续详细分析事件的具体经过和处理结果。", []string{"几乎立即就道歉"}},
	} {
		cue := Cue{ID: "grammar", Text: tc.text, StartMs: 1000, EndMs: 16000}
		report, err := SmartSplit(context.Background(), Document{Format: "srt", Cues: []Cue{cue}}, "grammar", true, PunctuationOptions{})
		if err != nil || report.Changed != 1 {
			t.Fatal("grammar protection prevented an otherwise feasible long-cue split", report, err)
		}
		assertSplitEnvelope(t, cue, report.Document.Cues)
		texts := []string{}
		for _, part := range report.Document.Cues {
			texts = append(texts, part.Text)
		}
		for _, term := range tc.terms {
			if !strings.Contains(strings.Join(texts, "|"), term) {
				t.Fatal("tore a predicate or mixed-language name", term, texts)
			}
		}
		t.Log(strings.Join(texts, " | "))
	}
}

func TestSmartSplitPreservesWholeQuestionsInLongDialogue(t *testing.T) {
	first, second, third := "为什么公司几乎立即就道歉？", "你能不能具体讲讲发生了什么？", "接着我们再继续核对所有记录。"
	cue := Cue{ID: "questions", Text: first + second + third, StartMs: 1000, EndMs: 9000}
	report, err := SmartSplit(context.Background(), Document{Format: "srt", Cues: []Cue{cue}}, "questions", true, PunctuationOptions{})
	if err != nil || report.Changed != 1 || len(report.Document.Cues) != 3 || report.Document.Cues[0].Text != first || report.Document.Cues[1].Text != second || report.Document.Cues[2].Text != third {
		t.Fatal("cut a short question instead of separating whole sentences", report, err)
	}
	assertSplitEnvelope(t, cue, report.Document.Cues)
}

func TestSmartSplitSlowSpeechDoesNotIncreaseTextPartCount(t *testing.T) {
	text := "我们今天一起来研究关于人工智能的发展前景和未来的商业落地应用"
	var wanted []string
	for _, duration := range []int64{4500, 20000, 60000} {
		cue := Cue{ID: "slow", Text: text, StartMs: 0, EndMs: duration}
		report, err := SmartSplitCue(context.Background(), Document{Format: "srt", Cues: []Cue{cue}}, "slow", true, cue.ID, PunctuationOptions{})
		if err != nil || report.Changed != 1 || len(report.Document.Cues) != 3 {
			t.Fatal("slow delivery caused unnecessary short pieces", report, err)
		}
		assertSplitEnvelope(t, cue, report.Document.Cues)
		got := []string{}
		for _, part := range report.Document.Cues {
			got = append(got, part.Text)
		}
		if wanted == nil {
			wanted = got
		} else if !slices.Equal(got, wanted) {
			t.Fatal("duration changed the linguistic cuts", wanted, got)
		}
	}
}

func TestSmartSplitCompactTranslationIsNotDrivenByLongSource(t *testing.T) {
	first := "This first source sentence explains the complete history in considerable detail. "
	last := "This last source sentence describes every remaining operation and its results."
	for _, aligned := range []bool{false, true} {
		cue := Cue{ID: "compact-translation", Text: "你能不能具体讲讲发生了什么？", TranslationSource: first + last, Translated: true, StartMs: 1000, EndMs: 7000}
		if aligned {
			cue.Words = []Word{{first, 1000, 3000}, {last, 3800, 6400}}
		}
		doc := Document{Format: "srt", Cues: []Cue{cue}}
		for _, id := range []string{"", cue.ID} {
			var report SplitReport
			var err error
			punctuation := PunctuationOptions{StripTrailing: true, ReplacePeriods: true}
			if id == "" {
				report, err = SmartSplit(context.Background(), doc, "compact-translation", false, punctuation)
			} else {
				report, err = SmartSplitCue(context.Background(), doc, "compact-translation", false, id, punctuation)
			}
			if err != nil || report.Changed != 0 || !reflect.DeepEqual(report.Document.Cues, doc.Cues) {
				t.Fatal("long source or its clock forced a compact translation apart", aligned, id, report, err)
			}
		}
	}
}

func TestSmartSplitWordPauseCannotOverrideChineseGrammar(t *testing.T) {
	words := []Word{
		{"今天我们详细介绍Kylian Mbappé 是", 0, 2600},
		{"个极具统治力的球员，", 3400, 6300},
		{"然后我们继续分析所有比赛表现。", 7000, 11000},
	}
	text := words[0].Text + words[1].Text + words[2].Text
	cue := Cue{ID: "grammar-clock", Text: text, Words: words, StartMs: 0, EndMs: 12000}
	report, err := SmartSplit(context.Background(), Document{Format: "srt", Cues: []Cue{cue}}, "grammar-clock", true, PunctuationOptions{})
	if err != nil || report.Changed != 1 || len(report.Document.Cues) != 2 || report.Estimated != 0 || report.Document.Cues[0].Text != words[0].Text+words[1].Text || report.Document.Cues[1].StartMs != 7000 {
		t.Fatal("an ASR pause reopened 是/个 instead of keeping the grammatical unit", report, err)
	}
	assertSplitEnvelope(t, cue, report.Document.Cues)
	allWords := append(slices.Clone(report.Document.Cues[0].Words), report.Document.Cues[1].Words...)
	if !reflect.DeepEqual(allWords, words) {
		t.Fatal("lost, duplicated or retimed word evidence", allWords, words)
	}
}

func TestSmartSplitCompactCueCanUseIndependentSentencesOrMeasuredPause(t *testing.T) {
	for _, aligned := range []bool{false, true} {
		cue := Cue{ID: "natural", Text: "先检查所有文件。再运行全部测试。", StartMs: 0, EndMs: 6000}
		if aligned {
			cue.Words = []Word{{"先检查所有文件", 0, 2200}, {"再运行全部测试", 3000, 5400}}
			cue.Text = cue.Words[0].Text + cue.Words[1].Text
		}
		report, err := SmartSplitCue(context.Background(), Document{Format: "srt", Cues: []Cue{cue}}, "natural", true, cue.ID, PunctuationOptions{})
		if err != nil || report.Changed != 1 || len(report.Document.Cues) != 2 || !strings.HasPrefix(report.Document.Cues[1].Text, "再运行") {
			t.Fatal("blocked a meaningful manual split", report, err)
		}
		assertSplitEnvelope(t, cue, report.Document.Cues)
		if aligned && (report.Estimated != 0 || report.Document.Cues[1].StartMs != 3000) {
			t.Fatal("changed the actual word boundary", report)
		}
	}
}
