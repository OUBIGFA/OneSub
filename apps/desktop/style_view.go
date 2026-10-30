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
	ui.Column(c).Fill().Label(locales.A11yStylePanel()).Children(func() {
		panelHeader(c, nil, func() { panelTitle(c, locales.StyleTitle()) }, nil)
		ui.Divider(c)
		switch {
		case !hasCues:
			ui.Column(c).Grow(1).Center().Children(func() { muted(c, locales.ListEmpty()) })
		case !ready:
			ui.Column(c).Grow(1).Center().Children(func() { muted(c, locales.StyleLoading()) })
		default:
			ui.Scroll(c).Grow(1).TrackScroll(&a.styleScroll).Children(func() {
				content := ui.Column(c).Padding(em(c, 12), em(c, panelPad)).Gap(em(c, 20))
				a.styleRoom = content.Bounds().W - 2*em(c, panelPad)
				content.Children(func() {
					line(c, func() {
						a.outputFields(c, false)
						if !ass {
							ui.Field(c, locales.StylePreset(), func() {
								ui.Row(c).Gap(em(c, 8)).AlignItems(ui.Center).Children(func() { a.presetControls(c, layer) })
							})
						}
					})
					if ass {
						muted(c, locales.StyleKeepAss())
						return
					}
					if a.settings.SubtitleOutput.Normalized().Mode == "bilingual" {
						ui.Segmented(c, &a.styleLayer, locales.StylePrimaryLayer(), locales.StyleSecondaryLayer()).Label(locales.A11yStyleLayers()).AlignSelf(ui.Start)
					}
					a.styleControls(c, layer)
				})
				measurePanelContent(c, content, &a.styleHeight)
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

// Both layers edit SmartSub's existing schema. Wide panes use three equal
// native grid columns. Narrower panes group short parameters into wrapping
// rows, instead of stacking three tall one-field-per-line inspectors.
func (a *NativeApp) styleControls(c *ui.Context, layer int) {
	style, _ := a.layerStyle(layer)
	columns := 1
	if a.styleRoom >= em(c, 960) {
		columns = 3
	}
	ui.Grid(c).Columns(columns).Gap(em(c, 24)).AlignItems(ui.Start).Children(func() {
		a.textSection(c, style)
		a.appearanceSection(c, style)
		if layer == 0 {
			a.layoutSection(c, style)
		}
	})
	if layer == 1 {
		muted(c, locales.StyleSecondaryLayout())
	}
}

func (a *NativeApp) textSection(c *ui.Context, style *styles.Style) {
	fonts := []string{}
	for _, font := range a.styleCatalog.Fonts {
		if font.Available {
			fonts = append(fonts, font.Name)
		}
	}
	ui.Fieldset(c, locales.StyleText(), func() {
		line(c, func() {
			inlineField(c, locales.StyleFontShort(), func() {
				ui.Row(c).Gap(em(c, 4)).AlignItems(ui.Center).Children(func() {
					ui.Combobox(c, &style.FontName, fonts).Label(locales.StyleFont()).Width(em(c, 176))
					ui.Toolbar(c, func() {
						toolIcon(c, locales.StyleReloadFonts(), "rotate-cw").Disabled(a.fontCatalogLoading).OnClick(a.reloadFonts)
					}).Padding(0)
				})
			})
			inlineField(c, locales.StyleFontSize(), func() { number(c, locales.StyleFontSize(), &style.FontSize, 10, 72, 1) })
			inlineField(c, locales.StyleColor(), func() { colorWell(c, locales.StyleTextColor(), &style.PrimaryColor) })
			inlineField(c, locales.StyleEmphasis(), func() {
				ui.ToggleGroup(c, func() {
					ui.Toggle(c, &style.Bold, "").Label(locales.StyleBold()).Tooltip(locales.StyleBold()).Children(func() { icon(c, "bold") })
					ui.Toggle(c, &style.Italic, "").Label(locales.StyleItalic()).Tooltip(locales.StyleItalic()).Children(func() { icon(c, "italic") })
					ui.Toggle(c, &style.Underline, "").Label(locales.StyleUnderline()).Tooltip(locales.StyleUnderline()).Children(func() { icon(c, "underline") })
				}).Label(locales.StyleEmphasis()).AlignSelf(ui.Start)
			})
		}).AlignItems(ui.Center)
	}).Margin(0)
}

// boxOpacity is SmartSub's default opacity of the background box. A box
// chosen while the opacity is 0, OneSub's default for the shadow, gets it:
// a box no one can see is not what choosing one asks for.
const boxOpacity = 50

func (a *NativeApp) appearanceSection(c *ui.Context, style *styles.Style) {
	ui.Fieldset(c, locales.StyleAppearance(), func() {
		line(c, func() {
			// SmartSub maps BackColor to either a shadow or a background box.
			// Only the parameters for the selected border mode are presented.
			box := style.BorderStyle == 3
			border := 0
			if box {
				border = 1
			}
			inlineField(c, locales.StyleBorder(), func() {
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
				inlineField(c, locales.StylePadding(), func() { number(c, locales.StyleBoxPadding(), &style.Outline, 0, 10, .1) })
				inlineField(c, locales.StyleBackground(), func() { colorWell(c, locales.StyleBoxColor(), &style.BackColor) })
				inlineField(c, locales.StyleBoxOpacityShort(), func() { percent(c, locales.StyleBoxOpacity(), &style.BackOpacity) })
				return
			}
			inlineField(c, locales.StyleBorderOutline(), func() {
				ui.Row(c).Gap(em(c, 8)).AlignItems(ui.Center).Children(func() {
					colorWell(c, locales.StyleOutlineColor(), &style.OutlineColor)
					number(c, locales.StyleOutlineWidth(), &style.Outline, 0, 10, .1)
				})
			})
			inlineField(c, locales.StyleShadow(), func() {
				ui.Row(c).Gap(em(c, 8)).AlignItems(ui.Center).Children(func() {
					colorWell(c, locales.StyleShadowColor(), &style.BackColor)
					number(c, locales.StyleShadowDistance(), &style.Shadow, 0, 10, .1)
				})
			})
			inlineField(c, locales.StyleShadowOpacityShort(), func() { percent(c, locales.StyleShadowOpacity(), &style.BackOpacity) })
		}).AlignItems(ui.Center)
	}).Margin(0)
}

func (a *NativeApp) layoutSection(c *ui.Context, style *styles.Style) {
	ui.Fieldset(c, locales.StyleLayout(), func() {
		line(c, func() {
			// ASS numpad alignment: rows from the bottom, columns from the left.
			row, column := (style.Alignment-1)/3, (style.Alignment-1)%3
			inlineField(c, locales.StyleAlign(), func() {
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
			inlineField(c, locales.StylePosition(), func() {
				ui.Segmented(c, &row, locales.PositionBottom(), locales.PositionMiddle(), locales.PositionTop()).Label(locales.StylePosition()).AlignSelf(ui.Start)
			})
			style.Alignment = row*3 + column + 1
			inlineField(c, locales.StyleMarginLeft(), func() { number(c, locales.StyleMarginLeft(), &style.MarginL, 0, 384, 1) })
			inlineField(c, locales.StyleMarginRight(), func() { number(c, locales.StyleMarginRight(), &style.MarginR, 0, 384, 1) })
			inlineField(c, locales.StyleMarginVertical(), func() { number(c, locales.StyleMarginVertical(), &style.MarginV, 0, 288, 1) })
		}).AlignItems(ui.Center)
	}).Margin(0)
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
