package translator

import (
	"slices"
	"testing"
)

// Answers are read however a model wraps them: reasoning, fences, loose
// JSON, nested values, an object cut short.
func TestParseObjectReadsLooseAnswers(t *testing.T) {
	for name, c := range map[string]struct {
		answer string
		want   map[string]string
	}{
		"plain":    {`{"1": "一", "2": "二"}`, map[string]string{"1": "一", "2": "二"}},
		"fenced":   {"Here:\n```json\n{\"1\": \"一\"}\n```", map[string]string{"1": "一"}},
		"thinking": {"<think>plan {draft}</think>{\"1\": \"一\"}", map[string]string{"1": "一"}},
		"unclosed": {"<think>still planning\n```json\n{\"1\": \"一\"}\n```", map[string]string{"1": "一"}},
		"trailing": {"{\"1\": \"一\",\n\"2\": \"二\",\n}", map[string]string{"1": "一", "2": "二"}},
		"newline":  {"{\"1\": \"第一行\n第二行\"}", map[string]string{"1": "第一行\n第二行"}},
		"nested":   {`{"1": {"src": "One", "tr": "一"}, "2": {"translation": "二"}}`, map[string]string{"1": "一", "2": "二"}},
		"cut":      {`{"1": "一", "2": "二", "3": "三`, map[string]string{"1": "一", "2": "二"}},
		"escaped":  {`{"1": "say \"hi\"。"}`, map[string]string{"1": `say "hi"。`}},
	} {
		got, err := parseObject(c.answer)
		if err != nil || len(got) != len(c.want) {
			t.Fatal(name, got, err)
		}
		for k, v := range c.want {
			if got[k] != v {
				t.Fatal(name, k, got[k])
			}
		}
	}
	if _, err := parseObject("I cannot help with that."); err == nil {
		t.Fatal("prose read as an object")
	}
}

// A streamed answer shows the subtitle being written.
func TestScanPairsShowsTheValueBeingWritten(t *testing.T) {
	pairs, partial := scanPairs(`{"1": "一", "2": "正在写`)
	if len(pairs) != 1 || partial == nil || partial.key != "2" || partial.value != "正在写" {
		t.Fatal(pairs, partial)
	}
}

// Lines map onto the source atoms by their letters and digits: changed
// spaces, punctuation or capitals never move a boundary, and a boundary
// inside an atom, which cannot be cut, joins the line before it.
func TestAlignLines(t *testing.T) {
	atoms := []string{"Hello", " world,", " this", " is", " a test."}
	a := alignLines(atoms, []string{"hello world", "This is a test"})
	if !slices.Equal(a.cuts, []int{2}) || a.similarity < 0.99 {
		t.Fatal(a)
	}
	if a = alignLines([]string{"Hello world", "this is"}, []string{"Hello", "world this is"}); len(a.cuts) != 0 {
		t.Fatal("a cut inside an atom", a)
	}
	if a = alignLines([]string{"one two", "three"}, []string{"something else entirely"}); a.similarity > 0.5 {
		t.Fatal("a rewritten answer matched", a)
	}
	if a = alignLines([]string{"第一句", "第二句", "第三句"}, []string{"第一句第二句", "第三句"}); !slices.Equal(a.cuts, []int{2}) {
		t.Fatal(a)
	}
}

// The target profile's mechanical rules, as the skill's checker tests them.
func TestPolishFollowsTheTargetProfile(t *testing.T) {
	zh := language{Code: "zh", FinalPunctuation: "none", Spacing: true}
	for in, want := range map[string]string{
		"设置为 20。":          "设置为 20",
		"真的吗？":             "真的吗？",
		"在C4D中设置100%":      "在 C4D 中设置 100%",
		"<i>你好。</i>":       "<i>你好</i>",
		"好了！":              "好了",
		"第一行，\n第二行。":       "第一行\n第二行",
		"{\\an8}按Ctrl+S保存": "{\\an8}按 Ctrl+S 保存",
	} {
		if got := polish(in, zh); got != want {
			t.Fatalf("%q: %q, want %q", in, got, want)
		}
	}
	en := language{Code: "en", FinalPunctuation: "standard"}
	if got := polish("Hello.", en); got != "Hello." {
		t.Fatal(got)
	}
}

// A number of the source missing in the translation is named; a number the
// translation writes for a spelled-out one is not.
func TestMissingNumbers(t *testing.T) {
	if got := missingNumbers("Set it to 20 and 1,000 frames", "设成 20 和 1000 帧"); len(got) != 0 {
		t.Fatal(got)
	}
	if got := missingNumbers("divisible by three", "能被 3 整除"); len(got) != 0 {
		t.Fatal(got)
	}
	if got := missingNumbers("Set it to 20 and 1,000", "设成二十"); !slices.Equal(got, []string{"20", "1,000"}) {
		t.Fatal(got)
	}
}

// A copy of the source is caught in every target script, Korean included;
// a translation in the target's script, or a short name, is not.
func TestUntranslated(t *testing.T) {
	zh, ko := language{Code: "zh"}, language{Code: "ko"}
	if !untranslated("Select the inner faces", "Select the inner faces", zh) || !untranslated("Select the inner faces", "select the inner faces!", ko) {
		t.Fatal("copy passed")
	}
	if untranslated("Select the inner faces", "안쪽 면을 선택하세요", ko) || untranslated("Select the inner faces", "选择内部面", zh) || untranslated("Blender", "Blender", zh) {
		t.Fatal("translation flagged")
	}
	if !untranslated("Select the inner faces", "Choose the inner faces", zh) {
		t.Fatal("an answer in the wrong language passed")
	}
}
