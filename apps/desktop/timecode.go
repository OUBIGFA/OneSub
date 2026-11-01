package main

import (
	"regexp"
	"strconv"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

var timecodePattern = regexp.MustCompile(`^([0-9]{2,}):([0-9]{2}):([0-9]{2})\.([0-9]{3})$`)

// Timecodes are durations, not times of day: hours can exceed 23. Parse to
// integer milliseconds before converting to the editor's existing seconds.
func parseTimecode(text string) (float64, bool) {
	parts := timecodePattern.FindStringSubmatch(text)
	if parts == nil {
		return 0, false
	}
	hours, err := strconv.ParseInt(parts[1], 10, 64)
	if err != nil {
		return 0, false
	}
	minutes, _ := strconv.ParseInt(parts[2], 10, 64)
	seconds, _ := strconv.ParseInt(parts[3], 10, 64)
	millis, _ := strconv.ParseInt(parts[4], 10, 64)
	remainder := minutes*60000 + seconds*1000 + millis
	const maxMillis = int64(9007199254740991)
	if minutes > 59 || seconds > 59 || hours > (maxMillis-remainder)/3600000 {
		return 0, false
	}
	return float64(hours*3600000+remainder) / 1000, true
}

type timecodeDraft struct {
	text    string
	value   float64
	focused bool
	invalid bool
	stepped bool
}

// Use the native text field, with the very same formatter as the list/player.
// MyGo's time-of-day input omits milliseconds and cannot represent long media.
// Incomplete typing stays local; only a complete valid time updates the cue.
func timecodeField(c *ui.Context, label string, value *float64, step float64, disabled bool, changed func()) ui.Element {
	box := ui.Column(c.Key(label)).Shrink(0)
	draft := ui.Local(box, "timecode", func() timecodeDraft { return timecodeDraft{text: stamp(*value), value: *value} })
	if draft.value != *value {
		draft.text, draft.value, draft.invalid = stamp(*value), *value, false
	}
	set := func(seconds float64) {
		if seconds != *value {
			*value, draft.value = seconds, seconds
			if changed != nil {
				changed()
			}
		}
	}
	var input ui.Element
	box.Children(func() {
		// Form supplies MyGo's native inline label layout. The time field
		// retains complete text editing and the NumberInput button styling.
		ui.Form(c, func() {
			field := ui.Field(c, label, func() {
				ui.Row(c).Gap(c.Theme().Space(1)).AlignItems(ui.Center).Shrink(0).Children(func() {
					width, _ := c.MeasureText(0, ui.Span{Text: stamp(*value)})
					input = ui.TextInput(c, &draft.text).Width(width + c.Theme().Space(5) + 4).Tooltip(stamp(*value)).Disabled(disabled)
					input.OnTextKey(func(k ui.TextKey) bool {
						if disabled || k.Mods != 0 || (k.Key != ui.KeyUp && k.Key != ui.KeyDown) {
							return false
						}
						seconds, valid := parseTimecode(k.Text())
						if valid {
							if k.Key == ui.KeyDown {
								seconds -= step
							} else {
								seconds += step
							}
							k.SetText(stamp(max(0, min(9007199254740, seconds))))
							draft.stepped = true
						}
						return true
					})
					if input.Changed() || draft.stepped {
						draft.stepped = false
						seconds, valid := parseTimecode(draft.text)
						draft.invalid = !valid
						if valid {
							set(seconds)
						}
					}
					if input.Submitted() && !draft.invalid || draft.focused && !input.Focused() {
						// Invalid input never silently changes the time: restore the
						// last valid value on blur and retain the inline error message.
						draft.text = stamp(*value)
					}
					draft.focused = input.Focused()
					for _, button := range []struct {
						text, name string
						delta      float64
					}{{"−", locales.EditorDecreaseTime(label), -step}, {"+", locales.EditorIncreaseTime(label), step}} {
						ui.Button(c, button.text).Padding(c.Theme().Space(1), c.Theme().Space(2)).Label(button.name).Tooltip(button.name).
							Disabled(disabled || button.delta < 0 && *value <= 0 || button.delta > 0 && *value >= 9007199254740).
							OnClick(func() {
								// Format and parse once to keep stepping in integer
								// milliseconds, including repeated decimal increments.
								draft.text = stamp(max(0, min(9007199254740, *value+button.delta)))
								seconds, _ := parseTimecode(draft.text)
								draft.invalid = false
								set(seconds)
							})
					}
				})
			})
			if draft.invalid {
				field.Error(locales.EditorTimecodeFormat())
			}
		}).Shrink(0)
	})
	return input
}
