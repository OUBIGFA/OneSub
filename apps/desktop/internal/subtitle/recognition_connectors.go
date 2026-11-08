package subtitle

import (
	"context"
	"strings"
	"unicode"

	"github.com/jdkato/prose/v3/tokenize"
)

// Punctuation has already isolated this long clause. Try only meaningful
// connector boundaries first, so length balancing cannot outweigh them.
// Any still-long piece then uses the existing phrase and word-clock solver.
func splitRecognitionClause(ctx context.Context, cue Cue, format string, lane splitText) ([]Cue, error) {
	lane, connectors := recognitionConnectorLanes(lane)
	parts := []Cue{cue}
	if len(connectors.boundaries) > 2 {
		split, _, err := splitCueLanes(ctx, cue, format, "connector", true, false, []splitText{connectors})
		if err != nil {
			return nil, err
		}
		if len(split) > 1 {
			parts = split
		}
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
		return strings.HasPrefix(tag, "NN") || strings.HasPrefix(tag, "JJ") || tag == "CD"
	}
	for i := 1; i+1 < len(tokens); i++ {
		word := strings.ToLower(tokens[i].Text)
		coordinate := word == "and" || word == "or" || word == "but" || word == "nor"
		subordinate := word == "because" || word == "although" || word == "while" || word == "whereas" || word == "unless" || word == "if" || word == "when"
		if !coordinate && !subordinate {
			continue
		}
		// A connector always travels with what follows, including "or".
		protected = append(protected, protectedGap{tokens[i].End(), tokens[i+1].Start})
		if coordinate && nominal(tokens[i-1].Tag) && nominal(tokens[i+1].Tag) {
			protected = append(protected, protectedGap{tokens[i-1].End(), tokens[i].Start})
			continue
		}
		if subordinate || recognitionClauseOpening(tokens[i+1:]) {
			starts[tokens[i].Start] = true
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
			preferred.boundaries = append(preferred.boundaries, b)
		}
	}
	// Two readable lines are allowed; a shorter length target must not add
	// extra cuts when the connector-only plan already fits the ceiling.
	preferred.limit = preferred.ceiling
	return full, preferred
}

func recognitionClauseOpening(tokens []tokenize.Token) bool {
	i := 0
	for i < len(tokens) && strings.HasPrefix(tokens[i].Tag, "RB") {
		i++
	}
	if i == len(tokens) {
		return false
	}
	first := tokens[i].Tag
	if strings.HasPrefix(first, "VB") || first == "MD" || first == "PRP" || first == "EX" {
		return true
	}
	// An explicit noun subject needs a finite predicate. An infinitive in
	// "models and textures to download" does not make it a new clause.
	if first != "DT" && first != "PRP$" && !strings.HasPrefix(first, "NN") {
		return false
	}
	for _, t := range tokens[i+1:] {
		switch t.Tag {
		case "VBP", "VBZ", "VBD", "MD":
			return true
		case "TO", "IN", "CC", ".", ",", ":":
			return false
		}
	}
	return false
}
