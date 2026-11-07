package main

import "github.com/egoist/mygo/ui"

// WinOne light-theme color values, adapted from kit/theme.go at
// 90c479ce28b0cc6d690d3fa35115ea7dd4a4c89f (GPL-3.0-only).
// Source, file hashes, license and adaptation boundary:
// third_party/ports/winone/README.md.
// Base colors are shared; OneSub retains its typography, native metrics
// and original blue accent, including selection and keyboard focus.
func applyWinOneLightColors(t *ui.Theme, highContrast bool) {
	t.Background = ui.Hex("#fcfcfc")
	t.Surface = ui.Hex("#f3f3f3")
	t.SurfaceHover = ui.Hex("#ebebeb")
	t.SurfacePressed = ui.Hex("#e2e2e2")
	t.Border = ui.Hex("#d6d6d6")
	t.Text = ui.Hex("#262626")
	t.TextMuted = ui.Hex("#5e5e5e")
	primary := ui.LightTheme()
	t.Accent, t.AccentHover, t.AccentPressed, t.AccentText = primary.Accent, primary.AccentHover, primary.AccentPressed, primary.AccentText
	t.Danger = ui.Hex("#c42b1c")
	t.Warning = ui.Hex("#9d5d00")
	t.Success = ui.Hex("#0f7b0f")
	t.Selection, t.Focus = primary.Selection, primary.Focus
	t.Inverse = ui.Hex("#2b2b2b")
	t.InverseText = ui.Hex("#fafafa")
	t.Scrollbar = ui.RGBA(0, 0, 0, 0.3)
	if highContrast {
		t.Border = t.Border.Mix(t.Text, 0.45)
		t.TextMuted = t.TextMuted.Mix(t.Text, 0.4)
		t.Focus = t.Accent
	}
}

// Extra surface roles use WinOne's lightPalette; the selected feature is the
// blue accent's soft state so emphasis remains consistent with native controls.
func studioColors(c *ui.Context) studio {
	t := c.Theme()
	if t.Dark {
		panel := ui.Hex("#141414")
		if c.Preferences().HighContrast {
			panel = t.Background
		}
		hairline := ui.Hex("#343434")
		if c.Preferences().HighContrast {
			hairline = t.Border
		}
		return studio{chrome: t.Surface, panel: panel, hairline: hairline, selected: t.Accent.Alpha(0.12), warningSoft: t.Warning.Alpha(0.16)}
	}
	look := studio{
		chrome:      ui.Hex("#f3f3f3"),
		panel:       ui.Hex("#ffffff"),
		hairline:    ui.RGBA(0, 0, 0, 0.08),
		selected:    t.Accent.Alpha(0.12),
		warningSoft: ui.RGBA(157, 93, 0, 0.1),
	}
	if c.Preferences().HighContrast {
		look.hairline = t.Border
	}
	return look
}
