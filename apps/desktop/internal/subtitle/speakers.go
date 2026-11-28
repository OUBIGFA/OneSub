package subtitle

import (
	"errors"
	"fmt"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"slices"
	"sort"
	"strings"
)

// SpeakerTurn keeps the original audio clock. Overlap is meaningful and is
// never flattened to a single person merely to make the table simpler.
type SpeakerTurn struct {
	StartMs int64    `json:"startMs"`
	EndMs   int64    `json:"endMs"`
	ID      string   `json:"id"`
	Score   *float64 `json:"score,omitempty"`
}
type SpeakerAssignment struct {
	IDs        []string `json:"ids,omitempty"`
	Status     string   `json:"status"`
	AnalysisID string   `json:"analysisId,omitempty"`
	Manual     bool     `json:"manual,omitempty"`
	Review     bool     `json:"review,omitempty"`
}
type SpeakerState struct {
	OffsetMs        int64             `json:"offsetMs,omitempty"`
	Artifact        string            `json:"artifact,omitempty"`
	AnalysisID      string            `json:"analysisId,omitempty"`
	MediaSHA256     string            `json:"mediaSha256,omitempty"`
	MediaPath       string            `json:"mediaPath,omitempty"`
	MediaBytes      int64             `json:"mediaBytes,omitempty"`
	MediaModifiedNs int64             `json:"mediaModifiedNs,omitempty"`
	Names           map[string]string `json:"names"`
	Turns           []SpeakerTurn     `json:"turns,omitempty"`
}

// ValidateSpeakerState is the trust boundary for worker and saved artifacts.
func ValidateSpeakerState(s *SpeakerState) error {
	if s == nil {
		return nil
	}
	if len(s.Turns) > 1000000 || len(s.Names) > 1000 {
		return errors.New("speaker result exceeds limits")
	}
	for id, name := range s.Names {
		if id == "" || strings.TrimSpace(name) == "" || len(name) > 512 || strings.ContainsAny(name, "\r\n\x00") {
			return errors.New("invalid speaker name")
		}
	}
	var previous int64 = -1
	for _, t := range s.Turns {
		if t.StartMs < 0 || t.EndMs <= t.StartMs || t.EndMs > 9007199254740991 || t.StartMs < previous || s.Names[t.ID] == "" {
			return errors.New("invalid speaker interval")
		}
		if t.Score != nil && !(*t.Score >= -1 && *t.Score <= 1) {
			return errors.New("invalid speaker score")
		}
		previous = t.StartMs
	}
	return nil
}

// AssignSpeaker uses covered intervals, not the nearest person across a gap.
// At least 80% of the evidence must be covered; this is a coverage rule, not
// a claimed probability. Ambiguity remains visible for manual review.
type speakerIndex struct {
	turns   []SpeakerTurn
	maxEnds []int64
}

func newSpeakerIndex(turns []SpeakerTurn) speakerIndex {
	index := speakerIndex{turns: turns, maxEnds: make([]int64, len(turns))}
	var end int64
	for i, t := range turns {
		end = max(end, t.EndMs)
		index.maxEnds[i] = end
	}
	return index
}
func AssignSpeaker(c Cue, turns []SpeakerTurn, analysis string) *SpeakerAssignment {
	return newSpeakerIndex(turns).assign(c, analysis)
}
func (index speakerIndex) assign(c Cue, analysis string) *SpeakerAssignment {
	turns := index.turns
	a := &SpeakerAssignment{Status: "unknown", AnalysisID: analysis, Review: true}
	if c.StartMs < 0 || c.EndMs <= c.StartMs {
		return a
	}
	spans := [][2]int64{{c.StartMs, c.EndMs}}
	if len(c.Words) > 0 {
		spans = nil
		for _, w := range c.Words {
			if w.EndMs > w.StartMs {
				spans = append(spans, [2]int64{w.StartMs, w.EndMs})
			}
		}
	}
	total, covered := int64(0), int64(0)
	overlap, low := false, false
	ids := map[string]bool{}
	for _, span := range spans {
		total += max(int64(0), span[1]-span[0])
		bounds := []int64{span[0], span[1]}
		var candidates []SpeakerTurn
		first := sort.Search(len(turns), func(i int) bool { return turns[i].StartMs >= span[1] })
		begin := sort.Search(len(index.maxEnds), func(i int) bool { return index.maxEnds[i] > span[0] })
		for _, t := range turns[begin:first] {
			if t.EndMs <= span[0] {
				continue
			}
			t.StartMs = max(t.StartMs, span[0])
			t.EndMs = min(t.EndMs, span[1])
			candidates = append(candidates, t)
			bounds = append(bounds, t.StartMs, t.EndMs)
		}
		slices.Sort(bounds)
		bounds = slices.Compact(bounds)
		for i := 1; i < len(bounds); i++ {
			active := map[string]bool{}
			for _, t := range candidates {
				if t.StartMs <= bounds[i-1] && t.EndMs >= bounds[i] {
					active[t.ID] = true
					if t.Score != nil && *t.Score < 0 {
						low = true
					}
				}
			}
			if len(active) > 0 {
				covered += bounds[i] - bounds[i-1]
			}
			if len(active) > 1 {
				overlap = true
			}
			for id := range active {
				ids[id] = true
			}
		}
	}
	for id := range ids {
		a.IDs = append(a.IDs, id)
	}
	slices.Sort(a.IDs)
	if overlap {
		a.Status = "overlap"
	} else if len(ids) > 1 {
		a.Status = "mixed"
	} else if len(ids) == 1 && total > 0 && covered*5 >= total*4 && !low {
		a.Status = "single"
		a.Review = false
	}
	return a
}

