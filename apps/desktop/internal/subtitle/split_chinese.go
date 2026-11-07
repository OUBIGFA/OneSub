package subtitle

import (
	"math"
	"strings"
	"unicode"
	"unicode/utf8"
)

// Chinese variants are scored in their original spelling. Taking the larger
// of the two official margins avoids converting user text or guessing script
// from a handful of shared characters. The lexicon/grammar hard mask still
// wins over both models. Weak learned breaks cost more than strong ones.
func scoreChinesePhrases(text string, ws *splitAnalysis) {
	hans, hant := simplifiedChinesePhrases(), traditionalChinesePhrases()
	lexicon := splitLexicon()
	for i := 1; i < len(ws.runes); i++ {
		if ws.blocked[i] || !unicode.Is(unicode.Han, ws.runes[i-1]) || !unicode.Is(unicode.Han, ws.runes[i]) {
			continue
		}
		margin := max(hans.score(text, ws.bytes, i), hant.score(text, ws.bytes, i))
		ws.phraseCosts[i] = 24 - math.Max(-24, math.Min(24, float64(margin)/100))
	}
	previous, previousEnd, previousClass := 0, 0, uint32(0)
	for start := 0; start < len(ws.runes); {
		if !unicode.Is(unicode.Han, ws.runes[start]) {
			previousClass = 0
			start++
			continue
		}
		end := ws.next[start]
		class := lexicon.lexicalClass(text[ws.bytes[start]:ws.bytes[end]])
		if start == previousEnd {
			if (previousClass == 1 || previousClass == 2 || previousClass == 4) && class == 1 || previousClass == 1 && class == 3 && end-previous <= 4 || previousClass == 4 && (class == 2 || class == 3) {
				ws.phraseCosts[start] += 45
			}
			// An unknown single Han character is weak evidence for a complete
			// subtitle phrase; prefer a longer learned/dictionary boundary.
			if class == 0 && end-start == 1 || previousClass == 0 && start-previous == 1 {
				ws.phraseCosts[start] += 20
			}
		}
		previous, previousEnd, previousClass, start = start, end, class, end
	}
}

// These grammatical classes bind to the following word. They are tested on
// the DAG's selected whole tokens, not arbitrary suffixes: 就职/对方/是非 must
// not acquire the restrictions of 就/对/是. Punctuation ends the connection.
var chineseForwardWords = func() map[string]bool {
	groups := []string{
		// Copulas, auxiliaries and negative forms retain their predicate.
		"是 不是 就是 只是 并不是 並不是 并非 並非 是否 有 没有 沒有 并没有 並沒有 有没有 有沒有",
		"能 能够 能夠 可以 可不可以 会 會 不会 不會 要 不要 应 應 应该 應該 不该 不該 必须 必須 需 需要 得 想 愿 願 愿意 願意 继续 繼續 持续 持續 开始 開始",
		"不 没 沒 未 非 别 別 莫 无 無 并不 並不 从不 從不 从未 從未 尚未 不能 不必 不用 无需 無需 不得 不可 不肯 不愿 不願",
		// Coverbial/prepositional heads retain their object, including 把/被.
		"把 被 将 將 让 讓 给 給 向 对 對 从 從 于 於 在 到 为 為 跟 与 與",
		// Degree, scope, time and manner modifiers retain the next predicate.
		"就 才 都 也 又 再 还 還 更 最 很 太 真 只 仅 僅 仍 仍然 正 正在 已 已经 已經 曾 曾经 曾經 将要 將要 即将 即將",
		"几乎 幾乎 立即 马上 馬上 立刻 始终 始終 一直 总是 總是 往往 通常 经常 經常 从来 從來 终于 終於 已然 尚且 依然 依旧 依舊",
		"非常 十分 特别 特別 尤其 极 極 极其 極其 极为 極為 极具 極具 具 较 較 比较 比較 相当 相當 更加 最为 最為 过于 過於 稍微 有点 有點",
		"具体 具體 详细 詳細 仔细 仔細 认真 認真 清楚 完整 全面 直接 公开 公開 充分 彻底 徹底 共同 一起 互相 逐步 逐渐 逐漸",
		// Personal subjects and clause connectors do not hang before a predicate.
		"我 你 您 他 她 它 我们 我們 你们 你們 他们 他們 她们 她們 它们 它們 咱们 咱們 大家 自己",
		"接下来 接下來",
		// Nominal determiners and modifier particles retain their head.
		"个 個 所有 全部 一些 这些 這些 那些 哪些 每个 每個 各个 各個 的 地",
	}
	words := make(map[string]bool)
	for _, group := range groups {
		for _, word := range strings.Fields(group) {
			words[word] = true
		}
	}
	// Scoring and hard protection use the same connective/phrase heads.
	for _, heads := range [][]string{chineseClauseHeads, chinesePhraseHeads} {
		for _, word := range heads {
			words[word] = true
		}
	}
	return words
}()

