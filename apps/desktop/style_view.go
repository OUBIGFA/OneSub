package main

import (
	"fmt"
	"strings"

	"github.com/OUBIGFA/OneSub/apps/desktop/internal/settings"
	"github.com/OUBIGFA/OneSub/apps/desktop/internal/styles"
	"github.com/OUBIGFA/OneSub/apps/desktop/locales"
	"github.com/egoist/mygo/ui"
	"github.com/google/uuid"
)

// stylePanel is the inspector of the subtitle style: the preset in its
// header, then what the subtitles show and their look in native forms, as
// video editors' inspectors group properties under section names.
func (a *NativeApp) stylePanel(c *ui.Context) {
	hasCues := a.snapshot != nil && len(a.snapshot.Document.Cues) > 0
	ready := hasCues && len(a.styleCatalog.Presets) > 0
	ass := a.snapshot != nil && (a.snapshot.Document.Format == "ass" || a.snapshot.Document.Format == "ssa")
	layer := a.editedLayer()
	ui.Column(c).Fill().Children(func() {
		panelHeader(c, func() { panelTitle(c, locales.StyleTitle()) }, nil, func() {
			if ready && !ass {
				a.presetControls(c, layer)
			}
		})
		ui.Divider(c)
		switch {
		case !hasCues:
			ui.Column(c).Grow(1).Center().Children(func() { muted(c, locales.ListEmpty()) })
		case !ready:
			ui.Column(c).Grow(1).Center().Children(func() { muted(c, locales.StyleLoading()) })
		default:
			ui.Scroll(c).Grow(1).Children(func() {
				content := ui.Column(c).Padding(em(c, 12), em(c, panelPad)).Gap(em(c, 16))
				// The room the sections have, from the last frame's layout.
				a.styleRoom = content.Bounds().W - 2*em(c, panelPad)
				content.Children(func() {
					ui.Form(c, func() { a.outputFields(c, false) })
					if ass {
						muted(c, locales.StyleKeepAss())
						return
					}
					if a.settings.SubtitleOutput.Normalized().Mode == "bilingual" {
						ui.Segmented(c, &a.styleLayer, locales.StylePrimaryLayer(), locales.StyleSecondaryLayer()).Label(locales.A11yStyleLayers()).AlignSelf(ui.Start)
					}
					a.styleControls(c, layer)
				})
			})
		}
	})
}

// presetControls choose a preset; its actions are in one menu, as style
// menus of editors are. Saving always makes a new preset; SmartSub's presets
// and earlier copies stay as they are, and only the user's own are renamed
// or deleted.
func (a *NativeApp) presetControls(c *ui.Context, layer int) {
	style, preset := a.layerStyle(layer)
	idSelect(c, preset, a.presetOptions(*preset)).Width(em(c, 200)).Label(locales.StylePreset()).Tooltip(locales.StylePreset()).OnChange(func() {
		if chosen, ok := a.presetStyle(*preset); ok {
			*style = chosen
		}
	})
	ui.Toolbar(c, func() {
		toolIcon(c, locales.PresetActions(), "ellipsis").Menu(func(m *ui.Menu) {
			if m.Item(locales.PresetSaveNew()).Chosen() {
				a.presetDialog, a.presetDialogLayer = "save", layer
				a.presetName = locales.PresetNewName(len(a.settings.UserPresets) + 1)
			}
			own := a.userPreset(*preset) >= 0
			if m.Item(locales.PresetRename()).Disabled(!own).Chosen() && own {
				a.presetDialog, a.presetDialogLayer = "rename", layer
				a.presetName = a.settings.UserPresets[a.userPreset(*preset)].Name
			}
			if m.Item(locales.PresetDelete()).Disabled(!own).Chosen() && own {
				a.presetDialog, a.presetDialogLayer = "delete", layer
			}
		})
	}).Padding(0)
}

// editedLayer is 1 while the secondary style of bilingual output is shown.
func (a *NativeApp) editedLayer() int {
	if a.settings.SubtitleOutput.Normalized().Mode == "bilingual" && a.styleLayer == 1 {
		return 1
	}
	return 0
}
func (a *NativeApp) layerStyle(layer int) (*styles.Style, *string) {
	if layer == 1 {
		return &a.secondaryStyle, &a.secondaryPreset
	}
	return &a.currentStyle, &a.stylePreset
}

// presetName says SmartSub's built-in presets in the app's language.
func presetName(p styles.Preset) string {
	switch p.ID {
	case "bilibili_knowledge":
		return locales.PresetBilibili()
	case "netflix_bilingual":
		return locales.PresetNetflix()
	case "variety_glow":
		return locales.PresetVariety()
	case "classic":
		return locales.PresetClassic()
	case "movie":
		return locales.PresetMovie()
	case "youtube":
		return locales.PresetYoutube()
	case "black_box":
		return locales.PresetBlackBox()
	case "clean":
		return locales.PresetClean()
	case "bold_impact":
		return locales.PresetBold()
	}
	return p.Name
}

