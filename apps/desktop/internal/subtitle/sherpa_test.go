package subtitle

import (
	"strings"
	"testing"
)

// A recognizer result is the original subtitle boundary. Display length,
// punctuation and pauses must not turn its trailing word into another cue.
func TestSherpaOriginalResultIsNotSplit(t *testing.T) {
	for _, text := range []string{
		"We will use the selected object and then apply a simple bevel modifier.",
		"A complete sentence. Another one! The end?",
		strings.Repeat("这是一段需要保留完整边界的中文原文", 4) + "。",
	} {
		t.Run(text, func(t *testing.T) {
			words := []SpacedWord{}
			for i, part := range strings.Fields(text) {
				words = append(words, SpacedWord{Word: Word{Text: part, StartMs: int64(i) * 2000, EndMs: int64(i)*2000 + 300}, Space: i > 0})
			}
			// Split Chinese into token evidence, just as the native model does.
			if len(words) == 1 {
				words = nil
				for i, r := range []rune(text) {
					words = append(words, SpacedWord{Word: Word{Text: string(r), StartMs: int64(i) * 200, EndMs: int64(i)*200 + 200}})
				}
			}
			cue := SherpaCue(text, words, words[len(words)-1].EndMs+100)
			if cue.Text != text || cue.OriginalText != text || len(cue.Words) != len(words) || cue.StartMs != words[0].StartMs || cue.EndMs != words[len(words)-1].EndMs+100 {
				t.Fatalf("engine result changed: %+v", cue)
			}
		})
	}
}

// The results below are real sherpa-onnx v2.0.1 output of the models'
// own test recordings.
func TestSherpaWordsSenseVoiceChinese(t *testing.T) {
	results, err := ParseSherpaResults("Done!\n" + `{"lang": "<|zh|>", "emotion": "<|NEUTRAL|>", "event": "<|Speech|>", "text": "开饭时间早上9点至下午5点。", "timestamps": [0.72, 0.96, 1.26, 1.50, 1.92, 2.10, 2.58, 2.88, 3.30, 3.90, 4.26, 4.62, 4.80, 5.52], "durations": [], "tokens":["开", "饭", "时", "间", "早", "上", "9", "点", "至", "下", "午", "5", "点", "。"], "ys_log_probs": [], "words": []}`)
	if err != nil || len(results) != 1 {
		t.Fatal(results, err)
	}
	words := SherpaWords(results[0], SherpaPieces, 5600)
	if len(words) != 13 || words[12].Text != "点。" || words[0].StartMs != 720 || words[0].EndMs != 960 || words[12].EndMs != 5400 {
		t.Fatal(words)
	}
	cue := SherpaCue(results[0].Text, words, 5600)
	if cue.Text != "开饭时间早上9点至下午5点。" || cue.StartMs != 720 || cue.EndMs != 5500 {
		t.Fatal(cue)
	}
	if _, _, err := attachCheck([]Cue{cue}); err != nil {
		t.Fatal(err)
	}
}

func TestSherpaWordsSentencePiecesJoinWordsAndNumbers(t *testing.T) {
	r := SherpaResult{Tokens: []string{"The", " tri", "bal", " chief", "tain", " with", " ", "5", "0", " pieces", "."}, Timestamps: []float64{0.90, 1.32, 1.56, 1.86, 2.22, 4.98, 5.16, 5.34, 5.52, 5.94, 6.24}}
	words := SherpaWords(r, SherpaPieces, 7000)
	got := []string{}
	for _, w := range words {
		got = append(got, w.Text)
	}
	// A pause inside the engine result does not create a subtitle boundary.
	if strings.Join(got, "|") != "The|tribal|chieftain|with|50|pieces." {
		t.Fatal(got)
	}
}

