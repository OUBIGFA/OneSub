package subtitle

import (
	"context"
	"fmt"
	"reflect"
	"strings"
	"testing"
	"unicode"
)

func TestPostSplitUntimedKeepsPunctuationBeforeBalancing(t *testing.T) {
	for _, parts := range [][]string{
		{"We first inspect the available materials in the project.", "Next we check all the lights.", "Finally we save the complete collection of reusable assets."},
		{"首先检查所有模型。", "接下来仔细核对场景里面的灯光。", "最后保存所有修改。"},
		{"Nous vérifions les modèles.", "Ensuite nous inspectons toutes les lumières du projet.", "Enfin nous sauvegardons le résultat."},
		{"Primero revisamos los modelos.", "Después comprobamos todas las luces del proyecto.", "Finalmente guardamos los cambios."},
		{"最初にモデルを確認します。", "次にすべての照明を確認します。", "最後に変更を保存します。"},
	} {
		text := strings.Join(parts, " ")
		cue := Cue{ID: "import", Text: text, StartMs: 1200, EndMs: 16200}
		r, err := SmartSplit(context.Background(), Document{Format: "srt", Cues: []Cue{cue}}, "post", true, PunctuationOptions{})
		if err != nil || len(r.Document.Cues) != len(parts) {
			t.Fatal(r, err)
		}
		for i, want := range parts {
			if strings.TrimSpace(r.Document.Cues[i].Text) != want {
				t.Fatalf("sentence boundary lost: %+v", r.Document.Cues)
			}
		}
		assertUntimedPostSplit(t, cue, r)
	}
}

func TestPostSplitUntimedClauseAndProtectedPhrase(t *testing.T) {
	for _, pair := range [][2]string{
		{"We can inspect the models", "because the complete collection is ready for the project."},
		{"Perhaps this is a bit too bright", "so we can change the gamma setting before saving."},
		{"We can use black or white models", "and then inspect every material in the complete project."},
		{"We should finish the remaining checks", "as soon as the client approves all the changes."},
		{"It helps us understand a little more about the", "so because the environment light uses the image as its source,"},
		{"我们需要仔细检查模型里面的所有设置", "然后我们继续检查灯光和材质的配置。"},
	} {
		cue := Cue{ID: "import", Text: strings.Join(pair[:], " "), StartMs: 2000, EndMs: 14000}
		r, err := SmartSplit(context.Background(), Document{Format: "srt", Cues: []Cue{cue}}, "post", true, PunctuationOptions{})
		if err != nil || len(r.Document.Cues) != 2 {
			t.Fatal(r, err)
		}
		for i, want := range pair {
			if strings.TrimSpace(r.Document.Cues[i].Text) != want {
				t.Fatalf("clause/phrase was split incorrectly: %+v", r.Document.Cues)
			}
		}
		assertUntimedPostSplit(t, cue, r)
	}
}

func assertUntimedPostSplit(t *testing.T, input Cue, report SplitReport) {
	t.Helper()
	assertSplitEnvelope(t, input, report.Document.Cues)
	if report.Estimated != len(report.Document.Cues) {
		t.Fatal("missing estimated timing label", report)
	}
	for _, c := range report.Document.Cues {
		if c.TimingQuality != "estimated" || len(c.Words) > 0 || c.Text != strings.TrimLeftFunc(c.Text, unicode.IsSpace) {
			t.Fatal("invented word clock or leading whitespace", c)
		}
	}
}

func TestPostSplitUntimedMultilingualEnvelope(t *testing.T) {
	for _, text := range []string{
		"我们先检查所有模型的材质设置，然后调整光源的位置，最后保存整个场景的修改。",
		"我們先檢查所有模型的材質設定，然後調整光源的位置，最後儲存整個場景的修改。",
		"오늘은 모든 모델의 재질을 확인합니다. 다음으로 조명의 위치를 조정합니다. 마지막으로 변경 사항을 저장합니다.",
		"วันนี้เราจะตรวจสอบการทำงานของระบบอย่างละเอียด จากนั้นเราจะตรวจสอบผลลัพธ์ทั้งหมดก่อนดำเนินการต่อไป",
		"Zuerst prüfen wir alle Modelle. Danach überprüfen wir die Beleuchtung. Zum Schluss speichern wir das Ergebnis.",
		"Сначала мы проверяем все модели. Затем проверяем освещение сцены. В конце сохраняем все изменения.",
		"نراجع جميع النماذج في المشهد، ثم نفحص إعدادات الإضاءة، وبعد ذلك نحفظ كل التغييرات في المشروع.",
		"首先检查 Render Settings 里面的配置，然后使用 OpenEXR 保存完整的渲染结果。",
	} {
		cue := Cue{ID: "import", Text: text, StartMs: 1200, EndMs: 21200}
		input := Document{Format: "srt", Cues: []Cue{cue}}
		r, err := SmartSplit(context.Background(), input, "post", true, PunctuationOptions{})
		if err != nil || r.Changed != 1 {
			t.Fatal(r, err)
		}
		assertUntimedPostSplit(t, cue, r)
		if !reflect.DeepEqual(input.Cues, []Cue{cue}) {
			t.Fatal("mutated input")
		}
	}
}

