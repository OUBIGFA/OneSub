package subtitle

import (
	"context"
	"math"
	"slices"
	"strings"
	"sync"
)

const (
	minimumSplitDuration = int64(833)
	preferredSplitSpan   = int64(1200)
	maximumSplitParts    = 16
	maximumSplitBounds   = 256
)

// splitClock is available only after exact source-text and chronological word
// validation. Secondary text can project to that clock but cannot create one.
type splitClock struct {
	times  map[int]int64
	wordAt map[int]int
	words  []Word
}

type splitDPWorkspace struct {
	cost   []float64
	parent []int
	bounds []textBoundary
}

var splitDPPool = sync.Pool{New: func() any { return &splitDPWorkspace{} }}

func (s splitText) enoughContent(a, b textBoundary) bool {
	characters, words := b.characters-a.characters, b.words-a.words
	if s.language == splitChinese || s.language == splitJapanese || s.language == splitThai {
		return characters >= 4
	}
	return words >= 3 || characters >= 10
}

// A short dramatic callout is not ordinary filler. Both gaps must be measured
// between real words; cue edges or estimated times are not silence evidence.
func (s splitText) shortCallout(a, b textBoundary, clock splitClock) bool {
	if clock.wordAt == nil {
		return false
	}
	first, firstOK := clock.wordAt[a.offset]
	last, lastOK := clock.wordAt[b.offset]
	if !firstOK || !lastOK || first <= 0 || last >= len(clock.words) || last <= first {
		return false
	}
	if clock.words[first].StartMs-clock.words[first-1].EndMs < 600 || clock.words[last].StartMs-clock.words[last-1].EndMs < 600 {
		return false
	}
	text := strings.TrimSpace(s.text[a.offset:b.offset])
	return slices.Contains([]string{"快跑！", "救命！", "好！", "停！", "住手！", "冲！", "衝！", "谁？", "誰？", "为什么？", "為什麼？", "凭什么？", "憑什麼？", "Stop!", "Help!", "Who?"}, text)
}

// divide remains the small text-only entry used by tests. The cue pipeline
// supplies cancellation, minimum duration and optional primary projection.
func (s splitText) divide(count int, times map[int]int64) ([]string, []int) {
	parts, offsets, _ := s.partition(context.Background(), count, splitClock{times: times}, nil, 0, minimumSplitDuration)
	return parts, offsets
}

// partition uses a flat global DP. Every transition obeys content and clock
// gates, so a weak comma or a good first cut cannot leave an orphan tail.
// Fractions describe an already solved primary lane: secondary lanes project
// to those positions on their own legal word boundaries, not independent cuts.
func (s splitText) partition(ctx context.Context, count int, clock splitClock, fractions []float64, duration, minDuration int64) ([]string, []int, error) {
	if count < 2 || count > maximumSplitParts || len(s.boundaries) > maximumSplitBounds || clock.times == nil && duration > 0 && int64(count)*minDuration > duration {
		return nil, nil, nil
	}
	ws := splitDPPool.Get().(*splitDPWorkspace)
	defer splitDPPool.Put(ws)
	ws.bounds = ws.bounds[:0]
	for _, b := range s.boundaries {
		if clock.times != nil {
			if _, exists := clock.times[b.offset]; !exists {
				continue
			}
		}
		ws.bounds = append(ws.bounds, b)
	}
	bounds := ws.bounds
	if len(bounds) < count+1 || s.units <= 0 {
		return nil, nil, nil
	}
	stride := len(bounds)
	size := (count + 1) * stride
	ws.cost = slices.Grow(ws.cost[:0], size)[:size]
	ws.parent = slices.Grow(ws.parent[:0], size)[:size]
	for i := range ws.cost {
		ws.cost[i], ws.parent[i] = math.Inf(1), -1
	}
	ws.cost[0] = 0
	target := s.units / float64(count)
	for part := 1; part <= count; part++ {
		if err := ctx.Err(); err != nil {
			return nil, nil, err
		}
		for j := part; j < stride; j++ {
			if part < count && j == stride-1 || part == count && j != stride-1 {
				continue
			}
			b := bounds[j]
			for p := j - 1; p >= part-1; p-- {
				previous := ws.cost[(part-1)*stride+p]
				if math.IsInf(previous, 1) {
					continue
				}
				a := bounds[p]
				units := b.units - a.units
				if units <= 0 || !s.enoughContent(a, b) && !s.shortCallout(a, b, clock) {
					continue
				}
				if clock.times != nil {
					if clock.times[b.offset]-clock.times[a.offset] < minDuration {
						continue
					}
				}
				cost := 20*math.Pow((units-target)/s.limit, 2) + 20*math.Pow(math.Max(0, units-s.limit), 2)
				if clock.times == nil && duration > 0 {
					// Estimated spans can redistribute time inside the original
					// envelope. Penalize a deficit; the final clock enforces the
					// minimum without rejecting a valid short opening phrase.
					ideal := float64(duration) * units / s.units
					cost += 40 * math.Pow(math.Max(0, float64(minDuration)-ideal)/float64(minDuration), 2)
				}
				if !s.enoughContent(a, b) {
					cost += 80 // only an acoustically evidenced callout can reach here
				}
				if part < count {
					cost += b.penalty
					if word, ok := clock.wordAt[b.offset]; ok && word > 0 && word < len(clock.words) {
						gap := clock.words[word].StartMs - clock.words[word-1].EndMs
						if gap >= 300 {
							cost -= math.Min(40, float64(gap-300)/15)
						} else if gap < 80 && b.penalty >= -5 {
							cost += 30
						}
					}
				}
				if len(fractions) == count+1 && part < count {
					position := b.units / s.units
					if clock.times != nil {
						position = float64(clock.times[b.offset]-clock.times[0]) / float64(clock.times[len(s.text)]-clock.times[0])
					}
					// Alignment outweighs a secondary punctuation preference,
					// while hard word/content/time gates remain non-negotiable.
					cost += 8000 * math.Pow(position-fractions[part], 2)
				} else if count == 2 && part == 1 {
					other := s.units - units
					cost += 30*math.Pow((units-other)/s.units, 2) + math.Max(0, units-other)*1.5
				}
				index := part*stride + j
				if previous+cost < ws.cost[index] {
					ws.cost[index], ws.parent[index] = previous+cost, p
				}
			}
		}
	}
	if math.IsInf(ws.cost[count*stride+stride-1], 1) {
		return nil, nil, nil
	}
	offsets := make([]int, count+1)
	j := stride - 1
	for part := count; part >= 0; part-- {
		offsets[part] = bounds[j].offset
		if part > 0 {
			j = ws.parent[part*stride+j]
		}
	}
	parts := make([]string, count)
	for i := range parts {
		parts[i] = s.text[offsets[i]:offsets[i+1]] // lossless, including original spaces
	}
	return parts, offsets, nil
}

func (s splitText) unitsAt(offset int) float64 {
	index, _ := slices.BinarySearchFunc(s.boundaries, offset, func(boundary textBoundary, offset int) int {
		return boundary.offset - offset
	})
	return s.boundaries[index].units // offsets are selected from these same boundaries
}

func (s splitText) fractions(offsets []int) []float64 {
	fractions := make([]float64, len(offsets))
	boundary := 0
	for i, offset := range offsets {
		for boundary < len(s.boundaries)-1 && s.boundaries[boundary].offset < offset {
			boundary++
		}
		fractions[i] = s.boundaries[boundary].units / s.units
	}
	return fractions
}
