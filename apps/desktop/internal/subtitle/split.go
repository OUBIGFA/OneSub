package subtitle

import (
	"context"
	"errors"
	"fmt"
	"math"
	"slices"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// SplitReport retains unsolved cues rather than claiming a split cures fast
// speech. Estimated times are only permitted by this explicit manual command;
// TimelineAtoms/Resegment keep their stricter evidence requirements.
type SplitReport struct {
	Document                             Document
	Changed, Added, Estimated, Remaining int
	First                                int
	Skipped                              map[string]string
}

// SmartSplit plans a whole-track local edit without mutating its input. The
// caller must strictly serialize formats and commit at the same revision.
// Punctuation is applied only to successfully split pieces, after boundaries
// and word timings have been resolved from the unchanged source text.
func SmartSplit(ctx context.Context, doc Document, command string, original bool, punctuation PunctuationOptions) (SplitReport, error) {
	return smartSplit(ctx, doc, command, original, "", false, punctuation)
}

// SmartSplitCue is an explicit single-cue command. It also accepts readable
// cues, while still respecting word boundaries, protected formats and time.
func SmartSplitCue(ctx context.Context, doc Document, command string, original bool, id string, punctuation PunctuationOptions) (SplitReport, error) {
	if id == "" {
		return SplitReport{}, errors.New(locales.ErrCuePosition())
	}
	return smartSplit(ctx, doc, command, original, id, false, punctuation)
}

// SmartSplitCueForced is the user's request to cut one cue that already reads
// well. It uses any legal word boundary in place of a clause or measured
// pause, so the cue becomes two parts. Word, protected-format and minimum-time
// gates still apply.
func SmartSplitCueForced(ctx context.Context, doc Document, command string, original bool, id string, punctuation PunctuationOptions) (SplitReport, error) {
	if id == "" {
		return SplitReport{}, errors.New(locales.ErrCuePosition())
	}
	return smartSplit(ctx, doc, command, original, id, true, punctuation)
}

func smartSplit(ctx context.Context, doc Document, command string, original bool, id string, force bool, punctuation PunctuationOptions) (SplitReport, error) {
	if id != "" && !slices.ContainsFunc(doc.Cues, func(c Cue) bool { return c.ID == id }) {
		return SplitReport{}, errors.New(locales.ErrCuePosition())
	}
	out := SplitReport{Document: doc, First: -1, Skipped: map[string]string{}}
	out.Document.Cues = make([]Cue, 0, len(doc.Cues))
	for _, cue := range doc.Cues {
		if err := ctx.Err(); err != nil {
			return SplitReport{}, err
		}
		parts := []Cue{cue}
		if id == cue.ID || id == "" && InspectCueReading(cue).Long() {
			var reason string
			var err error
			parts, reason, err = splitCue(ctx, cue, doc.Format, command, original, force)
			if err != nil {
				return SplitReport{}, err
			}
			if len(parts) < 2 {
				parts = []Cue{cue}
				out.Skipped[cue.ID] = reason
			} else {
				if out.First < 0 {
					out.First = len(out.Document.Cues)
				}
				out.Changed++
				out.Added += len(parts) - 1
				for i := range parts {
					part := &parts[i]
					*part = trimSplitPadding(*part, doc.Format)
					source := punctuation.Apply(SourceText(*part))
					if punctuation.StripTrailing {
						source = strings.TrimSpace(source) // remove only split-edge padding during explicit cleanup
					}
					if part.Translated {
						part.TranslationSource = source
						part.Text = punctuation.Apply(part.Text)
						if punctuation.StripTrailing {
							part.Text = strings.TrimSpace(part.Text)
						}
					} else {
						part.Text = source
					}
				}
			}
		}
		for _, part := range parts {
			part.Position = len(out.Document.Cues)
			out.Document.Cues = append(out.Document.Cues, part)
			if len(parts) > 1 && part.TimingQuality == "estimated" {
				out.Estimated++
			}
			if InspectCueReading(part).Problem() {
				out.Remaining++
			}
		}
	}
	return out, nil
}

// Exact source coverage is required before using word evidence. A hand-edited
// source or optimized wording must not silently inherit the old word clock.
func wordBoundaries(cue Cue, text string) (map[int]int64, map[int]int) {
	if len(cue.Words) < 2 {
		return nil, nil
	}
	times, words := map[int]int64{0: cue.StartMs}, map[int]int{0: 0}
	var all strings.Builder
	for i, w := range cue.Words {
		if strings.TrimSpace(w.Text) == "" || w.StartMs < cue.StartMs || w.EndMs > cue.EndMs || w.StartMs >= w.EndMs || i > 0 && w.StartMs < cue.Words[i-1].EndMs {
			return nil, nil
		}
		if i > 0 {
			times[all.Len()], words[all.Len()] = w.StartMs, i
		}
		all.WriteString(w.Text)
	}
	if all.String() != text {
		return nil, nil
	}
	times[len(text)], words[len(text)] = cue.EndMs, len(cue.Words)
	return times, words
}

// The original track is source-driven. An optimized cue with a translation is
// translation-driven, even if the source has an ASR clock or is much longer.
// Multi-line imports retain simultaneous lanes; the longest primary-group
// lane drives cuts and the others project, never sequence top then bottom.
func primarySplitLane(lanes []splitText, sourceCount int, translated bool) (int, int, int) {
	first, last := 0, sourceCount
	if translated {
		first, last = sourceCount, len(lanes)
	}
	primary := first
	for i := first + 1; i < last; i++ {
		if lanes[i].units/lanes[i].limit > lanes[primary].units/lanes[primary].limit {
			primary = i
		}
	}
	return primary, first, last
}

// partitionCue solves primary text, projects it to a real source clock when
// available, then projects secondary text to the resulting shared times.
// Estimated clocks redistribute within the envelope; aligned clocks never do.
func partitionCue(ctx context.Context, cue Cue, format string, lanes []splitText, primary, count int, clock splitClock) ([][]string, [][]int, []int64, string, error) {
	_, minDuration := splitTimeUnits(format)
	pieces, offsets := make([][]string, len(lanes)), make([][]int, len(lanes))
	var primaryClock splitClock
	if primary == 0 {
		primaryClock = clock
	}
	var err error
	pieces[primary], offsets[primary], err = lanes[primary].partition(ctx, count, primaryClock, nil, cue.EndMs-cue.StartMs, minDuration)
	if err != nil || len(pieces[primary]) != count {
		return nil, nil, nil, "boundary", err
	}
	return projectSplitOffsets(ctx, cue, format, lanes, primary, clock, offsets[primary])
}

// Project a chosen primary-language plan onto one shared clock and all lanes.
// No word clock is manufactured for third-party subtitles.
func projectSplitOffsets(ctx context.Context, cue Cue, format string, lanes []splitText, primary int, clock splitClock, primaryOffsets []int) ([][]string, [][]int, []int64, string, error) {
	count := len(primaryOffsets) - 1
	unit, minDuration := splitTimeUnits(format)
	pieces, offsets := make([][]string, len(lanes)), make([][]int, len(lanes))
	offsets[primary] = primaryOffsets
	for i := range count {
		pieces[primary] = append(pieces[primary], lanes[primary].text[primaryOffsets[i]:primaryOffsets[i+1]])
	}
	var err error
	fractions := lanes[primary].fractions(offsets[primary])
	if clock.times != nil && primary != 0 {
		// A translation determines linguistic cuts; its source chooses the
		// nearest legal ASR boundaries, without estimating any word times.
		pieces[0], offsets[0], err = lanes[0].partition(ctx, count, clock, fractions, 0, minDuration)
		if err != nil || len(pieces[0]) != count {
			return nil, nil, nil, "bilingual", err
		}
	}
	cuts := make([]int64, count+1)
	cuts[0], cuts[count] = cue.StartMs, cue.EndMs
	remainingWeight := 1.0
	for i := 1; i < count; i++ {
		if clock.times != nil {
			cuts[i] = clock.times[offsets[0][i]]
		} else {
			// Clamp only estimated spans, distributing the remaining time by
			// primary text weight. A five-character introduction may need
			// 833 ms rather than its unclamped 700 ms, without adding a gap.
			remaining := cue.EndMs - cuts[i-1]
			weight := fractions[i] - fractions[i-1]
			ideal := int64(math.Round(float64(remaining)*weight/remainingWeight/float64(unit))) * unit
			span := max(minDuration, min(ideal, remaining-int64(count-i)*minDuration))
			cuts[i] = cuts[i-1] + span
			remainingWeight -= weight
		}
	}
	for i := range count {
		if cuts[i+1]-cuts[i] < minDuration || cuts[i]%unit != 0 || cuts[i+1]%unit != 0 {
			return nil, nil, nil, "duration", nil
		}
	}
	for i := range fractions {
		fractions[i] = float64(cuts[i]-cue.StartMs) / float64(cue.EndMs-cue.StartMs)
	}
	for i, lane := range lanes {
		if i == primary || i == 0 && clock.times != nil {
			continue
		}
		pieces[i], offsets[i], err = lane.partition(ctx, count, splitClock{}, fractions, 0, minDuration)
		if err != nil || len(pieces[i]) != count {
			return nil, nil, nil, "bilingual", err
		}
	}
	return pieces, offsets, cuts, "", nil
}

func splitCue(ctx context.Context, cue Cue, format, command string, original bool, force bool) ([]Cue, string, error) {
	if cue.ReadOnly {
		return nil, "protected", nil
	}
	source, reason := splitLanes(SourceText(cue), format)
	if reason != "" {
		return nil, reason, nil
	}
	if times, _ := wordBoundaries(cue, SourceText(cue)); times == nil && !force {
		return splitUntimedCue(ctx, cue, format, command, original, source)
	}
	return splitCueLanes(ctx, cue, format, command, original, force, source)
}

func splitTimeUnits(format string) (unit, minimum int64) {
	unit = 1
	if format == "ass" || format == "ssa" {
		unit = 10
	}
	return unit, (minimumSplitDuration + unit - 1) / unit * unit
}

// The recognition path supplies wider reading limits without changing the
// explicit editor command's length preferences or timing/protection gates.
func splitCueLanes(ctx context.Context, cue Cue, format, command string, original bool, force bool, source []splitText) ([]Cue, string, error) {
	reason := ""
	lanes := source
	if cue.Translated {
		translated, why := splitLanes(cue.Text, format)
		if why != "" {
			return nil, why, nil
		}
		lanes = append(slices.Clone(source), translated...)
	}
	primary, groupStart, groupEnd := primarySplitLane(lanes, len(source), !original && cue.Translated)
	primaryText := SourceText(cue)
	if !original && cue.Translated {
		primaryText = cue.Text
	}
	// The 16-column (CJK) or 56-column target chooses cuts in long
	// text; it does not force another split in a compact utterance. Use the
	// wider 20/64-column ceilings, plus the existing combined reading-load gate.
	// A manual request can still separate independent clauses or real pauses.
	lanes[primary].clauseOnly = !InspectReading(primaryText, cue.EndMs-cue.StartMs).TooLong
	for i := groupStart; i < groupEnd; i++ {
		lanes[primary].clauseOnly = lanes[primary].clauseOnly && lanes[i].units <= lanes[i].ceiling
	}
	if force {
		lanes[primary].clauseOnly = false
	}
	maximum := min(maximumSplitParts, int((cue.EndMs-cue.StartMs)/preferredSplitSpan))
	if maximum < 2 {
		return nil, "duration", nil
	}
	// Slow delivery must not increase the number of linguistic pieces.
	// Duration bounds feasibility, while the primary text bounds necessity.
	count := 2
	for i, lane := range lanes {
		if len(lane.boundaries) > maximumSplitBounds {
			return nil, "capacity", nil
		}
		minimumCharacters := 3 // Korean: three tiny complete words, or ten printed characters
		switch lane.language {
		case splitChinese, splitJapanese, splitThai:
			minimumCharacters = 4
		case splitWords:
			minimumCharacters = 15 // three words and 15 printed characters
		}
		maximum = min(maximum, len(lane.boundaries)-1, lane.boundaries[len(lane.boundaries)-1].characters/minimumCharacters)
		if i >= groupStart && i < groupEnd && !lanes[primary].clauseOnly {
			count = max(count, int(math.Ceil(lane.units/lane.limit)))
		}
	}
	if maximum < 2 {
		if len(lanes) > 1 {
			return nil, "bilingual", nil
		}
		return nil, "boundary", nil
	}
	count = min(count, maximum)
	var clock splitClock
	if len(source) == 1 {
		clock.times, clock.wordAt = wordBoundaries(cue, source[0].text)
		clock.words = cue.Words
	}
	separator := "\n"
	if format == "ass" || format == "ssa" {
		separator = `\N`
	}
	var pieces [][]string
	var offsets [][]int
	var cuts []int64
	bestCost := math.Inf(1)
	try := func(candidate int) (bool, error) {
		parts, positions, boundaries, why, err := partitionCue(ctx, cue, format, lanes, primary, candidate, clock)
		if err != nil {
			return false, err
		}
		if why != "" {
			reason = why
			return false, nil
		}
		longest := 0.0
		multilineOverflow := 0.0
		// A complete sequence of punctuated clauses may use the existing
		// compact ceiling. A weak conjunction or arbitrary lexical cut is
		// not evidence for enlarging every subtitle in the sequence.
		completeClauses := true
		for _, offset := range positions[primary][1:candidate] {
			index, _ := slices.BinarySearchFunc(lanes[primary].boundaries, offset, func(b textBoundary, offset int) int { return b.offset - offset })
			completeClauses = completeClauses && lanes[primary].boundaries[index].penalty <= -15
		}
		for i := range candidate {
			texts := make([]string, groupEnd-groupStart)
			for lane := groupStart; lane < groupEnd; lane++ {
				text := parts[lane][i]
				texts[lane-groupStart] = text
				// Compare physical scan columns, including punctuation. Reading
				// DU is a separate review signal and does not set line capacity.
				columns := lanes[lane].unitsAt(positions[lane][i+1]) - lanes[lane].unitsAt(positions[lane][i])
				limit := lanes[lane].limit
				if completeClauses {
					limit = lanes[lane].ceiling
				}
				longest = max(longest, columns/limit)
			}
			if len(texts) > 1 {
				load := InspectReading(strings.Join(texts, separator), boundaries[i+1]-boundaries[i])
				longest = max(longest, load.Units/load.MaxUnits)
				multilineOverflow += math.Pow(math.Max(0, load.Units-load.MaxUnits), 2) * 20
			}
		}
		primaryClock := splitClock{}
		if primary == 0 {
			primaryClock = clock
		}
		cost := lanes[primary].planCost(positions[primary], primaryClock) + multilineOverflow
		if cost < bestCost {
			pieces, offsets, cuts, bestCost = parts, positions, boundaries, cost
		}
		return longest <= 1, nil
	}
	for candidate := count; candidate <= maximum; candidate++ {
		fit, err := try(candidate)
		if err != nil {
			return nil, "", err
		}
		if fit {
			break
		}
	}
	// If the desired count has no legal path, absorb fragments by solving
	// fewer whole phrases globally. Never relax a word, content or time gate.
	for candidate := count - 1; pieces == nil && candidate >= 2; candidate-- {
		if _, err := try(candidate); err != nil {
			return nil, "", err
		}
	}
	if pieces == nil {
		if lanes[primary].clauseOnly && reason == "boundary" {
			reason = "readable"
		}
		return nil, reason, nil
	}
	return splitCuesFromPlan(cue, command, original, len(source), lanes, separator, clock, pieces, offsets, cuts), "", nil
}

func splitCuesFromPlan(cue Cue, command string, original bool, sourceCount int, lanes []splitText, separator string, clock splitClock, pieces [][]string, offsets [][]int, cuts []int64) []Cue {
	count := len(cuts) - 1
	from := cue.SplitCueIDs
	if len(from) == 0 {
		from = cue.MergedCueIDs
	}
	if len(from) == 0 {
		from = []string{cue.ID}
	}
	out := make([]Cue, count)
	for i := range count {
		texts := make([]string, len(lanes))
		for lane := range lanes {
			texts[lane] = pieces[lane][i]
		}
		src := strings.Join(texts[:sourceCount], separator)
		part := cue
		part.ID = fmt.Sprintf("split-%s-%s-%d", command, cue.ID, i)
		part.StartMs, part.EndMs = cuts[i], cuts[i+1]
		part.Text, part.OriginalText, part.TranslationSource = src, src, ""
		if cue.Translated {
			part.TranslationSource, part.Text = src, strings.Join(texts[sourceCount:], separator)
		}
		part.TimeLocked, part.TextLocked = true, cue.TextLocked || cue.Translated
		part.TimingQuality, part.Words = "estimated", nil
		part.SplitReview = len(lanes) > 1
		part.SplitCueIDs, part.MergedCueIDs = slices.Clone(from), nil
		if !original && len(part.SourceCueIDs) == 0 {
			part.SourceCueIDs = []string{cue.ID}
		}
		// An old atom range describes the unsplit cue. Preserve actual source
		// IDs, never misleading fine-grained provenance or fabricated words.
		part.SourceRange = nil
		if clock.times != nil {
			part.TimingQuality = "aligned"
			part.Words = slices.Clone(cue.Words[clock.wordAt[offsets[0][i]]:clock.wordAt[offsets[0][i+1]]])
		}
		out[i] = part
	}
	return out
}
