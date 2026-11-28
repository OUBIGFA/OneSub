package translator

import (
	"context"
	"encoding/json"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
)

// Capture the actual service request: retry and single-line repair must carry
// the same budget, and a short clock must never fabricate room for 12 letters.
func TestBudgetTravelsThroughRetries(t *testing.T) {
	f, config := serve(t, func(s seen, n int) (string, string, int) {
		if n < 3 {
			return `{}`, "stop", 0
		}
		return translated(s), "stop", 0
	})
	doc := subtitle.Document{Format: "srt", Cues: []subtitle.Cue{{ID: "one", Text: "Source text", StartMs: 1000, EndMs: 1200}}}
	result, err := Translate(context.Background(), config, doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(result.Document.Cues) != 1 {
		t.Fatal(result)
	}
	for _, request := range f.seen() {
		if request.Operation != "translate" {
			continue
		}
		var payload struct {
			Units []struct {
				DurationMs    int64
				ReadingBudget *struct {
					Units, MaxLineUnits float64
					Counting            string
				}
			}
		}
		if err := json.Unmarshal([]byte(request.Messages[0].Content), &payload); err != nil {
			t.Fatal(err)
		}
		if len(payload.Units) != 1 || payload.Units[0].DurationMs != 200 || payload.Units[0].ReadingBudget == nil {
			t.Fatalf("missing budget: %s", request.Messages[0].Content)
		}
		b := payload.Units[0].ReadingBudget
		if b.Units != 0.9 || b.MaxLineUnits != 25 || b.Counting != "cjk" {
			t.Fatalf("wrong short budget: %+v", b)
		}
	}
}

func TestBudgetClockInvalidatesCache(t *testing.T) {
	f, config := serve(t, ok)
	doc := subtitle.Document{Format: "srt", Cues: []subtitle.Cue{{ID: "one", Text: "Source text", StartMs: 0, EndMs: 1000}}}
	if _, err := Translate(context.Background(), config, doc); err != nil {
		t.Fatal(err)
	}
	before := len(f.seen())
	doc.Cues[0].EndMs = 6000
	if _, err := Translate(context.Background(), config, doc); err != nil {
		t.Fatal(err)
	}
	if len(f.seen()) == before {
		t.Fatal("changed clock reused old translation")
	}
}

func TestBudgetLegacyResumeAndSelected(t *testing.T) {
	f, config := serve(t, ok)
	doc := subtitle.Document{Format: "srt", Cues: []subtitle.Cue{{ID: "one", Text: "Source text", StartMs: 0, EndMs: 3000}}}
	first, err := Translate(context.Background(), config, doc)
	if err != nil {
		t.Fatal(err)
	}
	if first.Resume.BudgetVersion != readingBudgetVersion {
		t.Fatal(first.Resume)
	}
	old := *first.Resume
	old.BudgetVersion = 0
	old.BudgetRules = ""
	old.Completed = nil
	config.Resume = &old
	config.CacheDir = ""
	if _, err = Translate(context.Background(), config, doc); err != nil {
		t.Fatal(err)
	}
	if last := f.seen()[len(f.seen())-1]; last.Payload.Units[0].ReadingBudget != nil {
		t.Fatal("legacy run changed mode")
	}
	if _, err = TranslateSelected(context.Background(), config, doc, []string{"one"}); err != nil {
		t.Fatal(err)
	}
	if last := f.seen()[len(f.seen())-1]; last.Payload.Units[0].ReadingBudget == nil {
		t.Fatal("selected translation lost budget")
	}
}
