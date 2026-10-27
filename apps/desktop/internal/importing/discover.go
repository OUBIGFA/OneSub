// Package importing owns the file types and pairing rules used by every import entry.
package importing

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sort"
	"strings"
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
		return result, fmt.Errorf("请添加视频或字幕文件")
	}
	var identities []os.FileInfo
	for _, input := range paths {
		path, err := filepath.Abs(input)
		if err != nil {
			return result, err
		}
		info, err := os.Stat(path)
		if err != nil {
			return result, fmt.Errorf("无法读取 %s：%w", filepath.Base(path), err)
		}
		if !info.Mode().IsRegular() {
			return result, fmt.Errorf("请添加文件，不能添加文件夹")
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
				return result, fmt.Errorf("一次只能打开一份字幕；请先选择要校对的版本")
			}
			result.Subtitle = path
		case "video", "audio":
			if result.Media != "" {
				return result, fmt.Errorf("一次只能打开一段媒体；请先选择一组视频与字幕")
			}
			result.Media = path
		default:
			return result, fmt.Errorf("不支持此文件类型：%s", filepath.Base(path))
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
		return result, fmt.Errorf("无法检查同目录关联文件：%w", err)
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