func SpeakerCompatible(a, b *SpeakerAssignment) bool {
	if a == nil || b == nil {
		return a == nil && b == nil
	}
	return a.Status == "single" && b.Status == "single" && !a.Review && !b.Review && slices.Equal(a.IDs, b.IDs)
}

func MergeSpeakers(a, b *SpeakerAssignment) *SpeakerAssignment {
	if a == nil && b == nil {
		return nil
	}
	out := &SpeakerAssignment{Status: "mixed", Review: true}
	for _, v := range []*SpeakerAssignment{a, b} {
		if v != nil {
			out.IDs = append(out.IDs, v.IDs...)
			out.Manual = out.Manual || v.Manual
			if v.AnalysisID != "" {
				out.AnalysisID = v.AnalysisID
			}
		}
	}
	slices.Sort(out.IDs)
	out.IDs = slices.Compact(out.IDs)
	if SpeakerCompatible(a, b) {
		*out = *a
		out.IDs = slices.Clone(a.IDs)
		out.Manual = a.Manual || b.Manual
	}
	return out
}

// MarkSpeakerReview never edits a shared assignment behind an undo snapshot.
func MarkSpeakerReview(c *Cue) {
	if c.Speaker != nil {
		s := *c.Speaker
		s.Review = true
		if !s.Manual {
			s.Status = "unknown"
		}
		c.Speaker = &s
	}
}

func AttachSpeakers(doc Document, s *SpeakerState) Document {
	out := doc
	out.Speakers = s
	out.Cues = slices.Clone(doc.Cues)
	index := newSpeakerIndex(s.Turns)
	for i := range out.Cues {
		c := &out.Cues[i]
		if c.ReadOnly || c.Speaker != nil && c.Speaker.Manual {
			continue
		}
		c.Speaker = index.assign(*c, s.AnalysisID)
	}
	return out
}

// SplitSpeakerChanges only cuts newly recognized text at evidenced word
// boundaries. Imported/manually edited subtitles use annotation only.
func SplitSpeakerChanges(doc Document) Document {
	if doc.Speakers == nil {
		return doc
	}
	out := doc
	out.Cues = nil
	index := newSpeakerIndex(doc.Speakers.Turns)
	for _, c := range doc.Cues {
		if c.ReadOnly || c.TextLocked || c.TimeLocked || c.Translated || len(c.Words) < 2 {
			out.Cues = append(out.Cues, c)
			continue
		}
		var joined strings.Builder
		for _, w := range c.Words {
			joined.WriteString(w.Text)
		}
		if joined.String() != c.Text {
			out.Cues = append(out.Cues, c)
			continue
		}
		start := 0
		last := index.assign(Cue{StartMs: c.Words[0].StartMs, EndMs: c.Words[0].EndMs}, doc.Speakers.AnalysisID)
		flush := func(end int) {
			p := c
			p.Words = slices.Clone(c.Words[start:end])
			var text strings.Builder
			for _, w := range p.Words {
				text.WriteString(w.Text)
			}
			p.Text = text.String()
			p.OriginalText = p.Text
			p.StartMs = p.Words[0].StartMs
			p.EndMs = p.Words[len(p.Words)-1].EndMs
			p.ID = fmt.Sprintf("%s:s%d", c.ID, start)
			p.SourceCueIDs = []string{c.ID}
			p.Speaker = index.assign(p, doc.Speakers.AnalysisID)
			out.Cues = append(out.Cues, p)
		}
		for i := 1; i < len(c.Words); i++ {
			w := c.Words[i]
			current := index.assign(Cue{StartMs: w.StartMs, EndMs: w.EndMs}, doc.Speakers.AnalysisID)
			if last.Status == "single" && current.Status == "single" && !SpeakerCompatible(last, current) {
				flush(i)
				start = i
			}
			last = current
		}
		if start == 0 {
			out.Cues = append(out.Cues, c)
		} else {
			flush(len(c.Words))
		}
	}
	for i := range out.Cues {
		out.Cues[i].Position = i
	}
	return out
}

// SpeakerLabel has stable fallback labels; localization is done by the UI
// for unknown/mixed/overlap, while exported names remain user-authored data.
func SpeakerLabel(c Cue, s *SpeakerState) string {
	if c.Speaker == nil {
		return ""
	}
	if c.Speaker.Status != "single" || len(c.Speaker.IDs) != 1 {
		return ""
	}
	if s != nil {
		return s.Names[c.Speaker.IDs[0]]
	}
	return ""
}

// WithSpeakerNames is an output projection. The source document is untouched.
func WithSpeakerNames(doc Document) Document {
	out := doc
	out.Cues = slices.Clone(doc.Cues)
	for i := range out.Cues {
		c := &out.Cues[i]
		if c.ReadOnly || c.Speaker == nil {
			continue
		}
		name := SpeakerLabel(*c, doc.Speakers)
		if name == "" {
			name = locales.SpeakerUnknown()
			if c.Speaker.Status == "mixed" || c.Speaker.Status == "overlap" {
				name = locales.SpeakerMixed()
			}
		}
		name = strings.NewReplacer("<", "＜", ">", "＞", "{", "｛", "}", "｝", "\\", "＼").Replace(name)
		c.Text = name + "：" + c.Text
	}
	return out
}