func TestSherpaWordsKoreanAndJapanese(t *testing.T) {
	ko := SherpaResult{Tokens: []string{"조", "금", "만", " 생각을", " 하면서", "."}, Timestamps: []float64{0.1, 0.2, 0.3, 0.5, 1.0, 1.4}}
	koWords := SherpaWords(ko, SherpaPieces, 2000)
	if len(koWords) != 5 || koWords[4].Text != "하면서." || !koWords[3].Space {
		t.Fatal(koWords)
	}
	ja := SherpaResult{Tokens: []string{"は", "5", "0", "円", "。"}, Timestamps: []float64{0.1, 0.2, 0.3, 0.4, 0.5}}
	words := SherpaWords(ja, SherpaPieces, 1000)
	if len(words) != 3 || words[1].Text != "50" || words[2].Text != "円。" {
		t.Fatal(words)
	}
}

func TestSherpaWordsTransducerDurations(t *testing.T) {
	r := SherpaResult{Tokens: []string{" A", "sk", " not", ",", " a", "sk", "."}, Timestamps: []float64{0, 0.24, 0.40, 0.64, 0.80, 0.96, 1.12}, Durations: []float64{0.24, 0.16, 0.24, 0.16, 0.16, 0.08, 0.08}}
	words := SherpaWords(r, SherpaPieces, 2000)
	if len(words) != 3 || words[0].Text != "Ask" || words[0].EndMs != 400 || words[1].Text != "not," || words[1].EndMs != 640 || words[2].EndMs != 1040 {
		t.Fatal(words)
	}
}

func TestSherpaWordsParaformerContinuationAndPunctuation(t *testing.T) {
	r := SherpaResult{Tokens: []string{"ye@@", "ster@@", "day", "was", "星", "期", "一", "today", "is"}, Timestamps: []float64{0.38, 0.60, 0.90, 1.22, 2.26, 2.50, 2.74, 3.54, 4.06}}
	words := SherpaWords(r, SherpaContinuation, 5000)
	if len(words) != 7 || words[0].Text != "yesterday" || words[0].StartMs != 380 {
		t.Fatal(words)
	}
	// CT-Transformer removes the spaces around Chinese and adds marks.
	punctuated, ok := PunctuateWords(words, "yesterday was星期一，today is。")
	if !ok || punctuated[4].Text != "一，" || punctuated[6].Text != "is。" {
		t.Fatal(punctuated, ok)
	}
	if _, ok := PunctuateWords(words, "yesterday was 星期二"); ok {
		t.Fatal("changed words accepted")
	}
}

// Preserve engine spacing and punctuation even when the vocabulary uses
// multiple token pieces; inference input boundaries must remain separate.
func TestSherpaCueKeepsNativeTextAndRegionBoundary(t *testing.T) {
	words := []SpacedWord{{Word: Word{Text: "Hello", StartMs: 100, EndMs: 300}}, {Word: Word{Text: "world.", StartMs: 400, EndMs: 700}, Space: true}}
	cue := SherpaCue("Hello  world.", words, 750)
	if cue.Text != "Hello  world." || cue.EndMs != 750 || len(cue.Words) != 2 {
		t.Fatal(cue)
	}
	if _, _, err := attachCheck([]Cue{cue}); err != nil {
		t.Fatal(err)
	}
	words[0].Text = "changed"
	if cue.Words[0].Text != "Hello" {
		t.Fatal("cue aliases mutable word evidence")
	}
}

func TestSherpaResultsRejectMismatchedTokens(t *testing.T) {
	if _, err := ParseSherpaResults(`{"text": "a", "timestamps": [0.1], "tokens": ["a", "b"]}`); err == nil {
		t.Fatal("accepted")
	}
}

// attachCheck runs the cues and their words through the same attachment
// used for OneAsr artifacts, so a recognized cue stays splittable.
func attachCheck(cues []Cue) (Document, []string, error) {
	all := []Word{}
	doc := Document{Format: "srt"}
	for i, cue := range cues {
		all = append(all, cue.Words...)
		cue.Words = nil
		cue.ID = "c" + string(rune('a'+i))
		doc.Cues = append(doc.Cues, cue)
	}
	data, err := MarshalOneAsrWords(all)
	if err != nil {
		return Document{}, nil, err
	}
	out, warnings, err := AttachOneAsrWords(doc, data)
	if err == nil && len(warnings) > 0 {
		return out, warnings, errNotAttached
	}
	return out, warnings, err
}

var errNotAttached = errorString("words not attached")

type errorString string

func (e errorString) Error() string { return string(e) }
