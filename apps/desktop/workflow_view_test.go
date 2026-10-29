package main

import (
	"fmt"
	"math"
	"strings"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
)

// Fields use their content width. A wide recognition panel keeps output
// location beside the format/chunk/token controls, not against the far edge.
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

// The translation tab uses the same compact directory group, next to the
// model rather than as a full-width row with distant actions.
func TestTranslationFolderStaysNextToModel(t *testing.T) {
	a := nativeForTest(t)
	a.workflowTab = 1
	a.settings.ProviderModel = "example-model"
	tt := settled(a.workflowPanel, 1000, 500)
	model, _ := tt.Find("example-model")
	directory, _ := tt.Find(locales.TranslationBesideSource())
	if math.Abs(float64(model.Y+model.H/2-directory.Y-directory.H/2)) > 1 || directory.X-model.X-model.W > 16 {
		t.Fatal("model and folder separated", model, directory)
	}
}
