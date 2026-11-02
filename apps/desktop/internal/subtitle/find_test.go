package subtitle

import "testing"

func TestLiteralFindReplaceProtectsTagsAndUnicodeOffsets(t *testing.T) {
	for _, tt := range []struct {
		text, query, replacement, want string
		sensitive                      bool
		count                          int
	}{
		{"One one ONE", "one", "two", "two two two", false, 3},
		{"One one ONE", "one", "two", "One two ONE", true, 1},
		{"你好你好", "你好", "好", "好好", false, 2},
		{"Σ σ ς", "σ", "x", "x x x", false, 3},
		{"<i title=\"One\">One</i> One", "one", "two", "<i title=\"One\">two</i> two", false, 2},
		{`{\fs12.5}12.5\N12.5`, "12.5", "13", `{\fs12.5}13\N13`, false, 2},
		{"a.*b aZZb", ".*", "$1", "a$1b aZZb", false, 1},
		{"hello hello", "hello", "", " ", false, 2},
		{"hello", "", "x", "hello", false, 0},
	} {
		finder := NewLiteralFinder(tt.query, tt.sensitive)
		got, count := finder.ReplaceAll(tt.text, tt.replacement)
		if got != tt.want || count != tt.count {
			t.Fatalf("%q: %q/%d want %q/%d", tt.text, got, count, tt.want, tt.count)
		}
	}
}
