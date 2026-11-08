package subtitle

import (
	"context"
	"math"
	"slices"
	"strings"
)

// Third-party captions have event clocks but generally no word clocks. Choose
// linguistic boundaries first, then distribute the original event's time once.
// Parallel source/translation lines are projected, never read sequentially.
func splitUntimedCue(ctx context.Context, cue Cue, format, command string, original bool, source []splitText) ([]Cue, string, error) {
	lanes := slices.Clone(source)
	if cue.Translated {
		translated, reason := splitLanes(cue.Text, format)
		if reason != "" {
			return nil, reason, nil
		}
		lanes = append(lanes, translated...)
	}
	primary, first, last := primarySplitLane(lanes, len(source), !original && cue.Translated)
	lane, connectors := recognitionConnectorLanes(lanes[primary])
	lanes[primary] = lane
	for _, text := range lanes {
		if len(text.boundaries) > maximumSplitBounds {
			return nil, "capacity", nil
		}
	}
	_, minimum := splitTimeUnits(format)
	duration := cue.EndMs - cue.StartMs
	if duration < 2*preferredSplitSpan {
		return nil, "duration", nil
	}
	offsets, err := untimedSplitPlan(ctx, lane, connectors, duration, minimum)
	if err != nil {
		return nil, "", err
	}
	if len(offsets) < 3 {
		if lane.units > lane.ceiling {
			return nil, "boundary", nil
		}
		return nil, "readable", nil
	}
	pieces, positions, cuts, reason, err := projectSplitOffsets(ctx, cue, format, lanes, primary, splitClock{}, offsets)
	if err != nil {
		return nil, "", err
	}
	separator := "\n"
	if format == "ass" || format == "ssa" {
		separator = `\N`
	}
	// A primary plan may be too fine for the other simultaneous lane, or
	// their combined reading load may still be long. Solve them jointly using
	// the existing bounded planner in that case; never emit mispaired lanes.
	if reason == "" && last-first > 1 {
		for i := range len(cuts) - 1 {
			var texts []string
			for j := first; j < last; j++ {
				texts = append(texts, pieces[j][i])
			}
			if InspectReading(strings.Join(texts, "\n"), cuts[i+1]-cuts[i]).TooLong {
				reason = "bilingual"
				break
			}
		}
	}
	if reason != "" {
		if reason != "bilingual" {
			return nil, reason, nil
		}
		return splitCueLanes(ctx, cue, format, command, original, false, source)
	}
	return splitCuesFromPlan(cue, command, original, len(source), lanes, separator, splitClock{}, pieces, positions, cuts), "", nil
}

