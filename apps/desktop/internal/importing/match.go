package importing

import (
	"math"
	"os"
	"path/filepath"
	"regexp"
	"runtime"
	"slices"
	"sort"
	"strings"
	"unicode"
	"unicode/utf8"

	"github.com/agnivade/levenshtein"
	"golang.org/x/text/cases"
	"golang.org/x/text/language"
	"golang.org/x/text/unicode/norm"
)

// Candidate scores rank filename evidence, not translation quality or a
// probability that the subtitle belongs to the media. Low/close scores need
// a choice; exact and language-suffix matches occupy stronger score bands.
type Candidate struct {
	Path   string `json:"path"`
	Score  int    `json:"score"`
	Reason string `json:"reason"` // exact, normalized, language or similar
}

const (
	proposalScore  = 80
	automaticScore = 85
	minimumLead    = 4
	languageScore  = 95
)

var (
	filenameNumbers  = regexp.MustCompile(`[0-9]+`)
	filenameOrdinals = regexp.MustCompile(`第([零〇一二两三四五六七八九十百千]+)[集章节部期课季话]`)
	filenameIndices  = regexp.MustCompile(`(?i)\b(?:episode|ep|season|chapter|chap|part|lesson|unit|section|vol|volume)\s+([a-z]|[ivxlcdm]{1,6}|one|two|three|four|five|six|seven|eight|nine|ten)\b`)
	filenameRoman    = regexp.MustCompile(`^(?:XX|X(?:IX|IV|V?I{0,3})|IX|IV|V?I{1,3}|V)$`)
	trailingRoman    = regexp.MustCompile(`(?:^|[ ._-])(?:XX|X(?:IX|IV|V?I{0,3})|IX|IV|V?I{1,3}|V)$`)
)

type matchName struct {
	stem, compact string
	words         []string
	numbers       []string
	hasLanguage   bool
}

// Tokenization handles separators, camel case and letter/number boundaries.
// NFKC and Unicode case folding happen before distance comparison, as the
// upstream Unicode-aware Levenshtein implementation requires.
func filenameWords(stem string) []string {
	runes := []rune(norm.NFKC.String(stem))
	var text strings.Builder
	for i, r := range runes {
		if !unicode.IsLetter(r) && !unicode.IsDigit(r) && !unicode.IsMark(r) {
			text.WriteByte(' ')
			continue
		}
		if i > 0 {
			prior := runes[i-1]
			camel := unicode.IsUpper(r) && (unicode.IsLower(prior) || unicode.IsUpper(prior) && i+1 < len(runes) && unicode.IsLower(runes[i+1]))
			number := unicode.IsDigit(r) && unicode.IsLetter(prior) || unicode.IsLetter(r) && unicode.IsDigit(prior)
			if camel || number {
				text.WriteByte(' ')
			}
		}
		text.WriteRune(r)
	}
	return strings.Fields(cases.Fold().String(text.String()))
}

// Parse real BCP-47/ISO language codes rather than deleting any last word.
// CHS/CHT are common subtitle-filename aliases, not BCP-47 language tags.
// Three-letter codes must resolve to a common two-letter base, or cmn/yue;
// this avoids treating ordinary words such as "new" as language suffixes.
func withoutLanguage(words []string) ([]string, bool) {
	for count := min(3, len(words)-1); count > 0; count-- {
		tag := strings.Join(words[len(words)-count:], "-")
		if count == 1 && (tag == "chs" || tag == "cht") {
			return words[:len(words)-1], true
		}
		parsed, err := language.Parse(tag)
		if err != nil {
			continue
		}
		base, confidence := parsed.Base()
		code := base.String()
		if confidence == language.Exact && (len(code) == 2 || code == "cmn" || code == "yue") {
			return words[:len(words)-count], true
		}
	}
	return words, false
}

