package translator

import (
	"context"
	"reflect"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
)

// Exercise the real provider/cache/projection path, not only Apply: changing
// local options must never alter literal data, the source, or the AI request.
func TestTranslationPunctuationPreservesEmailNumbersAndCachedReply(t *testing.T) {
	const raw = "Email AsiaPacific@bbc.co.uk. Use 3.14. Done."
	const source = "向AsiaPacific@bbc.co.uk发送邮件。使用3.14。完成。"
	f, config := serve(t, func(_ seen, _ int) (string, string, int) {
		return `{"1":"Email AsiaPacific@bbc.co.uk. Use 3.14. Done."}`, "stop", 0
	})
	config.TargetLanguage = "en"
	doc := testDocument(1)
	doc.Cues[0].Text, doc.Cues[0].OriginalText = source, source
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
	if len(f.seen()) != 1 || plain.Document.Cues[0].Text != raw || cleaned.Document.Cues[0].Text != "Email AsiaPacific@bbc.co.uk, Use 3.14, Done" {
		t.Fatal("cleanup changed literal data or made another request", len(f.seen()), cleaned.Document.Cues)
	}
	if !reflect.DeepEqual(doc.Cues[0], before) || cleaned.Document.Cues[0].TranslationSource != source || cleaned.Document.Cues[0].StartMs != before.StartMs || cleaned.Document.Cues[0].EndMs != before.EndMs {
		t.Fatal("cleanup changed the source or timeline")
	}
}