// Apply sentence, clause punctuation and linguistic-connector stages in order.
// Only a still-long span reaches the ordinary lexical DP. Fixed stages and
// bounded segment counts keep the work finite; the final plan respects time.
func untimedSplitPlan(ctx context.Context, lane, connectors splitText, duration, minimum int64) ([]int, error) {
	connectorAt := map[int]bool{}
	for i, b := range connectors.boundaries {
		if i > 0 && i < len(connectors.boundaries)-1 {
			connectorAt[b.offset] = true
		}
	}
	capacity := min(maximumSplitParts, int(duration/preferredSplitSpan))
	english := englishSplitText(lane.text)
	var plan func(splitText, int, int, bool) ([]int, error)
	plan = func(span splitText, base, stage int, whole bool) ([]int, error) {
		if err := ctx.Err(); err != nil {
			return nil, err
		}
		// A compact complete sentence survives; a single-cue request may
		// still separate clear punctuation in a compact multi-clause event.
		if stage > 0 && span.units <= span.ceiling && !(stage == 1 && whole) {
			return []int{0, len(span.text)}, nil
		}
		if stage == 3 {
			return untimedLexicalPlan(ctx, span, capacity, duration, minimum)
		}
		previous := span.boundaries[0]
		end := span.boundaries[len(span.boundaries)-1]
		bounds := []textBoundary{previous}
		for _, b := range span.boundaries[1 : len(span.boundaries)-1] {
			if stage == 2 && end.units-previous.units <= span.ceiling {
				break
			}
			selected := b.penalty <= -60 || recognitionInitialSentenceEnd(span.text, b.offset)
			if stage == 1 {
				selected = b.penalty <= -15
			}
			if stage == 2 {
				selected = connectorAt[base+b.offset] || !english && b.penalty < 0
				// Chinese 和 often coordinates nouns rather than clauses; the
				// lexical planner can still weigh it if no stronger cut fits.
				if span.language == splitChinese && strings.HasPrefix(strings.TrimSpace(span.text[b.offset:]), "和") {
					selected = false
				}
			}
			if !selected || !span.enoughContent(previous, b) || !span.enoughContent(b, end) || len(bounds) >= capacity {
				continue
			}
			bounds = append(bounds, b)
			previous = b
		}
		if len(bounds) == 1 {
			return plan(span, base, stage+1, whole)
		}
		bounds = append(bounds, end)
		out := []int{0}
		for i := 0; i < len(bounds)-1; i++ {
			a, b := bounds[i], bounds[i+1]
			sub := sliceSplitText(span, a, b)
			child, err := plan(sub, base+a.offset, stage+1, false)
			if err != nil {
				return nil, err
			}
			for _, cut := range child[1:] {
				out = append(out, a.offset+cut)
			}
		}
		return out, nil
	}
	offsets, err := plan(lane, 0, 0, true)
	if err != nil {
		return nil, err
	}
	if len(offsets)-1 > capacity {
		// Time is a hard limit. Restrict the bounded DP to the hierarchy's
		// choices so reducing the count does not invent worse midpoint cuts.
		limited := lane
		limited.boundaries = nil
		for _, b := range lane.boundaries {
			if slices.Contains(offsets, b.offset) {
				limited.boundaries = append(limited.boundaries, b)
			}
		}
		return untimedLexicalPlan(ctx, limited, capacity, duration, minimum)
	}
	return offsets, nil
}

// Rebase existing boundaries instead of re-tokenizing each phrase. The word,
// literal, Unicode grapheme and language-specific masks remain the same.
func sliceSplitText(lane splitText, a, b textBoundary) splitText {
	out := lane
	out.text = lane.text[a.offset:b.offset]
	out.units = b.units - a.units
	out.boundaries = nil
	for _, bound := range lane.boundaries {
		if bound.offset < a.offset || bound.offset > b.offset {
			continue
		}
		bound.offset -= a.offset
		bound.runeIndex -= a.runeIndex
		bound.characters -= a.characters
		bound.words -= a.words
		bound.units -= a.units
		out.boundaries = append(out.boundaries, bound)
	}
	return out
}

func untimedLexicalPlan(ctx context.Context, lane splitText, capacity int, duration, minimum int64) ([]int, error) {
	if lane.units <= lane.ceiling {
		return []int{0, len(lane.text)}, nil
	}
	maximum := min(capacity, len(lane.boundaries)-1)
	if maximum < 2 || len(lane.boundaries) > maximumSplitBounds {
		return []int{0, len(lane.text)}, nil
	}
	count := min(maximum, max(2, int(math.Ceil(lane.units/lane.limit))))
	bestCost := math.Inf(1)
	var best []int
	for candidate := count; candidate <= maximum; candidate++ {
		_, offsets, err := lane.partition(ctx, candidate, splitClock{}, nil, duration, minimum)
		if err != nil {
			return nil, err
		}
		if offsets == nil {
			continue
		}
		cost := lane.planCost(offsets, splitClock{})
		if cost < bestCost {
			bestCost = cost
			best = offsets
		}
		fits := true
		for i := 0; i < len(offsets)-1; i++ {
			fits = fits && lane.unitsAt(offsets[i+1])-lane.unitsAt(offsets[i]) <= lane.limit
		}
		if fits {
			break
		}
	}
	for candidate := count - 1; best == nil && candidate >= 2; candidate-- {
		_, offsets, err := lane.partition(ctx, candidate, splitClock{}, nil, duration, minimum)
		if err != nil {
			return nil, err
		}
		best = offsets
	}
	if best == nil {
		return []int{0, len(lane.text)}, nil
	}
	return best, nil
}
