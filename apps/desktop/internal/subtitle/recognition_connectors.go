package subtitle

import (
	"context"
	"strings"
	"unicode"
)

// Punctuation has already isolated this long clause. Try only meaningful
// connector boundaries first, so length balancing cannot swallow a valid
// clause merely because two adjacent clauses happen to fit under the ceiling.
// Any still-long piece then uses the existing phrase and word-clock solver.
func splitRecognitionClause(ctx context.Context, cue Cue, format string, lane splitText) ([]Cue, error) {
	lane, connectors := recognitionConnectorLanes(lane)
	parts := []Cue{cue}
	if len(connectors.boundaries) > 2 {
		parts = recognitionPunctuationParts(cue, connectors, false)
	}
	var out []Cue
	for _, part := range parts {
		pieceLane := lane
		if len(parts) > 1 {
			pieceLane, _ = recognitionConnectorLanes(recognitionSplitText(part.Text))
		}
		if recognitionLong(pieceLane) {
			split, _, err := splitCueLanes(ctx, part, format, "recognition", true, false, []splitText{pieceLane})
			if err != nil {
				return nil, err
			}
			if len(split) > 1 {
				out = append(out, split...)
				continue
			}
		}
		out = append(out, part)
	}
	return out, nil
}

// Keep coordinating noun/adjective pairs indivisible in both attempts. POS is
// a local heuristic, not a full parse: only clear clause/verb openings receive
// priority. Other languages retain their existing phrase boundaries.
func recognitionConnectorLanes(lane splitText) (splitText, splitText) {
	preferred := lane
	preferred.boundaries = nil
	if lane.language != splitWords || !englishSplitText(lane.text) {
		return lane, preferred
	}
	tokens := englishSplitTokens(lane.text)
	starts := map[int]bool{}
	type protectedGap struct{ from, to int }
	var protected []protectedGap
	nominal := func(tag string) bool {
		return strings.HasPrefix(tag, "NN") || strings.HasPrefix(tag, "JJ") || tag == "CD" || tag == "PRP"
	}
	for i := 1; i+1 < len(tokens); i++ {
		word := strings.ToLower(tokens[i].Text)
		coordinate := word == "and" || word == "or" || word == "but" || word == "nor"
		marker := englishClauseMarker(tokens, i)
		if !coordinate && marker == 0 {
			continue
		}
		// A connector always travels with what follows, including "or".
		protected = append(protected, protectedGap{tokens[i].End(), tokens[i+1].Start})
		right := i + 1
		for right < len(tokens)-1 && (tokens[right].Tag == "DT" || tokens[right].Tag == "PRP$" || strings.HasPrefix(tokens[right].Tag, "RB")) {
			right++
		}
		if coordinate && nominal(tokens[i-1].Tag) && nominal(tokens[right].Tag) && !englishClauseOpening(tokens[i+1:]) {
			protected = append(protected, protectedGap{tokens[i-1].End(), tokens[i].Start})
			protected = append(protected, protectedGap{tokens[i].End(), tokens[right].Start})
			continue
		}
		if marker > 0 {
			starts[tokens[i].Start] = true
			i += marker - 1
		}
	}
	full := lane
	full.boundaries = make([]textBoundary, 0, len(lane.boundaries))
	for i, b := range lane.boundaries {
		edge := i == 0 || i == len(lane.boundaries)-1
		blocked := false
		for _, span := range protected {
			if b.offset >= span.from && b.offset <= span.to {
				blocked = true
				break
			}
		}
		if !edge && blocked {
			continue
		}
		full.boundaries = append(full.boundaries, b)
		start := len(lane.text) - len(strings.TrimLeftFunc(lane.text[b.offset:], unicode.IsSpace))
		if edge || starts[start] {
			b.clause = true
			if !edge {
				b.penalty = -15
			}
			preferred.boundaries = append(preferred.boundaries, b)
		}
	}
	return full, preferred
}
