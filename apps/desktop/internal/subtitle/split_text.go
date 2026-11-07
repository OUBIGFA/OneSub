package subtitle

import (
	"slices"
	"strings"
	"sync"
	"unicode"

	"github.com/go-text/typesetting/segmenter"
	"golang.org/x/text/width"
)

type splitLanguage uint8

const (
	splitChinese splitLanguage = iota
	splitJapanese
	splitKorean
	splitThai
	splitWords
)

// Routes are inferred from the actual lane, never the UI/ASR language. All
// spaced scripts use UAX #29 words; Korean keeps whole eojeol, not Hangul
// syllables. Japanese/Thai use their own trained phrase models, not Chinese.
func splitLanguageOf(text string) splitLanguage {
	han, kana, hangul, thai := 0, 0, 0, 0
	for _, r := range text {
		switch {
		case unicode.In(r, unicode.Hiragana, unicode.Katakana):
			kana++
		case unicode.Is(unicode.Hangul, r):
			hangul++
		case unicode.Is(unicode.Han, r):
			han++
		case unicode.Is(unicode.Thai, r):
			thai++
		}
	}
	switch {
	case kana > 0 && kana+han >= max(hangul, thai):
		return splitJapanese
	case hangul > 0 && hangul >= max(han, thai):
		return splitKorean
	case han > 0 && han >= thai:
		return splitChinese
	case thai > 0:
		return splitThai
	default:
		return splitWords
	}
}

func nativeSplitRune(language splitLanguage, r rune) bool {
	switch language {
	case splitChinese:
		return unicode.Is(unicode.Han, r)
	case splitJapanese:
		return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana)
	case splitThai:
		return unicode.Is(unicode.Thai, r)
	}
	return false
}

type textBoundary struct {
	offset, runeIndex, characters, words int
	units, penalty                       float64
	clause                               bool // linguistic clause evidence, separate from model/length costs
}

type splitText struct {
	text                  string
	boundaries            []textBoundary
	units, limit, ceiling float64 // preferred line width and compact-utterance ceiling are distinct
	language              splitLanguage
	clauseOnly            bool // readable primary text needs a clause or measured pause, not a forced midpoint
}

type splitAnalysis struct {
	seg                        segmenter.Segmenter
	runes                      []rune
	bytes, characters, wordEnd []int
	wordCount, score, next     []int
	units                      []float64
	blocked, literals          []bool
	phraseCosts                []float64
}

var splitAnalysisPool = sync.Pool{New: func() any { return &splitAnalysis{} }}

