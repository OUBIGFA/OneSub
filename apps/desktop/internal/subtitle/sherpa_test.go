package subtitle

import (
	"testing"
)

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
	cues := WordCues(words)
	if len(cues) != 1 || cues[0].Text != "开饭时间早上9点至下午5点。" || cues[0].StartMs != 720 || cues[0].EndMs != 5500 {
		t.Fatal(cues)
	}
	if _, _, err := attachCheck(cues); err != nil {
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
	// The 2.16 s pause between "chieftain" and "with" ends the first cue.
	cues := WordCues(words)
	if len(cues) != 2 || cues[0].Text != "The tribal chieftain" || cues[1].Text != "with 50 pieces." || cues[0].EndMs != 2920 {
		t.Fatal(got, cues)
	}
}

func TestSherpaWordsKoreanAndJapanese(t *testing.T) {
	ko := SherpaResult{Tokens: []string{"조", "금", "만", " 생각을", " 하면서", "."}, Timestamps: []float64{0.1, 0.2, 0.3, 0.5, 1.0, 1.4}}
	if text := WordCues(SherpaWords(ko, SherpaPieces, 2000))[0].Text; text != "조금만 생각을 하면서." {
		t.Fatal(text)
	}
	ja := SherpaResult{Tokens: []string{"は", "5", "0", "円", "。"}, Timestamps: []float64{0.1, 0.2, 0.3, 0.4, 0.5}}
	words := SherpaWords(ja, SherpaPieces, 1000)
	if len(words) != 3 || words[1].Text != "50" || WordCues(words)[0].Text != "は50円。" {
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
	if text := WordCues(words)[0].Text; text != "yesterday was星期一today is" {
		t.Fatal(text)
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

func TestWordCuesBreakAtSentencesPausesAndLength(t *testing.T) {
	words := []SpacedWord{}
	add := func(text string, start, end int64, space bool) {
		words = append(words, SpacedWord{Word: Word{Text: text, StartMs: start, EndMs: end}, Space: space})
	}
	add("Pi", 0, 200, false)
	add("is", 200, 400, true)
	add("3.", 400, 600, true)
	add("14.", 600, 800, false)
	add("Yes!", 900, 1100, true)
	add("Later", 2000, 2300, true)
	cues := WordCues(words)
	if len(cues) != 3 || cues[0].Text != "Pi is 3.14." || cues[1].Text != "Yes!" || cues[2].Text != "Later" || cues[0].EndMs != 900 || cues[1].EndMs != 1200 {
		t.Fatal(cues)
	}
	long := []SpacedWord{}
	for i := 0; i < 40; i++ {
		text := "字"
		if i == 19 {
			text = "字，"
		}
		long = append(long, SpacedWord{Word: Word{Text: text, StartMs: int64(i) * 200, EndMs: int64(i)*200 + 200}})
	}
	cues = WordCues(long)
	if len(cues) != 2 || len([]rune(cues[0].Text)) != 21 {
		t.Fatal(cues)
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
