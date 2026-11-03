package subtitle

import (
	"bufio"
	"encoding/json"
	"errors"
	"math"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// SherpaResult is one recognized file in the JSON that the pinned
// sherpa-onnx-offline CLI prints, one object per line, in input order.
// Timestamps are token starts in seconds; Durations exist only for
// transducer models such as Parakeet-TDT.
type SherpaResult struct {
	Lang       string    `json:"lang"`
	Event      string    `json:"event"`
	Text       string    `json:"text"`
	Timestamps []float64 `json:"timestamps"`
	Durations  []float64 `json:"durations"`
	Tokens     []string  `json:"tokens"`
}

// ParseSherpaResults reads the result lines of the CLI's output. Every
// result must carry one start time per token; a mismatch is a protocol
// fault, not a recognition with fewer words.
func ParseSherpaResults(output string) ([]SherpaResult, error) {
	results := []SherpaResult{}
	scanner := bufio.NewScanner(strings.NewReader(output))
	scanner.Buffer(make([]byte, 64*1024), 16*1024*1024)
	for scanner.Scan() {
		line := strings.TrimSpace(scanner.Text())
		if !strings.HasPrefix(line, "{") {
			continue
		}
		var r SherpaResult
		if err := json.Unmarshal([]byte(line), &r); err != nil {
			return nil, errors.New(locales.ErrSherpaOutput())
		}
		if len(r.Timestamps) != len(r.Tokens) || (len(r.Durations) > 0 && len(r.Durations) != len(r.Tokens)) {
			return nil, errors.New(locales.ErrSherpaOutput())
		}
		for _, t := range r.Timestamps {
			if t < 0 || math.IsNaN(t) || math.IsInf(t, 0) {
				return nil, errors.New(locales.ErrSherpaOutput())
			}
		}
		results = append(results, r)
	}
	if err := scanner.Err(); err != nil {
		return nil, err
	}
	return results, nil
}

// SpacedWord is a recognized word and whether a space comes before it in
// the text. Scripts written without spaces (Chinese, Japanese, and Korean
// syllables inside a word) join their words directly.
type SpacedWord struct {
	Word
	Space bool
}

// Token styles of the engines' vocabularies.
const (
	// SherpaPieces marks a word start with a leading space (SentencePiece:
	// SenseVoice, Parakeet-TDT).
	SherpaPieces = "pieces"
	// SherpaContinuation marks a word that goes on with a trailing "@@"
	// (Paraformer).
	SherpaContinuation = "continuation"
)

// maxTokenMs is the most a token without its own duration lasts. CTC and
// CIF timestamps give only where a token starts; it then lasts until the
// next one, but not across a pause.
const maxTokenMs = 600

// SherpaWords turns the tokens of one result into words on the clock of its
// input file, durationMs long. Punctuation joins the word before it (an
// opening mark joins the word after it), so the words cover the text with
// only spaces between them. A result whose text is only an audio event
// (BGM, laughter) has no tokens and gives no words.
func SherpaWords(r SherpaResult, style string, durationMs int64) []SpacedWord {
	words := []SpacedWord{}
	pendingSpace, prefix := false, ""
	for i, token := range r.Tokens {
		lead := strings.HasPrefix(token, " ") || strings.HasPrefix(token, "▁")
		body := strings.TrimSpace(strings.TrimPrefix(token, "▁"))
		if style == SherpaContinuation {
			body = strings.TrimSuffix(body, "@@")
		}
		if body == "" {
			pendingSpace = pendingSpace || lead
			continue
		}
		start := min(int64(math.Round(r.Timestamps[i]*1000)), durationMs)
		if punctuationOnly(body) {
			if len(words) > 0 && !opening(body) {
				words[len(words)-1].Text += body
			} else {
				prefix += body
			}
			continue
		}
		if len(words) > 0 && start < words[len(words)-1].StartMs {
			start = words[len(words)-1].StartMs
		}
		joins := false
		if n := len(words); n > 0 && prefix == "" {
			previous := words[n-1].Text
			last, _ := utf8.DecodeLastRuneInString(previous)
			first, _ := utf8.DecodeRuneInString(body)
			switch style {
			case SherpaContinuation:
				joins = strings.HasSuffix(r.Tokens[i-1], "@@")
			default:
				joins = !lead && !pendingSpace && !noSpaceScript(first) && !noSpaceScript(last) && !unicode.IsPunct(last)
			}
		}
		if joins {
			words[len(words)-1].Text += body
		} else {
			space := lead || pendingSpace
			if style == SherpaContinuation && len(words) > 0 {
				space = spaced(words[len(words)-1].Text, body)
			}
			words = append(words, SpacedWord{Word: Word{Text: prefix + body, StartMs: start}, Space: space && len(words) > 0})
			prefix = ""
		}
		pendingSpace = false
		end := start + maxTokenMs
		if len(r.Durations) > 0 {
			end = start + int64(math.Round(r.Durations[i]*1000))
		}
		words[len(words)-1].EndMs = min(end, durationMs)
	}
	if prefix != "" && len(words) > 0 {
		words[len(words)-1].Text += prefix
	}
	// A word lasts until the next starts at the latest. The timestamps are
	// in order, so this never makes words overlap.
	for i := range words {
		if i+1 < len(words) {
			words[i].EndMs = min(words[i].EndMs, words[i+1].StartMs)
		}
		words[i].EndMs = max(words[i].EndMs, words[i].StartMs)
	}
	return words
}

// PunctuateWords copies the punctuation of punctuated, the same words with
// marks added, onto words: each mark joins the word before it. It reports
// false, with words unchanged, when punctuated holds other changes.
func PunctuateWords(words []SpacedWord, punctuated string) ([]SpacedWord, bool) {
	out := append([]SpacedWord{}, words...)
	text := []rune(punctuated)
	at := 0
	marks := func() string {
		extra := ""
		for at < len(text) && (unicode.IsSpace(text[at]) || unicode.IsPunct(text[at])) {
			if !unicode.IsSpace(text[at]) {
				extra += string(text[at])
			}
			at++
		}
		return extra
	}
	for i := range out {
		if extra := marks(); extra != "" {
			if i == 0 {
				out[i].Text = extra + out[i].Text
			} else {
				out[i-1].Text += extra
			}
		}
		for _, r := range out[i].Text {
			if at >= len(text) || unicode.ToLower(text[at]) != unicode.ToLower(r) {
				if unicode.IsPunct(r) {
					continue // already in the word
				}
				return words, false
			}
			at++
		}
	}
	extra := marks()
	if at != len(text) {
		return words, false
	}
	if extra != "" && len(out) > 0 {
		out[len(out)-1].Text += extra
	}
	return out, true
}

// Cue boundaries of recognized words: a sentence ends at a strong mark, a
// pause of breakGapMs or more always ends a cue, and a cue never becomes
// too long to read (the readability limits) or lasts more than maxCueMs.
// An over-long cue ends at its last weak mark when it has one.
const (
	breakGapMs = 600
	maxCueMs   = 10000
	cuePadMs   = 100
)

// WordCues makes sentence cues of words, one cue per sentence, with the
// words as their timing evidence. Each cue ends cuePadMs after its last
// word, never after the next cue starts.
func WordCues(words []SpacedWord) []Cue {
	groups := [][]SpacedWord{}
	current := []SpacedWord{}
	text := func(ws []SpacedWord) string {
		var b strings.Builder
		for i, w := range ws {
			if i > 0 && w.Space {
				b.WriteByte(' ')
			}
			b.WriteString(w.Text)
		}
		return b.String()
	}
	flush := func() {
		if len(current) > 0 {
			groups = append(groups, current)
			current = nil
		}
	}
	for _, w := range words {
		if n := len(current); n > 0 && sentenceEnd(current[n-1].Text, &w) {
			flush()
		}
		if n := len(current); n > 0 {
			if w.StartMs-current[n-1].EndMs >= breakGapMs {
				flush()
			} else {
				candidate := append(append([]SpacedWord{}, current...), w)
				if w.EndMs-current[0].StartMs > maxCueMs || InspectReading(text(candidate), w.EndMs-current[0].StartMs).TooLong {
					cut := n
					for j := n - 1; j > 0; j-- {
						if endsWith(current[j-1].Text, weakMarks) {
							cut = j
							break
						}
					}
					tail := append([]SpacedWord{}, current[cut:]...)
					current = current[:cut]
					flush()
					current = tail
				}
			}
		}
		current = append(current, w)
	}
	flush()
	cues := []Cue{}
	for i, g := range groups {
		end := g[len(g)-1].EndMs + cuePadMs
		if i+1 < len(groups) {
			end = min(end, groups[i+1][0].StartMs)
		}
		end = max(end, g[len(g)-1].EndMs)
		ws := []Word{}
		for _, w := range g {
			ws = append(ws, w.Word)
		}
		t := text(g)
		cues = append(cues, Cue{Position: i, StartMs: g[0].StartMs, EndMs: end, Text: t, OriginalText: t, Words: ws})
	}
	return cues
}

const (
	strongMarks = "。！？!?…"
	weakMarks   = "，、,：:；;"
)

// endsWith reports whether text ends with one of marks, ignoring closing
// quotes and brackets after it.
func endsWith(text, marks string) bool {
	last, _ := utf8.DecodeLastRuneInString(trimClosing(text))
	return last != utf8.RuneError && strings.ContainsRune(marks, last)
}

// sentenceEnd reports whether a sentence ends after the word text, before
// next. A period does not end one inside a number (3.5, the next word goes
// on without a space) or after a letter abbreviation (U.S.).
func sentenceEnd(text string, next *SpacedWord) bool {
	if endsWith(text, strongMarks) {
		return true
	}
	before, found := strings.CutSuffix(trimClosing(text), ".")
	if !found || before == "" || strings.HasSuffix(before, ".") {
		return false
	}
	if next != nil && !next.Space {
		if first, _ := utf8.DecodeRuneInString(next.Text); unicode.IsDigit(first) {
			return false
		}
	}
	runes := []rune(before)
	return !(len(runes) >= 2 && runes[len(runes)-2] == '.' && unicode.IsLetter(runes[len(runes)-1]))
}

// trimClosing drops the closing quotes and brackets that end text.
func trimClosing(text string) string {
	return strings.TrimRightFunc(text, func(r rune) bool { return unicode.In(r, unicode.Pe, unicode.Pf) || r == '"' || r == '\'' })
}

func punctuationOnly(s string) bool {
	for _, r := range s {
		if !unicode.IsPunct(r) && !unicode.IsSymbol(r) {
			return false
		}
	}
	return true
}

func opening(s string) bool {
	r, _ := utf8.DecodeRuneInString(s)
	return unicode.In(r, unicode.Ps, unicode.Pi) || r == '¿' || r == '¡'
}

// BoundarySpace reports whether a space joins two words recognized in
// separate inputs: always, unless a side is Chinese or Japanese text or a
// full-width mark.
func BoundarySpace(before, next string) bool {
	last, _ := utf8.DecodeLastRuneInString(before)
	first, _ := utf8.DecodeRuneInString(next)
	for _, r := range []rune{last, first} {
		if unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana) || (r >= 0x3000 && r <= 0x303F) || (r >= 0xFF00 && r <= 0xFFEF) {
			return false
		}
	}
	return true
}

// noSpaceScript reports runes of scripts written without spaces between
// words: each such token is a word of its own.
func noSpaceScript(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul)
}
