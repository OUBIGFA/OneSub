package translator

import (
	"context"
	"reflect"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
)

// Cleanup is a projection of accepted output, not a prompt change or another
// AI request. Cached/resumed raw answers can be projected with new options.
func TestTranslationPunctuationIsLocalAndPreservesSource(t *testing.T) {
	f, config := serve(t, func(s seen, _ int) (string, string, int) {
		return `{"1":"First 0. Next。 Done."}`, "stop", 0
	})
	config.TargetLanguage = "en"
	doc := testDocument(1)
	before := doc.Cues[0]
	config.Punctuation = &subtitle.PunctuationOptions{}
	plain, err := Translate(context.Background(), config, doc)
	if err != nil {
		t.Fatal(err)
	}
	config.Punctuation = &subtitle.PunctuationOptions{StripTrailing: true, ReplacePeriods: true}
	cleaned, err := Translate(context.Background(), config, doc)
	if err != nil {
		t.Fatal(err)
	}
	if len(f.seen()) != 1 || plain.Document.Cues[0].Text != "First 0. Next。 Done." || cleaned.Document.Cues[0].Text != "First 0, Next， Done" {
		t.Fatal("cleanup changed requests or output", len(f.seen()), plain.Document.Cues, cleaned.Document.Cues)
	}
	if !reflect.DeepEqual(doc.Cues[0], before) || cleaned.Document.Cues[0].StartMs != before.StartMs || cleaned.Document.Cues[0].TranslationSource != before.Text {
		t.Fatal("cleanup changed source/timing")
	}
	// With a fresh cache, requests are identical too.
	config.CacheDir = t.TempDir()
	_, err = Translate(context.Background(), config, doc)
	requests := f.seen()
	if err != nil || len(requests) != 2 || !reflect.DeepEqual(requests[0], requests[1]) {
		t.Fatal("options leaked into provider request", err)
	}
}
