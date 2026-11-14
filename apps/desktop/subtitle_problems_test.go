package main

import (
	"fmt"
	"slices"
	"strings"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// Reading recommendations stay available in the existing checks. The subtitle
// itself gets only actionable content/rendering errors, even in the issue filter.
func TestSubtitleRowsShowOnlySeriousProblemDetails(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		for _, live := range []bool{false, true} {
			for _, filtered := range []bool{false, true} {
				t.Run(fmt.Sprintf("%s/live%t/filter%t", language, live, filtered), func(t *testing.T) {
					inLocale(t, language, func() {
						text := strings.Repeat("A long sentence. ", 8)
						a := storeApp(t, subtitle.Cue{ID: "cue", Text: text, StartMs: 0, EndMs: 400})
						a.problems.filter = filtered
						if live {
							a.subtitleTab = 1
							a.translationLive = newTranslationLive()
							a.translationLive.stage, a.translationLive.total, a.translationLive.stopped = "translate", 1, "Interrupted"
							a.translationLive.units = slices.Clone(a.snapshot.Document.Cues)
							a.translationLive.texts[0], a.translationLive.final[0], a.translationLive.selected = text, true, 0
						}
						tt := settled(a.subtitlesView, 1200, 700)
						check := func(serious bool) {
							t.Helper()
							var row string
							for _, node := range tt.AccessAll() {
								if node.Role == ui.RoleRow && strings.Contains(node.Name, "A long sentence.") {
									row = node.Name
								}
							}
							if row == "" {
								t.Fatal("subtitle row was not rendered")
							}
							for _, warning := range []string{locales.ReadingLong(), locales.ReadingFast(), locales.ReadingBrief()} {
								if strings.Contains(row, warning) {
									t.Errorf("reading advice is still printed below the subtitle: %s", warning)
								}
							}
							if strings.Contains(row, "U+4E7FD") != serious {
								t.Fatal("serious character diagnosis was lost or shown for clean text", row)
							}
						}
						check(false)
						if len(a.subtitleProblems()[0]) == 0 {
							t.Fatal("hiding inline advice removed the existing issue checks")
						}
						if live {
							a.translationLive.texts[0] += "\U0004e7fd"
							a.translationLive.revision++
						} else {
							a.draft, a.editPending = text+"\U0004e7fd", true
						}
						tt.Frame()
						check(true)
					})
				})
			}
		}
	}
}

func TestInlineProblemDetailsKeepBlockingErrorsWithoutEmptyRows(t *testing.T) {
	for _, kind := range []string{"long", "fast", "brief", "overlap", "characters", "glyphs", "empty", "missing", "time"} {
		t.Run(kind, func(t *testing.T) {
			a := nativeForTest(t)
			a.problems.rows = [][]subtitle.CueProblem{{{Kind: kind, Characters: "\U0004e7fd", Other: 2}}}
			tt := settled(func(c *ui.Context) {
				ui.Column(c).Gap(8).Children(func() {
					ui.Text(c, "Before")
					a.subtitleProblemDetail(c, 0, true)
					ui.Text(c, "After")
				})
			}, 700, 200)
			before, _ := tt.Find("Before")
			after, _ := tt.Find("After")
			serious := slices.Contains([]string{"characters", "glyphs", "empty", "missing", "time"}, kind)
			if serious {
				if after.Y <= before.Y+before.H+8 {
					t.Fatal("blocking error has no visible detail", kind)
				}
			} else if after.Y != before.Y+before.H+8 {
				t.Fatal("advice left a line or empty layout gap under the subtitle", kind, before, after)
			}
		})
	}
}

