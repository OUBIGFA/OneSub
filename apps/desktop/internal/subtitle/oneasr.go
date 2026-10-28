package subtitle

import (
	"encoding/json"
	"errors"
	"math"
	"sort"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// AttachOneAsrWords reads the documented --words-json artifact (seconds).
// A cue is splittable only when token text exactly covers its current source
// and every time lies inside that cue. Unmatched cues retain cue boundaries.
func AttachOneAsrWords(doc Document, data []byte) (Document, []string, error) {
	var wire struct {
		Unit      string `json:"unit"`
		WordCount int    `json:"word_count"`
		Words     []struct {
			Text  string  `json:"text"`
			Start float64 `json:"start"`
			End   float64 `json:"end"`
		} `json:"words"`
	}
	if len(data) > 64*1024*1024 {
		return Document{}, nil, errors.New(locales.ErrWordsSize())
	}
	if err := json.Unmarshal(data, &wire); err != nil {
		return Document{}, nil, err
	}
	if wire.Unit != "aligner_token" || wire.WordCount != len(wire.Words) {
		return Document{}, nil, errors.New(locales.ErrWordsProtocol())
	}
	words := []Word{}
	for _, w := range wire.Words {
		if w.Start < 0 || w.End <= w.Start || math.IsNaN(w.Start) || math.IsInf(w.End, 0) {
			return Document{}, nil, errors.New(locales.ErrWordsTiming())
		}
		if len(words) > 0 && w.Start*1000 < float64(words[len(words)-1].StartMs) {
			return Document{}, nil, errors.New(locales.ErrWordsOrder())
		}
		words = append(words, Word{Text: w.Text, StartMs: int64(math.Round(w.Start * 1000)), EndMs: int64(math.Round(w.End * 1000))})
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
