package main

import (
	"fmt"
	"path/filepath"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// A module appears only after it has been used. Its completed receipt survives
// later tasks and errors without creating empty slots for unused modules.
func TestStatusBarShowsUsedModuleTimes(t *testing.T) {
	a := nativeForTest(t)
	a.errorText, a.status = "An unrelated error", "Previous result"
	tt := settled(a.view, 1280, 800)
	for _, label := range []string{"语音识别 —", "字幕翻译 —", "音视频处理 —", "视频合成 —", "字幕处理 —", "字幕导出 —"} {
		if tt.HasText(label) {
			t.Fatal("unused module has a status-bar placeholder", label)
		}
	}
	a.taskTimes[timingTranslation] = &taskTiming{elapsed: 9*time.Minute + 21*time.Second, finished: true}
	tt.Frame()
	labels := []string{"字幕翻译 9 分 21 秒"}
	a.busy, a.translationRunning = true, true
	a.translationLive = newTranslationLive()
	tt.Frame()
	tt.Advance(errorDuration)
	for _, label := range labels {
		if !tt.HasText(label) {
			t.Fatal("task or error expiry hid a module's elapsed time", label)
		}
	}
	bar, _ := tt.Find("状态栏")
	for _, label := range labels {
		r, ok := tt.Find(label)
		if !ok || r.X < bar.X || r.X+r.W > bar.X+bar.W+1 || r.Y < bar.Y || r.Y+r.H > bar.Y+bar.H+1 {
			t.Fatal("module time was clipped outside the status bar", label, r, bar)
		}
	}
	if receipt, _ := tt.Find(labels[0]); receipt.Y+receipt.H/2 < bar.Y+bar.H/2-1 || receipt.Y+receipt.H/2 > bar.Y+bar.H/2+1 {
		t.Fatal("module times are not on the status bar's single row", receipt, bar)
	}
}

func TestElapsedTimeUsesReadableUnits(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			want := "9 分 21 秒"
			if language == "en" {
				want = "9 min 21 s"
			}
			if got := elapsedLabel(9*time.Minute + 21*time.Second); got != want {
				t.Fatal("elapsed time does not use human-readable units", got, want)
			}
		})
	}
}

func TestTaskTimingExcludesPausedAndFinishedWait(t *testing.T) {
	start := time.Date(2026, 11, 13, 12, 0, 0, 0, time.UTC)
	clock := &taskTiming{started: start}
	clock.pause(start.Add(1250 * time.Millisecond))
	if got := clock.duration(start.Add(2 * time.Hour)); got != 1250*time.Millisecond {
		t.Fatal("user wait was included in processing time", got)
	}
	clock.resume(start.Add(2 * time.Hour))
	clock.stop(start.Add(2*time.Hour + 750*time.Millisecond))
	clock.stop(start.Add(3 * time.Hour))
	clock.resume(start.Add(4 * time.Hour))
	if got := clock.duration(start.Add(5 * time.Hour)); got != 2*time.Second {
		t.Fatal("completion or a duplicate stop extended the clock", got)
	}
	continued := &taskTiming{elapsed: clock.duration(start), started: start.Add(6 * time.Hour)}
	continued.stop(start.Add(6*time.Hour + 400*time.Millisecond))
	if got := continued.duration(start.Add(7 * time.Hour)); got != 2400*time.Millisecond {
		t.Fatal("continuation lost the completed execution interval", got)
	}
}

func TestFinishTaskFreezesPreviousModuleBeforeStartingNext(t *testing.T) {
	a := nativeForTest(t)
	now := time.Now()
	first := &taskTiming{started: now.Add(-1250 * time.Millisecond)}
	first.stop(now) // the worker has finished, but its UI callback is still queued
	a.activeTaskTiming, a.taskTimes[timingRecognition] = first, first
	a.finishTask("recognition", func() {
		next := &taskTiming{started: now}
		a.activeTaskTiming, a.taskTimes[timingTranslation] = next, next
	}, nil)
	if got := a.taskTimes[timingRecognition].duration(now.Add(time.Hour)); got != 1250*time.Millisecond {
		t.Fatal("recognition included the UI queue or translation time", got)
	}
	if a.activeTaskTiming != a.taskTimes[timingTranslation] || a.activeTaskTiming.duration(now.Add(500*time.Millisecond)) != 500*time.Millisecond {
		t.Fatal("starting translation overwrote or stopped the wrong module")
	}
}

