package importing

import (
	"os"
	"path/filepath"
	"testing"
)

// Public discovery behavior, not a duplicate implementation of the scorer.
// Each name variant must pair in both directions without requiring a current
// workbench; explicit selection and numeric conflicts have separate gates.
func TestWeightedPairFromEitherEntry(t *testing.T) {
	for _, names := range [][2]string{
		{"Lesson 01.mp4", "Lesson 01-zh.srt"},
		{"Lesson 01.mp4", "Lesson 01.zh-CN.vtt"},
		{"Lesson 01.mp4", "Lesson 01_zh_Hans.ass"},
		{"Lesson 01.mp4", "Lesson 01[en].srt"},
		{"Lesson 01.mp4", "Lesson 01.eng.srt"},
		{"Lesson 01.mp4", "Lesson 01.chs.srt"},
		{"Blender Material Editing.mp4", "Blnder Material Editing.srt"},
		{"Blender Material Editing.mp4", "Blender Materiaal Editing.srt"},
		{"Introduction to Blender Materials.mp4", "Blender Materials.srt"},
		{"Creating Custom Blender Materials.mp4", "Creating Blender Materials.srt"},
		{"剪辑和制作视频字幕.mp4", "剪辑制作视频字幕.srt"},
		{"Ｃｌｉｐ ０１.mp4", "Clip_01.srt"},
		{"Résumé Intro.mp4", "Re\u0301sume\u0301 Intro.srt"},
		{"Intro1Part2.mp4", "Intro01Part02.srt"},
	} {
		t.Run(names[1], func(t *testing.T) {
			p := files(t, names[0], names[1])
			for _, entry := range p {
				got, err := Discover([]string{entry})
				if err != nil || got.Media != p[0] || got.Subtitle != p[1] || got.Automatic == "" || len(got.Candidates) != 0 {
					t.Fatalf("entry %s: %+v, %v", filepath.Base(entry), got, err)
				}
				if got.Match == nil || got.Match.Score < 85 || got.Match.Score > 100 || got.Match.Reason == "" || got.Match.Path == entry {
					t.Fatalf("missing or invalid match evidence: %+v", got.Match)
				}
			}
		})
	}
}

func TestHigherWeightWinsInsteadOfDirectoryOrder(t *testing.T) {
	for _, names := range [][]string{
		{"Lesson One Intro.mp4", "Lesson One Intro.srt", "Lesson One Intro-zh.srt", "Lesson On Intro.srt"},
		{"Lesson One Intro.mp4", "Lesson One Intro-zh.srt", "Lesson On Intro.srt"},
		{"Introduction to Advanced Materials.mp4", "Introducton to Advanced Materials.srt", "Introduction for Basic Materials.srt"},
	} {
		p := files(t, names...)
		got, err := Discover(p[:1])
		if err != nil || got.Subtitle != p[1] || got.Automatic != "subtitle" || len(got.Candidates) != 0 {
			t.Fatalf("weight did not choose %s: %+v %v", names[1], got, err)
		}
	}
}

func TestCloseWeightsAndWeakMatchesNeedChoice(t *testing.T) {
	for _, names := range [][]string{
		{"Introduction to Blender.mp4", "Introducton to Blender.srt", "Introduction for Blender.vtt"},
		{"Lesson One Intro.mp4", "Lesson One Intro-zh.srt", "Lesson One Intro-en.vtt"},
		{"abcdefghijkl.mp4", "abcdefghXXXX.srt"},
	} {
		p := files(t, names...)
		got, err := Discover(p[:1])
		if err != nil || got.Subtitle != "" || got.Automatic != "" || got.CandidateKind != "subtitle" || len(got.Candidates) != len(p)-1 || got.Match != nil {
			t.Fatalf("unsafe automatic choice: %+v %v", got, err)
		}
		for i, candidate := range got.Candidates {
			if candidate.Score < 80 || candidate.Score > 100 || candidate.Reason == "" || i > 0 && got.Candidates[i-1].Score < candidate.Score {
				t.Fatalf("unordered or invalid candidate weights: %+v", got.Candidates)
			}
		}
	}
}

func TestWeightedDiscoveryRejectsUnrelatedAndNumberConflicts(t *testing.T) {
	for _, names := range [][2]string{
		{"Course01 Introduction.mp4", "Course02 Introduction.srt"},
		{"Course01 Introduction.mp4", "Course010 Introduction.srt"},
		{"Course01 Introduction.mp4", "Course Introduction.srt"},
		{"Course01 Introduction.mp4", "Course02 Introduction-zh.srt"},
		{"第六章 Blender Materials.mp4", "第七章 Blender Materials.srt"},
		{"Intro 01 Part 02.mp4", "Intro 01 Part 03.srt"},
		{"Tutorial 2025.mp4", "Tutorial 2026.srt"},
		{"Chapter A Introduction.mp4", "Chapter B Introduction.srt"},
		{"Part I Introduction.mp4", "Part II Introduction.srt"},
		{"Lesson One Introduction.mp4", "Lesson Two Introduction.srt"},
		{"Lord of Rings II.mp4", "Lord of Rings III.srt"},
		{"one.mp4", "two.srt"},
		{"picture.mp4", "different.srt"},
		{"Notes.mp4", "new Notes.srt"},
	} {
		p := files(t, names[0], names[1])
		for _, entry := range p {
			got, err := Discover([]string{entry})
			if err != nil || got.Automatic != "" || len(got.Candidates) != 0 || entry == p[0] && got.Subtitle != "" || entry == p[1] && got.Media != "" {
				t.Fatalf("unsafe pair %v: %+v %v", names, got, err)
			}
		}
	}
}

func TestWeightedDiscoveryDeduplicatesSameFileCandidates(t *testing.T) {
	p := files(t, "Lesson One Intro.mp4", "Lesson One Intro.srt")
	alias := filepath.Join(filepath.Dir(p[1]), "Lesson One Intro.vtt")
	if err := os.Link(p[1], alias); err != nil {
		t.Skipf("file identity fixture unavailable: %v", err)
	}
	got, err := Discover(p[:1])
	if err != nil || got.Subtitle == "" || got.Automatic != "subtitle" || len(got.Candidates) != 0 {
		t.Fatalf("same file became an ambiguous pair: %+v %v", got, err)
	}
}
