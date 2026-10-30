package main

import (
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// OneSub's design tokens. Sizes follow Microsoft Fluent 2 for Windows and
// Adobe Spectrum 2, the system of Premiere Pro; docs/ui-design.md gives the
// sources. Text sizes are relative to the body size, which carries the
// desktop's text scale.
const (
	bodySize    = 15 // the product's body text (docs/ui-design.md)
	captionSize = 13 // the smallest text: status bar, units, hints
	headingSize = 17 // dialog sections
	titleSize   = 20 // dialog titles: Fluent's Subtitle
	headerSize  = 40 // panel headers: Spectrum component-height-200
	panelPad    = 16 // Fluent: 16 between a surface's edge and its text
	iconSize    = 16 // Fluent and Spectrum's icon beside 14–15 text
)

// studio holds the colors the theme has no field for.
type studio struct {
	chrome ui.Color // the bars around the panels: toolbar and status bar
}

// studioTheme is the app's theme over MyGo's default one for the window's
// appearance. Neutral grays layer the window (Spectrum 2's gray scale, with
// WinUI's text and stroke contrasts); one blue marks primary actions, the
// choice and the focus. A desktop in high contrast keeps MyGo's own colors,
// which follow it.
func studioTheme(base *ui.Theme, highContrast bool) (ui.Theme, studio) {
	t := *base
	scale := base.FontSize / 14
	t.FontSize = bodySize * scale
	t.Radius = 6
	t.Spacing = 4
	if highContrast {
		return t, studio{chrome: t.Surface}
	}
	if t.Dark {
		t.Background = ui.Hex("#1F1F1F")
		t.Surface, t.SurfaceHover, t.SurfacePressed = ui.Hex("#2A2A2A"), ui.Hex("#333333"), ui.Hex("#3B3B3B")
		t.Border = ui.Hex("#383838")
		t.Text, t.TextMuted = ui.Hex("#E8E8E8"), ui.Hex("#A3A3A3")
		t.Accent, t.AccentHover, t.AccentPressed, t.AccentText = ui.Hex("#2563EB"), ui.Hex("#3B76F0"), ui.Hex("#1D4FD0"), ui.Hex("#FFFFFF")
		t.Danger, t.Warning, t.Success = ui.Hex("#F87171"), ui.Hex("#F5B342"), ui.Hex("#5CC98B")
		t.Selection, t.Focus = ui.RGBA(59, 130, 246, 0.4), ui.RGBA(96, 165, 250, 0.7)
		t.Scrollbar = ui.RGBA(255, 255, 255, 0.3)
		t.Inverse, t.InverseText = ui.Hex("#3A3A3A"), ui.Hex("#F2F2F2")
		return t, studio{chrome: ui.Hex("#181818")}
	}
	t.Background = ui.Hex("#FAFAFA")
	t.Surface, t.SurfaceHover, t.SurfacePressed = ui.Hex("#FFFFFF"), ui.Hex("#F2F2F2"), ui.Hex("#E8E8E8")
	t.Border = ui.Hex("#D6D6D6")
	t.Text, t.TextMuted = ui.Hex("#1B1B1B"), ui.Hex("#5F5F5F")
	t.Accent, t.AccentHover, t.AccentPressed, t.AccentText = ui.Hex("#2563EB"), ui.Hex("#1D4ED8"), ui.Hex("#1E40AF"), ui.Hex("#FFFFFF")
	t.Danger, t.Warning, t.Success = ui.Hex("#C42B1C"), ui.Hex("#9D5D00"), ui.Hex("#0F7B0F")
	t.Selection, t.Focus = ui.RGBA(37, 99, 235, 0.22), ui.RGBA(37, 99, 235, 0.55)
	t.Scrollbar = ui.RGBA(0, 0, 0, 0.3)
	t.Inverse, t.InverseText = ui.Hex("#2B2B2B"), ui.Hex("#F5F5F5")
	return t, studio{chrome: ui.Hex("#F0F0F0")}
}

// em scales a design size in DIPs with the theme's text size.
func em(c *ui.Context, size float32) float32 { return size * c.Theme().FontSize / bodySize }

// appearances are the app's light/dark choices; dark is the default, as in
// video editors, where the picture is judged against a dark surround.
func appearances() []option {
	return []option{
		{string(mygo.ThemeDark), locales.AppearanceDark()},
		{string(mygo.ThemeLight), locales.AppearanceLight()},
		{string(mygo.ThemeSystem), locales.AppearanceSystem()},
	}
}

// icon is a Lucide icon at the size every button and bar uses.
func icon(c *ui.Context, name string) ui.Element {
	return ui.Icon(c, icons[name]).Size(em(c, iconSize), em(c, iconSize))
}

// iconButton is a native button with an icon before its label.
func iconButton(c *ui.Context, label, name string) ui.Element {
	return ui.Button(c, "").Label(label).Tooltip(label).Children(func() { icon(c, name); ui.Text(c, label).SingleLine() })
}

// primaryIconButton is iconButton in the accent color, one per area.
func primaryIconButton(c *ui.Context, label, name string) ui.Element {
	return ui.PrimaryButton(c, "").Label(label).Tooltip(label).Children(func() { icon(c, name); ui.Text(c, label).SingleLine() })
}

// toolIcon is an icon-only native button, square and with the icon in its
// middle; the tooltip and the accessible name carry the action.
func toolIcon(c *ui.Context, label, name string) ui.Element {
	return ui.Button(c, "").Label(label).Tooltip(label).Padding(c.Theme().Space(2)).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
		icon(c, name).Label(locales.A11yIconOf(label))
	})
}