func TestModuleTimesFitBothLanguagesAtEveryScale(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			for _, width := range []int{1360, 1600, 2200} {
				for _, scale := range []float32{1, 1.5, 2} {
					t.Run(fmt.Sprintf("%s/%d/%g", language, width, scale), func(t *testing.T) {
						a := nativeForTest(t)
						a.errorText, a.lastOutput = "A transient error beside permanent timings", filepath.Join(a.dataRoot, "result.srt")
						for module := timingRecognition; module < timingCount; module++ {
							a.taskTimes[module] = &taskTiming{elapsed: 123*time.Hour + 456780*time.Millisecond, finished: true}
						}
						tt := settled(func(c *ui.Context) { a.statusBar(c, applyStudioTheme(c)) }, int(float32(width)*scale), int(260*scale))
						tt.SetPreferences(ui.Preferences{TextScale: scale, ReduceMotion: true})
						tt.Frame()
						bar, _ := tt.Find(locales.A11yStatusBar())
						for module := timingRecognition; module < timingCount; module++ {
							value := "123 小时 7 分 36 秒"
							if language == "en" {
								value = "123 h 7 min 36 s"
							}
							label := moduleLabel(module) + " " + value
							r, exists := tt.Find(label)
							if !exists || r.X < bar.X || r.X+r.W > bar.X+bar.W+1 || r.Y < bar.Y || r.Y+r.H > bar.Y+bar.H+1 {
								t.Fatal("module time crosses the footer boundary", label, r, bar)
							}
							measured, _ := uiTextMeasureForCount(t, label, scale)
							if r.W+1 < measured {
								t.Fatal("module time was shrunk or ellipsized", label, r, measured)
							}
							if delta := r.Y + r.H/2 - bar.Y - bar.H/2; delta < -1 || delta > 1 {
								t.Fatal("module time left the status bar's single row", label, r, bar)
							}
						}
						before := bar
						tt.Advance(time.Hour)
						if after, _ := tt.Find(locales.A11yStatusBar()); before != after {
							t.Fatal("completed timings changed the footer's geometry", before, after)
						}
					})
				}
			}
		})
	}
}

func TestUsedModuleTimesAndCountsShareOneFooterRow(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := nativeForTest(t)
			a.snapshot = &project.Snapshot{SourceDocument: subtitle.Document{Cues: navigationCues(20000)}, Document: subtitle.Document{Cues: navigationCues(12345)}}
			a.lastOutput, a.errorText = filepath.Join(a.dataRoot, "result.srt"), "A current error with permanent module receipts"
			for module := timingRecognition; module < timingCount; module++ {
				a.taskTimes[module] = &taskTiming{elapsed: 9*time.Minute + 21*time.Second, finished: true}
			}
			tt := settled(a.view, minimumWindowWidth, 800)
			bar, _ := tt.Find(locales.A11yStatusBar())
			labels := []string{locales.StatusOriginalCount(20000), locales.StatusOptimizedCount(12345), locales.StatusOwnerOutput() + locales.ResultOpenFolder()}
			value := "9 分 21 秒"
			if language == "en" {
				value = "9 min 21 s"
			}
			for module := timingRecognition; module < timingCount; module++ {
				labels = append(labels, moduleLabel(module)+" "+value)
			}
			for _, label := range labels {
				r, exists := tt.Find(label)
				if !exists || r.X < bar.X || r.X+r.W > bar.X+bar.W+1 || r.Y+r.H/2 < bar.Y+bar.H/2-1 || r.Y+r.H/2 > bar.Y+bar.H/2+1 {
					t.Fatal("timing, count or output left the single footer row", label, r, bar)
				}
			}
		})
	}
}
