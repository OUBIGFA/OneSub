package subtitle

import (
	"crypto/sha256"
	"errors"
	"fmt"
	"reflect"
	"strings"
	"unicode"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// Atom is the smallest evidenced source unit. Imported cues have no invented
// word times; only verified word coverage can introduce interior cut points.
type Atom struct {
	ID        string `json:"id"`
	CueID     string `json:"cueId"`
	Text      string `json:"text"`
	StartMs   int64  `json:"startMs"`
	EndMs     int64  `json:"endMs"`
	Protected bool   `json:"protected"`
}

func TimelineAtoms(doc Document) ([]Atom, error) {
	atoms := []Atom{}
	for _, cue := range doc.Cues {
		text := SourceText(cue)
		protected := doc.Format != "srt" || cue.ReadOnly || cue.TextLocked || cue.TimeLocked || strings.ContainsAny(text, "<{\\") || strings.Contains(text, "\n")
		if len(cue.Words) == 0 || protected {
			atoms = append(atoms, Atom{cue.ID, cue.ID, text, cue.StartMs, cue.EndMs, protected})
			continue
		}
		var all strings.Builder
		for i, word := range cue.Words {
			if strings.TrimSpace(word.Text) == "" || word.StartMs < cue.StartMs || word.EndMs > cue.EndMs || word.EndMs <= word.StartMs || (i > 0 && word.StartMs < cue.Words[i-1].EndMs) {
				return nil, errors.New(locales.ErrAtomWords(cue.ID))
			}
			all.WriteString(word.Text)
			start, end := word.StartMs, cue.EndMs
			if i == 0 {
				start = cue.StartMs
			}
			if i+1 < len(cue.Words) {
				end = cue.Words[i+1].StartMs
			}
			atoms = append(atoms, Atom{fmt.Sprintf("%s:w%d", cue.ID, i), cue.ID, word.Text, start, end, false})
		}
		// Exact source coverage, not ASR spelling guesses or character ratios.
		if all.String() != text {
			return nil, errors.New(locales.ErrAtomText(cue.ID))
		}
	}
	return atoms, nil
}

// Resegment resolves ordered closed ranges into times on the host. It rejects
// missing/duplicate coverage, protected edits, and overlap-crossing joins.
func Resegment(doc Document, ranges []SourceRange) (Document, []string, error) {
	atoms, err := TimelineAtoms(doc)
	if err != nil {
		return Document{}, nil, err
	}
	lookup := map[string]int{}
	cues := map[string]Cue{}
	for i, atom := range atoms {
		if _, exists := lookup[atom.ID]; exists {
			return Document{}, nil, errors.New(locales.ErrAtomDuplicate())
		}
		lookup[atom.ID] = i
	}
	for _, cue := range doc.Cues {
		cues[cue.ID] = cue
	}
	out := doc
	out.Cues = []Cue{}
	warnings := []string{}
	next := 0
	for _, r := range ranges {
		first, ok := lookup[r.FirstAtom]
		last, lastOK := lookup[r.LastAtom]
		if !ok || !lastOK || first != next || last < first {
			return Document{}, nil, errors.New(locales.ErrRangeInvalid())
		}
		group := atoms[first : last+1]
		start, end := group[0].StartMs, group[len(group)-1].EndMs
		ids := []string{}
		var text strings.Builder
		words := []Word{}
		for i, atom := range group {
			if atom.Protected && len(group) != 1 {
				return Document{}, nil, errors.New(locales.ErrRangeProtected())
			}
			if i > 0 && atom.CueID != group[i-1].CueID {
				if atom.StartMs < group[i-1].EndMs {
					return Document{}, nil, errors.New(locales.ErrRangeOverlap())
				}
				if atom.StartMs > group[i-1].EndMs {
					if strings.TrimSpace(r.Reason) == "" {
						return Document{}, nil, errors.New(locales.ErrRangeGapReason())
					}
					warnings = append(warnings, locales.WarnRangeGap(int(group[i-1].EndMs), int(atom.StartMs), r.Reason))
				}
				// Joined cues are one text: a space between words, none
				// between characters of scripts written without one.
				if spaced(text.String(), atom.Text) {
					text.WriteString(" ")
				}
			}
			if len(ids) == 0 || ids[len(ids)-1] != atom.CueID {
				ids = append(ids, atom.CueID)
			}
			text.WriteString(atom.Text)
		}
		original := cues[ids[0]]
		unchanged := len(ids) == 1 && start == original.StartMs && end == original.EndMs && text.String() == SourceText(original)
		if !unchanged && strings.TrimSpace(r.Reason) == "" {
			return Document{}, nil, errors.New(locales.ErrRangeReason())
		}
		cue := original
		if !unchanged {
			if doc.Format != "srt" {
				return Document{}, nil, errors.New(locales.ErrRangeFormat())
			}
			// A piece divided out of a cue starts with its first word, not
			// the space before it.
			value := strings.TrimSpace(text.String())
			cue = Cue{ID: fmt.Sprintf("seg-%x", sha256.Sum256([]byte(r.FirstAtom+"/"+r.LastAtom)))[:28], StartMs: start, EndMs: end, Text: value, OriginalText: value, SourceCueIDs: ids, TimingQuality: "cue_boundary"}
			// Retain original word evidence only if every cue in this range has
			// it; cue-level evidence must not turn into fictional word evidence.
			allWords := true
			for _, id := range ids {
				if len(cues[id].Words) == 0 {
					allWords = false
				}
			}
			if allWords {
				for _, id := range ids {
					for _, word := range cues[id].Words {
						if word.StartMs >= start && word.EndMs <= end {
							words = append(words, word)
						}
					}
				}
				// Cross-cue joins introduce a separator in source text.
				if len(ids) == 1 && len(words) > 0 {
					// The words still spell the text exactly, trimmed alike.
					words[0].Text = strings.TrimLeftFunc(words[0].Text, unicode.IsSpace)
					words[len(words)-1].Text = strings.TrimRightFunc(words[len(words)-1].Text, unicode.IsSpace)
					cue.Words = words
					cue.TimingQuality = "aligned"
				}
			}
		}
		if cue.EndMs <= cue.StartMs {
			return Document{}, nil, errors.New(locales.ErrRangeDuration())
		}
		if !cue.ReadOnly && !cue.TextLocked {
			copyRange := r
			cue.SourceRange = &copyRange
		}
		cue.Position = len(out.Cues)
		out.Cues = append(out.Cues, cue)
		next = last + 1
	}
	if next != len(atoms) {
		return Document{}, nil, errors.New(locales.ErrRangeCoverage())
	}
	return out, warnings, nil
}

// ValidateTranslation repeats provenance/locks against the current store, not
// against a report supplied by a model or a stale review window.
func ValidateTranslation(base, candidate Document) error {
	if base.Format != candidate.Format || base.Original != candidate.Original {
		return errors.New(locales.ErrTranslationFormat())
	}
	atoms, err := TimelineAtoms(base)
	if err != nil {
		return err
	}
	ranges := []SourceRange{}
	for _, cue := range candidate.Cues {
		if cue.SourceRange != nil && !cue.ReadOnly && !cue.TextLocked {
			ranges = append(ranges, *cue.SourceRange)
			continue
		}
		found := []Atom{}
		for _, atom := range atoms {
			if atom.CueID == cue.ID {
				found = append(found, atom)
			}
		}
		if len(found) == 0 {
			return errors.New(locales.ErrTranslationSource())
		}
		ranges = append(ranges, SourceRange{FirstAtom: found[0].ID, LastAtom: found[len(found)-1].ID})
	}
	rebuilt, _, err := Resegment(base, ranges)
	if err != nil {
		return err
	}
	if len(rebuilt.Cues) != len(candidate.Cues) {
		return errors.New(locales.ErrTranslationCount())
	}
	for i, want := range rebuilt.Cues {
		got := candidate.Cues[i]
		if want.ReadOnly || want.TextLocked {
			// Position can move when earlier unlocked cues merge.
			want.Position = got.Position
			if !reflect.DeepEqual(want, got) {
				return errors.New(locales.ErrTranslationLocked())
			}
			continue
		}
		want.TranslationSource = SourceText(want)
		want.Translated = true
		want.Text = got.Text
		if strings.TrimSpace(got.Text) == "" || len(got.Text) > 24000 || !reflect.DeepEqual(want, got) {
			return errors.New(locales.ErrTranslationMetadata())
		}
	}
	return nil
}
