package main

import (
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// OneSub uses MyGo's native theme and one four-DIP spacing rhythm. Only
// typography and the primary-action blue are product choices; control states,
// focus, contrast, menus and surfaces continue to belong to the framework.
const (
	bodySize    = 15 // the product's body text (docs/ui-design.md)
	captionSize = 13 // the smallest text: status bar, units, hints
	headingSize = 17 // dialog sections
	titleSize   = 20 // dialog titles: Fluent's Subtitle
	headerSize  = 44 // a common, comfortable navigation/title row
	panelPad    = 16 // Fluent: 16 between a surface's edge and its text
	iconSize    = 16 // Fluent and Spectrum's icon beside 14–15 text
)

// studio holds the colors the theme has no field for.
type studio struct {
	chrome ui.Color // the bars around the panels: toolbar and status bar
}

// studioTheme retains the framework's coordinated light/dark palette instead
// of maintaining a second set of component colors. High contrast is untouched.
func studioTheme(base *ui.Theme, highContrast bool) (ui.Theme, studio) {
	t := *base
	t.FontSize = bodySize * base.FontSize / 14
	if !highContrast {
		primary := ui.LightTheme()
		t.Accent, t.AccentHover, t.AccentPressed, t.AccentText = primary.Accent, primary.AccentHover, primary.AccentPressed, primary.AccentText
	}
	return t, studio{chrome: t.Surface}
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
	row := ui.Row(c).Height(em(c, headerSize)).Padding(0, em(c, 8)).Gap(em(c, 4)).AlignItems(ui.Center).Shrink(0).Background(c.Theme().Surface)
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

// fixedDialog gives settings and output dialogs the same hierarchy: title,
// fixed working area, and an anchored action row. Option changes cannot resize
// the shell; no hidden page is built merely to reserve its intrinsic height.
func fixedDialog(c *ui.Context, open *bool, title string, width, height float32, body, actions func()) {
	vw, vh := c.Size()
	ui.Modal(c, open, func() {
		ui.Column(c).Width(min(em(c, width), vw-c.Theme().Space(20))).Height(min(em(c, height), vh-c.Theme().Space(20))).Gap(em(c, 16)).Label(title).Children(func() {
			dialogTitle(c, title)
			ui.Divider(c)
			ui.Column(c).Grow(1).MinHeight(0).Gap(em(c, 20)).Children(body)
			ui.Divider(c)
			ui.Row(c).Gap(em(c, 8)).Justify(ui.End).Shrink(0).Children(actions)
		})
	})
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
