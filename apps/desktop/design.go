package main

import (
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
)

// Shared native typography: controls stay compact; subtitle content and the
// editor carry the larger reading sizes, as the work rather than the chrome.
const (
	bodySize     = 15 // the product's body text
	captionSize  = 13 // the smallest text: status bar, units, hints
	headingSize  = 18 // section headings, distinct from form labels
	titleSize    = 24 // dialog titles
	headerSize   = 48 // common panel navigation/title row
	panelPad     = 20 // breathing room around inspector content
	iconSize     = 16 // Fluent and Spectrum's icon beside 14–15 text
	subtitleSize = 17
	editSize     = 20
)

// studio holds the colors the theme has no field for.
type studio struct {
	chrome, panel, hairline, selected, warningSoft ui.Color
}

// MyGo owns all widget behavior. The neutral dark ramp follows Fluent 2's
// background/foreground roles; light retains the registered WinOne palette.
// See third_party/ports/fluent-ui/README.md for the reference boundary.
func studioTheme(base *ui.Theme, highContrast bool) (ui.Theme, studio) {
	t := *base
	t.FontSize = bodySize * base.FontSize / 14
	if !base.Dark {
		applyWinOneLightColors(&t, highContrast)
	} else if !highContrast {
		t.Background = ui.Hex("#1f1f1f")
		t.Surface = ui.Hex("#292929")
		t.SurfaceHover = ui.Hex("#3d3d3d")
		t.SurfacePressed = ui.Hex("#383838")
		t.Border = ui.Hex("#525252")
		t.Text, t.TextMuted = ui.Hex("#efefef"), ui.Hex("#adadad")
		primary := ui.LightTheme()
		t.Accent, t.AccentHover, t.AccentPressed, t.AccentText = primary.Accent, primary.AccentHover, primary.AccentPressed, primary.AccentText
		// Keep semantic text readable on both the canvas and the controls.
		t.Danger = ui.Hex("#f87171")
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
	return ui.Button(c, "").Label(label).Tooltip(label).Children(func() {
		icon(c, name).Label(locales.A11yIconOf(label))
		ui.Text(c, label).SingleLine()
	})
}

// primaryIconButton is iconButton in the accent color, one per area.
func primaryIconButton(c *ui.Context, label, name string) ui.Element {
	return ui.PrimaryButton(c, "").Label(label).Tooltip(label).Children(func() {
		icon(c, name).Label(locales.A11yIconOf(label))
		ui.Text(c, label).SingleLine()
	})
}

// toolIcon is an icon-only native button, square and with the icon in its
// middle; the tooltip and the accessible name carry the action.
func toolIcon(c *ui.Context, label, name string) ui.Element {
	return ui.Button(c, "").Label(label).Tooltip(label).Padding(c.Theme().Space(2)).Justify(ui.Center).AlignItems(ui.Center).Children(func() {
		icon(c, name).Label(locales.A11yIconOf(label))
	})
}

// panelHeader is the row every panel starts with, the same height in each,
// so titles and actions line up across the workbench. A native grid of
// equal flexible sides around the middle centers it in the panel in the
// same layout pass, as the top bar does; the middle shrinks, as a file name
// does.
func panelHeader(c *ui.Context, start, middle, end func()) {
	ui.Grid(c).ColumnTracks(ui.Fr(1), ui.FitContent(), ui.Fr(1)).Height(em(c, headerSize)).Padding(0, em(c, 8)).GapX(em(c, 4)).AlignItems(ui.Center).Shrink(0).Background(c.Theme().Surface).Children(func() {
		side := func(justify ui.Align, gap float32, content func()) {
			row := ui.Row(c).Gap(gap).Justify(justify).AlignItems(ui.Center).MinWidth(0)
			if content != nil {
				row.Children(content)
			}
		}
		side(ui.Start, em(c, 8), start)
		ui.Row(c).MinWidth(0).Gap(em(c, 8)).AlignItems(ui.Center).Children(func() {
			if middle != nil {
				middle()
			}
		})
		side(ui.End, em(c, 4), end)
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

// Forms take their natural content height. A positive height is reserved for
// unbounded readers/lists; these use the available viewport rather than letting
// a large document resize the window. Width is a readable minimum, not a cap
// that can truncate a longer translated label or action.
func fixedDialog(c *ui.Context, open *bool, title string, width, height float32, body, actions func()) {
	vw, vh := c.Size()
	look := studioColors(c)
	ui.Modal(c, open, func() {
		// Native measurement resolves the content and form-label widths in
		// this frame. The shell itself never introduces a scrolling region.
		shell := ui.Column(c).MinWidth(min(em(c, width+40), vw-em(c, 40))).MaxWidth(vw - em(c, 40)).Label(title)
		if height > 0 {
			shell.Height(min(em(c, height+40), vh-em(c, 40)))
		}
		shell.Children(func() {
			ui.Row(c).Padding(em(c, 16), em(c, 24)).Gap(em(c, 16)).AlignItems(ui.Center).Shrink(0).Children(func() {
				ui.Text(c, title).FontSize(em(c, 20)).FontWeight(500).Grow(1)
				if actions == nil {
					ui.Button(c, locales.ActionClose()).OnClick(func() { *open = false })
				}
			})
			separator(c)
			content := ui.Column(c).Gap(em(c, 12))
			if height > 0 {
				content.Grow(1).MinHeight(0)
			}
			if actions != nil {
				content.Padding(em(c, 16), em(c, 24))
			}
			content.Children(body)
			if actions != nil {
				separator(c)
				ui.Row(c).Padding(em(c, 14), em(c, 24)).Gap(em(c, 8)).Justify(ui.End).AlignItems(ui.Center).Shrink(0).Background(look.chrome).Radius(0, 0, c.Theme().Space(2.5), c.Theme().Space(2.5)).Children(actions)
			}
		})
	}).Padding(0).Background(c.Theme().Background)
}

// Confirmations use the same native modal chrome, but size to their message.
// Modal retains focus trapping, Escape and focus restoration; callers keep
// AlertDialog's chosen-index contract, including cancellation via Escape.
func confirmationDialog(c *ui.Context, open *bool, title, message string, buttons ...string) int {
	if !*open {
		return -1
	}
	chosen := -1
	fixedDialog(c, open, title, 600, 0, func() {
		ui.Text(c, message)
	}, func() {
		for i, label := range buttons {
			button := ui.Button
			if i == len(buttons)-1 {
				button = ui.PrimaryButton
			}
			if button(c, label).Shrink(0).Clicked() {
				chosen, *open = i, false
			}
		}
	})
	if !*open && chosen < 0 {
		return ui.AlertDismissed
	}
	return chosen
}

// muted is secondary text, as units, counts and hints.
func muted(c *ui.Context, text string) ui.Element {
	return ui.Text(c, text).TextColor(c.Theme().TextMuted)
}

// Boundaries are quieter than input borders. Keep them in the same native
// divider system across the workbench, settings and review windows.
func separator(c *ui.Context) ui.Element {
	return ui.Divider(c).Background(studioColors(c).hairline)
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
		// Shared edges and heading spacing define the group. An extra card
		// around every group crowds narrow panes and duplicates modal borders.
		ui.Column(c).Gap(em(c, 16)).Children(fields)
	})
}

// parameterGrid sets a deliberate density ceiling instead of packing every
// field that happens to fit on one line: at most maximum cells a line, each
// at least minimum wide; narrow panes become one column without shrinking
// text. Native flex wrapping decides the lines from the real width in the
// same layout pass. No size measured in an earlier frame decides the
// layout, so a page is placed correctly in its first frame (see
// layout_frame_test.go).
func parameterGrid(c *ui.Context, minimum float32, maximum int, cells ...func()) ui.Element {
	// A basis between 1/(maximum+1) and 1/maximum of the line lets at most
	// maximum cells share it; growing fills it.
	share := 50 * (1/float32(maximum) + 1/float32(maximum+1))
	return ui.Row(c).Wrap().GapX(em(c, 20)).GapY(em(c, 16)).AlignItems(ui.Start).Children(func() {
		for _, cell := range cells {
			ui.Column(c).Grow(1).BasisPercent(share).MinWidth(em(c, minimum)).Children(cell)
		}
	})
}
