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

// SherpaCue preserves one engine result as one original subtitle. Text comes
// from the engine, not reconstructed tokens. Word times bound the cue but never
// split it by punctuation, pauses, reading length or duration. endMs is the
// input region's end on the same clock as words.
func SherpaCue(text string, words []SpacedWord, endMs int64) Cue {
	cue := Cue{Text: text, OriginalText: text}
	if len(words) == 0 {
		return cue
	}
	cue.StartMs = words[0].StartMs
	cue.EndMs = min(endMs, words[len(words)-1].EndMs+100)
	for _, w := range words {
		cue.Words = append(cue.Words, w.Word)
	}
	return cue
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

// noSpaceScript reports runes of scripts written without spaces between
// words: each such token is a word of its own.
func noSpaceScript(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul)
}