// Only plain content can be distributed onto new event clocks. Real and ASS
// newlines remain simultaneous lanes. A literal '&', brace or Windows path
// is not formatting; actual tags, entities and ASS timing remain protected.
func splitLanes(text, format string) ([]splitText, string) {
	if readingMarkup.MatchString(text) || strings.ContainsAny(text, "<>") || splitEntity.MatchString(text) {
		return nil, "protected"
	}
	if format == "ass" || format == "ssa" {
		text = strings.NewReplacer(`\N`, "\n", `\n`, "\n").Replace(text)
		if strings.Contains(text, `\`) {
			return nil, "protected"
		}
	}
	texts := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	lanes := make([]splitText, 0, len(texts))
	for _, lane := range texts {
		if strings.TrimSpace(lane) == "" {
			return nil, "boundary"
		}
		lanes = append(lanes, newSplitText(lane))
	}
	return lanes, ""
}

func splitClusterUnits(g []rune, cjk bool) float64 {
	if cjk && (unicode.IsPunct(g[0]) || unicode.IsSpace(g[0])) {
		kind := width.LookupRune(g[0]).Kind()
		if kind == width.EastAsianWide || kind == width.EastAsianFullwidth || kind == width.EastAsianAmbiguous {
			return 1
		}
		return 0.5
	}
	return clusterUnits(g, cjk)
}

func newSplitText(text string) splitText {
	language := splitLanguageOf(text)
	cjk, _, _ := readingProfile(text)
	// Space-separated scripts aim for a line and a half: 56 columns preferred,
	// 64 at most. One 42-column line cut English into fragments; two full
	// lines (84) left cues too long to read and to translate.
	limit, ceiling := 56.0, 64.0
	if language == splitChinese || language == splitJapanese || language == splitKorean {
		limit, ceiling = 16, 20
	}
	out := splitText{text: text, limit: limit, ceiling: ceiling, language: language}
	ws := splitAnalysisPool.Get().(*splitAnalysis)
	defer splitAnalysisPool.Put(ws)
	ws.runes, ws.bytes = ws.runes[:0], ws.bytes[:0]
	for offset, r := range text {
		ws.runes, ws.bytes = append(ws.runes, r), append(ws.bytes, offset)
	}
	ws.bytes = append(ws.bytes, len(text))
	n := len(ws.runes)
	ws.units = slices.Grow(ws.units[:0], n+1)[:n+1]
	ws.characters = slices.Grow(ws.characters[:0], n+1)[:n+1]
	ws.wordEnd = slices.Grow(ws.wordEnd[:0], n+1)[:n+1]
	ws.wordCount = slices.Grow(ws.wordCount[:0], n+1)[:n+1]
	ws.score = slices.Grow(ws.score[:0], n+1)[:n+1]
	ws.next = slices.Grow(ws.next[:0], n+1)[:n+1]
	ws.blocked = slices.Grow(ws.blocked[:0], n+1)[:n+1]
	ws.literals = slices.Grow(ws.literals[:0], n)[:n]
	ws.phraseCosts = slices.Grow(ws.phraseCosts[:0], n+1)[:n+1]
	clear(ws.units)
	clear(ws.characters)
	clear(ws.wordEnd)
	clear(ws.wordCount)
	clear(ws.blocked)
	clear(ws.literals)
	clear(ws.phraseCosts)

	ws.seg.Init(ws.runes)
	it := ws.seg.GraphemeIterator()
	characters := 0
	for it.Next() {
		g := it.Grapheme()
		out.units += splitClusterUnits(g.Text, cjk)
		r := g.Text[0]
		if !unicode.IsSpace(r) && !unicode.IsPunct(r) && !unicode.IsControl(r) && !unicode.Is(unicode.Cf, r) {
			characters++
		}
		for j := g.Offset + 1; j <= g.Offset+len(g.Text); j++ {
			ws.units[j], ws.characters[j] = out.units, characters
		}
	}

	if language == splitChinese {
		splitLexicon().segment(text, ws.runes, ws.bytes, ws.score, ws.next, ws.blocked, ws.wordEnd)
	} else if language == splitJapanese || language == splitThai {
		var model phraseModel
		if language == splitJapanese {
			model = japanesePhrases()
		} else {
			model = thaiPhrases()
		}
		for i := 1; i <= n; i++ {
			if !nativeSplitRune(language, ws.runes[i-1]) {
				continue
			}
			if i == n || !nativeSplitRune(language, ws.runes[i]) || model.boundary(text, ws.bytes, i) {
				ws.wordEnd[i]++
			} else {
				ws.blocked[i] = true
			}
		}
	}
	words := ws.seg.WordIterator()
	nameEnd := -1
	for words.Next() {
		w := words.Word()
		end, native := w.Offset+len(w.Text), false
		for _, r := range w.Text {
			native = native || nativeSplitRune(language, r)
		}
		if !native {
			ws.wordEnd[end]++
		}
		for i := w.Offset + 1; i < end; i++ {
			if !nativeSplitRune(language, ws.runes[i-1]) || !nativeSplitRune(language, ws.runes[i]) {
				ws.blocked[i] = true
			}
		}
		if language == splitChinese && !native && mixedLatinNameWord(w.Text) {
			if nameEnd >= 0 {
				joined := true
				for _, r := range ws.runes[nameEnd:w.Offset] {
					joined = joined && unicode.IsSpace(r)
				}
				if joined {
					for cut := nameEnd; cut <= w.Offset; cut++ {
						ws.blocked[cut] = true // Kylian / Mbappé is not a phrase boundary
					}
				}
			}
			nameEnd = end
		} else {
			nameEnd = -1
		}
	}
	if language == splitChinese {
		protectChineseConnections(text, ws)
		scoreChinesePhrases(text, ws)
	} else if language == splitWords {
		protectEnglishPhrases(text, ws)
	}
	for i := 1; i <= n; i++ {
		ws.wordCount[i] = ws.wordCount[i-1] + ws.wordEnd[i]
	}

	spans := literalSpans(text)
	span := 0
	for i, offset := range ws.bytes[:n] {
		for span < len(spans) && offset >= spans[span].end {
			span++
		}
		if span < len(spans) && offset >= spans[span].start {
			ws.literals[i] = true
			if offset > spans[span].start {
				ws.blocked[i] = true
			}
		}
		if i > 0 && (ws.runes[i] == '…' && ws.runes[i-1] == '…' || ws.runes[i] == '—' && ws.runes[i-1] == '—' || ws.runes[i] == '.' && ws.runes[i-1] == '.') {
			ws.blocked[i] = true
		}
	}
	protectSplitPairs(ws.runes, ws.units, ws.literals, ws.blocked, limit)

	out.boundaries = make([]textBoundary, 1, n+1)
	it = ws.seg.GraphemeIterator()
	for it.Next() {
		g := it.Grapheme()
		i := g.Offset + len(g.Text)
		if i == n {
			break
		}
		right := i
		for right < n && unicode.IsSpace(ws.runes[right]) {
			right++
		}
		if ws.blocked[i] || right == n || lineStartPunctuation(ws.runes[right]) || right > i && ws.blocked[right] && (ws.runes[right] == '"' || ws.runes[right] == '\'') || unicode.In(ws.runes[i-1], unicode.Ps, unicode.Pi) {
			continue
		}
		penalty, allow := splitBoundaryPenalty(text, i, ws, language)
		if allow {
			clause := penalty < 0
			if !clause {
				penalty += ws.phraseCosts[i]
			}
			out.boundaries = append(out.boundaries, textBoundary{ws.bytes[i], i, ws.characters[i], ws.wordCount[i], ws.units[i], penalty, clause})
		}
	}
	out.boundaries = append(out.boundaries, textBoundary{len(text), n, characters, ws.wordCount[n], out.units, 0, true})
	return out
}

func splitBoundaryPenalty(text string, index int, ws *splitAnalysis, language splitLanguage) (float64, bool) {
	left := index - 1
	for left >= 0 && (unicode.IsSpace(ws.runes[left]) || closingPunctuation(ws.runes[left])) {
		left--
	}
	if left < 0 {
		return 0, false
	}
	r := ws.runes[left]
	switch {
	case strings.ContainsRune("。！？!?؟…", r) || sentencePeriod(ws.runes, left, ws.literals) || r == '.' && left > 0 && ws.runes[left-1] == '.':
		return -60, true
	case strings.ContainsRune("，；：、,;:،؛—", r) && !ws.literals[left]:
		if language == splitChinese || language == splitJapanese || language == splitThai {
			if ws.characters[index] < 4 || ws.characters[len(ws.runes)]-ws.characters[index] < 4 {
				return 0, false // weak punctuation must not emit an edge island
			}
		}
		if strings.ContainsRune("；;؛—", r) {
			return -30, true
		}
		return -15, true
	}

	right := strings.TrimLeftFunc(text[ws.bytes[index]:], unicode.IsSpace)
	if slices.Contains(noSplitAfterWords, lastSpacedWord(text[:ws.bytes[index]])) {
		return 0, false
	}
	if language == splitChinese {
		// Grammar is a whole-token hard mask, including surrounding spaces;
		// scoring cannot reintroduce it or reject 调和 merely for ending in 和.
		if hasPhrasePrefix(right, chineseClauseHeads) {
			return -5, true
		}
		if hasPhrasePrefix(right, chinesePhraseHeads) {
			return 0, true
		}
		if hasPhrasePrefix(right, chineseSubjects) {
			return 5, true
		}
	} else if language == splitWords || language == splitKorean {
		word, _, _ := strings.Cut(right, " ")
		if slices.Contains(spacedClauseHeads, strings.ToLower(word)) {
			return -5, true
		}
	}
	return 10, true
}
