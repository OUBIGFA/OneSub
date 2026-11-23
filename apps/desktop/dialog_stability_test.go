package main

import (
	"fmt"
	"math"
	"path/filepath"
	"strings"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/importing"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// Every state participates in the initial size, not only the state first
// opened. Select gestures keep the shell and primary action still; the visible
// footer buttons pack to the right instead of reserving hidden preview space.
func TestMediaToolsKeepGeometryAcrossModes(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			for _, scale := range []float32{1, 1.5, 2} {
				for _, first := range []string{"compress", "audio"} {
					t.Run(fmt.Sprintf("%s/%g/%s", language, scale, first), func(t *testing.T) {
						a := nativeForTest(t)
						a.compressOpen, a.settings.MediaProcessMode = true, first
						tt := ui.NewTester(func(c *ui.Context) { applyStudioTheme(c); a.compressView(c) }, int(900*scale), int(720*scale))
						tt.SetPreferences(ui.Preferences{TextScale: scale, ReduceMotion: true})
						tt.Frame()
						labels := []string{locales.ActionCompress(), locales.MediaProcessMode(), locales.CompressFormat(), locales.ComposeAskPath()}
						before := make([]ui.Rect, len(labels))
						for i, label := range labels {
							var ok bool
							if before[i], ok = tt.Find(label); !ok {
								t.Fatal("missing dialog control", label)
							}
						}
						var action ui.Rect
						if first == "audio" {
							action, _ = tt.Find(locales.AudioExtractStart())
						} else {
							action, _ = tt.Find(locales.CompressStart())
						}
						for _, mode := range []string{locales.MediaExtractAudio(), locales.MediaCompressVideo(), locales.MediaExtractAudio(), locales.MediaCompressVideo()} {
							if err := tt.Click(locales.MediaProcessMode()); err != nil {
								t.Fatal(err)
							}
							if err := tt.Click(mode); err != nil {
								t.Fatal(err)
							}
							for i, label := range labels {
								after, ok := tt.Find(label)
								if !ok || after != before[i] {
									t.Errorf("%s moved after choosing %s: %v -> %v", label, mode, before[i], after)
								}
							}
							label := locales.CompressStart()
							if a.settings.MediaProcessMode == "audio" {
								label = locales.AudioExtractStart()
							}
							if after, _ := tt.Find(label); after != action {
								t.Errorf("primary action moved: %v -> %v", action, after)
							}
							cancel, _ := tt.Find(locales.ActionCancel())
							neighbor := action
							if a.settings.MediaProcessMode == "audio" {
								if tt.HasText(locales.ComposeSample()) {
									t.Fatal("audio mode exposes the video sample action")
								}
							} else {
								neighbor, _ = tt.Find(locales.ComposeSample())
							}
							if gap := neighbor.X - cancel.X - cancel.W; math.Abs(float64(gap-8*scale)) > 1 {
								t.Errorf("footer leaves a hidden-action gap: mode=%s gap=%g", a.settings.MediaProcessMode, gap)
							}
							if scale == 1 {
								captureRedesign(t, tt, "stable-media-"+language+"-"+a.settings.MediaProcessMode)
							}
						}
						for _, enc := range []struct {
							label string
							id    string
							note  string
						}{
							{locales.ComposeEncoderGpu(), "gpu", locales.CompressGpuNote()},
							{locales.ComposeEncoderCpu(), "cpu", locales.CompressAv1Note()},
						} {
							if err := tt.Click(locales.ComposeEncoder()); err != nil {
								t.Fatal(err)
							}
							if err := tt.Click(enc.label); err != nil {
								t.Fatal(err)
							}
							for i, label := range labels {
								after, ok := tt.Find(label)
								if !ok || after != before[i] {
									t.Errorf("%s moved after encoder %s: %v -> %v", label, enc.id, before[i], after)
								}
							}
							if after, _ := tt.Find(locales.CompressStart()); after != action {
								t.Errorf("compress action moved after encoder %s: %v -> %v", enc.id, action, after)
							}
							if !tt.HasText(enc.note) {
								t.Errorf("missing encoder note after choosing %s", enc.id)
							}
							prefs, err := settings.Load(filepath.Join(a.dataRoot, "settings.json"))
							if err != nil || prefs.CompressionEncoder != enc.id {
								t.Fatalf("compression encoder %s not persisted: %v (%q)", enc.id, err, prefs.CompressionEncoder)
							}
						}
					})
				}
			}
		})
	}
}

func TestToolbarKeepsFilenameAndOutputActionsInOrder(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			for _, width := range []int{900, 1800} {
				for _, scale := range []float32{1, 1.5, 2} {
					a := draftApp(t)
					filename := strings.Repeat("subtitle-", 40) + ".srt"
					a.snapshot.Manifest.SubtitlePath = filepath.Join("captions", filename)
					tt := ui.NewTester(func(c *ui.Context) { a.topBar(c, applyStudioTheme(c)) }, int(float32(width)*scale), int(100*scale))
					tt.SetPreferences(ui.Preferences{TextScale: scale, ReduceMotion: true})
					tt.Frame()
					var previous ui.Rect
					for _, label := range []string{locales.ActionAddFiles(), locales.ActionClearWorkbench(), filename, locales.ActionSettings(), locales.ActionCompress(), locales.ActionCompose(), locales.MenuExportSubtitles()} {
						r, ok := tt.Find(label)
						if !ok || r.X < previous.X+previous.W {
							t.Fatalf("toolbar order incorrect: %s %v after %v", label, r, previous)
						}
						previous = r
					}
					insideRedesign(t, tt, locales.A11yToolbar(), filename, locales.ActionSettings(), locales.MenuExportSubtitles())
					a.snapshot.Manifest.MediaPath = filepath.Join("media", "lesson.mp4")
					tt.Frame()
					insideRedesign(t, tt, locales.A11yToolbar(), "lesson.mp4")
				}
			}
		})
	}
}

// Candidate rows contain two lines, not a single-line button. Their natural
// height must contain the complete reason and keep scores away from filenames.
func TestCandidateRowsContainDetails(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			for _, scale := range []float32{1, 1.5, 2} {
				a := nativeForTest(t)
				name := strings.Repeat("Long subtitle filename ", 6) + ".srt"
				a.candidatesOpen = true
				a.pending = &importing.Loaded{Selection: importing.Selection{Candidates: []importing.Candidate{{Path: filepath.Join("fixtures", name), Score: 100, Reason: "exact"}}}}
				tt := ui.NewTester(func(c *ui.Context) { applyStudioTheme(c); a.candidatesView(c) }, int(900*scale), int(640*scale))
				tt.SetPreferences(ui.Preferences{TextScale: scale, ReduceMotion: true})
				tt.Frame()
				insideRedesign(t, tt, name, locales.CandidatesExact())
				visibleRedesign(t, tt, locales.CandidatesTitle(), locales.CandidatesScore(100), locales.CandidatesLater())
				if scale == 1 {
					captureRedesign(t, tt, "candidate-details-"+language)
				}
			}
		})
	}
}