func TestPostSplitUntimedBilingualAndTimingCapacity(t *testing.T) {
	for _, format := range []string{"srt", "vtt", "ass", "ssa"} {
		cue := Cue{ID: "paired", Text: "首先检查所有材质。然后检查灯光的位置。最后保存整个场景。", TranslationSource: "We inspect every material. Then we check all the lights. Finally we save the scene.", Translated: true, StartMs: 1000, EndMs: 13000}
		r, err := SmartSplit(context.Background(), Document{Format: format, Cues: []Cue{cue}}, "pair", false, PunctuationOptions{})
		if err != nil || r.Changed != 1 {
			t.Fatal(r, err)
		}
		assertUntimedPostSplit(t, cue, r)
		for _, c := range r.Document.Cues {
			if !c.SplitReview || strings.TrimSpace(c.TranslationSource) == "" {
				t.Fatal("lost paired lane/review state", c)
			}
			if format == "ass" || format == "ssa" {
				if c.StartMs%10 != 0 || c.EndMs%10 != 0 {
					t.Fatal("invalid ASS time precision")
				}
			}
		}
	}
	text := "We inspect every material in this collection. We check all the lights carefully. We save the final project for the client."
	cue := Cue{ID: "short-clock", Text: text, StartMs: 1000, EndMs: 3500}
	r, err := SmartSplit(context.Background(), Document{Format: "srt", Cues: []Cue{cue}}, "capacity", true, PunctuationOptions{})
	if err != nil || r.Changed != 1 || len(r.Document.Cues) != 2 {
		t.Fatal("failed to fit event time capacity", r, err)
	}
	assertUntimedPostSplit(t, cue, r)
	protected := Cue{ID: "tagged", Text: "<i>" + text + "</i>", StartMs: 1000, EndMs: 16000}
	r, err = SmartSplit(context.Background(), Document{Format: "srt", Cues: []Cue{protected}}, "protect", true, PunctuationOptions{})
	if err != nil || r.Changed != 0 || !reflect.DeepEqual(r.Document.Cues, []Cue{protected}) {
		t.Fatal("altered protected markup", r, err)
	}
	ctx, cancel := context.WithCancel(context.Background())
	cancel()
	if _, err = SmartSplit(ctx, Document{Format: "srt", Cues: []Cue{cue}}, "cancel", true, PunctuationOptions{}); err == nil {
		t.Fatal("ignored cancellation")
	}
}

func TestPostSplitKeepsEachImportedCueInItsOwnTimeRange(t *testing.T) {
	text := "We first inspect all the materials in the project. Then we save every change for the client."
	for _, gap := range []int64{0, 5000, -1000} {
		first := Cue{ID: "one", Text: text, StartMs: 1000, EndMs: 10000}
		second := Cue{ID: "two", Position: 1, Text: text, StartMs: 10000 + gap, EndMs: 19000 + gap}
		r, err := SmartSplit(context.Background(), Document{Format: "srt", Cues: []Cue{first, second}}, "range", true, PunctuationOptions{})
		if err != nil || r.Changed != 2 {
			t.Fatal(r, err)
		}
		for _, original := range []Cue{first, second} {
			var parts []Cue
			for _, c := range r.Document.Cues {
				if len(c.SplitCueIDs) == 1 && c.SplitCueIDs[0] == original.ID {
					parts = append(parts, c)
				}
			}
			assertSplitEnvelope(t, original, parts)
		}
	}
}

func BenchmarkPostSplitThirdParty1000Cues(b *testing.B) {
	doc := Document{Format: "srt"}
	for i := range 1000 {
		text := "We inspect every model carefully. Then we review all the lighting. Finally we save the complete collection."
		if i%2 == 1 {
			text = "首先检查模型中的所有材质。然后调整灯光的位置。最后保存完整场景。"
		}
		doc.Cues = append(doc.Cues, Cue{ID: fmt.Sprint(i), Text: text, StartMs: int64(i) * 16000, EndMs: int64(i)*16000 + 15000})
	}
	if r, err := SmartSplit(context.Background(), doc, "warm", true, PunctuationOptions{}); err != nil || r.Changed != 1000 {
		b.Fatal("benchmark warmup failed", err)
	}
	b.ResetTimer()
	b.ReportAllocs()
	for range b.N {
		r, err := SmartSplit(context.Background(), doc, "bench", true, PunctuationOptions{})
		if err != nil || r.Changed != 1000 {
			b.Fatal(r.Changed, err)
		}
	}
}