func TestSubtitleIssuesDoNotReuseAnotherProjectsFontReport(t *testing.T) {
	a := storeApp(t, subtitle.Cue{ID: "old", Text: "Old \U000f0000", StartMs: 0, EndMs: 5000})
	a.problems.glyphs, a.problems.filter = []string{"\U000f0000"}, true
	tt := settled(a.subtitlesView, 1100, 640)
	if !tt.HasText("1 条异常字幕 · 1 项") {
		t.Fatal("the first project's known missing glyph was not shown")
	}
	b := storeApp(t, subtitle.Cue{ID: "new", Text: "New \U000f0000", StartMs: 0, EndMs: 5000})
	a.setSnapshot(*b.snapshot)
	tt.Frame()
	if tt.HasText("1 条异常字幕 · 1 项") || !tt.HasText("New \U000f0000") {
		t.Fatal("the new project inherited the old project's font diagnosis or filter", tt.Texts())
	}
}

func TestSubtitleIssueControlsFitLanguagesAndTextScales(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		for _, scale := range []float32{1, 1.5, 2} {
			for _, width := range []int{420, 1100} {
				t.Run(fmt.Sprintf("%s/%g/%d", language, scale, width), func(t *testing.T) {
					inLocale(t, language, func() {
						a := storeApp(t, subtitle.Cue{ID: "bad", Text: "Caption \U0004e7fd", StartMs: 0, EndMs: 5000})
						tt := settled(a.subtitlesView, int(float32(width)*scale), int(800*scale))
						tt.SetPreferences(ui.Preferences{TextScale: scale})
						if err := tt.Click(locales.SubtitleIssuesToggle()); err != nil {
							t.Fatal(err)
						}
						for _, label := range []string{locales.SubtitleIssuesToggle(), locales.SubtitleIssuesSummary(1, 1), locales.SubtitleIssuesPrevious(), locales.SubtitleIssuesNext()} {
							r, ok := tt.Find(label)
							if !ok || r.W <= 0 || r.H <= 0 || r.X < 0 || r.X+r.W > float32(width)*scale+1 {
								t.Fatal("issue control is missing or clipped", label, r)
							}
						}
					})
				})
			}
		}
	}
}

