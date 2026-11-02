package subtitle

import "testing"

func TestPunctuationOptions(t *testing.T) {
	for _, tc := range []struct {
		text, want string
		options    PunctuationOptions
	}{
		{"First. Next。 End．", "First, Next， End．", PunctuationOptions{ReplacePeriods: true}},
		{"First. Next。 End．", "First, Next， End", PunctuationOptions{ReplacePeriods: true, StripTrailing: true}},
		{"Really? 真的吗？", "Really? 真的吗？", PunctuationOptions{StripTrailing: true, Keep: "?"}},
		{"First. Next.", "First. Next.", PunctuationOptions{}},
		{"3.14。然后。", "3,14，然后。", PunctuationOptions{ReplacePeriods: true}},
		{"<i>First. Next.</i>", "<i>First, Next</i>", PunctuationOptions{ReplacePeriods: true, StripTrailing: true}},
		{`{\fs12.5}First. Next.\NSecond。Next。`, `{\fs12.5}First, Next\NSecond，Next`, PunctuationOptions{ReplacePeriods: true, StripTrailing: true}},
		{`<a href="https://example.test">First. Next.</a>`, `<a href="https://example.test">First, Next.</a>`, PunctuationOptions{ReplacePeriods: true}},
		{"他说“好的。”", "他说“好的。”", PunctuationOptions{ReplacePeriods: true}},
		{"他说“好的。”", "他说“好的”", PunctuationOptions{StripTrailing: true}},
		{"...", "...", PunctuationOptions{ReplacePeriods: true, StripTrailing: true}},
		{"Done.\r\nNext。", "Done\nNext", PunctuationOptions{StripTrailing: true}},
	} {
		if got := tc.options.Apply(tc.text); got != tc.want {
			t.Errorf("%q => %q, want %q", tc.text, got, tc.want)
		}
	}
}
