// Package importing owns the file types and pairing rules used by every import entry.
package importing

import (
	"errors"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/apperr"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

type Selection struct {
	Media         string   `json:"media"`
	Subtitle      string   `json:"subtitle"`
	Candidates    []string `json:"candidates"`
	CandidateKind string   `json:"candidateKind"`
	Automatic     string   `json:"automatic"`
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
	result := Selection{Candidates: []string{}}
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
	stem := strings.TrimSuffix(filepath.Base(primary), filepath.Ext(primary))
	for _, entry := range entries {
		name := entry.Name()
		otherStem := strings.TrimSuffix(name, filepath.Ext(name))
		matches := stem == otherStem
		if runtime.GOOS == "windows" {
			matches = strings.EqualFold(stem, otherStem)
		}
		if !matches || Kind(name) != wanted || entry.IsDir() {
			continue
		}
		result.Candidates = append(result.Candidates, filepath.Join(filepath.Dir(primary), name))
	}
	sort.Strings(result.Candidates)
	if len(result.Candidates) == 1 {
		if wanted == "subtitle" {
			result.Subtitle = result.Candidates[0]
		} else {
			result.Media = result.Candidates[0]
		}
		result.Automatic = wanted
		result.Candidates = []string{}
	} else if len(result.Candidates) > 1 {
		result.CandidateKind = wanted
	}
	return result, nil
}
