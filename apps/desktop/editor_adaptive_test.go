package main

import (
	"math"
	"strings"
	"testing"
	"time"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/subtitle"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
)

func TestEditorDisclosurePreservesDraftAndOtherPanels(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := storeApp(t, subtitle.Cue{ID: "draft", Text: "Original subtitle", StartMs: 1000, EndMs: 3000})
			a.subtitleSplit = 55
			tt := settled(a.view, 1800, 1000)
			before, _ := tt.Find(locales.A11yEditorPanel())
			player, _ := tt.Find(locales.A11yPlayer())
			if err := tt.Click(locales.A11yCueText()); err != nil {
				t.Fatal(err)
			}
			tt.Key(ui.Cmd, ui.KeyA)
			tt.Type("Edited subtitle")
			if err := tt.Click(locales.EditorCollapse()); err != nil {
				t.Fatal(err)
			}
			if tt.HasText(locales.A11yCueText()) || !tt.HasText(locales.EditorExpand()) || a.draft != "Edited subtitle" || a.subtitleSplit != 55 {
				t.Fatal("folding lost the draft or expanded divider position", a.draft, a.subtitleSplit)
			}
			if after, _ := tt.Find(locales.A11yPlayer()); after != player || a.settings.StylePanelCollapsed {
				t.Fatal("folding the editor changed the video or style panel", player, after)
			}
			tt.Advance(time.Second)
			prefs, err := settings.Load(a.dataRoot + "/settings.json")
			if err != nil || !prefs.EditorPanelCollapsed || prefs.StylePanelCollapsed || prefs.SubtitleHeight != 55 {
				t.Fatal("editor disclosure choice was not saved independently", err)
			}
			if a.snapshot.SourceDocument.Cues[0].Text != "Edited subtitle" {
				t.Fatal("folded editor did not retain the pending edit in the document")
			}
			// The native disclosure keeps keyboard and accessibility semantics.
			if info, ok := tt.Access(locales.EditorExpand()); !ok || info.Role != ui.RoleDisclosure || info.Expanded {
				t.Fatal("folded editor lacks a native disclosure", info)
			}
			tt.Key(0, ui.KeySpace)
			if after, _ := tt.Find(locales.A11yEditorPanel()); after != before || !tt.HasText(locales.A11yCueText()) || a.draft != "Edited subtitle" {
				t.Fatal("keyboard expansion did not restore the editor and draft", before, after)
			}
			tt.Move(0, 0)
			firstFrameSettled(t, tt, "fold-editor", func() { a.settings.EditorPanelCollapsed = true })
			firstFrameSettled(t, tt, "expand-editor", func() { a.settings.EditorPanelCollapsed = false })
		})
	}
}

// Resizing a short editor must allocate this viewport's remaining space, not
// keep a minimum-height scroll page with the editable text outside the panel.
func TestEditorTextUsesAvailableHeight(t *testing.T) {
	for _, language := range []string{"zh-Hans", "en"} {
		inLocale(t, language, func() {
			a := storeApp(t, subtitle.Cue{ID: "adaptive", Text: strings.Repeat("Editable subtitle line\n", 40), StartMs: 1000, EndMs: 3000})
			tt := settled(a.editorView, 900, 260)
			for _, height := range []int{260, 800, 320, 640} {
				tt.SetSize(900, height)
				body, ok := tt.Find(locales.A11yCueText())
				if !ok || body.H <= 0 || body.Y+body.H > float32(height) || float32(height)-body.Y-body.H > 24 {
					t.Fatal("text area does not fill the current editor viewport", height, body)
				}
				for range 3 {
					tt.Frame()
					after, _ := tt.Find(locales.A11yCueText())
					if after != body {
						t.Fatal("editor continued resizing after the viewport settled", body, after)
					}
				}
			}
		})
	}
}

func TestTimecodeLabelIsBesideInput(t *testing.T) {
	value := 123456.789
	var input ui.Rect
	tt := settled(func(c *ui.Context) {
		input = timecodeField(c, "开始", &value, .001, false, nil).Bounds()
	}, 500, 160)
	label, ok := tt.Find("开始")
	if !ok || math.Abs(float64(label.Y+label.H/2-input.Y-input.H/2)) > 2 {
		t.Fatal("timecode label is above its input", label, input)
	}
}

func TestTimecodeButtonsStepAndHonorReadOnly(t *testing.T) {
	value, disabled, changes := 1.234, false, 0
	tt := settled(func(c *ui.Context) {
		timecodeField(c, "开始", &value, .001, disabled, func() { changes++ })
	}, 500, 160)
	if err := tt.Click("增加开始时间"); err != nil {
		t.Fatal(err)
	}
	if value != 1.235 || changes != 1 {
		t.Fatal("increase did not apply one millisecond", value, changes)
	}
	if err := tt.Click("减少开始时间"); err != nil || value != 1.234 || changes != 2 {
		t.Fatal("decrease did not apply one millisecond", value, changes, err)
	}
	value = 0
	tt.Frame()
	_ = tt.Click("减少开始时间")
	if value != 0 || changes != 2 {
		t.Fatal("decrease crossed zero", value, changes)
	}
	disabled = true
	tt.Frame()
	_ = tt.Click("增加开始时间")
	if value != 0 || changes != 2 {
		t.Fatal("read-only time was changed by its button", value, changes)
	}
}
