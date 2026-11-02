package main

import (
	"math"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/project"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

func TestTimecodeHoldStepThresholds(t *testing.T) {
	for _, tc := range []struct {
		held time.Duration
		want float64
	}{
		{0, .001},
		{999 * time.Millisecond, .001},
		{time.Second, .01},
		{1999 * time.Millisecond, .01},
		{2 * time.Second, .1},
		{3999 * time.Millisecond, .1},
		{4 * time.Second, 1},
		{30 * time.Second, 1},
	} {
		if got := timecodeHoldStep(.001, tc.held); got != tc.want {
			t.Fatalf("held %v: step %v, want %v", tc.held, got, tc.want)
		}
	}
	if got := timecodeHoldStep(.01, 0); got != .01 {
		t.Fatal("ASS insertion lost its centisecond precision", got)
	}
}

func TestTimecodeHoldUsesNativeDelayAndAccelerates(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			value, lastMillis := 1.234, int64(1234)
			var increments []int64
			tt := settled(func(c *ui.Context) {
				timecodeField(c, locales.EditorStart(), &value, .001, false, func() {
					millis := int64(math.Round(value * 1000))
					increments = append(increments, millis-lastMillis)
					lastMillis = millis
				})
			}, 600, 160)
			button, ok := tt.Find(locales.EditorIncreaseTime(locales.EditorStart()))
			if !ok {
				t.Fatal("increment button missing")
			}
			x, y := button.X+button.W/2, button.Y+button.H/2
			started := tt.Now()
			tt.Press(x, y)
			if value != 1.235 || len(increments) != 1 || !tt.Focused(locales.EditorStart()) {
				t.Fatal("press did not make one fine step and focus the time field", value, increments)
			}
			tt.Advance(timecodeRepeatDelay - time.Millisecond)
			if len(increments) != 1 {
				t.Fatal("repeat started before the native delay", increments)
			}
			tt.Advance(time.Millisecond)
			if len(increments) != 2 || increments[1] != 1 {
				t.Fatal("repeat did not start at the native delay", increments)
			}
			for _, phase := range []struct {
				at   time.Duration
				step int64
			}{{1250 * time.Millisecond, 10}, {2250 * time.Millisecond, 100}, {4250 * time.Millisecond, 1000}} {
				tt.Advance(phase.at - tt.Now().Sub(started))
				if got := increments[len(increments)-1]; got != phase.step {
					t.Fatalf("held %v: increment %v ms, want %v ms", phase.at, got, phase.step)
				}
			}
			before, changes := value, len(increments)
			tt.Release(x, y)
			tt.Advance(2 * time.Second)
			if value != before || len(increments) != changes {
				t.Fatal("release added a step or kept repeating", value, increments)
			}
			if err := tt.Click(locales.EditorIncreaseTime(locales.EditorStart())); err != nil || increments[len(increments)-1] != 1 {
				t.Fatal("new press retained the previous acceleration", err, increments)
			}
		})
	}
}

func TestTimecodeHoldStopsOnLeaveBlurAndDisable(t *testing.T) {
	for _, stop := range []string{"leave", "blur", "disable", "zero", "upper-limit"} {
		t.Run(stop, func(t *testing.T) {
			value, disabled := 5.0, false
			label := locales.EditorDecreaseTime(locales.EditorStart())
			if stop == "zero" {
				value = .002
			} else if stop == "upper-limit" {
				value = 9007199254739.998
				label = locales.EditorIncreaseTime(locales.EditorStart())
			}
			tt := settled(func(c *ui.Context) {
				timecodeField(c, locales.EditorStart(), &value, .001, disabled, nil)
			}, 600, 160)
			button, _ := tt.Find(label)
			x, y := button.X+button.W/2, button.Y+button.H/2
			tt.Press(x, y)
			tt.Advance(500 * time.Millisecond)
			switch stop {
			case "leave":
				tt.Move(590, 150)
			case "blur":
				tt.SetFocused(false)
			case "disable":
				disabled = true
				tt.Frame()
			case "zero":
				if value != 0 {
					t.Fatal("decrement did not clamp at zero", value)
				}
			case "upper-limit":
				if value != 9007199254740 {
					t.Fatal("increment did not clamp at the upper limit", value)
				}
			}
			before := value
			tt.Advance(5 * time.Second)
			if value != before {
				t.Fatal("repeat continued after cancellation or a bound", before, value)
			}
			if stop == "leave" {
				tt.Move(x, y)
				tt.Advance(time.Second)
				tt.Release(x, y)
				if value != before {
					t.Fatal("returning before release resumed a cancelled gesture", before, value)
				}
			} else {
				tt.Release(590, 150)
			}
		})
	}
}

func TestTimecodeSecondaryClickDoesNotStep(t *testing.T) {
	value := 5.0
	tt := settled(func(c *ui.Context) {
		timecodeField(c, locales.EditorStart(), &value, .001, false, nil)
	}, 600, 160)
	if err := tt.RightClick(locales.EditorIncreaseTime(locales.EditorStart())); err != nil {
		t.Fatal(err)
	}
	tt.Advance(time.Second)
	if value != 5 {
		t.Fatal("secondary click changed the time", value)
	}
}