// presetOptions lists the default style, SmartSub's presets and the user's
// own copies. A style that matches none of them is shown as custom.
func (a *NativeApp) presetOptions(current string) []option {
	options := []option{{"default", locales.PresetDefault()}}
	for _, p := range a.styleCatalog.Presets {
		options = append(options, option{p.ID, presetName(p)})
	}
	for _, p := range a.settings.UserPresets {
		options = append(options, option{p.ID, p.Name})
	}
	for _, o := range options {
		if o.ID == current {
			return options
		}
	}
	return append(options, option{current, locales.PresetCustom()})
}
func (a *NativeApp) presetLabel(id string) string {
	for _, o := range a.presetOptions(id) {
		if o.ID == id {
			return o.Label
		}
	}
	return locales.PresetCustom()
}
func (a *NativeApp) presetStyle(id string) (styles.Style, bool) {
	if id == "default" {
		return a.styleCatalog.DefaultStyle, true
	}
	for _, p := range a.styleCatalog.Presets {
		if p.ID == id {
			return p.Style, true
		}
	}
	for _, p := range a.settings.UserPresets {
		if p.ID == id {
			return p.Style, true
		}
	}
	return styles.Style{}, false
}
func (a *NativeApp) userPreset(id string) int {
	for i, p := range a.settings.UserPresets {
		if p.ID == id {
			return i
		}
	}
	return -1
}

// colorWell edits a #RRGGBB color; its name is the property it paints.
func colorWell(c *ui.Context, label string, value *string) ui.Element {
	color := ui.Hex(*value)
	return ui.ColorWell(c, &color).Label(label).Tooltip(label).AlignSelf(ui.Start).OnChange(func() { *value = fmt.Sprintf("#%02X%02X%02X", color.R, color.G, color.B) })
}

// number is a native number input at its own size: a narrower one would
// draw its step buttons over what follows.
func number(c *ui.Context, label string, value *float64, low, high, step float64) ui.Element {
	return ui.NumberInput(c, value, low, high, step).AlignSelf(ui.Start).Label(label).Tooltip(label)
}

// percent is a number input for an opacity, with its unit after it.
func percent(c *ui.Context, label string, value *float64) {
	ui.Row(c).Gap(em(c, 8)).AlignItems(ui.Center).Children(func() {
		number(c, label, value, 0, 100, 1)
		muted(c, "%").SingleLine()
	})
}

// Both layers edit SmartSub's existing schema. Bilingual block placement is
// shared; secondary typography and appearance remain independently editable.
//
// The sections, the text, its appearance and the main layer's layout, are
// each as wide as their fields and spread across the panel's whole width:
// the room left over is shared equally on both sides of the thin lines
// between them, so the spaces are even and the last section ends at the
// panel's edge. As many sit side by side as the panel has room for, by
// their widths in the last frame, which their content alone sets; the
// others go on new lines, under a line across.
func (a *NativeApp) styleControls(c *ui.Context, layer int) {
	style, _ := a.layerStyle(layer)
	sections := []func(){
		func() { a.textSection(c, style) },
		func() { a.appearanceSection(c, style) },
	}
	if layer == 0 {
		sections = append(sections, func() { a.layoutSection(c, style) })
	}
	room := a.styleRoom
	if room <= 0 {
		width, _ := c.Size()
		room = width*(100-a.bottomSplit)/100*a.styleSplit/100 - 2*em(c, panelPad)
	}
	gap := em(c, 16)
	perLine := len(sections)
	for perLine > 1 && !a.sectionsFit(perLine, len(sections), room, gap) {
		perLine--
	}
	lines := ui.Column(c).Gap(em(c, 20))
	// Until every section was measured, the sections are laid out but not
	// shown: they appear at once where they belong, a frame or two later.
	for i := range sections {
		if a.styleWidths[i] == 0 {
			lines.Invisible()
		}
	}
	lines.Children(func() {
		for first := 0; first < len(sections); first += perLine {
			if first > 0 {
				ui.Divider(c)
			}
			ui.Row(c.Key(fmt.Sprint("line-", first))).AlignItems(ui.Stretch).Children(func() {
				for i := first; i < min(first+perLine, len(sections)); i++ {
					if i > first {
						// A vertical line, as tall as the line, centered in
						// the space between two sections.
						ui.Spacer(c).MinWidth(gap)
						ui.Divider(c)
						ui.Spacer(c).MinWidth(gap)
					}
					box := ui.Column(c.Key(fmt.Sprint("section-", i))).Shrink(0).Children(sections[i])
					// A section the last frame lacked has no width yet: keep
					// the one it had, or none, which stacks the sections.
					if w := box.Bounds().W; w > 0 && w != a.styleWidths[i] {
						a.styleWidths[i] = w
						c.After(0)
					}
				}
			})
		}
	})
	if layer == 1 {
		muted(c, locales.StyleSecondaryLayout())
	}
}

