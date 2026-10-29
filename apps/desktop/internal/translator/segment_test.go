package translator

import (
	"context"
	"os"
	"strings"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// fragments is a tutorial's opening, its third block a trailing modifier
// of the second: the case the skill's segmentation reference names.
func fragments() subtitle.Document {
	cue := func(id string, start, end int64, text string) subtitle.Cue {
		return subtitle.Cue{ID: id, StartMs: start, EndMs: end, Text: text, OriginalText: text}
	}
	return subtitle.Document{Format: "srt", Cues: []subtitle.Cue{
		cue("c0", 7920, 10000, "Okay, to start with, I'll use a circle mesh"),
		cue("c1", 10880, 14640, "As you can see in the reference image, the object has three identical sections"),
		cue("c2", 14720, 17600, "each spaced exactly 120 degrees apart"),
		cue("c3", 17680, 22720, "Because of this, we need to use a circle with a vertex count that's divisible by three"),
	}}
}

// lines answers a segmentation request with the given subtitle lines and a
// translation request with every unit translated.
func lines(text string) func(seen, int) (string, string, int) {
	return func(s seen, _ int) (string, string, int) {
		if s.Operation == "segment" {
			return text, "stop", 0
		}
		ids, values := []string{}, map[string]string{}
		for _, u := range s.Payload.Units {
			ids = append(ids, u.ID)
			values[u.ID] = "译：" + u.Text
		}
		return orderedObject(ids, values), "stop", 0
	}
}

// The model joins a trailing modifier to its sense group; OneSub keeps the
// source's words and times, so the joined subtitle spans both blocks. The
// checker then compares the result with the source's speech.
func TestSenseGroupsJoinFragmentsAndKeepTimes(t *testing.T) {
	doc := fragments()
	f, config := serve(t, lines("Okay, to start with, I'll use a circle mesh\nAs you can see in the reference image, the object has three identical sections each spaced exactly 120 degrees apart\nBecause of this, we need to use a circle with a vertex count that's divisible by three"))
	config.OptimizeTimeline = true
	result, err := Translate(context.Background(), config, doc)
	if err != nil {
		t.Fatal(err)
	}
	requests := f.seen()
	if requests[0].Operation != "segment" || !strings.Contains(requests[0].Messages[0].Content, "[10.88→14.64] As you can see") || !strings.Contains(requests[0].System, "## references/segmentation.md") {
		t.Fatal("segmentation request without the source spans or the skill's rules")
	}
	cues := result.Document.Cues
	if len(cues) != 3 || cues[1].StartMs != 10880 || cues[1].EndMs != 17600 || !result.Report.Rebuilt || result.Report.TimelineChanges != 1 {
		t.Fatal(cues, result.Report)
	}
	if source := subtitle.SourceText(cues[1]); source != "As you can see in the reference image, the object has three identical sections each spaced exactly 120 degrees apart" {
		t.Fatal(source)
	}
	if cues[1].SourceRange.Reason != locales.ReasonJoined() || !strings.Contains(strings.Join(result.Report.Warnings, "\n"), locales.ReasonJoined()) {
		t.Fatal(cues[1].SourceRange, result.Report.Warnings)
	}
	if err = subtitle.ValidateTranslation(doc, result.Document); err != nil {
		t.Fatal(err)
	}
	checker := Checker{Executable: os.Getenv("ONESUB_SKILL_WORKER"), SkillRoot: config.SkillRoot, Scratch: t.TempDir(), Subtitle: subtitle.Client{Executable: os.Getenv("ONESUB_SUBTITLE_WORKER"), Scratch: t.TempDir()}}
	findings, err := checker.Check(context.Background(), doc, result.Document, "zh-CN", false)
	if err != nil || len(findings.Errors) != 0 {
		t.Fatal(findings, err)
	}
}

// A pause longer than maxJoinGap is never joined over: the subtitle would
// stand through the silence.
func TestSenseGroupsNeverJoinOverAPause(t *testing.T) {
	doc := fragments()
	doc.Cues[2].StartMs, doc.Cues[2].EndMs = 16700, 19000
	doc.Cues[3].StartMs, doc.Cues[3].EndMs = 19100, 22720
	_, config := serve(t, lines("Okay, to start with, I'll use a circle mesh\nAs you can see in the reference image, the object has three identical sections each spaced exactly 120 degrees apart\nBecause of this, we need to use a circle with a vertex count that's divisible by three"))
	config.OptimizeTimeline = true
	result, err := Translate(context.Background(), config, doc)
	if err != nil || len(result.Document.Cues) != 4 || result.Report.Rebuilt {
		t.Fatal(err, len(result.Document.Cues))
	}
}

// With word timing, a long subtitle divides at a sense-group boundary, at
// the time its second part's first word starts; without it, a line the
// model divides stays whole.
func TestSenseGroupsDivideOnlyWithWordTiming(t *testing.T) {
	text := "Select the inner faces and extrude them along their normals by twenty centimeters"
	words := []subtitle.Word{}
	at := int64(0)
	for i, w := range strings.Fields(text) {
		piece := w
		if i > 0 {
			piece = " " + w
		}
		words = append(words, subtitle.Word{Text: piece, StartMs: at, EndMs: at + 400})
		at += 450
	}
	doc := subtitle.Document{Format: "srt", Cues: []subtitle.Cue{{ID: "c0", StartMs: 0, EndMs: at, Text: text, OriginalText: text, Words: words, TimingQuality: "aligned"}}}
	answer := lines("Select the inner faces\nand extrude them along their normals by twenty centimeters")
	_, config := serve(t, answer)
	config.OptimizeTimeline = true
	result, err := Translate(context.Background(), config, doc)
	if err != nil {
		t.Fatal(err)
	}
	cues := result.Document.Cues
	if len(cues) != 2 || cues[0].EndMs != 4*450 || cues[1].StartMs != 4*450 || cues[1].SourceRange.Reason != locales.ReasonDivided() {
		t.Fatal(cues)
	}
	// The second piece starts with its word, and its words still spell it,
	// so it can be divided again.
	if source := subtitle.SourceText(cues[1]); source != "and extrude them along their normals by twenty centimeters" || len(cues[1].Words) != 9 {
		t.Fatalf("%q %d", source, len(cues[1].Words))
	}
	if _, err = subtitle.TimelineAtoms(result.Source); err != nil {
		t.Fatal(err)
	}
	if err = subtitle.ValidateTranslation(doc, result.Document); err != nil {
		t.Fatal(err)
	}
	doc.Cues[0].Words = nil
	_, config = serve(t, answer)
	config.OptimizeTimeline = true
	if result, err = Translate(context.Background(), config, doc); err != nil || len(result.Document.Cues) != 1 || result.Report.Rebuilt {
		t.Fatal(err, result.Document.Cues)
	}
}

// A piece divided out of a subtitle shorter than minPiece rejoins its
// neighbor in that subtitle, rather than flash by.
func TestSenseGroupsShortPiecesRejoin(t *testing.T) {
	text := "Okay now open the settings panel"
	words := []subtitle.Word{}
	for i, w := range strings.Fields(text) {
		piece := w
		if i > 0 {
			piece = " " + w
		}
		words = append(words, subtitle.Word{Text: piece, StartMs: int64(i) * 300, EndMs: int64(i)*300 + 250})
	}
	doc := subtitle.Document{Format: "srt", Cues: []subtitle.Cue{{ID: "c0", StartMs: 0, EndMs: 2400, Text: text, OriginalText: text, Words: words}}}
	_, config := serve(t, lines("Okay\nnow open the settings panel"))
	config.OptimizeTimeline = true
	result, err := Translate(context.Background(), config, doc)
	if err != nil || len(result.Document.Cues) != 1 {
		t.Fatal(err, result.Document.Cues)
	}
}

// An answer that rewrites the words is asked once more, then the window
// keeps its source boundaries, with a warning; the translation goes on.
func TestSegmentAnswerThatRewordsKeepsTheSource(t *testing.T) {
	f, config := serve(t, lines("We begin with a circle.\nThe reference shows three equal parts, 120 degrees apart."))
	config.OptimizeTimeline = true
	result, err := Translate(context.Background(), config, fragments())
	if err != nil {
		t.Fatal(err)
	}
	segments := 0
	for _, r := range f.seen() {
		if r.Operation == "segment" {
			segments++
		}
	}
	if segments != 2 || len(result.Document.Cues) != 4 || result.Report.Rebuilt || !strings.Contains(strings.Join(result.Report.Warnings, "\n"), locales.WarnSegmentKept("00:00:07", "00:00:22")) {
		t.Fatal(segments, len(result.Document.Cues), result.Report.Warnings)
	}
}

// ASS and VTT keep their events: the timeline is rebuilt only for SRT.
func TestSenseGroupsOnlyForSRT(t *testing.T) {
	doc := fragments()
	doc.Format = "vtt"
	f, config := serve(t, lines("unused"))
	config.OptimizeTimeline = true
	if _, err := Translate(context.Background(), config, doc); err != nil {
		t.Fatal(err)
	}
	for _, r := range f.seen() {
		if r.Operation == "segment" {
			t.Fatal("a VTT timeline was sent for segmentation")
		}
	}
}