func TestTimecodeHoldStaysOneUndoableEdit(t *testing.T) {
	a := storeApp(t, subtitle.Cue{ID: "current", Text: "Current subtitle", StartMs: 1000, EndMs: 30000})
	tt := settled(a.view, 1600, 1000)
	button, _ := tt.Find(locales.EditorIncreaseTime(locales.EditorStart()))
	x, y := button.X+button.W/2, button.Y+button.H/2
	tt.Press(x, y)
	tt.Advance(4250 * time.Millisecond)
	if a.snapshot.Revision != 0 || !a.editPending {
		t.Fatal("one held gesture committed intermediate steps", a.snapshot.Revision, a.editPending)
	}
	tt.Release(x, y)
	tt.Advance(editCommitDelay + time.Millisecond)
	if a.snapshot.Revision != 1 || a.editPending || !a.snapshot.CanUndo {
		t.Fatal("release did not commit one undo step", a.snapshot.Revision, a.editPending)
	}
	a.undo(false)
	if a.start != 1 || a.snapshot.SourceDocument.Cues[0].StartMs != 1000 {
		t.Fatal("undo did not restore the whole held gesture", a.start)
	}
}

// Inspect the input itself, not its nearby warning icon: a red icon must not
// make a still-black timecode pass the warning test.
func timecodeHasRedText(t *testing.T, tt *ui.Tester, label string, danger ui.Color) bool {
	t.Helper()
	name, named := tt.Find(label)
	button, found := tt.Find(locales.EditorDecreaseTime(label))
	if !named || !found {
		t.Fatal("timecode field missing", label, tt.Texts())
	}
	pixels := 0
	frame := tt.Image()
	for y := int(button.Y); y < int(button.Y+button.H); y++ {
		for x := int(name.X + name.W); x < int(button.X); x++ {
			p := frame.RGBAAt(x, y)
			if p.R == danger.R && p.G == danger.G && p.B == danger.B {
				pixels++
			}
		}
	}
	return pixels >= 5
}

func TestEditorTimeOverlapUsesLiveDraftAndDocumentNeighbors(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := storeApp(t,
				subtitle.Cue{ID: "previous", Position: 0, Text: "Previous", StartMs: 1000, EndMs: 2000},
				subtitle.Cue{ID: "current", Position: 1, Text: "Current", StartMs: 3000, EndMs: 4000},
				subtitle.Cue{ID: "next", Position: 2, Text: "Next", StartMs: 5000, EndMs: 6000},
			)
			a.selected, a.query = 1, "Current"
			a.loadDraft()
			var danger ui.Color
			tt := settled(func(c *ui.Context) { danger = c.Theme().Danger; a.editorView(c) }, 900, 400)
			for _, tc := range []struct {
				start, end       float64
				redStart, redEnd bool
			}{{3, 4, false, false}, {1.999, 4, true, false}, {3, 5.001, false, true}, {1.999, 5.001, true, true}, {2, 5, false, false}} {
				a.start, a.end = tc.start, tc.end
				a.changed(tt.Now())
				tt.Frame()
				if red := timecodeHasRedText(t, tt, locales.EditorStart(), danger); red != tc.redStart {
					t.Fatal("start color does not reflect the previous cue", tc.start, red)
				}
				if red := timecodeHasRedText(t, tt, locales.EditorEnd(), danger); red != tc.redEnd {
					t.Fatal("end color does not reflect the next cue", tc.end, red)
				}
				if tt.HasText(locales.A11yWarningIcon()) != (tc.redStart || tc.redEnd) {
					t.Fatal("warning icon does not follow the overlap", tc)
				}
			}
			// The endpoints of the document have only one real neighbor.
			a.selected = 0
			a.loadDraft()
			tt.Frame()
			if timecodeHasRedText(t, tt, locales.EditorStart(), danger) {
				t.Fatal("first cue has a fictitious previous neighbor")
			}
			a.selected = 2
			a.loadDraft()
			tt.Frame()
			if timecodeHasRedText(t, tt, locales.EditorEnd(), danger) {
				t.Fatal("last cue has a fictitious next neighbor")
			}
		})
	}
}

