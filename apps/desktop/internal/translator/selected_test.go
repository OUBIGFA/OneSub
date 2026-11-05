package translator

import (
	"context"
	"errors"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
)

func TestSelectedTranslationBoundsContextKeepsMetadataAndRequestsAgain(t *testing.T) {
	f, config := serve(t, ok)
	config.OptimizeTimeline = true
	config.Resume = &ResumeState{InputHash: "must-not-resume"}
	doc := testDocument(40)
	doc.Cues[2].TranslationSource = doc.Cues[2].Text
	doc.Cues[2].Text, doc.Cues[2].Translated, doc.Cues[2].TextLocked = "人工旧译文 2", true, true
	doc.Cues[2].TimeLocked = true
	doc.Cues[2].SourceCueIDs = []string{"earlier-id"}
	doc.Cues[2].SourceRange = &subtitle.SourceRange{FirstAtom: "earlier-first", LastAtom: "earlier-last", Reason: "existing provenance"}
	before := marshal(doc)
	for range 2 {
		result, err := TranslateSelected(context.Background(), config, doc, []string{"c2", "c35"})
		if err != nil || result.Report.Translated != 2 || result.Report.Resumed != 0 || result.Report.Rebuilt {
			t.Fatal(err, result.Report)
		}
		for i, cue := range result.Document.Cues {
			want := doc.Cues[i]
			if i == 2 || i == 35 {
				want.Text = strings.Replace(subtitle.SourceText(want), "Source", "译文", 1)
				want.Translated, want.TranslationSource = true, subtitle.SourceText(doc.Cues[i])
			}
			if !reflect.DeepEqual(cue, want) {
				t.Fatalf("cue %d changed outside selected text: %+v", i, cue)
			}
		}
	}
	if marshal(doc) != before || len(f.seen()) != 2 {
		t.Fatal("mutated input or replayed cache", f.seen())
	}
	for _, request := range f.seen() {
		if request.Operation != "translate" || len(request.Payload.Units) != 2 {
			t.Fatal("rebuilt or translated neighbors", request)
		}
		for _, line := range append(append([]unitLine{}, request.Payload.Units...), request.Payload.Neighbors...) {
			if line.Text == "Source 20" || line.Text == "人工旧译文 2" {
				t.Fatal("sent unrelated gap or translated a prior translation", line)
			}
		}
	}
}

func TestSelectedTranslationRejectsInvalidScopeAndCancellation(t *testing.T) {
	f, config := serve(t, ok)
	doc := testDocument(3)
	doc.Cues[1].ReadOnly = true
	for _, ids := range [][]string{nil, {"unknown"}, {"c1"}} {
		if _, err := TranslateSelected(context.Background(), config, doc, ids); err == nil {
			t.Fatal("accepted invalid scope", ids)
		}
	}
	if len(f.seen()) != 0 {
		t.Fatal("invalid selection sent a request")
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err := TranslateSelected(ctx, config, doc, []string{"c0"}); !errors.Is(err, context.Canceled) {
		t.Fatal(err)
	}
}

func TestSelectedCheckerUsesOriginalFormatAndOnlyChosenDialogue(t *testing.T) {
	client := subtitle.Client{Executable: os.Getenv("ONESUB_SUBTITLE_WORKER"), Scratch: t.TempDir()}
	checker := Checker{Executable: os.Getenv("ONESUB_SKILL_WORKER"), SkillRoot: os.Getenv("ONESUB_SKILL_ROOT"), Scratch: t.TempDir(), Subtitle: client}
	inputs := map[string]string{
		"srt": "1\n00:00:00,000 --> 00:00:04,000\n<i>Hello</i>\n\n2\n00:00:05,000 --> 00:00:09,000\nUnselected\n",
		"vtt": "WEBVTT\n\nfirst\n00:00:00.000 --> 00:00:04.000 align:start\n<i>Hello</i>\n\nsecond\n00:00:05.000 --> 00:00:09.000\nUnselected\n",
		"ass": "[Script Info]\nScriptType: v4.00+\nPlayResX: 1920\nPlayResY: 1080\n\n[V4+ Styles]\nFormat: Name, Fontname, Fontsize, PrimaryColour, SecondaryColour, OutlineColour, BackColour, Bold, Italic, Underline, StrikeOut, ScaleX, ScaleY, Spacing, Angle, BorderStyle, Outline, Shadow, Alignment, MarginL, MarginR, MarginV, Encoding\nStyle: Default,Arial,20,&H00FFFFFF,&H000000FF,&H00000000,&H00000000,0,0,0,0,100,100,0,0,1,2,0,2,10,10,10,1\n\n[Events]\nFormat: Layer, Start, End, Style, Name, MarginL, MarginR, MarginV, Effect, Text\nDialogue: 0,0:00:00.00,0:00:04.00,Default,,0,0,0,,{\\i1}Hello{\\i0}\nDialogue: 0,0:00:05.00,0:00:09.00,Default,,0,0,0,,Unselected\n",
	}
	for format, text := range inputs {
		t.Run(format, func(t *testing.T) {
			path := filepath.Join(t.TempDir(), "source."+format)
			if err := os.WriteFile(path, []byte(text), 0600); err != nil {
				t.Fatal(err)
			}
			doc, err := client.Read(context.Background(), path)
			if err != nil {
				t.Fatal(err)
			}
			candidate := doc
			candidate.Cues = append([]subtitle.Cue{}, doc.Cues...)
			candidate.Cues[0].Text = strings.Replace(doc.Cues[0].Text, "Hello", "你好", 1)
			findings, err := checker.CheckSelected(context.Background(), doc, candidate, []string{doc.Cues[0].ID}, "zh-CN")
			if err != nil || len(findings.Errors) != 0 || findings.Blocks != 1 {
				t.Fatal(err, findings)
			}
			candidate.Cues[0].Text = "你好"
			findings, err = checker.CheckSelected(context.Background(), doc, candidate, []string{doc.Cues[0].ID}, "zh-CN")
			if err == nil && len(findings.Errors) == 0 {
				t.Fatal("accepted removed markup", findings)
			}
		})
	}
}
