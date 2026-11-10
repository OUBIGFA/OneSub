package main

import (
	"context"
	"fmt"
	"math"
	"os"
	"path/filepath"
	"reflect"
	"strings"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

func TestProofreadingModeUsesBilingualListAndRestoresTabs(t *testing.T) {
	a := storeApp(t, subtitle.Cue{ID: "a", Text: "Source", StartMs: 100, EndMs: 2000})
	tt := settled(a.subtitlesView, 800, 600)
	if err := tt.Click("校对模式"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("校对字幕") || tt.HasText(locales.TabOriginal()) || tt.HasText(locales.TabRebuilt()) || !tt.HasText("Source") {
		t.Fatal("proofreading did not use the existing bilingual list")
	}
	if _, ok := tt.Find(locales.ListSave()); !ok {
		t.Fatal("toolbar disappeared")
	}
	if err := tt.Click("退出校对"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText(locales.TabOriginal()) || !tt.HasText(locales.TabRebuilt()) {
		t.Fatal("normal tabs did not return")
	}
}

func TestProofreadingImportsEitherSideFirstAndSavesBoth(t *testing.T) {
	for _, translationFirst := range []bool{false, true} {
		a := nativeForTest(t)
		a.toggleProofreading()
		a.worker = subtitle.Client{Executable: os.Getenv("ONESUB_SUBTITLE_WORKER"), Scratch: t.TempDir()}
		dir := t.TempDir()
		paths := []string{filepath.Join(dir, "source.srt"), filepath.Join(dir, "translation.srt")}
		for i, text := range []string{"Original source", "Translated text"} {
			if err := os.WriteFile(paths[i], []byte("1\n00:00:00,100 --> 00:00:02,000\n"+text+"\n"), 0600); err != nil {
				t.Fatal(err)
			}
		}
		order := []int{0, 1}
		if translationFirst {
			order = []int{1, 0}
		}
		for _, side := range order {
			a.importPaths([]string{paths[side]})
			if a.proofreadingImport == nil || !a.proofreadingImport.open || a.busy {
				t.Fatal("import bypassed role choice")
			}
			p := a.proofreadingImport
			p.roles[0], p.open = side, false
			apply, err := a.importProofreadingFiles(context.Background(), p)
			if err != nil {
				t.Fatal(err)
			}
			apply()
		}
		defer a.store.Close()
		cue := a.activeDocument().Cues[0]
		if cue.Text != "Translated text" || subtitle.SourceText(cue) != "Original source" {
			t.Fatal(cue)
		}
		if a.unsaved[0] || a.unsaved[1] || a.unsaved[2] {
			t.Fatal("unchanged imported files are dirty", a.unsaved)
		}
		a.editRebuiltSource = true
		a.loadDraft()
		a.draft = "Corrected source"
		a.changed(time.Now())
		if !a.commitEdit(true) || !a.unsaved[2] {
			t.Fatal("source correction not tracked")
		}
		a.editRebuiltSource = false
		a.loadDraft()
		a.draft = "Corrected translation"
		a.changed(time.Now())
		if !a.commitEdit(true) || !a.unsaved[1] {
			t.Fatal("translation correction not tracked")
		}
		apply, err := a.saveTracksWork([]int{2, 1}, false, nil)(context.Background())
		if err != nil {
			t.Fatal(err)
		}
		apply()
		for side, want := range []string{"Corrected source", "Corrected translation"} {
			doc, err := a.worker.Read(context.Background(), paths[side])
			if err != nil || doc.Cues[0].Text != want {
				t.Fatal("separate save failed", side, err, doc)
			}
		}
		a.toggleProofreading()
		if a.proofreading || a.unsaved[1] || a.unsaved[2] {
			t.Fatal("exit lost save state")
		}
	}
}

func TestProofreadingRoleCancelDoesNotReplaceProject(t *testing.T) {
	a := storeApp(t, subtitle.Cue{ID: "a", Text: "Keep me", StartMs: 0, EndMs: 2000})
	a.toggleProofreading()
	before := *a.snapshot
	a.importSubtitle("missing.srt")
	tt := settled(a.proofreadingImportView, 900, 640)
	if err := tt.Click(locales.ActionCancel()); err != nil {
		t.Fatal(err)
	}
	if !reflect.DeepEqual(before, *a.snapshot) || a.savePromptOpen || a.busy {
		t.Fatal("cancel mutated project")
	}
}

func TestProofreadingLayoutAndRoleDialogAreStable(t *testing.T) {
	for _, lang := range []string{"zh-Hans", "en"} {
		inLocale(t, lang, func() {
			a := storeApp(t, subtitle.Cue{ID: "a", Text: "Source", StartMs: 0, EndMs: 2000})
			a.toggleProofreading()
			for _, width := range []int{400, 680, 1000} {
				tt := settled(a.subtitlesView, width, 800)
				visibleRedesign(t, tt, locales.ProofreadingExit(), locales.ListImport(), locales.ListSave(), locales.ListSaveAs())
				captureRedesign(t, tt, fmt.Sprintf("proofreading-%s-%d", lang, width))
			}
			tt := ui.NewTester(a.proofreadingImportView, 900, 640)
			tt.SetPreferences(ui.Preferences{ReduceMotion: true})
			tt.Frame()
			firstFrameSettled(t, tt, "proofreading-import", func() { a.requestProofreadingImport([]string{"source.srt"}) })
			visibleRedesign(t, tt, locales.TabSource(), locales.TabTranslation(), locales.ActionCancel())
		})
	}
}

func TestSubtitleHeadersCenterExceptText(t *testing.T) {
	nativeForTest(t)
	var state ui.ListState
	widths := []float32{90, 180, 180, 400}
	titles := []string{"#", locales.ListColumnStart(), locales.ListColumnEnd(), locales.ListColumnText()}
	tt := settled(func(c *ui.Context) {
		cols := []ui.TableColumn{}
		for i, title := range titles {
			cols = append(cols, ui.TableColumn{Title: title, Width: widths[i], Fixed: true})
		}
		subtitleTable(c, &state, cols, 1, func(row, col int) { ui.Text(c, fmt.Sprintf("cell %d", col)) }, func(bool) {})
	}, 1000, 400)
	x := float32(0)
	pixels := tt.Image()
	for i, title := range titles {
		box, ok := tt.Find(title)
		if !ok {
			t.Fatal("header missing", title)
		}
		left, right := int(x+widths[i]), int(x)
		for y := int(box.Y) + 4; y < int(box.Y+box.H)-4; y++ {
			for px := int(x) + 4; px < int(x+widths[i])-4; px++ {
				color := pixels.RGBAAt(px, y)
				if color.R < 160 && color.G < 160 && color.B < 160 {
					left = min(left, px)
					right = max(right, px)
				}
			}
		}
		if right < left {
			t.Fatal("header ink missing", title, box)
		}
		if i < 3 && math.Abs(float64(left+right)/2-float64(x+widths[i]/2)) > 2 {
			t.Fatal("header ink is not centered", title, left, right, box)
		}
		if i == 3 && float32(left) > x+30 {
			t.Fatal("subtitle header is not left aligned", box)
		}
		x += widths[i]
	}
}

func TestProofreadingSavePreservesVTTTemplate(t *testing.T) {
	a := storeApp(t, subtitle.Cue{ID: "a", Text: "Source", StartMs: 100, EndMs: 2000})
	a.toggleProofreading()
	a.worker = subtitle.Client{Executable: os.Getenv("ONESUB_SUBTITLE_WORKER"), Scratch: t.TempDir()}
	path := filepath.Join(t.TempDir(), "translated.vtt")
	raw := "WEBVTT\n\nNOTE retained metadata\n\n1\n00:00:00.100 --> 00:00:02.000\nTranslation\n"
	if err := os.WriteFile(path, []byte(raw), 0600); err != nil {
		t.Fatal(err)
	}
	p := &proofreadingImport{subtitles: []string{path}, roles: []int{1}}
	apply, err := a.importProofreadingFiles(context.Background(), p)
	if err != nil {
		t.Fatal(err)
	}
	apply()
	a.draft = "Corrected VTT"
	a.changed(time.Now())
	if !a.commitEdit(true) {
		t.Fatal("edit failed")
	}
	apply, err = a.saveTracksWork([]int{1}, false, nil)(context.Background())
	if err != nil {
		t.Fatal(err)
	}
	apply()
	data, err := os.ReadFile(path)
	if err != nil || !strings.Contains(string(data), "NOTE retained metadata") || !strings.Contains(string(data), "Corrected VTT") {
		t.Fatal("VTT metadata lost", err, string(data))
	}
}

func TestEmptyWorkbenchCanEnterProofreading(t *testing.T) {
	a := nativeForTest(t)
	tt := settled(a.subtitlesView, 400, 600)
	if err := tt.Click("校对模式"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("校对字幕") {
		t.Fatal("empty proofreading entry missing")
	}
}