const chineseQuantifierPrefixes = "零〇一二两兩三四五六七八九十百千万萬亿億几幾数數这這那哪每各某此诸諸"
const chineseNominalClassifiers = "个個位名项項件条條份种種本只支把张張篇座头頭套组組部台辆輛双雙门門场場些"
const chineseNominalSuffixes = "性化者员員力度率型类類式感"

func chineseQuantifierPrefix(word string) bool {
	if word == "" {
		return false
	}
	for _, r := range word {
		if !strings.ContainsRune(chineseQuantifierPrefixes, r) && !unicode.IsDigit(r) {
			return false
		}
	}
	return true
}

func chineseBindsForward(word, previous string) bool {
	if chineseForwardWords[word] {
		return true
	}
	// A-not-A questions are productive, not separate full-sentence entries.
	for _, negative := range []string{"不", "没", "沒"} {
		if left, right, ok := strings.Cut(word, negative); ok && left != "" && left == right && chineseForwardWords[left] {
			return true
		}
	}
	// Keep 数词/指示词 + 量词 + 名词 together whether the dictionary chose
	// 三本 as one token or 三/本 as two. Ordinary words such as 本领 are not
	// classifiers, and completed counts such as 三次 can end an adverbial.
	last, size := utf8.DecodeLastRuneInString(word)
	if strings.ContainsRune(chineseNominalClassifiers, last) {
		prefix := word[:len(word)-size]
		return chineseQuantifierPrefix(prefix) || prefix == "" && chineseQuantifierPrefix(previous)
	}
	return false
}

// A left-binding token includes padding and a preceding closed name/quote.
// Stop at real punctuation rather than silently crossing a sentence boundary.
func protectChineseLeft(ws *splitAnalysis, start int) {
	left := start
	for left > 0 && unicode.IsSpace(ws.runes[left-1]) {
		left--
	}
	if left == 0 {
		return
	}
	previous := ws.runes[left-1]
	if unicode.IsLetter(previous) || unicode.IsDigit(previous) || unicode.IsMark(previous) || closingPunctuation(previous) {
		for cut := left; cut <= start; cut++ {
			ws.blocked[cut] = true
		}
	}
}

// Protect connections before candidate scoring so neither length balancing,
// ASR gaps nor bilingual projection can override grammar. Word counts retain
// their original lexical meaning; this pass only removes unsafe cut positions.
func protectChineseConnections(text string, ws *splitAnalysis) {
	previous := ""
	for start := 0; start < len(ws.runes); {
		chinese := unicode.Is(unicode.Han, ws.runes[start])
		end := start + 1
		switch {
		case chinese:
			end = ws.next[start]
		case unicode.IsDigit(ws.runes[start]):
			for end < len(ws.runes) && unicode.IsDigit(ws.runes[end]) {
				end++
			}
		default:
			if !unicode.IsSpace(ws.runes[start]) {
				previous = ""
			}
			start++
			continue
		}
		word := text[ws.bytes[start]:ws.bytes[end]]
		if chinese && end == start+1 && (strings.ContainsRune(chineseParticles, ws.runes[start]) || previous != "" && strings.ContainsRune(chineseNominalSuffixes, ws.runes[start])) {
			protectChineseLeft(ws, start)
		}
		// A nominal locative belongs to its place/time noun: 环境中/系统内.
		// Match a whole one-character token and a dictionary noun, not the
		// first character of 中心 or the last character of an unrelated word.
		if chinese && end == start+1 && strings.ContainsRune("中内內外上下前后後", ws.runes[start]) && splitLexicon().lexicalClass(previous) == 1 {
			protectChineseLeft(ws, start)
		}
		right := end
		for right < len(ws.runes) && unicode.IsSpace(ws.runes[right]) {
			right++
		}
		bind := false
		if right < len(ws.runes) {
			_, opener := splitPairs[ws.runes[right]]
			if unicode.IsLetter(ws.runes[right]) || unicode.IsDigit(ws.runes[right]) || opener {
				bind = chineseBindsForward(word, previous)
				bind = bind || chineseQuantifierPrefix(word) && strings.ContainsRune(chineseNominalClassifiers, ws.runes[right])
				bind = bind || chinese && end == start+1 && right == end && ws.runes[start] == ws.runes[right] // AA verb reduplication: 讲讲/看看
			}
		}
		if bind {
			for cut := end; cut <= right; cut++ {
				ws.blocked[cut] = true // spaces must not reopen the same connection
			}
		}
		previous, start = word, end
	}
}

// Unicode whole-word evidence identifies a conservative title-cased Latin
// name run in a Chinese lane. This is not a general named-entity recognizer;
// restricting it to mixed Chinese avoids joining an English title wholesale.
func mixedLatinNameWord(word []rune) bool {
	if !unicode.IsUpper(word[0]) && !unicode.IsTitle(word[0]) {
		return false
	}
	for _, r := range word {
		if !unicode.Is(unicode.Latin, r) && !unicode.IsMark(r) {
			return false
		}
	}
	return true
}
