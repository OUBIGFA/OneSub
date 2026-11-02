package subtitle

import (
	"context"
	"fmt"
	"math"
	"slices"
	"strings"
	"unicode"
	"unicode/utf8"

	"errors"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/go-text/typesetting/segmenter"
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
	return smartSplit(ctx, doc, command, original, "", punctuation)
}

// SmartSplitCue is an explicit single-cue command. It also accepts readable
// cues, while still respecting word boundaries, protected formats and time.
func SmartSplitCue(ctx context.Context, doc Document, command string, original bool, id string, punctuation PunctuationOptions) (SplitReport, error) {
	if id == "" {
		return SplitReport{}, errors.New(locales.ErrCuePosition())
	}
	return smartSplit(ctx, doc, command, original, id, punctuation)
}

func smartSplit(ctx context.Context, doc Document, command string, original bool, id string, punctuation PunctuationOptions) (SplitReport, error) {
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
			parts, reason = splitCue(cue, doc.Format, command, original)
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
					source := punctuation.Apply(SourceText(*part))
					if part.Translated {
						part.TranslationSource = source
						part.Text = punctuation.Apply(part.Text)
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

type textBoundary struct {
	offset         int
	units, penalty float64
}
type splitText struct {
	text               string
	boundaries         []textBoundary
	units, limit, rate float64
}

// Only plain text is structurally split for now. Tag-state, karaoke and inline
// timestamps must never be copied across independent event clocks. Newlines
// are separate simultaneous lanes, so imported bilingual text is not sequenced.
func splitLanes(text, format string) ([]splitText, string) {
	if readingMarkup.MatchString(text) || strings.ContainsAny(text, "<>{}") || strings.Contains(text, "&") {
		return nil, "protected"
	}
	if format == "ass" || format == "ssa" {
		text = strings.NewReplacer(`\N`, "\n", `\n`, "\n").Replace(text)
	}
	if strings.Contains(text, `\`) {
		return nil, "protected"
	}
	texts := strings.Split(strings.ReplaceAll(text, "\r\n", "\n"), "\n")
	lanes := make([]splitText, 0, len(texts))
	for _, text := range texts {
		if strings.TrimSpace(text) == "" {
			return nil, "boundary"
		}
		lanes = append(lanes, newSplitText(text))
	}
	return lanes, ""
}

func newSplitText(text string) splitText {
	cjk, limit, rate := readingProfile(text)
	out := splitText{text: text, limit: limit, rate: rate, boundaries: []textBoundary{{}}}
	var seg segmenter.Segmenter
	seg.Init([]rune(text))
	it := seg.GraphemeIterator()
	depth := 0
	for it.Next() {
		g := it.Grapheme()
		r := g.Text[0]
		if strings.ContainsRune("([{（【「『《", r) {
			depth++
		}
		if strings.ContainsRune(")]}）】」』》", r) && depth > 0 {
			depth--
		}
		out.units += clusterUnits(g.Text, cjk)
		end := g.OffsetInBytes + g.LengthInBytes
		if end == len(text) {
			break
		}
		if depth > 0 {
			continue
		}
		next, _ := utf8.DecodeRuneInString(text[end:])
		if strings.ContainsRune("，。！？；：、,.!?;:…)]}）】」』》”’", next) {
			continue
		}
		// ASCII punctuation inside an unspaced token is not a phrase break:
		// preserve URL schemes, numeric separators and code-like identifiers.
		if strings.ContainsRune(",;:!?", r) && !unicode.IsSpace(next) && !isCJK(next) {
			continue
		}
		penalty := 7.0
		switch {
		case unicode.IsSpace(r) || unicode.IsSpace(next):
			penalty = 3
		case strings.ContainsRune("。！？!?…", r):
			penalty = 0
		case strings.ContainsRune("，；：、,;:", r):
			penalty = 1
		case r == '.' && unicode.IsSpace(next):
			penalty = 0
		case isCJK(r) && isCJK(next):
			penalty = 7
		default:
			continue
		}
		// A space after punctuation carries that punctuation's preference.
		left := strings.TrimSpace(text[:end])
		last, _ := utf8.DecodeLastRuneInString(left)
		if strings.ContainsRune("。！？!?…", last) {
			penalty = 0
		}
		if strings.ContainsRune("，；：、,;:", last) {
			penalty = 1
		}
		if last == '.' {
			word := left[strings.LastIndexAny(left, " \t")+1:]
			if strings.Contains(strings.TrimSuffix(word, "."), ".") || slices.Contains([]string{"mr.", "mrs.", "ms.", "dr.", "prof.", "vs.", "etc.", "e.g.", "i.e."}, strings.ToLower(word)) || utf8.RuneCountInString(word) <= 2 {
				continue
			}
			penalty = 0
		}
		out.boundaries = append(out.boundaries, textBoundary{end, out.units, penalty})
	}
	out.boundaries = append(out.boundaries, textBoundary{len(text), out.units, 0})
	return out
}

func isCJK(r rune) bool {
	return unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul)
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

// Choose balanced natural boundaries with bounded linear scans. No arbitrary
// mid-word fallback is used for a URL, number, or unspaced non-CJK word.
func (s splitText) divide(count int, times map[int]int64) ([]string, []int) {
	bounds := s.boundaries
	if times != nil {
		bounds = slices.DeleteFunc(slices.Clone(bounds), func(b textBoundary) bool { _, ok := times[b.offset]; return !ok })
	}
	if len(bounds) < count+1 {
		return nil, nil
	}
	parts, offsets := []string{}, []int{0}
	previous := 0
	for part := 1; part < count; part++ {
		remaining := count - part
		target := (s.units - bounds[previous].units) / float64(remaining+1)
		best, score := -1, math.Inf(1)
		for i := previous + 1; i < len(bounds)-remaining; i++ {
			b := bounds[i]
			units := b.units - bounds[previous].units
			if units <= 0 || s.units-b.units <= 0 {
				continue
			}
			if times != nil && (times[b.offset]-times[bounds[previous].offset] < 1000 || times[len(s.text)]-times[b.offset] < int64(remaining)*1000) {
				continue
			}
			cost := math.Pow((units-target)/s.limit, 2)*20 + b.penalty
			cost += math.Max(0, units/s.limit-1)*100 + math.Max(0, (s.units-b.units)/float64(remaining)/s.limit-1)*100
			if cost < score {
				best, score = i, cost
			}
			if units > max(s.limit, target)*2 && best >= 0 {
				break
			}
		}
		if best < 0 {
			return nil, nil
		}
		parts = append(parts, strings.TrimSpace(s.text[bounds[previous].offset:bounds[best].offset]))
		offsets = append(offsets, bounds[best].offset)
		previous = best
	}
	parts = append(parts, strings.TrimSpace(s.text[bounds[previous].offset:]))
	offsets = append(offsets, len(s.text))
	if slices.Contains(parts, "") {
		return nil, nil
	}
	return parts, offsets
}

// partitionCue shares one timeline across all simultaneous text lanes. Missing
// word times use proportional allocation constrained to a one-second minimum;
// available time is redistributed, not shortened by artificial gaps.
func partitionCue(cue Cue, format string, lanes []splitText, count int, times map[int]int64) ([][]string, []int, []int64, string) {
	pieces := make([][]string, len(lanes))
	var offsets []int
	for i, lane := range lanes {
		var evidence map[int]int64
		if i == 0 {
			evidence = times
		}
		parts, positions := lane.divide(count, evidence)
		if len(parts) != count {
			if len(lanes) > 1 {
				return nil, nil, nil, "bilingual"
			}
			return nil, nil, nil, "boundary"
		}
		pieces[i] = parts
		if i == 0 {
			offsets = positions
		}
	}
	weights := make([]float64, count)
	total := 0.0
	for i := range count {
		for lane, parts := range pieces {
			cjk, _, _ := readingProfile(lanes[lane].text)
			weights[i] = max(weights[i], readingUnits(parts[i], cjk)/lanes[lane].rate)
		}
		weights[i] = max(0.1, weights[i])
		total += weights[i]
	}
	cuts := make([]int64, count+1)
	cuts[0], cuts[count] = cue.StartMs, cue.EndMs
	unit := int64(1)
	if format == "ass" || format == "ssa" {
		unit = 10
	}
	for i := 1; i < count; i++ {
		remaining := cue.EndMs - cuts[i-1]
		ideal := int64(math.Round(float64(remaining)*weights[i-1]/total/float64(unit))) * unit
		span := max(int64(1000), min(ideal, remaining-int64(count-i)*1000))
		cuts[i] = cuts[i-1] + span
		total -= weights[i-1]
		if times != nil {
			cuts[i] = times[offsets[i]]
		}
	}
	for i := range count {
		if cuts[i+1]-cuts[i] < 1000 || cuts[i]%unit != 0 || cuts[i+1]%unit != 0 {
			return nil, nil, nil, "duration"
		}
	}
	return pieces, offsets, cuts, ""
}

func splitCue(cue Cue, format, command string, original bool) ([]Cue, string) {
	if cue.ReadOnly {
		return nil, "protected"
	}
	source, reason := splitLanes(SourceText(cue), format)
	if reason != "" {
		return nil, reason
	}
	lanes := source
	if cue.Translated {
		translated, why := splitLanes(cue.Text, format)
		if why != "" {
			return nil, why
		}
		lanes = append(slices.Clone(source), translated...)
	}
	load := InspectCueReading(cue)
	count := max(2, int(math.Ceil(load.Source.Units/load.Source.MaxUnits)))
	if cue.Translated {
		count = max(count, int(math.Ceil(load.Translation.Units/load.Translation.MaxUnits)))
	}
	maximum := int((cue.EndMs - cue.StartMs) / 1000)
	if maximum < 2 {
		return nil, "duration"
	}
	for _, lane := range lanes {
		maximum = min(maximum, len(lane.boundaries)-1)
	}
	if maximum < 2 {
		if len(lanes) > 1 {
			return nil, "bilingual"
		}
		return nil, "boundary"
	}
	count = min(count, maximum)
	var times map[int]int64
	var wordAt map[int]int
	if len(source) == 1 {
		times, wordAt = wordBoundaries(cue, source[0].text)
		if times != nil {
			// A genuine pause helps rank otherwise similar natural boundaries.
			// It cannot override the minimum duration or invent a cut in a word.
			for i, boundary := range lanes[0].boundaries {
				if word, ok := wordAt[boundary.offset]; ok && word > 0 && word < len(cue.Words) {
					gap := cue.Words[word].StartMs - cue.Words[word-1].EndMs
					lanes[0].boundaries[i].penalty -= min(4, float64(gap)/250)
				}
			}
		}
	}
	separator := "\n"
	if format == "ass" || format == "ssa" {
		separator = `\N`
	}
	var pieces [][]string
	var offsets []int
	var cuts []int64
	bestLoad := math.Inf(1)
	for candidate := count; candidate <= maximum; candidate++ {
		parts, positions, boundaries, why := partitionCue(cue, format, lanes, candidate, times)
		if why != "" {
			if pieces == nil {
				return nil, why
			}
			break
		}
		longest := 0.0
		for i := range candidate {
			texts := make([]string, len(lanes))
			for lane := range lanes {
				texts[lane] = parts[lane][i]
			}
			for _, group := range [][]string{texts[:len(source)], texts[len(source):]} {
				if len(group) == 0 {
					continue
				}
				r := InspectReading(strings.Join(group, separator), boundaries[i+1]-boundaries[i])
				longest = max(longest, r.Units/r.MaxUnits)
			}
		}
		// Retry finer partitions only while they improve the worst length.
		// An indivisible URL/phrase must not turn other words into tiny cues.
		if pieces != nil && longest >= bestLoad {
			break
		}
		pieces, offsets, cuts, bestLoad = parts, positions, boundaries, longest
		if longest <= 1 {
			break
		}
	}
	count = len(cuts) - 1
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
		src := strings.Join(texts[:len(source)], separator)
		part := cue
		part.ID = fmt.Sprintf("split-%s-%s-%d", command, cue.ID, i)
		part.StartMs, part.EndMs = cuts[i], cuts[i+1]
		part.Text, part.OriginalText, part.TranslationSource = src, src, ""
		if cue.Translated {
			part.TranslationSource, part.Text = src, strings.Join(texts[len(source):], separator)
		}
		part.TimeLocked, part.TextLocked = true, cue.TextLocked || cue.Translated
		part.TimingQuality, part.Words = "estimated", nil
		part.SplitReview = len(lanes) > 1
		part.SplitCueIDs, part.MergedCueIDs = slices.Clone(from), nil
		if !original && len(part.SourceCueIDs) == 0 {
			part.SourceCueIDs = []string{cue.ID}
		}
		// The old atom range covered a whole cue, not this new part. Retain
		// actual source IDs, never misleading fine-grained atom provenance.
		part.SourceRange = nil
		if times != nil {
			part.TimingQuality = "aligned"
			part.Words = slices.Clone(cue.Words[wordAt[offsets[i]]:wordAt[offsets[i+1]]])
			part.Words[0].Text = strings.TrimLeftFunc(part.Words[0].Text, unicode.IsSpace)
			part.Words[len(part.Words)-1].Text = strings.TrimRightFunc(part.Words[len(part.Words)-1].Text, unicode.IsSpace)
		}
		out[i] = part
	}
	return out, ""
}