// sectionsFit reports whether count sections, n to a line, fit in room. A
// section not measured yet fits beside no other.
func (a *NativeApp) sectionsFit(n, count int, room, gap float32) bool {
	for start := 0; start < count; start += n {
		width := float32(0)
		for i := start; i < min(start+n, count); i++ {
			if a.styleWidths[i] == 0 {
				return false
			}
			if i > start {
				width += 2*gap + 1
			}
			width += a.styleWidths[i]
		}
		if width > room {
			return false
		}
	}
	return true
}

func (a *NativeApp) textSection(c *ui.Context, style *styles.Style) {
	fonts := []string{}
	for _, font := range a.styleCatalog.Fonts {
		if font.Available {
			fonts = append(fonts, font.Name)
		}
	}
	ui.Form(c, func() {
		ui.Fieldset(c, locales.StyleText(), func() {
			ui.Field(c, locales.StyleFontShort(), func() {
				ui.Row(c).Gap(em(c, 4)).AlignItems(ui.Center).Children(func() {
					ui.Combobox(c, &style.FontName, fonts).Label(locales.StyleFont()).Width(em(c, 176))
					ui.Toolbar(c, func() {
						toolIcon(c, locales.StyleReloadFonts(), "rotate-cw").Disabled(a.fontCatalogLoading).OnClick(a.reloadFonts)
					}).Padding(0)
				})
			})
			ui.Field(c, locales.StyleFontSize(), func() { number(c, locales.StyleFontSize(), &style.FontSize, 10, 72, 1) })
			ui.Field(c, locales.StyleColor(), func() { colorWell(c, locales.StyleTextColor(), &style.PrimaryColor) })
			ui.Field(c, locales.StyleEmphasis(), func() {
				ui.ToggleGroup(c, func() {
					ui.Toggle(c, &style.Bold, "").Label(locales.StyleBold()).Tooltip(locales.StyleBold()).Children(func() { icon(c, "bold") })
					ui.Toggle(c, &style.Italic, "").Label(locales.StyleItalic()).Tooltip(locales.StyleItalic()).Children(func() { icon(c, "italic") })
					ui.Toggle(c, &style.Underline, "").Label(locales.StyleUnderline()).Tooltip(locales.StyleUnderline()).Children(func() { icon(c, "underline") })
				}).Label(locales.StyleEmphasis()).AlignSelf(ui.Start)
			})
		})
	})
}

// boxOpacity is SmartSub's default opacity of the background box. A box
// chosen while the opacity is 0, OneSub's default for the shadow, gets it:
// a box no one can see is not what choosing one asks for.
const boxOpacity = 50

func (a *NativeApp) appearanceSection(c *ui.Context, style *styles.Style) {
	ui.Form(c, func() {
		// Outline and box each show only their own parameters. In SmartSub's
		// mapping the back colour is the shadow in outline mode and the box
		// in box mode, so it is named after what it paints.
		ui.Fieldset(c, locales.StyleAppearance(), func() {
			box := style.BorderStyle == 3
			border := 0
			if box {
				border = 1
			}
			ui.Field(c, locales.StyleBorder(), func() {
				if ui.Segmented(c, &border, locales.StyleBorderOutline(), locales.StyleBorderBox()).Label(locales.StyleBorder()).AlignSelf(ui.Start).Changed() {
					style.BorderStyle = 1
					if border == 1 {
						style.BorderStyle = 3
						if style.BackOpacity == 0 {
							style.BackOpacity = boxOpacity
						}
					}
				}
			})
			if box {
				ui.Field(c, locales.StylePadding(), func() { number(c, locales.StyleBoxPadding(), &style.Outline, 0, 10, .1) })
				ui.Field(c, locales.StyleBackground(), func() { colorWell(c, locales.StyleBoxColor(), &style.BackColor) })
				ui.Field(c, locales.StyleBoxOpacityShort(), func() { percent(c, locales.StyleBoxOpacity(), &style.BackOpacity) })
				return
			}
			// A color and its size share a row: the color well, then the value.
			ui.Field(c, locales.StyleBorderOutline(), func() {
				ui.Row(c).Gap(em(c, 8)).AlignItems(ui.Center).Children(func() {
					colorWell(c, locales.StyleOutlineColor(), &style.OutlineColor)
					number(c, locales.StyleOutlineWidth(), &style.Outline, 0, 10, .1)
				})
			})
			ui.Field(c, locales.StyleShadow(), func() {
				ui.Row(c).Gap(em(c, 8)).AlignItems(ui.Center).Children(func() {
					colorWell(c, locales.StyleShadowColor(), &style.BackColor)
					number(c, locales.StyleShadowDistance(), &style.Shadow, 0, 10, .1)
				})
			})
			ui.Field(c, locales.StyleShadowOpacityShort(), func() { percent(c, locales.StyleShadowOpacity(), &style.BackOpacity) })
		})
	})
}