func describeMatchName(path string) matchName {
	stem := strings.TrimSuffix(filepath.Base(path), filepath.Ext(path))
	words := filenameWords(stem)
	core, suffix := words, false
	// Uppercase Roman sequels are numbering, not ISO "II"/"VI" suffixes.
	// Other language codes remain parsed normally; an explicit choice can
	// still associate filenames whose notation is genuinely ambiguous.
	if !trailingRoman.MatchString(norm.NFKC.String(stem)) {
		core, suffix = withoutLanguage(words)
	}
	name := matchName{stem: stem, compact: strings.Join(words, ""), words: core, hasLanguage: suffix}
	// Keep every numeric group, in order. Whitespace must not merge "1 2"
	// into "12", and dropping a chapter number must not make a fuzzy pair.
	for _, number := range filenameNumbers.FindAllString(strings.Join(core, " "), -1) {
		number = strings.TrimLeft(number, "0")
		if number == "" {
			number = "0"
		}
		name.numbers = append(name.numbers, "n:"+number)
	}
	for _, ordinal := range filenameOrdinals.FindAllStringSubmatch(strings.Join(core, ""), -1) {
		name.numbers = append(name.numbers, "ordinal:"+ordinal[1])
	}
	indices := filenameIndices.FindAllStringSubmatch(strings.Join(words, " "), -1)
	for _, index := range indices {
		name.numbers = append(name.numbers, "section:"+index[1])
	}
	if len(indices) == 0 && len(core) > 1 && filenameRoman.MatchString(strings.ToUpper(core[len(core)-1])) {
		name.numbers = append(name.numbers, "roman:"+core[len(core)-1])
	}
	return name
}

func companionScore(a, b matchName) (int, string) {
	if a.stem == b.stem || runtime.GOOS == "windows" && strings.EqualFold(a.stem, b.stem) {
		return 100, "exact"
	}
	if !slices.Equal(a.numbers, b.numbers) {
		return 0, ""
	}
	if a.compact != "" && a.compact == b.compact {
		return 98, "normalized"
	}
	left, right := strings.Join(a.words, ""), strings.Join(b.words, "")
	if left != "" && left == right && (a.hasLanguage || b.hasLanguage) {
		return languageScore, "language"
	}
	leftSize, rightSize := utf8.RuneCountInString(left), utf8.RuneCountInString(right)
	if min(leftSize, rightSize) < 6 {
		return 0, "" // short generic names have too little fuzzy evidence
	}
	similarity := 1 - float64(levenshtein.ComputeDistance(left, right))/float64(max(leftSize, rightSize))
	score := 0
	if similarity >= .6 {
		score = int(math.Round(62 + 30*similarity))
	}
	// Missing words need a substantial shared phrase, not just a common
	// "lesson"/"intro" or matching numbers. Multisets count repeated words
	// once per occurrence, so repetition cannot inflate this evidence.
	leftWords, rightWords := contentWords(a.words), contentWords(b.words)
	remaining := map[string]int{}
	for _, word := range rightWords {
		remaining[word]++
	}
	common, commonRunes := 0, 0
	for _, word := range leftWords {
		if remaining[word] > 0 {
			remaining[word]--
			common++
			commonRunes += utf8.RuneCountInString(word)
		}
	}
	shorter := min(len(leftWords), len(rightWords))
	if common >= 2 && commonRunes >= 8 && float64(common)/float64(shorter) >= .75 {
		dice := float64(2*common) / float64(len(leftWords)+len(rightWords))
		if dice >= .65 {
			score = max(score, int(math.Round(70+22*dice)))
		}
	}
	return score, "similar"
}

func contentWords(words []string) []string {
	var content []string
	for _, word := range words {
		if strings.ContainsFunc(word, unicode.IsLetter) {
			content = append(content, word)
		}
	}
	return content
}

// Ranking and identity deduplication are shared by both import directions.
// Stat failures remain candidates: Load reports a failed companion without
// discarding the primary file. Directories and aliases of one entity do not
// become extra choices, and lexical ordering is only a stable tie breaker.
func rankCompanions(primary, wanted string, entries []os.DirEntry, seen []os.FileInfo) []Candidate {
	name, directory := describeMatchName(primary), filepath.Dir(primary)
	var ranked []Candidate
	for _, entry := range entries {
		if entry.IsDir() || Kind(entry.Name()) != wanted {
			continue
		}
		path := filepath.Join(directory, entry.Name())
		score, reason := companionScore(name, describeMatchName(path))
		if score >= proposalScore {
			ranked = append(ranked, Candidate{Path: path, Score: score, Reason: reason})
		}
	}
	sort.Slice(ranked, func(i, j int) bool {
		if ranked[i].Score != ranked[j].Score {
			return ranked[i].Score > ranked[j].Score
		}
		return ranked[i].Path < ranked[j].Path
	})
	var unique []Candidate
	for _, candidate := range ranked {
		if info, err := os.Stat(candidate.Path); err == nil {
			if !info.Mode().IsRegular() {
				continue
			}
			duplicate := false
			for _, prior := range seen {
				if os.SameFile(info, prior) {
					duplicate = true
					break
				}
			}
			if duplicate {
				continue
			}
			seen = append(seen, info)
		}
		unique = append(unique, candidate)
	}
	return unique
}