// panelHeader is the row every panel starts with, the same height in each,
// so titles and actions line up across the workbench. Each side would be
// as wide as the wider one, by their contents' widths in the last frame,
// which centers the middle part in the panel; with less room, a side gives
// way down to its own content, never further, and the middle shrinks, as a
// file name does.
func panelHeader(c *ui.Context, start, middle, end func()) {
	row := ui.Row(c).Height(em(c, headerSize)).Padding(0, em(c, 8)).Gap(em(c, 2)).AlignItems(ui.Center).Shrink(0)
	widths := ui.Local(row, "sides", func() [2]float32 { return [2]float32{} })
	row.Children(func() {
		side := func(i int, justify ui.Align, gap float32, content func()) {
			ui.Row(c).Width(max(widths[0], widths[1])).MinWidth(widths[i]).Shrink(1).Justify(justify).Children(func() {
				inner := ui.Row(c).Gap(gap).AlignItems(ui.Center).Shrink(0)
				if content != nil {
					inner.Children(content)
				}
				if w := inner.Bounds().W; w > 0 && w != widths[i] {
					widths[i] = w
					c.After(0)
				}
			})
		}
		side(0, ui.Start, em(c, 8), start)
		ui.Spacer(c)
		if middle != nil {
			ui.Row(c).Shrink(1).MinWidth(0).Gap(em(c, 8)).AlignItems(ui.Center).Children(middle)
		}
		ui.Spacer(c)
		side(1, ui.End, em(c, 4), func() {
			if end != nil {
				end()
			}
		})
	})
}

// panelTitle is a panel's name: body size, semibold, as Fluent's Body
// Strong titles confined areas.
func panelTitle(c *ui.Context, text string) ui.Element {
	return ui.Text(c, text).FontWeight(600).SingleLine()
}

// dialogTitle opens every dialog.
func dialogTitle(c *ui.Context, text string) ui.Element {
	return ui.Text(c, text).FontSize(em(c, titleSize)).FontWeight(600)
}

// muted is secondary text, as units, counts and hints.
func muted(c *ui.Context, text string) ui.Element {
	return ui.Text(c, text).TextColor(c.Theme().TextMuted)
}

// parameterGroup separates meaning with a heading and breathing room, not
// cards. Optional management actions sit with the settings they manage.
func parameterGroup(c *ui.Context, title string, action, fields func()) {
	ui.Column(c).Gap(em(c, 10)).Children(func() {
		ui.Row(c).Gap(em(c, 8)).AlignItems(ui.Center).Children(func() {
			panelTitle(c, title)
			ui.Spacer(c)
			if action != nil {
				action()
			}
		})
		fields()
	})
}

// A short parameter uses a native Form's inline label. The enclosing line
// wraps the whole field, never leaving its label separated from its control.
func inlineField(c *ui.Context, label string, control func()) ui.Element {
	return ui.Form(c, func() { ui.Field(c, label, control) }).AlignSelf(ui.Start)
}
