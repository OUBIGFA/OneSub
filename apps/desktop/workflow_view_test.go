package main

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

func TestWorkflowFieldsHaveTaskHierarchy(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := nativeForTest(t)
			a.settings.RecognitionMode = recognitionManual
			tt := settled(a.workflowPanel, 1200, 1000)
			for range 4 {
				tt.Frame()
			}
			model, _ := tt.Find(locales.WorkflowRecognitionModel())
			spoken, _ := tt.Find(locales.SettingsSourceLanguage())
			format, _ := tt.Find(locales.ExportFormat())
			folder, _ := tt.Find(locales.RecognitionDirectory())
			if model.H <= 0 || math.Abs(float64(model.Y-spoken.Y)) > 1 || format.Y <= model.Y+model.H || folder.Y <= format.Y+format.H {
				t.Fatal("recognition, output format and destination lack hierarchy", model, spoken, format, folder)
			}
			location, _ := tt.Find(locales.RecognitionBesideMedia())
			choose, _ := tt.Find(locales.RecognitionChooseFolder())
			if math.Abs(float64(location.Y+location.H/2-choose.Y-choose.H/2)) > 1 || choose.X <= location.X {
				t.Fatal("destination and action are not aligned", location, choose)
			}
			decoding, _ := tt.Find(locales.WorkflowRecognitionAdvanced())
			if !tt.HasText(locales.SettingsMaxTokens()) || tt.HasText(locales.WorkflowStripPunctuation()) || decoding.Y <= folder.Y+folder.H {
				t.Fatal("expanded decoding does not remain below recognition output")
			}
		})
	}
}

func TestWorkflowFolderActionsStayInsideNarrowPanels(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		for _, width := range []int{320, 450, 800} {
			for _, state := range [][2]int{{0, 0}, {0, 1}, {1, 0}} {
				t.Run(fmt.Sprintf("%s/%d/%v", language, width, state), func(t *testing.T) {
					inLocale(t, language, func() {
						a := nativeForTest(t)
						a.workflowTab, a.recognitionMode = state[0], state[1]
						a.settings.RecognitionDir = strings.Repeat("folder/", 50)
						a.settings.TranslationDir = a.settings.RecognitionDir
						tt := settled(a.workflowPanel, width, 700)
						for range 4 {
							tt.Frame()
						}
						if a.recognitionMode == 1 {
							for _, label := range []string{locales.BatchAddFiles(), locales.BatchStart(), locales.BatchRemove()} {
								box, ok := tt.Find(label)
								if !ok || box.X < 0 || box.X+box.W > float32(width) || box.Y+box.H > 700 {
									t.Fatal("clipped queue control", label, box)
								}
							}
							area, ok := tt.Find(locales.BatchDropArea())
							if !ok || area.H < 300 || area.X < 0 || area.X+area.W > float32(width) || area.Y+area.H > 700 {
								t.Fatal("queue does not use the available panel", area)
							}
							return
						}
						if a.workflowScroll.MaxY > 1 {
							tt.Scroll(float32(width)/2, 400, 0, 2000)
						}
						for _, label := range []string{locales.RecognitionChooseFolder(), locales.RecognitionResetFolder(), locales.RecognitionDirectory()} {
							box, ok := tt.Find(label)
							if !ok || box.W <= 0 || box.X < 0 || box.X+box.W > float32(width) || box.Y+box.H > 700 {
								t.Fatal("clipped folder control", label, box)
							}
						}
					})
				})
			}
		}
	}
}

// Scheduling remains expanded. Its peer fields remain aligned,
// reachable, and persistent, including edits intended for the next task.
func TestTranslationSchedulingOnWorkbench(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := nativeForTest(t)
			a.workflowTab = 1
			tt := settled(a.workflowPanel, 800, 1100)
			for range 4 {
				tt.Frame()
			}
			if !tt.HasText(locales.TranslationConcurrency()) {
				t.Fatal("scheduling is not expanded")
			}
			for range 4 {
				tt.Frame()
			}
			concurrency, _ := tt.Find(locales.TranslationConcurrency())
			batch, _ := tt.Find(locales.TranslationBatchSize())
			model, _ := tt.Find(locales.TranslationModel())
			if concurrency.H <= 0 || math.Abs(float64(concurrency.Y-batch.Y)) > 1 || concurrency.Y <= model.Y+model.H {
				t.Fatal("scheduling is not a separate aligned group", concurrency, batch, model)
			}
			a.busy = true
			tt.Frame()
			for _, field := range []struct{ label, value string }{{locales.TranslationConcurrency(), "4"}, {locales.TranslationBatchSize(), "35"}} {
				if err := tt.Click(field.label); err != nil {
					t.Fatal(err)
				}
				tt.Key(ui.Cmd, ui.KeyA)
				tt.Type(field.value)
				tt.Key(0, ui.KeyEnter)
			}
			prefs, err := settings.Load(filepath.Join(a.dataRoot, "settings.json"))
			if err != nil || prefs.TranslationConcurrency != 4 || prefs.TranslationBatchSize != 35 {
				t.Fatal("scheduling changes not saved", err, prefs.TranslationConcurrency, prefs.TranslationBatchSize)
			}
		})
	}
}

// A shortened inspector may scroll, but neither primary actions nor user-owned
// divider ratios move with its content. No horizontal parameter overflow.
// Both languages run at the shortest desktop height; the ultrawide case uses
// English, whose longer labels are the first to overflow.
func TestDesktopInspectorScrollKeepsCommandsAndDivider(t *testing.T) {
	for _, c := range []struct {
		language string
		size     [2]int
	}{{"zh-Hans", [2]int{1920, 1040}}, {"en", [2]int{1920, 1040}}, {"en", [2]int{3440, 1392}}} {
		language, size := c.language, c.size
		t.Run(fmt.Sprintf("%s/%dx%d", language, size[0], size[1]), func(t *testing.T) {
			inLocale(t, language, func() {
				a := withStyleCatalog(t)
				a.snapshot.Manifest.MediaPath = "test.mp4"
				a.splitY = 35
				tt := settled(a.view, size[0], size[1])
				for tab := 0; tab < 2; tab++ {
					a.workflowTab = tab
					for range 6 {
						tt.Frame()
					}
					label := locales.WorkflowStartRecognize()
					if tab == 1 {
						label = locales.WorkflowStartTranslate()
					}
					command, _ := tt.Find(label)
					panel, _ := tt.Find(locales.WorkflowTitle())
					tt.Scroll(panel.X+panel.W/2, float32(size[1])-100, 0, 2000)
					if after, _ := tt.Find(label); after != command {
						t.Fatal("primary action scrolled away", command, after)
					}
					if a.workflowScroll.MaxX > 1 || a.styleScroll.MaxX > 1 {
						t.Fatal("horizontal parameter overflow")
					}
					r, ok := tt.Find(locales.RecognitionResetFolder())
					if !ok || r.W <= 0 || r.Y+r.H > float32(size[1])-32 || r.X+r.W > float32(size[0]) {
						t.Fatal("destination action unreachable", r)
					}
				}
				if a.splitY != 35 {
					t.Fatal("content changed the user's divider")
				}
			})
		})
	}
}