func TestEditorTimeOverlapUsesIndependentTrackTimeline(t *testing.T) {
	a := storeApp(t,
		subtitle.Cue{ID: "previous", Position: 0, Text: "Previous", StartMs: 1000, EndMs: 2000},
		subtitle.Cue{ID: "current", Position: 1, Text: "Current", StartMs: 3000, EndMs: 4000},
		subtitle.Cue{ID: "next", Position: 2, Text: "Next", StartMs: 5000, EndMs: 6000},
	)
	_, err := a.store.Apply("retime-working", a.snapshot.Revision, []project.Edit{
		{ID: "previous", Track: "source", Text: "Previous", StartMs: 1000, EndMs: 1500},
		{ID: "next", Track: "source", Text: "Next", StartMs: 7000, EndMs: 8000},
	})
	if err != nil {
		t.Fatal(err)
	}
	state, err := a.store.Snapshot()
	if err != nil {
		t.Fatal(err)
	}
	a.setSnapshot(state)
	a.selected = 1
	var danger ui.Color
	tt := settled(func(c *ui.Context) { danger = c.Theme().Danger; a.editorView(c) }, 900, 400)
	for _, track := range []int{0, 1} {
		a.subtitleTab, a.editRebuiltSource = track, true
		a.loadDraft()
		a.start, a.end = 1.9, 6
		a.changed(tt.Now())
		tt.Frame()
		if timecodeHasRedText(t, tt, locales.EditorStart(), danger) != (track == 0) || timecodeHasRedText(t, tt, locales.EditorEnd(), danger) != (track == 0) {
			t.Fatal("overlap used the other track's timeline", track)
		}
	}
}

func TestEditorTimeOverlapFollowsTypingAndButtonEdits(t *testing.T) {
	a := storeApp(t,
		subtitle.Cue{ID: "previous", Position: 0, Text: "Previous", StartMs: 1000, EndMs: 2000},
		subtitle.Cue{ID: "current", Position: 1, Text: "Current", StartMs: 3000, EndMs: 5000},
		subtitle.Cue{ID: "next", Position: 2, Text: "Next", StartMs: 5000, EndMs: 6000},
	)
	a.selected = 1
	a.loadDraft()
	var danger ui.Color
	tt := settled(func(c *ui.Context) { danger = c.Theme().Danger; a.editorView(c) }, 900, 400)
	if err := tt.Click(locales.EditorStart()); err != nil {
		t.Fatal(err)
	}
	tt.Key(ui.Cmd, ui.KeyA)
	tt.Type("00:00:01.999")
	if a.start != 1.999 || !timecodeHasRedText(t, tt, locales.EditorStart(), danger) {
		t.Fatal("typing did not update the overlap color", a.start)
	}
	warning, _ := tt.Find(locales.A11yWarningIcon())
	tt.Move(warning.X+warning.W/2, warning.Y+warning.H/2)
	tt.Advance(time.Second)
	if !tt.HasText(locales.EditorStartOverlap(1)) {
		t.Fatal("warning does not name the preceding subtitle", tt.Texts())
	}
	if err := tt.Click(locales.EditorIncreaseTime(locales.EditorEnd())); err != nil {
		t.Fatal(err)
	}
	if a.end != 5.001 || !timecodeHasRedText(t, tt, locales.EditorEnd(), danger) {
		t.Fatal("button step did not update the overlap color", a.end)
	}
	if !a.commitEdit(false) || a.snapshot.SourceDocument.Cues[0].EndMs != 2000 || a.snapshot.SourceDocument.Cues[2].StartMs != 5000 {
		t.Fatal("overlap warning blocked a valid manual edit or rewrote a neighbor")
	}
	if err := tt.Click(locales.EditorIncreaseTime(locales.EditorStart())); err != nil {
		t.Fatal(err)
	}
	if a.start != 2 || timecodeHasRedText(t, tt, locales.EditorStart(), danger) {
		t.Fatal("touching the preceding boundary did not clear the warning", a.start)
	}
	if err := tt.Click(locales.EditorDecreaseTime(locales.EditorEnd())); err != nil {
		t.Fatal(err)
	}
	if a.end != 5 || timecodeHasRedText(t, tt, locales.EditorEnd(), danger) || tt.HasText(locales.A11yWarningIcon()) {
		t.Fatal("resolved end overlap did not clear the warning", a.end)
	}
}

func TestEditorTimeOverlapControlsFitNarrowPane(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			for _, scale := range []float32{1, 1.5, 2} {
				a := storeApp(t,
					subtitle.Cue{ID: "previous", Text: "Previous", EndMs: 2000},
					subtitle.Cue{ID: "current", Text: "Current", StartMs: 1999, EndMs: 5001},
					subtitle.Cue{ID: "next", Text: "Next", StartMs: 5000, EndMs: 7000},
				)
				a.selected = 1
				a.loadDraft()
				width, height := int(360*scale), int(420*scale)
				tt := settled(a.editorView, width, height)
				tt.SetPreferences(ui.Preferences{TextScale: scale})
				tt.Frame()
				for _, label := range []string{locales.EditorStart(), locales.EditorEnd(), locales.EditorIncreaseTime(locales.EditorStart()), locales.EditorDecreaseTime(locales.EditorEnd()), locales.A11yWarningIcon()} {
					r, ok := tt.Find(label)
					if !ok || r.W <= 0 || r.X < 0 || r.X+r.W > float32(width) || r.Y+r.H > float32(height) {
						t.Fatal("timecode controls clipped with overlap warnings", language, scale, label, r)
					}
				}
			}
		})
	}
}
