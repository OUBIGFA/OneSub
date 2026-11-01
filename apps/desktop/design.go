package main

import (
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// Typography and spacing are shared by every native view. Light colors are
// reused from WinOne; dark controls retain the established MyGo palette.
const (
	bodySize    = 15 // the product's body text (docs/ui-design.md)
	captionSize = 13 // the smallest text: status bar, units, hints
	headingSize = 18 // section headings, distinct from form labels
	titleSize   = 24 // dialog titles
	headerSize  = 48 // common panel navigation/title row
	panelPad    = 20 // breathing room around inspector content
	iconSize    = 16 // Fluent and Spectrum's icon beside 14–15 text
)

// studio holds the colors the theme has no field for.
type studio struct {
	chrome, panel, hairline, selected, warningSoft ui.Color
}

// Apply the shared light color contract without changing native widget
// behavior or OneSub's type scale. Dark appearance is deliberately unchanged.
func studioTheme(base *ui.Theme, highContrast bool) (ui.Theme, studio) {
	t := *base
	t.FontSize = bodySize * base.FontSize / 14
	if !base.Dark {
		applyWinOneLightColors(&t, highContrast)
	} else if !highContrast {
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
	return ui.Text(c, text).FontSize(em(c, 16)).FontWeight(600).SingleLine()
}

// dialogTitle opens every dialog.
func dialogTitle(c *ui.Context, text string) ui.Element {
	return ui.Text(c, text).FontSize(em(c, titleSize)).FontWeight(500)
}

// fixedDialog gives settings and output dialogs the same hierarchy: title,
// fixed working area, and an anchored action row. Option changes cannot resize
// the shell; no hidden page is built merely to reserve its intrinsic height.
func fixedDialog(c *ui.Context, open *bool, title string, width, height float32, body, actions func()) {
	vw, vh := c.Size()
	modal := ui.Modal(c, open, func() {
		ui.Column(c).Width(min(em(c, width), vw-c.Theme().Space(20))).Height(min(em(c, height), vh-c.Theme().Space(20))).Gap(em(c, 16)).Label(title).Children(func() {
			dialogTitle(c, title)
			ui.Divider(c)
			ui.Column(c).Grow(1).MinHeight(0).Gap(em(c, 20)).Children(body)
			ui.Divider(c)
			ui.Row(c).Gap(em(c, 8)).Justify(ui.End).Shrink(0).Children(actions)
		})
	})
	if !c.Theme().Dark {
		modal.Background(studioColors(c).panel)
	}
}

// muted is secondary text, as units, counts and hints.
func muted(c *ui.Context, text string) ui.Element {
	return ui.Text(c, text).TextColor(c.Theme().TextMuted)
}

// A section heading leads one flat native settings panel. There are no
// nested cards; optional management actions belong to the section heading.
func parameterGroup(c *ui.Context, title string, action, fields func()) {
	ui.Column(c).Gap(em(c, 16)).Children(func() {
		ui.Row(c).Gap(em(c, 12)).AlignItems(ui.Center).Children(func() {
			ui.Text(c, title).FontSize(em(c, headingSize)).FontWeight(500)
			ui.Spacer(c)
			if action != nil {
				action()
			}
		})
		look := studioColors(c)
		ui.Column(c).Gap(em(c, 16)).Padding(em(c, 16)).Background(look.panel).
			Border(1, look.hairline).Radius(c.Theme().Radius + em(c, 4)).Children(fields)
	})
}

// parameterGrid sets a deliberate density ceiling instead of packing every
// field that happens to fit on one line. Equal native grid columns give each
// related decision room; narrow panes become one column without shrinking text.
func parameterGrid(c *ui.Context, minimum float32, maximum int, fields func()) ui.Element {
	grid := ui.Grid(c).GapX(em(c, 20)).GapY(em(c, 16)).AlignItems(ui.Start)
	room := ui.Local(grid, "field-room", func() float32 { return 0 })
	columns := max(1, min(maximum, int((*room+em(c, 20))/(em(c, minimum)+em(c, 20)))))
	grid.Columns(columns).Children(fields)
	if width := grid.Bounds().W; width > 0 && width != *room {
		*room = width
		c.After(0)
	}
	return grid
}