func (a *NativeApp) layoutSection(c *ui.Context, style *styles.Style) {
	ui.Form(c, func() {
		ui.Fieldset(c, locales.StyleLayout(), func() {
			// ASS numpad alignment: rows from the bottom, columns from the left.
			row, column := (style.Alignment-1)/3, (style.Alignment-1)%3
			ui.Field(c, locales.StyleAlign(), func() {
				ui.ToggleGroup(c, func() {
					for i, name := range []string{"align-left", "align-center", "align-right"} {
						on := column == i
						label := []string{locales.AlignLeft(), locales.AlignCenterH(), locales.AlignRight()}[i]
						if ui.Toggle(c, &on, "").Label(label).Tooltip(label).Children(func() { icon(c, name) }).Changed() {
							column = i
						}
					}
				}).Label(locales.StyleAlign()).AlignSelf(ui.Start)
			})
			ui.Field(c, locales.StylePosition(), func() {
				ui.Segmented(c, &row, locales.PositionBottom(), locales.PositionMiddle(), locales.PositionTop()).Label(locales.StylePosition()).AlignSelf(ui.Start)
			})
			style.Alignment = row*3 + column + 1
			ui.Field(c, locales.StyleMarginLeft(), func() { number(c, locales.StyleMarginLeft(), &style.MarginL, 0, 384, 1) })
			ui.Field(c, locales.StyleMarginRight(), func() { number(c, locales.StyleMarginRight(), &style.MarginR, 0, 384, 1) })
			ui.Field(c, locales.StyleMarginVertical(), func() { number(c, locales.StyleMarginVertical(), &style.MarginV, 0, 288, 1) })
		})
	})
}

// presetDialogView names a new or renamed preset, or confirms a deletion
// in an alert.
func (a *NativeApp) presetDialogView(c *ui.Context) {
	style, preset := a.layerStyle(a.presetDialogLayer)
	index := a.userPreset(*preset)
	if a.presetDialog == "delete" {
		open := index >= 0
		if !open {
			a.presetDialog = ""
			return
		}
		switch ui.AlertDialog(c, &open, locales.PresetDeleteTitle(a.settings.UserPresets[index].Name), locales.PresetDeleteBody(), locales.ActionCancel(), locales.PresetDeleteConfirm()) {
		case 1:
			// The current values stay; only the saved copy goes.
			a.settings.UserPresets = append(a.settings.UserPresets[:index], a.settings.UserPresets[index+1:]...)
			*preset = "custom"
			a.saveSettings()
		}
		if !open {
			a.presetDialog = ""
		}
		return
	}
	open := a.presetDialog != ""
	ui.Modal(c, &open, func() {
		ui.Column(c).Width(em(c, 440)).Gap(em(c, 16)).Children(func() {
			title, confirm := locales.PresetSaveNew(), locales.PresetSaveConfirm()
			if a.presetDialog == "rename" {
				title, confirm = locales.PresetRename(), locales.PresetRenameConfirm()
			}
			dialogTitle(c, title)
			ui.TextInput(c, &a.presetName).AutoFocus().Label(locales.PresetNameLabel()).Placeholder(locales.PresetNameLabel())
			name := strings.TrimSpace(a.presetName)
			ui.Row(c).Gap(em(c, 8)).Justify(ui.End).Children(func() {
				ui.Button(c, locales.ActionCancel()).OnClick(func() { a.presetDialog = "" })
				ui.PrimaryButton(c, confirm).Disabled(name == "").OnClick(func() {
					if a.presetDialog == "rename" && index >= 0 {
						a.settings.UserPresets[index].Name = name
					} else {
						id := "user-" + uuid.NewString()
						a.settings.UserPresets = append(a.settings.UserPresets, settings.UserPreset{ID: id, Name: name, Style: *style})
						*preset = id
					}
					a.presetDialog = ""
					a.saveSettings()
				})
			})
		})
	})
	if !open {
		a.presetDialog = ""
	}
}
