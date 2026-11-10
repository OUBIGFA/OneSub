package main

import (
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo"
	"github.com/egoist/mygo/ui"
	lobeAction "github.com/lobehub/lobe-mygo/pkg/action"
	lobeChoice "github.com/lobehub/lobe-mygo/pkg/choice"
	lobeDisplay "github.com/lobehub/lobe-mygo/pkg/display"
	lobeFeedback "github.com/lobehub/lobe-mygo/pkg/feedback"
	lobeField "github.com/lobehub/lobe-mygo/pkg/field"
	lobeForm "github.com/lobehub/lobe-mygo/pkg/form"
	lobeNav "github.com/lobehub/lobe-mygo/pkg/nav"
	lobeTheme "github.com/lobehub/lobe-mygo/pkg/theme"
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

// Switch foregrounds and surfaces atomically. Lobe's color transitions would
// otherwise paint the previous light fill underneath the new dark text until
// their next animation frame. Ordinary hover and subtitle motion stay enabled.
func applyStudioTheme(c *ui.Context) studio {
	t, look := studioTheme(c.Theme(), c.Preferences().HighContrast)
	type appearance struct{ dark, contrast bool }
	now := appearance{t.Dark, c.Preferences().HighContrast}
	previous := ui.Local(c.Root(), "studio-appearance", func() appearance { return now })
	if *previous != now {
		c.DisableAnimations()
		*previous = now
	}
	c.SetTheme(&t)
	// Text-edit and table-header system commands stay native, but render in
	// the same themed window rather than opening an unrelated Windows menu.
	c.SetMenuPresentation(ui.PopoverMenus)
	return look
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

// iconButton is a native button with an icon before its label. MyGo centers
// line boxes; raise the system font's visible ink by one DIP to align it with
// the icon's center. This native transform scales with display DPI, not layout.
func iconButton(c *ui.Context, label, name string) ui.Element {
	return lobeAction.Button(c, "", lobeAction.ButtonProps{}).Label(label).Children(func() {
		icon(c, name).Label(locales.A11yIconOf(label))
		ui.Text(c, label).SingleLine().Translate(0, -1)
	})
}

// primaryIconButton is iconButton in the accent color, one per area.
func primaryIconButton(c *ui.Context, label, name string) ui.Element {
	return lobeAction.Button(c, "", lobeAction.ButtonProps{Variant: lobeAction.Primary}).Label(label).Children(func() {
		icon(c, name).Label(locales.A11yIconOf(label))
		ui.Text(c, label).SingleLine().Translate(0, -1)
	})
}

// toolIcon is an icon-only native button, square and with the icon in its
// middle; the tooltip and the accessible name carry the action.
func toolIcon(c *ui.Context, label, name string) ui.Element {
	return lobeAction.ActionIcon(c, icons[name], lobeAction.ActionIconProps{Size: lobeTheme.Middle, IconLabel: locales.A11yIconOf(label)}).Label(label).Tooltip(label).Size(em(c, 32), em(c, 32))
}

// The wrappers keep the existing view code's event chains while replacing each
// primitive with its Lobe MyGo counterpart. This is one migration seam, so
// later upstream kit updates do not require editing every view.
func lobeButton(c *ui.Context, label string) ui.Element {
	return lobeAction.Button(c, label, lobeAction.ButtonProps{})
}

func lobePrimaryButton(c *ui.Context, label string) ui.Element {
	return lobeAction.Button(c, label, lobeAction.ButtonProps{Variant: lobeAction.Primary})
}

func lobeTextInput(c *ui.Context, value *string) ui.Element {
	return lobeField.Input(c, value, lobeField.InputProps{Variant: lobeField.InputOutlined})
}

func lobeTextArea(c *ui.Context, value *string) ui.Element {
	return lobeField.TextArea(c, value, lobeField.TextAreaProps{Variant: lobeField.InputOutlined, Fixed: true})
}

// Single-line content reveals its full value only when it has been elided.
func elidedText(c *ui.Context, value string) ui.Element {
	return lobeField.ChoiceLabel(c, value)
}

func lobeSearchField(c *ui.Context, value *string, changed func(), frame ...func(ui.Element)) ui.Element {
	props := lobeField.SearchBarProps{Variant: lobeField.InputOutlined, OnClear: changed, ClearLabel: locales.ActionClearInput()}
	if len(frame) > 0 {
		props.Frame = frame[0]
	}
	return lobeField.SearchBar(c, value, props).OnChange(changed)
}

func lobeNumberInput(c *ui.Context, value *float64, low, high, step float64, disabled bool, changed ...func()) ui.Element {
	props := lobeField.InputNumberProps{Min: low, Max: high, Step: step, Variant: lobeField.InputOutlined, Disabled: disabled, IncreaseLabel: locales.ActionIncrease(), DecreaseLabel: locales.ActionDecrease()}
	if len(changed) > 0 {
		props.OnChange = func(float64) { changed[0]() }
	}
	return lobeField.InputNumber(c, value, props)
}

func lobeSelect(c *ui.Context, selected *string, options []string) ui.Element {
	items := make([]lobeField.SelectOption, len(options))
	for i, option := range options {
		items[i] = lobeField.SelectOption{Value: option, Label: option}
	}
	return lobeField.Select(c, selected, lobeField.SelectProps{Options: items, Variant: lobeField.InputOutlined})
}

func lobeCombobox(c *ui.Context, selected *string, options []string, frame ...func(ui.Element)) ui.Element {
	items := make([]lobeField.AutoCompleteOption, len(options))
	for i, option := range options {
		items[i] = lobeField.AutoCompleteOption{Value: option, Label: option}
	}
	props := lobeField.AutoCompleteProps{Options: items, Variant: lobeField.InputOutlined, DropdownLabel: locales.ActionShowChoices()}
	if len(frame) > 0 {
		props.Frame = frame[0]
	}
	return lobeField.AutoComplete(c, selected, props)
}

func lobeCheckbox(c *ui.Context, checked *bool, label string) ui.Element {
	return lobeChoice.Checkbox(c, checked, label, lobeChoice.CheckboxProps{})
}

func lobeToggle(c *ui.Context, checked *bool, label string) ui.Element {
	variant := lobeAction.Filled
	if *checked {
		variant = lobeAction.Default
	}
	// StyleButton supports native button bases, including ToggleBase. Keep
	// the toggle's bound value, keyboard behavior and change notifications.
	button := lobeAction.StyleButton(c, ui.ToggleBase(c, checked), label, lobeAction.ButtonProps{Variant: variant}).Label(label)
	// Match Lobe's segmented selection: an elevated selected surface over a
	// neutral track, independent of the containing page's background.
	tokens := lobeTheme.TokensOf(c)
	background := tokens.BgLayout
	if *checked {
		background = tokens.BgElevated
	}
	return button.Background(background)
}

func lobeSlider(c *ui.Context, value *float64, low, high float64) ui.Element {
	return lobeChoice.Slider(c, value, lobeChoice.SliderProps{Min: low, Max: high})
}

func lobeTabs(c *ui.Context, selected *int, labels ...string) ui.Element {
	items := make([]lobeNav.TabItem, len(labels))
	for i, label := range labels {
		items[i] = lobeNav.TabItem{Label: label}
	}
	tabs := lobeNav.Tabs(c, selected, lobeNav.TabsProps{Items: items, Variant: lobeNav.TabsSquare}, nil)
	if c.Theme().Dark {
		tabs.InsetShadow(0, -1, 0, 0, darkTabBorder(c))
	}
	return tabs
}

// Navigation tabs and segmented property tabs share the same visible dark
// boundary. Keep the native one-DIP outline and selection styling in both.
func darkTabBorder(c *ui.Context) ui.Color {
	if c.Preferences().HighContrast {
		return c.Theme().Border
	}
	return lobeTheme.Dark().FillBorderStrong()
}

func lobeProgress(c *ui.Context, fraction float64) ui.Element {
	return lobeFeedback.Progress(c, float32(fraction*100), lobeFeedback.ProgressProps{})
}

func lobeBadge(c *ui.Context, text string) ui.Element {
	return lobeDisplay.Tag(c, text, lobeDisplay.TagProps{}).Height(em(c, 24)).Padding(em(c, 2), em(c, 8)).FontSize(em(c, captionSize)).AlignItems(ui.Center)
}

func lobeColorPicker(c *ui.Context, color *ui.Color) ui.Element {
	return lobeChoice.ColorPicker(c, color, lobeChoice.ColorPickerProps{Trigger: lobeChoice.ColorTriggerField})
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
	lobeFeedback.Modal(c, open, lobeFeedback.ModalProps{Width: min(em(c, width+40), vw-em(c, 40)), NoFooter: true, Instant: true, Panel: func(panel ui.Element) {
		// The Lobe modal sizes all fields in this frame, without wrapping the
		// whole form in a scroll view or leaving a dismissing panel mounted.
		shell := panel.Column().MinWidth(min(em(c, width+40), vw-em(c, 40))).MaxWidth(vw - em(c, 40)).Label(title)
		if height > 0 {
			shell.Height(min(em(c, height+40), vh-em(c, 40)))
		}
		shell.Children(func() {
			ui.Row(c).Padding(em(c, 16), em(c, 24)).Gap(em(c, 16)).AlignItems(ui.Center).Shrink(0).Children(func() {
				ui.Text(c, title).FontSize(em(c, 20)).FontWeight(500).Grow(1)
				if actions == nil {
					lobeButton(c, locales.ActionClose()).OnClick(func() { *open = false })
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
				// Flush surfaces inherit the modal's outer clipping, so their
				// corners cannot disagree with its border or the divider above.
				ui.Row(c).Padding(em(c, 14), em(c, 24)).Gap(em(c, 8)).Justify(ui.End).AlignItems(ui.Center).Shrink(0).Background(look.chrome).Children(actions)
			}
		})
	}}, nil)
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
			button := lobeButton
			if i == len(buttons)-1 {
				button = lobePrimaryButton
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
	lobeGroup(c, title, action, fields)
}

// Use Lobe's borderless form section, without introducing a second preference
// store: the existing application remains the owner of values and validation.
func lobeGroup(c *ui.Context, title string, action, fields func()) ui.Element {
	var form lobeForm.Form[struct{}]
	return lobeForm.Group(c, &form, lobeForm.GroupProps{Title: title, Extra: action, Fixed: true}, func() {
		ui.Column(c).Gap(em(c, 16)).Padding(em(c, 16), 0, 0, 0).Children(fields)
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