func TestSubtitleIssueFilterLocatesAndUpdatesAfterEditing(t *testing.T) {
	a := storeApp(t,
		subtitle.Cue{ID: "clean", Text: "正常字幕", Translated: true, TranslationSource: "Clean source", StartMs: 0, EndMs: 5000},
		subtitle.Cue{ID: "bad", Text: "异常文字 \U0004e7fd", Translated: true, TranslationSource: "Second source", StartMs: 6000, EndMs: 11000})
	a.subtitleTab = 1
	a.loadDraft()
	a.position, a.paused = 2, true
	tt := settled(a.subtitlesView, 1100, 640)
	if !tt.HasText("1 条异常字幕 · 1 项") {
		t.Fatal("the affected subtitle and issue counts are missing", tt.Texts())
	}
	if err := tt.Click("定位异常字幕"); err != nil {
		t.Fatal(err)
	}
	if tt.HasText("正常字幕") || a.selected != 1 || a.query != "" || a.position != 2 || !a.paused || a.pendingClick != nil {
		t.Fatal("issue filter did not locate the cue without seeking", a.selected, a.position, tt.Texts())
	}
	if !strings.Contains(strings.Join(tt.Texts(), "\n"), "U+4E7FD") {
		t.Fatal("the invalid character has no actionable reason", tt.Texts())
	}
	a.draft, a.editPending = "已修正", true
	tt.Frame()
	if !tt.HasText("未发现字幕异常") {
		t.Fatal("issue count did not follow the current draft", tt.Texts())
	}
	if !a.commitEdit(true) {
		t.Fatal("corrected text could not be committed")
	}
	tt.Frame()
	if err := tt.Click("定位异常字幕"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("正常字幕") || !tt.HasText("已修正") || !a.trackUnsaved(1) {
		t.Fatal("turning the filter off lost the full list or unsaved edit", tt.Texts())
	}
}

func TestSubtitleIssueFilterKeepsTheCurrentProblemWhilePlaying(t *testing.T) {
	a := storeApp(t,
		subtitle.Cue{ID: "bad", Text: "Caption \U0004e7fd", StartMs: 0, EndMs: 5000},
		subtitle.Cue{ID: "clean", Text: "Next caption", StartMs: 6000, EndMs: 11000})
	a.position, a.paused = 1, false
	tt := settled(a.subtitlesView, 1100, 640)
	if err := tt.Click(locales.SubtitleIssuesToggle()); err != nil {
		t.Fatal(err)
	}
	// The next playback update must not move editing off the problem the
	// filter has just located, even when it was already selected on entry.
	a.position = 7
	tt.Frame()
	if a.selected != 0 || a.paused || a.position != 7 {
		t.Fatal("playback took selection away from the current subtitle problem", a.selected, a.paused, a.position)
	}
}

func TestSubtitleIssuesIncludeBothTextLayersAndTiming(t *testing.T) {
	a := storeApp(t,
		subtitle.Cue{ID: "source-bad", Text: "正常译文", TranslationSource: "Broken \uFFFD", Translated: true, StartMs: 0, EndMs: 5000},
		subtitle.Cue{ID: "time-bad", Text: "另一条字幕", StartMs: 6000, EndMs: 11000})
	a.subtitleTab = 1
	// A draft may temporarily have an invalid end even though the persisted
	// project prevents committing it. Diagnostics must include that draft.
	a.selected = 1
	a.loadDraft()
	a.end, a.editPending = a.start, true
	tt := settled(a.subtitlesView, 1100, 640)
	text := strings.Join(tt.Texts(), "\n")
	if !strings.Contains(text, "2 条异常字幕") {
		t.Fatal("source-text and timing issues were not counted", text)
	}
}

func TestSubtitleIssueFilterUsesInterruptedLiveRows(t *testing.T) {
	a := nativeForTest(t)
	a.subtitleTab = 1
	a.translationLive = newTranslationLive()
	live := a.translationLive
	live.stage, live.total, live.done, live.stopped = "translate", 2, 1, "Interrupted"
	live.units = []subtitle.Cue{
		{ID: "completed", Text: "Completed source", StartMs: 0, EndMs: 5000},
		{ID: "missing", Text: "Missing source", StartMs: 6000, EndMs: 11000},
	}
	live.texts[0], live.final[0] = "已完成 \U0004e7fd", true
	live.texts[1] = "未完成片段"
	tt := settled(a.subtitlesView, 1100, 640)
	if !tt.HasText("2 条异常字幕 · 2 项") {
		t.Fatal("interrupted live issues were confused with the stored source track", tt.Texts())
	}
	if err := tt.Click("定位异常字幕"); err != nil {
		t.Fatal(err)
	}
	if live.selected != 0 || !strings.Contains(strings.Join(tt.Texts(), "\n"), "U+4E7FD") {
		t.Fatal("live issue selection did not reach the completed row", live.selected, tt.Texts())
	}
	if err := tt.Click("下一条异常字幕"); err != nil {
		t.Fatal(err)
	}
	if live.selected != 1 {
		t.Fatal("live issue navigation did not reach the missing translation", live.selected)
	}
}

func TestSubtitleIssuesPreserveValidUnicodeAndProtectedASS(t *testing.T) {
	a := storeApp(t,
		subtitle.Cue{ID: "unicode", Text: "中文 e\u0301 👩\u200d💻\nالعربية", StartMs: 0, EndMs: 15000},
		subtitle.Cue{ID: "drawing", Text: "{\\p1}m 0 0 l 10 10", ReadOnly: true, StartMs: 16000, EndMs: 17000})
	tt := settled(func(c *ui.Context) { a.subtitlesView(c) }, 1100, 640)
	if tt.HasText("字幕时间无效") || strings.Contains(strings.Join(tt.Texts(), "\n"), "条异常字幕") {
		t.Fatal("valid Unicode or protected drawing content was marked as broken", tt.Texts())
	}
	if err := tt.Click("定位异常字幕"); err != nil {
		t.Fatal(err)
	}
	if !tt.HasText("未发现字幕异常") {
		t.Fatal("empty issue filter did not explain the result", tt.Texts())
	}
}
