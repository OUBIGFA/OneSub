package subtitle

import (
	"fmt"
	"slices"
	"strings"
	"sync"
	"unicode"

	"github.com/jdkato/prose/v3/tag"
	"github.com/jdkato/prose/v3/tokenize"
)

var englishSplitTokenizer = tokenize.New()
var englishSplitTagger = sync.OnceValue(func() *tag.Tagger {
	tagger, err := tag.New()
	if err != nil {
		panic(fmt.Errorf("invalid embedded English split model: %w", err))
	}
	return tagger
})

// English-only models must not assign grammatical constraints to every
// space-separated language. Require Latin text and several English function
// words; ambiguous short/non-English lanes retain their Unicode boundaries.
func englishSplitText(text string) bool {
	for _, r := range text {
		if unicode.IsLetter(r) && !unicode.Is(unicode.Latin, r) {
			return false
		}
	}
	anchors, strong := 0, false
	for _, word := range strings.Fields(text) {
		word = strings.ToLower(strings.TrimFunc(word, unicode.IsPunct))
		switch word {
		case "the", "a", "an", "we", "you", "they", "he", "she", "it", "is", "are", "was", "were", "has", "have", "had", "this", "that", "these", "those", "will", "would", "should", "could", "and", "but", "because", "with", "from", "of", "to", "our", "their", "your":
			anchors++
		}
		switch word {
		case "the", "we", "you", "they", "she", "is", "are", "was", "were", "these", "those", "will", "would", "should", "could", "with", "our", "their", "your", "have", "had", "because":
			strong = true
		}
	}
	return strong && anchors >= 2
}

// POS tags describe local phrase structure, not a full syntactic parse. Lock
// determiner/modifier/noun and auxiliary/predicate connections; keep ordinary
// verb/object and subject/predicate boundaries available with a soft cost.
// The model never emits text or timing and cannot reopen protected literals.
func protectEnglishPhrases(text string, ws *splitAnalysis) {
	if !englishSplitText(text) {
		return
	}
	tokens := englishSplitTokenizer.Tokenize(text)
	// Reset the model's previous-tag context at sentence punctuation.
	start := 0
	for i, token := range tokens {
		if strings.ContainsAny(token.Text, ".!?") && len(token.Text) == 1 || i == len(tokens)-1 {
			englishSplitTagger().TagTokens(tokens[start : i+1])
			start = i + 1
		}
	}
	for i := 1; i < len(tokens); i++ {
		left, right := tokens[i-1], tokens[i]
		gap := text[left.End():right.Start]
		if gap == "" || strings.TrimSpace(gap) != "" {
			continue
		}
		l, r := left.Tag, right.Tag
		noun := func(s string) bool { return strings.HasPrefix(s, "NN") }
		adjective := func(s string) bool { return strings.HasPrefix(s, "JJ") }
		verb := func(s string) bool { return strings.HasPrefix(s, "VB") }
		adverb := func(s string) bool { return strings.HasPrefix(s, "RB") }
		bind := (l == "DT" || l == "PDT" || l == "PRP$" || l == "POS" || l == "CD") && (noun(r) || adjective(r) || r == "CD")
		bind = bind || adjective(l) && (adjective(r) || noun(r)) || noun(l) && noun(r)
		bind = bind || (l == "MD" || l == "TO") && (verb(r) || adverb(r))
		bind = bind || (l == "PRP" || l == "EX") && (verb(r) || r == "MD" || adverb(r))
		bind = bind || adverb(l) && (verb(r) || adjective(r) || adverb(r)) || verb(l) && r == "RP"
		// Taggers may label a phrasal particle as an adverb or preposition.
		// Keep the verb + particle + preposition chain together in that case.
		if verb(l) && (r == "RP" || r == "IN" || adverb(r)) && i+1 < len(tokens) && (tokens[i+1].Tag == "IN" || tokens[i+1].Tag == "TO") {
			bind = true
		}
		auxiliary := slices.Contains([]string{"am", "is", "are", "was", "were", "be", "been", "being", "have", "has", "had", "do", "does", "did", "'s", "’s", "'re", "’re", "'ve", "’ve"}, strings.ToLower(left.Text))
		bind = bind || auxiliary && (verb(r) || adverb(r) || adjective(r))
		from, _ := slices.BinarySearch(ws.bytes, left.End())
		to, _ := slices.BinarySearch(ws.bytes, right.Start)
		for cut := from; cut <= to; cut++ {
			if bind {
				ws.blocked[cut] = true
			} else if verb(l) && (r == "DT" || r == "PRP$" || noun(r)) || noun(l) && (verb(r) || r == "MD") {
				ws.phraseCosts[cut] += 16
			}
		}
	}
}
