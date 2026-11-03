package subtitle

import (
	"html"
	"regexp"
	"strings"
	"unicode"

	"github.com/go-text/typesetting/segmenter"
	"golang.org/x/text/width"
)

// ReadingLoad is a review signal, not a claim that a sentence can be shortened
// without losing meaning. Length and speed are independent: splitting cannot
// add reading time. Thresholds follow the registered subtitle skill; unknown
// Latin languages use its conservative default instead of guessing English.
type ReadingLoad struct {
	Units, MaxUnits, Rate, MaxRate float64
	TooLong, TooFast, TooBrief     bool
}

func (r ReadingLoad) Problem() bool { return r.TooLong || r.TooFast || r.TooBrief }

type CueReadingLoad struct{ Source, Translation ReadingLoad }

func (r CueReadingLoad) Problem() bool { return r.Source.Problem() || r.Translation.Problem() }
func (r CueReadingLoad) Long() bool    { return r.Source.TooLong || r.Translation.TooLong }

var readingMarkup = regexp.MustCompile(`<[^>\r\n]+>|\{\\[^}]*\}`)
var readingParenthesis = regexp.MustCompile(`\([^()（）]*[A-Za-z][^()（）]*\)|（[^()（）]*[A-Za-z][^()（）]*）`)

func visibleReadingText(text string) string {
	text = readingMarkup.ReplaceAllString(text, "")
	text = strings.NewReplacer(`\N`, "\n", `\n`, "\n", `\h`, " ").Replace(text)
	return html.UnescapeString(text)
}

func readingProfile(text string) (cjk bool, maxUnits, maxRate float64) {
	var han, kana, hangul int
	for _, r := range text {
		switch {
		case unicode.In(r, unicode.Hiragana, unicode.Katakana):
			kana++
		case unicode.In(r, unicode.Hangul):
			hangul++
		case unicode.In(r, unicode.Han):
			han++
		}
	}
	switch {
	case kana > 0:
		return true, 25, 4
	case hangul > 0 && hangul >= han:
		return true, 25, 12
	case han > 0:
		return true, 25, 4.5
	default:
		// One 42-character line cut English into fragments and two full
		// lines (84) kept it too long to read or translate. Allow a line and a
		// half, 64 characters, as the comfortable maximum.
		return false, 64, 17
	}
}

// Count grapheme clusters using the same mature Unicode implementation as
// MyGo. Combining marks and emoji sequences are never treated as split points.
func readingUnits(text string, cjk bool) float64 {
	var seg segmenter.Segmenter
	seg.Init([]rune(strings.TrimSpace(text)))
	it := seg.GraphemeIterator()
	units := 0.0
	for it.Next() {
		g := it.Grapheme().Text
		if cjk {
			units += clusterUnits(g, true)
		} else {
			units += clusterUnits(g, false)
		}
	}
	return units
}

func clusterUnits(g []rune, cjk bool) float64 {
	r := g[0]
	if unicode.IsControl(r) || unicode.Is(unicode.Cf, r) {
		return 0
	}
	if !cjk {
		return 1
	}
	if unicode.IsSpace(r) || unicode.IsPunct(r) {
		return 0
	}
	if k := width.LookupRune(r).Kind(); k == width.EastAsianWide || k == width.EastAsianFullwidth {
		return 1
	}
	return 0.5
}

func InspectReading(text string, durationMs int64) ReadingLoad {
	text = visibleReadingText(text)
	cjk, maxUnits, maxRate := readingProfile(text)
	// Parenthetical Latin originals are reference notes under the skill's
	// DU rules. Mixed or Chinese parentheses still carry their full content.
	if cjk {
		text = readingParenthesis.ReplaceAllStringFunc(text, func(s string) string {
			for _, r := range s {
				if unicode.In(r, unicode.Han, unicode.Hiragana, unicode.Katakana, unicode.Hangul) {
					return s
				}
			}
			return ""
		})
	}
	units := readingUnits(text, cjk)
	rate := units * 1000 / float64(max(durationMs, 1))
	return ReadingLoad{Units: units, MaxUnits: maxUnits, Rate: rate, MaxRate: maxRate,
		TooLong: units > maxUnits, TooFast: rate > maxRate, TooBrief: units > 0 && durationMs < 833}
}

func InspectCueReading(cue Cue) CueReadingLoad {
	if cue.ReadOnly {
		return CueReadingLoad{}
	} // ASS drawing is not dialogue.
	load := CueReadingLoad{Source: InspectReading(SourceText(cue), cue.EndMs-cue.StartMs)}
	if cue.Translated {
		load.Translation = InspectReading(cue.Text, cue.EndMs-cue.StartMs)
	}
	return load
}
