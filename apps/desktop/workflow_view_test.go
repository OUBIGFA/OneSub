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

// Output location stays beside the format; recognition parameters and
// subtitle output have distinct headings rather than arbitrary row breaks.
func TestWorkflowFieldsStayCompactAndAligned(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := nativeForTest(t)
			tt := settled(a.workflowPanel, 1200, 500)
			location, ok := tt.Find(locales.RecognitionBesideMedia())
			choose, found := tt.Find(locales.RecognitionChooseFolder())
			if !ok || !found || math.Abs(float64(location.Y+location.H/2-choose.Y-choose.H/2)) > 1 || choose.X-location.X-location.W > 32 {
				t.Fatal("directory and action separated", language, location, choose)
			}
			format, _ := tt.Find("SRT")
			if math.Abs(float64(format.Y+format.H/2-location.Y-location.H/2)) > 1 {
				t.Fatal("directory not beside output parameters", language, format, location)
			}
			strip, _ := tt.Find(locales.WorkflowStripPunctuation())
			keep, _ := tt.Find(locales.WorkflowKeepPunctuation())
			if math.Abs(float64(strip.Y+strip.H/2-keep.Y-keep.H/2)) > 1 {
				t.Fatal("punctuation controls not center aligned", language, strip, keep)
			}
		})
	}
}

// Compact groups wrap instead of overflowing the smallest panel. Long
// user paths may elide; operation names must remain completely inside it.
func TestWorkflowFolderActionsStayInsideNarrowPanels(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		for _, width := range []int{320, 450, 800} {
			for _, tab := range []int{0, 1} {
				t.Run(fmt.Sprintf("%s/%d/%d", language, width, tab), func(t *testing.T) {
					inLocale(t, language, func() {
						a := nativeForTest(t)
						a.workflowTab = tab
						a.settings.RecognitionDir = strings.Repeat("folder/", 50)
						a.settings.TranslationDir = a.settings.RecognitionDir
						tt := settled(a.workflowPanel, width, 700)
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

// Batching shares one row with the chosen service and model whenever it
// fits. Changes survive reload even during a task.
func TestTranslationSchedulingOnWorkbench(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := nativeForTest(t)
			a.workflowTab = 1
			a.settings.ProviderModel = "example-model"
			tt := settled(a.workflowPanel, 800, 650)
			concurrency, _ := tt.Find(locales.TranslationConcurrency())
			batch, _ := tt.Find(locales.TranslationBatchSize())
			model, _ := tt.Find(locales.TranslationModel())
			directory, _ := tt.Find(locales.RecognitionDirectory())
			if math.Abs(float64(concurrency.Y-batch.Y)) > 1 || math.Abs(float64(concurrency.Y-model.Y)) > 1 || directory.Y <= concurrency.Y {
				t.Fatal("batching is not in the service row", concurrency, batch, model, directory)
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
				t.Fatal("workbench scheduling changes were not saved", prefs.TranslationConcurrency, prefs.TranslationBatchSize, err)
			}
		})
	}
}

// Exercise real native layout with populated styles and both task tabs. A
// max-sized desktop must fit its parameter content, not hide the scrollbar
// while still clipping fields. Saved divider ratios remain user-owned.
func TestDesktopParametersFitWithoutScrolling(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		for _, size := range [][2]int{{1920, 1040}, {2560, 1400}, {3440, 1392}} {
			t.Run(fmt.Sprintf("%s/%dx%d", language, size[0], size[1]), func(t *testing.T) {
				inLocale(t, language, func() {
					a := withStyleCatalog(t)
					a.snapshot.Manifest.MediaPath = "test.mp4"
					tt := settled(a.view, size[0], size[1])
					for range 8 {
						tt.Frame()
					}
					for tab := 0; tab < 2; tab++ {
						a.workflowTab = tab
						for range 4 {
							tt.Frame()
						}
						if a.workflowScroll.MaxY > 1 || a.styleScroll.MaxY > 1 {
							t.Fatal("desktop parameter content overflows", tab, a.workflowScroll, a.styleScroll, a.workflowHeight, a.styleHeight)
						}
						for _, label := range []string{locales.StyleMarginVertical(), locales.RecognitionResetFolder()} {
							r, ok := tt.Find(label)
							if !ok || r.Y+r.H > float32(size[1])-32 || r.X+r.W > float32(size[0]) {
								t.Fatal("parameter clipped", label, r)
							}
						}
					}
					if float64(a.splitY) != settings.Default().TopHeight {
						t.Fatal("content fit overwrote user divider preference")
					}
				})
			})
		}
	}
}
