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
	tokens := englishSplitTokens(text)
	// Multiword operators form one grammatical unit. Their words cannot be
	// separated even when a later fallback uses ordinary lexical boundaries.
	for i := range tokens {
		length := englishClauseMarker(tokens, i)
		if length > 0 && i+length < len(tokens) {
			blockEnglishGap(ws, tokens[i].End(), tokens[i+length].Start)
		}
		for _, phrase := range englishProtectedOperators {
			if matchEnglishTokens(tokens, i, phrase) {
				blockEnglishGap(ws, tokens[i].End(), tokens[i+len(strings.Fields(phrase))-1].Start)
			}
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
		bind = bind || verb(l) && (r == "IN" || r == "TO")
		// Written-out numbers are one value even if the POS tagger labels
		// one component as an adjective. Units belong to that value too.
		bind = bind || englishNumberWord(left.Text) && (englishNumberWord(right.Text) || slices.Contains([]string{"percent", "percentage", "degrees", "degree"}, strings.ToLower(right.Text)))
		if strings.EqualFold(right.Text, "and") && englishNumberWord(left.Text) && i+1 < len(tokens) && englishNumberWord(tokens[i+1].Text) {
			bind = true
		}
		if strings.EqualFold(left.Text, "and") && i >= 2 && englishNumberWord(tokens[i-2].Text) && englishNumberWord(right.Text) {
			bind = true
		}
		// Taggers may label a phrasal particle as an adverb or preposition.
		// Keep the verb + particle + preposition chain together in that case.
		if verb(l) && (r == "RP" || r == "IN" || adverb(r)) && i+1 < len(tokens) && (tokens[i+1].Tag == "IN" || tokens[i+1].Tag == "TO") {
			bind = true
		}
		auxiliary := slices.Contains([]string{"am", "is", "are", "was", "were", "be", "been", "being", "have", "has", "had", "do", "does", "did", "'s", "’s", "'re", "’re", "'ve", "’ve"}, strings.ToLower(left.Text))
		bind = bind || auxiliary && (verb(r) || adverb(r) || adjective(r))
		from, _ := slices.BinarySearch(ws.bytes, left.End())
		to, _ := slices.BinarySearch(ws.bytes, right.Start)
		clauseHead := englishClauseMarker(tokens, i) > 0
		// Degree phrases use a noun-shaped "bit" as a modifier. Preserve
		// "a (little) bit too bright" just as ordinary adverb/adjective chains.
		if i >= 2 && strings.EqualFold(left.Text, "bit") && (adverb(r) || adjective(r)) {
			previous := strings.ToLower(tokens[i-2].Text)
			bind = bind || previous == "a" || (previous == "little" || previous == "tiny") && i >= 3 && strings.EqualFold(tokens[i-3].Text, "a")
		}
		for cut := from; cut <= to; cut++ {
			if clauseHead {
				ws.clauseHeads[cut] = true
			} else if bind {
				ws.blocked[cut] = true
			} else if verb(l) && (r == "DT" || r == "PRP$" || noun(r)) || noun(l) && (verb(r) || r == "MD") {
				ws.phraseCosts[cut] += 16
			}
		}
	}
}

func englishNumberWord(text string) bool {
	switch strings.ToLower(text) {
	case "zero", "one", "two", "three", "four", "five", "six", "seven", "eight", "nine", "ten", "eleven", "twelve", "thirteen", "fourteen", "fifteen", "sixteen", "seventeen", "eighteen", "nineteen", "twenty", "thirty", "forty", "fifty", "sixty", "seventy", "eighty", "ninety", "hundred", "thousand", "million", "billion", "trillion", "point":
		return true
	}
	return false
}

func blockEnglishGap(ws *splitAnalysis, fromByte, toByte int) {
	from, _ := slices.BinarySearch(ws.bytes, fromByte)
	to, _ := slices.BinarySearch(ws.bytes, toByte)
	for cut := from; cut <= to; cut++ {
		ws.blocked[cut] = true
	}
}

// Independently expressed grammar categories; not an imported collocation
// dictionary. See the dated source review in the splitting integration notes.
var englishClauseOperators = []string{
	"as soon as", "as long as", "so long as", "in order that", "in order to", "so as to",
	"on condition that", "even if", "even though", "as if", "as though", "now that",
	"provided that", "providing that", "in case", "so that", "so because", "so if", "so when",
}
var englishProtectedOperators = []string{
	"because of", "in spite of", "in front of", "as well as", "instead of", "according to",
	"not only", "but also", "rather than", "such as", "kind of", "sort of", "a little", "a tiny",
	"look forward to", "take care of",
}

func matchEnglishTokens(tokens []tokenize.Token, index int, phrase string) bool {
	words := strings.Fields(phrase)
	if index+len(words) > len(tokens) {
		return false
	}
	for j, word := range words {
		if !strings.EqualFold(tokens[index+j].Text, word) {
			return false
		}
	}
	return true
}

// Return the complete opener, only with evidence of a following clause/action.
// Ambiguous temporal words such as "after lunch" are prepositions, not clauses.
func englishClauseMarker(tokens []tokenize.Token, i int) int {
	for _, phrase := range englishClauseOperators {
		n := len(strings.Fields(phrase))
		if matchEnglishTokens(tokens, i, phrase) && i+n < len(tokens) && englishClauseOpening(tokens[i+n:]) {
			return n
		}
	}
	if i+1 >= len(tokens) {
		return 0
	}
	word := strings.ToLower(tokens[i].Text)
	if word == "so" && englishResultSo(tokens, i) {
		return 1
	}
	switch word {
	case "and", "or", "but", "nor", "yet", "because", "although", "though", "while", "whereas", "unless", "if", "when", "before", "after", "since", "until", "once", "as", "whether", "provided", "providing",
		"however", "therefore", "nevertheless", "nonetheless", "moreover", "furthermore", "otherwise", "meanwhile", "then", "anyway":
		if englishClauseOpening(tokens[i+1:]) {
			return 1
		}
	}
	return 0
}

// "So" introducing a result/restart differs from degree modifiers such as
// "so bright" and "so much brighter". Keep a following causal opener with it.
func englishResultSo(tokens []tokenize.Token, i int) bool {
	if !strings.EqualFold(tokens[i].Text, "so") || i+1 >= len(tokens) {
		return false
	}
	next := strings.ToLower(tokens[i+1].Text)
	if next == "because" || next == "if" || next == "when" {
		return true
	}
	return englishClauseOpening(tokens[i+1:])
}

func englishSplitTokens(text string) []tokenize.Token {
	tokens := englishSplitTokenizer.Tokenize(text)
	// Reset the model's previous-tag context at sentence punctuation.
	start := 0
	for i, token := range tokens {
		if strings.ContainsAny(token.Text, ".!?") && len(token.Text) == 1 || i == len(tokens)-1 {
			englishSplitTagger().TagTokens(tokens[start : i+1])
			start = i + 1
		}
	}
	return tokens
}

func englishClauseOpening(tokens []tokenize.Token) bool {
	i := 0
	for i < len(tokens) && strings.HasPrefix(tokens[i].Tag, "RB") {
		i++
	}
	if i == len(tokens) {
		return false
	}
	first := tokens[i].Tag
	if strings.HasPrefix(first, "VB") || first == "MD" {
		return true
	}
	// An explicit noun subject needs a finite predicate. An infinitive in
	// "models and textures to download" does not make it a new clause.
	if first != "DT" && first != "PRP$" && first != "PRP" && first != "EX" && !strings.HasPrefix(first, "NN") {
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
