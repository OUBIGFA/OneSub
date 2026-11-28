package translator

import (
	"errors"
	"fmt"
	"regexp"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

func validateResume(doc subtitle.Document, resume *ResumeState) error {
	candidate := resume.Units
	candidate.Cues = append([]subtitle.Cue{}, candidate.Cues...)
	for i := range candidate.Cues {
		cue := &candidate.Cues[i]
		if !cue.ReadOnly && !cue.TextLocked {
			cue.TranslationSource = subtitle.SourceText(*cue)
			cue.Translated = true
		}
	}
	if err := subtitle.ValidateTranslation(doc, candidate); err != nil {
		return err
	}
	for i, text := range resume.Completed {
		if i < 0 || i >= len(resume.Units.Cues) || strings.TrimSpace(text) == "" {
			return errors.New(locales.TranslationResumeStale())
		}
		cue := resume.Units.Cues[i]
		if cue.ReadOnly || cue.TextLocked || !sameMarkup(cue.Text, text) {
			return errors.New(locales.TranslationResumeStale())
		}
	}
	return nil
}

func sameMarkup(source, translation string) bool {
	a, b := markupTags.FindAllString(source, -1), markupTags.FindAllString(translation, -1)
	if len(a) != len(b) {
		return false
	}
	for i := range a {
		if a[i] != b[i] {
			return false
		}
	}
	return true
}

// untranslated catches an answer that copied its source, the failure
// SmartSub's untranslated check exists for: the same letters, or none of
// the target's script at all where the source has none either. A name or a
// code alone is short and passes.
func untranslated(source, translation string, l language) bool {
	letters := func(s string) string {
		var b strings.Builder
		for _, r := range strings.ToLower(s) {
			if unicode.IsLetter(r) || unicode.IsDigit(r) {
				b.WriteRune(r)
			}
		}
		return b.String()
	}
	a := letters(source)
	if len([]rune(a)) < 8 {
		return false
	}
	if a == letters(translation) {
		return true
	}
	script := targetScript(l)
	return script != nil && !strings.ContainsFunc(source, script) && !strings.ContainsFunc(translation, script)
}

// targetScript is the script a translation into l is written in, where
// it differs from Latin: Han for Chinese, kana or Han for Japanese, Hangul
// for Korean.
func targetScript(l language) func(rune) bool {
	switch {
	case l.chinese():
		return func(r rune) bool { return unicode.Is(unicode.Han, r) }
	case l.Code == "ja":
		return func(r rune) bool {
			return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana)
		}
	case l.Code == "ko":
		return func(r rune) bool { return unicode.Is(unicode.Hangul, r) }
	}
	return nil
}

// sentenceEnd reports whether a subtitle closes its sentence: its text,
// without closing tags, quotes or brackets, ends in a full stop, a question
// or exclamation mark, or an ellipsis.
func sentenceEnd(text string) bool {
	text = strings.TrimRightFunc(trailingTags.ReplaceAllString(strings.TrimSpace(text), ""), func(r rune) bool {
		return unicode.IsSpace(r) || strings.ContainsRune("\"'”’»」』）)]", r)
	})
	if text == "" {
		return false
	}
	last, _ := utf8.DecodeLastRuneInString(text)
	return strings.ContainsRune(".?!。？！…", last)
}

// isCJK is the skill checker's CJK_RE: ideographs and kana.
func isCJK(r rune) bool {
	return (r >= 0x3400 && r <= 0x4DBF) || (r >= 0x4E00 && r <= 0x9FFF) || (r >= 0xF900 && r <= 0xFAFF) || (r >= 0x3040 && r <= 0x30FF)
}

var numbers = regexp.MustCompile(`\d+(?:[.,]\d+)*`)

// missingNumbers lists the numbers of the source the translation lacks:
// dropped payload. Numbers the translation adds, as a spelled-out "three"
// written 3, are what the skill asks for.
func missingNumbers(source, translation string) []string {
	count := map[string]int{}
	for _, n := range numbers.FindAllString(translation, -1) {
		count[strings.ReplaceAll(n, ",", "")]++
	}
	missing := []string{}
	for _, n := range numbers.FindAllString(source, -1) {
		key := strings.ReplaceAll(n, ",", "")
		if count[key] > 0 {
			count[key]--
		} else {
			missing = append(missing, n)
		}
	}
	return missing
}

// stampMs writes a time as 00:01:02.
func stampMs(ms int64) string {
	s := ms / 1000
	return fmt.Sprintf("%02d:%02d:%02d", s/3600, s/60%60, s%60)
}
