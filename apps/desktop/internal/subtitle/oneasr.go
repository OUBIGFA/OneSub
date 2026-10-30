package subtitle

import (
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

type oneAsrWord struct {
	Text  string  `json:"text"`
	Start float64 `json:"start"`
	End   float64 `json:"end"`
}

type oneAsrWordFile struct {
	Unit      string       `json:"unit"`
	WordCount int          `json:"word_count"`
	Words     []oneAsrWord `json:"words"`
}

// ParseOneAsrWords validates the documented --words-json artifact at the
// boundary and converts seconds to the same millisecond clock as subtitles.
// Both direct attachment and speech-region remapping use this contract.
func ParseOneAsrWords(data []byte) ([]Word, error) {
	var wire oneAsrWordFile
	if len(data) > 64*1024*1024 {
		return nil, errors.New(locales.ErrWordsSize())
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return nil, err
	}
	if wire.Unit != "aligner_token" || wire.WordCount != len(wire.Words) {
		return nil, errors.New(locales.ErrWordsProtocol())
	}
	words := []Word{}
	for _, w := range wire.Words {
		if w.Start < 0 || w.End < w.Start || math.IsNaN(w.Start) || math.IsNaN(w.End) || math.IsInf(w.Start, 0) || math.IsInf(w.End, 0) || w.End*1000 > 9007199254740991 {
			return nil, errors.New(locales.ErrWordsTiming())
		}
		start, end := int64(math.Round(w.Start*1000)), int64(math.Round(w.End*1000))
		if len(words) > 0 && start < words[len(words)-1].StartMs {
			return nil, errors.New(locales.ErrWordsOrder())
		}
		words = append(words, Word{Text: w.Text, StartMs: start, EndMs: end})
	}
	return words, nil
}

// MarshalOneAsrWords stores already validated, source-clock word evidence in
// the same artifact schema, including collapsed tokens that cannot be split.
func MarshalOneAsrWords(words []Word) ([]byte, error) {
	wire := oneAsrWordFile{Unit: "aligner_token", WordCount: len(words), Words: []oneAsrWord{}}
	for _, word := range words {
		wire.Words = append(wire.Words, oneAsrWord{Text: word.Text, Start: float64(word.StartMs) / 1000, End: float64(word.EndMs) / 1000})
	}
	return json.Marshal(wire)
}

// AttachOneAsrWords reads the documented --words-json artifact (seconds).
// A cue is splittable only when token text exactly covers its current source
// and every time lies inside that cue. Unmatched cues retain cue boundaries.
func AttachOneAsrWords(doc Document, data []byte) (Document, []string, error) {
	words, err := ParseOneAsrWords(data)
	if err != nil {
		return Document{}, nil, err
	}
	out := doc
	out.Cues = append([]Cue{}, doc.Cues...)
	unmatched := 0
	for i, cue := range out.Cues {
		text, offset := SourceText(cue), 0
		selected := []Word{}
		first := sort.Search(len(words), func(j int) bool { return words[j].StartMs >= cue.StartMs })
		for j := first; j < len(words) && words[j].StartMs < cue.EndMs; j++ {
			word := words[j]
			if word.StartMs < cue.StartMs || word.EndMs > cue.EndMs {
				continue
			}
			// OneAsr v1.3.0 deliberately retains zero-duration aligner tokens.
			// They are valid evidence, but not safe split anchors. Preserve this
			// whole cue (including the token's text), without inventing duration.
			if word.EndMs == word.StartMs || (len(selected) > 0 && word.StartMs < selected[len(selected)-1].EndMs) {
				selected = nil
				break
			}
			at := strings.Index(text[offset:], word.Text)
			if word.Text == "" || at < 0 || strings.TrimSpace(text[offset:offset+at]) != "" {
				selected = nil
				break
			}
			end := offset + at + len(word.Text)
			word.Text = text[offset:end]
			selected = append(selected, word)
			offset = end
		}
		if len(selected) == 0 || strings.TrimSpace(text[offset:]) != "" {
			unmatched++
			continue
		}
		selected[len(selected)-1].Text += text[offset:]
		out.Cues[i].Words = selected
		out.Cues[i].TimingQuality = "aligned"
	}
	if _, err := TimelineAtoms(out); err != nil {
		return Document{}, nil, err
	}
	warnings := []string{}
	if unmatched > 0 {
		warnings = append(warnings, locales.WarnWordsUnmatched(unmatched))
	}
	return out, warnings, nil
}

// NormalizeRecognition makes any recognition result writable as a
// subtitle file: every cue has text, starts at 0 or later and ends after it
// starts. ASR output breaks this in ways no one model run predicts: OneAsr
// v1.3.0 writes a cue whose end is its start for a sentence whose aligned
// words all have no duration (seen over long stretches without speech), an
// aligner may end a cue before it starts, and a cue may hold only spaces.
// No text is dropped: a cue without length followed by one that starts no
// later is the start of that cue's sentence and joins it; another gets up
// to zeroLengthCue of the gap before the next cue, never past its start; a
// cue with no room at all joins the cue before it. Blank cues, which hold
// no text, go. It reports how many cues it changed.
func NormalizeRecognition(doc Document) (Document, int) {
	out := doc
	out.Cues = nil
	changed := 0
	cues := append([]Cue{}, doc.Cues...)
	for i := range cues {
		if cues[i].StartMs < 0 {
			cues[i].StartMs = 0
			changed++
		}
	}
	for i := 0; i < len(cues); i++ {
		cue := cues[i]
		if strings.TrimSpace(cue.Text) == "" {
			changed++
			continue
		}
		if cue.EndMs > cue.StartMs {
			out.Cues = append(out.Cues, cue)
			continue
		}
		changed++
		cue.EndMs = cue.StartMs
		next := int64(-1)
		if i+1 < len(cues) {
			next = cues[i+1].StartMs
		}
		switch {
		case next >= 0 && next <= cue.StartMs && strings.TrimSpace(cues[i+1].Text) != "":
			cues[i+1].StartMs = min(cues[i+1].StartMs, cue.StartMs)
			cues[i+1].Text = joinText(cue.Text, cues[i+1].Text)
			cues[i+1].OriginalText = joinText(cue.OriginalText, cues[i+1].OriginalText)
		case next > cue.StartMs || next < 0 || len(out.Cues) == 0:
			cue.EndMs = cue.StartMs + zeroLengthCue
			if next > cue.StartMs {
				cue.EndMs = min(cue.EndMs, next)
			}
			out.Cues = append(out.Cues, cue)
		default:
			last := &out.Cues[len(out.Cues)-1]
			last.Text = joinText(last.Text, cue.Text)
			last.OriginalText = joinText(last.OriginalText, cue.OriginalText)
		}
	}
	for i := range out.Cues {
		out.Cues[i].Position = i
	}
	return out, changed
}

// zeroLengthCue is the time a cue without one gets: a short reading time.
const zeroLengthCue = 1000

// joinText joins two parts of a sentence, with a space between words of
// scripts that use one.
func joinText(a, b string) string {
	a, b = strings.TrimSpace(a), strings.TrimSpace(b)
	if spaced(a, b) {
		return a + " " + b
	}
	return a + b
}

// spaced reports whether next joins before with a space: between words of
// scripts that use spaces, not between CJK characters, nor where a side
// already ends or starts with a space.
func spaced(before, next string) bool {
	if before == "" || next == "" {
		return false
	}
	last, _ := utf8.DecodeLastRuneInString(before)
	first, _ := utf8.DecodeRuneInString(next)
	if unicode.IsSpace(last) || unicode.IsSpace(first) {
		return false
	}
	return last < 0x2E80 && first < 0x2E80
}
