package main

import (
	"context"
	"math"
	"os"
	"reflect"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/styles"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

func withStyleCatalog(t *testing.T) *NativeApp {
	t.Helper()
	a := nativeForTest(t)
	client := styles.Client{Executable: os.Getenv("ONESUB_STYLE_WORKER"), Scratch: t.TempDir()}
	catalog, err := client.Catalog(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	a.styleCatalog = catalog
	a.currentStyle = catalog.DefaultStyle
	a.stylePreset = "默认"
	a.snapshot = &project.Snapshot{Document: subtitle.Document{Format: "srt", Cues: []subtitle.Cue{{ID: "c1", Text: "source", OriginalText: "source", StartMs: 0, EndMs: 2000}}}}
	a.selected = 0
	a.loadDraft()
	return a
}

func TestWorkbenchRegionsAndInlineMargins(t *testing.T) {
	a := withStyleCatalog(t)
	tt := ui.NewTester(a.view, 1700, 1000)
	video, ok := tt.Find("视频播放区")
	if !ok {
		t.Fatal(tt.Texts())
	}
	original, ok := tt.Find("原文")
	if !ok {
		t.Fatal(tt.Texts())
	}
	editor, _ := tt.Find("字幕编辑")
	style, _ := tt.Find("字幕样式")
	if original.X <= video.X || editor.Y <= original.Y || style.X <= editor.X || math.Abs(float64(style.Y-editor.Y)) > 1 {
		t.Fatal(video, original, editor, style)
	}
	left, ok := tt.Find("左边距")
	if !ok {
		t.Fatal("missing left margin")
	}
	right, ok := tt.Find("右边距")
	if !ok || left.Y != right.Y {
		t.Fatal("margins not inline", left, right)
	}
	for _, label := range []string{"更新预览", "更多样式", "属性"} {
		if tt.HasText(label) {
			t.Fatal("obsolete tab/manual preview", label)
		}
	}
	before := a.previewRevision
	if err := tt.Click("粗体"); err != nil {
		t.Fatal(err)
	}
	if !a.currentStyle.Bold || a.previewRevision <= before || !a.previewPending || a.busy {
		t.Fatal("bold did not queue non-blocking preview")
	}
	before = a.previewRevision
	if err := tt.Click("斜体"); err != nil {
		t.Fatal(err)
	}
	if !a.currentStyle.Italic || a.previewRevision <= before {
		t.Fatal("italic did not queue preview")
	}
}

func TestEveryStyleFieldInvalidatesPreview(t *testing.T) {
	a := withStyleCatalog(t)
	a.observeStyle(time.Now())
	// Reflection intentionally enumerates the production style schema: adding a
	// parameter without automatic invalidation must fail this test.
	value := reflect.ValueOf(&a.currentStyle).Elem()
	for i := 0; i < value.NumField(); i++ {
		field := value.Field(i)
		before := a.previewRevision
		switch field.Kind() {
		case reflect.String:
			field.SetString(field.String() + " changed")
		case reflect.Float64:
			field.SetFloat(field.Float() + 1)
		case reflect.Int:
			field.SetInt(field.Int() + 1)
		case reflect.Bool:
			field.SetBool(!field.Bool())
		case reflect.Slice:
			field.Set(reflect.ValueOf([]string{"term"}))
		default:
			t.Fatalf("uncovered style field: %s", value.Type().Field(i).Name)
		}
		a.observeStyle(time.Now())
		if a.previewRevision != before+1 || !a.previewPending || a.busy {
			t.Fatal("not invalidated", value.Type().Field(i).Name)
		}
	}
}
func TestInitialWindowFitsWorkAreaWithoutDoubleScaling(t *testing.T) {
	for _, work := range []mygo.Rectangle{{Width: 3440, Height: 1392}, {Width: 1920, Height: 1040}, {Width: 1280, Height: 680}, {X: -1280, Y: 100, Width: 1280, Height: 720}} {
		got := initialWindowBounds(work)
		if got.X < work.X || got.Y < work.Y || got.X+got.Width > work.X+work.Width || got.Y+got.Height > work.Y+work.Height {
			t.Fatal(work, got)
		}
		if got.Height < work.Height*8/10 {
			t.Fatal("default remains too small", work, got)
		}
		if work.Width >= 1920 && got.Width <= 1280 {
			t.Fatal("large display still uses old default", got)
		}
	}
}
func TestSourceAndTranslationTabsRemainDistinct(t *testing.T) {
	cue := subtitle.Cue{Text: "edited source", OriginalText: "raw source"}
	if cueDisplayText(cue, 0) != "edited source" || cueDisplayText(cue, 1) != "" {
		t.Fatal("untranslated source appeared as translation")
	}
	cue.Translated = true
	cue.TranslationSource = "edited source"
	cue.Text = "译文"
	if cueDisplayText(cue, 0) != "edited source" || cueDisplayText(cue, 1) != "译文" {
		t.Fatal("source overwritten by translation")
	}
}
