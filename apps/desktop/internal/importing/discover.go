// Package importing owns the file types and pairing rules used by every import entry.
package importing

import (
	"errors"
	"os"
	"path/filepath"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/apperr"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

type Selection struct {
	Media         string      `json:"media"`
	Subtitle      string      `json:"subtitle"`
	Candidates    []Candidate `json:"candidates"`
	CandidateKind string      `json:"candidateKind"`
	Automatic     string      `json:"automatic"`
	Match         *Candidate  `json:"match,omitempty"`
}

// Extensions is shared by native dialogs, discovery and importer validation.
var Extensions = []string{"srt", "vtt", "ass", "ssa", "mp4", "mkv", "mov", "avi", "webm", "m4v", "mpeg", "mpg", "ts", "mts", "m2ts", "wmv", "mp3", "wav", "flac", "m4a", "ogg"}

func Kind(path string) string {
	ext := strings.ToLower(filepath.Ext(path))
	switch ext {
	case ".srt", ".vtt", ".ass", ".ssa":
		return "subtitle"
	case ".mp3", ".wav", ".flac", ".m4a", ".ogg":
		return "audio"
	}
	for _, known := range Extensions {
		if ext == "."+known {
			return "video"
		}
	}
	return ""
}

// Discover does not parse, upload or modify files. Callers validate the chosen
// files before committing an import; a failed companion must not discard its primary.
func Discover(paths []string) (Selection, error) {
	result := Selection{Candidates: []Candidate{}}
	if len(paths) == 0 {
		return result, errors.New(locales.ErrImportEmpty())
	}
	var identities []os.FileInfo
	for _, input := range paths {
		path, err := filepath.Abs(input)
		if err != nil {
			return result, err
		}
		info, err := os.Stat(path)
		if err != nil {
			return result, apperr.Wrap(locales.ErrImportRead(filepath.Base(path), err.Error()), err)
		}
		if !info.Mode().IsRegular() {
			return result, errors.New(locales.ErrImportFolder())
		}
		duplicate := false
		for _, prior := range identities {
			if os.SameFile(info, prior) {
				duplicate = true
				break
			}
		}
		if duplicate {
			continue
		}
		identities = append(identities, info)
		switch Kind(path) {
		case "subtitle":
			if result.Subtitle != "" {
				return result, errors.New(locales.ErrImportManySubtitles())
			}
			result.Subtitle = path
		case "video", "audio":
			if result.Media != "" {
				return result, errors.New(locales.ErrImportManyMedia())
			}
			result.Media = path
		default:
			return result, errors.New(locales.ErrImportType(filepath.Base(path)))
		}
	}
	if result.Media != "" && result.Subtitle != "" {
		return result, nil
	}
	primary, wanted := result.Media, "subtitle"
	if result.Subtitle != "" {
		primary, wanted = result.Subtitle, "video"
	}
	entries, err := os.ReadDir(filepath.Dir(primary))
	if err != nil {
		return result, apperr.Wrap(locales.ErrImportSiblings(err.Error()), err)
	}
	ranked := rankCompanions(primary, wanted, entries, identities)
	if len(ranked) == 0 {
		return result, nil
	}
	best := ranked[0]
	automatic := best.Score >= automaticScore
	if len(ranked) > 1 {
		if best.Score >= languageScore {
			// Stronger naming rules win outright, but two equally strong
			// files (including different languages/formats) still need a choice.
			automatic = best.Score > ranked[1].Score
		} else {
			automatic = automatic && best.Score-ranked[1].Score >= minimumLead
		}
	}
	if automatic {
		if wanted == "subtitle" {
			result.Subtitle = best.Path
		} else {
			result.Media = best.Path
		}
		result.Automatic, result.Match = wanted, &best
	} else {
		result.Candidates, result.CandidateKind = ranked, wanted
	}
	return result, nil
}
