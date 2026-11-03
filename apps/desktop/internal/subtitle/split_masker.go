package subtitle

import (
	"strings"
	"unicode"
	"unicode/utf8"
)

var splitPairs = map[rune]rune{
	'“': '”', '‘': '’', '"': '"', '\'': '\'',
	'（': '）', '(': ')', '【': '】', '[': ']', '《': '》', '〈': '〉',
	'「': '」', '『': '』', '〖': '〗', '〔': '〕', '{': '}', '｛': '｝',
	'«': '»', '‹': '›',
}

// protectSplitPairs resolves complete pairs before masking. A genuine long
// quote may contain sentences; those sentences must not falsely "time out" a
// later real closing quote. Unmatched ASR marks never lock the remaining cue.
// Long containers release their interior to the same lexical/DP pipeline;
// no opening or closing symbols are invented on the resulting pieces.
func protectSplitPairs(runes []rune, units []float64, literals, blocked []bool, limit float64) {
	type opener struct {
		index int
		close rune
	}
	var stack []opener
	for i, r := range runes {
		if literals[i] {
			continue
		}
		// Apostrophes in contractions, possessives and surnames are not
		// quote delimiters. Unicode word protection keeps the word intact.
		if (r == '\'' || r == '’') && i > 0 && i+1 < len(runes) && (unicode.IsLetter(runes[i-1]) || unicode.IsDigit(runes[i-1])) && (unicode.IsLetter(runes[i+1]) || unicode.IsDigit(runes[i+1])) {
			continue
		}
		match := -1
		for j := len(stack) - 1; j >= 0; j-- {
			if stack[j].close == r {
				match = j
				break
			}
		}
		if match >= 0 {
			open := stack[match]
			stack = stack[:match] // unmatched inner marks are ASR artifacts
			blocked[i] = true     // never strand a closing mark on the next cue
			if units[i+1]-units[open.index] <= limit {
				for j := open.index + 1; j <= i; j++ {
					blocked[j] = true
				}
			}
			continue
		}
		if close, ok := splitPairs[r]; ok {
			// Inch/foot notation and trailing plural apostrophes do not
			// open a quote. A matched quote above still closes normally.
			if (r == '\'' || r == '"') && i > 0 && (unicode.IsDigit(runes[i-1]) || r == '\'' && unicode.IsLetter(runes[i-1])) {
				continue
			}
			stack = append(stack, opener{i, close})
			blocked[i+1] = true // even an unmatched opener cannot end a piece
		}
	}
}

func lineStartPunctuation(r rune) bool {
	return unicode.In(r, unicode.Pe, unicode.Pf) || strings.ContainsRune("，。！？；：、,.!?;:…،؛؟", r)
}

var chineseClauseHeads = strings.Fields("但是 然而 虽然 雖然 因为 因為 所以 因此 而且 并且 並且 不过 不過 尽管 儘管 否则 否則 和")
var chinesePhraseHeads = strings.Fields("关于 關於 对于 對於 为了 為了 通过 通過 根据 根據 按照 至于 至於")
var chineseSubjects = strings.Fields("我们 我們 你们 你們 他们 他們 这次 這次 下一个 下一個")
var chineseParticles = "的地得着著了过過"

// The Unicode word path also handles Cyrillic, Arabic and other spaced
// scripts. Function words are kept with their following phrase; no interface
// language or recognition setting is used to pretend the text is English.
var noSplitAfterWords = strings.Fields("a an the and but because although however in on at with for of to from by mr mrs ms dr prof de du des le la les un une et à en el los las del al y con para por der die das ein eine und mit zu von im am il lo gli per di da i o os as dos em com do в во на с со из к ко по для и")
var spacedClauseHeads = strings.Fields("but because although however therefore and mais parce pourtant cependant aber weil obwohl jedoch porque aunque pero porém porque но потому однако хотя")

func hasPhrasePrefix(text string, phrases []string) bool {
	for _, phrase := range phrases {
		if strings.HasPrefix(text, phrase) {
			return true
		}
	}
	return false
}

func lastSpacedWord(text string) string {
	text = strings.TrimSpace(text)
	text = strings.TrimRightFunc(text, unicode.IsPunct)
	if index := strings.LastIndexFunc(text, func(r rune) bool {
		return unicode.IsSpace(r) || unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul, unicode.Thai)
	}); index >= 0 {
		_, size := utf8.DecodeRuneInString(text[index:])
		text = text[index+size:]
	}
	return strings.ToLower(text)
}
