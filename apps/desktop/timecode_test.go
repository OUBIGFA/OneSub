package main

import (
	"fmt"
	"math"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

func TestTimecodeParsingMatchesSharedFormatter(t *testing.T) {
	for text, seconds := range map[string]float64{"00:00:00.000": 0, "01:23:45.678": 5025.678, "34:17:36.789": 123456.789, "100:00:00.001": 360000.001} {
		got, ok := parseTimecode(text)
		if !ok || got != seconds || stamp(got) != text {
			t.Fatal("timecode did not round trip", text, got, ok)
		}
	}
	for _, text := range []string{"123456.789", "01:60:00.000", "01:00:60.000", "-01:00:00.000", "01:02:03,004", "01:02:03.04", "01:02:03", "99e3:00:00.000", "999999999999999999999:00:00.000"} {
		if _, ok := parseTimecode(text); ok {
			t.Fatal("invalid or non-timecode input accepted", text)
		}
	}
}

func TestNativeTimecodeFieldKeepsFullTextAndEditing(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		for _, scale := range []float32{1, 1.5, 2} {
			t.Run(fmt.Sprintf("%s/%g", language, scale), func(t *testing.T) {
				inLocale(t, language, func() {
					value := 360000.001
					changes := 0
					var needed float32
					var input ui.Rect
					tt := settled(func(c *ui.Context) {
						needed, _ = c.MeasureText(0, ui.Span{Text: stamp(value)})
						needed += c.Theme().Space(5) + 2
						input = timecodeField(c, locales.EditorStart(), &value, .001, false, func() { changes++ }).Bounds()
						ui.Button(c, "Leave")
					}, 600, 400)
					tt.SetPreferences(ui.Preferences{TextScale: scale})
					tt.Frame()
					if input.W < needed {
						t.Fatal("complete timecode does not fit", input, needed)
					}
					if err := tt.Click(locales.EditorStart()); err != nil {
						t.Fatal(err)
					}
					tt.Key(ui.Cmd, ui.KeyA)
					tt.Key(ui.Cmd, ui.KeyC)
					if tt.Clipboard() != "100:00:00.001" {
						t.Fatal("time field does not use the shared timestamp format", tt.Clipboard())
					}
					tt.Type("01:23:45.678")
					if value != 5025.678 || changes == 0 {
						t.Fatal("valid timestamp did not update time", value, changes)
					}
					tt.Key(0, ui.KeyUp)
					if math.Abs(value-5025.679) > .000001 {
						t.Fatal("millisecond step changed", value)
					}
					tt.Key(ui.Cmd, ui.KeyA)
					tt.Type("01:75:00.000")
					if value != 5025.679 || !tt.HasText(locales.EditorTimecodeFormat()) {
						t.Fatal("invalid timestamp changed the cue or has no error", value)
					}
					if err := tt.Click("Leave"); err != nil {
						t.Fatal(err)
					}
					tt.Frame()
					if err := tt.Click(locales.EditorStart()); err != nil {
						t.Fatal(err)
					}
					tt.Key(ui.Cmd, ui.KeyA)
					tt.Key(ui.Cmd, ui.KeyC)
					if tt.Clipboard() != "01:23:45.679" {
						t.Fatal("invalid blur did not restore last valid time", tt.Clipboard())
					}
				})
			})
		}
	}
}
