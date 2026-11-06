package main

import (
	"bytes"
	"fmt"
	"image/png"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// layoutFeedback matches view code that sizes or places elements from
// what an earlier frame measured, then asks for another frame to apply it.
// That layout is wrong for the first frame of every page and jumps on the
// next. Layout comes from MyGo's native grid, flex and wrap, or from
// values known in the frame, such as c.Size().
var layoutFeedback = regexp.MustCompile(`\.(Layout)?Bounds\(\)|c\.After\(0\)`)

// layoutFeedbackAllowed are the matches that do not lay out a view: image
// resampling, and the repeat of a held time step.
var layoutFeedbackAllowed = map[string]int{"branding.go": 2, "timecode.go": 1, "player_view.go": 1}

func TestViewsDoNotLayOutFromEarlierFrames(t *testing.T) {
	files, err := filepath.Glob("*.go")
	if err != nil {
		t.Fatal(err)
	}
	for _, file := range files {
		if strings.HasSuffix(file, "_test.go") {
			continue
		}
		data, err := os.ReadFile(file)
		if err != nil {
			t.Fatal(err)
		}
		if n := len(layoutFeedback.FindAll(data, -1)); n > layoutFeedbackAllowed[file] {
			t.Errorf("%s lays out from an earlier frame (%d uses of Bounds or After(0)); use native layout", file, n)
		}
	}
}

// A deliberately deferred width change proves that FrameOnce observes the
// first frame, rather than silently running the Tester's normal settling loop.
func TestSingleFrameTesterDoesNotHideDeferredLayout(t *testing.T) {
	grow := false
	width := float32(100)
	tt := ui.NewTester(func(c *ui.Context) {
		ui.Button(c, "Probe").Label("First-frame probe").Width(width)
		if grow && width != 200 {
			width = 200
			c.Invalidate()
		}
	}, 320, 160)
	grow = true
	tt.FrameOnce()
	first, _ := tt.Find("First-frame probe")
	pixels := append([]byte(nil), tt.Image().Pix...)
	if first.W != 100 || !tt.FramePending() {
		t.Fatal("single frame drained the deferred layout", first, tt.FramePending())
	}
	tt.Frame()
	settled, _ := tt.Find("First-frame probe")
	if settled.W != 200 || bytes.Equal(pixels, tt.Image().Pix) {
		t.Fatal("the regression probe did not reveal its second-frame width", first, settled)
	}
}

// firstFrameSettled renders open in one frame and reports whether more
// frames change any pixel.
func firstFrameSettled(t *testing.T, tt *ui.Tester, page string, open func()) {
	t.Helper()
	open()
	// Frame() settles up to 20 frames internally. Observe a raw frame so
	// a next-frame layout correction cannot pass as an already stable page.
	tt.FrameOnce()
	firstImage := *tt.Image()
	firstImage.Pix = append([]byte(nil), firstImage.Pix...)
	for range 4 {
		tt.Frame()
	}
	if !bytes.Equal(firstImage.Pix, tt.Image().Pix) {
		name := strings.NewReplacer("/", "-", " ", "_").Replace(t.Name() + "-" + page)
		captureRedesign(t, tt, name+"-settled")
		if dir := os.Getenv("ONESUB_UI_REVIEW_DIR"); dir != "" {
			f, err := os.Create(filepath.Join(dir, name+"-first.png"))
			if err != nil {
				t.Fatal(err)
			}
			if err = png.Encode(f, &firstImage); err != nil {
				f.Close()
				t.Fatal(err)
			}
			if err = f.Close(); err != nil {
				t.Fatal(err)
			}
		}
		t.Fatal("the first frame differs from the settled page:", page)
	}
}

// Every settings page and workbench tab is laid out in its first frame.
func TestPagesAreSettledInTheirFirstFrame(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		for _, width := range []int{900, 1400} {
			for _, scale := range []float32{1, 1.5, 2} {
				t.Run(fmt.Sprintf("%s/%d/%g", language, width, scale), func(t *testing.T) {
					inLocale(t, language, func() {
						a := nativeForTest(t)
						tt := ui.NewTester(a.view, int(float32(width)*scale), int(640*scale))
						tt.SetPreferences(ui.Preferences{TextScale: scale, ReduceMotion: true})
						for range 4 {
							tt.Frame()
						}
						for _, dark := range []bool{false, true} {
							tt.SetDark(dark)
							a.settingsOpen = true
							for _, tab := range []int{0, 1, 5, 2, 3, 4, 6} {
								firstFrameSettled(t, tt, fmt.Sprintf("settings-%d-dark%t", tab, dark), func() { a.settingsTab = tab })
							}
							a.settingsOpen = false
							for _, state := range []struct {
								workflow, queue int
								mode            string
							}{{0, 0, recognitionRouted}, {0, 0, recognitionManual}, {0, 1, recognitionManual}, {1, 0, recognitionManual}, {0, 0, recognitionRouted}} {
								firstFrameSettled(t, tt, fmt.Sprintf("workflow-%v-dark%t", state, dark), func() {
									a.workflowTab, a.recognitionMode, a.settings.RecognitionMode = state.workflow, state.queue, state.mode
								})
							}
							available := a.downloadedAsr
							a.downloadedAsr = nil
							firstFrameSettled(t, tt, fmt.Sprintf("workflow-no-models-dark%t", dark), func() {
								a.workflowTab, a.recognitionMode, a.settings.RecognitionMode = 0, 0, recognitionManual
							})
							a.downloadedAsr = available
							if !tt.HasText(locales.WorkflowRecognize()) {
								t.Fatal(tt.Texts())
							}
						}
					})
				})
			}
		}
	}
}
