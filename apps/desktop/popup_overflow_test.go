package main

import (
	"fmt"
	"testing"

	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

// Long native choice lists must clip and scroll inside their elevated panel,
// including when opening upward beside controls near the window bottom.
func TestChoicePopupsContainAndScrollLongLists(t *testing.T) {
	for _, kind := range []string{"select", "autocomplete"} {
		for _, bottom := range []bool{false, true} {
			t.Run(fmt.Sprintf("%s/bottom=%t", kind, bottom), func(t *testing.T) {
				options := make([]string, 14)
				for i := range options {
					options[i] = fmt.Sprintf("choice-%02d", i)
				}
				selected := options[0]
				tt := settled(func(c *ui.Context) {
					applyStudioTheme(c)
					ui.Column(c).Fill().Padding(20).Children(func() {
						if bottom {
							ui.Spacer(c)
						}
						switch kind {
						case "select":
							lobeSelect(c, &selected, options).Label("choose")
						case "autocomplete":
							lobeCombobox(c, &selected, options).Label("choose")
						}
					})
				}, 420, 360)
				if err := tt.Click("choose"); err != nil {
					t.Fatal(err)
				}
				if kind == "autocomplete" {
					tt.Key(0, ui.KeyDown)
				}
				optionRect := func(name string) ui.Rect {
					for _, item := range tt.AccessAll() {
						if item.Name == name {
							return item.Bounds
						}
					}
					t.Fatalf("missing option %s", name)
					return ui.Rect{}
				}
				first := optionRect(options[1])
				tt.Scroll(first.X+first.W/2, first.Y+first.H/2, 0, 120)
				after := optionRect(options[1])
				if after.Y >= first.Y {
					t.Fatalf("long popup does not scroll: before=%v after=%v", first, after)
				}
				tt.Scroll(first.X+first.W/2, first.Y+first.H/2, 0, 1000)
				if err := tt.Click(options[len(options)-1]); err != nil {
					t.Fatal("last choice is unreachable", err)
				}
				if selected != options[len(options)-1] {
					t.Fatalf("last choice not selected: %q", selected)
				}

			})
		}
	}
}

// Presets are shown in full on opening, without requiring a wheel action.
func TestPresetChoicesVisibleWithoutScrolling(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := withStyleCatalog(t)
			tt := settled(func(c *ui.Context) {
				applyStudioTheme(c)
				ui.Column(c).Fill().Padding(20).Children(func() {
					ui.Spacer(c)
					ui.Row(c).Children(func() { a.presetControls(c, 0) })
				})
			}, 720, 640)
			if err := tt.Click(locales.StylePreset()); err != nil {
				t.Fatal(err)
			}
			var panel ui.Rect
			for _, item := range tt.AccessAll() {
				if item.Role == ui.RoleListBox {
					panel = item.Bounds
				}
			}
			count := 0
			for _, item := range tt.AccessAll() {
				if item.Role != ui.RoleOption {
					continue
				}
				count++
				r := item.Bounds
				if r.Y < panel.Y || r.Y+r.H > panel.Y+panel.H || r.Y < 0 || r.Y+r.H > 640 {
					t.Fatalf("preset is not fully visible on opening: %s %v panel=%v", item.Name, r, panel)
				}
			}
			options := a.presetOptions(a.stylePreset)
			if count != len(options) {
				t.Fatalf("got %d visible options, want %d", count, len(options))
			}
			last := options[len(options)-1]
			if err := tt.Click(last.Label); err != nil {
				t.Fatal(err)
			}
			if a.stylePreset != last.ID {
				t.Fatal("last preset did not apply")
			}
		})
	}
}
